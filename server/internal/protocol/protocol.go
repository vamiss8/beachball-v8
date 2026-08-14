// Package protocol defines every message that crosses the websocket. keeping
// it in one place means the client and server can never drift apart silently.
package protocol

import (
	"encoding/json"

	"beachball-v8/server/internal/game"
)

// message type tags
const (
	// from server to client
	TypeWelcome = "welcome"
	TypeState   = "state"
	TypePong    = "pong"

	// from client to server
	TypeInput = "input"
	TypeLobby = "lobby"
	TypePing  = "ping"
)

// Envelope is the outer shape of every message. Data is decoded lazily so a
// single read can dispatch on Type first.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Arena tells the client the fixed dimensions it should scale its canvas to.
type Arena struct {
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	GroundY   float64 `json:"groundY"`
	NetX      float64 `json:"netX"`
	NetY      float64 `json:"netY"`
	NetWidth  float64 `json:"netWidth"`
	NetHeight float64 `json:"netHeight"`

	PlayerWidth  float64 `json:"playerWidth"`
	PlayerHeight float64 `json:"playerHeight"`

	PointsToWin int `json:"pointsToWin"`
	TickRate    int `json:"tickRate"`
}

// CurrentArena reports the arena the server is actually simulating.
func CurrentArena() Arena {
	return Arena{
		Width:        game.ArenaWidth,
		Height:       game.ArenaHeight,
		GroundY:      game.GroundY,
		NetX:         game.NetX,
		NetY:         game.NetY,
		NetWidth:     game.NetWidth,
		NetHeight:    game.NetHeight,
		PlayerWidth:  game.PlayerWidth,
		PlayerHeight: game.PlayerHeight,
		PointsToWin:  game.PointsToWin,
		TickRate:     game.TickRate,
	}
}

// Welcome is sent once, right after the connection is accepted. without it the
// client has no way to tell which player on screen is itself.
type Welcome struct {
	PlayerID  string    `json:"playerId"`
	Side      game.Side `json:"side"`
	Spectator bool      `json:"spectator"`
	RoomID    string    `json:"roomId"`
	Arena     Arena     `json:"arena"`
}

// State is a full snapshot of the world, sent every broadcast tick.
type State struct {
	World *game.World `json:"world"`
}

// Input carries the key state, sent whenever it changes.
type Input struct {
	Keys game.Input `json:"keys"`
}

// Lobby is what a player sends from the pre-match screen: the name they want
// and whether they are ready to start. the name is sanitized server-side, so
// nothing here is trusted as-is.
type Lobby struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// a ping carries whatever the client wants echoed back, and the server returns
// it untouched. no struct for it here on purpose: the payload means something
// only to the client that sent it, so the server never parses it and cannot
// disagree with the client about what a timestamp is.
//
// this exists because the websocket protocol's own ping and pong frames are
// handled by the browser itself and never surface in javascript, so a client
// has no way to time them.

// Encode wraps a payload in an envelope and marshals it.
func Encode(msgType string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: msgType, Data: data})
}
