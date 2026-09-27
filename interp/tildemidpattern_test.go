// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// tildeMid runs one snippet under a grammar that lets a `~(…)` group into a
// word, with extended patterns on so that a group can stand around one.
func tildeMid(t *testing.T, src string, setup func(*Runner)) string {
	t.Helper()
	out, st := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		// The two flags the rows need, named rather than a shell that
		// happens to carry both: the group is a grammar answer — the lexer
		// has to let the `(` into the word — and a row that puts one inside
		// `@(…)` needs a pattern group to put it in.
		d.TildeGroup = true
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
	}, setup)
	if st != 0 && !strings.Contains(src, "== ") {
		t.Fatalf("%s: status %d, out %q", src, st, out)
	}
	return strings.TrimSpace(out)
}

// A `~(…)` group is read where it stands and not only at the front of a
// pattern.
//
// The one letter honored from the middle is `i`, and the rows are
// splitTildeHereGroup's own table put to the three surfaces a pattern has:
// the condition, a `case` arm, and pathname expansion. Nothing here names a
// shell — the construct is a grammar flag and the letter is that construct's.
func TestATildeGroupInTheMiddleOfAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The two controls, and they are what make the rest readable: the
		// first says the probe can see a pattern *not* fold and the second
		// that it can see one fold. Without them every row below reads the
		// same whether the group was honored or the subject happened to
		// match.
		{"no fold at all", `[[ za == zA ]] && echo YES || echo NO`, "NO"},
		{"a group at the front folds", `[[ za == ~(i)zA ]] && echo YES || echo NO`, "YES"},

		{"and one in the middle folds", `[[ za == z~(i)A ]] && echo YES || echo NO`, "YES"},
		{"with text behind it", `[[ zab == z~(i)AB ]] && echo YES || echo NO`, "YES"},
		{"inside a pattern group", `[[ za == @(z~(i)A) ]] && echo YES || echo NO`, "YES"},
		{"behind a star", `[[ za == *~(i)A ]] && echo YES || echo NO`, "YES"},
		{"twice over", `[[ zab == z~(i)A~(i)B ]] && echo YES || echo NO`, "YES"},

		// **The sense sign is read**, which is what says the group applies
		// from where it stands rather than to the whole pattern: a reading
		// that took any `i` anywhere would fold the `A` here and match.
		{
			"and `-i` turns it off again",
			`[[ za == ~(i)z~(-i)A ]] && echo YES || echo NO`, "NO",
		},
		{
			"the other way round", `[[ za == ~(-i)z~(i)A ]] && echo YES || echo NO`,
			"YES",
		},

		// The fold reaches a bracket and a class, which is the difference
		// between this fold and the run-time option's.
		{"a bracket folds", `[[ za == z~(i)[A] ]] && echo YES || echo NO`, "YES"},
		{"and a class", `[[ za == z~(i)[[:upper:]] ]] && echo YES || echo NO`, "YES"},

		// A group that asks for nothing is stepped over and changes no
		// answer.
		{"an empty group asks nothing", `[[ zA == z~()A ]] && echo YES || echo NO`, "YES"},

		// **A group holding a *flavor* letter is not this file's**, and it is
		// read all the same — by interp/tildeflavorhere.go, which settles the
		// language of the whole pattern rather than setting a flag on a
		// branch. These two rows were written here pinning the opposite, so
		// that they would fail on the day the group started being consumed;
		// #4883 was that day, and they carry the reference's answers now.
		// They stay because they are the boundary between the two readers:
		// the first is the flavor being honored and the second is `E` not
		// being a letter that matches itself.
		{
			"a flavor letter is read by the other reader",
			`[[ zA == z~(E)A ]] && echo YES || echo NO`, "YES",
		},
		{
			"and is not text that matches itself",
			`[[ zEA == z~(E)A ]] && echo YES || echo NO`, "NO",
		},

		// A `case` arm is the same pattern language.
		{"a case arm reads it", `case za in z~(i)A) echo YES;; *) echo NO;; esac`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// And pathname expansion reaches it, which takes one thing more than the
// matcher: the gate at the top of the walk has to read the field as a pattern
// at all.
//
// A field with no `*`, `?` or bracket in it and a group somewhere other than
// its front was a spelled-out name — looked up as the characters it was
// written with, found missing, and passed back through as text — so the
// matcher never saw it. See holdsTildeFoldGroup.
func TestATildeGroupInTheMiddleOfAGlob(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"za", "zb", "zab"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		// The controls again, on this surface: a field that folds nothing
		// stands as written, and one whose group is at the front folds.
		{"no fold at all", `printf "[%s]" zA`, `[zA]`},
		{"a group at the front folds", `printf "[%s]" ~(i)zA`, `[za]`},

		{"and one in the middle folds", `printf "[%s]" z~(i)A`, `[za]`},
		{"with text behind it", `printf "[%s]" z~(i)AB`, `[zab]`},
		{"behind a star", `printf "[%s]" *~(i)A`, `[za]`},
		{"a bracket folds", `printf "[%s]" z~(i)[AB]`, `[za][zb]`},

		// **A group that asks for nothing still makes the field a pattern
		// here, and at the front it does not.** The test is whether the walk
		// will consume the group, not whether the group changes anything:
		// `printf "[%s]" z~()b` is `[zb]` and `printf "[%s]" ~()zb` is
		// `[~()zb]`, both of them ksh93u+'s answers.
		{"an empty group in the middle is still a pattern", `printf "[%s]" z~()b`, `[zb]`},
		{"a plus with no letter behind it too", `printf "[%s]" z~(+)b`, `[zb]`},
		{"one at the front is not", `printf "[%s]" ~()zb`, `[~()zb]`},

		// And one holding a flavor letter is read by the other reader here
		// too, which is the glob half of the two condition rows above: the
		// gate at the top of the walk reads the field as a pattern and
		// interp/tildeflavorhere.go answers it. #4883.
		{"one holding a flavor letter is read too", `printf "[%s]" z~(E)b`, `[zb]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tildeMid(t, tc.src, func(r *Runner) { r.Dir = dir })
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
