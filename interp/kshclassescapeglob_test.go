// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `~(K)` class escape has to survive the **field**, and not only the
// pattern operand.
//
// `#4961` gave the matcher the six classes and `#4978` gave them the
// surfaces, and pathname expansion was reading the letter all the same —
// because the backslash never reached the matcher. A `\d` the script wrote
// is a span of its own with the backslash folded into its
// [syntax.Quoting], and the field path took its value bare where
// `Runner.patternOf` re-emits the backslash for a pattern operand. So
// `echo ~(K)a\db` globbed for `adb`.
//
// Measured 2026-09-28 against `/bin/ksh` `Version AJM 93u+ 2012-08-01`
// (`go version -m` says *not a Go executable*), `-c` under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME`.
func TestAKshClassEscapeSurvivesAFieldsQuoteRemoval(t *testing.T) {
	full := t.TempDir()
	for _, n := range []string{"a1b", "adb"} {
		if err := os.WriteFile(filepath.Join(full, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	empty := t.TempDir()
	zw := t.TempDir()
	for _, n := range []string{"ab", "abb"} {
		if err := os.WriteFile(filepath.Join(zw, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name, dir, src, want string
	}{
		// The row the issue is about, and its control: with no group in
		// front of it the escape is the letter, in that shell as here, so a
		// difference between these two is the group's doing.
		{"a class escape names the class", full, `printf "[%s]" ~(K)a\db`, `[a1b]`},
		{"and with no group it is the letter", full, `printf "[%s]" a\db`, `[adb]`},
		{"another letter's group is not it", full, `printf "[%s]" ~(i)a\db`, `[adb]`},

		// The three other class letters that can tell the two names apart
		// here, so the row above is not one letter getting lucky.
		{"the complement names the other one", full, `printf "[%s]" ~(K)a\Db`, `[adb]`},
		{"a word character names both", full, `printf "[%s]" ~(K)a\wb`, `[a1b][adb]`},
		{"and a wildcard is the control on the width", full, `printf "[%s]" ~(K)a?b`, `[a1b][adb]`},

		// **The backslash must not reach the output of an unmatched
		// field**, which is the row that decides the shape of the fix: a
		// representation that survived quote removal as text would print
		// `~(K)a\db` where both columns print `~(K)adb`.
		{"an unmatched field keeps no backslash", empty, `printf "[%s]" ~(K)a\db`, `[~(K)adb]`},
		{"nor with text in front of the group", empty, `printf "[%s]" zz~(K)a\db`, `[zz~(K)adb]`},
		{"nor with two escapes in it", empty, `printf "[%s]" ~(K)a\db\db`, `[~(K)adbdb]`},
		{"a control escape the same way", empty, `printf "[%s]" ~(K)a\nb`, `[~(K)anb]`},
		{"and a letter no family names", empty, `printf "[%s]" ~(K)a\qb`, `[~(K)aqb]`},
		{"with no group either", empty, `printf "[%s]" a\db`, `[adb]`},

		// Quoting decides it here as it does for the pattern operand: a
		// group that arrived quoted is the characters it was written with,
		// so the escape behind it is the letter and the field spells a name.
		{"a quoted group is characters", empty, `printf "[%s]" "~(K)"a\db`, `[~(K)adb]`},
		{"an escaped one too", empty, `printf "[%s]" \~\(K\)a\db`, `[~(K)adb]`},

		// The **zero-width** family reaches this surface by the same route
		// and is the sharpest evidence that what is kept is the glob's own
		// backslash: `\b` consumes nothing, so a field that read it as the
		// letter would match a *longer* name. Both directions are here —
		// the boundary holding where the letter would not match and
		// declining where it would.
		{"a boundary at the end", zw, `printf "[%s]" ~(K)ab\b`, `[ab]`},
		{"where the letter names the longer name", zw, `printf "[%s]" ab\b`, `[abb]`},
		{"and none between two letters", zw, `printf "[%s]" ~(K)a\bb`, `[~(K)abb]`},
		{"its complement holding there", zw, `printf "[%s]" ~(K)a\Bb`, `[ab]`},
		{"the end anchor", zw, `printf "[%s]" ~(K)ab\z`, `[ab]`},
		{"where the letter names nothing", zw, `printf "[%s]" ab\z`, `[abz]`},
		// An unmatched field keeps its live star and loses its backslash,
		// which is the two halves of the representation in one row.
		{"an unmatched one keeps its star", zw, `printf "[%s]" ~(K)a\bb*`, `[~(K)abb*]`},

		// And a **matched** field takes its text from the filesystem rather
		// than from the pattern, so there is nothing in it to leak — a
		// control rather than a risk, and cheap enough to hold.
		{"a matched field is the name on disk", full, `printf "[%s]" ~(K)a\wb`, `[a1b][adb]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tildeMid(t, tc.src, func(r *Runner) { r.Dir = tc.dir })
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
