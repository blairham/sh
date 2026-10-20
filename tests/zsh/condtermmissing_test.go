// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A condition with no term in it is reported at the token **behind** the
// `]]` here, at that token's own line, where bash names the `]]` on every
// route.
//
// Measured 2026-09-18 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin LC_ALL=C`
// (#2964).
//
// The routes are the point of the row and not decoration: `-c '[[ ]]'` and a
// file with no final newline answer alike in both columns, and it is a file
// **with** one that separates them — which is why this was filed as a reading
// the `-c` probe could not see.
func TestAConditionWithNoTermIsBlamedOnTheTokenAfterTheCloser(t *testing.T) {
	var out, errs bytes.Buffer
	run := func(argv ...string) (string, int) {
		out.Reset()
		errs.Reset()
		sh := zshShell()
		sh.Stdout, sh.Stderr = &out, &errs
		code := driver.MainArgs(sh, append([]string{"zsh"}, argv...))
		return errs.String(), code
	}
	file := func(src string) string {
		path := filepath.Join(t.TempDir(), "s.sh")
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// `-c`, where nothing stands behind the closer: the `]]` is the last
	// token read and is what gets named.
	if e, st := run("-c", "[[ ]]"); !strings.Contains(e, "parse error near `]]'") || st != 1 {
		t.Errorf("-c: err %q status %d, want the closer named at 1", e, st)
	}
	// A file **without** a final newline answers the same way, and a file
	// with one does not — which is the pair the issue turns on.
	if e, st := run(file("[[ ]]")); !strings.Contains(e, ":1: parse error near `]]'") || st != 1 {
		t.Errorf("a file with no final newline: err %q status %d", e, st)
	}
	if e, st := run(file("[[ ]]\n")); !strings.Contains(e, ":2: parse error near `") || st != 1 {
		t.Errorf("a file with one: err %q status %d, want the newline named on line 2", e, st)
	}
	// A real token behind it, on the same line and on the next — and blank
	// lines between are skipped, so the complaint lands on the word.
	for _, tc := range []struct{ src, want string }{
		{"[[ ]] == x ]]", ":1: parse error near `=='"},
		{"[[ ]] ]]", ":1: parse error near `]]'"},
		{"[[ ]]; echo after", ":1: parse error near `;'"},
		{"[[ ]] echo after", ":1: parse error near `echo'"},
		{"[[ ]]\necho after\n", ":2: parse error near `echo'"},
		{"[[ ]]\n\n\necho after\n", ":4: parse error near `echo'"},
	} {
		if e, st := run(file(tc.src)); !strings.Contains(e, tc.want) || st != 1 {
			t.Errorf("%q: err %q status %d, want it to hold %q", tc.src, e, st, tc.want)
		}
	}
	// And a condition that **has** a term is untouched, which is the control:
	// nothing about an ordinary refusal moved.
	for _, tc := range []struct{ src, want string }{
		{"[[ -n ]]", "unknown condition: -n"},
		{"[[ p q ]]", "parse error: condition expected: p"},
		{"[[ p q r ]]", "condition expected: q"},
		{"[[ ( ) ]]", "parse error near `)'"},
	} {
		if e, _ := run("-c", tc.src); !strings.Contains(e, tc.want) {
			t.Errorf("%q: err %q, want it to hold %q", tc.src, e, tc.want)
		}
	}
	// One shape is measured and not modeled, and it is here so that a change
	// to it is a change to a written-down row: `[[ ]] && x ]]` is
	// `condition expected: x` in that shell — a run-time complaint about a
	// word, the `&&` being read as the list operator it also is — where this
	// reading names the `&&` as the token the parse stopped on.
	if e, _ := run("-c", "[[ ]] && x ]]"); !strings.Contains(e, "parse error near `&&'") {
		t.Errorf("the recorded divergence: err %q", e)
	}
}
