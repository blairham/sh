// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// traceOf runs a script and returns only what went to stderr, because the
// whole question about `set -x` is which stream it uses.
func traceOf(t *testing.T, src string, sem Semantics, diag Diagnostics) string {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errOut bytes.Buffer
	r := &Runner{Stdout: &out, Stderr: &errOut, Semantics: &sem, Diagnostics: &diag, Name: "sh"}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errOut.String()
}

func TestXtraceStructureIsUnanimous(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  Semantics
		diag Diagnostics
	}{
		{"dash", dash.Semantics(), dash.Diagnostics()},
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"ksh93", ksh.Semantics(), ksh.Diagnostics()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Each simple command, expanded, before it runs — and nothing on
			// stdout.
			if got := traceOf(t, `set -x; echo a b`, tc.sem, tc.diag); got != "+ echo a b\n" {
				t.Errorf("got %q", got)
			}
			// A compound command is not traced; the commands inside it are.
			if got := traceOf(t, `set -x; if true; then echo y; fi`, tc.sem, tc.diag); got != "+ true\n+ echo y\n" {
				t.Errorf("compound: got %q", got)
			}
			// Off by default, and `set +x` stops it.
			if got := traceOf(t, `echo a`, tc.sem, tc.diag); got != "" {
				t.Errorf("off by default: got %q", got)
			}
		})
	}
}

func TestXtraceQuotingIsADialectAnswer(t *testing.T) {
	const src = `set -x; x="hello wor"; echo "$x"`
	// dash prints an expanded field with a space unquoted, so one argument
	// and two are indistinguishable in its trace.
	if got := traceOf(t, src, dash.Semantics(), dash.Diagnostics()); !strings.Contains(got, "+ echo hello wor\n") {
		t.Errorf("dash: got %q", got)
	}
	for _, tc := range []struct {
		name string
		sem  Semantics
		diag Diagnostics
	}{
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"ksh93", ksh.Semantics(), ksh.Diagnostics()},
	} {
		if got := traceOf(t, src, tc.sem, tc.diag); !strings.Contains(got, `+ echo 'hello wor'`) {
			t.Errorf("%s: got %q", tc.name, got)
		}
	}
	// An embedded quote is where bash and ksh93 part: one closes, escapes
	// and reopens, the other reaches for $'…'.
	const q = `set -x; x="it's"; echo "$x"`
	if got := traceOf(t, q, bash.Semantics(), bash.Diagnostics()); !strings.Contains(got, `'it'\''s'`) {
		t.Errorf("bash: got %q", got)
	}
	if got := traceOf(t, q, ksh.Semantics(), ksh.Diagnostics()); !strings.Contains(got, `$'it\'s'`) {
		t.Errorf("ksh93: got %q", got)
	}
}

func TestXtracePrefixIsADialectAnswer(t *testing.T) {
	// zsh names the script and the line, and the function and 0 inside one.
	got := traceOf(t, `set -x; f() { echo in; }; f`, zsh.Semantics(), zsh.Diagnostics())
	if !strings.Contains(got, "+sh:1> f\n") || !strings.Contains(got, "+f:0> echo in\n") {
		t.Errorf("zsh: got %q", got)
	}
	if got := traceOf(t, `set -x; echo a`, bash.Semantics(), bash.Diagnostics()); got != "+ echo a\n" {
		t.Errorf("bash: got %q", got)
	}
}

func TestXtraceAssignmentsAndDisabling(t *testing.T) {
	// bash and ksh93 give each assignment its own line; dash and zsh do not.
	if got := traceOf(t, `set -x; a=1 b=2`, bash.Semantics(), bash.Diagnostics()); got != "+ a=1\n+ b=2\n" {
		t.Errorf("bash: got %q", got)
	}
	if got := traceOf(t, `set -x; a=1 b=2`, dash.Semantics(), dash.Diagnostics()); got != "+ a=1 b=2\n" {
		t.Errorf("dash: got %q", got)
	}
	// ksh93 applies `set +x` before printing it, so it leaves no trace of
	// itself; the others print it and then stop.
	if got := traceOf(t, `set -x; set +x; echo done`, ksh.Semantics(), ksh.Diagnostics()); got != "" {
		t.Errorf("ksh93: got %q", got)
	}
	if got := traceOf(t, `set -x; set +x; echo done`, dash.Semantics(), dash.Diagnostics()); got != "+ set +x\n" {
		t.Errorf("dash: got %q", got)
	}
}

// TestXtracePipelineOrderIsDeterministic guards the gate chain. Pipeline
// elements run at the same time, so without one their trace lines come out in
// whatever order the scheduler chose.
func TestXtracePipelineOrderIsDeterministic(t *testing.T) {
	for i := 0; i < 40; i++ {
		got := traceOf(t, `set -x; echo a | cat`, bash.Semantics(), bash.Diagnostics())
		if got != "+ echo a\n+ cat\n" {
			t.Fatalf("run %d: got %q", i, got)
		}
	}
}
