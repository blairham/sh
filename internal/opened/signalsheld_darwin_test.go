// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// currentMask reads this thread's signal mask without changing it.
func currentMask(t *testing.T) sigset {
	t.Helper()
	var none, old sigset
	if err := threadSigmask(sigBlock, &none, &old); err != nil {
		t.Fatalf("__pthread_sigmask: %v", err)
	}
	return old
}

// Inside the call every signal is held on the thread making it, and after it
// the thread's mask is what it was.
//
// Read back rather than trusted. The mask is set by a raw system call number,
// and a wrong number or a wrong argument order answers with an error that
// WithSignalsHeld deliberately swallows — so a broken hold would look like a
// working one everywhere but under load, which is where #5763 was found.
func TestSignalsAreHeldForTheLengthOfTheCall(t *testing.T) {
	// Locked around the call too, so the mask read after it is the mask of
	// the thread the call ran on.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	before := currentMask(t)
	if before == allSignals {
		t.Fatalf("mask before the call already holds everything (%#x); the test cannot tell", before)
	}
	var inside sigset
	if _, err := WithSignalsHeld(func() (int, error) {
		inside = currentMask(t)
		return 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	// Every signal but the two the kernel will not let a thread hold.
	want := allSignals &^ (1<<(syscall.SIGKILL-1) | 1<<(syscall.SIGSTOP-1))
	if inside != want {
		t.Errorf("mask inside the call = %#x, want every signal held (%#x)", inside, want)
	}
	if after := currentMask(t); after != before {
		t.Errorf("mask after the call = %#x, want it back at %#x", after, before)
	}
}

// A FIFO opened for reading while this process is being sent SIGCHLD still
// meets the writer that opened, wrote and closed it.
//
// The shape of #5763, outside the shell: the open waits for its writer, the
// writer comes and goes, and a child stopped and continued in a loop keeps a
// SIGCHLD arriving at whichever thread is not holding it. Without the hold
// this lost the rendezvous about once in a hundred tries on an idle machine
// and the reader waited for a writer that had already been, which is why the
// count is in the hundreds; a lost one is reported rather than left hanging.
func TestAFifoReaderMeetsItsWriterThroughSignals(t *testing.T) {
	if testing.Short() {
		t.Skip("a stress loop")
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skip("no sleep to signal:", err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = child.Process.Signal(syscall.SIGSTOP)
			_ = child.Process.Signal(syscall.SIGCONT)
		}
	}()

	dir := t.TempDir()
	openers := map[string]func(string) (*os.File, error){
		// The route a redirection takes with no gate.
		"held": func(p string) (*os.File, error) {
			return WithSignalsHeld(func() (*os.File, error) { return os.OpenFile(p, os.O_RDONLY, 0) })
		},
		// The route a gated one takes, which walks with openat.
		"walked": func(p string) (*os.File, error) {
			r, err := Open(p, os.O_RDONLY, 0)
			return r.File, err
		},
	}
	for name, open := range openers {
		t.Run(name, func(t *testing.T) {
			lost := 0
			for i := range 200 {
				p := filepath.Join(dir, fmt.Sprintf("%s-%d", name, i))
				if err := syscall.Mkfifo(p, 0o600); err != nil {
					t.Fatal(err)
				}
				got := make(chan string, 1)
				go func() {
					f, err := open(p)
					if err != nil {
						got <- "open: " + err.Error()
						return
					}
					defer f.Close()
					b := make([]byte, 8)
					n, _ := f.Read(b)
					got <- string(b[:n])
				}()
				// The writer is in this process too, as `echo >f` is in the
				// shell, so it opens the same way: a writer can lose its
				// reader exactly as a reader loses its writer.
				wrote := make(chan error, 1)
				go func() {
					w, err := WithSignalsHeld(func() (*os.File, error) { return os.OpenFile(p, os.O_WRONLY, 0) })
					if err == nil {
						_, err = w.WriteString("x\n")
						_ = w.Close()
					}
					wrote <- err
				}()
				select {
				case s := <-got:
					if s != "x\n" {
						t.Fatalf("try %d: reader got %q, want %q", i, s, "x\n")
					}
					if err := <-wrote; err != nil {
						t.Fatalf("try %d: writer: %v", i, err)
					}
				case <-time.After(5 * time.Second):
					lost++
					// Let whichever end is stranded go, so the test ends.
					if f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
						defer f.Close()
					}
					if f, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
						_ = f.Close()
					}
					<-got
					<-wrote
				}
			}
			if lost != 0 {
				t.Errorf("%d of 200 opens never met the peer that had opened, written and closed", lost)
			}
		})
	}
}
