package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ella.to/ecs"
)

// Each system builds its query once at construction; Update/Draw then run a
// straight loop over packed component data every frame.

// InputSystem turns keyboard state into velocity for controllable entities.
type InputSystem struct {
	query ecs.Query2[Controllable, Velocity]
}

func NewInputSystem(w *ecs.World) *InputSystem {
	return &InputSystem{query: w.Query2[Controllable, Velocity]()}
}

func (s *InputSystem) Update() {
	s.query.Each(func(_ ecs.Entity, c *Controllable, v *Velocity) {
		v.X, v.Y = 0, 0
		if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
			v.X -= c.Speed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
			v.X += c.Speed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
			v.Y -= c.Speed
		}
		if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
			v.Y += c.Speed
		}
	})
}

// MovementSystem integrates velocity into position and keeps entities on screen.
type MovementSystem struct {
	query ecs.Query2[Position, Velocity]
}

func NewMovementSystem(w *ecs.World) *MovementSystem {
	return &MovementSystem{query: w.Query2[Position, Velocity]()}
}

func (s *MovementSystem) Update(dt float64) {
	s.query.Each(func(_ ecs.Entity, p *Position, v *Velocity) {
		p.X = clamp(p.X+v.X*dt, 0, screenWidth-boxSize)
		p.Y = clamp(p.Y+v.Y*dt, 0, screenHeight-boxSize)
	})
}

func clamp(v, lo, hi float64) float64 {
	return min(max(v, lo), hi)
}

// RenderSystem draws every entity that has a position and a box.
type RenderSystem struct {
	query ecs.Query2[Position, Box]
}

func NewRenderSystem(w *ecs.World) *RenderSystem {
	return &RenderSystem{query: w.Query2[Position, Box]()}
}

func (s *RenderSystem) Draw(screen *ebiten.Image) {
	s.query.Each(func(_ ecs.Entity, p *Position, b *Box) {
		vector.FillRect(screen,
			float32(p.X), float32(p.Y),
			float32(b.Size), float32(b.Size),
			b.Color, false)
	})
}
