// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect writes where a printf *format* carries the escape
// character, byte by byte (#3225).
//
// Measured 2026-09-16 under `LC_ALL=C` with `od -An -c`, over a script file,
// against bash 5.3.20, bash as sh, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2,
// dash 0.5.12 and BusyBox ash 1.37.0 in the pinned Alpine image:
//
//	printf 'a\eZ'   a 033 Z  in all but dash, which writes a \ e Z
//	printf 'a\EZ'   a 033 Z  in the three bash builds and ksh93u+;
//	                a \ E Z  in zsh, BusyBox ash and dash
//
// No suite tier can hold that. A dialect's own file guards one column against
// its own reference and `core/` needs all five to agree, so a 6-1 split has
// no tier — which is why the whole panel's answer is asserted here, in one
// table, for all five presets at once.
//
// The `%b` site is in the same table on purpose. The two sites are four
// fields and not two, and ksh93u+ is what makes that necessary by taking `\e`
// in a format and writing the two characters in a `%b`; a table that asked
// only the format's would have let the sites drift back together.
func TestEachDialectReadsTheFormatsEscapeCharacter(t *testing.T) {
	asher := dialecttest.Preset{
		Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
		Diagnostics: ash.Diagnostics, Apply: ash.Apply,
	}
	const esc = "\x1b"
	for _, c := range []struct {
		snippet                     string
		bash, zsh, ksh, dashed, asa string
	}{
		// The format's two letters, which is the whole of the fix.
		{`printf 'a\eZ'`, "a" + esc + "Z", "a" + esc + "Z", "a" + esc + "Z", `a\eZ`, "a" + esc + "Z"},
		{`printf 'a\EZ'`, "a" + esc + "Z", `a\EZ`, "a" + esc + "Z", `a\EZ`, `a\EZ`},

		// The idiom this is a P1 for, read across a format reused over its
		// operands so the escape is met on every pass.
		{
			`printf '\e[1m%s\e[0m' bold`,
			esc + "[1mbold" + esc + "[0m", esc + "[1mbold" + esc + "[0m",
			esc + "[1mbold" + esc + "[0m", `\e[1mbold\e[0m`, esc + "[1mbold" + esc + "[0m",
		},

		// The `%b` argument's two letters beside them. ksh93 is the column
		// whose two sites disagree, and BusyBox ash is the column whose `%b`
		// answer this change corrected: the golden record has said the
		// escape character since the row was recorded and the preset said
		// the two characters.
		{`printf '%b' 'a\eZ'`, "a" + esc + "Z", "a" + esc + "Z", `a\eZ`, `a\eZ`, "a" + esc + "Z"},
		{`printf '%b' 'a\EZ'`, "a" + esc + "Z", `a\EZ`, "a" + esc + "Z", `a\EZ`, `a\EZ`},

		// The controls, unanimous in all seven reference columns: the rest
		// of the format's escape set, an octal, a doubled backslash and two
		// letters next to the two that moved. Without these the table would
		// read as a claim about the escape reader rather than about `\e`
		// and `\E`.
		{`printf 'a\tb\vc\fd\re\af'`, "a\tb\vc\fd\re\af", "a\tb\vc\fd\re\af", "a\tb\vc\fd\re\af", "a\tb\vc\fd\re\af", "a\tb\vc\fd\re\af"},
		{`printf 'a\0101Z'`, "a\x081Z", "a\x081Z", "a\x081Z", "a\x081Z", "a\x081Z"},
		{`printf 'a\\eZ'`, `a\eZ`, `a\eZ`, `a\eZ`, `a\eZ`, `a\eZ`},
		{`printf 'a\gZ'`, `a\gZ`, `a\gZ`, `a\gZ`, `a\gZ`, `a\gZ`},
		{`printf 'a\FZ'`, `a\FZ`, `a\FZ`, `a\FZ`, `a\FZ`, `a\FZ`},
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
