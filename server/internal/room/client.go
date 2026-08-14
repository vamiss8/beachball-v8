package room

import (
	"encoding/json"
	"log"
	"time"

	"beachball-v8/server/internal/game"
	"beachball-v8/server/internal/protocol"

	"github.com/gorilla/websocket"
)

const (
	// how long a client may go silent before we assume it is gone
	pongWait   = 60 * time.Second
	pingPeriod = pongWait * 9 / 10
	writeWait  = 10 * time.Second

	// a client is never allowed to send more than a key-state blob
	maxMessageSize = 512

	// buffered so a brief network hiccup does not cost the client its seat
	sendBufferSize = 32

	// the shortest gap between two answered pings. the client asks every
	// couple of seconds, so this only ever trims a client asking far more
	// often than any measurement of its own could use
	minPingInterval = 250 * time.Millisecond
)

// Client is one websocket connection. reads and writes each get their own
// goroutine so a stalled socket can never block the simulation.
type Client struct {
	room *Room
	conn *websocket.Conn
	send chan []byte

	// set by Room.add, read only by the room goroutine afterwards
	playerID  string
	side      game.Side
	spectator bool

	// touched only by readPump, which is the only goroutine answering pings
	lastPingReply time.Time

	closed chan struct{}
}

// Serve attaches an upgraded connection to the room and blocks until the
// client disconnects.
func (r *Room) Serve(conn *websocket.Conn) {
	c := &Client{
		room:   r,
		conn:   conn,
		send:   make(chan []byte, sendBufferSize),
		closed: make(chan struct{}),
	}

	select {
	case r.register <- c:
	case <-r.quit:
		conn.Close()
		return
	}

	go c.writePump()
	c.readPump() // blocks; returns once the peer goes away
}

// trySend queues a message without ever blocking the room goroutine. it
// reports false when the client's buffer is full, meaning it fell behind.
func (c *Client) trySend(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	case <-c.closed:
		return true // already going away, not a backpressure failure
	default:
		return false
	}
}

// close signals both pumps to wind down. safe to call more than once.
func (c *Client) close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
		c.conn.Close()
	}
}

func (c *Client) describe() string {
	if c.playerID == "" {
		return "spectator"
	}
	return c.playerID + "(" + string(c.side) + ")"
}

// readPump consumes client messages until the connection dies.
func (c *Client) readPump() {
	defer func() {
		c.close()
		select {
		case c.room.unregister <- c:
		case <-c.room.quit:
		}
	}()

	// hard limit: nothing a client legitimately sends is bigger than this
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			// browsers usually close without a status code, so 1005 is a
			// normal goodbye here and not worth logging as an error
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived) {
				log.Printf("room %s: read error from %s: %v", c.room.ID, c.describe(), err)
			}
			return
		}
		c.handleMessage(raw)
	}
}

// handleMessage decodes one client message. every failure path here is a
// no-op on purpose: malformed input must never take the server down.
func (c *Client) handleMessage(raw []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	// answered before the spectator check, since watching a match is exactly
	// when someone wants to know whether their connection is the problem
	if env.Type == protocol.TypePing {
		c.replyToPing(env.Data)
		return
	}

	// spectators have no player to drive, so nothing else they send means
	// anything
	if c.spectator {
		return
	}

	switch env.Type {
	case protocol.TypeInput:
		var in protocol.Input
		if err := json.Unmarshal(env.Data, &in); err != nil {
			return
		}
		// dropped rather than queued if the room is busy: stale key state is
		// worthless, the client resends it next frame anyway
		select {
		case c.room.inputs <- playerInput{playerID: c.playerID, keys: in.Keys}:
		default:
		}

	case protocol.TypeLobby:
		var lb protocol.Lobby
		if err := json.Unmarshal(env.Data, &lb); err != nil {
			return
		}
		// never dropped, unlike input: readying up happens once and the
		// client has no reason to repeat it, so losing it would strand
		// everyone in the lobby
		select {
		case c.room.lobby <- lobbyUpdate{playerID: c.playerID, name: lb.Name, ready: lb.Ready}:
		case <-c.room.quit:
		}
	}
}

// replyToPing echoes a ping payload straight back.
//
// answered here in the read goroutine rather than by handing it to the room:
// a round trip is meant to measure the network, and queueing it behind a tick
// would fold the simulation's own timing into the number. it also means the
// reading side never blocks on the room being busy.
//
// the payload is passed through as raw json without being parsed. the server
// has no opinion about what a client puts in there.
func (c *Client) replyToPing(payload json.RawMessage) {
	// a client that pings faster than this learns nothing extra, so the
	// spare ones are dropped rather than answered
	now := time.Now()
	if now.Sub(c.lastPingReply) < minPingInterval {
		return
	}
	c.lastPingReply = now

	msg, err := protocol.Encode(protocol.TypePong, payload)
	if err != nil {
		return
	}
	c.trySend(msg)
}

// writePump drains the send queue and keeps the connection alive with pings.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// room closed the channel, say goodbye politely
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.closed:
			return
		}
	}
}
