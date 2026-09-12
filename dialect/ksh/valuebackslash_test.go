// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell reads a backslash that arrived in a **value** as data, and the
// metacharacter behind it stays live — it against the other five.
//
// Measured 2026-09-12 on ksh93u+ from a script file, in a directory holding
// exactly `a\b` and `a*`. Both names are there on purpose: a directory
// holding neither prints the same word whichever rule is in force, so it
// could not tell the readings apart, and the row would pass with the axis
// wired to nothing (#1367).
func TestAValueBackslashIsDataAndWhatFollowsStaysLive(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{`a\b`, `a*`} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		// The discriminating row: the other five columns print `[a\*]`.
		{`v='a\*'; set -- $v; printf '[%s]' "$@"`, `[a\b]`},
		// A live metacharacter with no backslash in front of it, which both
		// readings answer alike — so a fix that merely stopped escaping
		// would pass the row above and fail this one.
		{`v='a*'; set -- $v; printf '[%s]' "$@"`, `[a*][a\b]`},
		// And a backslash before an ordinary character, unanimous in the
		// panel: no live metacharacter is left, so nothing is globbed and
		// the backslash is printed.
		{`v='a\b'; set -- $v; printf '[%s]' "$@"`, `[a\b]`},
		// A doubled backslash, where this axis has nothing to say: both
		// readings encode the pair the same way, so the `*` is live either
		// way and the pattern is two literal backslashes, which matches
		// neither name. bash answers `[a\b]` here — the first backslash
		// quoting the second *and vanishing from the pattern* — and that is
		// #1370's row rather than this one's.
		{`v='a\\*'; set -- $v; printf '[%s]' "$@"`, `[a\\*]`},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "sh", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
		}, tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}
