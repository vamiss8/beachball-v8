package game

import "testing"

// stepN advances the world and returns after n ticks.
func stepN(w *World, n int) {
	for i := 0; i < n; i++ {
		w.Step()
	}
}

func TestServeHoldKeepsBallStill(t *testing.T) {
	w := NewWorld()
	start := w.Ball.Pos

	// one tick short of the hold expiring, the ball must not have moved
	stepN(w, ServeHoldTicks-1)
	if w.Ball.Pos != start {
		t.Fatalf("ball moved during serve hold: got %v, want %v", w.Ball.Pos, start)
	}
	if w.Phase != PhaseServe {
		t.Fatalf("phase = %q, want %q", w.Phase, PhaseServe)
	}

	// the next tick releases it
	w.Step()
	if w.Phase != PhasePlaying {
		t.Fatalf("phase = %q, want %q", w.Phase, PhasePlaying)
	}
	w.Step()
	if w.Ball.Pos.Y <= start.Y {
		t.Fatalf("ball did not fall after serve: y = %v, want > %v", w.Ball.Pos.Y, start.Y)
	}
}

func TestBallGravityIsAppliedOncePerTick(t *testing.T) {
	w := NewWorld()
	stepN(w, ServeHoldTicks) // release the ball

	w.Step()
	// exactly one gravity step must have been integrated, a second
	// integration in the same tick would double this
	if got, want := w.Ball.Velocity.Y, BallGravityBase; got != want {
		t.Fatalf("velocity.y after one tick = %v, want %v", got, want)
	}
}

func TestUntouchedServeScoresForTheOtherSide(t *testing.T) {
	w := NewWorld() // serve side is left, so the ball spawns on the left half

	// long enough for the ball to fall the whole arena height
	for i := 0; i < 600 && w.Phase != PhaseScored; i++ {
		w.Step()
	}

	if w.Phase != PhaseScored {
		t.Fatalf("phase = %q, want %q", w.Phase, PhaseScored)
	}
	if w.Score[SideRight] != 1 || w.Score[SideLeft] != 0 {
		t.Fatalf("score = %v, want right:1 left:0", w.Score)
	}
	// whoever won the point serves the next one
	if w.ServeSide != SideRight {
		t.Fatalf("serveSide = %q, want %q", w.ServeSide, SideRight)
	}
}

func TestMatchFinishesAtPointsToWin(t *testing.T) {
	w := NewWorld()

	for i := 0; i < 600*PointsToWin*2 && w.Phase != PhaseFinished; i++ {
		w.Step()
	}

	if w.Phase != PhaseFinished {
		t.Fatalf("phase = %q, want %q", w.Phase, PhaseFinished)
	}
	if w.Winner == "" {
		t.Fatal("winner is empty on a finished match")
	}
	if w.Score[w.Winner] != PointsToWin {
		t.Fatalf("winner score = %d, want %d", w.Score[w.Winner], PointsToWin)
	}

	// a finished match must stay frozen
	before := w.Ball.Pos
	stepN(w, 60)
	if w.Ball.Pos != before {
		t.Fatal("ball kept moving after the match finished")
	}
}

func TestPlayerCannotCrossTheNet(t *testing.T) {
	w := NewWorld()
	p := w.AddPlayer("p1", SideLeft)
	p.SetInput(Input{Right: true, DashR: true})

	stepN(w, 120)

	if limit := ArenaWidth/2 - PlayerWidth - NetGap; p.Pos.X > limit {
		t.Fatalf("left player crossed the net: x = %v, want <= %v", p.Pos.X, limit)
	}
}

func TestSideAssignmentFreesUpOnLeave(t *testing.T) {
	w := NewWorld()
	w.AddPlayer("p1", SideLeft)
	w.AddPlayer("p2", SideRight)

	if !w.SideTaken(SideLeft) || !w.SideTaken(SideRight) {
		t.Fatal("both sides should be taken")
	}

	w.RemovePlayer("p1")
	if w.SideTaken(SideLeft) {
		t.Fatal("left side should be free after the player left")
	}
	if !w.SideTaken(SideRight) {
		t.Fatal("right side should still be taken")
	}
}

func TestPlayerLandsAndRegainsAbilities(t *testing.T) {
	w := NewWorld()
	p := w.AddPlayer("p1", SideLeft)

	p.SetInput(Input{Jump: true})
	w.Step()
	if !p.IsJumping {
		t.Fatal("player did not leave the ground")
	}

	// holding jump must not re-trigger, the edge is already consumed
	stepN(w, 2)
	if !p.canDoubleJump {
		t.Fatal("double jump should still be available while holding the key")
	}

	p.SetInput(Input{})
	stepN(w, 120) // plenty of time to come back down

	if p.IsJumping {
		t.Fatal("player never landed")
	}
	if p.dashesLeft != DashesPerAirtime {
		t.Fatalf("dashes after landing = %d, want %d", p.dashesLeft, DashesPerAirtime)
	}
}
