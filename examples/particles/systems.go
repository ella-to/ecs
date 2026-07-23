package main

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ella.to/ecs"
)

// Each system builds its query once at construction; Update/Draw then run a
// straight loop over packed component data every frame.

// palette holds the base colors an explosion picks from — one per burst, so
// each click reads as a single firework.
var palette = []color.RGBA{
	{R: 0xff, G: 0x6b, B: 0x35, A: 0xff}, // orange
	{R: 0x3c, G: 0xb4, B: 0x64, A: 0xff}, // green
	{R: 0x4d, G: 0x9d, B: 0xe0, A: 0xff}, // blue
	{R: 0xe7, G: 0x4c, B: 0x3c, A: 0xff}, // red
	{R: 0xf7, G: 0xc5, B: 0x4a, A: 0xff}, // gold
	{R: 0xb1, G: 0x6b, B: 0xe0, A: 0xff}, // purple
}

// SpawnSystem turns mouse clicks into particle bursts. It caches the four
// storages at construction, so a burst's 4×N component writes skip the
// per-call storage lookup that World.Set does.
type SpawnSystem struct {
	w          *ecs.World
	positions  *ecs.Storage[Position]
	velocities *ecs.Storage[Velocity]
	lifetimes  *ecs.Storage[Lifetime]
	dots       *ecs.Storage[Dot]
}

func NewSpawnSystem(w *ecs.World) *SpawnSystem {
	return &SpawnSystem{
		w:          w,
		positions:  w.Storage[Position](),
		velocities: w.Storage[Velocity](),
		lifetimes:  w.Storage[Lifetime](),
		dots:       w.Storage[Dot](),
	}
}

func (s *SpawnSystem) Update() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	s.explode(float64(mx), float64(my))
}

func (s *SpawnSystem) explode(x, y float64) {
	base := palette[rand.IntN(len(palette))]
	for range particlesPerBurst {
		angle := rand.Float64() * 2 * math.Pi
		speed := 60 + rand.Float64()*300
		life := 0.6 + rand.Float64()*0.8

		// Jitter brightness per particle so the burst shimmers.
		shade := 0.7 + rand.Float64()*0.3

		e := s.w.NewEntity()
		s.positions.Set(e, Position{X: x, Y: y})
		s.velocities.Set(e, Velocity{X: math.Cos(angle) * speed, Y: math.Sin(angle) * speed})
		s.lifetimes.Set(e, Lifetime{Remaining: life, Total: life})
		s.dots.Set(e, Dot{
			Size: 1.5 + rand.Float64()*2,
			Color: color.RGBA{
				R: uint8(float64(base.R) * shade),
				G: uint8(float64(base.G) * shade),
				B: uint8(float64(base.B) * shade),
				A: 0xff,
			},
		})
	}
}

// PhysicsSystem integrates velocity into position, with gravity and drag.
type PhysicsSystem struct {
	query ecs.Query2[Position, Velocity]
}

func NewPhysicsSystem(w *ecs.World) *PhysicsSystem {
	return &PhysicsSystem{query: w.Query2[Position, Velocity]()}
}

func (s *PhysicsSystem) Update(dt float64) {
	s.query.Each(func(_ ecs.Entity, p *Position, v *Velocity) {
		v.Y += gravity * dt
		v.X -= v.X * drag * dt
		v.Y -= v.Y * drag * dt
		p.X += v.X * dt
		p.Y += v.Y * dt
	})
}

// LifetimeSystem counts particles down and destroys the expired ones.
// Destroying the current entity inside Each is safe (iteration runs
// backwards over the dense slice).
type LifetimeSystem struct {
	w     *ecs.World
	query ecs.Query[Lifetime]
}

func NewLifetimeSystem(w *ecs.World) *LifetimeSystem {
	return &LifetimeSystem{w: w, query: w.Query[Lifetime]()}
}

func (s *LifetimeSystem) Update(dt float64) {
	s.query.Each(func(e ecs.Entity, l *Lifetime) {
		l.Remaining -= dt
		if l.Remaining <= 0 {
			s.w.Destroy(e)
		}
	})
}

// RenderSystem draws every particle as a soft glowing dot stretched along its
// velocity (a motion streak), fading and shrinking as its lifetime runs out.
// It needs four components at once — position, velocity (streak direction),
// lifetime (fade), and dot (look) — which is exactly what Query4 is for.
//
// All particles are batched into reusable vertex/index buffers and rendered
// with a handful of DrawTriangles calls (one per 16k particles) against a
// single soft-circle texture. Per-primitive vector calls (10k+ tessellations
// and draw commands per frame) are what killed the framerate at high particle
// counts — the ECS side of a frame is microseconds.
type RenderSystem struct {
	query    ecs.Query4[Position, Velocity, Lifetime, Dot]
	count    ecs.Query[Dot]
	vertices []ebiten.Vertex
	indices  []uint16      // static pattern: 6 indices per quad, reused every frame
	texture  *ebiten.Image // soft radial dot, created lazily on first Draw
}

// maxQuadsPerDraw keeps each DrawTriangles call within uint16 index range
// (16383 quads = 65532 vertices).
const maxQuadsPerDraw = math.MaxUint16 / 4

// streak is how many seconds of travel the motion tail represents.
const streak = 0.05

// texSize is the particle texture's width and height in pixels.
const texSize = 32

func NewRenderSystem(w *ecs.World) *RenderSystem {
	return &RenderSystem{
		query: w.Query4[Position, Velocity, Lifetime, Dot](),
		count: w.Query[Dot](),
	}
}

// buildGeometry fills s.vertices with one quad per particle. This is the
// entire CPU cost of rendering; Draw just hands the buffers to the GPU.
func (s *RenderSystem) buildGeometry() {
	s.vertices = s.vertices[:0]
	s.query.Each(func(_ ecs.Entity, p *Position, v *Velocity, l *Lifetime, d *Dot) {
		life := l.Remaining / l.Total // 1 at spawn, 0 at death
		size := d.Size * life
		if size <= 0 {
			return
		}

		// The texture is premultiplied white; the vertex color scales it, so
		// fading multiplies every channel (colors stay premultiplied).
		cr := float32(d.Color.R) / 255 * float32(life)
		cg := float32(d.Color.G) / 255 * float32(life)
		cb := float32(d.Color.B) / 255 * float32(life)
		ca := float32(life)

		// Quad running from the tail (position minus a bit of travel) to the
		// head, extended by size at both ends so a slow particle still shows
		// as a round dot, and half a size wide.
		dx, dy := v.X*streak, v.Y*streak
		ux, uy := 1.0, 0.0
		if dlen := math.Hypot(dx, dy); dlen > 1e-3 {
			ux, uy = dx/dlen, dy/dlen
		}
		hx, hy := p.X+ux*size, p.Y+uy*size       // head end
		tx, ty := p.X-dx-ux*size, p.Y-dy-uy*size // tail end
		px, py := -uy*size, ux*size              // half-width, perpendicular

		s.vertices = append(s.vertices,
			ebiten.Vertex{DstX: float32(tx - px), DstY: float32(ty - py), SrcX: 0, SrcY: 0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: float32(tx + px), DstY: float32(ty + py), SrcX: 0, SrcY: texSize, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: float32(hx - px), DstY: float32(hy - py), SrcX: texSize, SrcY: 0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: float32(hx + px), DstY: float32(hy + py), SrcX: texSize, SrcY: texSize, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		)
	})
}

// ensureIndices grows the static index pattern to cover n quads.
func (s *RenderSystem) ensureIndices(n int) {
	for q := len(s.indices) / 6; q < n; q++ {
		i := uint16(q * 4)
		s.indices = append(s.indices, i, i+1, i+2, i+1, i+3, i+2)
	}
}

func (s *RenderSystem) Draw(screen *ebiten.Image) {
	if s.texture == nil {
		s.texture = newParticleTexture()
	}
	s.buildGeometry()

	quads := len(s.vertices) / 4
	s.ensureIndices(min(quads, maxQuadsPerDraw))
	op := &ebiten.DrawTrianglesOptions{Blend: ebiten.BlendLighter} // additive: overlapping particles glow
	for start := 0; start < quads; start += maxQuadsPerDraw {
		n := min(quads-start, maxQuadsPerDraw)
		screen.DrawTriangles(s.vertices[start*4:(start+n)*4], s.indices[:n*6], s.texture, op)
	}
}

// Count returns the number of live particles in O(1), for the debug overlay.
func (s *RenderSystem) Count() int { return s.count.Count() }

// newParticleTexture renders a soft radial falloff dot, premultiplied white.
func newParticleTexture() *ebiten.Image {
	img := ebiten.NewImage(texSize, texSize)
	pix := make([]byte, 4*texSize*texSize)
	const half = texSize / 2
	for y := range texSize {
		for x := range texSize {
			dx := (float64(x) + 0.5 - half) / half
			dy := (float64(y) + 0.5 - half) / half
			a := 1 - math.Hypot(dx, dy)
			if a < 0 {
				a = 0
			}
			v := byte(255 * a * a) // quadratic falloff: bright core, soft glow
			i := 4 * (y*texSize + x)
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, v
		}
	}
	img.WritePixels(pix)
	return img
}
