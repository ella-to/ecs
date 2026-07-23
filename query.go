package ecs

import (
	"runtime"
	"sync"
)

// minParallelChunk is the smallest slice of a pass worth a goroutine: below
// a few thousand entities the fan-out overhead outweighs the parallelism, so
// EachParallel caps its worker count to keep chunks at least this large.
const minParallelChunk = 4096

// eachParallel splits [0, n) into contiguous ranges and runs them on up to
// `workers` goroutines. workers <= 0 means one per CPU; a single worker (or a
// pass too small to split) runs inline on the calling goroutine with no
// synchronization at all.
func eachParallel(n, workers int, run func(lo, hi int)) {
	if n == 0 {
		return
	}
	if workers <= 0 {
		// Re-read every pass on purpose: it's a cheap query, and since Go
		// 1.25 the runtime may adjust GOMAXPROCS while the program runs
		// (e.g. container CPU limits changing).
		workers = runtime.GOMAXPROCS(0)
	}
	if maxWorkers := (n + minParallelChunk - 1) / minParallelChunk; workers > maxWorkers {
		workers = maxWorkers
	}
	if workers <= 1 {
		run(0, n)
		return
	}
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += chunk {
		hi := min(lo+chunk, n)
		wg.Go(func() { run(lo, hi) })
	}
	wg.Wait()
}

// Query iterates every entity that has component A.
//
// Create a query once (typically when a system is constructed) and reuse it
// every frame: construction resolves the storage, so Each is just a loop over
// a packed slice.
type Query[A any] struct {
	w *World
	a *Storage[A]
}

// Query returns a query over all entities with component A.
func (w *World) Query[A any]() Query[A] {
	return Query[A]{w: w, a: w.Storage[A]()}
}

// Count returns the number of matching entities.
func (q Query[A]) Count() int { return q.a.Len() }

// Each calls fn for every entity with component A. Iteration runs backwards
// over the dense slice, so destroying the current entity (or removing its A
// component) inside fn is safe.
func (q Query[A]) Each(fn func(Entity, *A)) {
	a, w := q.a, q.w
	for d := len(a.dense) - 1; d >= 0; d-- {
		index := a.entities[d]
		fn(newEntity(index, w.versions[index]), &a.dense[d])
	}
}

// EachParallel calls fn for every entity with component A, splitting the
// pass across worker goroutines. fn receives the entity's dense slot index —
// stable for the duration of the pass and unique per entity, so concurrent
// workers can write results into disjoint slots of one shared buffer with no
// locks (e.g. vertices at index*4).
//
// workers selects the parallelism: <= 0 means one worker per CPU
// (runtime.GOMAXPROCS(0), re-read each pass since the runtime may adjust it),
// and 1 runs inline on the calling goroutine with no goroutines or WaitGroup
// at all. The count is also capped so each worker gets at least a few
// thousand entities — small passes always run inline, so EachParallel(0, fn)
// is safe as a default.
//
// Unlike Each, iteration is forward and NO structural change to the world
// (Destroy, Remove, Set of a new component, NewEntity) is allowed anywhere
// during the pass. No two workers ever see the same entity.
func (q Query[A]) EachParallel(workers int, fn func(int, Entity, *A)) {
	eachParallel(q.a.Len(), workers, func(lo, hi int) { q.eachRange(lo, hi, fn) })
}

func (q Query[A]) eachRange(lo, hi int, fn func(int, Entity, *A)) {
	a, w := q.a, q.w
	for d := lo; d < hi; d++ {
		index := a.entities[d]
		fn(d, newEntity(index, w.versions[index]), &a.dense[d])
	}
}

// Query2 iterates every entity that has both components A and B.
type Query2[A, B any] struct {
	w *World
	a *Storage[A]
	b *Storage[B]
}

// Query2 returns a query over all entities with components A and B.
func (w *World) Query2[A, B any]() Query2[A, B] {
	return Query2[A, B]{w: w, a: w.Storage[A](), b: w.Storage[B]()}
}

// Each calls fn for every entity with both A and B. It walks the smaller of
// the two storages and probes the other, so cost scales with the rarer
// component. As with Query.Each, destroying the current entity inside fn is
// safe.
func (q Query2[A, B]) Each(fn func(Entity, *A, *B)) {
	a, b, w := q.a, q.b, q.w
	if a.Len() <= b.Len() {
		for d := len(a.dense) - 1; d >= 0; d-- {
			index := a.entities[d]
			if bd := b.denseIndex(index); bd != none {
				fn(newEntity(index, w.versions[index]), &a.dense[d], &b.dense[bd])
			}
		}
	} else {
		for d := len(b.dense) - 1; d >= 0; d-- {
			index := b.entities[d]
			if ad := a.denseIndex(index); ad != none {
				fn(newEntity(index, w.versions[index]), &a.dense[ad], &b.dense[d])
			}
		}
	}
}

// Len returns the number of dense slots in the driving storage (component A):
// an upper bound on matches, and the slot-index space of EachParallel.
func (q Query2[A, B]) Len() int { return q.a.Len() }

// EachParallel is the multi-goroutine variant of Each — see Query.EachParallel
// for the workers semantics and rules. Unlike Each it always drives from A's
// storage (so cost scales with A, not the rarest component) and skips
// entities lacking B.
func (q Query2[A, B]) EachParallel(workers int, fn func(int, Entity, *A, *B)) {
	eachParallel(q.a.Len(), workers, func(lo, hi int) { q.eachRange(lo, hi, fn) })
}

func (q Query2[A, B]) eachRange(lo, hi int, fn func(int, Entity, *A, *B)) {
	a, b, w := q.a, q.b, q.w
	for d := lo; d < hi; d++ {
		index := a.entities[d]
		if bd := b.denseIndex(index); bd != none {
			fn(d, newEntity(index, w.versions[index]), &a.dense[d], &b.dense[bd])
		}
	}
}

// Query3 iterates every entity that has components A, B, and C.
type Query3[A, B, C any] struct {
	w *World
	a *Storage[A]
	b *Storage[B]
	c *Storage[C]
}

// Query3 returns a query over all entities with components A, B, and C.
func (w *World) Query3[A, B, C any]() Query3[A, B, C] {
	return Query3[A, B, C]{w: w, a: w.Storage[A](), b: w.Storage[B](), c: w.Storage[C]()}
}

// Each calls fn for every entity with all of A, B, and C. It walks the
// smallest of the three storages and probes the other two, so cost scales
// with the rarest component. As with Query.Each, destroying the current
// entity inside fn is safe.
func (q Query3[A, B, C]) Each(fn func(Entity, *A, *B, *C)) {
	a, b, c, w := q.a, q.b, q.c, q.w
	switch {
	case a.Len() <= b.Len() && a.Len() <= c.Len():
		for d := len(a.dense) - 1; d >= 0; d-- {
			index := a.entities[d]
			bd := b.denseIndex(index)
			if bd == none {
				continue
			}
			cd := c.denseIndex(index)
			if cd == none {
				continue
			}
			fn(newEntity(index, w.versions[index]), &a.dense[d], &b.dense[bd], &c.dense[cd])
		}
	case b.Len() <= c.Len():
		for d := len(b.dense) - 1; d >= 0; d-- {
			index := b.entities[d]
			ad := a.denseIndex(index)
			if ad == none {
				continue
			}
			cd := c.denseIndex(index)
			if cd == none {
				continue
			}
			fn(newEntity(index, w.versions[index]), &a.dense[ad], &b.dense[d], &c.dense[cd])
		}
	default:
		for d := len(c.dense) - 1; d >= 0; d-- {
			index := c.entities[d]
			ad := a.denseIndex(index)
			if ad == none {
				continue
			}
			bd := b.denseIndex(index)
			if bd == none {
				continue
			}
			fn(newEntity(index, w.versions[index]), &a.dense[ad], &b.dense[bd], &c.dense[d])
		}
	}
}

// Len returns the number of dense slots in the driving storage (component A):
// an upper bound on matches, and the slot-index space of EachParallel.
func (q Query3[A, B, C]) Len() int { return q.a.Len() }

// EachParallel is the multi-goroutine variant of Each — see Query.EachParallel
// for the workers semantics and rules. Unlike Each it always drives from A's
// storage and skips entities lacking B or C.
func (q Query3[A, B, C]) EachParallel(workers int, fn func(int, Entity, *A, *B, *C)) {
	eachParallel(q.a.Len(), workers, func(lo, hi int) { q.eachRange(lo, hi, fn) })
}

func (q Query3[A, B, C]) eachRange(lo, hi int, fn func(int, Entity, *A, *B, *C)) {
	a, b, c, w := q.a, q.b, q.c, q.w
	for d := lo; d < hi; d++ {
		index := a.entities[d]
		bd := b.denseIndex(index)
		if bd == none {
			continue
		}
		cd := c.denseIndex(index)
		if cd == none {
			continue
		}
		fn(d, newEntity(index, w.versions[index]), &a.dense[d], &b.dense[bd], &c.dense[cd])
	}
}

// smallest returns the shortest of the given entity lists — the driver side
// for a multi-component query. At four and more components, Query4 and Query5
// walk this list and probe every storage (including the driver's own — one
// redundant two-array-read probe) rather than unrolling a branch per driver
// as Query2 and Query3 do.
func smallest(lists ...[]uint32) []uint32 {
	driver := lists[0]
	for _, l := range lists[1:] {
		if len(l) < len(driver) {
			driver = l
		}
	}
	return driver
}

// Query4 iterates every entity that has components A, B, C, and D.
type Query4[A, B, C, D any] struct {
	w *World
	a *Storage[A]
	b *Storage[B]
	c *Storage[C]
	d *Storage[D]
}

// Query4 returns a query over all entities with components A, B, C, and D.
func (w *World) Query4[A, B, C, D any]() Query4[A, B, C, D] {
	return Query4[A, B, C, D]{
		w: w,
		a: w.Storage[A](), b: w.Storage[B](), c: w.Storage[C](), d: w.Storage[D](),
	}
}

// Each calls fn for every entity with all of A, B, C, and D. It walks the
// smallest of the four storages and probes the others, so cost scales with
// the rarest component. As with Query.Each, destroying the current entity
// inside fn is safe.
func (q Query4[A, B, C, D]) Each(fn func(Entity, *A, *B, *C, *D)) {
	a, b, c, d, w := q.a, q.b, q.c, q.d, q.w
	driver := smallest(a.entities, b.entities, c.entities, d.entities)
	for i := len(driver) - 1; i >= 0; i-- {
		index := driver[i]
		ad := a.denseIndex(index)
		if ad == none {
			continue
		}
		bd := b.denseIndex(index)
		if bd == none {
			continue
		}
		cd := c.denseIndex(index)
		if cd == none {
			continue
		}
		dd := d.denseIndex(index)
		if dd == none {
			continue
		}
		fn(newEntity(index, w.versions[index]),
			&a.dense[ad], &b.dense[bd], &c.dense[cd], &d.dense[dd])
	}
}

// Len returns the number of dense slots in the driving storage (component A):
// an upper bound on matches, and the slot-index space of EachParallel.
func (q Query4[A, B, C, D]) Len() int { return q.a.Len() }

// EachParallel is the multi-goroutine variant of Each — see Query.EachParallel
// for the workers semantics and rules. Unlike Each it always drives from A's
// storage and skips entities lacking any other component.
func (q Query4[A, B, C, D]) EachParallel(workers int, fn func(int, Entity, *A, *B, *C, *D)) {
	eachParallel(q.a.Len(), workers, func(lo, hi int) { q.eachRange(lo, hi, fn) })
}

func (q Query4[A, B, C, D]) eachRange(lo, hi int, fn func(int, Entity, *A, *B, *C, *D)) {
	a, b, c, d, w := q.a, q.b, q.c, q.d, q.w
	for i := lo; i < hi; i++ {
		index := a.entities[i]
		bd := b.denseIndex(index)
		if bd == none {
			continue
		}
		cd := c.denseIndex(index)
		if cd == none {
			continue
		}
		dd := d.denseIndex(index)
		if dd == none {
			continue
		}
		fn(i, newEntity(index, w.versions[index]),
			&a.dense[i], &b.dense[bd], &c.dense[cd], &d.dense[dd])
	}
}

// Query5 iterates every entity that has components A, B, C, D, and E.
type Query5[A, B, C, D, E any] struct {
	w *World
	a *Storage[A]
	b *Storage[B]
	c *Storage[C]
	d *Storage[D]
	e *Storage[E]
}

// Query5 returns a query over all entities with components A, B, C, D, and E.
func (w *World) Query5[A, B, C, D, E any]() Query5[A, B, C, D, E] {
	return Query5[A, B, C, D, E]{
		w: w,
		a: w.Storage[A](), b: w.Storage[B](), c: w.Storage[C](), d: w.Storage[D](), e: w.Storage[E](),
	}
}

// Each calls fn for every entity with all of A, B, C, D, and E. It walks the
// smallest of the five storages and probes the others, so cost scales with
// the rarest component. As with Query.Each, destroying the current entity
// inside fn is safe.
func (q Query5[A, B, C, D, E]) Each(fn func(Entity, *A, *B, *C, *D, *E)) {
	a, b, c, d, e, w := q.a, q.b, q.c, q.d, q.e, q.w
	driver := smallest(a.entities, b.entities, c.entities, d.entities, e.entities)
	for i := len(driver) - 1; i >= 0; i-- {
		index := driver[i]
		ad := a.denseIndex(index)
		if ad == none {
			continue
		}
		bd := b.denseIndex(index)
		if bd == none {
			continue
		}
		cd := c.denseIndex(index)
		if cd == none {
			continue
		}
		dd := d.denseIndex(index)
		if dd == none {
			continue
		}
		ed := e.denseIndex(index)
		if ed == none {
			continue
		}
		fn(newEntity(index, w.versions[index]),
			&a.dense[ad], &b.dense[bd], &c.dense[cd], &d.dense[dd], &e.dense[ed])
	}
}

// Len returns the number of dense slots in the driving storage (component A):
// an upper bound on matches, and the slot-index space of EachParallel.
func (q Query5[A, B, C, D, E]) Len() int { return q.a.Len() }

// EachParallel is the multi-goroutine variant of Each — see Query.EachParallel
// for the workers semantics and rules. Unlike Each it always drives from A's
// storage and skips entities lacking any other component.
func (q Query5[A, B, C, D, E]) EachParallel(workers int, fn func(int, Entity, *A, *B, *C, *D, *E)) {
	eachParallel(q.a.Len(), workers, func(lo, hi int) { q.eachRange(lo, hi, fn) })
}

func (q Query5[A, B, C, D, E]) eachRange(lo, hi int, fn func(int, Entity, *A, *B, *C, *D, *E)) {
	a, b, c, d, e, w := q.a, q.b, q.c, q.d, q.e, q.w
	for i := lo; i < hi; i++ {
		index := a.entities[i]
		bd := b.denseIndex(index)
		if bd == none {
			continue
		}
		cd := c.denseIndex(index)
		if cd == none {
			continue
		}
		dd := d.denseIndex(index)
		if dd == none {
			continue
		}
		ed := e.denseIndex(index)
		if ed == none {
			continue
		}
		fn(i, newEntity(index, w.versions[index]),
			&a.dense[i], &b.dense[bd], &c.dense[cd], &d.dense[dd], &e.dense[ed])
	}
}
