package main

import "image/color"

// Components are plain data — all behavior lives in systems.

// Position is where a particle is, in screen pixels.
type Position struct{ X, Y float64 }

// Velocity is how far a particle moves per second, in pixels.
type Velocity struct{ X, Y float64 }

// Lifetime counts a particle down to destruction. Remaining/Total drives the
// fade-out, so both are kept.
type Lifetime struct {
	Remaining float64 // seconds left
	Total     float64 // seconds at spawn
}

// Dot makes a particle visible as a colored point with a motion streak.
type Dot struct {
	Size  float64 // radius in pixels at full life
	Color color.RGBA
}
