// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A subscript that reaches `[[ -v ]]` as text is expanded once more before it
// is looked up, and this shell writes no bracket of its own into that text:
// it has no written-subscript reading, so every spelling of the operand takes
// the round.
//
// Measured 2026-09-19 on zsh 5.9.2, each probe from a script file with
// standard input on /dev/null, over `typeset -A m; k='x y'; kk='$k'; m[$k]=V`.
// bash 5.3.20 answers unset on the two rows whose bracket it reads as written
// — see interp.Semantics.ConditionIsSetExpandsAFlatSubscript (#3298).
func TestTheIsSetConditionExpandsASubscriptThatReachedItAsText(t *testing.T) {
	const setup = `typeset -A m; k='x y'; kk='$k'; m[$k]=V; w='m[$k]'; `
	for _, c := range []struct{ name, expr, want string }{
		{"single quoted operand", `'m[$k]'`, "SET"},
		{"out of a value", `$w`, "SET"},
		// The two rows where the other shell that expands stops at the one
		// expansion its written bracket already had. Here the `$kk` becomes
		// `$k` and the `$k` then becomes the key `x y`.
		{"a written bracket", `m[$kk]`, "SET"},
		{"a bracket in double quotes", `"m[$kk]"`, "SET"},
		// The controls: nothing to expand reaches the same key either way,
		// and an absent key is still absent.
		{"a plain present key", `'m[x y]'`, "SET"},
		{"a plain absent key", `'m[zz]'`, "UNSET"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := setup + `[[ -v ` + c.expr + ` ]] && echo SET || echo UNSET`
			if out, st := answersRun(t, src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", src, out, st, c.want)
			}
		})
	}
}

// A declaration's operand gets the same second round, over an indexed array
// as well as a table.
//
// Measured 2026-09-19 on zsh 5.9.2 from a script file — see
// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
func TestADeclarationOperandExpandsASubscriptThatReachedItAsText(t *testing.T) {
	const keys = `; for q in "${(@k)d}"; do printf '[%s]' "$q"; done`
	for _, c := range []struct{ name, src, want string }{
		{"typeset over a table", `typeset -A d; k='x y'; typeset 'd[$k]'=Q` + keys, "[x y]"},
		{"declare over a table", `typeset -A d; k='x y'; declare 'd[$k]'=Q` + keys, "[x y]"},
		{
			"the operand out of a value",
			`typeset -A d; k='x y'; kk='$k'; typeset "d[$kk]"=Q` + keys,
			"[x y]",
		},
		{
			"an indexed element",
			`i=3; typeset -a a=(z z z z); typeset 'a[$i]'=Q; printf '[%s][%s]' "${a[3]}" "${a[2]}"`,
			"[Q][z]",
		},
		{"nothing to expand", `typeset -A d; typeset 'd[k]'=Q` + keys, "[k]"},
		{"the brackets written plainly", `typeset -A d; k='x y'; typeset d[$k]=Q` + keys, "[x y]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The two axes this dialect answers, pinned where the rest of them are.
func TestThisDialectExpandsASubscriptThatArrivedAsText(t *testing.T) {
	if got := zsh.Semantics().ConditionIsSetExpandsAFlatSubscript; got != interp.Yes {
		t.Errorf("ConditionIsSetExpandsAFlatSubscript = %v, want %v", got, interp.Yes)
	}
	if got := zsh.Semantics().DeclarationOperandExpandsItsSubscript; got != interp.Yes {
		t.Errorf("DeclarationOperandExpandsItsSubscript = %v, want %v", got, interp.Yes)
	}
}

// The builtins' surfaces, where this shell agrees with the column that has a
// name for them everywhere but `unset`.
//
// That one row is the reason the group is three axes rather than one.
// Measured 2026-09-19 on zsh 5.9.2, each probe from a script file with
// standard input on /dev/null, over `typeset -A m; k='x y'; m[$k]=hello`:
// `unset 'm[$k]'` leaves the element standing, while `read 'm[$k]'` and
// `printf -v 'm[$k]'` write the key `x y` and `test -v 'm[$k]'` finds it. The
// control is the same operand with nothing in it to expand — `unset
// 'm[plain]'` does take the element away — which says the survival is the
// round not happening rather than the quoted brackets not reaching `unset`.
// See interp.Semantics.UnsetExpandsAFlatSubscript (#3298).
func TestABuiltinsOperandRoundsExceptAtUnset(t *testing.T) {
	const table = `typeset -A m; k='x y'; m[$k]=hello; `
	const keys = `; for q in "${(@k)m}"; do printf '[%s]' "$q"; done`
	for _, c := range []struct{ name, src, want string }{
		{"unset leaves the element standing", table + `unset 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`, "[hello]"},
		{"and with the letter", table + `unset -v 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`, "[hello]"},
		{"but takes one it can name", `typeset -A m; m[plain]=hello; unset 'm[plain]'; printf '[%s]' "${m[plain]-GONE}"`, "[GONE]"},
		{"read writes the expanded key", `typeset -A m; k='x y'; read 'm[$k]' <<<Z` + keys, "[x y]"},
		{"printf writes the expanded key", `typeset -A m; k='x y'; printf -v 'm[$k]' P` + keys, "[x y]"},
		{"the is-set builtin finds it", table + `test -v 'm[$k]' && echo SET || echo UNSET`, "SET\n"},
		{"and its bracket spelling", table + `[ -v 'm[$k]' ] && echo SET || echo UNSET`, "SET\n"},
		{"a key that is not there", table + `test -v 'm[nope]' && echo SET || echo UNSET`, "UNSET\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The three the builtins read, pinned where the rest of them are — and this
// shell parts from the one that names the group at exactly one of them.
func TestThisDialectRoundsABuiltinsSubscriptTextExceptAtUnset(t *testing.T) {
	s := zsh.Semantics()
	if got := s.UnsetExpandsAFlatSubscript; got != interp.No {
		t.Errorf("UnsetExpandsAFlatSubscript = %v, want %v", got, interp.No)
	}
	if got := s.OutputOperandExpandsAFlatSubscript; got != interp.Yes {
		t.Errorf("OutputOperandExpandsAFlatSubscript = %v, want %v", got, interp.Yes)
	}
	if got := s.TestIsSetExpandsAFlatSubscript; got != interp.Yes {
		t.Errorf("TestIsSetExpandsAFlatSubscript = %v, want %v", got, interp.Yes)
	}
}
