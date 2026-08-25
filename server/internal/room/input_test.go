package room

import (
	"testing"

	"beachball-v8/server/internal/game"
)

// queueOnly builds a room without starting its goroutine: these tests drive
// the queue by hand, and a running loop would race them for it.
func queueOnly() (*Room, *game.Player) {
	r := &Room{
		world:   game.NewWorld(),
		pending: make(map[string][]playerInput),
	}
	return r, r.world.AddPlayer("p", game.SideLeft)
}

func walkRight(seq uint32) playerInput {
	return playerInput{playerID: "p", seq: seq, keys: game.Input{Right: true}}
}

func TestQueuedInputIsSpentOnePerTick(t *testing.T) {
	r, p := queueOnly()

	for seq := uint32(1); seq <= 3; seq++ {
		r.queueInput(walkRight(seq))
	}

	// one tick may only ever claim one input, which is what lets a client
	// replaying the same inputs cover the same number of ticks
	for want := uint32(1); want <= 3; want++ {
		r.applyQueuedInputs()
		if p.LastInputSeq != want {
			t.Fatalf("after tick %d: lastInputSeq = %d, want %d", want, p.LastInputSeq, want)
		}
	}

	// nothing left: the player keeps the keys they last sent
	r.applyQueuedInputs()
	if p.LastInputSeq != 3 {
		t.Fatalf("lastInputSeq = %d, want it to stay at 3 on an empty queue", p.LastInputSeq)
	}
}

func TestFloodedQueueKeepsOnlyTheFreshestInputs(t *testing.T) {
	r, p := queueOnly()

	// far more than a tick could ever consume, as a stalled connection
	// delivering a burst would produce
	const flood = maxPendingInputs * 3
	for seq := uint32(1); seq <= flood; seq++ {
		r.queueInput(walkRight(seq))
	}

	if got := len(r.pending["p"]); got != maxPendingInputs {
		t.Fatalf("queue length = %d, want it capped at %d", got, maxPendingInputs)
	}

	// the survivors are the newest ones: stale keys describe a moment that
	// has already passed
	r.applyQueuedInputs()
	if want := uint32(flood - maxPendingInputs + 1); p.LastInputSeq != want {
		t.Fatalf("first applied seq = %d, want %d", p.LastInputSeq, want)
	}
}

func TestQueueIsDroppedWhenThePlayerLeaves(t *testing.T) {
	r, _ := queueOnly()
	r.queueInput(walkRight(1))

	r.world.RemovePlayer("p")
	r.applyQueuedInputs()

	if _, ok := r.pending["p"]; ok {
		t.Fatal("a departed player's queue was left behind")
	}
}
