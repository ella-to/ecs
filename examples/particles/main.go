package main

import (
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

	"ella.to/ecs"
)

const (
	screenWidth  = 640
	screenHeight = 480
	dt           = 1.0 / 60 // Ebiten's Update runs at a fixed 60 ticks per second

	particlesPerBurst = 12000
	gravity           = 240 // pixels per second², pulls particles down
	drag              = 1.2 // per-second velocity decay
)

// Game wires the ECS world and its systems into Ebiten's game loop.
type Game struct {
	world    *ecs.World
	spawn    *SpawnSystem
	physics  *PhysicsSystem
	lifetime *LifetimeSystem
	render   *RenderSystem
}

func NewGame() *Game {
	w := ecs.NewWorld()
	return &Game{
		world:    w,
		spawn:    NewSpawnSystem(w),
		physics:  NewPhysicsSystem(w),
		lifetime: NewLifetimeSystem(w),
		render:   NewRenderSystem(w),
	}
}

func (g *Game) Update() error {
	g.spawn.Update()
	g.physics.Update(dt)
	g.lifetime.Update(dt)
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.render.Draw(screen)
	ebitenutil.DebugPrint(screen,
		fmt.Sprintf("Click anywhere to explode\nparticles: %d", g.render.Count()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("ECS Particles")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
