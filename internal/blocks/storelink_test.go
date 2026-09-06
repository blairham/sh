// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/policy"
)

// The block store is aimed by a shell variable, so it is HISTFILE's shape.
//
// SH_BLOCKS_DIR names the directory, which means a typed line chooses where
// the shell writes a record of everything it ran — and, if the index inside
// that directory is a symbolic link, chooses what file the append lands in.
// Until #942 the gate was asked about the index's path and the append was done
// on whatever the link reached.
func TestABlockIndexThatIsALinkIsCheckedOnWhatItReached(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	root := t.TempDir()
	hidden := filepath.Join(root, "hidden")
	store := filepath.Join(root, "store")
	for _, d := range []string{hidden, store} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(hidden, "protected")
	if err := os.WriteFile(target, []byte("KEEP THIS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(store, IndexName)); err != nil {
		t.Fatal(err)
	}

	p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
		"version 1\ndefault allow\ndeny write %s/**\n", hidden)))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	s := Open(store, boundary.Boundary{Gate: p}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })

	// A refused store is silent — a policy that hid it meant for it not to be
	// written, and there is nobody at the prompt to tell — so the assertion is
	// on the file rather than on an error.
	if err := s.Append(t.Context(), Record{Command: "echo hi"}); err != nil {
		t.Fatalf("Append reported %v; a refused store is silent", err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "KEEP THIS\n" {
		t.Errorf("the protected file holds %q, want it unwritten", body)
	}
}
