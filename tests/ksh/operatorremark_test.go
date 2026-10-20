// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// This shell's `-n` is a lint mode with rules rather than a parse check that
// happens to warn, and #2409 is its second rule: two operators written with
// no blank between them draw a line about the layout (#1466 is the first).
//
// Measured on 93u+ 2012-08-01 over a script file, 2026-09-12. Every wanted
// string here is a line that shell wrote.

func operatorRemarks(t *testing.T, src string) []string {
	t.Helper()
	p := syntax.NewParser(src, ksh.Dialect())
	p.Parse()
	d := ksh.Diagnostics()
	var out []string
	for _, r := range p.Remarks() {
		if msg := d.Remark(r); msg != "" {
			out = append(out, msg)
		}
	}
	return out
}

func TestTwoOperatorsRunTogetherAreWorded(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"(:);(:)\n", "warning: line 1: use space or tab to separate operators ; and ("},
		{"echo a&;b\n", "warning: line 1: use space or tab to separate operators & and ;"},
		{"if |; then :; fi\n", "warning: line 1: use space or tab to separate operators | and ;"},
		{"if ;| then :; fi\n", "warning: line 1: use space or tab to separate operators ; and |"},
		{"a|;b\n", "warning: line 1: use space or tab to separate operators | and ;"},
		{":;>>f\n", "warning: line 1: use space or tab to separate operators ; and >"},
		{"echo one\necho two\n(:);(:)\n", "warning: line 3: use space or tab to separate operators ; and ("},
	} {
		got := operatorRemarks(t, c.src)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q said %q, want [%q]", c.src, got, c.want)
		}
	}
}

// The rows this shell reads without a word, which are what say the rule is
// about two operators run together and not about the constructs: a blank
// between them, a pair that is one operator, and anything but an operator
// after it. `$(` is the sharpest — it is silent where a bare `(` is not.
func TestWhatThisShellReadsWithoutAWord(t *testing.T) {
	for _, src := range []string{
		"a | ; b\n", "(:) ; (:)\n", ":;$(:)\n", ":&&(:)\n", ":||(:)\n",
		":;;:\n", ":;&:\n", ":|&:\n", ":;x\n", ":;{ :; }\n", ":;!:\n",
		"#(:);(:)\n", "echo \"(:);(:)\"\n",
	} {
		if got := operatorRemarks(t, src); len(got) != 0 {
			t.Errorf("%q said %q, want nothing", src, got)
		}
	}
}

// One line per occurrence, in order, which is what the shell writes for a
// line holding two of them.
func TestEachOccurrenceIsWordedOnItsOwn(t *testing.T) {
	got := operatorRemarks(t, "(:);(:);(:)\n")
	want := "warning: line 1: use space or tab to separate operators ; and ("
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("said %q, want that line twice", got)
	}
}

// It is held back once the shell is going to run the program, the same as the
// backquote remark and measured the same way: `ksh -n s.sh` writes it and
// `ksh s.sh`, `ksh -c` and `ksh < s.sh` write nothing at all on the same text.
func TestItIsOnlySaidWhenTheShellIsNotRunning(t *testing.T) {
	if !interp.RemarkOnlyWhenNotRunning(syntax.RemarkOperatorsNotSeparated) {
		t.Error("said while running, want it held back")
	}
}

// And no other dialect words it, which is five of the six columns: bash 5.3,
// bash-as-sh, bash 3.2, dash and zsh 5.9.2 all read `(:);(:)` without a word.
func TestOnlyThisDialectWordsIt(t *testing.T) {
	var d interp.Diagnostics
	rk := syntax.Remark{
		Kind: syntax.RemarkOperatorsNotSeparated, Token: ";", Next: "(",
		Pos: syntax.Pos{Line: 1}, At: syntax.Pos{Line: 1},
	}
	if got := d.Remark(rk); got != "" {
		t.Errorf("a dialect with no wording said %q, want silence", got)
	}
}
