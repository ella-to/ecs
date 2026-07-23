package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"time"

	_ "embed"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

	"ella.to/ecs"
)

const (
	screenWidth  = 1280
	screenHeight = 720
	dt           = 1.0 / 60 // Ebiten's Update runs at a fixed 60 ticks per second

	spawnPerTick = 5 // gophers added per tick while the mouse is held
)

//go:embed asset/gopher.png
var gopherPNG []byte

// gopherImage is the one shared sprite; gopherWidth/Height are its unscaled
// size in pixels, used by the bounce system.
var (
	gopherImage  *ebiten.Image
	gopherWidth  float64
	gopherHeight float64
)

// Game wires the ECS world and its systems into Ebiten's game loop, and
// times each phase so the overlay can attribute the frame cost.
type Game struct {
	world  *ecs.World
	spawn  *SpawnSystem
	bounce *BounceSystem
	render *RenderSystem

	updateDur time.Duration // last tick's ECS update pass
	drawDur   time.Duration // last frame's render pass (CPU side)
}

func NewGame(n int) *Game {
	w := ecs.NewWorld()
	g := &Game{
		world:  w,
		spawn:  NewSpawnSystem(w),
		bounce: NewBounceSystem(w),
		render: NewRenderSystem(w),
	}
	g.spawn.Spawn(n, screenWidth/2, screenHeight/2)
	return g
}

func (g *Game) Update() error {
	g.spawn.Update()

	start := time.Now()
	g.bounce.Update(dt)
	g.updateDur = time.Since(start)
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	start := time.Now()
	g.render.Draw(screen)
	g.drawDur = time.Since(start)

	count := g.render.Count()
	perGopher := 0.0
	if count > 0 {
		perGopher = float64(g.updateDur.Nanoseconds()) / float64(count)
	}
	ebitenutil.DebugPrint(screen, fmt.Sprintf(
		"gophers: %d   (hold mouse: +%d/tick)\nFPS: %0.1f   TPS: %0.1f\nECS update: %0.2f ms  (%0.1f ns/gopher)\ngeometry:   %0.2f ms\ndraw queue: %0.2f ms",
		count, spawnPerTick,
		ebiten.ActualFPS(), ebiten.ActualTPS(),
		float64(g.updateDur.Microseconds())/1000, perGopher,
		float64(g.render.geomDur.Microseconds())/1000,
		float64((g.drawDur-g.render.geomDur).Microseconds())/1000,
	))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	n := flag.Int("n", 10_000, "initial number of gophers")
	vsync := flag.Bool("vsync", true, "cap FPS to the display refresh rate")
	flag.Parse()

	img, _, err := image.Decode(bytes.NewReader(gopherPNG))
	if err != nil {
		log.Fatal(err)
	}
	gopherImage = ebiten.NewImageFromImage(keyOutBackground(img))
	b := gopherImage.Bounds()
	gopherWidth, gopherHeight = float64(b.Dx()), float64(b.Dy())

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("ECS Gophers")
	ebiten.SetVsyncEnabled(*vsync)
	if err := ebiten.RunGame(NewGame(*n)); err != nil {
		log.Fatal(err)
	}
}
