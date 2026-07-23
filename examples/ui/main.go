// A small login form built from ECS pieces: TextInput and Button are
// components, and the systems that run them never talk to each other
// directly. The widget systems (TextInputSystem, ButtonSystem) publish
// TextChanged and ButtonClicked events; LoginSystem polls those events and
// owns all the form logic. That event queue is the only link between them.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"ella.to/ecs"
)

const (
	screenWidth  = 640
	screenHeight = 480
)

// Game wires the ECS world and its systems into Ebiten's game loop.
type Game struct {
	world  *ecs.World
	text   *TextInputSystem
	button *ButtonSystem
	login  *LoginSystem
	render *RenderSystem
}

func NewGame() *Game {
	w := ecs.NewWorld()

	const formX, formW = 220.0, 200.0

	username := w.NewEntity()
	w.Set(username, Rect{X: formX, Y: 130, W: formW, H: 36})
	w.Set(username, TextInput{Label: "Username", Placeholder: "you@example.com"})

	password := w.NewEntity()
	w.Set(password, Rect{X: formX, Y: 210, W: formW, H: 36})
	w.Set(password, TextInput{Label: "Password", Placeholder: "At least 6 characters", Password: true})

	submit := w.NewEntity()
	w.Set(submit, Rect{X: formX, Y: 290, W: formW, H: 40})
	w.Set(submit, Button{Label: "Sign in"})

	status := w.NewEntity()
	w.Set(status, Rect{X: formX, Y: 350, W: formW})
	w.Set(status, Status{})

	return &Game{
		world:  w,
		text:   NewTextInputSystem(w),
		button: NewButtonSystem(w),
		login:  NewLoginSystem(w, username, password, submit, status),
		render: NewRenderSystem(w),
	}
}

func (g *Game) Update() error {
	g.text.Update()   // emits TextChanged
	g.button.Update() // emits ButtonClicked
	g.login.Update()  // polls both and drives the form
	g.world.FlushEvents()
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
	ebiten.SetWindowTitle("ECS UI — events")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
