// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// runWithSourced runs src with `inc` written beside it as a file the script
// may source, and returns everything the run wrote.
func runWithSourced(t *testing.T, inc, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc"), []byte(inc), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	// The three conditions these cases are about, and the letter that
	// carries two of them into a call. A shell without them refuses the
	// `trap` outright, which would make every count below zero.
	sem.TrapHasReturnCondition = Yes
	sem.TrapHasDebugCondition = Yes
	sem.TrapHasErrCondition = Yes
	sem.SetHasTheFunctraceLetter = Yes
	// And the axis for the trap reaching a call it was not set in, at the
	// answer the cases are written against: it does not, so what the trace
	// changes is visible on its own.
	sem.DebugTrapRunsInsideCalls = No
	var out strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Dir: dir, Stdout: &out, Stderr: &out,
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String()
}

// The RETURN trap a sourced file fires is bounded by the *function* it was
// set in, exactly as it is for a function's own return: a trap set at the top
// level is not in scope inside a function that did not set it, so a file that
// function sources has none to fire.
//
// Measured 2026-09-22 on the shell with the condition, from a script file and
// with call tracing off. The trap is scoped rather than global, which is the
// half this used to miss — a sourced file fired it from anywhere.
func TestASourcedFilesReturnTrapIsBoundedByTheFunctionAroundIt(t *testing.T) {
	out := runWithSourced(t, "echo inc\n", `trap 'echo R' RETURN
f() { . ./inc; }
f
echo between
. ./inc
echo end`)
	// Once, for the file sourced at the top level where the trap was set —
	// and nothing for the one inside the function.
	if got := strings.Count(out, "R\n"); got != 1 {
		t.Errorf("the trap fired %d times, want 1:\n%s", got, out)
	}
	if strings.Index(out, "R\n") < strings.Index(out, "between") {
		t.Errorf("the firing came from inside the function:\n%s", out)
	}
}

// And a trap the sourced file itself sets belongs to the function that
// sourced it, so that function's own return fires it too — two firings for
// one call, where recording the file's frame gave one.
func TestATrapASourcedFileSetsBelongsToTheFunctionThatSourcedIt(t *testing.T) {
	out := runWithSourced(t, "trap 'echo R' RETURN\necho inc\n", `f() { . ./inc; }
f
echo between
g() { :; }
g
echo end`)
	if got := strings.Count(out, "R\n"); got != 2 {
		t.Errorf("the trap fired %d times, want 2 — the file's end and the call's:\n%s", got, out)
	}
	// And not in a function that never set it, with tracing off.
	if strings.Contains(out[strings.Index(out, "between"):], "R\n") {
		t.Errorf("a function that did not set the trap fired it:\n%s", out)
	}
}

// Call tracing is what carries it in, as it does for a function: with tracing
// on, the file sourced inside the function fires it as well.
func TestTracingCarriesTheReturnTrapIntoASourcedFileInsideACall(t *testing.T) {
	out := runWithSourced(t, "echo inc\n", `set -T
trap 'echo R' RETURN
f() { . ./inc; }
f
echo end`)
	// Twice: the file's end and the function's own return, both now in
	// scope.
	if got := strings.Count(out, "R\n"); got != 2 {
		t.Errorf("the trap fired %d times, want 2 with tracing on:\n%s", got, out)
	}
}

// The DEBUG trap does not reach a RETURN body while tracing is off, because a
// RETURN body is part of a call unwinding and the trace is what carries the
// trap into a call. The other bodies are the control: an ERR body fires it
// with tracing off.
func TestTheDebugTrapReachesAReturnBodyOnlyWhileTracingIsOn(t *testing.T) {
	out := runWithSourced(t, "echo inc\n", `trap 'echo D' DEBUG
trap 'echo R' RETURN
. ./inc
echo end`)
	// One D each for the `trap … RETURN`, the `. ./inc` and the `echo end`,
	// and none for the RETURN body — which is what the R with no D in front
	// of it says.
	if !strings.Contains(out, "inc\nR\nD\n") {
		t.Errorf("the RETURN body fired the DEBUG trap with tracing off:\n%s", out)
	}
	if got := strings.Count(out, "D\n"); got != 3 {
		t.Errorf("the DEBUG trap fired %d times, want 3 — and none for the RETURN body:\n%s", got, out)
	}
	traced := runWithSourced(t, "echo inc\n", `set -T
trap 'echo D' DEBUG
trap 'echo R' RETURN
. ./inc
echo end`)
	// With tracing on the body fires one more, and so does the sourced
	// file's own line.
	if strings.Count(traced, "D\n") <= strings.Count(out, "D\n") {
		t.Errorf("tracing carried nothing into the bodies:\n%s", traced)
	}
}

// An ERR body is the control: it fires the DEBUG trap with tracing off, which
// is what says the rule above is about the *return* rather than about trap
// bodies in general.
func TestAnErrBodyReachesTheDebugTrapWithTracingOff(t *testing.T) {
	out := runWithSourced(t, "", `trap 'echo D' DEBUG
trap 'echo E' ERR
false
echo end`)
	if !strings.Contains(out, "D\nD\nE\n") {
		t.Errorf("the ERR body did not fire the DEBUG trap:\n%s", out)
	}
}
