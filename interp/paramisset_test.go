// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// isSetGrammar turns on the one flag `-v` needs, by name rather than by
// naming a shell that happens to have it.
func isSetGrammar(d *syntax.Dialect) { d.ParameterIsSetTest = true }

// isSetRunner gives the Runner the same flag. It is needed as well as the
// parse dialect because the two constructs reach it by different routes: the
// condition's `-v` is a *grammar* question answered while parsing, and the
// `[` builtin's is asked of the running shell, which reads its own Dialect.
// A test that set only the first had `[[ -v x ]]` answering and `[ -v x ]`
// refusing by name in the same script.
func isSetRunner(pos, spec bool) func(*interp.Runner) {
	return func(r *interp.Runner) {
		d := syntax.Core()
		isSetGrammar(&d)
		r.Dialect = &d
		r.Semantics.ParameterIsSetSeesPositionals = pos
		r.Semantics.ParameterIsSetSeesSpecials = spec
	}
}

// `-v` asks whether a parameter is set and never anything about its value.
//
// These rows are unanimous across bash 5.3.15, ksh93u+ and zsh 5.9.2 —
// measured 2026-09-07 under `-c` — so they are asserted here, against the
// flag, rather than three times over in the dialect packages.
func TestAParameterIsSetTestAsksAboutTheParameter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a set name", `x=1; [[ -v x ]] && echo SET || echo UNSET`, "SET\n"},
		{"set to empty is still set", `x=; [[ -v x ]] && echo SET || echo UNSET`, "SET\n"},
		{"never assigned", `[[ -v nope ]] && echo SET || echo UNSET`, "UNSET\n"},
		{"unset again", `x=1; unset x; [[ -v x ]] && echo SET || echo UNSET`, "UNSET\n"},
		{"negated", `[[ ! -v nope ]] && echo UNSET || echo SET`, "UNSET\n"},
		// The operand is an ordinary word, so it may come out of a variable.
		{"the name out of a variable", `x=1; n=x; [[ -v $n ]] && echo SET || echo UNSET`, "SET\n"},
		{"quoted is the same name", `x=1; [[ -v 'x' ]] && echo SET || echo UNSET`, "SET\n"},
		// A name that is not one is unset rather than an error, in all three.
		{"the empty name", `[[ -v "" ]] && echo SET || echo UNSET`, "UNSET\n"},
		{"a name with a blank in it", `[[ -v "a b" ]] && echo SET || echo UNSET`, "UNSET\n"},
		{"a set name with a blank after it", `x=1; [[ -v "x " ]] && echo SET || echo UNSET`, "UNSET\n"},
		// `@` is unset in every shell that has the operator, with parameters
		// set and in the one that answers for every other punctuation name,
		// so it is excluded outright rather than by an axis.
		{"@ with parameters set", `set -- a b; [[ -v @ ]] && echo SET || echo UNSET`, "UNSET\n"},
		// A positional past the end is unset wherever positionals are read
		// at all, so it needs no axis either.
		{"a positional past the end", `set -- p q; [[ -v 3 ]] && echo SET || echo UNSET`, "UNSET\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runGrammar(t, tc.src, isSetGrammar, isSetRunner(true, false))
			if got != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, tc.want)
			}
		})
	}
}

// The `[` builtin asks the same question and has to give the same answer:
// every row of the table above holds for `[ -v x ]` in all three shells that
// have the operator, so the two constructs share one function rather than
// each having one of its own. Asserted as the pair, which is what a second
// implementation would break.
func TestTheTestBuiltinAndTheConditionAgreeAboutIsSet(t *testing.T) {
	for _, src := range []string{
		`x=1`, `x=`, `unset x`, `x=1; unset x`,
	} {
		cond := src + `; [[ -v x ]] && echo SET || echo UNSET`
		builtin := src + `; [ -v x ] && echo SET || echo UNSET`
		a, _ := runGrammar(t, cond, isSetGrammar, isSetRunner(true, false))
		b, _ := runGrammar(t, builtin, isSetGrammar, isSetRunner(true, false))
		if a != b {
			t.Errorf("%s: `[[ -v x ]]` said %q and `[ -v x ]` said %q", src, a, b)
		}
	}
}

// Without the flag the builtin keeps the refusal by name it already gave,
// rather than answering a question this shell does not have. That is the
// honest half of #1255 and it was the behavior before the change; it is
// asserted here because nothing else would notice the gate going away — the
// operand would simply be read as a filename and the test would answer false,
// quietly, which is the wrong kind of no.
// Both routes into the builtin, because there are two and they are gated
// separately: an expression of exactly two words reaches unaryTest straight
// from testExpr with the operator table never consulted, and a longer one is
// read by the parser, which asks isTestUnary first. A test of the short form
// alone leaves the long form's gate unasserted — measured by mutation, where
// removing it changed no answer.
func TestWithoutTheFlagTheTestBuiltinRefusesIsSetByName(t *testing.T) {
	for _, src := range []string{
		`x=1; [ -v x ] && echo SET || echo UNSET`,
	} {
		got, st := run(t, src, nil)
		if !strings.Contains(got, "-v: unary operator expected") {
			t.Errorf("%s: got %q, want a refusal naming -v", src, got)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0 — the refusal is the `[`'s and the `||` arm ran", src, st)
		}
	}
}

// And the multi-term route, which is a different entrance: an expression of
// exactly two words reaches unaryTest straight from testExpr, while a longer
// one is read by the parser, which consults the operator table first. Both
// have to know about `-v` or the two forms disagree in the same shell.
func TestBothRoutesIntoTheTestBuiltinKnowIsSet(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=1; [ -v x ] && echo SET || echo UNSET`, "SET\n"},
		{`x=1; [ -v x -a -n x ] && echo SET || echo UNSET`, "SET\n"},
		{`x=1; [ -n x -a -v x ] && echo SET || echo UNSET`, "SET\n"},
		{`[ -v nope -o -v nope ] && echo SET || echo UNSET`, "UNSET\n"},
	} {
		got, st := runGrammar(t, tc.src, isSetGrammar, isSetRunner(true, false))
		if got != tc.want || st != 0 {
			t.Errorf("%s: got %q/%d, want %q/0", tc.src, got, st, tc.want)
		}
	}
}

// A subscripted name goes through the same lookup `${name+word}` uses, which
// is what keeps an element, a key and the base an index counts from out of
// this file. The rows are the ones that would move if it did not.
func TestAnIsSetTestReadsASubscriptTheSameWayAnExpansionDoes(t *testing.T) {
	arrays := func(d *syntax.Dialect) {
		isSetGrammar(d)
		d.ArrayLiteral = true
		d.ArraySubscript = true
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an element that is there", `a=(1 2 3); [[ -v "a[1]" ]] && echo SET || echo UNSET`, "SET\n"},
		{"an element past the end", `a=(1 2 3); [[ -v "a[9]" ]] && echo SET || echo UNSET`, "UNSET\n"},
		{"the array itself", `a=(1 2 3); [[ -v a ]] && echo SET || echo UNSET`, "SET\n"},
		{"and it agrees with the expansion", `a=(1 2 3); [[ -v "a[1]" ]] && echo S1 || echo U1; [[ -n ${a[1]+s} ]] && echo S2 || echo U2`, "S1\nS2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runGrammar(t, tc.src, arrays, isSetRunner(true, false))
			if got != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, tc.want)
			}
		})
	}
}

// The two axes, each asserted in both directions. They are the only places
// the three shells disagree, and in both the parameter is *there* — it is the
// operator declining to read a name of that kind, which is why the control
// row asks the same question through `${name+s}` and gets the other answer.
func TestWhichKindsOfNameAnIsSetTestReads(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		pos, spec bool
		want      string
	}{
		{"a positional, read", `set -- p q; [[ -v 1 ]] && echo SET || echo UNSET`, true, false, "SET\n"},
		{"a positional, not read", `set -- p q; [[ -v 1 ]] && echo SET || echo UNSET`, false, false, "UNSET\n"},
		{"the shell's name, read", `[[ -v 0 ]] && echo SET || echo UNSET`, true, false, "SET\n"},
		{"the shell's name, not read", `[[ -v 0 ]] && echo SET || echo UNSET`, false, false, "UNSET\n"},
		{"a special, read", `[[ -v "?" ]] && echo SET || echo UNSET`, false, true, "SET\n"},
		{"a special, not read", `[[ -v "?" ]] && echo SET || echo UNSET`, false, false, "UNSET\n"},
		{"and the rest of them", `[[ -v "#" ]] && echo A || echo a; [[ -v "$" ]] && echo B || echo b; [[ -v "!" ]] && echo C || echo c; [[ -v "*" ]] && echo D || echo d; [[ -v "-" ]] && echo E || echo e`, false, true, "A\nB\nC\nD\nE\n"},
		{"none of them", `[[ -v "#" ]] && echo A || echo a; [[ -v "$" ]] && echo B || echo b; [[ -v "!" ]] && echo C || echo c; [[ -v "*" ]] && echo D || echo d; [[ -v "-" ]] && echo E || echo e`, false, false, "a\nb\nc\nd\ne\n"},
		// The control that says the parameter is there either way, so the
		// axis is about the operator and not about the lookup.
		{"the parameter is there regardless", `set -- p; [[ -n ${1+s} ]] && echo THERE; [[ -n ${?+s} ]] && echo THERE2`, false, false, "THERE\nTHERE2\n"},
		// And `@` stays unset even where every other punctuation name is
		// read, which is what keeps it out of the axis.
		{"@ is excluded even with specials on", `set -- a b; [[ -v @ ]] && echo SET || echo UNSET`, true, true, "UNSET\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runGrammar(t, tc.src, isSetGrammar, isSetRunner(tc.pos, tc.spec))
			if got != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, tc.want)
			}
		})
	}
}
