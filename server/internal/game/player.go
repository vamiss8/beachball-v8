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

	// a dash is an instant impulse that decays, so it stacks on top of the
	// walk speed instead of replacing it
	if p.dashesLeft > 0 && p.dashCooldown <= 0 {
		switch {
		case p.input.DashL:
			p.dashVelocity = -DashVelocity
			p.rotationVel = -DashSpin
			p.dashesLeft--
			p.dashCooldown = DashCooldownTicks
		case p.input.DashR:
			p.dashVelocity = DashVelocity
			p.rotationVel = DashSpin
			p.dashesLeft--
			p.dashCooldown = DashCooldownTicks
		}
	}

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
