// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// `eval` takes a `name=( … )` operand, and what it takes is the assignment
// written out again rather than a declaration this shell performs.
//
// Measured 2026-09-20 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, `<word> foo=(1 2)` followed by `echo OK`, one word per file:
//
//	word        bash 5.3.20            ksh93u+        dash 0.5.12
//	eval        OK                     syntax error   syntax error
//	typeset     OK                     OK             —
//	export      OK                     OK             —
//	readonly    OK                     OK             —
//	echo        syntax error           syntax error   syntax error
//	command     syntax error           syntax error   syntax error
//	printf      syntax error           syntax error   syntax error
//	:           syntax error           syntax error   syntax error
//	set         syntax error           syntax error   syntax error
//	unset       syntax error           syntax error   syntax error
//	a function  syntax error           syntax error   syntax error
//	/bin/echo   syntax error           syntax error   syntax error
//
// so the reading belongs to a list of words and not to builtins as a class,
// and this shell refused the whole line — a **parse** failure, which ends the
// script rather than the command.
func TestEvalTakesACompoundAssignmentOperand(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a plain literal", `eval foo=(1 2); declare -p foo`, `declare -a foo=([0]="1" [1]="2")`},
		{"an empty one", `eval foo=(); declare -p foo`, `declare -a foo=()`},
		{
			// The quotes are gone by the time `eval` reads the word, which is
			// what says the operand is rendered rather than passed on as
			// written: a re-quoted `a b` would be one element and is two.
			"a quoted element is not one element",
			`eval foo=(1 "a b"); declare -p foo`,
			`declare -a foo=([0]="1" [1]="a" [2]="b")`,
		},
		{
			// And the elements are expanded **outside**, joined, and the
			// result parsed by `eval`. Both halves are needed: this row says
			// the expansion happened, and the quote row below says where.
			"an element that expands to two words",
			`x="p q"; eval foo=($x); declare -p foo`,
			`declare -a foo=([0]="p" [1]="q")`,
		},
		{"a keyed element", `eval foo=([2]=z [0]=a); declare -p foo`, `declare -a foo=([0]="a" [2]="z")`},
		{
			"the appending spelling",
			`foo=(x); eval foo+=(y z); declare -p foo`,
			`declare -a foo=([0]="x" [1]="y" [2]="z")`,
		},
		{
			"a key holding a separator, into a table",
			`declare -A m; eval m=([a b]=1); declare -p m`,
			`declare -A m=(["a b"]="1" )`,
		},
		{"two operands on one line", `eval foo=(1) bar=(2); declare -p foo bar`, "declare -a foo=([0]=\"1\")\ndeclare -a bar=([0]=\"2\")"},
		// Controls. The declaration utilities keep the reading they had —
		// the operand is a declaration there, performed by the utility — and
		// a word that is on nobody's list is still a syntax error.
		{"a declaration is unchanged", `declare -a q=(1 2); declare -p q`, `declare -a q=([0]="1" [1]="2")`},
		{"and so is export", `export e=(1 2); declare -p e`, `declare -ax e=([0]="1" [1]="2")`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if strings.TrimRight(out, "\n") != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The word `eval` is handed is **text**, so a quote a value was carrying is
// syntax there rather than a character.
//
// This is the row that separates the two readings the table above leaves
// standing. With `x` holding `a"b`, bash 5.3.20 answers
//
//	x='a"b'; eval foo=("$x")    unexpected EOF while looking for matching `"'
//
// which can only happen if the element was expanded outside and its value
// joined into the text `eval` then parsed. Had the word been passed on as
// written, with `$x` expanding inside `eval` instead, the line would have
// stored the three characters and reported 0.
func TestEvalsRejoinedOperandIsTextAndNotAReQuotedWord(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `x='a"b'; eval foo=("$x"); declare -p foo`)
	if !strings.Contains(out, "unexpected EOF") {
		t.Errorf("said %q, want a complaint about an unmatched quote", out)
	}
	if st == 0 {
		t.Errorf("status 0, want a failure")
	}
	// The control, and the reason the row above is about quoting rather than
	// about expansion failing in general: the same shape with nothing to
	// unbalance it stores what it expanded.
	out, st = runBash(t, t.TempDir(), `x='a b'; eval foo=("$x"); declare -p foo`)
	if want := `declare -a foo=([0]="a" [1]="b")`; strings.TrimRight(out, "\n") != want || st != 0 {
		t.Errorf("said %q (status %d), want %q at 0", out, st, want)
	}
}

// A word no shell takes a compound assignment behind is still a syntax error,
// and that is what keeps the list a list.
//
// Measured in the table above. `echo` is the one every column refuses,
// including bash, so a change that admitted the construct behind any command
// word would pass every row of the test above and fail this one.
func TestACompoundOperandBehindAnOrdinaryWordIsStillRefused(t *testing.T) {
	for _, c := range []struct {
		src   string
		reads bool
	}{
		// The words the measurement puts on the list.
		{`eval foo=(1 2)`, true},
		{`declare foo=(1 2)`, true},
		{`typeset foo=(1 2)`, true},
		{`export foo=(1 2)`, true},
		{`readonly foo=(1 2)`, true},
		{`local foo=(1 2)`, true},
		// And the words it does not. `echo` is the one every column in the
		// panel refuses, bash included, so a change that admitted the
		// construct behind any command word would pass every row of the
		// tests above and fail here.
		{`echo foo=(1 2)`, false},
		{`command foo=(1 2)`, false},
		{`printf foo=(1 2)`, false},
		{`: foo=(1 2)`, false},
		{`set foo=(1 2)`, false},
		{`unset foo=(1 2)`, false},
		{`/bin/echo foo=(1 2)`, false},
		{`f(){ :; }
f foo=(1 2)`, false},
		// The word has to be written as itself, which is the rule
		// DeclarationArrayFromTheCommandWord already carries and which a
		// sixth name on the list must not quietly widen.
		{`'eval' foo=(1 2)`, false},
		{`c=eval; $c foo=(1 2)`, false},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, err := syntax.Parse(c.src, bash.Dialect())
			if c.reads && err != nil {
				t.Errorf("refused %q: %v", c.src, err)
			}
			if !c.reads && err == nil {
				t.Errorf("read %q, want a refusal", c.src)
			}
		})
	}
}
