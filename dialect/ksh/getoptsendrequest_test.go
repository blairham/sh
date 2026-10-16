// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **The end `getopts` makes of the script at the end of the options is not
// caught by anything that catches a fatal error** — a `function`-form call,
// an `eval` and a `.` all let it through, where each of them ends an ordinary
// fatal error and goes on. A subshell contains it by being a copy. And the
// refusal names the innermost frame of the location's chain where the
// builtin's name goes: the function, `eval` or `.` (#6632).
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01, `env -i PATH=/bin ksh -c …`, so
// that `$0` is `ksh`, with `dot.sh` holding `getopts x o` and `echo in-dot`.
func TestGetoptsEndOfOptionsRefusalEndsTheScriptThroughEveryBoundary(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{
			`readonly o; function f { getopts x o; echo st=$?; }; f; echo after`,
			"ksh: f[1]: f: o: is read only\n", 2,
		},
		{
			`typeset -i n; function f { getopts x n; echo st=$?; }; f; echo after`,
			"ksh: f[1]: f: ?: arithmetic syntax error\n", 2,
		},
		{
			`readonly o; function f { getopts x o; }; function g { f; echo g-after; }; g; echo after`,
			"ksh: g[1]: f[1]: f: o: is read only\n", 2,
		},
		{
			`readonly o; eval "getopts x o"; echo after`,
			"ksh: eval[1]: eval: o: is read only\n", 2,
		},
		{
			`readonly o; . ./dot.sh; echo after`,
			"ksh: .[1]: .: o: is read only\n", 2,
		},
		{
			`readonly o; function f { . ./dot.sh; }; f; echo after`,
			"ksh: f[1]: .[1]: .: o: is read only\n", 2,
		},
		{
			`readonly o; function f { getopts x o; }; ( f ); echo "after $?"`,
			"ksh: f[1]: f: o: is read only\nafter 2\n", 0,
		},
		// A letter the scan found is only reported, wherever it is.
		{
			`readonly o; function f { eval "getopts x o -x"; echo st=$?; }; f; echo after`,
			"ksh: f[1]: eval[1]: eval: o: is read only\nst=2\nafter\n", 0,
		},
		{
			`readonly o; g() { getopts x o -x; echo st=$?; }; eval g; echo after`,
			"ksh: eval[1]: eval: o: is read only\nst=2\nafter\n", 0,
		},
		// The control: an ordinary fatal error ends at the call.
		{
			`function f { readonly x=1; x=2; echo in; }; f; echo after`,
			"ksh: f: line 1: x: is read only\nafter\n", 0,
		},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "dot.sh"), []byte("getopts x o\necho in-dot\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "ksh", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
		}, tc.src+"\n")
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if out != tc.want || st != tc.status {
			t.Errorf("%s:\ngot  %q, status %d\nwant %q, status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}
