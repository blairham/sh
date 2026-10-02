// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMarkDirsWritesASlashAfterADirectory pins `markdirs`. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155).
func TestMarkDirsWritesASlashAfterADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tmpcd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmpfile1"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := runZsh(t, dir, "setopt markdirs; print tmp*; print */; unsetopt markdirs; print tmp*")
	if want := "tmpcd/ tmpfile1\ntmpcd//\ntmpcd tmpfile1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
