// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/opened"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// allowEverything is a Gate that says yes, so that a runner's opens take the
// Gate's walk and nothing is refused.
var allowEverything = GateFunc(func(context.Context, Action) Decision { return Allow })

// canonicalTempDir is a temporary directory by the name the walk reports for
// it, with no symbolic link on the way — a Mac's is under /var, which is one —
// so that a name a hook is handed can be compared with one built here.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// Under a Gate a redirection's open is a walk: a descriptor for every
// directory on the way and then the file, each on the lowest free number, and
// a park in a body already running found them there and stepped over them.
// `cat <(echo <(true) <(true) <(true)) > out` with everything below the base
// taken and a Gate allowing everything published `10 21 11` or `10 20 11` in
// 90 runs of 300 on darwin, where a forking shell's body holds nothing its
// parent opens after the fork (#7341).
//
// Held open here, at the file and at a directory the walk passes through, so
// that the order is not a race: the body waits on a FIFO the test writes to
// only once the walk's descriptor is open and not yet moved, and the hold
// lasts until the body's park has missed on that number or half a second has
// passed. With the walk's lock taken out and the hold kept, both shapes
// published a number past one that was free.
func TestAGatedWalkIsMadeOutOfAParksWay(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skipf("no cat on PATH: %v", err)
	}
	for _, at := range []string{"the file", "a directory on the way"} {
		t.Run(at, func(t *testing.T) {
			dir := canonicalTempDir(t)
			gate, sub := filepath.Join(dir, "gate"), filepath.Join(dir, "sub")
			out := filepath.Join(sub, "out")
			if err := os.Mkdir(sub, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(gate, 0o600); err != nil {
				t.Fatal(err)
			}
			// Both ways, so that neither the body's open nor this one waits.
			g, err := os.OpenFile(gate, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			// After the FIFO, so that it is below the base with the rest.
			fillBelow(t, 10)
			free := freeFdsAlong(t, 10, 200, 3)
			target := out
			if at != "the file" {
				target = sub
			}
			missed := make(chan int, 64)
			defer WhenAWishMissesForTest(func(n int) {
				select {
				case missed <- n:
				default:
				}
			})()
			held := 0
			defer opened.WhileLowForTest(func(name string, fd int) {
				if name != target {
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
			_, errs := runOwnDescriptorsGated(t, SavedStreamsAndPipeReadEndsTakeNumbers,
				cat+` <(read -r _ <"`+gate+`"; echo <(true) <(true) <(true)) >"`+out+`"`, nil, allowEverything)
			if errs != "" {
				t.Fatalf("stderr: %s", errs)
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if got := substNumbers(t, string(b)); !sameFds(got, free) || held != 1 {
				t.Errorf("got %v with %s held low %d times, want %v once — the walk's "+
					"descriptor was counted in the body's table", got, at, held, free)
			}
		})
	}
}

// And the lock the walk takes is never held across an open that waits, under
// a Gate as without one: `> fifo` with its reader arriving late, while a body
// parks, has to finish — whether the name was a FIFO all along or became one
// between the walk's look and its open, which only the non-blocking open
// answers. See TestAFileOpenThatWaitsHoldsUpNoPark, which is this without a
// Gate. Without the flag the second shape deadlocked; with neither the look
// nor the flag, both did.
//
// In a child, so that a regression takes only the child's lock with it.
func TestAGatedFileOpenThatWaitsHoldsUpNoPark(t *testing.T) {
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
			dir := canonicalTempDir(t)
			p, got := filepath.Join(dir, "p"), filepath.Join(dir, "got")
			looked := false
			if swapped {
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				defer opened.BetweenTheLookAndTheOpenForTest(func(name string) {
					if name != p || looked {
						return
					}
					looked = true
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
				out, errs := runOwnDescriptorsGated(t, SavedStreamsAndPipeReadEndsTakeNumbers, src, nil, allowEverything)
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
			if swapped && !looked {
				t.Fatal("the walk never looked at the name, so nothing was swapped")
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

// A policy that refuses what a name reached still refuses it when the walk is
// made out of a park's way, in the same words and with nothing left behind:
// not the file a creation would have made, not a byte of a file it would have
// read, and not a descriptor. Compared with the same runner's ordinary road —
// a dialect without process substitution, where nothing parks and the walk
// moves nothing — which has to say exactly the same thing.
func TestAGatedWalkThatIsRefusedStillRefuses(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	dir := canonicalTempDir(t)
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "there"), []byte("SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	refuseSecret := GateFunc(func(_ context.Context, a Action) Decision {
		if a.Path == secret || strings.HasPrefix(a.Path, secret+"/") {
			return Deny
		}
		return Allow
	})
	for _, c := range []struct{ name, src string }{
		{"a creation through a link", `echo x >"` + dir + `/link/new"; echo status $?`},
		{"a read through a link", `read -r l <"` + dir + `/link/there"; echo status $? "[$l]"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			type answer struct {
				out, errs string
				lows      int
				leaked    []int
			}
			run := func(substitutes bool) answer {
				t.Helper()
				d := syntax.Core()
				f, err := syntax.Parse(c.src, d)
				if err != nil {
					t.Fatal(err)
				}
				lang := syntax.Core()
				lang.ProcessSubstitution = substitutes
				out, errs := &strings.Builder{}, &strings.Builder{}
				r := newTestRunner(t, &Runner{
					Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &lang,
					Stdout: out, Stderr: errs, Gate: refuseSecret,
				})
				// Once to settle whatever a runner keeps for itself, and
				// then the run that is measured.
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				errs.Reset()
				lows := 0
				restore := opened.WhileLowForTest(func(string, int) { lows++ })
				before := openFds(512)
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatal(err)
				}
				after := openFds(512)
				restore()
				var leaked []int
				for fd := range after {
					if !before[fd] {
						leaked = append(leaked, fd)
					}
				}
				slices.Sort(leaked)
				return answer{out.String(), errs.String(), lows, leaked}
			}
			raised, ordinary := run(true), run(false)
			if raised.lows == 0 || ordinary.lows != 0 {
				t.Fatalf("the walk moved %d descriptors with process substitution and %d without, "+
					"want some and none — the two roads are not the ones being compared", raised.lows, ordinary.lows)
			}
			if !strings.Contains(raised.errs, "refused") {
				t.Errorf("stderr is %q, want a refusal", raised.errs)
			}
			if raised.out != ordinary.out || raised.errs != ordinary.errs {
				t.Errorf("out of a park's way: %q, %q\nthe ordinary road: %q, %q",
					raised.out, raised.errs, ordinary.out, ordinary.errs)
			}
			if strings.Contains(raised.out, "SECRET") || raised.out == "" {
				t.Errorf("stdout is %q, want a failed redirection and nothing of the file", raised.out)
			}
			if len(raised.leaked) != 0 {
				t.Errorf("descriptors %v were open after the refusal and not before", raised.leaked)
			}
			if _, err := os.Lstat(filepath.Join(secret, "new")); !os.IsNotExist(err) {
				t.Errorf("the refused creation made the file: %v", err)
			}
		})
	}
}

// openFds is the set of descriptors below limit this process holds.
func openFds(limit int) map[int]bool {
	fds := map[int]bool{}
	for fd := range limit {
		if fdIsOpen(fd) {
			fds[fd] = true
		}
	}
	return fds
}
