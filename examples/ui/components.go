package main

// Components are plain data — all behavior lives in systems.

// Rect places a widget on screen: position of its top-left corner plus size,
// in pixels.
type Rect struct{ X, Y, W, H float64 }

// contains reports whether the point (x, y) is inside the rect.
func (r Rect) contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// TextInput is a single-line text field.
type TextInput struct {
	Label       string // caption drawn above the field
	Placeholder string // gray hint shown while Text is empty
	Text        string // current value
	Error       string // red message under the field; empty means no error
	Password    bool   // draw bullets instead of the text
	Focused     bool   // receives keyboard input; at most one input has it
}

// Button is a clickable button.
type Button struct {
	Label    string
	Disabled bool // ignores the mouse and draws dimmed
	Loading  bool // ignores the mouse and draws an animated spinner
	Hovered  bool // transient, set by ButtonSystem every tick
	Pressed  bool // transient, mouse went down inside and hasn't been released
}

// Status is a free-standing line of feedback text (e.g. the result of a
// form submission).
type Status struct {
	Text    string
	Success bool // green when true, red when false
}
