// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `;` written where a command belongs is stepped over here, anywhere and as
// many as are written — which is the wider of the two answers. Measured
// 2026-09-07 on zsh 5.9.2 over a script file under `env -i` with a scratch
// HOME, ZDOTDIR and HISTFILE; dash and all three bash columns refuse them.
func TestThisShellStepsOverAnySeparator(t *testing.T) {
	d := zsh.Dialect()
	if d.SeparatorWhereACommandBelongs != syntax.AnySeparatorWhereACommandBelongs {
		t.Errorf("zsh steps over any number of them, anywhere, got %v",
			d.SeparatorWhereACommandBelongs)
	}
	// And not the other shell's answer for a missing operand: this one drops
	// the operator, which is what OpenEndedAndOr already says.
	if d.AbsentAndOrOperandIsAnEmptyCommand {
		t.Error("zsh drops the operator rather than putting a command there")
	}
}

// Running them, including the two shapes ksh93 refuses: a second separator,
// and one after a bar.
func TestTheSeparatorIsSteppedOverHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
		want     int
		path     bool
	}{
		{"false || ; echo two", "two\n", 0, false},
		{"true || ; echo two", "", 0, false},
		{"true && ; echo two", "two\n", 0, false},
		{"false && ; echo two", "", 1, false},
		{"; echo two", "two\n", 0, false},
		{"true ; ; echo two", "two\n", 0, false},
		{"true & ; echo two", "two\n", 0, false},
		{`false ; ; echo "st=$?"`, "st=1\n", 0, false},
		// A second one, and one after a bar — both `` `;' unexpected `` in
		// ksh93, which is what makes the count and the bar two measured
		// differences rather than one rule.
		{"false || ; ; echo two", "two\n", 0, false},
		{"echo one | ; cat", "one\n", 0, true},
		{"echo one | ; ; cat", "one\n", 0, true},
		{"echo one |\n; cat", "one\n", 0, true},
	} {
		// The bar rows need a real command on the far side of the pipe, so
		// that what is asserted is the *pipe carrying the line* and not only
		// that the text parsed.
		base := dialecttest.Base{}
		if tc.path {
			base.Env = []string{"PATH=/usr/bin:/bin"}
		}
		out, st, err := preset.Combined(t, base, tc.src)
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

// Where the separator was the last thing before a closer, the operator is
// dropped and the status is the left-hand side's — not the empty command that
// succeeds in ksh93. `false || ;` is the shape that separates the two
// readings, and this shell answers 1 where that one answers 0.
func TestAnAbsentOperandKeepsTheLeftSideHere(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"{ false || ; }", 1},
		{"( false || ; )", 1},
		{"if :; then false || ; fi", 1},
		{"{ true && ; }", 0},
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

// The newline after the separator is stepped over here, so what follows is
// the right-hand side and a successful left-hand side short-circuits past it.
// ksh93 stops at the newline and prints `two`, which is the third place the
// two lenient shells part.
func TestANewlineAfterTheSeparatorIsSteppedOverHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
	}{
		{"true || ;\necho two", ""},
		{"true && ;\necho two", "two\n"},
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
