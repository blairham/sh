// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A body written as a single stepped-over `;` runs here, where a body written
// as nothing does not. Measured 2026-09-12 on ksh93u+ with `-n`, `env -i
// PATH=/usr/bin:/bin` and a scratch HOME; dash and every bash column refuse
// both spellings.
//
//	$ ksh -n s.sh       # { }
//	s.sh: syntax error at line 1: `}' unexpected
//	$ ksh -n s.sh       # { ; }
//	(nothing)
func TestABodyMayBeWrittenAsOneSeparatorHere(t *testing.T) {
	d := ksh.Dialect()
	if !d.SteppedOverSeparatorIsABody {
		t.Error("ksh93 takes a body written as one stepped-over `;`")
	}
	// And not the neighbor: `{ }` is a syntax error here, which is what
	// keeps the two flags apart.
	if d.EmptyCompoundBody {
		t.Error("ksh93 refuses a body written as nothing")
	}
	for _, src := range []string{
		"{ ; }",
		"( ; )",
		"if :; then ; fi",
		"if :; then :; else ; fi",
		"if :; then :; elif :; then ; fi",
		"while :; do ; done",
		"until :; do ; done",
		"for i in a; do ; done",
		"select i in a; do ; done",
		"x() { ; }",
	} {
		if _, err := syntax.Parse(src+"\n", d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"{ }", "syntax error at line 1: `}' unexpected"},
		{"{ ; ; }", "syntax error at line 1: `;' unexpected"},
	} {
		_, err := syntax.Parse(tc.src+"\n", d)
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// Running one. It does nothing and succeeds, which the failure in front of it
// is what shows: a body that left `$?` alone would answer 1.
//
//	$ ksh s.sh          # false; { ; }; echo "st=$?"
//	st=0
func TestABodyWrittenAsOneSeparatorSucceedsHere(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`false; { ; }`, 0},
		{`false; ( ; )`, 0},
		{`false; if :; then ; fi`, 0},
		{`false; for i in a; do ; done`, 0},
		{`false; x() { ; }; x`, 0},
		// The body is skipped rather than standing in for a command, so a
		// loop still walks its list and a group still takes its redirection.
		{`{ ; } >/dev/null`, 0},
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
