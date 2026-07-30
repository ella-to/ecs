package ecs

import "reflect"

// none marks an empty slot in a sparse array.
const none = ^uint32(0)

// hasPointers reports whether values of type t contain anything the garbage
// collector traces. Storage asks this once per component type, when the
// storage is created, so clear knows whether dropped values must be zeroed.
func hasPointers(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.String, reflect.Slice,
		reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		return true
	case reflect.Array:
		return t.Len() > 0 && hasPointers(t.Elem())
	case reflect.Struct:
		for f := range t.Fields() {
			if hasPointers(f.Type) {
				return true
			}
		}
	}
	// Everything left — bools, all the numeric kinds, uintptr — is opaque bytes.
	return false
}

// storage is the type-erased view of a Storage[T], used by the World to
// clean up components when an entity is destroyed or the world is cleaned.
type storage interface {
	removeIndex(index uint32)
	clear()
}

// Storage is a sparse set holding every component of type T in the World.
//
// All component values live in one densely packed slice, so iterating them is
// a linear scan over contiguous memory. The sparse slice maps an entity index
// to its slot in the dense slice, giving O(1) lookup, insert, and remove.
//
// World.Set/Get/Has/Remove look the storage up in a reflection-keyed map on
// every call. The equivalent methods here skip that: cache the storage once
// (via World.Storage) and spawn/mutate through it in hot paths.
type Storage[T any] struct {
	w        *World
	sparse   []uint32 // entity index -> dense index, or none
	dense    []T      // packed component values
	entities []uint32 // dense index -> entity index (mirrors dense)
	hasPtrs  bool     // T holds pointers, so clear must zero dropped values
}

// Len returns the number of components currently stored.
func (s *Storage[T]) Len() int { return len(s.dense) }

// Set attaches a component to e, replacing any existing one. Equivalent to
// World.Set without the per-call storage lookup.
func (s *Storage[T]) Set(e Entity, value T) {
	if !s.w.Alive(e) {
		return
	}
	s.set(e.index(), value)
}

// Get returns a pointer to e's component, or nil if e is dead or has no such
// component. Equivalent to World.Get without the per-call storage lookup.
func (s *Storage[T]) Get(e Entity) *T {
	if !s.w.Alive(e) {
		return nil
	}
	return s.get(e.index())
}

// Has reports whether e has this component.
func (s *Storage[T]) Has(e Entity) bool { return s.Get(e) != nil }

// Remove detaches e's component, if present.
func (s *Storage[T]) Remove(e Entity) {
	if !s.w.Alive(e) {
		return
	}
	s.removeIndex(e.index())
}

// denseIndex returns the dense slot for an entity index, or none.
func (s *Storage[T]) denseIndex(index uint32) uint32 {
	if index >= uint32(len(s.sparse)) {
		return none
	}
	return s.sparse[index]
}

func (s *Storage[T]) get(index uint32) *T {
	d := s.denseIndex(index)
	if d == none {
		return nil
	}
	return &s.dense[d]
}

func (s *Storage[T]) set(index uint32, value T) {
	if d := s.denseIndex(index); d != none {
		s.dense[d] = value
		return
	}
	for uint32(len(s.sparse)) <= index {
		s.sparse = append(s.sparse, none)
	}
	s.sparse[index] = uint32(len(s.dense))
	s.dense = append(s.dense, value)
	s.entities = append(s.entities, index)
}

// removeIndex deletes the component by swapping the last dense element into
// its slot, keeping the dense slice packed with no holes.
func (s *Storage[T]) removeIndex(index uint32) {
	d := s.denseIndex(index)
	if d == none {
		return
	}
	last := uint32(len(s.dense) - 1)
	s.dense[d] = s.dense[last]
	s.entities[d] = s.entities[last]
	s.sparse[s.entities[d]] = d
	var zero T
	s.dense[last] = zero
	s.dense = s.dense[:last]
	s.entities = s.entities[:last]
	s.sparse[index] = none
}

// clear drops every component in one shot, keeping the backing arrays for
// reuse. Truncating sparse to zero length is what makes this O(1) work per
// storage instead of a per-slot reset: denseIndex already reports none for any
// index past the end, and set regrows sparse into the retained capacity.
func (s *Storage[T]) clear() {
	// Dropped values only need zeroing if they hold pointers, in which case the
	// retained dense array would otherwise pin the whole previous scene's heap.
	// A pointer-free component is just stale bytes that the next set overwrites,
	// and skipping the memclr is most of the cost of clearing a big storage.
	if s.hasPtrs {
		clear(s.dense)
	}
	s.dense = s.dense[:0]
	s.entities = s.entities[:0]
	s.sparse = s.sparse[:0]
}
