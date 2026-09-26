// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// readArrayDefault runs src with the `-A` spelling of the array letter and
// one axis under the test's control: what that letter fills when the line
// names no parameter at all.
//
// Everything the rows could otherwise be reading is answered flat. In
// particular ReadNoFieldsIsOneEmptyElement is No here, so an array a read
// filled with nothing has **no** elements — which is what lets a row tell
// "the builtin filled it" from "the builtin left it alone". The rows that
// pre-set a name do it with data the read could not have produced, for the
// same reason.
func readArrayDefault(t *testing.T, src string, style interp.ReadArrayDefaultStyle) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
	}, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ReadOptions = "rA"
		sem.LastPipelineElementInCurrentShell = interp.Yes
		sem.TrailingSeparatorEndsAField = interp.No
		sem.ReadTrailingWhitespaceEndsAField = interp.No
		sem.ReadNoFieldsIsOneEmptyElement = interp.No
		sem.UnsetNameAtIsOneEmptyField = interp.No
		// A bare `read` with no operand: three shells fill REPLY and dash
		// wants a name. Answered flat as the two shells with the array
		// letter answer it, because the contrast between that rule and this
		// axis is what several rows below are about.
		sem.ReadRequiresAVariableName = interp.No
		sem.ReadArrayDefault = style
		r.Semantics = &sem
	})
}

// showReply prints the two parameters the question is between: the array
// `reply` by element count and contents, and the scalar `REPLY` whole.
//
// Both on every row on purpose. The rule is keyed on the **parameter**, and a
// row that read back only the one it expected to be filled could not tell a
// shell that filled the other one from a shell that filled nothing.
const showReply = `printf 'n=%s' "${#reply[@]}"; for e in "${reply[@]}"; do printf '[%s]' "$e"; done; printf ' REPLY=[%s]\n' "$REPLY"`

// `read -A` with no name fills the array `reply` where the dialect says so.
//
// The bug: the default was the scalar spelling `REPLY`, filled as an array —
// a parameter zsh does not create here at all — so a script's `$reply` kept
// whatever it held before and the status was the 0 of a line that arrived
// (#4593). Measured 2026-09-26 on zsh 5.9.2 under `-f`: `reply=(x y); read -A
// <<<'hello world'` leaves `typeset -a reply=( hello world )` and `typeset -p
// REPLY` saying no such variable.
func TestTheArrayLetterWithNoNameFillsTheDialectsOwnArray(t *testing.T) {
	src := `printf 'hello world\n' | { read -A; ` + showReply + `; }`
	out, _ := readArrayDefault(t, src, interp.ReadArrayDefaultIsTheArrayReply)
	if want := "n=2[hello][world] REPLY=[]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The other reading: the letter with no name to apply to is not an array at
// all, and the builtin reads as a bare `read` does.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01 from a script file: `read -A
// <<<'a b c'` leaves `REPLY='a b c'` — one scalar, `${#REPLY[@]}` at 1 — and
// a pre-set `reply` untouched. `reply` is zsh's parameter and not one ksh93
// has ever heard of.
func TestTheArrayLetterWithNoNameCanBeAPlainRead(t *testing.T) {
	src := `printf 'hello world\n' | { read -A; ` + showReply + `; }`
	out, _ := readArrayDefault(t, src, interp.ReadArrayDefaultIsAPlainRead)
	if want := "n=0 REPLY=[hello world]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The two readings write to two different parameters, so neither can be
// mistaken for the other by a script that reads back the one it expects.
//
// This is the row that says the rule is keyed on the **parameter** rather
// than on the array-ness: the reading that fills `reply` leaves `REPLY`
// empty, and the reading that fills `REPLY` leaves `reply` with no elements,
// with both names pre-set to data neither read could have produced.
func TestTheTwoArrayDefaultsDoNotOverwriteEachOther(t *testing.T) {
	for _, c := range []struct {
		name  string
		style interp.ReadArrayDefaultStyle
		want  string
	}{
		{"the array", interp.ReadArrayDefaultIsTheArrayReply, "n=2[p][q] REPLY=[keptscalar]\n"},
		{"a plain read", interp.ReadArrayDefaultIsAPlainRead, "n=2[keptone][kepttwo] REPLY=[p q]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := `reply=(keptone kepttwo); REPLY=keptscalar
printf 'p q\n' | { read -A; ` + showReply + `; }`
			out, _ := readArrayDefault(t, src, c.style)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The failing read, which is the control the two rows above need: an
// instrument that reads a name back after the builtin agrees with itself when
// the builtin did nothing and the name was already set.
//
// A `read` that ends at end of input still assigns, so here the pre-set
// elements are gone and the count is the empty one — visibly different from
// the "left alone" both rows above would show if the fill never happened.
func TestAFailingArrayReadWithNoNameStillClearsTheDefault(t *testing.T) {
	src := `reply=(keptone kepttwo)
printf '' | { read -A; printf 'st=%s ' "$?"; ` + showReply + `; }`
	out, _ := readArrayDefault(t, src, interp.ReadArrayDefaultIsTheArrayReply)
	if want := "st=1 n=0 REPLY=[]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A name the script wrote is not this question, and the axis does not reach
// it: `read -A r` fills `r` under either reading.
//
// The control that keeps the axis confined. It is also half of what separates
// the two nouns this rule could be keyed on — here the array-ness is fixed
// and present, and moving the axis moves nothing.
func TestAWrittenArrayNameIsNotTheDefault(t *testing.T) {
	for _, style := range []interp.ReadArrayDefaultStyle{
		interp.ReadArrayDefaultUnspecified,
		interp.ReadArrayDefaultIsTheArrayReply,
		interp.ReadArrayDefaultIsAPlainRead,
	} {
		t.Run(style.String(), func(t *testing.T) {
			src := `reply=(keptone kepttwo)
printf 'p q\n' | { read -A r; printf 'st=%s n=%s' "$?" "${#r[@]}"; for e in "${r[@]}"; do printf '[%s]' "$e"; done; printf ' '; ` + showReply + `; }`
			out, _ := readArrayDefault(t, src, style)
			if want := "st=0 n=2[p][q] n=2[keptone][kepttwo] REPLY=[]\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// The other half: the array-ness held fixed and the *name* varied. `read -A
// reply` and `read -A REPLY` are both written names, both fill an array, and
// each fills the one it names — so the failing case is the defaulting and not
// the spelling, and not the array.
//
// Measured on zsh 5.9.2 under `-f`: `read -A reply <<<'p q'` and `read -A
// REPLY <<<'a b'` each leave the named parameter an array of the line's words
// and the other one alone.
func TestTheTwoSpellingsAreBothFillableByName(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"reply by name",
			`printf 'p q\n' | { read -A reply; ` + showReply + `; }`,
			"n=2[p][q] REPLY=[]\n",
		},
		{
			"REPLY by name",
			`printf 'p q\n' | { read -A REPLY; printf 'n=%s' "${#REPLY[@]}"; for e in "${REPLY[@]}"; do printf '[%s]' "$e"; done; printf ' reply=%s\n' "${#reply[@]}"; }`,
			"n=2[p][q] reply=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := readArrayDefault(t, c.src, interp.ReadArrayDefaultIsTheArrayReply)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The neighboring rule, which is the discriminating contrast: a bare `read`
// with no name fills the **scalar** `REPLY` and never the array, under the
// same dialect whose `-A` fills `reply`. One is `REPLY` and the other is
// `reply`, and the shell is case-sensitive about which.
func TestABareReadStillFillsTheScalarUnderTheArrayDefault(t *testing.T) {
	src := `reply=(keptone kepttwo)
printf 'plain line\n' | { read; ` + showReply + `; }`
	out, _ := readArrayDefault(t, src, interp.ReadArrayDefaultIsTheArrayReply)
	if want := "n=2[keptone][kepttwo] REPLY=[plain line]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// No dialect chosen means no answer, and the substrate says so rather than
// picking one of the two.
func TestAnUnansweredArrayDefaultIsRefused(t *testing.T) {
	out, st := readArrayDefault(t, `read -A`, interp.ReadArrayDefaultUnspecified)
	if st != 2 {
		t.Errorf("status = %d, want the refusal's 2", st)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal naming the disagreement", out)
	}
	if !strings.Contains(out, "read -A") {
		t.Errorf("said %q, want the question named", out)
	}
}

// And the refusal ends the builtin there rather than carrying on into the
// bare read behind it.
//
// The two are the same status and are told apart by what is *said*: a shell
// that fell through would reach the bare-read rule with no operands, and in a
// dialect that wants a name for that it would print a second sentence about
// an argument count — a complaint about the line the script wrote, over a
// question the substrate has already said it cannot answer.
func TestAnUnansweredArrayDefaultDoesNotFallIntoTheBareRead(t *testing.T) {
	out, st := runGrammar(t, `read -A`, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
	}, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ReadOptions = "rA"
		// The other answer to the neighboring rule: a bare `read` with no
		// operand is an error here rather than a fill of REPLY, which is
		// what makes the second sentence visible at all.
		sem.ReadRequiresAVariableName = interp.Yes
		sem.ReadArrayDefault = interp.ReadArrayDefaultUnspecified
		r.Semantics = &sem
	})
	if st != 2 {
		t.Errorf("status = %d, want the refusal's 2", st)
	}
	if strings.Contains(out, "arg count") {
		t.Errorf("said %q, want only the unanswered axis, not the bare read behind it", out)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal naming the disagreement", out)
	}
}
