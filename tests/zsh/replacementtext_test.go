// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replacementDir holds names a live metacharacter in a replacement would find.
func replacementDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"Q", "axcd", "aQcd", "ax", "bx"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The flag group is a third route to the same operator, with its own reading
// of the operands, and the replacement is text on that route too.
//
// It is here rather than in the substrate because the group is this shell's
// grammar. Measured 2026-09-07 on zsh 5.9.2 in the directory above:
// `x=abcd; print -r -- "${(U)x//b/*}"` is `A*CD`, and `a=(ab cb); print -r --
// "${(U)a[@]//b/*}"` is `A* C*` (#1337).
func TestAFlaggedReplacementIsLiteralText(t *testing.T) {
	dir := replacementDir(t)
	for _, tc := range []struct{ src, want string }{
		{`x=abcd; print -r -- "${(U)x//b/*}"`, "A*CD"},
		{`a=(ab cb); print -r -- "${(U)a[@]//b/*}"`, "A* C*"},
		{`x=abcd; print -r -- "${(U)x//b/[Q]}"`, "A[Q]CD"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// And the unflagged operator here, where an unquoted expansion is neither
// split nor re-read as a pattern: what the replacement put in is what comes
// out, quoted or not. That is the pair the panel splits on — bash and ksh93
// glob the *result* — and both halves follow from axes already recorded, so
// the row exists to say the replacement itself contributed no pattern.
func TestAReplacementSurvivesAnUnquotedExpansion(t *testing.T) {
	dir := replacementDir(t)
	for _, tc := range []struct{ src, want string }{
		{`x=abcd; print -r -- ${x//b/*}`, "a*cd"},
		{`x=abcd; print -r -- "${x//b/*}"`, "a*cd"},
		{`x=abcd; print -r -- ${x//b/[Q]}`, "a[Q]cd"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}
