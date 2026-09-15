package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// the rate the game itself runs at: one input per tick. snapshots may come
	// less often, and the server says how often in its welcome
	tickRate     = 60
	tickInterval = time.Second / tickRate

	pingInterval = time.Second
	writeWait    = 5 * time.Second
)

var (
	// messages are told apart by their first bytes. the server always writes
	// the type first, and decoding a whole snapshot sixty times a second per
	// player just to read its type would make the load test the bottleneck
	stateTag = []byte(`{"type":"state"`)
	pongTag  = []byte(`{"type":"pong"`)
	sentAtAt = []byte(`"sentAt":`)
)

// metrics is shared by every bot. all of it is atomic, for the same reason
// the histograms are: the counters must not add contention of their own.
type metrics struct {
	connected   atomic.Int64
	connectFail atomic.Int64
	dropped     atomic.Int64

	snapshots atomic.Int64
	bytes     atomic.Int64

	// the rate the server says snapshots go out at, taken from the first
	// welcome. zero until then, which report treats as the tick rate
	snapshotRate atomic.Int64

	gaps *histogram
	rtts *histogram
}

// window is the stretch of time that counts. it is fixed before any bot
// starts and never changes, so bots read it without coordinating.
type window struct {
	from, to time.Time
}

func (w window) contains(t time.Time) bool { return !t.Before(w.from) && t.Before(w.to) }

// bot is one simulated player: it joins a room, readies up, and then behaves
// like a real browser tab. one numbered input every tick, a ping every second.
type bot struct {
	url  string
	name string
	m    *metrics
	win  window
}

// run joins the room with the given code, or opens a fresh one when the code
// is empty. the room it ended up in is passed on through joined, so the second
// player of the pair can follow it the way an invite link would.
func (b *bot) run(ctx context.Context, code string, joined chan<- string) {
	conn, roomID, err := b.dial(ctx, code)
	if err != nil {
		b.m.connectFail.Add(1)
		if joined != nil {
			close(joined)
		}
		return
	}
	b.m.connected.Add(1)
	if joined != nil {
		joined <- roomID
	}

	done := make(chan struct{})
	go b.read(ctx, conn, done)

	b.write(ctx, conn)
	conn.Close()
	<-done
}

func (b *bot) dial(ctx context.Context, code string) (*websocket.Conn, string, error) {
	u, err := url.Parse(b.url)
	if err != nil {
		return nil, "", err
	}
	if code != "" {
		q := u.Query()
		q.Set("room", code)
		u.RawQuery = q.Encode()
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, "", err
	}

	// the welcome is the only message worth decoding properly: it names the
	// room, which the second player of the pair needs
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var welcome struct {
		Data struct {
			RoomID string `json:"roomId"`
			Arena  struct {
				SnapshotRate int64 `json:"snapshotRate"`
			} `json:"arena"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&welcome); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("welcome: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	// the server decides how often snapshots come, so that is what an on
	// time snapshot is judged against rather than a number assumed here
	b.m.snapshotRate.CompareAndSwap(0, welcome.Data.Arena.SnapshotRate)

	lobby, err := json.Marshal(map[string]any{
		"type": "lobby",
		"data": map[string]any{"name": b.name, "ready": true},
	})
	if err != nil {
		conn.Close()
		return nil, "", err
	}
	if err := conn.WriteMessage(websocket.TextMessage, lobby); err != nil {
		conn.Close()
		return nil, "", err
	}

	return conn, welcome.Data.RoomID, nil
}

// write is the only goroutine that writes to the connection, which gorilla
// requires. input and pings share it for that reason.
func (b *bot) write(ctx context.Context, conn *websocket.Conn) {
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()

	stop := time.NewTimer(time.Until(b.win.to))
	defer stop.Stop()

	var seq uint32
	buf := make([]byte, 0, 128)

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop.C:
			return

		case <-tick.C:
			seq++
			buf = appendInput(buf[:0], seq)
			if !b.send(conn, buf) {
				return
			}

		case <-ping.C:
			buf = append(buf[:0], `{"type":"ping","data":{"sentAt":`...)
			buf = strconv.AppendInt(buf, time.Now().UnixNano(), 10)
			buf = append(buf, "}}"...)
			if !b.send(conn, buf) {
				return
			}
		}
	}
}

func (b *bot) send(conn *websocket.Conn, msg []byte) bool {
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteMessage(websocket.TextMessage, msg) == nil
}

// appendInput encodes one input without reflection. the keys follow a fixed
// rhythm so the player runs back and forth and jumps, rather than standing
// still and producing a cheaper tick than any real player would.
func appendInput(buf []byte, seq uint32) []byte {
	right := seq%120 < 60
	jump := seq%45 == 0

	buf = append(buf, `{"type":"input","data":{"seq":`...)
	buf = strconv.AppendUint(buf, uint64(seq), 10)
	buf = append(buf, `,"keys":{"left":`...)
	buf = strconv.AppendBool(buf, !right)
	buf = append(buf, `,"right":`...)
	buf = strconv.AppendBool(buf, right)
	buf = append(buf, `,"jump":`...)
	buf = strconv.AppendBool(buf, jump)
	buf = append(buf, `,"block":false}}}`...)
	return buf
}

func (b *bot) read(ctx context.Context, conn *websocket.Conn, done chan<- struct{}) {
	defer close(done)

	var last time.Time
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// our own close at the end of the window is not a drop; the server
			// closing on us before then is exactly what this test looks for
			if ctx.Err() == nil && time.Now().Before(b.win.to) {
				b.m.dropped.Add(1)
			}
			return
		}

		now := time.Now()
		switch {
		case bytes.HasPrefix(msg, stateTag):
			if b.win.contains(now) {
				b.m.snapshots.Add(1)
				b.m.bytes.Add(int64(len(msg)))
				if !last.IsZero() {
					b.m.gaps.record(now.Sub(last))
				}
			}
			last = now

		case bytes.HasPrefix(msg, pongTag):
			if sent, ok := parseSentAt(msg); ok && b.win.contains(now) {
				b.m.rtts.record(time.Duration(now.UnixNano() - sent))
			}
		}
	}
}

// parseSentAt pulls the echoed timestamp out of a pong without a decoder
func parseSentAt(msg []byte) (int64, bool) {
	i := bytes.Index(msg, sentAtAt)
	if i < 0 {
		return 0, false
	}
	rest := msg[i+len(sentAtAt):]

	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	v, err := strconv.ParseInt(string(rest[:end]), 10, 64)
	return v, err == nil
}
