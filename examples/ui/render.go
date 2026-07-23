package main

import (
	"bytes"
	"image"
	"image/color"
	"log"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"

	"ella.to/ecs"
)

var (
	colBackground  = color.RGBA{0x14, 0x16, 0x1e, 0xff}
	colField       = color.RGBA{0x1f, 0x22, 0x2e, 0xff}
	colBorder      = color.RGBA{0x3a, 0x3f, 0x52, 0xff}
	colBorderFocus = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
	colText        = color.RGBA{0xe8, 0xea, 0xf2, 0xff}
	colDim         = color.RGBA{0x8a, 0x8f, 0xa3, 0xff}
	colError       = color.RGBA{0xff, 0x6b, 0x6b, 0xff}
	colSuccess     = color.RGBA{0x3c, 0xb4, 0x64, 0xff}
	colButton      = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
	colButtonHover = color.RGBA{0x63, 0xae, 0xff, 0xff}
	colButtonPress = color.RGBA{0x36, 0x86, 0xe0, 0xff}
	colButtonDim   = color.RGBA{0x2c, 0x30, 0x40, 0xff}
)

// RenderSystem draws every widget from its component data. It reads state
// the other systems wrote (Focused, Hovered, Loading, Error, ...) and holds
// none of its own except a tick counter for the caret and spinner.
type RenderSystem struct {
	inputs   ecs.Query2[TextInput, Rect]
	buttons  ecs.Query2[Button, Rect]
	statuses ecs.Query2[Status, Rect]
	face     *text.GoTextFace
	small    *text.GoTextFace
	tick     int
}

func NewRenderSystem(w *ecs.World) *RenderSystem {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}
	return &RenderSystem{
		inputs:   w.Query2[TextInput, Rect](),
		buttons:  w.Query2[Button, Rect](),
		statuses: w.Query2[Status, Rect](),
		face:     &text.GoTextFace{Source: src, Size: 15},
		small:    &text.GoTextFace{Source: src, Size: 12},
	}
}

func (s *RenderSystem) Draw(screen *ebiten.Image) {
	s.tick++
	screen.Fill(colBackground)

	s.inputs.Each(func(_ ecs.Entity, t *TextInput, r *Rect) { s.drawInput(screen, t, r) })
	s.buttons.Each(func(_ ecs.Entity, b *Button, r *Rect) { s.drawButton(screen, b, r) })
	s.statuses.Each(func(_ ecs.Entity, st *Status, r *Rect) { s.drawStatus(screen, st, r) })
}

func (s *RenderSystem) drawInput(screen *ebiten.Image, t *TextInput, r *Rect) {
	s.drawText(screen, t.Label, s.small, r.X, r.Y-18, colDim)

	border := colBorder
	switch {
	case t.Error != "":
		border = colError
	case t.Focused:
		border = colBorderFocus
	}
	fillRect(screen, r, colField)
	strokeRect(screen, r, border)

	shown := t.Text
	if t.Password {
		shown = strings.Repeat("•", len([]rune(t.Text)))
	}

	// Text longer than the field scrolls left so its end (where the caret
	// is) stays in view, and is clipped to the field's padding box.
	const padding = 10.0
	avail := r.W - 2*padding
	textW := text.Advance(shown, s.face)
	textX := r.X + padding
	if textW > avail {
		textX -= textW - avail
	}
	clip := screen.SubImage(image.Rect(
		int(r.X+padding), int(r.Y),
		int(r.X+r.W-padding), int(r.Y+r.H),
	)).(*ebiten.Image)

	textY := r.Y + (r.H-s.face.Size*1.25)/2
	if shown == "" && !t.Focused {
		s.drawText(clip, t.Placeholder, s.face, textX, textY, colDim)
	} else {
		s.drawText(clip, shown, s.face, textX, textY, colText)
	}

	// Blinking caret at the end of the text while focused.
	if t.Focused && (s.tick/30)%2 == 0 {
		caretX := textX + textW
		vector.FillRect(screen, float32(caretX), float32(r.Y+8), 1.5, float32(r.H-16), colText, true)
	}

	if t.Error != "" {
		s.drawText(screen, t.Error, s.small, r.X, r.Y+r.H+6, colError)
	}
}

func (s *RenderSystem) drawButton(screen *ebiten.Image, b *Button, r *Rect) {
	bg, fg := colButton, colBackground
	switch {
	case b.Disabled || b.Loading:
		bg, fg = colButtonDim, colDim
	case b.Pressed:
		bg = colButtonPress
	case b.Hovered:
		bg = colButtonHover
	}
	fillRect(screen, r, bg)

	label := b.Label
	if b.Loading {
		label = "Signing in" + strings.Repeat(".", (s.tick/15)%4)
	}
	w, h := text.Measure(b.Label, s.face, 0) // measure the fixed label so the dots don't wobble it
	s.drawText(screen, label, s.face, r.X+(r.W-w)/2, r.Y+(r.H-h)/2, fg)
}

func (s *RenderSystem) drawStatus(screen *ebiten.Image, st *Status, r *Rect) {
	if st.Text == "" {
		return
	}
	col := colError
	if st.Success {
		col = colSuccess
	}
	w, _ := text.Measure(st.Text, s.face, 0)
	s.drawText(screen, st.Text, s.face, r.X+(r.W-w)/2, r.Y, col)
}

func (s *RenderSystem) drawText(screen *ebiten.Image, str string, face *text.GoTextFace, x, y float64, col color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x), math.Round(y))
	op.ColorScale.ScaleWithColor(col)
	text.Draw(screen, str, face, op)
}

func fillRect(screen *ebiten.Image, r *Rect, col color.Color) {
	vector.FillRect(screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), col, true)
}

func strokeRect(screen *ebiten.Image, r *Rect, col color.Color) {
	vector.StrokeRect(screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), 1.5, col, true)
}
