// Package protocol defines every message that crosses the websocket. keeping
// it in one place means the client and server can never drift apart silently.
package protocol

import (
	"bytes"
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

// Arena is what stays fixed for the whole match: the dimensions the client
// scales its canvas to, plus the few settings its own screens need before a
// single snapshot has arrived.
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
	// the longest name the server keeps. sent so the lobby can stop typing
	// there, instead of letting someone fill a box the server then cuts
	MaxNameLength int `json:"maxNameLength"`
	TickRate      int `json:"tickRate"`
	// how many snapshots a second actually arrive. lower than the tick rate,
	// see game.SnapshotEveryTicks; a client needs it to know how far behind
	// to render and what an on time snapshot looks like
	SnapshotRate int `json:"snapshotRate"`
}

// Tuning is every constant a client needs to reproduce the server's own
// player step, tick for tick.
//
// they are sent rather than written down on both sides, so there is still one
// place where these numbers are decided. a client that hardcoded them would
// keep predicting with last week's jump height the moment one was changed.
type Tuning struct {
	MoveSpeed          float64 `json:"moveSpeed"`
	JumpVelocity       float64 `json:"jumpVelocity"`
	DoubleJumpVelocity float64 `json:"doubleJumpVelocity"`
	Gravity            float64 `json:"gravity"`
	BlockGravity       float64 `json:"blockGravity"`

	DashVelocity      float64 `json:"dashVelocity"`
	DashFriction      float64 `json:"dashFriction"`
	DashCooldownTicks int     `json:"dashCooldownTicks"`
	DashesPerAirtime  int     `json:"dashesPerAirtime"`

	DoubleTapWindowTicks int `json:"doubleTapWindowTicks"`

	DashSpin       float64 `json:"dashSpin"`
	DoubleJumpSpin float64 `json:"doubleJumpSpin"`
	BlockAngle     float64 `json:"blockAngle"`

	NetGap float64 `json:"netGap"`
}

// CurrentTuning reports the constants the simulation is actually using.
func CurrentTuning() Tuning {
	return Tuning{
		MoveSpeed:            game.MoveSpeed,
		JumpVelocity:         game.JumpVelocity,
		DoubleJumpVelocity:   game.DoubleJumpVelocity,
		Gravity:              game.Gravity,
		BlockGravity:         game.BlockGravity,
		DashVelocity:         game.DashVelocity,
		DashFriction:         game.DashFriction,
		DashCooldownTicks:    game.DashCooldownTicks,
		DashesPerAirtime:     game.DashesPerAirtime,
		DoubleTapWindowTicks: game.DoubleTapWindowTicks,
		DashSpin:             game.DashSpin,
		DoubleJumpSpin:       game.DoubleJumpSpin,
		BlockAngle:           game.BlockAngle,
		NetGap:               game.NetGap,
	}
}

// CurrentArena reports the arena the server is actually simulating.
func CurrentArena() Arena {
	return Arena{
		Width:         game.ArenaWidth,
		Height:        game.ArenaHeight,
		GroundY:       game.GroundY,
		NetX:          game.NetX,
		NetY:          game.NetY,
		NetWidth:      game.NetWidth,
		NetHeight:     game.NetHeight,
		PlayerWidth:   game.PlayerWidth,
		PlayerHeight:  game.PlayerHeight,
		PointsToWin:   game.PointsToWin,
		MaxNameLength: game.MaxNameLength,
		TickRate:      game.TickRate,
		SnapshotRate:  game.SnapshotRate,
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
	Tuning    Tuning    `json:"tuning"`
}

// State is a full snapshot of the world, sent every game.SnapshotEveryTicks ticks.
type State struct {
	World *game.World `json:"world"`
}

// Input carries the key state for one tick. the client sends one every tick
// whether the keys changed or not, because the server spends exactly one per
// tick: anything else and the two sides hold the same keys for a different
// number of ticks.
//
// Seq numbers each input a client sends. the server echoes the last one it
// applied back in the snapshot, which is how a client predicting its own
// movement knows which of its inputs are already accounted for and which it
// still has to replay on top of the authoritative state.
type Input struct {
	Seq  uint32     `json:"seq"`
	Keys game.Input `json:"keys"`
}

// inputPrefix is how the game's own client begins every input it sends
var inputPrefix = []byte(`{"type":"input",`)

// DecodeInput is the fast path for the message every player sends on every
// tick. when raw begins the way the game's own client writes an input, it is
// decoded in a single pass straight into an Input.
//
// the general route decodes an Envelope, copying its data out as raw bytes,
// and then decodes those bytes a second time. under load that was the second
// largest cost the server had after writing to sockets, all of it spent on
// the one message that outnumbers every other kind put together.
//
// ok is false for anything that does not take this path, including a well
// formed input with its keys in another order, and the caller then falls back
// to the Envelope. such a client is slower, never misread.
func DecodeInput(raw []byte) (Input, bool) {
	if !bytes.HasPrefix(raw, inputPrefix) {
		return Input{}, false
	}

	var msg struct {
		Type string `json:"type"`
		Data Input  `json:"data"`
	}
	// the type is checked again after decoding: json keeps the last of a
	// duplicated key, so a message that opens like an input can still claim
	// to be something else further in
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != TypeInput {
		return Input{}, false
	}
	return msg.Data, true
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

// Encode wraps a payload in an envelope and marshals both in a single pass.
//
// it used to marshal the payload first and then marshal an Envelope holding
// those bytes. encoding/json does not pass a json.RawMessage through as it is:
// it re-validates and compacts every byte. with a snapshot going out for every
// room on every tick, that second pass over bytes the server had written a
// moment earlier was about a tenth of all its cpu under load.
func Encode(msgType string, payload any) ([]byte, error) {
	return json.Marshal(outgoing{Type: msgType, Data: payload})
}

// outgoing is the envelope as the server writes it. it differs from Envelope,
// which is the reading side, only in holding the payload as a value to be
// encoded rather than as bytes already encoded. the field order is part of the
// wire format: type comes first, so a reader can dispatch on the leading bytes.
type outgoing struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}
