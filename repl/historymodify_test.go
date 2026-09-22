// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// The history rewrite is the one time this file is replaced rather than
// appended to, and it finishes with a rename over HISTFILE. That is a change
// to a path a typed line chose — HISTFILE is a shell variable — and it was
// exempt as the shell's own scaffolding until #1824 gave Boundary a modify
// seam to ask with.
//
// Every fixture is under t.TempDir(). None points into a home.

// overLimit writes a history file with more lines in it than the bound, so
// that saving one more triggers the rewrite rather than an ordinary append.
func overLimit(t *testing.T, keep int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history")
	var b strings.Builder
	for i := range keep + 5 {
		b.WriteString("echo line")
		b.WriteByte(byte('0' + i%10))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// lines is the file's contents as a count, which is what the bound is about.
func lines(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(body), "\n")
}

// leftovers is what is beside the history file afterwards, so a refused
// rewrite can be checked not to have left its temporary behind.
func leftovers(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	return len(entries) - 1
}

// TestTheHistoryRewriteAsksBeforeItReplacesTheFile, both ways from one gate.
func TestTheHistoryRewriteAsksBeforeItReplacesTheFile(t *testing.T) {
	t.Run("allowed, and the bound is enforced", func(t *testing.T) {
		path := overLimit(t, 4)
		g := &gateFor{path: filepath.Join(t.TempDir(), "nothing")}
		h := historyFile{path: path, size: 4, file: 4, bound: boundary.Boundary{Gate: g}}
		if err := h.save(t.Context(), []string{"echo typed"}, nil); err != nil {
			t.Fatal(err)
		}
		if got := lines(t, path); got != 4 {
			t.Errorf("history has %d lines, want the bound of 4", got)
		}
		if got := leftovers(t, path); got != 0 {
			t.Errorf("%d files left beside the history, want none", got)
		}
		// The rename was asked about as a write to the history's own path,
		// which is the record an audit stream needs to see the file replaced.
		var renames int
		for _, a := range g.asked {
			if a.Kind == interp.ActionOpen && a.Write && a.Path == path {
				renames++
			}
		}
		// Two: the append that fills the file, and the rewrite that replaces
		// it. A run that asked once has one of them going around the gate.
		if renames != 2 {
			t.Errorf("gate saw %d writes to %q, want the append and the rewrite", renames, path)
		}
	})

	t.Run("refused, and the old history stands", func(t *testing.T) {
		path := overLimit(t, 4)
		before := lines(t, path)
		// Refuse the rewrite alone by allowing the append: the gate here
		// denies only the *write* to the path, which the append also is — so
		// the append is refused too and the file is left exactly as it was,
		// which is the state this asserts.
		g := &gateFor{path: path}
		h := historyFile{path: path, size: 4, file: 4, bound: boundary.Boundary{Gate: g}}
		if err := h.save(t.Context(), []string{"echo typed"}, nil); err != nil {
			t.Fatal(err)
		}
		if got := lines(t, path); got != before {
			t.Errorf("history has %d lines, want the %d it started with", got, before)
		}
		if got := leftovers(t, path); got != 0 {
			t.Errorf("%d files left beside the history, want none", got)
		}
	})
}

// A refused rename must not leave the half-written neighbor behind: a policy
// that will not have the file replaced did not ask for a second copy of it in
// the same directory.
func TestARefusedRewriteTakesItsTemporaryWithIt(t *testing.T) {
	t.Parallel()
	path := overLimit(t, 4)
	// Allow everything except a write to the history path itself, so the
	// append lands, the temporary is built, and only the rename is refused.
	g := &writeGateExcept{path: path}
	h := historyFile{path: path, size: 4, file: 4, bound: boundary.Boundary{Gate: g}}
	if err := h.save(t.Context(), []string{"echo typed"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := leftovers(t, path); got != 0 {
		t.Errorf("%d files left beside the history, want the temporary removed", got)
	}
	if got := lines(t, path); got != 10 {
		t.Errorf("history has %d lines, want the 9 it started with plus the appended one", got)
	}
}

// writeGateExcept allows every access but the *rename* over one path: the
// first write to it is the append and is allowed, and the second is the
// rewrite and is not. It is the only way to reach the refused-rename branch,
// since a gate that refused the path outright would stop the append first.
type writeGateExcept struct {
	path  string
	seen  int
	asked []interp.Action
}

func (g *writeGateExcept) Allow(_ context.Context, a interp.Action) interp.Decision {
	g.asked = append(g.asked, a)
	if a.Kind == interp.ActionOpen && a.Write && a.Path == g.path {
		g.seen++
		if g.seen > 1 {
			return interp.Deny
		}
	}
	return interp.Allow
}
