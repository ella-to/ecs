package main

import "image/color"

// Components are plain data — all behavior lives in systems.

// Position is where an entity is, in screen pixels.
type Position struct{ X, Y float64 }

// Velocity is how far an entity moves per second, in pixels.
type Velocity struct{ X, Y float64 }

// Box makes an entity visible as a colored square.
type Box struct {
	Size  float64
	Color color.RGBA
}

// Controllable marks an entity as keyboard-driven.
type Controllable struct {
	Speed float64 // movement speed in pixels per second
}
