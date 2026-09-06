// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/internal/childguard"
)

// takeProcSubs hands the pipes over and forgets them, and the forgetting is
// the part worth pinning: removal is idempotent, so a list that is never
// cleared removes the same paths again after every later command and never
// looks wrong — it only grows, for as long as the session lasts.
func TestTakeProcSubsForgetsWhatItHandedOver(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	r.procSubs = []procSubPipe{{path: "a"}, {path: "b"}}
	if got := r.takeProcSubs(); len(got) != 2 {
		t.Fatalf("took %v, want both pipes", got)
	}
	if got := r.takeProcSubs(); len(got) != 0 {
		t.Errorf("took %v a second time, want nothing left on the runner", got)
	}
}

// The name the guard looks for is the name this package writes.
//
// Two readers of one prefix: the shell makes `<TMPDIR>/sh-procsubNNNN/` and
// the guard that fails a test run leaving a process holding one of those pipes
// finds it by that name in a command line. A prefix that changed in one place
// and not the other would leave the guard reporting nothing, forever and
// silently, which is the failure mode a guard cannot have.
func TestTheGuardLooksForTheNameThisPackageWrites(t *testing.T) {
	if procSubDirPrefix != childguard.PipeMarker {
		t.Errorf("this package writes %q and the guard looks for %q; they have to be the same string",
			procSubDirPrefix, childguard.PipeMarker)
	}
}
