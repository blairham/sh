// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.PipelineEndsWithItsLastElement: whether a pipeline ends when its
// last element does, leaving the elements before it running (#7169).

// endsWithLast is the test vector with the axis answered a.
func endsWithLast(a Answer) Semantics {
	s := testSemantics()
	s.PipelineEndsWithItsLastElement = a
	return s
}

// outlivesTheLast is a first element that can only end once the shell has
// moved past the pipeline and created `go` — or, in a shell that waits for it
// and so never gets there, once it has given up looking. It is a handshake
// rather than a sleep, so where `first` falls says which of the two the shell
// did however loaded the machine is: a shell that moves on writes `after`
// first, and one that waits writes `first` first, after the four seconds the
// element looks for.
const outlivesTheLast = `{ i=0; until [ -e go ] || [ $i -ge 80 ]; do /bin/sleep 0.05; i=$((i+1)); done; ` +
	`echo first >&2; } | :`

// runToAFile runs src with both streams on one file and returns what the file
// holds once the run is over. A file and not a buffer, because an element is
// left running only where what it writes into is a descriptor of its own to
// hold — see strayelements.go — and a buffer is not one.
func runToAFile(t *testing.T, src string, sem Semantics) (string, int) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := syntax.Core()
	prog, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	r := newTestRunner(t, &Runner{Stdout: f, Stderr: f, Semantics: &sem, Env: testPATH(), Dialect: &d})
	st, err := r.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b), st
}

// TestAPipelineEndsWithItsLastElementWhereTheDialectSays: yes runs the next
// command while the first element is still running, a bare `wait` waits for
// that element, and the shell waits for it as it ends; no waits for it in the
// pipeline. Here the last element runs on a copy of the shell, so this is the
// route where the pipeline waits for that copy alone — the cross-dialect test
// has the last element run in the shell itself.
func TestAPipelineEndsWithItsLastElementWhereTheDialectSays(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		{"yes, then wait", Yes, outlivesTheLast + `; echo after; : >go; wait; echo "end $?"`, "after\nfirst\nend 0\n"},
		{"yes, to the end of the shell", Yes, outlivesTheLast + `; echo after; : >go`, "after\nfirst\n"},
		{"yes, the last element's status", Yes, outlivesTheLast + `; echo "after $?"; : >go`, "after 0\nfirst\n"},
		{"no", No, outlivesTheLast + `; echo after; : >go; wait; echo "end $?"`, "first\nafter\nend 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if out, st := runToAFile(t, tc.src, endsWithLast(tc.a)); out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestAPipelineWritingIntoABufferWaitsForEveryElement: an element writing into
// something that is not a descriptor is not left running even where the
// dialect leaves one, because it would be a second writer into a stream it
// holds no copy of. ksh93 is seen to wait in the same place: measured
// 2026-11-01 on ksh93u+, `x=$( { { sleep 0.3; echo first >&2; } | :; echo
// after; } 2>&1 )` holds `first` and then `after`.
func TestAPipelineWritingIntoABufferWaitsForEveryElement(t *testing.T) {
	t.Parallel()
	src := outlivesTheLast + `; echo after; : >go`
	if out, st := run(t, src, withSem(endsWithLast(Yes))); out != "first\nafter\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "first\nafter\n")
	}
}

// TestAPipelineThatEndsWithItsLastElementWaitsForPipefail: with pipefail on,
// the status needs every element, so even yes waits for them. Measured
// 2026-11-01 on ksh93u+: `set -o pipefail; { sleep 0.3; echo first >&2; exit
// 3; } | :; echo "after $?"` writes `first` and then `after 3`.
func TestAPipelineThatEndsWithItsLastElementWaitsForPipefail(t *testing.T) {
	t.Parallel()
	sem := endsWithLast(Yes)
	sem.PipefailOption = Yes
	src := `set -o pipefail; ` + strings.Replace(outlivesTheLast, "echo first >&2;", "echo first >&2; exit 3;", 1) +
		`; echo "after $?"; : >go`
	if out, st := runToAFile(t, src, sem); out != "first\nafter 3\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "first\nafter 3\n")
	}
}

// TestAnUnansweredPipelineEndWaitsAndAsksNothing: the axis is read and not
// asked, because an element outliving the last one is a matter of timing in
// most pipelines. A vector with no answer waits for every element and says
// nothing.
func TestAnUnansweredPipelineEndWaitsAndAsksNothing(t *testing.T) {
	t.Parallel()
	src := `{ /bin/sleep 0.2; echo first >&2; } | :; echo "after $?"`
	if out, st := runToAFile(t, src, endsWithLast(Unspecified)); out != "first\nafter 0\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "first\nafter 0\n")
	}
}

// TestAnElementEndingAsThePipelineLeavesItRacesNothing: an element that ends
// in the moment between the shell deciding it is still running and the shell
// moving on writes its answer where the shell's snapshot was read from, and
// not through the shell's own names as they are replaced. Two builtins end
// almost together, so a couple of thousand pipelines of them land in that
// moment; the race detector, which the Linux leg runs this under, is the
// assertion. Measured 2026-11-01: six reports in 3000 such pipelines before
// the elements bound their own arrays, none after.
func TestAnElementEndingAsThePipelineLeavesItRacesNothing(t *testing.T) {
	t.Parallel()
	src := `i=0; while [ $i -lt 2000 ]; do : | :; i=$((i+1)); done; wait; echo "end $?"`
	if out, st := runToAFile(t, src, endsWithLast(Yes)); out != "end 0\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "end 0\n")
	}
}
