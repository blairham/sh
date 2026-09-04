// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A command that redirects one stream twice either fills both files or only
// the last, and the axis is asked only where there *is* a second one.
func TestRedirectsWriteToEveryTargetIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		first  string
	}{
		{"every target", Yes, "x\n"},
		{"only the last", No, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsWriteToEveryTarget = tc.answer
			if _, st := run(t, "echo x >a >b", func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			}); st != 0 {
				t.Fatalf("status %d", st)
			}
			if got := readFile(t, dir, "a"); got != tc.first {
				t.Errorf("a = %q, want %q", got, tc.first)
			}
			if got := readFile(t, dir, "b"); got != "x\n" {
				t.Errorf("b = %q, want the last target always written", got)
			}
		})
	}

	// One redirection needs no answer from anyone, which is what keeps the
	// core usable rather than refusing every `>`.
	sem := CoreSemantics()
	out, _ := run(t, "echo x >a", func(r *Runner) { r.Semantics, r.Dir = &sem, t.TempDir() })
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("one target: got %q, want no question asked", out)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(b)
}

// TestReadWriteRedirection — `<>` opens both ways, creates, and never
// truncates; with no descriptor number it is standard input.
func TestReadWriteRedirection(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "keep\n")
	got, st, err := redirRunAt(t, dir, `exec 3<> f; exec 3>&-; cat f; exec 4<> made; echo "st=$?"`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(got, "keep") || !strings.Contains(got, "st=0") {
		t.Errorf("output = %q, want the file untouched and the open to succeed", got)
	}
	if st != 0 {
		t.Errorf("status = %d", st)
	}
	if _, err := os.Stat(filepath.Join(dir, "made")); err != nil {
		t.Errorf("the missing file was not created: %v", err)
	}
	out, _, err := redirRunAt(t, dir, `printf x > d; { head -c1; } <> d`)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(out, "x") {
		t.Errorf("output = %q, want <> to stand on standard input", out)
	}
}
