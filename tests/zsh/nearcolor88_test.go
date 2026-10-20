// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The 88-colour palette index zsh 5.9.2 wrote for each of 202 hex triplets
// under TERM=xterm-88color with `zsh/nearcolor` loaded, measured 2026-10-02
// through `print -P '%F{#rrggbb}'` — 150 at random and the grays from #000000
// in steps of five (#5496).
func TestTheNearest88ColorIsTheMeasuredOne(t *testing.T) {
	for _, c := range []struct {
		rgb  int
		want int
	}{
		{0x82c9b0, 42},
		{0xb791f7, 55},
		{0x0ed9c5, 26},
		{0xee6617, 68},
		{0x7f83d4, 38},
		{0x1a8c82, 21},
		{0x504ed1, 39},
		{0x39f621, 28},
		{0xbe5bb2, 54},
		{0xf029d1, 66},
		{0x7e3ecb, 34},
		{0xc2f2b3, 62},
		{0x3435fd, 18},
		{0x7fa846, 41},
		{0x06b6e7, 27},
		{0x6ef735, 44},
		{0xd0f92c, 60},
		{0x8f1850, 49},
		{0x5d3905, 81},
		{0xc76453, 69},
		{0x51b7a4, 26},
		{0x24d43c, 24},
		{0x470c85, 17},
		{0xe3c180, 73},
		{0x40deb8, 46},
		{0x43b512, 24},
		{0x00e8a2, 25},
		{0x02b867, 25},
		{0x6b3ddd, 34},
		{0x6e538c, 38},
		{0x54eb22, 24},
		{0x553d74, 38},
		{0x941fce, 34},
		{0xa096e9, 38},
		{0x65d4d2, 26},
		{0x68d8dc, 42},
		{0x5d03ac, 34},
		{0x64cd54, 45},
		{0xc435de, 50},
		{0x98fadb, 46},
		{0x0b0b40, 16},
		{0xb8ede3, 63},
		{0xd46ea7, 70},
		{0x54f831, 28},
		{0x4a9970, 41},
		{0x87125b, 49},
		{0x2159fe, 18},
		{0xa9e8d4, 42},
		{0x9a4bbd, 33},
		{0x01bb6e, 25},
		{0xad00d1, 34},
		{0x21cdb1, 26},
		{0x9eb2ce, 85},
		{0xb5f09c, 41},
		{0x9cb474, 57},
		{0xf6243b, 48},
		{0xa1afa2, 84},
		{0x5e9a99, 21},
		{0xf66241, 48},
		{0xf1f08b, 77},
		{0x5a2c69, 33},
		{0x1d2f39, 81},
		{0x832a43, 53},
		{0x0bb508, 24},
		{0xb71d1f, 32},
		{0xceffa2, 61},
		{0x094025, 81},
		{0xd67086, 69},
		{0xbb7c88, 53},
		{0xc0b2f7, 59},
		{0x04a5ec, 22},
		{0xe7d851, 56},
		{0x17ec9d, 25},
		{0x5ca17a, 41},
		{0x649e7d, 21},
		{0x3cf043, 24},
		{0x7dff16, 44},
		{0xeca62e, 52},
		{0xb052d0, 55},
		{0xb5a774, 57},
		{0x80752c, 36},
		{0xecfaa4, 77},
		{0x3752b4, 23},
		{0xbc0e61, 49},
		{0x977b76, 37},
		{0x12c15f, 25},
		{0xddaf1e, 52},
		{0x2eaf20, 20},
		{0x6abe55, 41},
		{0xae758b, 53},
		{0xb9aa0c, 36},
		{0x4be04c, 24},
		{0xae0f2c, 32},
		{0x8d2574, 33},
		{0x2f243c, 81},
		{0x9ff97e, 45},
		{0xa2339b, 33},
		{0x9cc274, 41},
		{0x5ade5d, 45},
		{0x280956, 17},
		{0x4c386e, 38},
		{0x9e5d09, 52},
		{0xf7b3d1, 75},
		{0x52b3fd, 22},
		{0x18dbd4, 26},
		{0x29735b, 21},
		{0xcfd08e, 57},
		{0x104b8f, 22},
		{0x79842e, 36},
		{0xb00add, 34},
		{0x801b60, 33},
		{0xe94cef, 50},
		{0xd7f092, 61},
		{0x4a8fe3, 22},
		{0x1c85a8, 22},
		{0x10c0d5, 26},
		{0xfcb472, 73},
		{0xab1a55, 49},
		{0x6a03c1, 34},
		{0x42da4f, 24},
		{0x43fb58, 24},
		{0xd3e088, 57},
		{0x369849, 20},
		{0x566fb0, 38},
		{0xde7e04, 68},
		{0xbef98b, 61},
		{0x4c69e2, 39},
		{0x1e172e, 81},
		{0xd75ee3, 71},
		{0x96f1b8, 46},
		{0x484bae, 39},
		{0xe803eb, 67},
		{0x56ad2a, 20},
		{0xe83664, 65},
		{0xf9eff9, 87},
		{0xa2a9de, 38},
		{0xf5450d, 48},
		{0x8c4e62, 53},
		{0x9500ea, 35},
		{0xf0ae6f, 73},
		{0xcebd7c, 57},
		{0x4b02e1, 18},
		{0x39a182, 21},
		{0xc1063d, 32},
		{0x5bd9d2, 26},
		{0xffa731, 52},
		{0xad4d16, 32},
		{0x5c38ba, 17},
		{0x2da36b, 25},
		{0xfbd556, 72},
		{0x000000, 16},
		{0x050505, 16},
		{0x0a0a0a, 16},
		{0x0f0f0f, 16},
		{0x141414, 16},
		{0x191919, 16},
		{0x1e1e1e, 81},
		{0x232323, 81},
		{0x282828, 81},
		{0x2d2d2d, 81},
		{0x323232, 81},
		{0x373737, 81},
		{0x3c3c3c, 81},
		{0x414141, 81},
		{0x464646, 82},
		{0x4b4b4b, 82},
		{0x505050, 82},
		{0x555555, 82},
		{0x5a5a5a, 82},
		{0x5f5f5f, 82},
		{0x646464, 82},
		{0x696969, 82},
		{0x6e6e6e, 82},
		{0x737373, 82},
		{0x787878, 37},
		{0x7d7d7d, 37},
		{0x828282, 37},
		{0x878787, 37},
		{0x8c8c8c, 37},
		{0x919191, 37},
		{0x969696, 37},
		{0x9b9b9b, 84},
		{0xa0a0a0, 84},
		{0xa5a5a5, 84},
		{0xaaaaaa, 84},
		{0xafafaf, 85},
		{0xb4b4b4, 85},
		{0xb9b9b9, 85},
		{0xbebebe, 85},
		{0xc3c3c3, 58},
		{0xc8c8c8, 58},
		{0xcdcdcd, 58},
		{0xd2d2d2, 86},
		{0xd7d7d7, 86},
		{0xdcdcdc, 87},
		{0xe1e1e1, 87},
		{0xe6e6e6, 87},
		{0xebebeb, 87},
		{0xf0f0f0, 87},
		{0xf5f5f5, 79},
		{0xfafafa, 79},
		{0xffffff, 79},
	} {
		got, ok := interp.NearestColor(88, c.rgb>>16, c.rgb>>8&0xff, c.rgb&0xff)
		if !ok || got != c.want {
			t.Errorf("#%06x: got %d (%v), want %d", c.rgb, got, ok, c.want)
		}
	}
}

// Which palette a triplet lands in follows the terminal's own count of
// colors: 256 and 88 have one, and every other count — measured 8, 16, 52,
// 64 and 16,777,216 — has none, so the color is the channel's default and a
// `region_highlight` element loses it. zsh 5.9.2, 2026-10-02.
func TestNearcolorFollowsTheTerminalsColorCount(t *testing.T) {
	for _, c := range []struct {
		colors int
		want   string
	}{
		{256, "\x1b[38;5;115m\x1b[48;5;24m|0 1 fg=115\n"},
		{88, "\x1b[38;5;42m\x1b[48;5;81m|0 1 fg=42\n"},
		{16, "\x1b[39m\x1b[49m|0 1 none\n"},
		{16777216, "\x1b[39m\x1b[49m|0 1 none\n"},
	} {
		dir := colorsFixture(t, c.colors)
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir(), Vars: colorsVars(t, dir)}, `zmodload zsh/nearcolor; print -rn -P '%F{#82c9b0}%K{#123456}'; print -rn '|'`)
		if err != nil {
			t.Fatal(err)
		}
		r, buf := zleRunner(t, "zmodload zsh/nearcolor\nw() { region_highlight=('0 1 fg=#82c9b0'); print -r -- $region_highlight; }\nzle -N w")
		for k, v := range colorsVars(t, dir) {
			r.SetVar(k, v)
		}
		_, _, said := runWidget(t, r, buf, "w", repl.Line{Buffer: "ab"})
		if got := out + said; got != c.want {
			t.Errorf("colors#%d: got %q, want %q", c.colors, got, c.want)
		}
	}
}
