package game

import "math"

// World is the authoritative game state. it knows nothing about networking:
// callers feed it inputs, call Step once per tick and serialise it.
type World struct {
	// Tick counts simulation steps since the world was created. the client
	// uses it as its clock: snapshots arrive with jittery timing, tick
	// numbers do not, so interpolation is driven by these instead of by
	// local receive times.
	Tick uint64 `json:"tick"`

	Players   map[string]*Player `json:"players"`
	Ball      Ball               `json:"ball"`
	Score     map[Side]int       `json:"score"`
	Phase     Phase              `json:"phase"`
	ServeSide Side               `json:"serveSide"`
	Winner    Side               `json:"winner,omitempty"`

	// ticks left in the current non-playing phase
	phaseTimer int
}

// NewWorld builds an empty match ready for its first serve.
func NewWorld() *World {
	w := &World{
		Players:   make(map[string]*Player),
		Score:     map[Side]int{SideLeft: 0, SideRight: 0},
		ServeSide: SideLeft,
	}
	w.beginServe()
	return w
}

// AddPlayer puts a player on the given side. the caller decides the side, so
// the world never has to guess from player count.
func (w *World) AddPlayer(id string, side Side) *Player {
	p := NewPlayer(id, side)
	w.Players[id] = p
	return p
}

// RemovePlayer drops a player from the simulation.
func (w *World) RemovePlayer(id string) { delete(w.Players, id) }

// SideTaken reports whether someone already occupies a side.
func (w *World) SideTaken(side Side) bool {
	for _, p := range w.Players {
		if p.Side == side {
			return true
		}
	}
	return false
}

// Step advances the whole simulation by exactly one tick.
func (w *World) Step() {
	w.Tick++

	// players keep moving in every phase, so they can reposition between
	// rallies instead of standing frozen
	for _, p := range w.Players {
		p.step()
	}

	switch w.Phase {
	case PhaseFinished:
		return

	case PhaseScored:
		// let the dead ball roll to a stop while the point sinks in
		w.Ball.Pos.X += w.Ball.Velocity.X
		w.Ball.Velocity.X *= 0.95
		if w.tickPhaseTimer() {
			w.beginServe()
		}
		return

	case PhaseServe:
		// ball hangs still until the timer runs out, then play is live
		if w.tickPhaseTimer() {
			w.Phase = PhasePlaying
		}
		return
	}

	w.Ball.integrate()
	w.Ball.bounceOffWalls()
	w.resolvePlayerHits()
	w.resolveNet()
	w.checkFloor()
}

// tickPhaseTimer counts the current phase down and reports whether it expired.
func (w *World) tickPhaseTimer() bool {
	w.phaseTimer--
	return w.phaseTimer <= 0
}

// resolvePlayerHits bounces the ball off every player it overlaps.
func (w *World) resolvePlayerHits() {
	for _, p := range w.Players {
		closest := closestPointOnRect(w.Ball.Pos, p.Pos.X, p.Pos.Y, PlayerWidth, PlayerHeight)
		dx := w.Ball.Pos.X - closest.X
		dy := w.Ball.Pos.Y - closest.Y
		distSq := dx*dx + dy*dy
		if distSq >= w.Ball.Radius*w.Ball.Radius {
			continue
		}

		dist := math.Sqrt(distSq)
		if dist == 0 {
			// ball centre sits exactly on the player, pick any normal
			dist, dx, dy = 0.1, 0, -0.1
		}
		nx, ny := dx/dist, dy/dist

		// push the ball out first so it never sinks into the player
		penetration := w.Ball.Radius - dist
		w.Ball.Pos.X += nx * penetration
		w.Ball.Pos.Y += ny * penetration

		// closing speed along the contact normal
		relX := w.Ball.Velocity.X - p.VelocityX
		relY := w.Ball.Velocity.Y - p.VelocityY
		closing := relX*nx + relY*ny
		if closing >= 0 {
			// already separating, an impulse here would suck the ball in
			continue
		}
		closing = math.Max(closing, -MaxImpactSpeed)

		restitution := PlayerRestitution
		switch {
		case p.IsBlocking:
			restitution = BlockRestitution
		case p.isSmashing():
			restitution = SmashRestitution
		}

		impulse := -(1 + restitution) * closing
		w.Ball.Velocity.X += impulse * nx
		w.Ball.Velocity.Y += impulse * ny

		// a small slice of the player's own motion, so movement aims the shot
		w.Ball.Velocity.X += p.VelocityX * AimTransferX
		if p.VelocityY < 0 {
			w.Ball.Velocity.Y += p.VelocityY * AimTransferY
		}

		w.Ball.HitCount++
	}
}

// resolveNet bounces the ball off the net post in the middle of the court.
func (w *World) resolveNet() {
	closest := closestPointOnRect(w.Ball.Pos, NetX, NetY, NetWidth, NetHeight)
	dx := w.Ball.Pos.X - closest.X
	dy := w.Ball.Pos.Y - closest.Y
	distSq := dx*dx + dy*dy
	if distSq >= w.Ball.Radius*w.Ball.Radius {
		return
	}

	dist := math.Sqrt(distSq)
	if dist == 0 {
		dist, dx, dy = 0.1, 0, -0.1
	}
	nx, ny := dx/dist, dy/dist

	penetration := w.Ball.Radius - dist
	w.Ball.Pos.X += nx * penetration
	w.Ball.Pos.Y += ny * penetration

	// reflect around the normal and damp it, the net is deliberately soft
	dot := w.Ball.Velocity.X*nx + w.Ball.Velocity.Y*ny
	w.Ball.Velocity.X = (w.Ball.Velocity.X - 2*dot*nx) * NetRestitution
	w.Ball.Velocity.Y = (w.Ball.Velocity.Y - 2*dot*ny) * NetRestitution
}

// checkFloor awards the point when the ball lands. checked last so a player
// can still save the ball on the same tick it would have touched down.
func (w *World) checkFloor() {
	if w.Ball.Pos.Y <= GroundY-w.Ball.Radius {
		return
	}

	w.Ball.Pos.Y = GroundY - w.Ball.Radius
	w.Ball.Velocity.Y *= -FloorRestitution

	// the ball landed on the loser's half, so the other side scores
	scorer := SideRight
	if w.Ball.Pos.X >= ArenaWidth/2 {
		scorer = SideLeft
	}
	w.awardPoint(scorer)
}

func (w *World) awardPoint(scorer Side) {
	w.Score[scorer]++
	// winner serves next, same as the original game
	w.ServeSide = scorer

	if w.Score[scorer] >= PointsToWin {
		w.Phase = PhaseFinished
		w.Winner = scorer
		return
	}

	w.Phase = PhaseScored
	w.phaseTimer = ScoreFreezeTicks
}

// beginServe parks the ball above the serving side and resets both players.
func (w *World) beginServe() {
	spawnX := SpawnOffsetX
	if w.ServeSide == SideRight {
		spawnX = ArenaWidth - SpawnOffsetX
	}

	w.Ball = Ball{
		Pos:    Vec2{X: spawnX, Y: ServeBallY},
		Radius: BallRadius,
	}

	for _, p := range w.Players {
		p.Reset()
	}

	w.Phase = PhaseServe
	w.phaseTimer = ServeHoldTicks
}
