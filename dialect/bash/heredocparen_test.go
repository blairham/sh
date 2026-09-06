// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// The parentheses are found first here, so a here-document's body is read
// from what is between them and the delimiter is a delimiter even with the
// `)` written onto its line.
//
//	v=$(cat <<EOF
//	a
//	EOF)
//
// Measured across bash 5.3, bash 3.2 and the same binary called `sh`: all
// three run it and `v` is `a`. dash and zsh read the body from the whole
// input instead, take the `)` into it, and refuse the construct — which is
// the dialect question this side of it answers yes to (#963).
func TestADelimiterMayCarryTheClosingParen(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			name: "the delimiter carries the paren",
			src:  "v=$(cat <<EOF\na\nEOF)\necho \"[$v]\"\n",
			want: "[a]",
		},
		{
			name: "a second here-document carries it",
			src:  "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\necho \"[$v]\"\n",
			want: "[x\ny]",
		},
		{
			name: "the tab-stripping operator",
			src:  "v=$(cat <<-EOF\n\ta\n\tEOF)\necho \"[$v]\"\n",
			want: "[a]",
		},
		{
			name: "parentheses that hold a program without a dollar sign",
			src:  "cat <(cat <<EOF\na\nEOF)\n",
			want: "a",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if got := strings.TrimSpace(out); got != c.want || st != 0 {
				t.Errorf("%q gave %q at %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// And the document is still remarked on, which is what says the body really
// did run out rather than being delimited by `EOF)`. This shell is the only
// one of the four with a sentence for it, so it is the only one where the
// distinction is visible at all.
func TestTheDocumentIsStillRemarkedOnWhenTheParenEndsIt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			name: "the delimiter carries the paren",
			src:  "v=$(cat <<EOF\na\nEOF)\necho \"[$v]\"\n",
			want: "warning: here-document at line 1 delimited by end-of-file (wanted `EOF')",
		},
		{
			name: "a second here-document carries it",
			src:  "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\n",
			want: "warning: here-document at line 4 delimited by end-of-file (wanted `B')",
		},
		{
			name: "parentheses that hold a program without a dollar sign",
			src:  "cat <(cat <<EOF\na\nEOF)\n",
			want: "warning: here-document at line 1 delimited by end-of-file (wanted `EOF')",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, bash.Dialect())
			p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			rs := p.Remarks()
			if len(rs) != 1 {
				t.Fatalf("got %d remarks, want 1: %+v", len(rs), rs)
			}
			if got := bash.Diagnostics().Remark(rs[0]); got != c.want {
				t.Errorf("said %q, want %q", got, c.want)
			}
		})
	}
}

// The counter-case, and the reason the rule is about parentheses that hold a
// *program*: inside `$(( ))` the `<<` is a left shift, so there is no
// here-document to end anywhere and nothing to remark on.
func TestAnArithmeticSubstitutionIsNotReachedByThisRule(t *testing.T) {
	if _, err := syntax.Parse("echo $((\n4 << 2\n))\n", bash.Dialect()); err != nil {
		t.Fatalf("a shift written across lines: %v", err)
	}
	out, st := answersRun(t, "echo $((\n4 << 2\n))\n")
	if strings.TrimSpace(out) != "16" || st != 0 {
		t.Errorf("gave %q at %d, want 16 at 0", out, st)
	}
}
