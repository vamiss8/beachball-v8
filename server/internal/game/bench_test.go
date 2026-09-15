package game

import "testing"

// benchMatch is a two player match already past its first serve, with both
// players holding keys that keep them running, jumping and dashing. it is the
// state a room spends almost all of its life in.
func benchMatch() (*World, *Player, *Player) {
	w, left, right := readyMatch()
	stepN(w, ServeHoldTicks)
	return w, left, right
}

// BenchmarkMatchTick measures one tick of a live match, averaged over whole
// rallies: serves, open play, scored freezes and finished matches restarting.
// that average is what a room actually costs, which is the number capacity is
// built from.
func BenchmarkMatchTick(b *testing.B) {
	w, left, right := benchMatch()

	// a steady rhythm of presses, so the players move, jump and dash instead
	// of standing still and making every tick look cheaper than it is
	pattern := []Input{
		{Right: true}, {Right: true}, {}, {Right: true, Jump: true},
		{Left: true}, {Left: true, Block: true}, {}, {Left: true, Jump: true},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := pattern[i%len(pattern)]
		left.SetInput(in)
		right.SetInput(in)

		w.Step()

		// a finished match would otherwise idle forever and flatter the result
		if w.Phase == PhaseFinished {
			left.SetReady(true)
			right.SetReady(true)
		}
	}
}
