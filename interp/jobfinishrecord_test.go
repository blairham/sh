// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// What the shell records about a job's ending is on the list before the close
// that releases a `wait` for it, and not after.
//
// The close is not only how a reader learns the job ended — it is the thing a
// `wait` is blocked on, and the shell carries straight on from there. So a
// record made after it is a record the script can already have run past.
//
// It was made after it, at all three places a job ends, and the cost was two
// required checks failing on diffs that could not reach them (#4571): the
// `CHLD` arrival a trap counts was raised after `close(j.done)`, so the
// three-children case counted `n=2` and the one-child case counted `n=0` —
// the wait came back, the script reached its end, and the goroutine that had
// released it was still on its way to the record. Widening that gap by two
// milliseconds reproduced both CI failures byte for byte.
//
// A unit test rather than only the script-level ones in
// waitkeepswaiting_test.go, because those fail on the ordering *sometimes* —
// they are what a loaded runner happened to catch. This one asks the ordering
// directly, and the hook is run where it can see the answer: `Finished` is
// true only once the close has happened, so a record moved back after it fails
// here every time rather than one run in a hundred.
func TestAJobsEndingIsRecordedBeforeTheCloseThatReleasesAWait(t *testing.T) {
	j := &Job{done: make(chan struct{}), ready: make(chan struct{})}
	recorded, published := false, false
	finished := j.finishRecording(7, 0, func() {
		recorded = true
		published = j.Finished()
	})
	if !finished {
		t.Error("the first finish did not report itself as the one that finished the job")
	}
	if !recorded {
		t.Fatal("the ending was never recorded")
	}
	if published {
		t.Error("the ending was recorded after the close that releases a `wait`, not before it")
	}
	if j.Status != 7 {
		t.Errorf("Status = %d, want 7 — the status is published before the record", j.Status)
	}
}

// And a second finish records nothing and says so, because the caller that
// owes the record whatever happens has to be able to tell.
//
// The once is what keeps one ending from being counted twice, and putting the
// record inside it is what makes the ordering above possible. The report is
// the other half: a job something else finished first — the shell waiting out
// a polled job's process itself — never reaches the hook, and its arrival
// would simply go missing if the caller trusted the once to have run it. See
// Runner.jobReaped.
func TestASecondFinishRecordsNothingAndSaysSo(t *testing.T) {
	j := &Job{done: make(chan struct{}), ready: make(chan struct{})}
	j.finish(3)
	recorded := false
	finished := j.finishRecording(9, 0, func() { recorded = true })
	if finished {
		t.Error("a second finish reported itself as the one that finished the job")
	}
	if recorded {
		t.Error("a second finish ran the record, which would count one ending twice")
	}
	if j.Status != 3 {
		t.Errorf("Status = %d, want 3 — the first ending is the one that stands", j.Status)
	}
}
