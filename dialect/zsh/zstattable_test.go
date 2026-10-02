// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `zstat -A` into a name that is already an associative array stores the
// pairs as a table, and an odd count is refused as a literal's is. See
// interp.Runner.SetList.
//
// Measured 2026-10-02 on zsh 5.9.2 with an empty file `e` and a three-byte
// file `x` (#5153).
func TestZstatStoresIntoATable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runZshOnPath(t, dir, "zmodload zsh/stat; typeset -A h; h=(old 1); zstat -A h +size -n -- x e; print -r -- $h[x] $h[e] ${+h[old]}")
	if want := "3 0 0\n"; out != want {
		t.Errorf("pairs: got %q, want %q", out, want)
	}
	out, st := runZshOnPath(t, dir, "zmodload zsh/stat; typeset -A h; zstat -A h +size -- x; print after")
	if want := "bad set of key/value pairs for associative array"; !strings.Contains(out, want) || strings.Contains(out, "zstat:") ||
		strings.Contains(out, "after") || st != 1 {
		t.Errorf("odd: got %q at %d, want the refusal located as the shell and the script stopped at 1", out, st)
	}
}

// And the refusal leaves 1 behind for the next command where the shell
// carries on past a fatal error: measured, under `setopt continueonerror` in
// a script file `print st=$?` after it writes `st=1`.
func TestZstatsRefusalLeavesItsStatus(t *testing.T) {
	src := "setopt continueonerror\nzmodload zsh/stat\nprint -n abc > x\ntypeset -A h\nzstat -A h +size -- x\nprint st=$?\n"
	out, _, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, src)
	if out != "st=1\n" || !strings.Contains(errs, "bad set of key/value pairs") {
		t.Errorf("got %q, %q, want st=1 after the refusal", out, errs)
	}
}
