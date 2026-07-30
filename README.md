# ecs — a sparse-set Entity Component System

A small, zero-dependency ECS built on Go 1.27 generic methods. ~300 lines of
stdlib-only code: no reflection in hot paths, no interface boxing, no
allocations per frame.

```go
w := ecs.NewWorld()

e := w.NewEntity()
w.Set(e, Position{X: 10})            // type inferred from the value
w.Set(e, Velocity{X: 1})

p := w.Get[Position](e)              // *Position, or nil
w.Has[Velocity](e)                   // true
w.Remove[Velocity](e)
w.Destroy(e)                         // removes e and all its components
w.Clean()                            // removes everything — scene transitions

// Build once (e.g. in a system's constructor), reuse every frame:
q := w.Query2[Position, Velocity]()
q.Each(func(e ecs.Entity, p *Position, v *Velocity) {
    p.X += v.X
    p.Y += v.Y
})
```

Components are plain structs. Any type works — there is no registration step;
a component type's storage is created lazily on first use.

## How it works

### Entities are handles, not objects

An `Entity` is a `uint64`: the low 32 bits are an index into the world's
tables, the high 32 bits are a **version**. When an entity is destroyed, its
index goes on a free list and its version is bumped. A recycled index produces
a new handle with a higher version, so any stale handle you kept around fails
the version check and reads as dead — it can never touch the entity that now
occupies the same index.

### Storage is a sparse set (this is where the speed comes from)

Each component type `T` gets one `Storage[T]` with three slices:

```
sparse:   [entity index] -> dense slot (or none)     one uint32 per entity slot
dense:    [slot]         -> T                        tightly packed, no holes
entities: [slot]         -> entity index             mirrors dense
```

- **Data locality:** all `Position` values in the world live back-to-back in
  one `[]Position`. `Query.Each` is a linear walk over that contiguous slice —
  the access pattern CPUs prefetch perfectly. This is Struct-of-Arrays layout:
  a system touching only `Position` never pulls `Velocity` or sprite data
  through the cache.
- **O(1) everything:** `Get`/`Set`/`Remove` are two array indexings. `Remove`
  swaps the last dense element into the vacated slot, so the dense slice never
  fragments.
- **Multi-component queries** (`Query2` through `Query5`) walk the dense slice
  of the *smallest* storage and probe the others through their sparse arrays. The
  driver side is sequential; each probe is two array reads. Cost scales with
  the rarest component, not the world size.

The `w.Get[T](e)`-style API is a Go 1.27 **generic method** — the component
type is a compile-time type argument, so each instantiation compiles to direct
typed slice access. The only reflection is one `map[reflect.Type]` lookup to
find a storage, and queries do that once at construction, never during
iteration.

## Events

Systems often need to tell each other things — "this button was clicked",
"that enemy died" — and the tempting shortcuts (shared variables, one system
calling another) couple systems together and break the ECS model. `Events[T]`
is the decoupled alternative: a per-type, double-buffered queue that producers
push into and any number of consumers poll at their own pace.

```go
type ButtonClicked struct{ Button ecs.Entity }

// Producer (in its constructor, then per frame):
clicks := w.Events[ButtonClicked]()
clicks.Send(ButtonClicked{Button: e})

// Consumer (each system makes its own reader in its constructor):
reader := w.Events[ButtonClicked]().Reader()
reader.Each(func(ev ButtonClicked) { ... })

// Once per frame, after all systems ran:
w.FlushEvents()
```

Producer and consumer never reference each other — the event type is their
entire contract. `w.Send(event)` is the map-lookup convenience twin of
`w.Set`, for code outside hot paths.

The design is the one that won out across the ECS ecosystem (it is Bevy's
`Events`/`EventReader`, and what EnTT's author recommends over its own
callback dispatcher): **pull, not push**. Callbacks/observers run consumer
code in the middle of the producer's iteration — reentrancy hazards,
unpredictable ordering, per-event dispatch overhead. A polled queue keeps
every system running on the schedule you gave it, and processing N events is
one batched linear walk. The "events as entities" pattern (spawn an entity
per event, query it, destroy it) works with plain component storage but pays
entity churn and needs fragile manual cleanup; a dedicated queue does less
work and can't leak.

Semantics that follow from the double buffer:

- **Events live for exactly two flushes.** A consumer scheduled *before* the
  producer still sees the event on the next frame — one frame late, never
  lost. The flip side: a system must poll its reader every frame (or call
  `reader.Clear()` to skip); anything unread after two flushes is gone.
- **Readers are independent cursors.** Reading never removes events, so any
  number of systems can consume the same queue. Each reader tracks a
  monotonic sequence number, so buffer swaps can't corrupt its position. A
  fresh reader sees whatever is still buffered (up to two frames of history).
- **Call `w.FlushEvents()` exactly once per frame**, after all systems.
  Forgetting it means queues grow without bound; calling it twice halves
  event lifetime to "this frame only".
- **Events sent from inside `Each` are deferred** to the next `Each` call,
  so a system that reacts to events by sending more (same type) cannot loop
  forever.
- **Treat events as immutable messages.** `Each` passes them by value; every
  reader of a queue sees the same events.
- Same concurrency rule as the rest of the world: not goroutine-safe, no
  `Send` inside an `EachParallel` pass.

Performance: `Send` is an append into a slice whose capacity is reused after
every flush, reading is a linear walk of packed memory, and `FlushEvents` is
one swap per event type. Steady state allocates nothing (see
`BenchmarkEvents`: ~3 ns/event, 0 allocs). Worlds that never touch events pay
nothing — the feature is invisible until the first `w.Events[T]()` call.

## Performance

Running, `go test -bench=. -benchmem .`, 100 000 entities:

| Benchmark | Result |
|---|---|
| `Query2` full pass (pos += vel) | 183 µs/pass · **1.8 ns/entity** · 0 allocs |
| `Query3` full pass (physics step) | 262 µs/pass · **2.6 ns/entity** · 0 allocs |
| `Query2Sparse` (only 10% match) | 17 µs/pass — cost tracks matches, not world size |
| Raw slice loop (no ECS, upper bound) | 0.76 ns/entity |
| `Get` / `Set` existing | ~9 ns · 0 allocs |
| create + 2×`Set` + `Destroy` | 35 ns · 0 allocs |
| `Events` send + read + flush (1000/frame) | **2.9 ns/event** · 0 allocs |
| `Clean` (100k entities × 5 components) | 63 µs · 0 allocs — **37× a `Destroy` loop** |

At 60 FPS a frame budget is 16.6 ms; a full two-component pass over 100k
entities costs 0.18 ms. Nothing allocates, so the GC stays idle no matter how
long the game runs.

## Edge cases

### The zero Entity is never alive

Live entities have versions starting at 1, and the zero value `Entity` has
version 0. So `var e ecs.Entity` (or the exported `ecs.Nil`) is always dead:
`Alive` returns false, `Get` returns nil. Use `Nil` as a "no target" sentinel
in components that reference other entities.

### Stale handles are safe

```go
e := w.NewEntity()
w.Destroy(e)
e2 := w.NewEntity()      // may reuse e's index
w.Alive(e)               // false — version mismatch
w.Get[Position](e)       // nil, never e2's data
```

Operations on dead entities are harmless no-ops: `Set`, `Remove`, `Destroy`
do nothing; `Get` returns nil; `Has` returns false. You can hold `Entity`
values in components (e.g. `Target ecs.Entity`) across frames and just check
`Alive` before use.

The version counter is 32-bit, so a *single* index would need ~4.3 billion
destroy/recreate cycles before its version wraps and a stale handle could
falsely match. At 60 FPS that is over two years of destroying the same slot
every frame — theoretical, but stated for completeness.

### Resetting the world for a new scene

`Clean` empties the world — every entity, component, and buffered event —
without giving back the memory:

```go
func (g *Game) LoadScene(s Scene) {
    g.world.Clean()
    s.Spawn(g.world)   // rebuilds into arrays that are already the right size
}
```

The systems you built at startup keep working: `Clean` empties the storages in
place, so a cached `*Storage[T]` and any `Query` still point at the right
thing and observe the cleaned world. Nothing needs to be reconstructed.

It is not a `Destroy` loop. `Destroy` probes every storage for one entity, so
tearing a scene down that way costs entities × component types; `Clean` empties
each storage in one shot, leaving a single pass over the version table — 37×
faster on the benchmark above, and it stays flat as you add component types.

Two consequences worth knowing:

- **Every handle taken before the call is dead**, exactly as if each entity had
  been destroyed — including handles stored in components. Don't carry an
  `Entity` across a `Clean`.
- **Buffered events are dropped.** Otherwise a queue could deliver the previous
  scene's events, carrying its now-dead entities, into the scene that replaced
  it. Reader cursors stay valid and resume at the next event sent.

Since the arrays are retained, the second scene load and every one after it
allocates nothing at all.

### Component pointers are invalidated by structural changes

`Get` and query callbacks hand out pointers into the dense slice. They are
valid until the next **structural change** to that component type's storage:

- `Set` on an entity that doesn't yet have the component may grow the slice
  (reallocating it — every old pointer now points at the stale array),
- `Remove`/`Destroy` swap-moves the last element (a surviving component's
  pointer may suddenly point at different data).

Mutating through the pointer immediately (the normal system pattern) is always
fine. **Do not store `*T` across frames or across structural changes** — store
the `Entity` and call `Get` again. `Set` on an entity that *already has* the
component only overwrites in place and invalidates nothing.

### Mutating during iteration

Iteration runs backwards over the dense slice, which gives precise rules:

- **Safe:** destroying the *current* entity, or removing any of the *current*
  entity's components, inside `Each`. The swap-remove pulls an already-visited
  element into the hole, so nothing is skipped or double-visited.
- **Safe:** `Set` of a component type not part of the query, and any `Get`.
- **Avoid:** destroying *other* entities (or removing their queried
  components) mid-iteration. Nothing crashes if each callback removes at most
  the current entity plus one other, but an already-visited entity can be
  swapped into an unvisited slot and be visited twice.
- **Avoid:** adding the queried component to new entities mid-iteration —
  whether the new entity is visited this pass is unspecified.

For bulk changes, collect handles during the pass and apply after:

```go
var doomed []ecs.Entity
q.Each(func(e ecs.Entity, h *Health, _ *Position) {
    if h.HP <= 0 {
        doomed = append(doomed, e)
    }
})
for _, e := range doomed {
    w.Destroy(e)
}
```

### Iteration order is unspecified

Swap-remove reorders the dense slice, so query order depends on the full
history of inserts and removes. It *is* deterministic — the same sequence of
operations always yields the same order, so replays are reproducible — but do
not rely on any particular order. Sort explicitly if rendering needs a stable
draw order (e.g. by a `Layer` component).

### `Set` is upsert

`Set` on an entity that already has the component replaces the value. There
is no separate Add/Replace pair and no "already exists" error.

### Queries can be built before any data exists

`w.Query2[A, B]()` lazily creates empty storages as needed and caches direct
pointers to them. Storages live for the lifetime of the world, so a query
built at startup stays valid forever — it observes all later `Set`s. Build
queries in system constructors; never inside the frame loop (construction
does the reflection map lookups you want to keep out of hot paths).

### Concurrency

A `World` is **not** goroutine-safe. Even read-only calls like `Get` can
mutate internal state (lazy storage creation), and Ebiten calls `Update` and
`Draw` from one goroutine anyway — run systems sequentially.

To parallelize *within* a system, use `EachParallel(workers, fn)`: it
partitions the pass into contiguous ranges, one goroutine per worker.
`workers <= 0` means one per CPU (`runtime.GOMAXPROCS(0)`, re-read every pass
since the runtime may adjust it); `1` runs inline with no goroutines or
`WaitGroup`; and the count is capped so each worker gets at least a few
thousand entities, so small passes always run inline —
`EachParallel(0, fn)` is a safe default. No two workers ever see the same
entity, and the slot index passed to fn is a stable per-entity output slot
(e.g. write vertices at `index*4`). `EachParallel` drives from component A's
storage (not the smallest), iterates forward, and permits **no structural
changes** (`Set` new / `Remove` / `Destroy` / `NewEntity`) anywhere during
the pass. `examples/gopher` uses this for its bounce and vertex-building
passes — a 1M-entity frame drops from 10.6 ms to 1.8 ms on 16 cores.

## Working with large entity counts

**Memory per component type** is `sizeof(T)` per component (dense) plus
4 bytes per component (the `entities` mirror) plus 4 bytes of sparse per
entity *index slot* up to the highest index that ever had this component. Example: 1 000 000 entities that all have a 16-byte `Position`
≈ 16 MB dense + 4 MB entities + 4 MB sparse ≈ 24 MB. The world itself adds
4 bytes per entity (versions).

Practical guidance, in rough order of impact:

1. **Iterate with queries, not `Get`.** A query pass is ~1.8 ns/entity;
   calling `w.Get[T](e)` 100k times costs ~9 ns each *plus* a reflection map
   hit per call. `Get` is for occasional lookups (collisions, UI), not for
   per-frame sweeps.
2. **Cache storages in spawn-heavy code.** `w.Set` pays a reflection map
   lookup per call. A system that creates entities in bursts should grab
   `st := w.Storage[Position]()` once at construction and call
   `st.Set(e, v)` — same semantics, no lookup (~2× faster bursts; see
   `examples/particles`).
3. **Keep components small and split by access pattern.** The movement system
   should stream `Position`/`Velocity` — if those structs also carried sprite
   and AI data, every pass would drag dead weight through the cache. Many
   small components beat one fat one; that is the point of SoA.
4. **Tag components are almost free.** `type Frozen struct{}` has a zero-size
   dense slice; a tag's cost is just the sparse + entities arrays. Use tags
   plus `Query2[Tag, Data]` to carve subsets out of huge populations — the
   query walks the (small) tag storage, so cost tracks the subset.
5. **Sparse memory follows the highest index, not the count.** If only 10
   entities have `Boss` but one of them has index 900 000, `Boss`'s sparse
   slice holds 900 001 entries (~3.6 MB). Index recycling via the free list
   keeps indices compact automatically as long as you destroy what you spawn.
   Spawn long-lived common entities before rare high-index ones if this
   matters.
6. **Component order in `Query2`–`Query5` doesn't affect speed** — the query
   always drives from the smallest storage. Order the type arguments for
   readability.
7. **Pre-warm at load time if you must avoid growth stalls.** Slices grow by
   doubling; a spawn burst of 100k entities triggers a handful of reallocs +
   copies. Spawning and destroying a throwaway population at load time leaves
   the capacity in place (storages keep capacity; only lengths shrink).
8. **Destroy cost scales with component-type count.** `Destroy` probes every
   storage in the world (a few ns each). With dozens of component types and
   mass deaths every frame, prefer a `Dead` tag + one cleanup system so the
   cost is paid once, in one place.

## Files

| File | Contents |
|---|---|
| `entity.go` | `Entity` handle: index + version packing, `Nil` |
| `world.go` | `World`: entity lifecycle, generic `Set`/`Get`/`Has`/`Remove` |
| `storage.go` | `Storage[T]`: the sparse set |
| `query.go` | `Query` through `Query5`: cached, allocation-free iteration; `EachParallel` for multi-core passes |
| `events.go` | `Events[T]` / `EventReader[T]`: double-buffered event queues for system-to-system messaging |
| `ecs_test.go` | correctness tests, including the edge cases above |
| `events_test.go` | event lifetime, ordering, and zero-allocation tests |
| `bench_test.go` | the benchmarks quoted above |

## Examples

| Example | Contents |
|---|---|
| `examples/simple` | a keyboard-controlled box (Ebiten): input, movement, and render systems |
| `examples/particles` | click-to-explode particle bursts (Ebiten): spawn on mouse click, gravity + drag physics, lifetime-driven destruction, and a `Query4` render pass with fading motion streaks |
| `examples/gopher` | bunnymark-style stress test (Ebiten): `-n` gophers bounce off the view edges, hold the mouse to pour in more, FPS/TPS and per-system timings on screen |
| `examples/ui` | a login form (Ebiten) showing the events API: `TextInput` (label, placeholder, password, error) and `Button` (disabled, loading) components; the widget systems publish `TextChanged`/`ButtonClicked` events and a fully decoupled `LoginSystem` polls them |

Run one with `go run ./examples/particles`.
