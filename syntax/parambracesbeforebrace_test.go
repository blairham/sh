// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The printer drops the braces of a `${x}` a plain name does not need, and in
// front of a brace-expansion group it must not: the two spellings are two
// programs there.
//
// Brace expansion happens before parameter expansion, and in the shell that
// hands its output back to the word as text the character a group produced
// continues the name. Measured 2026-09-22 on bash 5.3.20 with
// `var=baz; varx=vx; vary=vy`: `${var}{x,y}` is `bazx bazy` and `$var{x,y}` is
// `vx vy`. See interp.Semantics.BraceOutputRereadAsText.
//
// So the spelling is load-bearing in both directions — a written pair stays and
// a bare name may not acquire one — which is what separates this from the
// arrangement that asks for every pair back.
func TestABraceGroupBehindAParameterKeepsTheSpellingThatWasRead(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo ${var}{x,y}", "echo ${var}{x,y}"},
		{"echo $var{x,y}", "echo $var{x,y}"},
		{"echo ${var}{x,y}z", "echo ${var}{x,y}z"},
		// A group that expands to nothing of the kind is still a group, so
		// the spelling is kept there too rather than read for what it
		// produces: a printer cannot know what a range will count.
		{"echo ${var}{1..3}", "echo ${var}{1..3}"},
		// The pair still goes where nothing can run into the name. `{` is
		// the only character this adds to that rule, so the ordinary cases
		// are the control.
		{"echo ${var}", "echo $var"},
		{"echo ${var} {x,y}", "echo $var {x,y}"},
		{"echo ${var}-{x,y}", "echo $var-{x,y}"},
		// And it goes for every name a bare `$` may be followed by that
		// cannot take another character: a special and a positional are one
		// character long, so `$#{a,b}` is `$#` with text behind it however
		// the text arrived.
		{"echo ${#}{a,b}", "echo $#{a,b}"},
		{"echo ${1}{a,b}", "echo $1{a,b}"},
		// The `$$` run one shell spells with a brace pair of its own is the
		// sharpest of those: keeping the braces there would write `${$}`,
		// which is a different construct.
		{"echo $${a,b}", "echo $${a,b}"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if got := syntax.Print(f); got != tc.want {
				t.Errorf("printed %q as %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
