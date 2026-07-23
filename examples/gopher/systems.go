package main

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ella.to/ecs"
)

// Each system builds its query once at construction; Update/Draw then run a
// straight loop over packed component data every frame.
//
// The two per-frame passes (bounce, geometry) are embarrassingly parallel, so
// they run through EachParallel(0, ...) — one worker per CPU, dropping to an
// inline serial pass automatically while the population is small. Structural
// changes (spawning) happen strictly outside these passes.

// SpawnSystem adds gophers: n at startup, then a batch per tick while the
// mouse button is held. Storages are cached so spawning skips the per-call
// storage lookup that World.Set does.
type SpawnSystem struct {
	w          *ecs.World
	positions  *ecs.Storage[Position]
	velocities *ecs.Storage[Velocity]
	sprites    *ecs.Storage[Sprite]
}

func NewSpawnSystem(w *ecs.World) *SpawnSystem {
	return &SpawnSystem{
		w:          w,
		positions:  w.Storage[Position](),
		velocities: w.Storage[Velocity](),
		sprites:    w.Storage[Sprite](),
	}
}

func (s *SpawnSystem) Update() {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		s.Spawn(spawnPerTick, float64(mx), float64(my))
	}
}

// Spawn adds n gophers at (x, y) with random direction, speed, and size.
func (s *SpawnSystem) Spawn(n int, x, y float64) {
	for range n {
		speed := 40 + rand.Float64()*260
		dirX, dirY := rand.Float64()*2-1, rand.Float64()*2-1
		scale := 0.08 + rand.Float64()*0.12

		e := s.w.NewEntity()
		s.positions.Set(e, Position{X: x, Y: y})
		s.velocities.Set(e, Velocity{X: dirX * speed, Y: dirY * speed})
		s.sprites.Set(e, Sprite{W: gopherWidth * scale, H: gopherHeight * scale})
	}
}

// BounceSystem integrates velocity into position and reflects gophers off the
// view edges.
type BounceSystem struct {
	query ecs.Query3[Position, Velocity, Sprite]
}

func NewBounceSystem(w *ecs.World) *BounceSystem {
	return &BounceSystem{query: w.Query3[Position, Velocity, Sprite]()}
}

func (s *BounceSystem) Update(dt float64) {
	s.query.EachParallel(0, func(_ int, _ ecs.Entity, p *Position, v *Velocity, sp *Sprite) {
		bounce(dt, p, v, sp)
	})
}

func bounce(dt float64, p *Position, v *Velocity, sp *Sprite) {
	p.X += v.X * dt
	p.Y += v.Y * dt

	if p.X < 0 {
		p.X, v.X = -p.X, -v.X
	} else if edge := screenWidth - sp.W; p.X > edge {
		p.X, v.X = 2*edge-p.X, -v.X
	}
	if p.Y < 0 {
		p.Y, v.Y = -p.Y, -v.Y
	} else if edge := screenHeight - sp.H; p.Y > edge {
		p.Y, v.Y = 2*edge-p.Y, -v.Y
	}
}

// RenderSystem batches every gopher into one reused vertex buffer and submits
// it as a single DrawTriangles32 call sampling the one shared image.
// Per-entity DrawImage calls cost ~150 ns of CPU each — at 74k gophers that
// alone blows the 16.6 ms frame budget, which is what capped the naive
// version at ~50 FPS. Building the four vertices ourselves costs a few ns per
// gopher, and the build parallelizes: EachParallel's slot index gives each
// gopher a fixed vertex slot, so workers write disjoint ranges of one buffer.
type RenderSystem struct {
	query    ecs.Query2[Position, Sprite]
	vertices []ebiten.Vertex
	indices  []uint32 // static pattern: 6 indices per quad, reused every frame

	geomDur time.Duration // last frame's buildGeometry time, for the overlay
}

func NewRenderSystem(w *ecs.World) *RenderSystem {
	return &RenderSystem{query: w.Query2[Position, Sprite]()}
}

// buildGeometry fills s.vertices with one axis-aligned quad per gopher. This
// is the entire CPU cost of rendering; Draw just hands the buffers to the GPU.
func (s *RenderSystem) buildGeometry() {
	n := s.query.Len()
	// Sized to n quads and written by slot index: every gopher has both
	// components, so every slot is filled each frame.
	s.vertices = slices.Grow(s.vertices[:0], 4*n)[: 4*n : 4*n]
	s.query.EachParallel(0, s.emit)
}

// emit writes gopher i's quad into its four fixed vertex slots.
func (s *RenderSystem) emit(i int, _ ecs.Entity, p *Position, sp *Sprite) {
	x0, y0 := float32(p.X), float32(p.Y)
	x1, y1 := x0+float32(sp.W), y0+float32(sp.H)
	sw, sh := float32(gopherWidth), float32(gopherHeight)
	v := s.vertices[4*i : 4*i+4 : 4*i+4]
	v[0] = ebiten.Vertex{DstX: x0, DstY: y0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[1] = ebiten.Vertex{DstX: x1, DstY: y0, SrcX: sw, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[2] = ebiten.Vertex{DstX: x0, DstY: y1, SrcX: 0, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	v[3] = ebiten.Vertex{DstX: x1, DstY: y1, SrcX: sw, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
}

// ensureIndices grows the static index pattern to cover n quads.
func (s *RenderSystem) ensureIndices(n int) {
	for q := len(s.indices) / 6; q < n; q++ {
		i := uint32(q * 4)
		s.indices = append(s.indices, i, i+1, i+2, i+1, i+3, i+2)
	}
}

func (s *RenderSystem) Draw(screen *ebiten.Image) {
	start := time.Now()
	s.buildGeometry()
	s.geomDur = time.Since(start)

	quads := len(s.vertices) / 4
	s.ensureIndices(quads)
	op := &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear}
	screen.DrawTriangles32(s.vertices, s.indices[:quads*6], gopherImage, op)
}

// Count returns the number of live gophers in O(1).
func (s *RenderSystem) Count() int { return s.query.Len() }
