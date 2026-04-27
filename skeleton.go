package main

import (
	"image/color"
	"math"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// MakeSkeleton returns a shimmering placeholder and a stop function.
// isImage controls shape (square vs text lines). estChars sizes the text lines.
func MakeSkeleton(isImage bool, estChars int) (fyne.CanvasObject, func()) {
	var stop atomic.Bool

	if isImage {
		rect := canvas.NewRectangle(color.NRGBA{50, 50, 55, 255})
		rect.SetMinSize(fyne.NewSize(350, 280))
		go shimmerRects(&stop, []*canvas.Rectangle{rect})
		return container.NewCenter(rect), func() { stop.Store(true) }
	}

	numLines := clamp(estChars/80+1, 4, 12)
	widths := []float32{0.95, 0.82, 0.88, 0.70, 0.93, 0.60, 0.85, 0.75, 0.90, 0.65, 0.80, 0.55}

	bars := make([]*canvas.Rectangle, numLines)
	var items []fyne.CanvasObject
	for i := range numLines {
		bar := canvas.NewRectangle(color.NRGBA{50, 50, 55, 255})
		bar.SetMinSize(fyne.NewSize(660*widths[i%len(widths)], 14))
		bars[i] = bar
		items = append(items, bar)
		sp := canvas.NewRectangle(color.Transparent)
		sp.SetMinSize(fyne.NewSize(1, 6))
		items = append(items, sp)
	}

	go shimmerRects(&stop, bars)
	return container.NewVBox(items...), func() { stop.Store(true) }
}

func shimmerRects(stop *atomic.Bool, bars []*canvas.Rectangle) {
	t := 0.0
	for !stop.Load() {
		t += 0.10
		for i, bar := range bars {
			phase := t - float64(i)*0.20
			v := uint8(40 + 20*math.Sin(phase))
			bar.FillColor = color.NRGBA{v, v, v + 5, 255}
			bar.Refresh()
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
