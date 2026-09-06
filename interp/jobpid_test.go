// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync"
	"testing"
)

// A job's process id is its first process and not its latest.
//
// It was its latest, and that was a data race rather than a preference: a job
// that runs more than one external command reached the assignment again while
// the shell that started it had already read the field — `background` releases
// on `<-job.ready`, which the *first* write closes, and every later write is
// unsynchronized. `go test -race` reported it from the background-job test in
// driver; a person would see a `jobs -l` row change under them.
//
// A unit test as well as the race detector's word, because a detector only
// reports what a run happened to interleave.
func TestAJobsProcessIdIsItsFirstProcess(t *testing.T) {
	j := &Job{done: make(chan struct{}), ready: make(chan struct{})}
	j.setPID(111)
	select {
	case <-j.ready:
	default:
		t.Fatal("the first process did not settle the job")
	}
	if j.PID != 111 {
		t.Fatalf("PID = %d, want 111", j.PID)
	}
	j.setPID(222)
	if j.PID != 111 {
		t.Errorf("PID = %d after a second process, want the first one, 111", j.PID)
	}
}

// And concurrently, which is the shape the race had: many writers and one
// reader that has already been released.
//
// The assertion is that the value never moves once it can be read. Without
// the once it moves, and under -race it is reported as well.
func TestAJobsProcessIdDoesNotMoveOnceItCanBeRead(t *testing.T) {
	j := &Job{done: make(chan struct{}), ready: make(chan struct{})}
	var writers sync.WaitGroup
	for pid := 1; pid <= 8; pid++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			j.setPID(pid)
		}()
	}
	<-j.ready
	// Read the way setLastJob reads it, then again after every writer has
	// been and gone.
	first := j.PID
	writers.Wait()
	if j.PID != first {
		t.Errorf("PID moved from %d to %d after it had been read", first, j.PID)
	}
	if first == 0 {
		t.Error("the job was released with no process id at all")
	}
}

// A job that never starts a process is released by finishing, with no pid,
// which is the other half of what settles one.
func TestAJobWithNoProcessIsReleasedByFinishing(t *testing.T) {
	j := &Job{done: make(chan struct{}), ready: make(chan struct{})}
	select {
	case <-j.ready:
		t.Fatal("released before anything happened")
	default:
	}
	j.finish(0)
	select {
	case <-j.ready:
	default:
		t.Fatal("finishing did not release the job")
	}
	if j.PID != 0 {
		t.Errorf("PID = %d, want 0 for a job that never had a process", j.PID)
	}
}
