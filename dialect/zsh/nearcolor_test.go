// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The palette index zsh 5.9.2 wrote for each of 202 hex triplets with
// `zsh/nearcolor` loaded, measured 2026-10-02 through a pseudo-terminal — 150
// at random and the grays from #000000 in steps of five. See interp/nearcolor.go,
// which says which distance these settle.
func TestTheNearestPaletteColorIsTheMeasuredOne(t *testing.T) {
	for _, c := range []struct {
		rgb  int
		want int
	}{
		{0x79d67f, 114},
		{0x42c6c6, 80},
		{0xbd6ac3, 133},
		{0xf2b725, 178},
		{0x218cff, 33},
		{0x06bdf4, 38},
		{0xf03f38, 203},
		{0x84ca0c, 112},
		{0x77fa3a, 118},
		{0x622c48, 96},
		{0xf0c660, 221},
		{0xf3e491, 186},
		{0xcb5539, 167},
		{0x4d1d98, 54},
		{0x76be7b, 71},
		{0x4da172, 72},
		{0xc7a5c9, 182},
		{0x07c150, 41},
		{0x20c8ba, 44},
		{0x519cde, 75},
		{0x15e871, 41},
		{0x9a3fc1, 134},
		{0x0fe0c5, 43},
		{0x89f2f2, 123},
		{0xf20c2b, 160},
		{0xc674a1, 175},
		{0xda9735, 172},
		{0xca38a4, 126},
		{0xe3a55e, 179},
		{0x44af31, 70},
		{0xbb2564, 125},
		{0x31e588, 42},
		{0x125fbb, 25},
		{0x459db5, 74},
		{0xfd615b, 203},
		{0x6f18e9, 56},
		{0x84161d, 52},
		{0xdf509b, 205},
		{0x9a201b, 88},
		{0xd7a0c7, 182},
		{0xc59043, 179},
		{0xb3aa8a, 144},
		{0xd0adc8, 182},
		{0x76fb5c, 119},
		{0xac6c76, 131},
		{0x0eac92, 36},
		{0x8f32f6, 92},
		{0x5381cb, 68},
		{0xa71ca0, 90},
		{0x35496c, 60},
		{0x6c1892, 54},
		{0x88bf5a, 107},
		{0x91e5f1, 116},
		{0x3fb5ec, 74},
		{0x207de7, 33},
		{0xf6c8e3, 225},
		{0xf78e75, 209},
		{0x2d523e, 23},
		{0xb02b47, 167},
		{0x221af8, 21},
		{0xd2280d, 160},
		{0x4d311e, 237},
		{0x0a4dde, 27},
		{0x9678e9, 141},
		{0xdab253, 179},
		{0xd494bd, 175},
		{0x3ce2b8, 43},
		{0x16a014, 34},
		{0x170268, 17},
		{0xc16fa0, 132},
		{0xa972d2, 140},
		{0x8ee2b8, 115},
		{0x78cea4, 115},
		{0x127046, 29},
		{0x9e8d12, 100},
		{0x03b402, 34},
		{0x2768b4, 25},
		{0x375b69, 240},
		{0x1010be, 19},
		{0x650fa0, 54},
		{0xd0dd01, 184},
		{0x954e38, 131},
		{0x86d8da, 116},
		{0x4ff950, 83},
		{0x15ba25, 34},
		{0xadfc13, 154},
		{0xa0ae64, 143},
		{0xb86baf, 133},
		{0x46d2ab, 79},
		{0xc16d97, 132},
		{0xc0e589, 150},
		{0xebbacf, 182},
		{0xc5bb14, 142},
		{0x34846c, 29},
		{0x8ae7ea, 116},
		{0xdcc523, 178},
		{0x79ab60, 107},
		{0x9a280e, 88},
		{0xdff8e3, 194},
		{0x843719, 52},
		{0x9b21ea, 92},
		{0xad855b, 137},
		{0x05ddc1, 43},
		{0xd49673, 173},
		{0xa1377a, 89},
		{0x0a445f, 24},
		{0xc0c6c7, 251},
		{0x443bc2, 62},
		{0x1ec2df, 45},
		{0xaa36ff, 129},
		{0xeeb860, 179},
		{0xb4b163, 143},
		{0xb484f8, 141},
		{0x8ecf2e, 112},
		{0xfaa01e, 214},
		{0x0b5b80, 24},
		{0x1f03ec, 20},
		{0x0ae19d, 42},
		{0xbd032c, 124},
		{0x809319, 100},
		{0xe9a1be, 218},
		{0x98e6db, 116},
		{0xa3da5e, 149},
		{0x5ad6ed, 81},
		{0xba5852, 167},
		{0x5ed891, 78},
		{0xa0165e, 125},
		{0xbd01bf, 127},
		{0x873cff, 92},
		{0x99ce8c, 150},
		{0xc11b29, 124},
		{0x35b1bf, 37},
		{0x0dc89d, 43},
		{0x434995, 61},
		{0x9eba82, 108},
		{0x71f07a, 120},
		{0x89ebee, 116},
		{0x7a3641, 131},
		{0xa7d39f, 151},
		{0x5ff226, 82},
		{0xded708, 184},
		{0x31aceb, 39},
		{0x342498, 18},
		{0xa4d8b9, 151},
		{0xaae1c2, 151},
		{0x72ec78, 120},
		{0xe079fe, 177},
		{0x56ac6c, 71},
		{0x28eefd, 87},
		{0xac6548, 173},
		{0x000000, 16},
		{0x050505, 232},
		{0x0a0a0a, 232},
		{0x0f0f0f, 233},
		{0x141414, 233},
		{0x191919, 234},
		{0x1e1e1e, 234},
		{0x232323, 235},
		{0x282828, 235},
		{0x2d2d2d, 236},
		{0x323232, 236},
		{0x373737, 237},
		{0x3c3c3c, 237},
		{0x414141, 238},
		{0x464646, 238},
		{0x4b4b4b, 239},
		{0x505050, 239},
		{0x555555, 240},
		{0x5a5a5a, 240},
		{0x5f5f5f, 59},
		{0x646464, 241},
		{0x696969, 242},
		{0x6e6e6e, 242},
		{0x737373, 243},
		{0x787878, 243},
		{0x7d7d7d, 244},
		{0x828282, 244},
		{0x878787, 102},
		{0x8c8c8c, 245},
		{0x919191, 246},
		{0x969696, 246},
		{0x9b9b9b, 247},
		{0xa0a0a0, 247},
		{0xa5a5a5, 248},
		{0xaaaaaa, 248},
		{0xafafaf, 145},
		{0xb4b4b4, 249},
		{0xb9b9b9, 250},
		{0xbebebe, 250},
		{0xc3c3c3, 251},
		{0xc8c8c8, 251},
		{0xcdcdcd, 252},
		{0xd2d2d2, 252},
		{0xd7d7d7, 188},
		{0xdcdcdc, 253},
		{0xe1e1e1, 254},
		{0xe6e6e6, 254},
		{0xebebeb, 255},
		{0xf0f0f0, 255},
		{0xf5f5f5, 255},
		{0xfafafa, 231},
		{0xffffff, 231},
	} {
		if got := interp.NearestPaletteColor(c.rgb>>16, c.rgb>>8&0xff, c.rgb&0xff); got != c.want {
			t.Errorf("#%06x: got %d, want %d", c.rgb, got, c.want)
		}
	}
}

// Loading `zsh/nearcolor` is what turns it on, for the prompt's colors and
// for `region_highlight` alike, and a subshell that takes it away takes it
// away from itself. Measured 2026-10-02 on zsh 5.9.2.
func TestLoadingNearcolorWritesHexAsThePalette(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, `print -rn -P '%F{#ff0000}'; print
zmodload zsh/nearcolor; print -r -- "load=$? [$(zmodload -lF zsh/nearcolor)]"
print -rn -P '%F{#ff0000}%K{#123456}'; print
(zmodload -u zsh/nearcolor; print -rn -P '%F{#ff0000}'; print)
print -rn -P '%F{#ff0000}'; print`)
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[38;2;255;0;0m\nload=0 []\n\x1b[38;5;196m\x1b[48;5;24m\n\x1b[38;2;255;0;0m\n\x1b[38;5;196m\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
	r, buf := zleRunner(t, `
		zmodload zsh/nearcolor
		w() { region_highlight=("0 2 fg=#ff0000"); print -rl -- $region_highlight; }
		zle -N w
	`)
	if _, ok, said := runWidget(t, r, buf, "w", repl.Line{Buffer: "ab"}); !ok || said != "0 2 fg=196\n" {
		t.Errorf("region_highlight read back %q (ran %v), want %q", said, ok, "0 2 fg=196\n")
	}
}
