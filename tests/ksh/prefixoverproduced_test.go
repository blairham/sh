// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"testing"
)

// A `function`-form call's prefix over a parameter the shell produces keeps
// the shell's type: `SECONDS=5 f` shows the body an exported `-F 3` clock
// counting from 5, `LINENO=x f` an exported line number counting the body's
// own lines, and `RANDOM=5 f` a plain exported 5 — and after the call every
// one of them is the shell's own again, where this shell's hidden cell read
// as an assignment after an `unset` and ended the producer for good (#6859).
//
// Measured 2026-10-27 on ksh93u+ 2012-08-01 under `-c`. The rows are patterns:
// a clock's fraction and a drawn RANDOM are not values to compare.
func TestAKeywordCallsPrefixOverAProducedParameterKeepsTheShellsType(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`function f { typeset -p SECONDS; }; SECONDS=5 f`, "typeset -x -F 3 SECONDS=5\\.\\d{3}\n"},
		{`function f { echo ${SECONDS%.*}; }; SECONDS=5 f; echo ${SECONDS%.*}`, "5\n0\n"},
		{`function f { typeset -p SECONDS; }; SECONDS=abc f`, "typeset -x -F 3 SECONDS=0\\.\\d{3}\n"},
		{`function f { typeset -p SECONDS; }; SECONDS=5.5 f`, "typeset -x -F 3 SECONDS=5\\.5\\d{2}\n"},
		{`function f { SECONDS=9; typeset -p SECONDS; }; SECONDS=5 f; typeset -p SECONDS`, "typeset -x -F 3 SECONDS=9\\.\\d{3}\ntypeset -F 3 SECONDS=0\\.\\d{3}\n"},
		{`function f { :; }; SECONDS=5 f; typeset -p SECONDS`, "typeset -F 3 SECONDS=0\\.\\d{3}\n"},
		{`function f { typeset -p SECONDS; }; export SECONDS; SECONDS=5 f; typeset -p SECONDS`, "typeset -x -F 3 SECONDS=5\\.\\d{3}\ntypeset -x -F 3 SECONDS=0\\.\\d{3}\n"},
		{`function f { typeset -p LINENO; }; LINENO=x f`, "typeset -x LINENO=1\n"},
		{`function f { typeset -p LINENO; }; LINENO=x f; echo $LINENO; typeset -p LINENO`, "typeset -x LINENO=1\n1\ntypeset -i LINENO=1\n"},
		{`function f { typeset -p RANDOM; echo $RANDOM; }; RANDOM=5 f`, "typeset -x RANDOM=5\n5\n"},
		{`function f { :; }; RANDOM=5 f; typeset -p RANDOM`, "typeset -i RANDOM=\\d+\n"},
		{`function f { :; }; RANDOM=5 f; echo ${#RANDOM}`, "[1-5]\n"},
		// The controls: a POSIX-form call's prefix is the shell's own
		// assignment and persists, and a prefix over a plain name is still
		// a fresh cell with no letters.
		{`f() { typeset -p SECONDS; }; SECONDS=5 f`, "typeset -F 3 SECONDS=5\\.\\d{3}\n"},
		{`typeset -i v=1; function g { typeset -p v; }; v=2+2 g; typeset -p v`, "typeset -x v=2\\+2\ntypeset -i v=1\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if !regexp.MustCompile("^" + tc.want + "$").MatchString(out) {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
