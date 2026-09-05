// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// traceOf runs a script and returns only what went to stderr, because the
// whole question about `set -x` is which stream it uses.
func traceOf(t *testing.T, src string, sem Semantics, diag Diagnostics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errOut bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errOut, Semantics: &sem, Diagnostics: &diag, Name: "sh", Env: testPATH()})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errOut.String()
}

// TestXtraceStructureIsUnanimous: every simple command is written to stderr
// before it runs, with its words already expanded, and compound commands are
// not traced. The decoration axes do not move any of that, which is what this
// asserts by holding the structure still while the quoting answer varies.
func TestXtraceStructureIsUnanimous(t *testing.T) {
	for _, q := range []TraceQuoting{QuoteNever, QuoteShell, QuoteDollar} {
		diag := Diagnostics{TraceQuoting: q}
		sem := permissive()
		// Each simple command, expanded, before it runs — and nothing on
		// stdout.
		if got := traceOf(t, `set -x; echo a b`, sem, diag); got != "+ echo a b\n" {
			t.Errorf("quoting %v: got %q", q, got)
		}
		// A compound command is not traced; the commands inside it are.
		if got := traceOf(t, `set -x; if true; then echo y; fi`, sem, diag); got != "+ true\n+ echo y\n" {
			t.Errorf("quoting %v: compound: got %q", q, got)
		}
		// Off by default, and `set +x` stops it.
		if got := traceOf(t, `echo a`, sem, diag); got != "" {
			t.Errorf("quoting %v: off by default: got %q", q, got)
		}
	}
}

// TestTraceQuotingIsAThreeWayAnswer names the TraceQuoting values rather than
// the shells that picked them; the presets' picks are asserted in dialect/.
func TestTraceQuotingIsAThreeWayAnswer(t *testing.T) {
	const src = `set -x; x="hello wor"; echo "$x"`
	sem := permissive()
	// QuoteNever prints an expanded field with a space unquoted, so one
	// argument and two are indistinguishable in its trace.
	if got := traceOf(t, src, sem, Diagnostics{TraceQuoting: QuoteNever}); !strings.Contains(got, "+ echo hello wor\n") {
		t.Errorf("QuoteNever: got %q", got)
	}
	for _, q := range []TraceQuoting{QuoteShell, QuoteDollar} {
		if got := traceOf(t, src, sem, Diagnostics{TraceQuoting: q}); !strings.Contains(got, `+ echo 'hello wor'`) {
			t.Errorf("%v: got %q", q, got)
		}
	}
	// An embedded quote is where the two quoting styles part: one closes,
	// escapes and reopens, the other reaches for $'…'.
	const q = `set -x; x="it's"; echo "$x"`
	if got := traceOf(t, q, sem, Diagnostics{TraceQuoting: QuoteShell}); !strings.Contains(got, `'it'\''s'`) {
		t.Errorf("QuoteShell: got %q", got)
	}
	if got := traceOf(t, q, sem, Diagnostics{TraceQuoting: QuoteDollar}); !strings.Contains(got, `$'it\'s'`) {
		t.Errorf("QuoteDollar: got %q", got)
	}
}

func TestTraceStyleIsADialectAnswer(t *testing.T) {
	// TraceNameLine names the script and the line, and the function and 0
	// inside one.
	sem := permissive()
	got := traceOf(t, `set -x; f() { echo in; }; f`, sem, Diagnostics{TraceStyle: TraceNameLine})
	if !strings.Contains(got, "+sh:1> f\n") || !strings.Contains(got, "+f:0> echo in\n") {
		t.Errorf("TraceNameLine: got %q", got)
	}
	if got := traceOf(t, `set -x; echo a`, sem, Diagnostics{TraceStyle: TracePlain}); got != "+ echo a\n" {
		t.Errorf("TracePlain: got %q", got)
	}
}

// TestTraceAssignmentsSeparately pins the axis by name: Yes gives each
// assignment of `a=1 b=2` its own line, No traces them as one.
func TestTraceAssignmentsSeparately(t *testing.T) {
	sem := permissive()
	sem.TraceAssignmentsSeparately = Yes
	if got := traceOf(t, `set -x; a=1 b=2`, sem, Diagnostics{}); got != "+ a=1\n+ b=2\n" {
		t.Errorf("Yes: got %q", got)
	}
	sem.TraceAssignmentsSeparately = No
	if got := traceOf(t, `set -x; a=1 b=2`, sem, Diagnostics{}); got != "+ a=1 b=2\n" {
		t.Errorf("No: got %q", got)
	}
}

// TestTraceShowsItsOwnDisabling pins the axis by name: No applies `set +x`
// before printing it, so it leaves no trace of itself; Yes prints it and then
// stops.
func TestTraceShowsItsOwnDisabling(t *testing.T) {
	sem := permissive()
	sem.TraceShowsItsOwnDisabling = No
	if got := traceOf(t, `set -x; set +x; echo done`, sem, Diagnostics{}); got != "" {
		t.Errorf("No: got %q", got)
	}
	sem.TraceShowsItsOwnDisabling = Yes
	if got := traceOf(t, `set -x; set +x; echo done`, sem, Diagnostics{}); got != "+ set +x\n" {
		t.Errorf("Yes: got %q", got)
	}
}

// TestXtracePipelineOrderIsDeterministic guards the gate chain. Pipeline
// elements run at the same time, so without one their trace lines come out in
// whatever order the scheduler chose.
func TestXtracePipelineOrderIsDeterministic(t *testing.T) {
	sem := permissive()
	for i := 0; i < 40; i++ {
		got := traceOf(t, `set -x; echo a | cat`, sem, Diagnostics{})
		if got != "+ echo a\n+ cat\n" {
			t.Fatalf("run %d: got %q", i, got)
		}
	}
}

// TestTraceForHeaderHasThreeAnswers records what a `for` loop prints when it
// takes another turn. It is not a bool: one answer prints nothing, one
// reprints the header, and one prints something the header does not say.
func TestTraceForHeaderHasThreeAnswers(t *testing.T) {
	const src = `set -x; for i in 1 2; do echo $i; done`
	sem := permissive()
	// TraceForSource reprints the header once per iteration, as written.
	want := "+ for i in 1 2\n+ echo 1\n+ for i in 1 2\n+ echo 2\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForSource}); got != want {
		t.Errorf("TraceForSource: got %q, want %q", got, want)
	}
	// TraceForNone prints only the commands inside.
	want = "+ echo 1\n+ echo 2\n"
	if got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForNone}); got != want {
		t.Errorf("TraceForNone: got %q, want %q", got, want)
	}
	// TraceForAssign prints neither, and shows the assignment instead.
	// Without the trailing space its dialect puts on an assignment that
	// stands alone as a command — measured, and the reason this does not
	// reuse traceLine.
	got := traceOf(t, src, sem, Diagnostics{TraceForHeader: TraceForAssign})
	if !strings.Contains(got, "+ i=1\n") || strings.Contains(got, "for i in") {
		t.Errorf("TraceForAssign: got %q, want an assignment and no header", got)
	}
}

// TestXtraceForHeaderIsUnexpanded is the reason the header is kept as source
// rather than rebuilt from the tree: TraceForSource prints what was written,
// so a rebuilt line would lose the `$` and the quotes.
func TestXtraceForHeaderIsUnexpanded(t *testing.T) {
	sem := permissive()
	diag := Diagnostics{TraceForHeader: TraceForSource}
	got := traceOf(t, `x="a b"; set -x; for i in $x; do :; done`, sem, diag)
	if !strings.Contains(got, "+ for i in $x\n") {
		t.Errorf("got %q, want it to contain %q", got, "+ for i in $x\n")
	}
	got = traceOf(t, `set -x; for i in "a b"; do :; done`, sem, diag)
	if !strings.Contains(got, `+ for i in "a b"`+"\n") {
		t.Errorf("got %q, want the quotes kept", got)
	}
}
