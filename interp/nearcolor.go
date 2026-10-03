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

// NearestColor is the palette index nearest to the 24-bit color r, g, b on a
// terminal with the given number of colors, and false on one that has
// neither of the two palettes the mapping knows.
//
// Measured 2026-10-02 against the one shell that does this, through
// `%F{#rrggbb}` with its module loaded and `$TERM` naming each count the
// terminfo database holds: 256 maps onto the 256-colour palette above;
// 88 onto the 88-colour one — 202 triplets, all matched by the same L*a*b*
// distance over palette88RGB; and 8, 16, 52, 64 and 16,777,216 map onto
// nothing, the color written as the channel's default instead.
func NearestColor(colors, r, g, b int) (int, bool) {
	switch colors {
	case 256:
		return NearestPaletteColor(r, g, b), true
	case 88:
		want := colorLab(r, g, b)
		best, bestDistance := 16, math.Inf(1)
		palette := palette88Lab()
		for i := 16; i < 88; i++ {
			d := 0.0
			for k := range want {
				d += (want[k] - palette[i][k]) * (want[k] - palette[i][k])
			}
			if d < bestDistance {
				best, bestDistance = i, d
			}
		}
		return best, true
	}
	return 0, false
}

// palette88Lab is the 88-colour palette from 16 up in L*a*b*, computed once.
var palette88Lab = sync.OnceValue(func() (lab [88][3]float64) {
	for i := 16; i < 88; i++ {
		lab[i] = colorLab(palette88RGB(i))
	}
	return lab
})

// palette88RGB is 88-colour palette entry i, for i from 16 to 87: a 4×4×4
// cube over the levels 0, 139, 205 and 255, then eight grays.
//
// The grays are the half the measurement fits rather than shows. The 202
// triplets chose 81, 82, 84, 85, 86 and 87 at 46, 92, 162, 185, 208 and 231,
// and never chose 80 or 83: every gray between two of those went to one of
// them or to a cube color. So 80 and 83 are written as ties with an earlier
// index — black with 16, and 139 with the cube's 37 — which no sample can
// tell from any other value that never wins, and which is stated here so
// that nobody reads it as measured.
func palette88RGB(i int) (r, g, b int) {
	if i >= 80 {
		v := [8]int{0, 46, 92, 139, 162, 185, 208, 231}[i-80]
		return v, v, v
	}
	levels := [4]int{0, 139, 205, 255}
	n := i - 16
	return levels[n/16], levels[n/4%4], levels[n%4]
}

// SetNearestColors installs what says whether this runner writes a 24-bit
// color as its nearest palette color, and on how many colors: zero is not
// at all, anything else the terminal's color count, which NearestColor turns
// into a palette or into none. Asked with the runner it answers for, because
// the answer is the shell's state and a subshell keeps its own.
func (r *Runner) SetNearestColors(near func(r *Runner) int) { r.nearestColors = near }

// NearestColors is the count SetNearestColors answers, zero where nothing was
// installed.
func (r *Runner) NearestColors() int {
	if r.nearestColors == nil {
		return 0
	}
	return r.nearestColors(r)
}
