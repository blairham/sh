// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect writes for C's three punctuation escapes in a printf
// *format* (#4169).
//
// Measured 2026-09-22 under `LC_ALL=C` over a script file, against bash
// 5.3.20, bash as sh, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2, dash 0.5.12
// and BusyBox ash 1.37.0 in the pinned Alpine image. bash and ksh93 write the
// character alone; zsh, dash and BusyBox ash write the backslash and the
// character. The three escapes move together in every column.
//
// Here rather than in one dialect's own file for the reason the escape
// character's table is here: no suite tier holds a split, and this one is
// 4-3. And the two columns that write the character reach it by *different
// routes*, which is the whole point of putting them side by side — bash
// defines the three and keeps a backslash on everything else, while ksh93
// drops the backslash from every escape it does not know. The `\q` row below
// is what separates them, and it is why ksh93's column is no evidence for
// interp.Semantics.PrintfQuoteAndQuestionEscapes and leaves that axis on its
// standing answer.
func TestEachDialectReadsCsQuoteAndQuestionEscapes(t *testing.T) {
	asher := dialecttest.Preset{
		Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
		Diagnostics: ash.Diagnostics, Apply: ash.Apply,
	}
	for _, c := range []struct {
		snippet                     string
		bash, zsh, ksh, dashed, asa string
	}{
		// The three, one at a time. The single quote's row spells its
		// format in double quotes, because a single-quoted word has no way
		// to hold one — and a backslash before a quote is two characters of
		// a double-quoted word in every shell, so the format still reaches
		// printf as the escape.
		{`printf 'a\?Z'`, "a?Z", `a\?Z`, "a?Z", `a\?Z`, `a\?Z`},
		{`printf 'a\"Z'`, `a"Z`, `a\"Z`, `a"Z`, `a\"Z`, `a\"Z`},
		{`printf "a\'Z"`, "a'Z", `a\'Z`, "a'Z", `a\'Z`, `a\'Z`},

		// The separator, and the reason two columns agree here for two
		// reasons: ksh93 drops the backslash from every undefined escape,
		// so it writes the character for a `\q` too, where bash keeps it.
		{`printf 'a\qZ'`, `a\qZ`, `a\qZ`, "aqZ", `a\qZ`, `a\qZ`},

		// The `%b` site, which keeps the backslash in all seven reference
		// columns — ksh93 included, since its drop is the format's alone.
		// Without this the axis could be read as a fact about the escape
		// rather than about the site.
		{`printf '%b' 'a\?Z'`, `a\?Z`, `a\?Z`, `a\?Z`, `a\?Z`, `a\?Z`},

		// And a control from the table the format does define, so the rows
		// above read as a claim about these three characters rather than
		// about the escape reader.
		{`printf 'a\tZ'`, "a\tZ", "a\tZ", "a\tZ", "a\tZ", "a\tZ"},
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
