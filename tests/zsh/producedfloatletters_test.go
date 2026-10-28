// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"testing"

	"github.com/blairham/sh/interp"
)

// A float letter a script puts on SECONDS is the shape it is written and
// listed in, format included, and the clock goes on counting under it: zsh
// never ends a produced parameter for a letter (#6854). Measured 2026-10-27
// on zsh 5.9.2 under `-f`: `typeset -E SECONDS` reads `1.012485000e+00`
// after a second and lists `typeset -E SECONDS=…`, where this wrote a
// fixed-point number and listed the stated `-i10`.
func TestAFloatLetterOnSecondsIsItsShape(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"typeset -F 5 SECONDS; echo ${#SECONDS}; typeset -p SECONDS\n", "7\ntypeset -F SECONDS=0\\.\\d{5}\n"},
		{"typeset -E SECONDS; echo ${#SECONDS}; echo ${SECONDS#*e}; typeset -p SECONDS\n", "15\n-0[1-9]\ntypeset -E SECONDS=\\d\\.\\d{9}e-0[1-9]\n"},
		{"typeset -E 3 SECONDS; echo ${#SECONDS}\n", "8\n"},
		// Read until it moves rather than a fixed number of times: five places
		// is ten microseconds, and three reads in a row fit inside that on a
		// fast runner. A clock that stopped never moves, and writes nothing.
		{"typeset -F 5 SECONDS; a=$SECONDS; i=0; while (( i++ < 100000 )); do [[ $SECONDS != $a ]] && { echo ticking; break; }; done\n", "ticking\n"},
		{"typeset -u RANDOM; a=$RANDOM; b=$RANDOM; [ \"$a\" != \"$b\" ] && echo drawing\n", "drawing\n"},
	} {
		out, _, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, c.src)
		if !regexp.MustCompile("^" + c.want + "$").MatchString(out) {
			t.Errorf("%q: wrote %q (%q), want %q", c.src, out, errs, c.want)
		}
	}
}
