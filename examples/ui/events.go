package main

import "ella.to/ecs"

// Events are how the widget systems talk to the rest of the game without
// knowing who is listening. TextInputSystem and ButtonSystem only emit;
// LoginSystem only reads. Neither side references the other — the event
// type is their entire contract.

// TextChanged is sent whenever the user edits a TextInput.
type TextChanged struct {
	Input ecs.Entity
	Text  string
}

// ButtonClicked is sent when an enabled button is clicked (mouse pressed
// and released inside it).
type ButtonClicked struct {
	Button ecs.Entity
}
