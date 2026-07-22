package game

import "math"

// Ball is the one object both players fight over.
type Ball struct {
	Pos      Vec2    `json:"pos"`
	Velocity Vec2    `json:"velocity"`
	Radius   float64 `json:"radius"`
	// HitCount drives the gravity ramp, exposed so the client can show it
	HitCount int `json:"hitCount"`
}

// gravity grows with the rally length so points always end eventually.
func (b *Ball) gravity() float64 {
	g := BallGravityBase + float64(b.HitCount)*BallGravityPerHit
	return math.Min(g, BallGravityMax)
}

// integrate applies gravity, moves the ball and clamps its speed.
func (b *Ball) integrate() {
	b.Velocity.Y += b.gravity()

	b.Velocity.X = clamp(b.Velocity.X, -BallMaxSpeed, BallMaxSpeed)
	b.Velocity.Y = clamp(b.Velocity.Y, -BallMaxSpeed, BallMaxSpeed)

	b.Pos.X += b.Velocity.X
	b.Pos.Y += b.Velocity.Y
}

// bounceOffWalls keeps the ball inside the left and right arena edges.
func (b *Ball) bounceOffWalls() {
	if b.Pos.X < b.Radius {
		b.Pos.X = b.Radius
		b.Velocity.X *= -WallRestitution
	}
	if b.Pos.X > ArenaWidth-b.Radius {
		b.Pos.X = ArenaWidth - b.Radius
		b.Velocity.X *= -WallRestitution
	}
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(v, hi))
}

// closestPointOnRect returns the point of an axis-aligned box nearest to p.
// this is the whole basis of circle-vs-box collision below.
func closestPointOnRect(p Vec2, x, y, w, h float64) Vec2 {
	return Vec2{
		X: clamp(p.X, x, x+w),
		Y: clamp(p.Y, y, y+h),
	}
}
