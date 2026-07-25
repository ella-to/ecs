package main

import (
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"ella.to/ecs"
)

// LayoutSystem keeps every element's coordinates true. It runs two passes in
// a fixed order, because the second needs what the first writes:
//
//  1. size — a Text element measures its string into Element.W/H, so the box
//     the pivot and the layout talk about always matches what is drawn.
//  2. place — a Layout element derives Element.X/Y from its alignment rule.
//
// Elements with no Layout skip pass 2 and keep the X and Y they were given.
type LayoutSystem struct {
	texts         ecs.Query2[Text, Element]
	layouts       ecs.Query2[Layout, Element]
	face          *text.GoTextFace
	lineSpacing   float64
	width, height float64 // the container elements are aligned inside
}

func NewLayoutSystem(w *ecs.World, face *text.GoTextFace, width, height float64) *LayoutSystem {
	return &LayoutSystem{
		texts:       w.Query2[Text, Element](),
		layouts:     w.Query2[Layout, Element](),
		face:        face,
		lineSpacing: lineSpacing(face),
		width:       width,
		height:      height,
	}
}

func (s *LayoutSystem) Update() {
	s.texts.Each(func(_ ecs.Entity, t *Text, e *Element) {
		e.W, e.H = text.Measure(t.Value, s.face, s.lineSpacing)
	})
	s.layouts.Each(func(_ ecs.Entity, l *Layout, e *Element) {
		l.apply(e, s.width, s.height)
	})
}

// SpinSystem advances the rotation of every spinning element. Because
// Rotation is defined about the pivot, this system says nothing about pivots
// at all — it just adds an angle, and Element.GeoM turns that into a spin
// around the right point.
type SpinSystem struct {
	spinners ecs.Query2[Spin, Element]
}

func NewSpinSystem(w *ecs.World) *SpinSystem {
	return &SpinSystem{spinners: w.Query2[Spin, Element]()}
}

func (s *SpinSystem) Update(dt float64) {
	s.spinners.Each(func(_ ecs.Entity, sp *Spin, e *Element) {
		e.Rotation += sp.Speed * dt
	})
}
