package ecs

import "testing"

type clicked struct{ N int }
type typed struct{ S string }

func collect(t *testing.T, r *EventReader[clicked]) []int {
	t.Helper()
	var got []int
	r.Each(func(ev clicked) { got = append(got, ev.N) })
	return got
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEventsSendAndRead(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	ev.Send(clicked{N: 1})
	ev.Send(clicked{N: 2})

	if got := collect(t, &r); !equal(got, []int{1, 2}) {
		t.Fatalf("first read = %v, want [1 2]", got)
	}
	if got := collect(t, &r); len(got) != 0 {
		t.Fatalf("second read = %v, want nothing (already seen)", got)
	}
}

func TestEventsSurviveOneFlushNotTwo(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	ev.Send(clicked{N: 1})
	w.FlushEvents()

	// One flush later the event is still there: a reader that runs before
	// the sender in the frame sees it the next frame.
	if got := collect(t, &r); !equal(got, []int{1}) {
		t.Fatalf("read after one flush = %v, want [1]", got)
	}

	slow := ev.Reader()
	w.FlushEvents()
	w.FlushEvents()

	// Two flushes without reading: the event has expired for everyone,
	// including a reader created while it was still buffered.
	if got := collect(t, &slow); len(got) != 0 {
		t.Fatalf("read after two flushes = %v, want nothing", got)
	}
}

func TestEventsOrderAcrossBuffers(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	ev.Send(clicked{N: 1})
	w.FlushEvents() // 1 moves to the back buffer
	ev.Send(clicked{N: 2})
	ev.Send(clicked{N: 3})

	if got := collect(t, &r); !equal(got, []int{1, 2, 3}) {
		t.Fatalf("read spanning both buffers = %v, want [1 2 3]", got)
	}
}

func TestEventsReadersAreIndependent(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r1 := ev.Reader()
	r2 := ev.Reader()

	ev.Send(clicked{N: 1})
	if got := collect(t, &r1); !equal(got, []int{1}) {
		t.Fatalf("r1 = %v, want [1]", got)
	}
	// r1 consuming must not consume for r2.
	if got := collect(t, &r2); !equal(got, []int{1}) {
		t.Fatalf("r2 = %v, want [1]", got)
	}
}

func TestEventsClear(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	ev.Send(clicked{N: 1})
	r.Clear()
	if got := collect(t, &r); len(got) != 0 {
		t.Fatalf("read after Clear = %v, want nothing", got)
	}

	ev.Send(clicked{N: 2})
	if got := collect(t, &r); !equal(got, []int{2}) {
		t.Fatalf("read after Clear then Send = %v, want [2]", got)
	}
}

func TestEventsSendDuringEachDeferred(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	ev.Send(clicked{N: 1})
	var got []int
	r.Each(func(e clicked) {
		got = append(got, e.N)
		if e.N == 1 {
			ev.Send(clicked{N: 2}) // must not be visited by this same Each
		}
	})
	if !equal(got, []int{1}) {
		t.Fatalf("Each visited %v, want [1] (event sent inside Each deferred)", got)
	}
	if got := collect(t, &r); !equal(got, []int{2}) {
		t.Fatalf("next Each = %v, want [2]", got)
	}
}

func TestWorldSendAndPerTypeQueues(t *testing.T) {
	w := NewWorld()
	clicks := w.Events[clicked]().Reader()
	keys := w.Events[typed]().Reader()

	w.Send(clicked{N: 7})
	w.Send(typed{S: "a"})

	if got := collect(t, &clicks); !equal(got, []int{7}) {
		t.Fatalf("clicked queue = %v, want [7]", got)
	}
	var strs []string
	keys.Each(func(ev typed) { strs = append(strs, ev.S) })
	if len(strs) != 1 || strs[0] != "a" {
		t.Fatalf("typed queue = %v, want [a]", strs)
	}

	// FlushEvents must advance every queue, not just one type.
	w.FlushEvents()
	w.FlushEvents()
	fresh1 := w.Events[clicked]().Reader()
	fresh2 := w.Events[typed]().Reader()
	if got := collect(t, &fresh1); len(got) != 0 {
		t.Fatalf("clicked queue after two flushes = %v, want nothing", got)
	}
	n := 0
	fresh2.Each(func(typed) { n++ })
	if n != 0 {
		t.Fatalf("typed queue after two flushes has %d events, want 0", n)
	}
}

func TestEventsSteadyStateDoesNotAllocate(t *testing.T) {
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()

	// Warm up both buffers' capacity.
	for range 3 {
		for i := range 64 {
			ev.Send(clicked{N: i})
		}
		r.Each(func(clicked) {})
		w.FlushEvents()
	}

	allocs := testing.AllocsPerRun(100, func() {
		for i := range 64 {
			ev.Send(clicked{N: i})
		}
		r.Each(func(clicked) {})
		w.FlushEvents()
	})
	if allocs != 0 {
		t.Fatalf("steady-state send/read/flush allocates %.1f times per frame, want 0", allocs)
	}
}
