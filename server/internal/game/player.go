package game

import "math"

// Player is one paddle-ish character. fields tagged "-" are simulation
// internals the client has no business knowing about.
type Player struct {
	ID   string `json:"id"`
	Side Side   `json:"side"`

	Pos       Vec2    `json:"pos"`
	VelocityX float64 `json:"velocityX"`
	VelocityY float64 `json:"velocityY"`
	Rotation  float64 `json:"rotation"`

	IsJumping  bool `json:"isJumping"`
	IsBlocking bool `json:"isBlocking"`

	dashVelocity  float64
	rotationVel   float64
	canDoubleJump bool
	dashesLeft    int
	dashCooldown  int
	prevJump      bool // edge detection, jump must be re-pressed every time
	prevLeft      bool
	prevRight     bool

	// ticks since a walk key was last tapped, negative when no tap is
	// waiting for its partner
	tapLeft  int
	tapRight int

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
	p.dashVelocity = 0
	p.rotationVel = 0
	p.IsJumping = false
	p.IsBlocking = false
	p.canDoubleJump = true
	p.dashesLeft = DashesPerAirtime
	p.dashCooldown = 0
	p.tapLeft = -1
	p.tapRight = -1
	p.touchingBall = false
}

// groundLevel is the y where a player's top edge rests on the sand.
func groundLevel() float64 { return GroundY - PlayerHeight }

// step advances one player by a single tick.
func (p *Player) step() {
	p.stepHorizontal()
	p.stepVertical()
	p.stepRotation()
}

func (p *Player) stepHorizontal() {
	if p.dashCooldown > 0 {
		p.dashCooldown--
	}

	p.dashOnDoubleTap()

	walk := 0.0
	if p.input.Left {
		walk = -MoveSpeed
	}
	if p.input.Right {
		walk = MoveSpeed
	}

	p.VelocityX = walk + p.dashVelocity
	p.Pos.X += p.VelocityX
	p.dashVelocity *= DashFriction

	p.clampToOwnHalf()
}

// dashOnDoubleTap fires a dash when a walk key is tapped twice inside the
// window. the detection sits here rather than in the client because the
// client only ever reports which keys are down, never what that means.
func (p *Player) dashOnDoubleTap() {
	pressedLeft := p.input.Left && !p.prevLeft
	pressedRight := p.input.Right && !p.prevRight
	p.prevLeft, p.prevRight = p.input.Left, p.input.Right

	// age the pending taps before this tick's presses, so a press lands on
	// zero and the window counts from the tick after it
	p.tapLeft = agePendingTap(p.tapLeft)
	p.tapRight = agePendingTap(p.tapRight)

	if pressedLeft {
		if p.tapLeft >= 0 {
			p.startDash(-1)
			// the pair is spent, otherwise holding a rhythm keeps dashing
			p.tapLeft = -1
		} else {
			p.tapLeft = 0
		}
	}

	if pressedRight {
		if p.tapRight >= 0 {
			p.startDash(1)
			p.tapRight = -1
		} else {
			p.tapRight = 0
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
	if p.dashesLeft <= 0 || p.dashCooldown > 0 {
		return
	}

	p.dashVelocity = dir * DashVelocity
	p.rotationVel = dir * DashSpin
	p.dashesLeft--
	p.dashCooldown = DashCooldownTicks
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
	// jump triggers on the rising edge only, holding the key does nothing
	justPressed := p.input.Jump && !p.prevJump
	p.prevJump = p.input.Jump

	if justPressed {
		switch {
		case !p.IsJumping:
			p.VelocityY = JumpVelocity
			p.IsJumping = true
			p.canDoubleJump = true
		case p.canDoubleJump:
			p.VelocityY = DoubleJumpVelocity
			p.canDoubleJump = false
			// double jump spins the player inward, which arms the smash
			if p.Side == SideLeft {
				p.rotationVel = DoubleJumpSpin
			} else {
				p.rotationVel = -DoubleJumpSpin
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
		p.rotationVel = 0
		p.IsJumping = false
		p.IsBlocking = false
		p.canDoubleJump = true
		p.dashesLeft = DashesPerAirtime
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
		p.rotationVel = 0
	case p.rotationVel != 0:
		p.Rotation += p.rotationVel
		// one full turn, then settle upright
		if math.Abs(p.Rotation) >= 2*math.Pi {
			p.Rotation = 0
			p.rotationVel = 0
		}
	default:
		p.Rotation = 0
	}
}

// isSmashing reports whether the player is mid-spin, which turns a normal
// bump into a smash.
func (p *Player) isSmashing() bool { return p.rotationVel != 0 }
