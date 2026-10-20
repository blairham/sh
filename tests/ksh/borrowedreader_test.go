// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Whether borrowed text runs the commands it has already read when a later
// line will not parse.
//
// The only column that reads both through before running either, so neither leaves anything behind.
//
// Counted as a side effect rather than read off a transcript: the complaint is
// written either way, so a test matching on it would pass with either answer.
// Measured against ksh93u+ 2012-08-01, 2026-09-11.
func TestBorrowedTextRunsWhatItRead(t *testing.T) {
	dir := t.TempDir()
	evalMark := filepath.Join(dir, "eval")
	fileMark := filepath.Join(dir, "file")
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("printf x >> "+fileMark+"\nif; then\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	answersRun(t, "eval 'printf x >> "+evalMark+"\nif; then'")
	answersRun(t, ". "+script)
	for _, tc := range []struct{ name, path, want string }{
		{"eval", evalMark, ""},
		{"a sourced file", fileMark, ""},
	} {
		if got := markOf(t, tc.path); got != tc.want {
			t.Errorf("%s ran %q of the line before the failure, want %q", tc.name, got, tc.want)
		}
	}
}

func markOf(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}
