// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// takeProcSubs hands the pipes over and forgets them, and the forgetting is
// the part worth pinning: removal is idempotent, so a list that is never
// cleared removes the same paths again after every later command and never
// looks wrong — it only grows, for as long as the session lasts.
func TestTakeProcSubsForgetsWhatItHandedOver(t *testing.T) {
	r := &Runner{}
	r.procSubs = []string{"a", "b"}
	if got := r.takeProcSubs(); len(got) != 2 {
		t.Fatalf("took %v, want both pipes", got)
	}
	if got := r.takeProcSubs(); len(got) != 0 {
		t.Errorf("took %v a second time, want nothing left on the runner", got)
	}
}
