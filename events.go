package ecs

import "reflect"

// Events is a double-buffered queue of events of type T — the pull-based
// channel between systems that the rest of the library's design implies:
// a producer system Sends events, consumer systems poll them through their
// own EventReader, and nobody holds a reference to anybody else.
//
//	clicks := w.Events[ButtonClicked]()   // in the producer's constructor
//	clicks.Send(ButtonClicked{Button: e}) // during its Update
//
//	reader := w.Events[ButtonClicked]().Reader() // in a consumer's constructor
//	reader.Each(func(ev ButtonClicked) { ... })  // during its Update
//
//	w.FlushEvents() // once per frame, after all systems ran
//
// Lifetime: an event survives until the second FlushEvents after it was
// sent. That two-frame window is what makes the queue order-independent —
// a consumer that runs *before* the producer in the frame still sees the
// event on the next frame, one frame late but never lost. The flip side:
// a reader must poll at least once per frame, or events expire unread.
//
// Each reader tracks its own cursor, so any number of systems can consume
// the same queue independently; reading never removes anything.
//
// Performance: Send is an append into a buffer whose capacity is reused
// after every flush, and reading is a linear walk of a packed slice — in
// steady state nothing allocates, and worlds that never touch events pay
// nothing at all.
//
// Like the rest of the World, an Events queue is not goroutine-safe: do not
// Send or read from inside an EachParallel pass.
type Events[T any] struct {
	front      []T    // events sent since the last flush
	back       []T    // events sent the frame before that
	frontStart uint64 // sequence number of front[0]; back ends here
}

// eventQueue is the type-erased view of an Events[T], used by the World to
// advance every queue in FlushEvents and to drop them all in Clean.
type eventQueue interface {
	flush()
	clear()
}

// Send appends an event to the queue. It stays readable until the second
// FlushEvents from now.
func (ev *Events[T]) Send(event T) {
	ev.front = append(ev.front, event)
}

// Reader returns a new independent cursor over the queue. Create one per
// consuming system (at construction, alongside its queries) and store it —
// the cursor is what remembers which events this system has already seen.
// A fresh reader starts before the oldest buffered event, so it also sees
// the up-to-two frames of history still in the queue.
func (ev *Events[T]) Reader() EventReader[T] {
	return EventReader[T]{ev: ev}
}

// flush advances the queue one frame: last frame's events are dropped and
// the events sent since the previous flush become "last frame's". Both
// buffers keep their capacity, so steady-state event traffic never
// allocates.
func (ev *Events[T]) flush() {
	ev.frontStart += uint64(len(ev.front))
	clear(ev.back) // zero dropped events so anything they point at can be GC'd
	ev.front, ev.back = ev.back[:0], ev.front
}

// clear drops every buffered event at once, keeping both buffers' capacity.
// Sequence numbers advance past the dropped events instead of rewinding, so an
// existing reader — whose cursor may sit anywhere in the discarded range —
// resumes at the next event sent rather than replaying or skipping one.
func (ev *Events[T]) clear() {
	ev.frontStart += uint64(len(ev.front))
	clear(ev.front) // zero dropped events so anything they point at can be GC'd
	clear(ev.back)
	ev.front = ev.front[:0]
	ev.back = ev.back[:0]
}

// EventReader is one consumer's cursor into an Events[T] queue. Each reader
// advances independently; reading never removes events from the queue.
//
// Store the reader by value in the consuming system and call Each through a
// pointer (methods mutate the cursor).
type EventReader[T any] struct {
	ev   *Events[T]
	next uint64 // sequence number of the first event this reader has not seen
}

// Each calls fn once for every event this reader has not seen yet, oldest
// first, and marks them seen. Events are passed by value: they are shared
// by every reader of the queue, so treat them as immutable messages.
//
// Events sent by fn itself (same type, from inside the callback) are not
// visited by this call — they are picked up by the next Each, which keeps a
// system that reacts to events by sending more from looping forever.
func (r *EventReader[T]) Each(fn func(T)) {
	ev := r.ev
	backStart := ev.frontStart - uint64(len(ev.back))
	if r.next < backStart {
		r.next = backStart // anything older has been flushed away
	}
	for r.next < ev.frontStart {
		fn(ev.back[r.next-backStart])
		r.next++
	}
	end := ev.frontStart + uint64(len(ev.front))
	for r.next < end {
		fn(ev.front[r.next-ev.frontStart])
		r.next++
	}
}

// Clear marks every currently buffered event as seen without visiting it.
func (r *EventReader[T]) Clear() {
	r.next = r.ev.frontStart + uint64(len(r.ev.front))
}

// Events returns the event queue for type T, creating it on first use — the
// event-side twin of Storage. Producers and consumers that name the same
// type get the same queue; that shared type is their only coupling.
//
// Like Storage, the lookup costs a reflection map hit: call it at system
// construction and keep the result, not inside the frame loop.
func (w *World) Events[T any]() *Events[T] {
	t := reflect.TypeFor[T]()
	if q, ok := w.events[t]; ok {
		return q.(*Events[T])
	}
	q := &Events[T]{}
	w.events[t] = q
	w.eventsList = append(w.eventsList, q)
	return q
}

// Send appends an event to the world's queue for type T. Convenience twin of
// World.Set: it pays the reflection map lookup per call, so hot producers
// should cache w.Events[T]() and Send on that instead.
func (w *World) Send[T any](event T) {
	w.Events[T]().Send(event)
}

// FlushEvents advances every event queue one frame. Call it exactly once
// per frame, after all systems have run. Events sent during frame N are
// readable for the rest of frame N and all of frame N+1, then dropped.
func (w *World) FlushEvents() {
	for _, q := range w.eventsList {
		q.flush()
	}
}
