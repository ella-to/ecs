package main

import (
	"testing"

	"ella.to/ecs"
)

// These benchmarks reproduce the "5000 live particles" scenario and measure
// every part of a frame except the actual GPU submission, to attribute the
// framerate drop: is it the ECS (spawn/iterate/destroy) or the rendering?
//
// Run with: go test -bench=. -benchmem ./examples/particles

const benchParticles = 5000

// spawnStatic fills the world with n particles that never expire, for
// benchmarks that need a stable population.
func spawnStatic(w *ecs.World, n int) {
	for i := range n {
		e := w.NewEntity()
		w.Set(e, Position{X: float64(i % 640), Y: float64(i % 480)})
		w.Set(e, Velocity{X: 30, Y: -40})
		w.Set(e, Lifetime{Remaining: 1e9, Total: 2e9}) // half life: exercises the fade math
		w.Set(e, Dot{Size: 2})
	}
}

// BenchmarkFrameSteadyState is one whole game frame at ~5000 live particles
// with realistic churn: particles age and die every tick, and a fresh burst
// "click" lands whenever the population dips below the target — exactly the
// workload that showed the framerate drop.
func BenchmarkFrameSteadyState(b *testing.B) {
	w := ecs.NewWorld()
	spawn := NewSpawnSystem(w)
	physics := NewPhysicsSystem(w)
	lifetime := NewLifetimeSystem(w)
	render := NewRenderSystem(w)

	for render.Count() < benchParticles {
		spawn.explode(320, 240)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if render.Count() < benchParticles {
			spawn.explode(320, 240)
		}
		physics.Update(dt)
		lifetime.Update(dt)
		render.buildGeometry() // the CPU side of Draw
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/frame")
}

// BenchmarkPhysicsPass isolates the Query2 integration pass.
func BenchmarkPhysicsPass(b *testing.B) {
	w := ecs.NewWorld()
	spawnStatic(w, benchParticles)
	physics := NewPhysicsSystem(w)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		physics.Update(dt)
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/frame")
}

// BenchmarkLifetimePass isolates the Query pass over Lifetime (no deaths, so
// it measures pure iteration, not Destroy).
func BenchmarkLifetimePass(b *testing.B) {
	w := ecs.NewWorld()
	spawnStatic(w, benchParticles)
	lifetime := NewLifetimeSystem(w)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lifetime.Update(dt)
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/frame")
}

// BenchmarkRenderGeometry isolates the Query4 pass plus per-particle quad
// math — everything Draw does except handing buffers to the GPU.
func BenchmarkRenderGeometry(b *testing.B) {
	w := ecs.NewWorld()
	spawnStatic(w, benchParticles)
	render := NewRenderSystem(w)
	render.buildGeometry() // pre-grow the vertex buffer

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		render.buildGeometry()
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/frame")
}

// BenchmarkBurstSpawnAndDestroy is the churn cost in isolation: spawn one
// full click's burst (4 Sets per particle), then destroy every particle.
// This is the path that hits the World's storage bookkeeping hardest.
func BenchmarkBurstSpawnAndDestroy(b *testing.B) {
	w := ecs.NewWorld()
	spawn := NewSpawnSystem(w)
	doomed := w.Query[Dot]()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		spawn.explode(320, 240)
		doomed.Each(func(e ecs.Entity, _ *Dot) {
			w.Destroy(e)
		})
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/burst")
}
