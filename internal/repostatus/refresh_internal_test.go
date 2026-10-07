// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repostatus

import (
	"os"
	"path/filepath"
	"testing"
)

// Not parallel: it sets a package hook.
func TestAWriteDuringARefreshIsNotCalledFreshAfterwards(t *testing.T) {
	// #6302. A head rewritten while a refresh is between reading it and
	// storing the answer must not leave that answer looking current. Witnesses
	// taken after the read pair the old answer with the new file, and every
	// later lookup serves it. The write is landed exactly in that gap, so this
	// fails every time rather than once in a loaded CI run.
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	head := filepath.Join(git, "HEAD")
	if err := os.MkdirAll(git, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(head, []byte("ref: refs/heads/one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New(nil)
	// No watch, as where none can be had: nothing but the next lookup is
	// coming to notice the write, which is the case the witnesses carry alone.
	c.tried = true

	landed := false
	afterRead = func() {
		if landed {
			return
		}
		landed = true
		if err := os.WriteFile(head, []byte("ref: refs/heads/twotwo\n"), 0o600); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterRead = nil })

	if status, _ := c.Status(root); status.Branch != "one" {
		t.Fatalf("the first answer was %q", status.Branch)
	}
	if !landed {
		t.Fatal("the write was never landed inside the refresh")
	}
	if status, _ := c.Status(root); status.Branch != "twotwo" {
		t.Errorf("a head rewritten during the refresh was still answered as %q", status.Branch)
	}
}
