package protocol

import (
	"bytes"
	"encoding/json"
	"testing"

	"beachball-v8/server/internal/game"
)

// liveWorld is a snapshot as it looks mid-rally: two named players off the
// ground, a ball in flight and a score on the board.
func liveWorld() *game.World {
	w := game.NewWorld()
	left := w.AddPlayer("player_1", game.SideLeft)
	right := w.AddPlayer("player_2", game.SideRight)
	left.SetName("vamiss8")
	right.SetName("opponent")
	left.SetReady(true)
	right.SetReady(true)

	for i := 0; i < game.ServeHoldTicks+20; i++ {
		left.SetInput(game.Input{Right: true, Jump: i%20 == 0})
		right.SetInput(game.Input{Left: true})
		w.Step()
	}
	w.Score[game.SideLeft] = 7
	w.Score[game.SideRight] = 5
	return w
}

func TestEncodedStateDecodesBack(t *testing.T) {
	w := liveWorld()

	msg, err := Encode(TypeState, State{World: w})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("envelope does not parse: %v", err)
	}
	if env.Type != TypeState {
		t.Fatalf("type = %q, want %q", env.Type, TypeState)
	}

	var got struct {
		World struct {
			Tick    uint64                     `json:"tick"`
			Score   map[string]int             `json:"score"`
			Players map[string]json.RawMessage `json:"players"`
		} `json:"world"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("payload does not parse: %v", err)
	}
	if got.World.Tick != w.Tick {
		t.Fatalf("tick = %d, want %d", got.World.Tick, w.Tick)
	}
	if got.World.Score["left"] != 7 || got.World.Score["right"] != 5 {
		t.Fatalf("score = %v, want left 7 right 5", got.World.Score)
	}
	if len(got.World.Players) != 2 {
		t.Fatalf("players = %d, want 2", len(got.World.Players))
	}
}

func TestEncodedTypeComesFirst(t *testing.T) {
	msg, err := Encode(TypeState, State{World: liveWorld()})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// part of the wire format: a reader handling thousands of these a second
	// can tell them apart from the leading bytes without decoding each one
	if want := []byte(`{"type":"state",`); !bytes.HasPrefix(msg, want) {
		t.Fatalf("message starts %q, want it to start %q", msg[:min(len(msg), 24)], want)
	}
}

// BenchmarkEncodeState is the other half of a room's cost per tick: the
// snapshot is marshalled once per room per tick and the same bytes go to
// every client.
func BenchmarkEncodeState(b *testing.B) {
	w := liveWorld()

	var size int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg, err := Encode(TypeState, State{World: w})
		if err != nil {
			b.Fatal(err)
		}
		size = len(msg)
	}

	// reported after the loop: ResetTimer clears custom metrics, and this is
	// the number every client downloads thirty times a second
	b.ReportMetric(float64(size), "bytes/snapshot")
}
