// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `;` written where a command belongs is stepped over here, and this shell
// draws the line in two places zsh does not. Measured 2026-09-07 on ksh93u+,
// `-n` and then a run, over a script file under `env -i` with a scratch HOME:
// dash, bash 5.3, bash 3.2 and bash-as-`sh` refuse every line below.
func TestThisShellStepsOverOneSeparator(t *testing.T) {
	d := ksh.Dialect()
	if d.SeparatorWhereACommandBelongs != syntax.OneSeparatorExceptAfterABar {
		t.Errorf("ksh93 steps over one `;` and not one after a bar, got %v",
			d.SeparatorWhereACommandBelongs)
	}
	if !d.AbsentAndOrOperandIsAnEmptyCommand {
		t.Error("ksh93 puts an empty command where the operand is missing")
	}
	// And not the neighbor, which is the other shell's answer for the same
	// text: `false ||` alone is a syntax error here.
	if d.OpenEndedAndOr {
		t.Error("ksh93 refuses an and-or that simply ends with its operator")
	}
}

// Running them. The `;` is stepped over rather than standing in for anything,
// which `false` shows and `true` hides.
//
//	$ ksh s.sh          # false || ; echo two
//	two
//	$ ksh s.sh          # true || ; echo two
//	(nothing)
func TestTheSeparatorIsSteppedOverHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
		want     int
	}{
		{"false || ; echo two", "two\n", 0},
		{"true || ; echo two", "", 0},
		{"true && ; echo two", "two\n", 0},
		{"false && ; echo two", "", 1},
		{"; echo two", "two\n", 0},
		{"true ; ; echo two", "two\n", 0},
		{"true & ; echo two", "two\n", 0},
		// Nothing runs for the separator itself, so it does not even set a
		// status: `$?` is still the failure before it.
		{`false ; ; echo "st=$?"`, "st=1\n", 0},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if out != tc.out {
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
		if st != tc.want {
			t.Errorf("%q: status = %d, want %d", tc.src, st, tc.want)
		}
	}
}

// Where the separator was the last thing on the line, a command that does
// nothing stands in its place and succeeds — which is this shell's answer and
// not zsh's, and the `||` is the only shape that tells them apart.
//
//	$ ksh s.sh          # false || ; ⏎ echo "st=$?"
//	st=0
//
// zsh answers 1 there, because it drops the operator instead.
func TestAnAbsentOperandSucceedsHere(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"false || ;", 0},
		{"true || ;", 0},
		{"false && ;", 1},
		{"true && ;", 0},
		{"{ false || ; }", 0},
		{"( false || ; )", 0},
		{"if :; then false || ; fi", 0},
	} {
		_, st, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if st != tc.want {
			t.Errorf("%q: status = %d, want %d", tc.src, st, tc.want)
		}
	}
}

// The two limits, both of which separate this shell from zsh. A second
// separator and a bar's are refused, and the refusal names the `;` it found.
//
//	$ ksh -n s.sh       # false || ; ; echo two
//	s.sh: syntax error at line 1: `;' unexpected
//	$ ksh -n s.sh       # echo one | ; cat
//	s.sh: syntax error at line 1: `;' unexpected
func TestTheSecondSeparatorAndTheBarsAreRefusedHere(t *testing.T) {
	for _, src := range []string{
		"false || ; ; echo two\n",
		"echo one | ; cat\n",
		"echo one |\n; cat\n",
	} {
		_, err := syntax.Parse(src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
			continue
		}
		want := "syntax error at line 1: `;' unexpected"
		if src == "echo one |\n; cat\n" {
			want = "syntax error at line 2: `;' unexpected"
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// The newline after the separator ends the and-or here, so what follows is
// the next statement rather than the right-hand side. Measured — and it is
// the shape that separates the two lenient shells a third time, after the
// count and the bar.
//
//	$ ksh s.sh          # true || ; ⏎ echo two
//	two
//	$ zsh s.sh
//	(nothing)
func TestANewlineAfterTheSeparatorEndsTheAndOrHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
	}{
		{"true || ;\necho two", "two\n"},
		{"true && ;\necho two", "two\n"},
		// A newline *before* it is stepped over as it always was, so the
		// command after the `;` is the right-hand side and the `true`
		// short-circuits past it.
		{"true ||\n; echo two", ""},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if out != tc.out {
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
	}
}

// And at the head of a compound body, which is its own position in the
// grammar: the five compounds that have one all take it here and are refused
// by dash and every bash column.
func TestASeparatorAtTheHeadOfACompoundBodyHere(t *testing.T) {
	for _, src := range []string{
		"{ ; echo two; }",
		"( ; echo two )",
		"if :; then ; echo two; fi",
		"while :; do ; echo two; break; done",
		"case x in x) ; echo two;; esac",
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{}, src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if out != "two\n" {
			t.Errorf("%q: out = %q, want %q", src, out, "two\n")
		}
		if st != 0 {
			t.Errorf("%q: status = %d, want 0", src, st)
		}
	}
}
