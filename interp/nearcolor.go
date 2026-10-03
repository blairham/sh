// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"sync"
)

// The nearest of the terminal's 256 palette colors to a 24-bit one.
//
// A dialect can have the shell write every color given as a hex triplet as
// the nearest palette color instead of as a 24-bit one — see
// Runner.SetNearestColors. The first sixteen palette colors are never chosen,
// because what a terminal shows for them varies. The distance is squared
// distance in CIE L*a*b* under D65, the lowest index winning a tie: measured
// over 202 triplets against the one shell in the panel that does this, where
// squared distance in RGB matched 133 and in YUV 145, and L*a*b* matched all
// 202; the dialect that does it holds the rows in its tests.
//
// The palette is the terminal's arithmetic and not the shell's: colors 16 to
// 231 are the 6×6×6 cube over the levels 0, 95, 135, 175, 215 and 255, and
// 232 to 255 the gray ramp from 8 in steps of 10.

// NearestPaletteColor is the palette index, 16 to 255, nearest to the 24-bit
// color r, g, b.
func NearestPaletteColor(r, g, b int) int {
	want := colorLab(r, g, b)
	best, bestDistance := 16, math.Inf(1)
	palette := paletteLab()
	for i := 16; i < 256; i++ {
		have := palette[i]
		d := 0.0
		for k := range want {
			d += (want[k] - have[k]) * (want[k] - have[k])
		}
		if d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return best
}

// paletteLab is every palette color from 16 up in L*a*b*, computed once.
var paletteLab = sync.OnceValue(func() (lab [256][3]float64) {
	for i := 16; i < 256; i++ {
		lab[i] = colorLab(paletteRGB(i))
	}
	return lab
})

// paletteRGB is palette color i, for i from 16 to 255.
func paletteRGB(i int) (r, g, b int) {
	if i >= 232 {
		v := 8 + 10*(i-232)
		return v, v, v
	}
	levels := [6]int{0, 95, 135, 175, 215, 255}
	n := i - 16
	return levels[n/36], levels[n/6%6], levels[n%6]
}

// colorLab is an sRGB color in CIE L*a*b*, D65.
func colorLab(r, g, b int) [3]float64 {
	linear := func(c int) float64 {
		u := float64(c) / 255
		if u > 0.04045 {
			return math.Pow((u+0.055)/1.055, 2.4)
		}
		return u / 12.92
	}
	lr, lg, lb := linear(r), linear(g), linear(b)
	x := (lr*0.4124 + lg*0.3576 + lb*0.1805) / 0.95047
	y := lr*0.2126 + lg*0.7152 + lb*0.0722
	z := (lr*0.0193 + lg*0.1192 + lb*0.9505) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// SetNearestColors installs what says whether this runner writes a 24-bit
// color as its nearest palette color — the prompt's `%F{#rrggbb}` and every
// other place the shell draws a hex triplet. Asked with the runner it answers
// for, because the answer is the shell's state and a subshell keeps its own.
func (r *Runner) SetNearestColors(near func(r *Runner) bool) { r.nearestColors = near }

// NearestColors reports whether this runner writes a 24-bit color as its
// nearest palette color. See SetNearestColors.
func (r *Runner) NearestColors() bool { return r.nearestColors != nil && r.nearestColors(r) }
