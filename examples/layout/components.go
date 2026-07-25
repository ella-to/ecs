package main

import "github.com/hajimehoshi/ebiten/v2"

// Components are plain data — all behavior lives in systems.

// Element is the transform every drawable entity carries: where it is, how
// big it is, and how it is turned and scaled.
//
// The pivot is what ties those together. It is a point inside the element's
// own W×H box, expressed as a fraction of that box, and it does three jobs at
// once:
//
//   - X and Y are the screen position of the pivot, not of the corner. Only
//     the pivot lands on an exact coordinate; everything else is placed
//     relative to it.
//   - Rotation turns the element around the pivot.
//   - Scale grows the element away from the pivot.
//
// So (0, 0) pivots at the top-left corner, (0.5, 0.5) at the center, (1, 1)
// at the bottom-right. Values outside 0..1 are legal too and put the pivot
// outside the box, which is how you orbit an element around a distant point.
type Element struct {
	X, Y           float64 // screen position of the pivot point, in pixels
	W, H           float64 // unscaled size of the element's box, in pixels
	PivotX, PivotY float64 // pivot, as a fraction of W and H
	Rotation       float64 // clockwise radians about the pivot
	Scale          float64 // uniform scale about the pivot; 1 is natural size, 0 is invisible
}

// GeoM returns the transform that maps a point in the element's own box —
// (0, 0) at its top-left, (W, H) at its bottom-right — to screen space.
//
// The three steps are the whole pivot story: move the pivot to the origin,
// scale and rotate there (so both happen around the pivot and nothing else),
// then move the origin out to (X, Y). Every consumer of Element goes through
// this, which is why rendering and rotation agree on where the pivot is.
func (e Element) GeoM() ebiten.GeoM {
	var g ebiten.GeoM
	g.Translate(-e.PivotX*e.W, -e.PivotY*e.H)
	g.Scale(e.Scale, e.Scale)
	g.Rotate(e.Rotation)
	g.Translate(e.X, e.Y)
	return g
}

// Text is a string to draw. The element's W and H are measured from it every
// tick, so changing Value re-sizes and re-positions the element by itself.
type Text struct {
	Value string
}

// Align is one axis of a Layout: where in the container the element sits.
type Align int

const (
	Start  Align = iota // left edge, or top edge
	Center              // horizontally centered, or vertically centered
	End                 // right edge, or bottom edge
)

// Padding insets the container an element is aligned inside.
type Padding struct {
	Left, Top, Right, Bottom float64
}

// Layout replaces an element's hand-written X and Y with a rule: "center it",
// "put it in the bottom-right corner, 16 pixels in". Elements without a
// Layout keep whatever X and Y you gave them.
//
// Layout is pivot-independent on purpose. It aligns the element's *box*
// inside the padded container and then derives X and Y from the pivot's place
// in that box, so the same Layout puts the element in the same spot no matter
// which pivot it spins around.
type Layout struct {
	Horizontal Align
	Vertical   Align
	Padding    Padding
}

// apply writes the element's X and Y from the layout rule, given the size of
// the container it lives in (here, the screen).
func (l Layout) apply(e *Element, containerW, containerH float64) {
	// The drawn box is the scaled one, so that centering a scaled element
	// looks centered. Rotation is deliberately ignored: layout uses the
	// unrotated box, so a spinning element stays anchored instead of
	// shuffling around as its corners swing wider.
	w, h := e.W*e.Scale, e.H*e.Scale
	minX := align(l.Horizontal, w, l.Padding.Left, containerW-l.Padding.Right)
	minY := align(l.Vertical, h, l.Padding.Top, containerH-l.Padding.Bottom)
	e.X = minX + e.PivotX*w
	e.Y = minY + e.PivotY*h
}

// align returns the low edge of a box of the given size placed between lo and
// hi along one axis.
func align(a Align, size, lo, hi float64) float64 {
	switch a {
	case Center:
		return lo + (hi-lo-size)/2
	case End:
		return hi - size
	default:
		return lo
	}
}

// Spin rotates an element around its pivot, in radians per second. It exists
// only to animate the demo — nothing in Element or Layout needs it.
type Spin struct {
	Speed float64
}
