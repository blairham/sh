// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A file is handed back as it came, and that is a behavior rather than a
// saving.
//
// os/exec connects a child directly to an *os.File and builds a pipe for
// anything else, so a wrapped stream means every later command is handed a
// pipe — and a child asking whether its output is a terminal gets a different
// answer for the rest of the session. #735 measured that price on the
// interpreter's side of the same rule: after `sleep 0.05 &`, `test -t 1` in a
// child said no where it says yes in all five shells of the panel. Wrapping
// here would buy the same thing for the same nothing, since two goroutines
// writing one file are serialized by the descriptor's own lock.
func TestGuardingHandsAFileBackUnwrapped(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	mu := new(sync.Mutex)
	if got := guardWriter(mu, f); got != any(f) {
		t.Errorf("guardWriter wrapped an *os.File (%T), want the file itself", got)
	}
	// And anything else is wrapped, or the line above would be a guard that
	// never guards.
	var buf bytes.Buffer
	got := guardWriter(mu, &buf)
	if _, wrapped := got.(*guardedWriter); !wrapped {
		t.Errorf("guardWriter left a %T alone, want it guarded", got)
	}
	// Both streams share one lock, which is what makes the guard exclude
	// anything at all when an embedder hands the same writer to both.
	out, errs := guardedStreams(&buf, &buf)
	go1, ge := out.(*guardedWriter), errs.(*guardedWriter)
	if go1.mu != ge.mu {
		t.Error("stdout and stderr were guarded by different locks, which excludes nothing")
	}
}
