// Pivot-based placement built from two components. Element is the transform —
// position, size, pivot, rotation, scale — and Text is the thing to draw. The
// pivot is the single idea holding it together: it is where the element is
// anchored on screen, what it rotates about, and what it scales from, so
// "spin this label about its own center" and "hang it off its left edge" are
// the same code with a different pivot.
//
// Layout sits on top and turns placement into data: "centered", "bottom-right
// with 16 pixels of padding". Nothing below computes a screen coordinate by
// hand, and because Layout aligns the element's box rather than its pivot, an
// element keeps its place on screen no matter which point it spins around.
//
// Press space to outline the boxes and mark the pivots.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ella.to/ecs"
)

const (
	screenWidth  = 800
	screenHeight = 600
	dt           = 1.0 / 60 // Ebiten's Update runs at a fixed 60 ticks per second
)

// Game wires the ECS world and its systems into Ebiten's game loop.
type Game struct {
	world  *ecs.World
	spin   *SpinSystem
	layout *LayoutSystem
	render *RenderSystem
}

func NewGame() *Game {
	w := ecs.NewWorld()
	face := newFace(20)

	buildScene(w)

	return &Game{
		world:  w,
		spin:   NewSpinSystem(w),
		layout: NewLayoutSystem(w, face, screenWidth, screenHeight),
		render: NewRenderSystem(w, face),
	}
}

// buildScene spawns the demo's elements. W and H are left at zero everywhere:
// LayoutSystem measures them from the text on the first tick, before anything
// is drawn.
func buildScene(w *ecs.World) {
	// Aligned by rule. Note that these five have nothing but a Layout and a
	// pivot — no coordinates anywhere.
	title := newText(w, "Element · Pivot · Layout", Element{Scale: 1.5})
	w.Set(title, Layout{Horizontal: Center, Vertical: Start, Padding: Padding{Top: 26}})

	middle := newText(w, "Horizontal: Center   Vertical: Center", Element{PivotX: 0.5, PivotY: 0.5, Scale: 1})
	w.Set(middle, Layout{Horizontal: Center, Vertical: Center})

	footer := newText(w, "space — outline the boxes and mark the pivots", Element{Scale: 0.8})
	w.Set(footer, Layout{Horizontal: Center, Vertical: End, Padding: Padding{Bottom: 20}})

	// The four corners deliberately use four different pivots, and still land
	// flush in their corners: Layout aligns the box, then places the pivot
	// inside it. Turn on the debug overlay and the blue dots sit in four
	// different spots while the boxes line up exactly.
	corners := []struct {
		label          string
		h, v           Align
		pivotX, pivotY float64
	}{
		{"Start / Start  ·  pivot 0, 0", Start, Start, 0, 0},
		{"End / Start  ·  pivot 1, 1", End, Start, 1, 1},
		{"Start / End  ·  pivot 0.5, 0.5", Start, End, 0.5, 0.5},
		{"End / End  ·  pivot 0, 1", End, End, 0, 1},
	}
	for _, c := range corners {
		e := newText(w, c.label, Element{PivotX: c.pivotX, PivotY: c.pivotY, Scale: 0.8})
		w.Set(e, Layout{
			Horizontal: c.h,
			Vertical:   c.v,
			Padding:    Padding{Left: 16, Top: 16, Right: 16, Bottom: 16},
		})
	}

	// Placed by hand instead, to show that Layout is optional — and to show
	// the pivot doing its other job. Both spin at the same speed through the
	// same system; only PivotX differs, so the left one turns in place and the
	// right one swings around its own left edge.
	const spinY = 420

	center := newText(w, "center pivot", Element{X: 220, Y: spinY, PivotX: 0.5, PivotY: 0.5, Scale: 1})
	w.Set(center, Spin{Speed: 0.7})

	edge := newText(w, "edge pivot", Element{X: 580, Y: spinY, PivotX: 0, PivotY: -0.5, Scale: 1})
	w.Set(edge, Spin{Speed: 0.7})

	// Captions under each spinner, centered on it the manual way: pivot the
	// caption at the top-center of its own box and put that point where you
	// want it. This is the same trick Layout automates.
	newText(w, "PivotX 0.5   PivotY 0.5", Element{X: 220, Y: spinY + 110, PivotX: 0.5, Scale: 0.7})
	newText(w, "PivotX 0   PivotY 0.5", Element{X: 580, Y: spinY + 110, PivotX: 0.5, Scale: 0.7})
}

// newText spawns an entity with a Text and an Element — the two components
// every visible thing here has.
func newText(w *ecs.World, value string, e Element) ecs.Entity {
	entity := w.NewEntity()
	w.Set(entity, Text{Value: value})
	w.Set(entity, e)
	return entity
}

func (g *Game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.render.Debug = !g.render.Debug
	}
	g.spin.Update(dt) // rotates about each element's pivot
	g.layout.Update() // measures text, then places every laid-out element
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.render.Draw(screen)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("ECS Layout — pivots and alignment")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
