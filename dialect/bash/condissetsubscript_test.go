// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `[[ -v a[k] ]]` reads the subscript the script *wrote*, so a bracket that
// was quoted or that arrived out of an expansion is part of the key rather
// than the end of the subscript.
//
// Measured 2026-09-16 on bash 5.3.20 and on the same binary as `sh`, each
// probe from a script file with standard input on /dev/null, against an
// associative array holding the keys `x]` and `q[r`. zsh 5.9.2 and ksh93u+
// answer unset on every row below that this answers set, and bash 3.2 has
// neither the operator nor the array — see
// interp.Semantics.ConditionIsSetReadsTheWrittenSubscript.
func TestTheIsSetConditionReadsTheSubscriptTheScriptWrote(t *testing.T) {
	const setup = `declare -A a; a['x]']=7; a['q[r']=8; key='x]'; k2='q[r'; `
	for _, c := range []struct{ name, expr, want string }{
		// A bracket inside quotes is content, so the subscript runs to the
		// bracket the script wrote after the closing quote.
		{"double quoted", `a["x]"]`, "SET"},
		{"single quoted", `a['x]']`, "SET"},
		{"backslash quoted", `a[x\]]`, "SET"},
		// So is one that comes out of an expansion, at either end.
		{"from a value, closing", `a[$key]`, "SET"},
		{"from a value, opening", `a[$k2]`, "SET"},
		{"from a substitution", `a[$(printf 'x]')]`, "SET"},
		// The two controls, and they are what makes this a rule about
		// written brackets rather than about the last bracket in the word:
		// the first closes its subscript and then has a bracket nobody
		// opened, the second opens one nobody closes. bash refuses both.
		{"a written bracket closes it", `a[x]]`, "UNSET"},
		{"a written bracket left open", `a[q[r]`, "UNSET"},
		// And the ordinary subscript is untouched.
		{"a plain present key", `a[x]`, "UNSET"},
		{"a plain absent key", `a[zz]`, "UNSET"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := setup + `[[ -v ` + c.expr + ` ]] && echo SET || echo UNSET`
			if out, st := answersRun(t, src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", src, out, st, c.want)
			}
		})
	}
}

// The `test` builtin is the row where bash's own two spellings of this
// operator part, and it is not an oversight on either side: `test` is given
// one word that has already been expanded, so there is no written bracket
// left in it to read.
//
// Measured on bash 5.3.20 from a script file: `[[ -v a[$key] ]]` is set and
// `test -v "a[$key]"` is unset, with `key='x]'`. Asserted as the *pair*,
// because what would break it is exactly the fix that made the condition
// right — routing the builtin through the same reading.
func TestTheTestBuiltinDoesNotSeeTheWrittenSubscript(t *testing.T) {
	const setup = `declare -A a; a['x]']=7; key='x]'; `
	cond := setup + `[[ -v a[$key] ]] && echo SET || echo UNSET`
	builtin := setup + `test -v "a[$key]" && echo SET || echo UNSET`
	if out, st := answersRun(t, cond); out != "SET\n" || st != 0 {
		t.Errorf("the condition = %q status %d, want SET at 0", out, st)
	}
	if out, st := answersRun(t, builtin); out != "UNSET\n" || st != 0 {
		t.Errorf("the builtin = %q status %d, want UNSET at 0", out, st)
	}
}

// Every other operand the condition takes still goes the way it went: the
// reading above applies to a subscripted name and to nothing else.
func TestTheWrittenSubscriptReadingLeavesPlainOperandsAlone(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=1; [[ -v x ]] && echo SET || echo UNSET`, "SET"},
		{`x=; [[ -v x ]] && echo SET || echo UNSET`, "SET"},
		{`[[ -v nope ]] && echo SET || echo UNSET`, "UNSET"},
		{`[[ -v "a b" ]] && echo SET || echo UNSET`, "UNSET"},
		{`[[ -v "" ]] && echo SET || echo UNSET`, "UNSET"},
		{`b=(9 8 7); [[ -v b[1] ]] && echo SET || echo UNSET`, "SET"},
		{`b=(9 8 7); [[ -v b[9] ]] && echo SET || echo UNSET`, "UNSET"},
		{`b=(9 8 7); i=2; [[ -v b[$i] ]] && echo SET || echo UNSET`, "SET"},
		{`b=(9 8 7); [[ -v b[1+1] ]] && echo SET || echo UNSET`, "SET"},
		// A word that is nothing but a subscript names nothing.
		{`[[ -v [1] ]] && echo SET || echo UNSET`, "UNSET"},
	} {
		if out, st := answersRun(t, c.src); out != c.want+"\n" || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The axis this dialect answers, pinned where the rest of them are pinned.
func TestThisDialectReadsTheWrittenSubscript(t *testing.T) {
	if got := bash.Semantics().ConditionIsSetReadsTheWrittenSubscript; got != interp.Yes {
		t.Errorf("ConditionIsSetReadsTheWrittenSubscript = %v, want %v", got, interp.Yes)
	}
}

// An associative subscript written inside an arithmetic expression is a
// quoting context here, exactly as `${m['k']}` is.
//
// The two spellings reach this shell in different shapes — a parameter
// expansion's subscript is a word with its quoting recorded per span, an
// arithmetic one is text, because the expression is expanded before it is
// parsed — so the question has to be asked twice, and it was only being asked
// once. Measured 2026-09-16 on `declare -A a; a[q]=7`: bash 5.3.20 and
// ksh93u+ answer 7 for every row below and zsh 5.9.2 answers 0, which is
// interp.Semantics.SubscriptIsAQuotingContext.
func TestAnArithmeticSubscriptIsAQuotingContext(t *testing.T) {
	const setup = `declare -A a; a[q]=7; a['a b']=9; `
	for _, c := range []struct{ name, src, want string }{
		{"bare", setup + `(( r = a[q] )); echo $r`, "7"},
		{"single quoted", setup + `(( r = a['q'] )); echo $r`, "7"},
		{"double quoted", setup + `(( r = a["q"] )); echo $r`, "7"},
		{"backslash", setup + `(( r = a[\q] )); echo $r`, "7"},
		{"a quoted blank", setup + `(( r = a['a b'] )); echo $r`, "9"},
		// A quoted *bracket* is deliberately not here. It is a defect one
		// stage earlier — the arithmetic parser ends the subscript at the
		// first `]` whether or not it is quoted, so `a[']']` never reaches
		// this reading at all — and it is #3302, not this.
		{"through a dollar-arithmetic", setup + `echo $(( a['q'] ))`, "7"},
		// The store takes the same key as the read, which is what keeps
		// `(( a['k'] = 5 ))` from putting an element where nothing can
		// reach it.
		{"assigned", `declare -A a; (( a['k'] = 5 )); echo "${a[k]-U}"`, "5"},
		{"assigned then read", `declare -A a; (( a['k'] = 5 )); (( r = a['k'] )); echo $r`, "5"},
		// A key that was never quoted is untouched, and so is an indexed
		// subscript, which is an expression and not a key at all.
		{"an absent key", setup + `(( r = a[zz] )); echo $r`, "0"},
		{"indexed stays arithmetic", `b=(9 8 7); (( r = b[1+1] )); echo $r`, "7"},
		{"indexed through a name", `b=(9 8 7); i=1; (( r = b[i] )); echo $r`, "8"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
