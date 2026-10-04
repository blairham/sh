// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// **A `( … )` and a `$( … )` share this shell's command hash** —
// Semantics.SubshellSharesTheCommandHash. Measured 2026-10-03 on ksh93u+:
// `( zzc ); hash` lists `zzc`, `x=$(zzc); hash` lists it too, and `hash zzc;
// ( hash -r ); hash` lists nothing, while a background `( zzc ) & wait` keeps
// its own.
func TestASubshellSharesTheCommandHash(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zzc"), []byte("#!/bin/sh\n:\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ src, want string }{
		{"( zzc ); hash", "zzc=" + filepath.Join(dir, "zzc") + "\n"},
		{"x=$(zzc); hash", "zzc=" + filepath.Join(dir, "zzc") + "\n"},
		{"hash zzc; ( hash -r ); hash", ""},
		{"( zzc ) & wait; hash", ""},
	} {
		out, _ := runKsh(t, dir, c.src)
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
