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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
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
	if env.Type != protocol.TypeInput || c.spectator {
		return
	}

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
