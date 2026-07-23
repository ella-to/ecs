package main

import (
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

	"ella.to/ecs"
)

const (
	screenWidth  = 640
	screenHeight = 480
	boxSize      = 40
	dt           = 1.0 / 60 // Ebiten's Update runs at a fixed 60 ticks per second
)

// Game wires the ECS world and its systems into Ebiten's game loop.
type Game struct {
	world    *ecs.World
	input    *InputSystem
	movement *MovementSystem
	render   *RenderSystem
}

func NewGame() *Game {
	w := ecs.NewWorld()

	// The player: a keyboard-controlled box in the middle of the screen.
	player := w.NewEntity()
	w.Set(player, Position{X: (screenWidth - boxSize) / 2, Y: (screenHeight - boxSize) / 2})
	w.Set(player, Velocity{})
	w.Set(player, Box{Size: boxSize, Color: color.RGBA{R: 0x3c, G: 0xb4, B: 0x64, A: 0xff}})
	w.Set(player, Controllable{Speed: 240})

	return &Game{
		world:    w,
		input:    NewInputSystem(w),
		movement: NewMovementSystem(w),
		render:   NewRenderSystem(w),
	}
}

func (g *Game) Update() error {
	g.input.Update()
	g.movement.Update(dt)
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.render.Draw(screen)
	ebitenutil.DebugPrint(screen, "Move with WASD or arrow keys")
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("ECS Box")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
