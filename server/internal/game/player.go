package game

import "math"

// Player is one paddle-ish character. the fields without a json tag are
// simulation internals no client needs.
type Player struct {
	ID   string `json:"id"`
	Side Side   `json:"side"`

	// chosen in the lobby. left empty when the player never picked one, and
	// the client falls back to naming them by their colour
	Name string `json:"name"`
	// whether this player has said they are ready to start
	Ready bool `json:"ready"`

	Pos       Vec2    `json:"pos"`
	VelocityX float64 `json:"velocityX"`
	VelocityY float64 `json:"velocityY"`
	Rotation  float64 `json:"rotation"`

	IsJumping  bool `json:"isJumping"`
	IsBlocking bool `json:"isBlocking"`

	// Motion carries the parts of the step that decide the next position but
	// are invisible on their own. exported so a client can replay its own
	// input from the same starting state; see the type for the reasoning
	Motion Motion `json:"motion"`

	// LastInputSeq is the sequence number of the last input applied here. the
	// client reads it to tell which of its own inputs the server has already
	// accounted for, and replays only the ones after it
	LastInputSeq uint32 `json:"lastInputSeq"`

	// whether the ball was already resting against this player last tick,
	// so one long contact counts as a single hit
	touchingBall bool

	input Input
}

// NewPlayer spawns a player on its side, standing on the ground.
func NewPlayer(id string, side Side) *Player {
	p := &Player{ID: id, Side: side}
	p.Reset()
	return p
}

// SetInput replaces the player's key state. called from the network layer.
func (p *Player) SetInput(in Input) { p.input = in }

// ApplyInput stores a key state along with the sequence number it arrived
// under, so the snapshot can report how far this player's input has been
// accounted for.
func (p *Player) ApplyInput(seq uint32, in Input) {
	p.input = in
	p.LastInputSeq = seq
}

// SetName stores a name the client asked for, cleaned up first.
func (p *Player) SetName(raw string) { p.Name = SanitizeName(raw) }

// SetReady records whether the player wants the match to start.
func (p *Player) SetReady(ready bool) { p.Ready = ready }

// Reset puts the player back at its spawn point for a new rally.
func (p *Player) Reset() {
	if p.Side == SideLeft {
		p.Pos = Vec2{X: SpawnOffsetX, Y: groundLevel()}
	} else {
		p.Pos = Vec2{X: ArenaWidth - SpawnOffsetX - PlayerWidth, Y: groundLevel()}
	}
	p.VelocityX = 0
	p.VelocityY = 0
	p.Rotation = 0
	p.Motion.DashVelocity = 0
	p.Motion.RotationVel = 0
	p.IsJumping = false
	p.IsBlocking = false
	p.Motion.CanDoubleJump = true
	p.Motion.DashesLeft = DashesPerAirtime
	p.Motion.DashCooldown = 0
	p.Motion.TapLeft = -1
	p.Motion.TapRight = -1
	p.touchingBall = false
}

// groundLevel is the y where a player's top edge rests on the sand
func groundLevel() float64 { return GroundY - PlayerHeight }

// step advances one player by a single tick
func (p *Player) step() {
	p.stepHorizontal()
	p.stepVertical()
	p.stepRotation()
}

func (p *Player) stepHorizontal() {
	if p.Motion.DashCooldown > 0 {
		p.Motion.DashCooldown--
	}

	p.dashOnDoubleTap()

	walk := 0.0
	if p.input.Left {
		walk = -MoveSpeed
	}
	if p.input.Right {
		walk = MoveSpeed
	}

	p.VelocityX = walk + p.Motion.DashVelocity
	p.Pos.X += p.VelocityX
	p.Motion.DashVelocity *= DashFriction

	p.clampToOwnHalf()
}

// dashOnDoubleTap fires a dash when a walk key is tapped twice inside the
// window. the detection sits here rather than in the client because the
// client only ever reports which keys are down, never what that means.
func (p *Player) dashOnDoubleTap() {
	pressedLeft := p.input.Left && !p.Motion.PrevLeft
	pressedRight := p.input.Right && !p.Motion.PrevRight
	p.Motion.PrevLeft, p.Motion.PrevRight = p.input.Left, p.input.Right

	// age the pending taps before this tick's presses, so a press lands on
	// zero and the window counts from the tick after it
	p.Motion.TapLeft = agePendingTap(p.Motion.TapLeft)
	p.Motion.TapRight = agePendingTap(p.Motion.TapRight)

	if pressedLeft {
		if p.Motion.TapLeft >= 0 {
			p.startDash(-1)
			// the pair is spent, otherwise holding a rhythm keeps dashing
			p.Motion.TapLeft = -1
		} else {
			p.Motion.TapLeft = 0
		}
	}

	if pressedRight {
		if p.Motion.TapRight >= 0 {
			p.startDash(1)
			p.Motion.TapRight = -1
		} else {
			p.Motion.TapRight = 0
		}
	}
}

// agePendingTap advances a waiting tap by one tick and forgets it once the
// window has passed. negative means nothing is waiting.
func agePendingTap(age int) int {
	if age < 0 {
		return -1
	}
	if age++; age > DoubleTapWindowTicks {
		return -1
	}
	return age
}

// startDash sends the player flying in dir (-1 left, 1 right) if they still
// have a dash left and the cooldown has expired. a dash is an instant impulse
// that decays, so it stacks on top of the walk speed instead of replacing it.
func (p *Player) startDash(dir float64) {
	if p.Motion.DashesLeft <= 0 || p.Motion.DashCooldown > 0 {
		return
	}

	p.Motion.DashVelocity = dir * DashVelocity
	p.Motion.RotationVel = dir * DashSpin
	p.Motion.DashesLeft--
	p.Motion.DashCooldown = DashCooldownTicks
}

// clampToOwnHalf keeps a player inside the arena and on their side of the net.
func (p *Player) clampToOwnHalf() {
	minX, maxX := 0.0, ArenaWidth-PlayerWidth
	if p.Side == SideLeft {
		maxX = math.Min(maxX, ArenaWidth/2-PlayerWidth-NetGap)
	} else {
		minX = math.Max(minX, ArenaWidth/2+NetGap)
	}

	if p.Pos.X < minX {
		p.Pos.X = minX
	}
	if p.Pos.X > maxX {
		p.Pos.X = maxX
	}
}

func (p *Player) stepVertical() {
	// jump triggers on the rising edge only, while holding the key does nothing
	justPressed := p.input.Jump && !p.Motion.PrevJump
	p.Motion.PrevJump = p.input.Jump

	if justPressed {
		switch {
		case !p.IsJumping:
			p.VelocityY = JumpVelocity
			p.IsJumping = true
			p.Motion.CanDoubleJump = true
		case p.Motion.CanDoubleJump:
			p.VelocityY = DoubleJumpVelocity
			p.Motion.CanDoubleJump = false
			// double jump spins the player inward, which arms the smash
			if p.Side == SideLeft {
				p.Motion.RotationVel = DoubleJumpSpin
			} else {
				p.Motion.RotationVel = -DoubleJumpSpin
			}
		}
	}

	// blocking is an air-only move that trades height for a dead bounce
	p.IsBlocking = p.input.Block && p.IsJumping
	if p.IsBlocking {
		p.VelocityY += BlockGravity
	} else {
		p.VelocityY += Gravity
	}
	p.Pos.Y += p.VelocityY

	if p.Pos.Y >= groundLevel() {
		p.Pos.Y = groundLevel()
		p.VelocityY = 0
		p.Rotation = 0
		p.Motion.RotationVel = 0
		p.IsJumping = false
		p.IsBlocking = false
		p.Motion.CanDoubleJump = true
		p.Motion.DashesLeft = DashesPerAirtime
	}
}

func (p *Player) stepRotation() {
	switch {
	case p.IsBlocking:
		// block locks a fixed lean toward the net
		if p.Side == SideLeft {
			p.Rotation = BlockAngle
		} else {
			p.Rotation = -BlockAngle
		}
		p.Motion.RotationVel = 0
	case p.Motion.RotationVel != 0:
		p.Rotation += p.Motion.RotationVel
		// one full turn, then settle upright
		if math.Abs(p.Rotation) >= 2*math.Pi {
			p.Rotation = 0
			p.Motion.RotationVel = 0
		}
	default:
		p.Rotation = 0
	}
}

// isSmashing reports whether the player is mid-spin, which turns a normal
// bump into a smash.
func (p *Player) isSmashing() bool { return p.Motion.RotationVel != 0 }
