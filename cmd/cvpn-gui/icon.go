package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/inoisy/cisco-vless/internal/daemon"
)

type state int

const (
	stateUnknown state = iota // служба недоступна
	stateOff                  // всё выключено
	stateExit                 // только выход
	stateCisco                // только cisco
	stateBoth                 // cisco + выход
)

func iconState(s *daemon.Status) state {
	switch {
	case s == nil:
		return stateUnknown
	case s.Cisco && s.Exit != "":
		return stateBoth
	case s.Cisco:
		return stateCisco
	case s.Exit != "":
		return stateExit
	}
	return stateOff
}

var iconCache = map[state][]byte{}

// trayIcon — кольцо, разрезанное на две половины: левая — cisco, правая —
// выход. Цвет половины — включено ли. Рисуем сами, чтобы не возить PNG.
func trayIcon(st state) []byte {
	if b, ok := iconCache[st]; ok {
		return b
	}
	const size = 64
	var (
		off   = color.NRGBA{0x8e, 0x8e, 0x93, 0xff}
		cisco = color.NRGBA{0x30, 0xa4, 0x6c, 0xff}
		exit  = color.NRGBA{0x2f, 0x7c, 0xf6, 0xff}
		dead  = color.NRGBA{0xd9, 0x4b, 0x3d, 0xff}
	)
	left, right := off, off
	switch st {
	case stateUnknown:
		left, right = dead, dead
	case stateExit:
		right = exit
	case stateCisco:
		left = cisco
	case stateBoth:
		left, right = cisco, exit
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size-1) / 2
	outer, inner := 30.0, 19.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			d := math.Hypot(dx, dy)
			// Сглаживание краёв кольца.
			a := clamp(outer+0.5-d) * clamp(d-inner+0.5)
			if math.Abs(dx) < 2.5 { // зазор между половинами
				a *= clamp(math.Abs(dx) - 1.5)
			}
			if a <= 0 {
				continue
			}
			col := right
			if dx < 0 {
				col = left
			}
			col.A = uint8(float64(col.A) * a)
			img.SetNRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	iconCache[st] = buf.Bytes()
	return iconCache[st]
}

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }
