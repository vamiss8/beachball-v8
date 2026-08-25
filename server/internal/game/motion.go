package game

// Motion is the part of a player's state that decides where they go next but
// never appears on screen by itself: dash charges, the double jump latch, the
// double-tap windows, and whether each key was already down last tick.
//
// it is sent to clients rather than kept private, which is a deliberate
// trade. a client predicting its own movement has to replay its input from
// exactly the state the server started from; without these fields the replay
// would forget that a dash was spent or a jump already used, and drift apart
// within a single rally. the cost is that the same data about the opponent
// travels too, since every client gets one shared snapshot. none of it is
// secret: it is all visible in how the other player moves.
type Motion struct {
	DashVelocity  float64 `json:"dashVelocity"`
	RotationVel   float64 `json:"rotationVel"`
	CanDoubleJump bool    `json:"canDoubleJump"`
	DashesLeft    int     `json:"dashesLeft"`
	DashCooldown  int     `json:"dashCooldown"`

	// a jump fires only on the tick its key goes down, so the previous state
	// of every key has to survive into the next tick
	PrevJump  bool `json:"prevJump"`
	PrevLeft  bool `json:"prevLeft"`
	PrevRight bool `json:"prevRight"`

	// ticks since a walk key was last tapped, negative when no tap is
	// waiting for its partner
	TapLeft  int `json:"tapLeft"`
	TapRight int `json:"tapRight"`
}
