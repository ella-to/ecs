package main

// Components are plain data — all behavior lives in systems.

// Position is the top-left corner of a gopher, in screen pixels.
type Position struct{ X, Y float64 }

// Velocity is how far a gopher moves per second, in pixels.
type Velocity struct{ X, Y float64 }

// Sprite is a gopher's on-screen size in pixels (the shared image scaled at
// spawn time). Storing the final size instead of a scale factor keeps the
// per-frame bounce and geometry passes multiply-free.
type Sprite struct {
	W, H float64
}
