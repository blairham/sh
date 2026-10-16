// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A `${ cmd;}` body closed by a `}` straight behind a `{name}` is refused as
// one that never ended, when the substitution runs: what stood before it on
// the line has run, nothing of the body has, and a branch nothing takes says
// nothing (#6642). Measured 2026-10-15 on ksh93u+ 2012-08-01 at /bin/ksh
// with `-c`, a script file and standard input.
func TestABodyClosedBehindABraceNameIsRefusedWhenItRuns(t *testing.T) {
	const refused = "syntax error at line 1: `end of file' unexpected"
	for _, c := range []struct {
		src, out, err string
	}{
		{"echo before; echo ${ echo {v}}; echo after", "before\n", refused},
		{"echo ${ print -u2 side; echo {v}}", "", refused},
		{"x=${ echo {v}}; echo after", "", refused},
		{"echo ${ echo {}}", "", refused},
		{"echo ${ echo {v}} }", "", refused},
		{"echo ${ echo a\necho {v}}", "", "syntax error at line 2: `end of file' unexpected"},
		// Wherever the body comes to run: a function read on an earlier
		// line, and a string handed to eval.
		{"f() { echo ${ echo {v}}; }\necho defined; f", "defined\n", "syntax error at line 1: `end of file' unexpected"},
		{"eval 'echo ${ echo {v}}'", "", "syntax error at line 1: `end of file' unexpected"},
		// The controls: never run, and a blank in front of the brace.
		{"false && echo ${ echo {v}}; echo after", "after\n", ""},
		{"echo ${ echo {v} }", "{v}\n", ""},
		{"echo ${ echo {v};}", "{v}\n", ""},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if !strings.HasPrefix(out, c.out) || strings.Contains(out, "side") ||
			(c.err == "" && (out != c.out || st != 0)) ||
			(c.err != "" && (!strings.HasSuffix(strings.TrimSuffix(out, "\n"), c.err) || st != 3)) {
			t.Errorf("%q: %q (status %d), want %q then %q", c.src, out, st, c.out, c.err)
		}
	}
}
