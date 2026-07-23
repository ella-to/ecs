package ecs

import "testing"

const benchEntities = 100_000

func newBenchWorld(withVel float64) *World {
	w := NewWorld()
	for i := range benchEntities {
		e := w.NewEntity()
		w.Set(e, pos{X: float64(i)})
		if float64(i) < withVel*benchEntities {
			w.Set(e, vel{X: 1, Y: 1})
		}
	}
	return w
}

// BenchmarkQuery2 is the hot path of a real game: one full movement-system
// pass over 100k entities that all have both components.
func BenchmarkQuery2(b *testing.B) {
	w := newBenchWorld(1.0)
	q := w.Query2[pos, vel]()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		q.Each(func(_ Entity, p *pos, v *vel) {
			p.X += v.X
			p.Y += v.Y
		})
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/benchEntities, "ns/entity")
}

// BenchmarkQuery2Sparse: 100k entities but only 10% have vel. Each drives the
// iteration from the smaller storage, so cost tracks the 10k matches, not the
// 100k total.
func BenchmarkQuery2Sparse(b *testing.B) {
	w := newBenchWorld(0.1)
	q := w.Query2[pos, vel]()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		q.Each(func(_ Entity, p *pos, v *vel) {
			p.X += v.X
		})
	}
}

// BenchmarkQuery3: 100k entities that all have pos, vel, and acc — one full
// physics-style pass (acceleration into velocity into position).
func BenchmarkQuery3(b *testing.B) {
	w := newBenchWorld(1.0)
	w.Query[pos]().Each(func(e Entity, _ *pos) {
		w.Set(e, acc{X: 0.1, Y: 0.1})
	})
	q := w.Query3[pos, vel, acc]()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		q.Each(func(_ Entity, p *pos, v *vel, a *acc) {
			v.X += a.X
			v.Y += a.Y
			p.X += v.X
			p.Y += v.Y
		})
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/benchEntities, "ns/entity")
}

// BenchmarkRawSlice is the theoretical ceiling: the same work as
// BenchmarkQuery2 over a plain slice with no ECS at all.
func BenchmarkRawSlice(b *testing.B) {
	type posvel struct {
		p pos
		v vel
	}
	data := make([]posvel, benchEntities)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for i := range data {
			data[i].p.X += data[i].v.X
			data[i].p.Y += data[i].v.Y
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/benchEntities, "ns/entity")
}

func BenchmarkGet(b *testing.B) {
	w := newBenchWorld(1.0)
	e := w.NewEntity()
	w.Set(e, pos{X: 7})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if w.Get[pos](e) == nil {
			b.Fatal("missing component")
		}
	}
}

func BenchmarkSetExisting(b *testing.B) {
	w := NewWorld()
	e := w.NewEntity()
	w.Set(e, pos{})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w.Set(e, pos{X: 1})
	}
}

func BenchmarkCreateDestroy(b *testing.B) {
	w := NewWorld()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		e := w.NewEntity()
		w.Set(e, pos{})
		w.Set(e, vel{})
		w.Destroy(e)
	}
}

// BenchmarkEvents is one frame of steady-state event traffic: a producer
// sends 1000 events, a consumer reads them, and the frame ends with
// FlushEvents. Buffer capacity is reused across frames, so nothing
// allocates.
func BenchmarkEvents(b *testing.B) {
	const eventsPerFrame = 1000
	w := NewWorld()
	ev := w.Events[clicked]()
	r := ev.Reader()
	b.ReportAllocs()
	b.ResetTimer()
	sum := 0
	for b.Loop() {
		for i := range eventsPerFrame {
			ev.Send(clicked{N: i})
		}
		r.Each(func(e clicked) { sum += e.N })
		w.FlushEvents()
	}
	_ = sum
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/eventsPerFrame, "ns/event")
}
