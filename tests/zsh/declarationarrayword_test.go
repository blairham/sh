// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `name=( … )` word is an array literal only where a declaration utility's
// operand stands, and the command word has to be one unquoted literal. An
// argument that merely looks like one is an ordinary word here, and this
// shell reads the parenthesis on it as a pattern group or a glob qualifier.
//
// Measured 2026-09-15 and 2026-09-16 on zsh 5.9.2, each probe in a script file
// of its own, `nonomatch` set where a pattern that matches nothing would
// otherwise be complained about:
//
//	print -r -- x=(a|b)c      x=(a|b)c   — one word
//	print -r -- x=(a)         number expected — a glob qualifier
//	'typeset' a=(x y)         unknown file attribute: — a glob qualifier
//	f() { local a=(x y); print -r ${#a}; }; f   2
//	typeset -a b=(p q r); print -r ${#b}        3
//
// The last two are what the guard must not lose and were right all along; the
// first three were `parse error near `('` here, because the array reading was
// offered wherever an assignment could not stand rather than where a
// declaration's operand does (#3087, #3351).
func TestAnArrayOperandNeedsAnUnquotedDeclarationWord(t *testing.T) {
	d := zsh.Dialect()
	if got := d.DeclarationArrayFromTheCommandWord; got != syntax.DeclarationArrayFromAnUnquotedLiteralWord {
		t.Fatalf("DeclarationArrayFromTheCommandWord = %v, want the unquoted-literal reading", got)
	}

	// The two that are arrays, and the two the guard exists for.
	for _, tc := range []struct{ src, want string }{
		{"f() { local a=(x y); print -r ${#a}; }; f", "2\n"},
		{"typeset -a b=(p q r); print -r ${#b}", "3\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, tc.want)
		}
	}

	// An argument is not one, and the word keeps its group: one word comes
	// out, with the parenthesis and the text after it still in it.
	out, _, err := preset.Combined(t, dialecttest.Base{},
		"setopt nonomatch\nprint -r -- x=(a|b)c")
	if err != nil || out != "x=(a|b)c\n" {
		t.Errorf("`print -r -- x=(a|b)c`: out = %q, err = %v, want %q", out, err, "x=(a|b)c\n")
	}

	// And a quoted command word is not a declaration, so its operand is an
	// ordinary word with a trailing glob qualifier on it — which this shell
	// then refuses, `a=` matching nothing and ` ` naming no attribute.
	for _, src := range []string{
		"'typeset' a=(x y)",
		"\\typeset a=(x y)",
		"type\"set\" a=(x y)",
		"cmd=typeset; $cmd a=(x y)",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		out, st, _ := preset.Combined(t, dialecttest.Base{}, src)
		if st == 0 || !strings.Contains(out, "file attribute") {
			t.Errorf("%q: out = %q at %d, want a glob qualifier refused", src, out, st)
		}
	}
}
