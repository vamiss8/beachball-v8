package game

// Vec2 is a plain 2d vector. y grows downward, matching canvas coordinates.
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Side is the half of the court a player belongs to.
type Side string

const (
	SideLeft  Side = "left"
	SideRight Side = "right"
)

// Phase is the current stage of a volleyball rally.
type Phase string

const (
	// ball hangs in the air, players reposition, nothing scores yet
	PhaseServe Phase = "serve"
	// ball is live
	PhasePlaying Phase = "playing"
	// short freeze right after a point
	PhaseScored Phase = "scored"
	// match is over, someone reached PointsToWin
	PhaseFinished Phase = "finished"
)

// Input is the raw key state sent by a client every tick. the server never
// trusts anything else from the client, so this is the whole attack surface.
// dashing is not a key of its own: the server watches for a walk key pressed
// twice in quick succession, so the client never decides when a dash happens.
type Input struct {
	Left  bool `json:"left"`
	Right bool `json:"right"`
	Jump  bool `json:"jump"`
	Block bool `json:"block"`
}
