package game

// these tests drive World directly and never touch the network layer, which
// is the whole reason game has no imports from room or protocol.

import (
	"math"
	"testing"
)

// stepN advances the world and returns after n ticks.
func stepN(w *World, n int) {
	for i := 0; i < n; i++ {
		w.Step()
	}
}

// servingWorld skips the lobby and drops straight into a serve. the ball
// tests care about physics, not about who pressed ready, and leaving the
// players out keeps them from batting the ball they are meant to watch land.
func servingWorld() *World {
	w := NewWorld()
	w.startMatch()
	return w
}

// readyMatch is the lobby route: two players who both said go, stepped once
// so the match has actually begun.
func readyMatch() (*World, *Player, *Player) {
	w := NewWorld()
	left := w.AddPlayer("left", SideLeft)
	right := w.AddPlayer("right", SideRight)

	left.SetReady(true)
	right.SetReady(true)
	w.Step()

	return w, left, right
}

// tapAgain releases every key for a tick and presses in again, which is the
// second half of a double tap. the world is stepped twice.
func tapAgain(w *World, p *Player, in Input) {
	p.SetInput(Input{})
	w.Step()
	p.SetInput(in)
	w.Step()
}

func TestServeHoldKeepsBallStill(t *testing.T) {
	w := servingWorld()
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
	w := servingWorld()
	stepN(w, ServeHoldTicks) // release the ball

	w.Step()
	// exactly one gravity step must have been integrated, a second
	// integration in the same tick would double this
	if got, want := w.Ball.Velocity.Y, BallGravityBase; got != want {
		t.Fatalf("velocity.y after one tick = %v, want %v", got, want)
	}
}

func TestUntouchedServeScoresForTheOtherSide(t *testing.T) {
	w := servingWorld() // serve side is left, so the ball spawns on the left half

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
	w := servingWorld()

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
	p.SetInput(Input{Right: true})
	tapAgain(w, p, Input{Right: true}) // dash straight at the net

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

func TestDoubleTapDashes(t *testing.T) {
	w := NewWorld()
	p := w.AddPlayer("p1", SideLeft)

	// the first press is an ordinary walk, nothing more
	p.SetInput(Input{Right: true})
	w.Step()
	if p.VelocityX > MoveSpeed {
		t.Fatalf("a single press already dashed: velocityX = %v", p.VelocityX)
	}

	tapAgain(w, p, Input{Right: true})

	if p.VelocityX <= MoveSpeed {
		t.Fatalf("double tap did not dash: velocityX = %v, want > %v", p.VelocityX, MoveSpeed)
	}
	// landing refills the dash count every tick, so the cooldown is what
	// actually proves a dash was spent while standing on the sand
	if p.dashCooldown == 0 {
		t.Fatal("dash cooldown was not armed")
	}
}

func TestSecondTapAfterTheWindowDoesNotDash(t *testing.T) {
	w := NewWorld()
	p := w.AddPlayer("p1", SideLeft)

	p.SetInput(Input{Right: true})
	w.Step()
	p.SetInput(Input{})
	stepN(w, DoubleTapWindowTicks+1) // let the pairing expire

	p.SetInput(Input{Right: true})
	w.Step()

	if p.VelocityX > MoveSpeed {
		t.Fatalf("a late second tap dashed: velocityX = %v", p.VelocityX)
	}
	if p.dashCooldown != 0 {
		t.Fatalf("dash cooldown = %d, want 0: no dash should have fired", p.dashCooldown)
	}
}

func TestRunningPlayerCarriesTheBall(t *testing.T) {
	w := servingWorld()
	p := w.AddPlayer("p1", SideLeft)
	stepN(w, ServeHoldTicks) // into open play

	// park the ball just above the player's head, barely falling
	p.Pos.X = SpawnOffsetX
	w.Ball.Pos = Vec2{X: p.Pos.X + PlayerWidth/2, Y: p.Pos.Y - BallRadius - 1}
	w.Ball.Velocity = Vec2{X: 0, Y: 1}

	p.SetInput(Input{Right: true})
	stepN(w, 30)

	// the ball should have travelled with the player instead of being
	// knocked out in front of them or left behind
	drift := math.Abs(w.Ball.Pos.X - (p.Pos.X + PlayerWidth/2))
	if drift > PlayerWidth {
		t.Fatalf("ball drifted %v from the player, want <= %v", drift, PlayerWidth)
	}
	if w.Phase != PhasePlaying {
		t.Fatalf("phase = %q, want %q: the carried ball should not have landed", w.Phase, PhasePlaying)
	}
}

func TestCarryingCountsAsOneHit(t *testing.T) {
	w := servingWorld()
	p := w.AddPlayer("p1", SideLeft)
	stepN(w, ServeHoldTicks)

	p.Pos.X = SpawnOffsetX
	w.Ball.Pos = Vec2{X: p.Pos.X + PlayerWidth/2, Y: p.Pos.Y - BallRadius - 1}
	w.Ball.Velocity = Vec2{X: 0, Y: 1}
	p.SetInput(Input{Right: true})

	stepN(w, 120) // two seconds of carrying

	// counting every touching tick instead of every arrival used to rack up
	// over a hundred hits here, which pinned ball gravity at its cap and
	// left the rest of the rally unplayable
	if w.Ball.HitCount > 10 {
		t.Fatalf("hit count = %d after carrying, want a handful (was over a hundred)", w.Ball.HitCount)
	}
}
