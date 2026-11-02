// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// A parent's redirection to a regular file lands on the lowest free number and
// is moved out of the way a moment later, and a park in a body already running
// found it there in that moment and stepped over it: `cat <(echo <(true)
// <(true) <(true)) > out`, everything below the base taken, published `10 12
// 11` or `10 12 13` in 70 runs of 300 on darwin, where a forking shell's body
// holds nothing its parent opens after the fork (#7335).
//
// Held open here, so the order is not a race: the body waits on a FIFO the
// test writes to only once the parent's file is open and not yet moved, and
// the hold lasts until the body's park has missed on the file's number or half
// a second has passed. With the lock taken out of the open and the hold kept,
// three runs of three published `10 12 13` or `10 12 11`.
func TestAParentsFileIsOpenedOutOfAParksWay(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skipf("no cat on PATH: %v", err)
	}
	dir := t.TempDir()
	gate, out := filepath.Join(dir, "gate"), filepath.Join(dir, "out")
	if err := syscall.Mkfifo(gate, 0o600); err != nil {
		t.Fatal(err)
	}
	// Both ways, so that neither the body's open nor this one waits.
	g, err := os.OpenFile(gate, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	fillBelow(t, 10)
	free := freeFdsAlong(t, 10, 200, 3)
	missed := make(chan int, 64)
	defer WhenAWishMissesForTest(func(n int) {
		select {
		case missed <- n:
		default:
		}
	})()
	held := 0
	defer WhileAFileIsLowForTest(func(path string, fd int) {
		if path != out {
			return
		}
		held++
		if _, err := g.WriteString("go\n"); err != nil {
			t.Errorf("letting the body go: %v", err)
			return
		}
		deadline := time.After(500 * time.Millisecond)
		for {
			select {
			case n := <-missed:
				if n == fd {
					return
				}
			case <-deadline:
				return
			}
		}
	})()
	runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers,
		cat+` <(read -r _ <"`+gate+`"; echo <(true) <(true) <(true)) >"`+out+`"`)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := substNumbers(t, string(b)); !sameFds(got, free) || held != 1 {
		t.Errorf("got %v with the file held low %d times, want %v once — the parent's "+
			"file was counted in the body's table", got, held, free)
	}
}

// And the lock that keeps a park out of that window is never held across an
// open that waits. A FIFO's open for writing waits for its reader; a park
// queued behind a lock held across it would hold up every start behind the
// park, and the reader here is a start. So `> fifo` with its reader arriving
// late, while a body parks, has to finish — whether the name was a FIFO all
// along or became one between the look that chose the road and the open,
// which only the non-blocking open answers. Without the flag the second shape
// deadlocked, and with neither the look nor the flag both did.
//
// In a child, so that a regression takes only the child's lock with it.
func TestAFileOpenThatWaitsHoldsUpNoPark(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skipf("no cat on PATH: %v", err)
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("no sleep on PATH: %v", err)
	}
	for _, swapped := range []bool{false, true} {
		name := "a fifo"
		if swapped {
			name = "a file that becomes a fifo after the look"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p, got := filepath.Join(dir, "p"), filepath.Join(dir, "got")
			if swapped {
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				done := false
				defer BetweenAFilesStatAndOpenForTest(func(path string) {
					if path != p || done {
						return
					}
					done = true
					if err := os.Remove(p); err != nil {
						t.Errorf("removing the file: %v", err)
					}
					if err := syscall.Mkfifo(p, 0o600); err != nil {
						t.Errorf("making the fifo: %v", err)
					}
				})()
			} else if err := syscall.Mkfifo(p, 0o600); err != nil {
				t.Fatal(err)
			}
			src := `{ ` + sleep + ` 0.2; ` + cat + ` "` + p + `" >"` + got + `"; } & ` +
				cat + ` <(` + sleep + ` 0.05; echo <(true) <(true) <(true)) >"` + p + `"; wait`
			type result struct{ out, errs string }
			ran := make(chan result, 1)
			go func() {
				out, errs := runOwnDescriptorsAllowingErrors(t, SavedStreamsAndPipeReadEndsTakeNumbers, src)
				ran <- result{out, errs}
			}()
			select {
			case res := <-ran:
				if res.errs != "" {
					t.Fatalf("stderr: %s", res.errs)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("still running after 20s — the open waiting for its reader held up " +
					"the park, and the park the reader's start")
			}
			b, err := os.ReadFile(got)
			if err != nil {
				t.Fatal(err)
			}
			if n := substNumbers(t, string(b)); len(n) != 3 {
				t.Errorf("the reader got %v, want three substitution paths", n)
			}
		})
	}
}
