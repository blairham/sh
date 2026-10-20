// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestADescriptorOverTheLimitIsRefusedAfterTheOpen: the file is created, and
// moving it to the number is what fails, named with the number the open
// produced; `exec` ends the shell at 1. Measured 2026-10-03 in the pinned
// image, `ulimit -n 64; exec 3>x; exec 70>fresh`. The limit is the hook's
// answer rather than this process's. See
// interp.Semantics.FdNumberBoundedByOpenFileLimit.
func TestADescriptorOverTheLimitIsRefusedAfterTheOpen(t *testing.T) {
	d := ash.Dialect()
	f, err := syntax.Parse("exec 3>x; exec 70>fresh; echo after\n", d)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := ash.Semantics(), ash.Diagnostics()
	dir := t.TempDir()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "ash", Dir: dir, Dialect: &d,
		GetRlimit: func(res interp.Resource) (int64, int64, error) {
			if res != interp.ResourceOpenFiles {
				return interp.RlimitInfinity, interp.RlimitInfinity, nil
			}
			return 64, 64, nil
		},
	}
	st, _ := r.Run(context.Background(), f)
	if got, want := buf.String(), "ash: dup2(4,70): Bad file descriptor\n"; got != want || st != 1 {
		t.Errorf("got %q at %d, want %q at 1", got, st, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh")); err != nil {
		t.Errorf("the file was not created: %v", err)
	}
}
