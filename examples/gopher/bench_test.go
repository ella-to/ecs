package main

import (
	"testing"

	"ella.to/ecs"
)

// BenchmarkFrame200k measures the whole CPU side of a frame with 200 000
// gophers: the bounce pass plus vertex-buffer construction. What's left out
// is only the DrawTriangles submission (13 calls) and the GPU itself. If this
// number sits well under 16.6 ms, 200k at 60 FPS is CPU-feasible.
//
// Run with: go test -bench=. -benchmem ./examples/gopher
func BenchmarkFrame200k(b *testing.B) {
	benchFrame(b, 200_000)
}

func BenchmarkFrame500k(b *testing.B) {
	benchFrame(b, 500_000)
}

func BenchmarkFrame1M(b *testing.B) {
	benchFrame(b, 1_000_000)
}

func benchFrame(b *testing.B, n int) {
	// The sprite image isn't loaded in tests; only its dimensions matter.
	gopherWidth, gopherHeight = 164, 223

	w := ecs.NewWorld()
	spawn := NewSpawnSystem(w)
	bounce := NewBounceSystem(w)
	render := NewRenderSystem(w)
	spawn.Spawn(n, screenWidth/2, screenHeight/2)
	render.buildGeometry() // pre-grow the vertex buffer

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		bounce.Update(dt)
		render.buildGeometry()
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N), "µs/frame")
}
