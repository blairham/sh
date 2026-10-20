// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
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
// Measured against ksh93u+, which runs it and gives `a` — in silence, where
// bash 5.3 remarks on the document that ran out. dash and zsh read the body
// from the whole input, take the `)` into it, and refuse the construct (#963).
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

// And this shell says nothing about it, which is the half a status cannot
// show: the parser records that the document ran out either way, and whether
// a word is put to it is the dialect's.
func TestTheDocumentThatRanOutIsNotRemarkedOn(t *testing.T) {
	p := syntax.NewParser("v=$(cat <<EOF\na\nEOF)\necho \"[$v]\"\n", ksh.Dialect())
	p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	rs := p.Remarks()
	if len(rs) != 1 {
		t.Fatalf("got %d remarks, want the one the parser records: %+v", len(rs), rs)
	}
	if got := ksh.Diagnostics().Remark(rs[0]); got != "" {
		t.Errorf("said %q, where this shell says nothing", got)
	}
}
