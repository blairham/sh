// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// FatalErrorEndsAtAKeywordFunctionCall, both answers, over both definition
// forms: only the keyword form with the answer yes is a boundary (#5508).
func TestAFatalErrorEndsAtAKeywordCallOnlyWhereTheAxisSays(t *testing.T) {
	const body = `{ : ${u?gone}; echo in; }; f; echo st=$?`
	for _, c := range []struct {
		name   string
		answer Answer
		def    string
		goesOn bool
	}{
		{"keyword, yes", Yes, "function f ", true},
		{"keyword, no", No, "function f ", false},
		{"POSIX, yes", Yes, "f() ", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := permissive()
			s.FatalErrorEndsAtAKeywordFunctionCall = c.answer
			out, st := run(t, c.def+body, withSem(s))
			// The diagnostic is written either way; what moves is whether
			// the caller goes on, and the body's own next line never runs.
			want := "sh: u: gone\n"
			if c.goesOn {
				want += "st=2\n"
			}
			if out != want || (st == 0) != c.goesOn {
				t.Errorf("got %q at %d, want %q, going on %v", out, st, want, c.goesOn)
			}
		})
	}
}

// A request to stop is not an error, and is not caught.
func TestAnExitInAKeywordCallIsNotCaught(t *testing.T) {
	s := permissive()
	s.FatalErrorEndsAtAKeywordFunctionCall = Yes
	out, st := run(t, `function f { exit 3; }; f; echo st=$?`, withSem(s))
	if out != "" || st != 3 {
		t.Errorf("got %q at %d, want nothing at 3", out, st)
	}
}
