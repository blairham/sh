// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A trapped signal that lands between the two questions a watched read asks
// at a byte already there — whether the signal gives the read up, and then
// whether its handlers run first — is still one that gives the read up (#7181).
//
// The first question found nothing waiting; the signal landed; the second
// found it. Answering "run the handlers" there takes the signal off the list
// and the read goes on to the end of its line, which is what the trapped-read
// rows of tests/cross saw under load: `T st=0` where `T st=258` belongs, on a
// descriptor open both ways and on a `-u` read of one this shell opened at
// that number alike. The race is put here as a sequence rather than waited
// for, so the row is the same every run.
//
// The control is the same sequence on a stream open for reading alone, part of
// a line in: the reading that gives the read up only at the start of a line
// does not give it up there, and the handlers do run first — so the answer the
// row asserts is one the question can tell apart.
func TestASignalLandingBetweenTheTwoQuestionsAtAByteStillGivesTheReadUp(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "p")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	bothWays, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bothWays.Close() })
	readingOnly, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readingOnly.Close(); _ = writer.Close() })

	for _, tc := range []struct {
		name    string
		in      *os.File
		fd      int
		verdict readVerdict
		sig     syscall.Signal
	}{
		{"open both ways", bothWays, 0, readGivenUp, syscall.SIGINT},
		{"by a number this shell opened", readingOnly, 3, readGivenUp, syscall.SIGINT},
		{"open for reading alone, part of a line in", readingOnly, 0, readRunsHandlers, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.ReadIsAbandonedByATrappedSignal = Yes
			sem.ReadAbandonedWhen = ReadAbandonedOnlyAtTheStartOfALine
			sem.ReadHandlersRun = ReadHandlersRunAsSoonAsTheyArrive
			r := newTestRunner(t, &Runner{Semantics: &sem})
			r.sigs().traps["INT"] = "echo T"
			w, ok := r.trapWatchFor(context.Background(), tc.in, tc.fd, false)
			if !ok {
				t.Fatal("no watch for a read with INT trapped")
			}
			w.inLine = true

			if _, gives := w.givesUpWithAByteThere(); gives {
				t.Fatal("the first question gave the read up with nothing waiting")
			}
			r.selfSignaled("INT")
			sig, verdict, stops := w.handlersRunWithAByteThere()
			if !stops || verdict != tc.verdict || sig != tc.sig {
				t.Errorf("the second question answered (%v, %d, stops=%v), want (%v, %d, stops=true)",
					sig, verdict, stops, tc.sig, tc.verdict)
			}
		})
	}
}
