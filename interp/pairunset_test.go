// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What `unset` does to half of a joined pair, and the one kind of join that
// comes apart.
//
// Two kinds reach this: a **tie**, which a script writes with `typeset -T` and
// which a dialect installs for its own eight, and a **wordless pair**, which
// the shell maintains between two of its own specials and which no listing
// calls tied. See Runner.PairNames and the `wordless` field in
// interp/tiedscalar.go.
//
// Tests name the seam and never a shell.

// pairRun runs src over a runner carrying one pair of each kind: `S`/`s` tied,
// and `W`/`w` joined wordlessly.
func pairRun(t *testing.T, src string) string {
	t.Helper()
	d := syntax.Core()
	d.ArraySubscript = true
	d.ParamSetTestFlag = true
	f, err := syntax.Parse(src+"\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	sem := testSemantics()
	r := newTestRunner(t, &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out})
	r.Tie("S", "s", ":")
	r.PairNames("W", "w", ":")
	st, err := r.Run(t.Context(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st != 0 {
		t.Fatalf("status %d, out %q", st, out.String())
	}
	return out.String()
}

// TestUnsettingHalfAWordlessPairLeavesTheOtherHalf is the seam's own row: the
// half named goes, the other stays and is **emptied**, and the join stands.
func TestUnsettingHalfAWordlessPairLeavesTheOtherHalf(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the array half", `W=a:b; unset w; echo "+W=${+W} +w=${+w} W=[$W]"`, "+W=1 +w=0 W=[]\n"},
		// The array half stays *joined* and holding nothing. Its **set
		// test** is the dialect's answer rather than this seam's: a name the
		// shell owns reads 1 with no elements under it — which is what
		// `${+watch}` of 1 is in the reference and in dialect/zsh's own rows
		// — and a plain name here reads 0. So the engine's row asks what the
		// engine decides, and the mirror rows below are where "it is still
		// there" is observable without the mark.
		{"the scalar half", `W=a:b; unset W; echo "+W=${+W} w=[${w[@]}]"`, "+W=0 w=[]\n"},
		{"both, a line at a time", `W=a:b; unset w; unset W; echo "+W=${+W} +w=${+w}"`, "+W=0 +w=0\n"},
		{"the other order", `W=a:b; unset W; unset w; echo "+W=${+W} +w=${+w}"`, "+W=0 +w=0\n"},
		// The control: with no `unset` the pair is whole and the join works,
		// so each row above is about the removal.
		{"no unset", `W=a:b; echo "+W=${+W} +w=${+w} w=[${w[@]}]"`, "+W=1 +w=1 w=[a b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pairRun(t, tc.src); got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// And the mirror runs into a half that is still there and nowhere else.
//
// Four cells, varying **which half was removed** and **which half is then
// written** — the pair of nouns the rule is keyed on. A mirror that did not
// ask gets the first and last wrong; one that stopped after any `unset` gets
// the middle two wrong.
func TestAWordlessPairMirrorsOnlyIntoAHalfThatIsStillThere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"array gone, scalar written", `W=a:b; unset w; W=x:y; echo "+w=${+w} W=[$W]"`, "+w=0 W=[x:y]\n"},
		{"array gone, array written", `W=a:b; unset w; w=(q r); echo "+w=${+w} W=[$W]"`, "+w=1 W=[q:r]\n"},
		{"scalar gone, scalar written", `W=a:b; unset W; W=x:y; echo "+W=${+W} w=[${w[@]}]"`, "+W=1 w=[x y]\n"},
		{"scalar gone, array written", `W=a:b; unset W; w=(q r); echo "+W=${+W} w=[${w[@]}]"`, "+W=0 w=[q r]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pairRun(t, tc.src); got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// The **tie** is the control, and it is what keeps this from being a change to
// every pair in the engine: both halves of a tie still go away together, and
// the pairing is still kept so that a write to either re-makes it.
//
// Without these rows the change above reads as "an `unset` takes one half",
// which is true of one join and false of the other — and the gate that told
// them apart was written wrong once and caught here: a mirror that asked only
// "is the other half removed" left `${+s}` at 0 after `unset s; S=/y`.
func TestUnsettingHalfATieStillRemovesBoth(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the scalar half", `S=a:b; unset S; echo "+S=${+S} +s=${+s}"`, "+S=0 +s=0\n"},
		{"the array half", `S=a:b; unset s; echo "+S=${+S} +s=${+s}"`, "+S=0 +s=0\n"},
		{"a write after the array half went", `S=a:b; unset s; S=x:y; echo "+S=${+S} +s=${+s} s=[${s[@]}]"`, "+S=1 +s=1 s=[x y]\n"},
		{"a write after the scalar half went", `S=a:b; unset S; s=(q r); echo "+S=${+S} +s=${+s} S=[$S]"`, "+S=1 +s=1 S=[q:r]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pairRun(t, tc.src); got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}
