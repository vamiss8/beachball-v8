// Package protocol defines every message that crosses the websocket. keeping
// it in one place means the client and server can never drift apart silently.
package protocol

import (
	"encoding/json"

	"beachball-v8/server/internal/game"
)

// message type tags
const (
	// server -> client
	TypeWelcome = "welcome"
	TypeState   = "state"
	TypeError   = "error"

	// client -> server
	TypeInput = "input"
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

// Error reports something the client did wrong, without dropping it.
type Error struct {
	Message string `json:"message"`
}

// Input is the only message a client is allowed to send.
type Input struct {
	Keys game.Input `json:"keys"`
}

// Encode wraps a payload in an envelope and marshals it.
func Encode(msgType string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: msgType, Data: data})
}
