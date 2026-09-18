// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `export v` and `readonly v` with no value are declarations, so what they
// leave behind is DeclaredNameWithoutValueIsEmpty's answer — the same question
// `typeset v` asks, under two words that also carry an attribute.
//
// They were the two spellings that did not ask it. `typeset`, `declare` and
// `local` all went through declareEmpty; `export` called only the
// standing-empty half and `readonly` called declareEmpty only where it had
// just taken a scope, so in the dialect that sets a declared name empty a
// top-level `export v` and `readonly v` left the name *unset* — the freeze and
// the attribute landing on a name that did not exist (#2887, #3345).
//
// The difference is invisible to `echo "$v"`, which is why every case here
// reads through `${v-UNSET}`: the two states print the same thing and take
// opposite branches of the operator that separates them.
func attributeWordRun(t *testing.T, src string, empty Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = empty
		// The two words' own scoping questions, answered so that a top-level
		// case is a top-level case: `readonly` is one shell's `typeset -r`
		// and takes a scope inside a function where it is, and the suites
		// about that set it themselves.
		s.ReadonlyDeclaresALocal = No
		s.ReadonlyOptions = "p"
	})
}

func TestAnAttributeWordWithNoValueBringsTheNameIntoBeing(t *testing.T) {
	for _, word := range []string{"export", "readonly"} {
		t.Run(word, func(t *testing.T) {
			src := word + ` v; printf '[%s]' "${v-UNSET}" "${v:-COLON}"`
			if got, st := attributeWordRun(t, src, Yes); got != "[][COLON]" || st != 0 {
				t.Errorf("where a declared name is empty: got %q/%d, want %q/0", got, st, "[][COLON]")
			}
			if got, st := attributeWordRun(t, src, No); got != "[UNSET][COLON]" || st != 0 {
				t.Errorf("where it is not: got %q/%d, want %q/0", got, st, "[UNSET][COLON]")
			}
		})
	}
}

// And a name that already holds a value is not one the word is bringing into
// being, so the value survives. This is the guard that says the change is a
// declaration and not a store: emptying here would make a line that only meant
// to add an attribute destroy what it was adding it to.
func TestAnAttributeWordWithNoValueLeavesAHeldValueAlone(t *testing.T) {
	for _, word := range []string{"export", "readonly"} {
		t.Run(word, func(t *testing.T) {
			src := `v=kept; ` + word + ` v; printf '[%s]' "$v"`
			for _, empty := range []Answer{Yes, No} {
				if got, st := attributeWordRun(t, src, empty); got != "[kept]" || st != 0 {
					t.Errorf("empty=%v: got %q/%d, want %q/0", empty, got, st, "[kept]")
				}
			}
		})
	}
}

// The empty an attribute word brings into being is the *declaration's* and not
// an assignment's, which only a child can see: a name exported with no value
// of its own reaches no command at all, and a second attribute word over it
// hands the same name over empty.
//
// That distinction already existed for the declaration builtins — see
// Runner.declaredEmpty — and it is the half that would have gone missing had
// the fix been a plain store. A `make`-style wrapper that exports the names it
// means to fill in later is exactly this shape.
func TestAnExportedNameADeclarationEmptiedReachesNoChild(t *testing.T) {
	const child = `; /usr/bin/env | /usr/bin/grep '^V' || printf '(none)'`
	if got, st := attributeWordRun(t, `export V`+child, Yes); got != "(none)" || st != 0 {
		t.Errorf("the declaration's own empty: got %q/%d, want %q/0", got, st, "(none)")
	}
	if got, st := attributeWordRun(t, `export V; export V`+child, Yes); got != "V=\n" || st != 0 {
		t.Errorf("a second attribute word owns it: got %q/%d, want %q/0", got, st, "V=\n")
	}
	// And the word the freeze is spelled with owns it too, which is what says
	// the ownership is about naming an attribute rather than about `export`.
	if got, st := attributeWordRun(t, `export V; readonly V`+child, Yes); got != "V=\n" || st != 0 {
		t.Errorf("the freeze owns it as well: got %q/%d, want %q/0", got, st, "V=\n")
	}
}

// The freeze still lands on the name the word brought into being, which is the
// half that was never wrong and is the reason the wrong half was invisible: a
// later write is refused either way, so only the expansion that separates set
// from unset could see it.
func TestAFrozenNameWithNoValueIsStillFrozen(t *testing.T) {
	for _, empty := range []Answer{Yes, No} {
		got, _ := attributeWordRun(t, `readonly v; v=late; printf '[%s]' "${v-UNSET}"`, empty)
		if got == "[late]" {
			t.Errorf("empty=%v: the write was not refused, got %q", empty, got)
		}
	}
}

// And the two words read an unanswered vector rather than refusing it, which
// `typeset` does not.
//
// `export name` and `readonly name` are POSIX's own spellings and every shell
// in the panel runs both, so a vector that has chosen nothing must still be
// able to run a line the standard mandates — the bargain CoreSemantics already
// strikes for `getopts` and the one BackgroundJobInput strikes for `&`. The
// reading it takes is the one four of the five columns give: the name is left
// unset. `typeset` is nobody's standard and asks.
func TestOnlyTheStandardAttributeWordsReadAnUnansweredVector(t *testing.T) {
	unanswered := func(s *Semantics) { s.DeclaredNameWithoutValueIsEmpty = Unspecified }
	for _, word := range []string{"export", "readonly"} {
		t.Run(word, func(t *testing.T) {
			src := word + ` v; printf '[%s]' "${v-UNSET}"`
			if got, st := axisRun(t, src, unanswered); got != "[UNSET]" || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, "[UNSET]")
			}
		})
	}
	got, _ := axisRun(t, `typeset v; printf '[%s]' "${v-UNSET}"`, unanswered)
	if !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("a declaration word that is nobody's standard should refuse, got %q", got)
	}
}
