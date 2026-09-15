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

// maskScopeRun runs src with the process's mask kept in a variable rather
// than in the machine, and answers with what was printed, what the shell's
// own mask is afterwards, and what the *process's* is.
//
// Its own helper rather than umaskRun because a body runs on a goroutine of
// its own: two shells reach this hook, so the value it stands for has to be
// guarded the way the process's own mask is. Under `-race`, umaskRun's bare
// closure is a data race on the day somebody backgrounds a `umask`.
func maskScopeRun(t *testing.T, d syntax.Dialect, start int, src string) (out string, shell, process int) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UmaskPrintsFourDigits = Yes
	sem.UmaskSetWithSPrints = No
	// One of the seven bodies is a coprocess, and where its descriptors end
	// up is an axis of its own that the shells split on. Answered here so
	// that the body runs; nothing below asks about the answer.
	sem.CoprocEndsInAnArray = No
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
	shell = umaskHeld(t, r)
	mu.Lock()
	defer mu.Unlock()
	return buf.String(), shell, held
}

// A mask set inside a body is that body's own: it is in force while the body
// runs, and the shell has the mask it started with afterwards.
//
// This was #2898 for the four bodies whose caller is blocked on them, and
// #2949 for the three that run beside it — a background job, a coprocess and
// a process substitution. The first fix put the mask back at the end of the
// body, which those three cannot use: the process has one mask and they run
// while the shell carries on, so what such a body found is stale by the time
// it could be handed back. The mask is the Runner's own now, so a body has
// one the way a fork would and nothing is handed back at all. See
// umaskscope.go.
func TestAMaskSetInAForkedBodyStaysThere(t *testing.T) {
	coproc := syntax.Core()
	coproc.Coproc = true
	for _, tc := range []struct {
		name string
		src  string
		want string // what the script prints, one line per `umask`
	}{{
		name: "subshell",
		src:  `umask 077; ( umask 002; umask ); umask`,
		want: "0002\n0077\n",
	}, {
		name: "command substitution",
		src:  `umask 077; v=$( umask 002; umask ); echo "$v"; umask`,
		want: "0002\n0077\n",
	}, {
		name: "pipeline element",
		src:  `umask 077; { umask 002; umask; } | cat_nothing; umask`,
		want: "0077\n",
	}, {
		name: "background job",
		src:  `umask 077; { umask 002; umask; } & wait; umask`,
		want: "0002\n0077\n",
	}, {
		name: "process substitution",
		src:  `umask 077; read v < <( umask 002; umask ); echo "$v"; umask`,
		want: "0002\n0077\n",
	}, {
		name: "coprocess",
		src:  `umask 077; coproc { umask 002; }; wait; umask`,
		want: "0077\n",
	}, {
		name: "nested bodies each put back what they found",
		src:  `umask 077; ( umask 002; ( umask 007; umask ); umask ); umask`,
		want: "0007\n0002\n0077\n",
	}, {
		name: "the last change is not what comes back",
		src:  `umask 077; ( umask 002; umask 007 ); umask`,
		want: "0077\n",
	}, {
		name: "a body that only reads has moved nothing",
		src:  `umask 077; ( umask ); umask`,
		want: "0077\n0077\n",
	}, {
		name: "a body inside a body",
		src:  `umask 077; ( v=$( umask 002; umask ); echo "$v"; umask ); umask`,
		want: "0002\n0077\n0077\n",
	}, {
		name: "a body beside a body it started",
		src:  `umask 077; { umask 002; ( umask 007; umask ); umask; } & wait; umask`,
		want: "0007\n0002\n0077\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, shell, _ := maskScopeRun(t, coproc, 0o022, tc.src)
			// The pipeline case writes into a command that is not there,
			// so the shell's complaint about it is not the assertion.
			out = withoutLines(out, "cat_nothing")
			if out != tc.want {
				t.Errorf("printed %q, want %q", out, tc.want)
			}
			if shell != 0o077 {
				t.Errorf("the shell is left with %#o, want 077", shell)
			}
		})
	}
}

// The shell's own mask is not a body's, so a `umask` written outside one is
// still the shell's to keep — the check that a body's privacy is scoped to a
// boundary rather than swallowing every change.
func TestAMaskSetByTheShellItselfStays(t *testing.T) {
	out, shell, _ := maskScopeRun(t, syntax.Core(), 0o022, `umask 077; ( : ); umask`)
	if strings.TrimSpace(out) != "0077" {
		t.Errorf("printed %q, want 0077", out)
	}
	if shell != 0o077 {
		t.Errorf("the shell is left with %#o, want 077", shell)
	}
}

// The process keeps no mask of this shell's, which is what makes all of the
// above possible: the hook is asked once, to empty it and say what was there,
// and after that only around a fork. A shell that left its mask in the
// process would be showing it to every body running beside it.
func TestTheProcessIsLeftWithNoMaskOfTheShells(t *testing.T) {
	_, shell, process := maskScopeRun(t, syntax.Core(), 0o022, `umask 077; ( umask 002 ); umask 027`)
	if shell != 0o027 {
		t.Errorf("the shell holds %#o, want 027", shell)
	}
	if process != 0 {
		t.Errorf("the process holds %#o, want it emptied", process)
	}
}

// A shell that was given no hook has no mask to offer and says so, which is
// #117's rule and the one thing here that did not change: the process's mask
// is left exactly as it was, and the kernel goes on applying it.
func TestAShellWithNoHookRefusesAndLeavesTheProcessAlone(t *testing.T) {
	f, err := syntax.Parse(`umask 077; echo "st=$?"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh"})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(buf.String(), "st=2") {
		t.Errorf("printed %q, want the refusal and a failing status", buf.String())
	}
	// The mode a redirection would have asked for is the unmasked one, so
	// the kernel's own mask is still the only thing deciding.
	if got := r.CreationMode(0o666); got != 0o666 {
		t.Errorf("CreationMode(0666) is %#o, want it untouched", got)
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
