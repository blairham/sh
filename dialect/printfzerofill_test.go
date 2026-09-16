// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect writes where a `0` fill meets the `0x` C's `#` wrote,
// byte by byte (#3066).
//
// Measured 2026-09-15 under `LC_ALL=C` against bash 5.3.20, bash as sh, bash
// 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0 —
// seven columns, and they part in exactly one place. C counts the prefix as
// part of the field, so the fill is the width less the `0x` and less the
// digits and `printf '%#05x' 7` is five characters; ksh93 pads the digits to
// the width and writes the prefix past it, which is seven. Go's `fmt` is the
// second reading, so this shell answered as ksh93 did in every dialect before
// the fix.
//
// ash is here and the other measurement of this axis (interp) cannot reach
// it: five dialects carry the answer and a table of four could not tell "ash
// was set" from "ash was never asked".
func TestEachDialectCountsTheAlternatePrefixOrDoesNot(t *testing.T) {
	asher := dialecttest.Preset{
		Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
		Diagnostics: ash.Diagnostics, Apply: ash.Apply,
	}
	for _, c := range []struct {
		snippet                     string
		bash, zsh, ksh, dashed, asa string
	}{
		// Where the readings part: the same bytes in six columns and two
		// characters more in ksh93.
		{`printf '[%#05x]' 7`, "[0x007]", "[0x007]", "[0x00007]", "[0x007]", "[0x007]"},
		{`printf '[%#05X]' 255`, "[0X0FF]", "[0X0FF]", "[0X000FF]", "[0X0FF]", "[0X0FF]"},
		{`printf '[%#010x]' 255`, "[0x000000ff]", "[0x000000ff]", "[0x00000000ff]", "[0x000000ff]", "[0x000000ff]"},
		{`printf '[%0#5x]' 7`, "[0x007]", "[0x007]", "[0x00007]", "[0x007]", "[0x007]"},
		// The width the prefix eats into, where C's fill goes empty and
		// ksh93's does not.
		{`printf '[%#05x]' 65535`, "[0xffff]", "[0xffff]", "[0x0ffff]", "[0xffff]", "[0xffff]"},
		{`printf '[%#06x]' 65535`, "[0xffff]", "[0xffff]", "[0x00ffff]", "[0xffff]", "[0xffff]"},
		{`printf '[%#07x]' 65535`, "[0x0ffff]", "[0x0ffff]", "[0x000ffff]", "[0x0ffff]", "[0x0ffff]"},
		{`printf '[%#020x]' -1`, "[0x00ffffffffffffffff]", "[0x00ffffffffffffffff]", "[0x0000ffffffffffffffff]", "[0x00ffffffffffffffff]", "[0x00ffffffffffffffff]"},

		// The controls. An octal's alternate form is a leading `0` that is
		// itself a fill character, so all seven write the same bytes; a
		// space fill goes in front of the whole field; `-` voids a zero fill
		// outright; a precision ignores the `0` flag; a sign is counted in
		// the width by everyone; and a width the digits already reach leaves
		// no fill to place.
		{`printf '[%#06o]' 255`, "[000377]", "[000377]", "[000377]", "[000377]", "[000377]"},
		{`printf '[%#05o]' 8`, "[00010]", "[00010]", "[00010]", "[00010]", "[00010]"},
		{`printf '[%#8x]' 7`, "[     0x7]", "[     0x7]", "[     0x7]", "[     0x7]", "[     0x7]"},
		{`printf '[%#-08x]' 7`, "[0x7     ]", "[0x7     ]", "[0x7     ]", "[0x7     ]", "[0x7     ]"},
		{`printf '[%05d]' -42`, "[-0042]", "[-0042]", "[-0042]", "[-0042]", "[-0042]"},
		{`printf '[%#03x]' 4095`, "[0xfff]", "[0xfff]", "[0xfff]", "[0xfff]", "[0xfff]"},
		{`printf '[%08x]' 255`, "[000000ff]", "[000000ff]", "[000000ff]", "[000000ff]", "[000000ff]"},

		// And the row #3070 pinned, which this must not have moved: a
		// nought's prefix is the other axis, and the column that keeps one
		// there is the column that writes it past the width.
		{`printf '[%#05x]' 0`, "[00000]", "[00000]", "[0x00000]", "[00000]", "[00000]"},
	} {
		for _, d := range []struct {
			name string
			p    dialecttest.Preset
			want string
		}{
			{"bash", presets["bash"], c.bash},
			{"zsh", presets["zsh"], c.zsh},
			{"ksh", presets["ksh"], c.ksh},
			{"dash", presets["dash"], c.dashed},
			{"ash", asher, c.asa},
		} {
			out, st, err := d.p.Combined(t, dialecttest.Base{}, c.snippet)
			if err != nil {
				t.Fatal(err)
			}
			if out != d.want || st != 0 {
				t.Errorf("%s: %s gave %q status %d, want %q and 0",
					d.name, c.snippet, out, st, d.want)
			}
		}
	}
}
