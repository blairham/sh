// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A subscript that reaches `[[ -v ]]` as text is expanded once more before it
// is looked up, so `k='x y'; [[ -v 'm[$k]' ]]` asks about the key `x y`.
//
// Measured 2026-09-19 on bash 5.3.20, each probe from a script file with
// standard input on /dev/null, over `declare -A m; k='x y'; kk='$k'; m[$k]=V`.
// The same answers with `shopt -s assoc_expand_once` set, which is what says
// this surface is not the option's — see interp.Semantics
// .ConditionIsSetExpandsAFlatSubscript (#3298).
func TestTheIsSetConditionExpandsASubscriptThatReachedItAsText(t *testing.T) {
	const setup = `declare -A m; k='x y'; kk='$k'; m[$k]=V; w='m[$k]'; `
	for _, c := range []struct{ name, expr, want string }{
		// The brackets were quoted, so nothing wrote one: the text is read
		// and the `$k` in it expanded.
		{"single quoted operand", `'m[$k]'`, "SET"},
		// And the whole operand out of a value is the same shape.
		{"out of a value", `$w`, "SET"},
		{"out of a quoted value", `"$w"`, "SET"},
		// A bracket written inside double quotes was still written, so the
		// one expansion the operand had is the subscript's and there is no
		// second round: the key is the two characters `$k`.
		{"a bracket in double quotes", `"m[$kk]"`, "UNSET"},
		{"a bracket in double quotes, name outside", `m"[$kk]"`, "UNSET"},
		// So is a bracket written plainly, which the written reading has
		// already answered.
		{"a written bracket", `m[$kk]`, "UNSET"},
		// The controls: a subscript with nothing in it to expand names the
		// same key under either reading, and an absent key is still absent.
		{"a plain present key", `'m[x y]'`, "SET"},
		{"a plain absent key", `'m[zz]'`, "UNSET"},
		// A whole table named through the same quoted spelling is the
		// question `[[ -v m ]]` already answers, and it answers unset here
		// — measured on bash 5.3.20 over a table holding one element. The
		// second round reaches subscripts and nothing else.
		{"a name with no subscript", `'m'`, "UNSET"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := setup + `[[ -v ` + c.expr + ` ]] && echo SET || echo UNSET`
			if out, st := answersRun(t, src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", src, out, st, c.want)
			}
		})
	}
}

// `test -v` takes the same text and does **not** get the second round here,
// which is the row that keeps the two spellings apart.
//
// Measured on bash 5.3.20: `test -v 'm[$k]'` is set by default and unset
// under `shopt -s assoc_expand_once`, while `[[ -v 'm[$k]' ]]` is set under
// both. So the builtin's surface moves with an option this shell does not
// have yet and the condition's does not move at all — routing the builtin
// through the condition's reading would pin one state of that option as this
// dialect's answer. See #3298, which is still open for the option.
func TestTheTestBuiltinTakesNoSecondRoundOverASubscriptText(t *testing.T) {
	const setup = `declare -A m; k='x y'; m[$k]=V; `
	cond := setup + `[[ -v 'm[$k]' ]] && echo SET || echo UNSET`
	builtin := setup + `test -v 'm[$k]' && echo SET || echo UNSET`
	if out, st := answersRun(t, cond); out != "SET\n" || st != 0 {
		t.Errorf("the condition = %q status %d, want SET at 0", out, st)
	}
	if out, st := answersRun(t, builtin); out != "UNSET\n" || st != 0 {
		t.Errorf("the builtin = %q status %d, want UNSET at 0", out, st)
	}
}

// A declaration's operand gets the same second round, in every spelling of
// the utility and over an indexed array as well as a table.
//
// Measured 2026-09-19 on bash 5.3.20 from a script file, and with `shopt -s
// assoc_expand_once` set as well — the key is `x y` either way, which is why
// this is the dialect's answer rather than the option's. See
// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
func TestADeclarationOperandExpandsASubscriptThatReachedItAsText(t *testing.T) {
	const keys = `; for q in "${!d[@]}"; do printf '[%s]' "$q"; done`
	for _, c := range []struct{ name, src, want string }{
		{"declare over a table", `declare -A d; k='x y'; declare 'd[$k]'=Q` + keys, "[x y]"},
		{"typeset over a table", `typeset -A d; k='x y'; typeset 'd[$k]'=Q` + keys, "[x y]"},
		{
			"local over a table",
			`k='x y'; f() { typeset -A d; local 'd[$k]'=Q` + keys + `; }; f`,
			"[x y]",
		},
		{
			"the operand out of a value",
			`declare -A d; k='x y'; kk='$k'; declare "d[$kk]"=Q` + keys,
			"[x y]",
		},
		// The indexed row: the text is expanded and then read as an
		// expression, so a `$i` between the brackets counts to an element
		// rather than ending the declaration.
		{
			"an indexed element",
			`i=3; declare -a a=(z z z z); declare 'a[$i]'=Q; printf '[%s][%s]' "${a[3]}" "${a[2]}"`,
			"[Q][z]",
		},
		// The two controls. A subscript with nothing in it to expand names
		// the same key under either reading, and an operand whose brackets
		// the parser lexed was expanded before any builtin saw it.
		{"nothing to expand", `declare -A d; declare 'd[k]'=Q` + keys, "[k]"},
		// `export` and `readonly` take no such operand at all in this
		// dialect and are not rows of this table: measured on bash 5.3.20,
		// each answers "`d[$k]': not a valid identifier" at 1, which is
		// what this shell already said before there was a second round.
		{"the brackets written plainly", `declare -A d; k='x y'; declare d[$k]=Q` + keys, "[x y]"},
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
	if got := bash.Semantics().ConditionIsSetExpandsAFlatSubscript; got != interp.Yes {
		t.Errorf("ConditionIsSetExpandsAFlatSubscript = %v, want %v", got, interp.Yes)
	}
	if got := bash.Semantics().DeclarationOperandExpandsItsSubscript; got != interp.Yes {
		t.Errorf("DeclarationOperandExpandsItsSubscript = %v, want %v", got, interp.Yes)
	}
}
