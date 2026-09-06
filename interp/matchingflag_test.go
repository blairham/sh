// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// selectingWithFlags is the grammar these expansions need, named by the
// constructs rather than by a shell.
func selectingWithFlags(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// The `M` flag substitutes what a pattern *took* rather than what it left.
//
// The issue this came from called it "the flag that inverts `:#` to keep
// matches", which is one of its two effects: measured across every operator a
// flag group may stand in front of, it reaches the four trims — where it
// substitutes the matched part — and `:#`, and nothing else. Every row below
// is a measurement on zsh 5.9.2, the only panel shell with the construct.
func TestTheMatchingFlagSubstitutesWhatThePatternTook(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The trims. The operator still chooses shortest or longest; the
		// flag only chooses which side of the split is substituted.
		{"a prefix trim keeps the prefix", `v=hello; printf "[%s]" "${(M)v#hel}"`, "[hel]"},
		{"the shortest prefix is still the shortest", `v=hello; printf "[%s]" "${(M)v#h*l}"`, "[hel]"},
		{"and the doubled operator the longest", `v=hello; printf "[%s]" "${(M)v##h*l}"`, "[hell]"},
		{"a suffix trim keeps the suffix", `v=hello; printf "[%s]" "${(M)v%llo}"`, "[llo]"},
		{"the shortest suffix", `v=hello; printf "[%s]" "${(M)v%l*o}"`, "[lo]"},
		{"and the longest", `v=hello; printf "[%s]" "${(M)v%%l*o}"`, "[llo]"},
		// Nothing taken is nothing substituted, which is the sharpest row:
		// the trim without the flag answers with the *whole* value here, so
		// a reading that fell back to it would be plausible and wrong.
		{"a pattern that matches nothing leaves nothing", `v=hello; printf "[%s]" "${(M)v#zzz}"`, "[]"},
		{"the same at the other end", `v=hello; printf "[%s]" "${(M)v%zzz}"`, "[]"},
		{"an empty pattern takes nothing", `v=hello; printf "[%s]" "${(M)v#}"`, "[]"},
		// Elementwise over an array, exactly as the trims are without it.
		{"elementwise over an array", `a=(abc bcd); printf "[%s]" "${(M@)a#b}"`, "[][b]"},

		// `:#` — the half the issue named. Keep what matches instead of
		// dropping it.
		{"the exclusion is inverted", `a=(f1 f22 f333); printf "[%s]" "${(M@)a:#f2*}"`, "[f22]"},
		{"the flag may be written either side of the @", `a=(f1 f22 f333); printf "[%s]" "${(@M)a:#f2*}"`, "[f22]"},
		{"without the flag it still drops", `a=(f1 f22 f333); printf "[%s]" "${(@)a:#f2*}"`, "[f1][f333]"},
		{"a scalar keeps itself when it matches", `v=hello; printf "[%s]" "${(M)v:#hel*}"`, "[hello]"},
		{"and comes to nothing when it does not", `v=hello; printf "[%s]" "${(M)v:#xyz}"`, "[]"},
		// The whole-match rule that separates `:#` from `#` is untouched by
		// the flag: `h` matches no whole element.
		{"the whole-match rule still holds", `v=hello; printf "[%s]" "${(M)v:#h}"`, "[]"},
		{"an empty array stays empty", `a=(); printf "[%s]" "${(M@)a:#x}"`, "[]"},
		// Quoted and without `(@)` the array joins first, so the pattern is
		// matched against one string and takes it or nothing. This is the
		// row the issue's own probe table got the other way round: its
		// `${(M)a:#f2*}` was written unquoted.
		{"quoted without @ the array joins first", `a=(f1 f22 f333); printf "[%s]" "${(M)a:#f2*}"`, "[]"},
		{"and unquoted it does not", `a=(f1 f22 f333); printf "[%s]" ${(M)a:#f2*}`, "[f22]"},

		// Everywhere else it does nothing, and that is measured rather than
		// assumed — one row per operator a flag group may stand in front of.
		{"no operator at all", `a=(f1 f22); printf "[%s]" "${(M)a}"`, "[f1 f22]"},
		{"a default", `v=hello; printf "[%s]" "${(M)v:-alt}"`, "[hello]"},
		{"a replacement", `v=hello; printf "[%s]" "${(M)v/l/L}"`, "[heLlo]"},
		{"every replacement", `v=hello; printf "[%s]" "${(M)v//l/L}"`, "[heLLo]"},
		{"an anchored replacement", `v=hello; printf "[%s]" "${(M)v/#he/X}"`, "[Xllo]"},
		{"a substring", `v=hello; printf "[%s]" "${(M)v:1}"`, "[ello]"},
		{"a set difference", `a=(x y z); b=(y w); printf "[%s]" "${(M@)a:|b}"`, "[x][z]"},
		{"a set intersection", `a=(x y z); b=(y w); printf "[%s]" "${(M@)a:*b}"`, "[y]"},

		// And it composes with the flags already built.
		{"with the uppercasing flag", `a=(f1 f22); printf "[%s]" "${(MU@)a:#f2*}"`, "[F22]"},
		{"with the lowercasing one, which runs after the match", `a=(f1 f22); printf "[%s]" "${(ML@)a:#F2*}"`, "[]"},
		{"written twice it is written once", `v=hello; printf "[%s]" "${(MM)v#hel}"`, "[hel]"},
		// An `M` that is not a flag is not this flag. The letter has to be
		// read from the flag group and not from the expansion's text, and
		// both of the places it can otherwise appear are here: the parameter
		// name and the pattern.
		{"an M in the parameter name is not the flag", `M=hello; printf "[%s]" "${(U)M#h}"`, "[ELLO]"},
		{"nor an M in the pattern", `v=hello; printf "[%s]" "${(U)v#M}"`, "[HELLO]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The by-name diagnostic for the flags still unbuilt does not degrade as one
// is built. It is what let this flag be found precisely instead of silently
// producing a wrong list, and a letter added to the implemented set is a
// letter taken out of that guarantee — so the guarantee is asserted, whole
// rendered line and all, rather than assumed.
func TestTheUnbuiltFlagsAreStillRefusedByName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"e", `v=x; printf "[%s]" "${(e)v}"`, "sh: ${(e)v}: the (e) expansion flag is not implemented\n"},
		{"t", `v=x; printf "[%s]" "${(t)v}"`, "sh: ${(t)v}: the (t) expansion flag is not implemented\n"},
		{"z", `v=x; printf "[%s]" "${(z)v}"`, "sh: ${(z)v}: the (z) expansion flag is not implemented\n"},
		{"D", `v=x; printf "[%s]" "${(D)v}"`, "sh: ${(D)v}: the (D) expansion flag is not implemented\n"},
		// `A` is the one that is easiest to lose here, because `a` beside it
		// *is* built and the two differ only in case: one orders a list by
		// its index and the other makes an assignment an array assignment.
		{"A", `printf "[%s]" "${(A)x::=a b c}"`, "sh: ${(A)x::=a b c}: the (A) expansion flag is not implemented\n"},
		// One built letter beside an unbuilt one still names the unbuilt
		// one, which is the half that would rot as the set grows.
		{"beside a built one", `a=(b a); printf "[%s]" "${(Uz)a}"`, "sh: ${(Uz)a}: the (z) expansion flag is not implemented\n"},
		{"and the built one may be written second", `a=(b a); printf "[%s]" "${(zU)a}"`, "sh: ${(zU)a}: the (z) expansion flag is not implemented\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the unbuilt flag refused")
			}
		})
	}
}

// And the one that was refused is not refused any more, said as a whole
// rendered line so that a message merely reworded would still fail.
func TestTheMatchingFlagIsNoLongerRefused(t *testing.T) {
	out, st := runGrammar(t, `a=(f1 f22 f333); printf "[%s]" "${(M@)a:#f2*}"`, selectingWithFlags, nil)
	if out != "[f22]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[f22]")
	}
}
