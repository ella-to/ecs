package ecs

// Entity is a unique handle to an object in a World.
//
// The low 32 bits are an index into the World's internal tables and the high
// 32 bits are a version counter. When an entity is destroyed its index is
// recycled with a bumped version, so stale handles held by game code are
// safely detected as dead instead of pointing at a new entity.
type Entity uint64

// Nil is the zero Entity. Live entities always have a version of at least 1,
// so Nil (and any zero-valued Entity field) never refers to a live entity:
// World.Alive(Nil) is always false. Use it as a "no entity" sentinel.
const Nil Entity = 0

func newEntity(index, version uint32) Entity {
	return Entity(index) | Entity(version)<<32
}

func (e Entity) index() uint32   { return uint32(e) }
func (e Entity) version() uint32 { return uint32(e >> 32) }
