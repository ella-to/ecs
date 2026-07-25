package main

import (
	"bytes"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"

	"ella.to/ecs"
)

var (
	colBackground = color.RGBA{0x14, 0x16, 0x1e, 0xff}
	colText       = color.RGBA{0xe8, 0xea, 0xf2, 0xff}
	colBox        = color.RGBA{0x3a, 0x3f, 0x52, 0xff}
	colPivot      = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
)

// RenderSystem draws every text element through its own transform. There is
// no positioning logic here: the element already knows where its pivot is on
// screen, and Element.GeoM turns that plus the rotation and scale into the one
// matrix the draw call needs.
type RenderSystem struct {
	texts       ecs.Query2[Text, Element]
	face        *text.GoTextFace
	lineSpacing float64

	// Debug outlines each element's box and marks its pivot, which is the
	// clearest way to see that a rotation really does turn about that point.
	Debug bool
}

func NewRenderSystem(w *ecs.World, face *text.GoTextFace) *RenderSystem {
	return &RenderSystem{
		texts:       w.Query2[Text, Element](),
		face:        face,
		lineSpacing: lineSpacing(face),
	}
}

func (s *RenderSystem) Draw(screen *ebiten.Image) {
	screen.Fill(colBackground)
	s.texts.Each(func(_ ecs.Entity, t *Text, e *Element) {
		op := &text.DrawOptions{}
		// Draw puts the text's top-left at the origin, which is exactly the
		// corner Element.GeoM expects to be handed.
		op.GeoM = e.GeoM()
		op.LineSpacing = s.lineSpacing
		op.ColorScale.ScaleWithColor(colText)
		text.Draw(screen, t.Value, s.face, op)

		if s.Debug {
			drawBounds(screen, e)
		}
	})
}

// drawBounds outlines the element's box and dots its pivot. The four corners
// go through the same GeoM as the text, so the outline rotates and scales with
// it and the dot sits still at (X, Y) however the element turns.
func drawBounds(screen *ebiten.Image, e *Element) {
	g := e.GeoM()
	var xs, ys [4]float32
	corners := [4][2]float64{{0, 0}, {e.W, 0}, {e.W, e.H}, {0, e.H}}
	for i, c := range corners {
		x, y := g.Apply(c[0], c[1])
		xs[i], ys[i] = float32(x), float32(y)
	}
	for i := range corners {
		j := (i + 1) % 4
		vector.StrokeLine(screen, xs[i], ys[i], xs[j], ys[j], 1, colBox, true)
	}
	vector.FillCircle(screen, float32(e.X), float32(e.Y), 3, colPivot, true)
}

// newFace loads the built-in Go font at the given pixel size.
func newFace(size float64) *text.GoTextFace {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}
	return &text.GoTextFace{Source: src, Size: size}
}

// lineSpacing is the baseline-to-baseline distance for a face. Measure and
// Draw must agree on it, or a multi-line element's box would not match the
// text drawn inside it.
func lineSpacing(face *text.GoTextFace) float64 { return face.Size * 1.3 }
