// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A backslash that ends a **value** quotes the field's next character, and where
// that character is the `/` closing a piece of a path the backslash goes with the rest of the piece's quoting.
//
// A value's end is not the field's end: `bs=\; echo ./tmp${bs}/a/b/*` is one
// word whose backslash quotes a separator in the *next* span. This shell wrote
// such a backslash as an ordinary literal, so the separator stayed unquoted and
// the metacharacter behind it stayed live (#4234).
//
// Measured 2026-09-22, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`.
// See interp.Semantics.ValueBackslashSurvivesAPatternPiece, answered No here.
func TestABackslashEndingAValueInFrontOfASeparator(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{`x\`, "y"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "e"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "tmp", "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmp", "a", "b", "c"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const bs = "bs='\\'; "
	for _, tc := range []struct{ name, src, want string }{
		// The opposite of bash and ksh93 on the same two words, which is what
		// makes this an axis rather than one reading of the separator.
		{"a pattern piece drops it", `printf "[%s]" ./[y]${bs}/e`, `[./y/e]`},
		{"and misses with it", `printf "[%s]" ./[x]${bs}/e`, `[./[x]\/e]`},
		// The unanimous half: a piece that spells a name loses the backslash
		// with the rest of its quoting in every column.
		{"a spelled piece loses it", `printf "[%s]" ./tmp${bs}/a/b/*`, `[./tmp/a/b/c]`},
		{"and one mid-piece too", `printf "[%s]" ./t${bs}mp/a/b/*`, `[./tmp/a/b/c]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runDash(t, dir, bs+tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
