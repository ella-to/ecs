package ecs

import "reflect"

// World owns all entities and their components.
//
// Component access uses Go 1.27 generic methods: the component type is a type
// argument, so there is no interface boxing and no type assertions in user
// code, e.g.
//
//	e := w.NewEntity()
//	w.Set(e, Position{X: 10})
//	pos := w.Get[Position](e)
type World struct {
	versions   []uint32 // current version per entity index
	free       []uint32 // destroyed indices ready for reuse
	storages   map[reflect.Type]storage
	list       []storage // same storages as a flat slice — Destroy iterates this, not the map
	events     map[reflect.Type]eventQueue
	eventsList []eventQueue // same queues as a flat slice — FlushEvents iterates this
}

// NewWorld creates an empty World.
func NewWorld() *World {
	return &World{
		storages: make(map[reflect.Type]storage),
		events:   make(map[reflect.Type]eventQueue),
	}
}

// NewEntity creates a new entity with no components.
func (w *World) NewEntity() Entity {
	if n := len(w.free); n > 0 {
		index := w.free[n-1]
		w.free = w.free[:n-1]
		return newEntity(index, w.versions[index])
	}
	index := uint32(len(w.versions))
	w.versions = append(w.versions, 1) // versions start at 1 so the zero Entity is never alive
	return newEntity(index, 1)
}

// Alive reports whether e still refers to a live entity.
func (w *World) Alive(e Entity) bool {
	index := e.index()
	return index < uint32(len(w.versions)) && w.versions[index] == e.version()
}

// Destroy removes an entity and all of its components. The entity's index is
// recycled, but any Entity handles to it become dead (Alive returns false).
func (w *World) Destroy(e Entity) {
	if !w.Alive(e) {
		return
	}
	index := e.index()
	for _, s := range w.list {
		s.removeIndex(index)
	}
	w.versions[index]++
	w.free = append(w.free, index)
}

// Storage returns the Storage for component type T, creating it on first use.
// Queries cache these, so the map lookup happens once per query, not per call.
//
// Hot paths that Set/Get outside a query (e.g. spawning bursts of entities)
// should cache the returned *Storage[T] and use its methods directly: they do
// the same work as World.Set/Get minus the per-call reflection map lookup.
func (w *World) Storage[T any]() *Storage[T] {
	t := reflect.TypeFor[T]()
	if s, ok := w.storages[t]; ok {
		return s.(*Storage[T])
	}
	s := &Storage[T]{w: w}
	w.storages[t] = s
	w.list = append(w.list, s)
	return s
}

// Set attaches a component of type T to e, replacing any existing one.
func (w *World) Set[T any](e Entity, value T) {
	if !w.Alive(e) {
		return
	}
	w.Storage[T]().set(e.index(), value)
}

// Get returns a pointer to e's component of type T, or nil if e is dead or
// has no such component. The pointer is valid until the component is removed.
func (w *World) Get[T any](e Entity) *T {
	if !w.Alive(e) {
		return nil
	}
	return w.Storage[T]().get(e.index())
}

// Has reports whether e has a component of type T.
func (w *World) Has[T any](e Entity) bool {
	return w.Get[T](e) != nil
}

// Remove detaches e's component of type T, if present.
func (w *World) Remove[T any](e Entity) {
	if !w.Alive(e) {
		return
	}
	w.Storage[T]().removeIndex(e.index())
}
