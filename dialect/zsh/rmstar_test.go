// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// rmStarRun runs src in an interactive runner whose streams are the terminal
// end of a pseudo-terminal, with answer already typed, and returns what the
// terminal received.
func rmStarRun(t *testing.T, dir, src, answer string) string {
	t.Helper()
	control, term, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	// Drained for the whole run and on its own goroutine: a read of the
	// control side blocks, and an unread one fills and blocks the writer.
	got := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := control.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				break
			}
		}
		got <- out.String()
	}()
	if _, err := control.Write([]byte(answer)); err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdin: term, Stdout: term, Stderr: term, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Route: interp.RouteStandardInput, Interactive: true,
		Vars:    map[string]string{"PATH": "/usr/bin:/bin"},
		Dialect: presetDialect(),
	}
	zsh.Apply(r)
	ran := make(chan struct{})
	go func() {
		_, _ = r.Run(context.Background(), f)
		close(ran)
	}()
	select {
	case <-ran:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: still waiting for an answer", src)
	}
	_ = term.Close()
	select {
	case out := <-got:
		return out
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: the terminal never closed", src)
	}
	return ""
}

// TestRmStarAsksFirst pins the question an interactive zsh asks before `rm
// *`. Measured 2026-10-02 on zsh 5.9.2 through a pseudo-terminal (#5155). See
// interp/rmstar.go.
func TestRmStarAsksFirst(t *testing.T) {
	for _, c := range []struct {
		name, src, answer string
		kept              bool
		wants             []string
	}{
		{"no", "rm *; print after", "n", true, []string{"zsh: sure you want to delete all 3 files in DIR [yn]? \an"}},
		{"yes", "rm *; print after", "y", false, []string{"\ay", "after"}},
		{"another key rings again", "rm *", "xn", true, []string{"\a\an"}},
		{"silent", "setopt rmstarsilent; rm *", "", false, nil},
		{"not a star", "rm b*; print after", "", true, []string{"after"}},
		{"a missing directory", "rm d/*", "n", true, []string{"all the files in DIR/d [yn]?"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range []string{"a", "b", "c"} {
				if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out := strings.ReplaceAll(rmStarRun(t, dir, c.src, c.answer), dir, "DIR")
			for _, w := range c.wants {
				if !strings.Contains(out, w) {
					t.Errorf("terminal got %q, want %q in it", out, w)
				}
			}
			if c.name == "no" && strings.Contains(out, "after") {
				t.Errorf("the line went on after a no: %q", out)
			}
			_, statErr := os.Stat(filepath.Join(dir, "a"))
			if kept := statErr == nil; kept != c.kept {
				t.Errorf("files kept = %v, want %v (terminal %q)", kept, c.kept, out)
			}
		})
	}
}
