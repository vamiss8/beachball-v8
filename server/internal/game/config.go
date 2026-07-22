package game

import "time"

// IMPORTANT: every velocity/acceleration constant below is expressed in
// units-per-tick, not units-per-second. the simulation always advances in
// fixed TickRate steps (see room.Room.run), so per-tick units stay
// deterministic and keep the arcade feel easy to tune by hand.

// simulation timing
const (
	TickRate     = 60
	TickDuration = time.Second / TickRate
)

// arena geometry in game units. the client scales these to its canvas size,
// so the server never needs to know the real resolution.
const (
	ArenaWidth  = 1600.0
	ArenaHeight = 900.0
	GroundY     = ArenaHeight - 100.0

	NetWidth  = 20.0
	NetHeight = 240.0
	NetX      = ArenaWidth/2 - NetWidth/2
	NetY      = GroundY - NetHeight

	// how close a player may get to the net before being blocked
	NetGap = 10.0
)

// player tuning
const (
	PlayerWidth  = 120.0
	PlayerHeight = 120.0

	SpawnOffsetX = 300.0

	MoveSpeed          = 10.0
	JumpVelocity       = -18.0
	DoubleJumpVelocity = -16.0
	Gravity            = 0.6
	BlockGravity       = 1.8 // holding block makes the player drop faster

	DashVelocity      = 35.0
	DashFriction      = 0.85 // per-tick decay of the dash impulse
	DashCooldownTicks = 30
	DashesPerAirtime  = 2

	// spin applied by dashes and double jumps; a spinning player smashes
	DashSpin       = 0.35
	DoubleJumpSpin = 0.25
	BlockAngle     = 0.7853981633974483 // pi/4
)

// ball tuning
const (
	BallRadius = 65.0
	ServeBallY = 150.0

	// gravity ramps up with every hit so rallies cannot stall forever
	BallGravityBase   = 0.3
	BallGravityPerHit = 0.01
	BallGravityMax    = 0.9

	// terminal velocity, prevents game-breaking speeds
	BallMaxSpeed = 25.0

	WallRestitution  = 0.8
	FloorRestitution = 0.3
	NetRestitution   = 0.3

	PlayerRestitution = 0.65 // normal bump
	BlockRestitution  = 0.1  // block absorbs the ball
	SmashRestitution  = 1.0  // spinning player smashes it back

	// caps the closing speed used for the bounce impulse, otherwise a
	// double jump into the ball launches it into space
	MaxImpactSpeed = 22.0

	// how much of the player's own motion is handed to the ball for aiming
	AimTransferX = 0.25
	AimTransferY = 0.15
)

// match rules
const (
	// ball hangs still at the start of a rally so both players can reposition
	ServeHoldTicks = 45
	// freeze after a point, before the next serve is set up
	ScoreFreezeTicks = 120

	PointsToWin = 15
)
