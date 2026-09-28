// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `~(…)` flavor group inside a **pattern group** is read.
//
// The group's body is a piece of pattern like any other and the matcher's
// contract for one is that it describes the piece whole, so the flavor is
// anchored to the span the group takes rather than searched over the subject
// the way one at the top of a pattern is. Both halves are measured, and the
// pair of rows at the top is what separates them.
func TestATildeFlavorGroupInsideAPatternGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// **The pair.** The same subject and the same flavor, with and
		// without a group around it, answer differently: at the top the
		// expression `za` is searched in `zza` and finds it; inside the
		// group it has to describe the span and does not.
		{"at the top it searches", `[[ zza == z~(E)a ]] && echo YES || echo NO`, "YES"},
		{"inside a group it does not", `[[ zza == @(z~(E)a) ]] && echo YES || echo NO`, "NO"},

		{"the group matches its span", `[[ za == @(z~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"and nothing else", `[[ zXa == @(z~(E)a) ]] && echo YES || echo NO`, "NO"},
		{"at the head of the body", `[[ za == @(~(E)za) ]] && echo YES || echo NO`, "YES"},

		// The span the group takes is exactly what the expression
		// describes, which these three say together: text behind the group
		// has to match what is left, and nothing may be swallowed.
		{"text behind the group", `[[ zab == @(z~(E)a)b ]] && echo YES || echo NO`, "YES"},
		{"which has to match", `[[ zaXb == @(z~(E)a)b ]] && echo YES || echo NO`, "NO"},
		{"and nothing is swallowed", `[[ zaaq == @(z~(E)a)q ]] && echo YES || echo NO`, "NO"},

		// **The expression is compiled rather than the glob walked**, which
		// is the row a reading that merely stepped over the group would get
		// wrong: `a*` is zero-or-more `a` in an expression and `a` then
		// anything in a glob.
		{"`a*` is the expression's", `[[ zab == @(z~(E)a*) ]] && echo YES || echo NO`, "NO"},
		{"and it repeats", `[[ zaa == @(z~(E)a*) ]] && echo YES || echo NO`, "YES"},
		{"as does `a+`", `[[ zaa == @(z~(E)a+) ]] && echo YES || echo NO`, "YES"},

		// Every shape a group has, since each is a different route into the
		// arm matcher.
		{"one arm of several", `[[ za == @(q|z~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"the other arm still answers", `[[ q == @(z~(E)a|q) ]] && echo YES || echo NO`, "YES"},
		{"a repeating group", `[[ zaza == *(z~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"an optional one", `[[ za == ?(z~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"a negated one", `[[ za == !(z~(E)b) ]] && echo YES || echo NO`, "YES"},
		{"a nested one", `[[ zab == @(@(z)~(E)a)b ]] && echo YES || echo NO`, "YES"},
		{"two groups in a row", `[[ zab == @(z~(E)a)@(b) ]] && echo YES || echo NO`, "YES"},

		// A glob in front of the group inside the body is translated, the
		// same way it is for one at the top of a pattern.
		{"a star in front of it", `[[ zaXXa == @(z*~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"and the star may take nothing", `[[ za == @(z*~(E)a) ]] && echo YES || echo NO`, "YES"},

		// **A pattern group in *front* of a flavor is translated too**, so
		// the whole is one searched expression rather than one anchored
		// where the group stopped (#4920). The third row is what says the
		// group is still read: a subject the arm does not describe misses.
		{"a group in front, span contiguous", `[[ za == @(z)~(E)a ]] && echo YES || echo NO`, "YES"},
		{"a group in front, span not", `[[ zaa == @(z)~(E)a ]] && echo YES || echo NO`, "YES"},
		{"and the group's arm still decides", `[[ zXa == @(z)~(E)a ]] && echo YES || echo NO`, "NO"},

		// **A fold the branch arrived with reaches the expression**, and the
		// pair is what says so: the same pattern without the fold at the
		// front does not match, so the row above it is the `i` composing
		// rather than the subject happening to agree.
		{"a fold at the front reaches it", `[[ zA == ~(i)@(z~(E)a) ]] && echo YES || echo NO`, "YES"},
		{"and without it nothing matches", `[[ zA == @(z~(E)a) ]] && echo YES || echo NO`, "NO"},
		{"the fold reaches the glob too", `[[ za == ~(i)@(z~(E)A) ]] && echo YES || echo NO`, "YES"},
		{"with text behind the group", `[[ zAb == ~(i)@(z~(E)a)b ]] && echo YES || echo NO`, "YES"},

		// A `case` arm is the same pattern language.
		{"a case arm reads it", `case za in @(z~(E)a)) echo YES;; *) echo NO;; esac`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// Pathname expansion reads it too, which takes one thing more than the
// matcher: the gate at the top of the walk has to see the field as a pattern.
// A `@(` is a metacharacter, so that half was already there.
func TestATildeFlavorGroupInsideAPatternGroupOnAGlob(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"za", "zb", "zab"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		// The control: a pattern that names nothing stands as written, so a
		// row below answering with a name is a match rather than a field
		// passed through.
		{"a group that names nothing", `printf "[%s]" @(z~(E)q)`, `[@(z~(E)q)]`},

		{"inside a group", `printf "[%s]" @(z~(E)a)`, `[za]`},
		{"one arm of several", `printf "[%s]" @(z~(E)a|q)`, `[za]`},
		{"and the span is the name", `printf "[%s]" @(z~(E)b)`, `[zb]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tildeMid(t, tc.src, func(r *Runner) { r.Dir = dir })
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A trim and a substitution are **left out**, and these rows are what keep
// them out rather than a comment saying so.
//
// The reference shell's answers for this shape on a surface that chooses a
// span do not compose — with `v=abcd`, `${v#@(a~(E)b)}` is the whole value,
// `${v/@(b~(E)c)/X}` is the whole value, and `${v%@(c~(E)d)}` is nothing at
// all, from one shape. No reading produces all three, so those surfaces
// answer exactly what they answered before the group inside a pattern group
// was read at all: the pattern does not match and the value comes back
// whole.
func TestAFlavorInsideAPatternGroupIsLeftOutOfATrim(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls, and they are the reason the rows below are readable:
		// an **ungrouped** mid-pattern flavor does reach a trim and a
		// substitution, so these three answers are known to be reachable
		// and the rows after them are the group being declined rather than
		// the flavor never being read.
		{"a trim reads an ungrouped one", `v=abcd; printf "[%s]" "${v#a~(E)b}"`, "[cd]"},
		{"and a suffix trim", `v=abcd; printf "[%s]" "${v%~(E)cd}"`, "[ab]"},
		{"and a substitution", `v=abcd; printf "[%s]" "${v/~(E)bc/X}"`, "[aXd]"},
		{"and a plain group with no flavor", `v=abcd; printf "[%s]" "${v#@(ab)}"`, "[cd]"},

		{"a grouped one in a prefix trim", `v=abcd; printf "[%s]" "${v#@(a~(E)b)}"`, "[abcd]"},
		{"in a suffix trim", `v=abcd; printf "[%s]" "${v%@(c~(E)d)}"`, "[abcd]"},
		{"in a substitution", `v=abcd; printf "[%s]" "${v/@(b~(E)c)/X}"`, "[abcd]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
