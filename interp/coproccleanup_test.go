// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A Runner that ran a coprocess and finished leaves no descriptor behind.
//
// The near ends go into the descriptor table and stay there until the
// coprocess is let go of, and the entry going is deliberately not a close:
// a script may have duplicated an end onto a number of its own, and that
// duplicate is the same open file. So while the shell is running, the file
// outliving the entry is the rule rather than a leak — and when the shell
// *stops being a shell* nothing was closing it, which left the moment the
// descriptor comes back to the process a question about the garbage
// collector rather than about the shell (#4499).
//
// A shell process does not care, because the kernel reclaims everything at
// exit. A Runner embedded in a long-lived program is exactly the shape this
// library is for, and there the ends accumulate against that program's
// open-file limit. Measured 2026-09-25 on darwin/arm64 before the fix:
// twenty Runners like these left 3 descriptors open before and 44 after, and
// two `runtime.GC()` calls did not bring it down.
//
// **The finished Runners are kept alive on purpose.** A collector that ran
// mid-test would close the files through their finalizers and the row would
// pass for a reason that has nothing to do with CleanUp — which is the shape
// this test exists to distinguish, so it must not be able to happen. The
// `KeepAlive` at the end is what holds them, and the explicit `GC` before the
// count is what says the pass is not the collector's doing either.
func TestAFinishedRunnerClosesItsCoprocessEnds(t *testing.T) {
	const rounds = 20

	before, err := openDescriptors()
	if err != nil {
		t.Skipf("this platform does not list its open descriptors: %v", err)
	}

	d := syntax.Core()
	d.Coproc = true
	d.CoprocName = true
	f, parseErr := syntax.Parse("coproc CP { /bin/cat </dev/null; }\n", d)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	sem := PosixSemantics()
	sem.CoprocEndsInAnArray = Yes

	kept := make([]*Runner, 0, rounds)
	for i := range rounds {
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
			Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
		})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		kept = append(kept, r)
	}

	// The coprocesses themselves are children that may still be exiting, and
	// their far ends are the operating system's to reclaim; what is counted
	// is whether the number comes back down at all.
	runtime.GC()
	after := waitForDescriptors(before + descriptorSlack)
	if after > before+descriptorSlack {
		t.Errorf("%d descriptors open after %d finished shells, %d before — "+
			"a coprocess end nothing closed", after, rounds, before)
	}
	runtime.KeepAlive(kept)
}
