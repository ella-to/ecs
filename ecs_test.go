package ecs

import (
	"sync"
	"testing"
)

type pos struct{ X, Y float64 }
type vel struct{ X, Y float64 }
type acc struct{ X, Y float64 }
type hp struct{ X float64 }
type tag struct{ X float64 }

func TestZeroEntityNeverAlive(t *testing.T) {
	w := NewWorld()
	if w.Alive(Nil) {
		t.Fatal("Nil entity reported alive in empty world")
	}

	// Even after the first entity claims index 0, the zero-valued handle
	// (index 0, version 0) must not alias it.
	first := w.NewEntity()
	w.Set(first, pos{X: 1})
	if w.Alive(Nil) {
		t.Fatal("Nil entity aliases the first created entity")
	}
	if w.Get[pos](Nil) != nil {
		t.Fatal("Nil entity can read the first entity's component")
	}
}

func TestSetGetRemove(t *testing.T) {
	w := NewWorld()
	e := w.NewEntity()

	if w.Has[pos](e) {
		t.Fatal("new entity should have no components")
	}

	w.Set(e, pos{X: 1, Y: 2})
	p := w.Get[pos](e)
	if p == nil || p.X != 1 || p.Y != 2 {
		t.Fatalf("Get after Set = %v, want &{1 2}", p)
	}

	p.X = 5 // mutate through the pointer
	if w.Get[pos](e).X != 5 {
		t.Fatal("mutation through Get pointer was lost")
	}

	w.Set(e, pos{X: 9}) // Set replaces
	if w.Get[pos](e).X != 9 {
		t.Fatal("Set did not replace existing component")
	}

	w.Remove[pos](e)
	if w.Has[pos](e) {
		t.Fatal("component still present after Remove")
	}
}

func TestDestroyInvalidatesHandle(t *testing.T) {
	w := NewWorld()
	e := w.NewEntity()
	w.Set(e, pos{X: 1})
	w.Destroy(e)

	if w.Alive(e) {
		t.Fatal("destroyed entity reported alive")
	}
	if w.Get[pos](e) != nil {
		t.Fatal("Get on dead entity should return nil")
	}

	// The index is recycled but the old handle must stay dead.
	e2 := w.NewEntity()
	w.Set(e2, pos{X: 42})
	if w.Alive(e) {
		t.Fatal("stale handle came back to life after index reuse")
	}
	if w.Get[pos](e) != nil {
		t.Fatal("stale handle can read the new entity's component")
	}
	if w.Get[pos](e2).X != 42 {
		t.Fatal("recycled entity lost its component")
	}
}

func TestQuery2MatchesIntersection(t *testing.T) {
	w := NewWorld()

	both := w.NewEntity()
	w.Set(both, pos{})
	w.Set(both, vel{X: 1})

	posOnly := w.NewEntity()
	w.Set(posOnly, pos{})

	velOnly := w.NewEntity()
	w.Set(velOnly, vel{})

	got := 0
	w.Query2[pos, vel]().Each(func(e Entity, _ *pos, v *vel) {
		got++
		if e != both {
			t.Fatalf("matched wrong entity %v", e)
		}
		if v.X != 1 {
			t.Fatalf("wrong component data: %v", v)
		}
	})
	if got != 1 {
		t.Fatalf("Query2 matched %d entities, want 1", got)
	}
}

func TestQuery3MatchesIntersection(t *testing.T) {
	w := NewWorld()

	all := w.NewEntity()
	w.Set(all, pos{X: 1})
	w.Set(all, vel{X: 2})
	w.Set(all, acc{X: 3})

	// Entities with every strict subset of the three components.
	for _, setup := range []func(Entity){
		func(e Entity) { w.Set(e, pos{}) },
		func(e Entity) { w.Set(e, vel{}) },
		func(e Entity) { w.Set(e, acc{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, acc{}) },
		func(e Entity) { w.Set(e, vel{}); w.Set(e, acc{}) },
	} {
		setup(w.NewEntity())
	}

	got := 0
	w.Query3[pos, vel, acc]().Each(func(e Entity, p *pos, v *vel, a *acc) {
		got++
		if e != all {
			t.Fatalf("matched wrong entity %v", e)
		}
		if p.X != 1 || v.X != 2 || a.X != 3 {
			t.Fatalf("wrong component data: %v %v %v", p, v, a)
		}
	})
	if got != 1 {
		t.Fatalf("Query3 matched %d entities, want 1", got)
	}
}

func TestQuery4MatchesIntersection(t *testing.T) {
	w := NewWorld()

	all := w.NewEntity()
	w.Set(all, pos{X: 1})
	w.Set(all, vel{X: 2})
	w.Set(all, acc{X: 3})
	w.Set(all, hp{X: 4})

	// Entities missing exactly one of the four components.
	for _, setup := range []func(Entity){
		func(e Entity) { w.Set(e, vel{}); w.Set(e, acc{}); w.Set(e, hp{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, acc{}); w.Set(e, hp{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}); w.Set(e, hp{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}); w.Set(e, acc{}) },
	} {
		setup(w.NewEntity())
	}

	got := 0
	w.Query4[pos, vel, acc, hp]().Each(func(e Entity, p *pos, v *vel, a *acc, h *hp) {
		got++
		if e != all {
			t.Fatalf("matched wrong entity %v", e)
		}
		if p.X != 1 || v.X != 2 || a.X != 3 || h.X != 4 {
			t.Fatalf("wrong component data: %v %v %v %v", p, v, a, h)
		}
	})
	if got != 1 {
		t.Fatalf("Query4 matched %d entities, want 1", got)
	}
}

func TestQuery5MatchesIntersection(t *testing.T) {
	w := NewWorld()

	all := w.NewEntity()
	w.Set(all, pos{X: 1})
	w.Set(all, vel{X: 2})
	w.Set(all, acc{X: 3})
	w.Set(all, hp{X: 4})
	w.Set(all, tag{X: 5})

	// Entities missing exactly one of the five components.
	for _, setup := range []func(Entity){
		func(e Entity) { w.Set(e, vel{}); w.Set(e, acc{}); w.Set(e, hp{}); w.Set(e, tag{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, acc{}); w.Set(e, hp{}); w.Set(e, tag{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}); w.Set(e, hp{}); w.Set(e, tag{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}); w.Set(e, acc{}); w.Set(e, tag{}) },
		func(e Entity) { w.Set(e, pos{}); w.Set(e, vel{}); w.Set(e, acc{}); w.Set(e, hp{}) },
	} {
		setup(w.NewEntity())
	}

	got := 0
	w.Query5[pos, vel, acc, hp, tag]().Each(func(e Entity, p *pos, v *vel, a *acc, h *hp, g *tag) {
		got++
		if e != all {
			t.Fatalf("matched wrong entity %v", e)
		}
		if p.X != 1 || v.X != 2 || a.X != 3 || h.X != 4 || g.X != 5 {
			t.Fatalf("wrong component data: %v %v %v %v %v", p, v, a, h, g)
		}
	})
	if got != 1 {
		t.Fatalf("Query5 matched %d entities, want 1", got)
	}
}

func TestEachParallelCoversAllOnce(t *testing.T) {
	w := NewWorld()
	const n = 20_000 // large enough that workers > 1 actually fan out
	for i := range n {
		e := w.NewEntity()
		w.Set(e, pos{X: float64(i)})
		if i%3 != 0 {
			w.Set(e, vel{X: 1}) // every third entity lacks vel and must be skipped
		}
	}

	q := w.Query2[pos, vel]()
	if q.Len() != n {
		t.Fatalf("Len() = %d, want %d (drives from A)", q.Len(), n)
	}
	want := n - (n+2)/3 // entities that have both components

	// 1 = inline serial path, 4 = WaitGroup fan-out, 0 = one worker per CPU.
	for _, workers := range []int{1, 4, 0} {
		var mu sync.Mutex
		seen := make(map[float64]int, want)
		slots := make(map[int]bool, want)
		q.EachParallel(workers, func(i int, e Entity, p *pos, v *vel) {
			if i < 0 || i >= n {
				t.Errorf("workers=%d: slot index %d out of range [0, %d)", workers, i, n)
			}
			if !w.Alive(e) || v.X != 1 {
				t.Errorf("workers=%d: bad entity or component data at slot %d", workers, i)
			}
			mu.Lock()
			seen[p.X]++
			slots[i] = true
			mu.Unlock()
		})
		for x, c := range seen {
			if c != 1 {
				t.Fatalf("workers=%d: entity with pos.X=%v visited %d times, want 1", workers, x, c)
			}
		}
		if len(seen) != want || len(slots) != want {
			t.Fatalf("workers=%d: visited %d entities in %d distinct slots, want %d",
				workers, len(seen), len(slots), want)
		}
	}
}

func TestQuery4DestroyCurrentDuringIteration(t *testing.T) {
	w := NewWorld()
	for i := range 10 {
		e := w.NewEntity()
		w.Set(e, pos{X: float64(i)})
		w.Set(e, vel{})
		w.Set(e, acc{})
		w.Set(e, hp{})
	}

	visited := 0
	w.Query4[pos, vel, acc, hp]().Each(func(e Entity, _ *pos, _ *vel, _ *acc, _ *hp) {
		visited++
		w.Destroy(e)
	})
	if visited != 10 {
		t.Fatalf("visited %d entities, want 10", visited)
	}
	if n := w.Query[pos]().Count(); n != 0 {
		t.Fatalf("%d entities survived destroy-during-iteration", n)
	}
}

func TestSwapRemoveKeepsStorageConsistent(t *testing.T) {
	w := NewWorld()
	entities := make([]Entity, 10)
	for i := range entities {
		entities[i] = w.NewEntity()
		w.Set(entities[i], pos{X: float64(i)})
	}

	// Remove from the middle; the last element gets swapped into the hole.
	w.Remove[pos](entities[3])

	for i, e := range entities {
		if i == 3 {
			if w.Has[pos](e) {
				t.Fatal("removed component still present")
			}
			continue
		}
		p := w.Get[pos](e)
		if p == nil || p.X != float64(i) {
			t.Fatalf("entity %d has wrong data after swap-remove: %v", i, p)
		}
	}
	if n := w.Query[pos]().Count(); n != 9 {
		t.Fatalf("Count = %d, want 9", n)
	}
}

func TestDestroyDuringIteration(t *testing.T) {
	w := NewWorld()
	for range 100 {
		e := w.NewEntity()
		w.Set(e, pos{})
	}

	visited := 0
	w.Query[pos]().Each(func(e Entity, _ *pos) {
		visited++
		w.Destroy(e) // must be safe mid-iteration
	})
	if visited != 100 {
		t.Fatalf("visited %d entities, want 100", visited)
	}
	if n := w.Query[pos]().Count(); n != 0 {
		t.Fatalf("%d entities left after destroying all, want 0", n)
	}
}
