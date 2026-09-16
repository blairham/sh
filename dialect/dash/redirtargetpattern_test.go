// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A pattern in a redirection's target is not matched here — POSIX's own rule,
// and this column follows it.
//
// The input side only reports differently; the output side loses a file. A
// shell that matches truncates `only-one.txt`, and this one creates a file
// literally called `only-*.txt`. Measured 2026-09-16 (#3207).
func TestARedirectionTargetIsNotPathnameExpanded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only-one.txt"), []byte("CONTENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runHere := func(src string) (string, int) {
		t.Helper()
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "dash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
		}, src)
		if err != nil {
			return out + "unsupported: " + err.Error(), -1
		}
		return out, st
	}

	out, st := runHere("cat < only-*.txt\n")
	if st == 0 || strings.Contains(out, "CONTENT") {
		t.Errorf("out %q status %d, want the literal name opened and refused", out, st)
	}
	if !strings.Contains(out, "only-*.txt") {
		t.Errorf("out %q, want the name as written in the complaint", out)
	}
	// The same answer for a pattern that arrived through an expansion.
	// Asked before the output side below, which creates a file literally
	// called `only-*.txt` — after that the literal open succeeds and the
	// case can no longer tell the two readings apart.
	if out, st := runHere("e='only-*.txt'\ncat < $e\n"); st == 0 || strings.Contains(out, "CONTENT") {
		t.Errorf("out %q status %d, want a stored pattern refused too", out, st)
	}
	if out, st := runHere("printf 'X\\n' > only-*.txt\n"); out != "" || st != 0 {
		t.Errorf("out %q status %d, want a file called `only-*.txt` written", out, st)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "only-one.txt")); err != nil || string(got) != "CONTENT\n" {
		t.Errorf("only-one.txt = %q (%v), want the match untouched", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "only-*.txt")); err != nil || string(got) != "X\n" {
		t.Errorf("only-*.txt = %q (%v), want a file of that name", got, err)
	}
	// The control: a pattern that matches nothing is the name as written
	// under every reading, which is why the output side looked like
	// agreement until a case with a match was written.
	if out, st := runHere("printf 'Y\\n' > nomatch-*.txt\n"); out != "" || st != 0 {
		t.Errorf("out %q status %d, want the unmatched name written as spelled", out, st)
	}
}
