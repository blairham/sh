// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// The placeholder a shell binary starts to lead a process substitution body's
// process group — see driver/procanchor.go for why it is this program rather
// than something already on the machine.

// TestTheArgumentMakesTheShellAPlaceholderAndNothingElse is the contract in
// one line: it runs no program, reads no startup file, and writes nothing.
//
// The source it is given is what makes that an assertion rather than a shape
// check. A shell that fell through to its ordinary routes would execute it,
// and `echo` writing to the standard output this test is watching is exactly
// what must not appear.
func TestTheArgumentMakesTheShellAPlaceholderAndNothingElse(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	done := make(chan int, 1)
	go func() {
		done <- driver.MainArgs(driver.Shell{
			Name:   "sh",
			Stdin:  read,
			Stdout: &out,
			Stderr: &out,
		}, []string{"sh", driver.AnchorArgForTest})
	}()

	// Still there while the pipe is open, which is the half that makes the
	// exit below mean something: a program that returned immediately would
	// pass an "it exits" test perfectly.
	select {
	case <-done:
		t.Fatal("the placeholder exited while its standard input was still open")
	case <-time.After(200 * time.Millisecond):
	}

	// Nothing is ever written to a placeholder's input, but if something were,
	// it must still be read and discarded rather than acted on.
	if _, err := write.Write([]byte("echo written\n")); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case status := <-done:
		if status != 0 {
			t.Errorf("the placeholder exited %d, want 0", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the placeholder did not exit when its standard input ended")
	}
	if got := out.String(); got != "" {
		t.Errorf("the placeholder wrote %q, want nothing at all", got)
	}
	_ = read.Close()
}

// And the word is read as a whole vector and nothing else, so an ordinary
// invocation that happens to mention it is still an ordinary invocation. A
// shell asked to be a placeholder is not also asked to run anything, and
// treating the word as a flag anywhere in the vector would make it one.
func TestOnlyTheWholeVectorAsksForAPlaceholder(t *testing.T) {
	var out strings.Builder
	status := driver.MainArgs(driver.Shell{
		Name:   "sh",
		Stdout: &out,
		Stderr: &out,
	}, []string{"sh", "-c", "echo " + driver.AnchorArgForTest})
	if status != 0 {
		t.Fatalf("status %d, want 0", status)
	}
	if got := out.String(); got != driver.AnchorArgForTest+"\n" {
		t.Errorf("out = %q, want %q", got, driver.AnchorArgForTest+"\n")
	}
}
