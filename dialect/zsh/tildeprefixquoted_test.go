// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A quote or an expansion inside a tilde prefix does **not** stop the
// expansion here. This shell is the one column in the panel that reads it that
// way: the quoting comes off and what is left is the name, so `~"/bar"`,
// `~$x` and `~+"/x"` all reach a directory where bash, bash 3.2, dash, ksh93
// and BusyBox ash print the word as written.
//
// Measured 2026-09-22, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with `HOME=/usr/xyz`, against all seven columns; the rows are in
// Semantics.TildePrefixStopsAtAQuoteOrAnExpansion, and the other six columns'
// answer is pinned in interp/tildeprefixend_test.go (#4156).
//
// `~\chet/bar` is measured and is deliberately not a case here: this shell
// reads the name and dies on a user it has no entry for, which is an answer
// this package cannot give — `~user` is left as written throughout, for want
// of a user database. What is asserted is the half that separates the two
// answers without needing one.
func TestAQuotedTildePrefixStillExpands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a backslash on the closing slash", `echo ~\/bar`, "/h/bar"},
		{"a quoted span where the slash would be", `echo ~"/bar"`, "/h/bar"},
		{"an expansion that begins with a slash", `x=/y; echo ~$x`, "/h/y"},
		{"an expansion before the slash", `x=; echo ~$x/y`, "/h/y"},
		{"a quoted span after a named tilde", `cd /; echo ~+"/x"`, "//x"},
		// The controls, where every column agrees: a plain prefix expands and
		// a `~` that is itself quoted does not.
		{"a plain prefix", `echo ~/bar`, "/h/bar"},
		{"the tilde itself quoted", `echo "~"/bar`, "~/bar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			sh := zshShell()
			sh.Stdout, sh.Stderr = &out, &errs
			sh.Env = []string{"HOME=/h", "PATH=/usr/bin:/bin", "LC_ALL=C"}
			code := driver.MainArgs(sh, []string{"zsh", "-c", tc.src})
			if got := strings.TrimSpace(out.String()); got != tc.want || errs.Len() != 0 || code != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q",
					tc.src, got, errs.String(), code, tc.want)
			}
		})
	}
}
