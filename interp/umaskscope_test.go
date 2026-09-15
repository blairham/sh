// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// maskScopeRun runs src with the mask kept in a variable rather than in the
// process, and answers with what was printed and what the mask was left at.
//
// Its own helper rather than umaskRun because a pipeline element runs on a
// goroutine of its own: two shells reach this hook, so the value it stands
// for has to be guarded the way the process's own mask is. Under `-race`,
// umaskRun's bare closure is a data race on the day somebody pipes a `umask`.
func maskScopeRun(t *testing.T, start int, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UmaskPrintsFourDigits = Yes
	sem.UmaskSetWithSPrints = No
	dg := Diagnostics{}
	var mu sync.Mutex
	held := start
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.SetUmask = func(mask int) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		old := held
		held = mask
		return old, nil
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	mu.Lock()
	defer mu.Unlock()
	return buf.String(), held
}

// A mask set inside a body the shell waits for is that body's own: it is in
// force while the body runs, and the shell has the mask it started with
// afterwards.
//
// This is #2898, which was every one of these at once. The mask is process
// state and a body of this shell is not a process, so `umask` reached the
// mask of the shell that started the body and stayed there — and a script
// that loosens the mask in a subshell went on writing files nobody meant to
// widen. See umaskscope.go.
func TestAMaskSetInAForkedBodyStaysThere(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string // what the script prints, one line per `umask`
		left int    // the mask the shell is left with
	}{{
		name: "subshell",
		src:  `umask 077; ( umask 002; umask ); umask`,
		want: "0002\n0077\n",
		left: 0o077,
	}, {
		name: "command substitution",
		src:  `umask 077; v=$( umask 002; umask ); echo "$v"; umask`,
		want: "0002\n0077\n",
		left: 0o077,
	}, {
		name: "pipeline element",
		src:  `umask 077; { umask 002; umask; } | cat_nothing; umask`,
		want: "0077\n",
		left: 0o077,
	}, {
		name: "nested bodies each put back what they found",
		src:  `umask 077; ( umask 002; ( umask 007; umask ); umask ); umask`,
		want: "0007\n0002\n0077\n",
		left: 0o077,
	}, {
		name: "the last change is not what comes back",
		src:  `umask 077; ( umask 002; umask 007 ); umask`,
		want: "0077\n",
		left: 0o077,
	}, {
		name: "a body that only reads has moved nothing",
		src:  `umask 077; ( umask ); umask`,
		want: "0077\n0077\n",
		left: 0o077,
	}, {
		name: "a body inside a body",
		src:  `umask 077; ( v=$( umask 002; umask ); echo "$v"; umask ); umask`,
		want: "0002\n0077\n0077\n",
		left: 0o077,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, left := maskScopeRun(t, 0o022, tc.src)
			// The pipeline case writes into a command that is not there,
			// so the shell's complaint about it is not the assertion.
			out = withoutLines(out, "cat_nothing")
			if out != tc.want {
				t.Errorf("printed %q, want %q", out, tc.want)
			}
			if left != tc.left {
				t.Errorf("the shell is left with %#o, want %#o", left, tc.left)
			}
		})
	}
}

// The shell's own mask is not a body's, so a `umask` written outside one is
// still the shell's to keep — the check that the restore above is scoped to a
// boundary rather than undoing every change.
func TestAMaskSetByTheShellItselfStays(t *testing.T) {
	out, left := maskScopeRun(t, 0o022, `umask 077; ( : ); umask`)
	if strings.TrimSpace(out) != "0077" {
		t.Errorf("printed %q, want 0077", out)
	}
	if left != 0o077 {
		t.Errorf("the shell is left with %#o, want 077", left)
	}
}

// withoutLines drops the lines holding s, so that a diagnostic about the
// fixture is not read as the shell's answer.
func withoutLines(out, s string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(out, "\n") {
		if line != "" && !strings.Contains(line, s) {
			b.WriteString(line)
		}
	}
	return b.String()
}
