// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// TestABangAfterABarTogglesThePipeline is #5272. In ksh93, a `!` written
// after a pipe bar inverts the whole pipeline's negation, as one in front of
// it does. Each `!` toggles, and one with nothing after it ends the
// pipeline. #5256 refused the word in every dialect; this dialect now takes
// it. See syntax.Dialect.NegationAfterABarTogglesThePipeline.
//
// Every row measured 2026-10-02 on ksh93u+ 2012-08-01 (`/bin/ksh`), as script
// files.
func TestABangAfterABarTogglesThePipeline(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"echo | ! true; echo st=$?", "st=1\n"},
		{"echo | ! false; echo st=$?", "st=0\n"},
		{"echo | ! true | true; echo st=$?", "st=1\n"},
		{"echo | ! true | false; echo st=$?", "st=0\n"},
		{"! true | ! true; echo st=$?", "st=0\n"},
		{"echo | ! ! true; echo st=$?", "st=0\n"},
		{"echo hi | !; echo st=$?", "st=1\n"},
		{"true && echo hi | ! && echo yes; echo st=$?", "st=1\n"},
		{"if echo | ! true; then echo T; else echo E; fi", "E\n"},
		{"eval 'echo | ! true'; echo st=$?", "st=1\n"},
		{`echo a | ! read x; echo "x=$x st=$?"`, "x=a st=1\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// A `!` after the `!`s with only a bar behind it is still the bar's missing
// command, refused at the bar — measured, `echo | ! | cat` is a syntax error,
// "at line 1: `|' unexpected". And `|&` is the coprocess operator here and
// not a bar, so a `!` after it begins a pipeline of its own: `echo |& ! true`
// is 1 in ksh93u+ and here.
func TestABangAfterABarStillNeedsTheNextBarsCommand(t *testing.T) {
	_, err := syntax.Parse("echo | ! | cat", ksh.Dialect())
	if e, ok := err.(*syntax.Error); !ok || e.Kind != syntax.ErrUnexpected {
		t.Errorf("`echo | ! | cat`: err = %v, want an unexpected-token refusal", err)
	}
	if _, err := syntax.Parse("echo |& ! true", ksh.Dialect()); err != nil {
		t.Errorf("`echo |& ! true`: %v, want it to parse", err)
	}
}
