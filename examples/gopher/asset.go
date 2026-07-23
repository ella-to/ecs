package main

import (
	"image"
	"image/color"
)

// keyOutBackground makes the sprite's background transparent. The source PNG
// is RGB with a white backdrop, so it flood-fills near-white pixels connected
// to the image border and clears them — interior whites (the gopher's eyes)
// are untouched because they aren't reachable from the edge.
func keyOutBackground(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			dst.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}

	isBackground := func(x, y int) bool {
		c := dst.NRGBAAt(x, y)
		return c.A != 0 && c.R > 0xe8 && c.G > 0xe8 && c.B > 0xe8
	}

	// BFS from every near-white border pixel.
	var queue []image.Point
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h || !isBackground(x, y) {
			return
		}
		dst.SetNRGBA(x, y, color.NRGBA{})
		queue = append(queue, image.Point{X: x, Y: y})
	}
	for x := range w {
		push(x, 0)
		push(x, h-1)
	}
	for y := range h {
		push(0, y)
		push(w-1, y)
	}
	for len(queue) > 0 {
		p := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		push(p.X-1, p.Y)
		push(p.X+1, p.Y)
		push(p.X, p.Y-1)
		push(p.X, p.Y+1)
	}
	return dst
}
