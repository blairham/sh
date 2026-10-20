// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A value's trailing backslash in front of something the *script* had already
// quoted (#4158).
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from script
// files, with `v='a\'` in a directory holding `a*b`, `a\*b`, `a\\*b`, `ab` and
// `axb`. The first row is the one this test is for and the panel is **unanimous**
// on it — six columns, one field — where this shell answered two:
//
//	written      dash            bash 5.3        bash 3.2        ksh93           zsh
//	$v\\*b       [a\\*b]         [a\\*b]         [a\\*b]         [a\\*b]         [a\\*b]
//	$v\*b        [a\*b][a\\*b]   [a\*b][a\\*b]   [a\*b][a\\*b]   [a\*b]          [a\*b]
//	$v\**b       [a\*b][a\\*b]   [a\*b][a\\*b]   [a\*b][a\\*b]   [a\*b]          [a\*b]
//	$v*b         [a\*b]          [a\*b]          [a\*b]          [a\*b][a\\*b]   [a\*b][a\\*b]
//	$v\x*b       [a\x*b]         [axb]           [a\x*b]         [a\x*b]         no match
//
// Rows two and three are a **panel split** and are deliberately not asserted
// here: dash and bash free the `*` where ksh93 and zsh keep the piece literal,
// and this shell answers with ksh93 and zsh. That is a reading rather than a
// defect and wants an axis; the note on
// interp.Semantics.ValueBackslashInAPattern records it. Row four is the control
// that says the split is about the written backslash and not about the value's,
// and row five is bash 5.3 departing from its own 3.2 again.
func TestAValueBackslashBeforeSomethingTheScriptQuoted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a*b", `a\*b`, `a\\*b`, "ab", "axb"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ name, src, want string }{
		{
			"a backslash the script quoted is not something to quote again",
			`v='a\'; printf '[%s]' $v\\*b`, `[a\\*b]`,
		},
		{
			// The control: with nothing quoted behind it the backslash quotes
			// the live `*`, and one name comes back rather than two.
			"and with a live metacharacter it still quotes",
			`v='a\'; printf '[%s]' $v*b`, `[a\*b]`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, dir, c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
