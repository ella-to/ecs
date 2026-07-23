package main

import (
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ella.to/ecs"
)

// TextInputSystem gives keyboard focus to the input under a mouse click and
// types into the focused one. Every edit is announced as a TextChanged
// event; the system has no idea who consumes them.
type TextInputSystem struct {
	inputs ecs.Query2[TextInput, Rect]
	events *ecs.Events[TextChanged]
	runes  []rune // scratch buffer for AppendInputChars, reused every tick
}

func NewTextInputSystem(w *ecs.World) *TextInputSystem {
	return &TextInputSystem{
		inputs: w.Query2[TextInput, Rect](),
		events: w.Events[TextChanged](),
	}
}

func (s *TextInputSystem) Update() {
	// A click focuses the input under the cursor and unfocuses the rest.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		s.inputs.Each(func(_ ecs.Entity, t *TextInput, r *Rect) {
			t.Focused = r.contains(float64(mx), float64(my))
		})
	}

	// Collect this tick's typed characters and backspace once, outside the
	// query: they apply to whichever input is focused.
	s.runes = ebiten.AppendInputChars(s.runes[:0])
	backspace := repeats(ebiten.KeyBackspace)

	s.inputs.Each(func(e ecs.Entity, t *TextInput, _ *Rect) {
		if !t.Focused {
			return
		}
		changed := false
		for _, r := range s.runes {
			if r >= ' ' { // drop control characters
				t.Text += string(r)
				changed = true
			}
		}
		if backspace && len(t.Text) > 0 {
			_, size := utf8.DecodeLastRuneInString(t.Text)
			t.Text = t.Text[:len(t.Text)-size]
			changed = true
		}
		if changed {
			s.events.Send(TextChanged{Input: e, Text: t.Text})
		}
	})
}

// repeats reports whether a held key should fire this tick: once when first
// pressed, then every 3 ticks after a half-second delay (the usual key
// repeat feel).
func repeats(key ebiten.Key) bool {
	d := inpututil.KeyPressDuration(key)
	return d == 1 || (d >= 30 && (d-30)%3 == 0)
}

// ButtonSystem tracks hover and press state for every button and emits a
// ButtonClicked event when one is clicked. Like TextInputSystem, it only
// emits — the click's meaning lives elsewhere.
type ButtonSystem struct {
	buttons ecs.Query2[Button, Rect]
	events  *ecs.Events[ButtonClicked]
}

func NewButtonSystem(w *ecs.World) *ButtonSystem {
	return &ButtonSystem{
		buttons: w.Query2[Button, Rect](),
		events:  w.Events[ButtonClicked](),
	}
}

func (s *ButtonSystem) Update() {
	mx, my := ebiten.CursorPosition()
	x, y := float64(mx), float64(my)
	pressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	released := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	s.buttons.Each(func(e ecs.Entity, b *Button, r *Rect) {
		inside := r.contains(x, y)
		active := !b.Disabled && !b.Loading
		b.Hovered = inside && active
		if pressed {
			b.Pressed = inside && active
		}
		if released {
			if b.Pressed && inside && active {
				s.events.Send(ButtonClicked{Button: e})
			}
			b.Pressed = false
		}
	})
}

// LoginSystem is the consumer side of the events: it owns the form logic
// and never touches the mouse or keyboard. It learns about edits and clicks
// purely by polling its two event readers — swap it out and the widget
// systems wouldn't change a line.
type LoginSystem struct {
	w        *ecs.World
	changes  ecs.EventReader[TextChanged]
	clicks   ecs.EventReader[ButtonClicked]
	username ecs.Entity
	password ecs.Entity
	submit   ecs.Entity
	status   ecs.Entity
	loading  int // ticks left on the fake network call; 0 means idle
}

func NewLoginSystem(w *ecs.World, username, password, submit, status ecs.Entity) *LoginSystem {
	return &LoginSystem{
		w:        w,
		changes:  w.Events[TextChanged]().Reader(),
		clicks:   w.Events[ButtonClicked]().Reader(),
		username: username,
		password: password,
		submit:   submit,
		status:   status,
	}
}

func (s *LoginSystem) Update() {
	// Editing a field clears its error and any stale result line.
	s.changes.Each(func(ev TextChanged) {
		if t := s.w.Get[TextInput](ev.Input); t != nil {
			t.Error = ""
		}
		*s.w.Get[Status](s.status) = Status{}
	})

	s.clicks.Each(func(ev ButtonClicked) {
		if ev.Button == s.submit && s.loading == 0 {
			s.trySubmit()
		}
	})

	// Resolve the pretend network call.
	if s.loading > 0 {
		s.loading--
		if s.loading == 0 {
			button := s.w.Get[Button](s.submit)
			button.Loading = false
			name := s.w.Get[TextInput](s.username).Text
			*s.w.Get[Status](s.status) = Status{Text: "Welcome back, " + name + "!", Success: true}
		}
	}
}

func (s *LoginSystem) trySubmit() {
	username := s.w.Get[TextInput](s.username)
	password := s.w.Get[TextInput](s.password)

	ok := true
	if strings.TrimSpace(username.Text) == "" {
		username.Error = "Username is required"
		ok = false
	}
	if utf8.RuneCountInString(password.Text) < 6 {
		password.Error = "Password must be at least 6 characters"
		ok = false
	}
	if !ok {
		*s.w.Get[Status](s.status) = Status{Text: "Please fix the errors above", Success: false}
		return
	}

	s.w.Get[Button](s.submit).Loading = true
	s.loading = 90 // ~1.5 s at 60 ticks per second
}
