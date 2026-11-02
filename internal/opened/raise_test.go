// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// raiseFloor is where testRaise moves descriptors: far above anything this
// test process holds, so a moved number is told from an unmoved one by
// looking at it.
const raiseFloor = 400

// testRaise is a Raise that records what it was handed and where it put it,
// and whether its hold was held at the time.
type testRaise struct {
	hold    sync.Mutex
	mu      sync.Mutex
	low     []int
	moved   []int
	outside int
}

func (tr *testRaise) raise() Raise {
	return Raise{Hold: &tr.hold, Up: func(fd int) (int, bool) {
		if tr.hold.TryLock() {
			tr.hold.Unlock()
			tr.mu.Lock()
			tr.outside++
			tr.mu.Unlock()
		}
		got, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_DUPFD_CLOEXEC, raiseFloor)
		if errno != 0 {
			return -1, false
		}
		tr.mu.Lock()
		tr.low = append(tr.low, fd)
		tr.moved = append(tr.moved, int(got))
		tr.mu.Unlock()
		return int(got), true
	}}
}

func fdOpen(fd int) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	return errno == 0
}

func fdOf(t *testing.T, f *os.File) int {
	t.Helper()
	conn, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	n := -1
	if err := conn.Control(func(fd uintptr) { n = int(fd) }); err != nil {
		t.Fatal(err)
	}
	return n
}

// A raised walk moves every descriptor it makes — the root, each directory
// on the way and the file — inside the hold, so that when anything outside
// the hold runs, the check included, none of them is on the number it was
// opened on. The check is asked twice here, before the creation and after
// the open, and each time the hold is free and every number the walk was
// handed low is closed. Without the move, or with it made after the hold is
// let go, the walk's directories would still be low when the creation is
// asked about.
func TestARaisedWalkHoldsNothingLowOutsideItsHold(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "there"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("there", filepath.Join(deep, "link")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		path  string
		flags int
		asks  int
	}{
		{"a creation", filepath.Join(deep, "new"), os.O_WRONLY | os.O_CREATE | os.O_TRUNC, 1},
		{"an existing file", filepath.Join(deep, "there"), os.O_RDONLY, 1},
		{"a link", filepath.Join(deep, "link"), os.O_RDONLY, 1},
		{"a directory", deep + "/", os.O_RDONLY, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			tr := &testRaise{}
			asked := 0
			check := func(Reached) error {
				asked++
				if !tr.hold.TryLock() {
					t.Error("the check was asked with the hold held")
				} else {
					tr.hold.Unlock()
				}
				tr.mu.Lock()
				defer tr.mu.Unlock()
				for _, fd := range tr.low {
					if fdOpen(fd) && !contains(tr.moved, fd) {
						t.Errorf("the walk's %d is still open when the check is asked", fd)
					}
				}
				return nil
			}
			f, raised, err := VerifiedRaised(c.path, c.flags, 0o600, check, tr.raise())
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if !raised {
				t.Error("raised is false for a file the walk made")
			}
			if tr.outside != 0 {
				t.Errorf("%d descriptors were moved outside the hold", tr.outside)
			}
			if n := fdOf(t, f); n < raiseFloor {
				t.Errorf("the file is on %d, below where it was moved to", n)
			}
			if asked < c.asks {
				t.Errorf("the check was asked %d times, want at least %d", asked, c.asks)
			}
			// The root, three directories under the temporary one and the
			// file: the temporary directory adds its own.
			if len(tr.moved) < 5 {
				t.Errorf("Up moved %d descriptors, want one per directory and the file", len(tr.moved))
			}
			for _, fd := range tr.moved {
				if fd != fdOf(t, f) && fdOpen(fd) {
					t.Errorf("the walk left its %d open", fd)
				}
			}
		})
	}
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// A refusal leaves nothing behind either: the check refuses the creation, or
// refuses what an existing name reached, and every descriptor the walk made
// and moved is closed, whichever it was.
func TestARefusedRaisedWalkLeavesNothingOpen(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "there"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	no := errors.New("refused")
	for _, c := range []struct {
		name  string
		path  string
		flags int
	}{
		{"a creation", filepath.Join(dir, "new"), os.O_WRONLY | os.O_CREATE},
		{"an existing file", filepath.Join(dir, "there"), os.O_WRONLY | os.O_TRUNC},
	} {
		t.Run(c.name, func(t *testing.T) {
			tr := &testRaise{}
			_, _, err := VerifiedRaised(c.path, c.flags, 0o600, func(Reached) error { return no }, tr.raise())
			if !errors.Is(err, no) {
				t.Fatalf("got %v, want the check's own refusal", err)
			}
			if len(tr.moved) == 0 {
				t.Fatal("Up moved nothing — the walk was not raised")
			}
			for _, fd := range tr.moved {
				if fdOpen(fd) {
					t.Errorf("the refused walk left %d open", fd)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "new")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the refused creation made the file: %v", err)
			}
		})
	}
}

// An open that waits for a peer is never made inside the hold, because what
// it waits for can be waiting for the hold. The peer here takes the hold
// before it opens, so an open waiting inside it never returns. A FIFO from the
// start is looked at and opened outside; a regular file swapped for a FIFO
// after the look is opened inside without waiting, and that is what the
// non-blocking open answers. Without the flag, the swapped shape deadlocked
// here; without the look as well, both did.
func TestARaisedOpenThatWaitsIsMadeOutsideTheHold(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	for _, c := range []struct {
		name    string
		swapped bool
		flags   int
	}{
		{"a fifo, for writing", false, os.O_WRONLY},
		{"a fifo, for reading", false, os.O_RDONLY},
		{"a file that becomes a fifo after the look, for writing", true, os.O_WRONLY},
		{"a file that becomes a fifo after the look, for reading", true, os.O_RDONLY},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "p")
			if c.swapped {
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				swapped := false
				defer BetweenTheLookAndTheOpenForTest(func(string) {
					if swapped {
						return
					}
					swapped = true
					if err := os.Remove(p); err != nil {
						t.Error(err)
					}
					if err := syscall.Mkfifo(p, 0o600); err != nil {
						t.Error(err)
					}
				})()
			} else if err := syscall.Mkfifo(p, 0o600); err != nil {
				t.Fatal(err)
			}
			tr := &testRaise{}
			peerFlags := os.O_RDONLY
			if c.flags == os.O_RDONLY {
				peerFlags = os.O_WRONLY
			}
			peer := make(chan *os.File, 1)
			var peerHeld atomic.Bool
			go func() {
				time.Sleep(50 * time.Millisecond)
				// The hold is taken and let go before the open, never
				// across it: the open below waits for the one under test.
				tr.hold.Lock()
				peerHeld.Store(true)
				tr.hold.Unlock()
				f, err := os.OpenFile(p, peerFlags, 0)
				if err != nil {
					t.Error(err)
				}
				peer <- f
			}()
			type result struct {
				f   *os.File
				err error
			}
			done := make(chan result, 1)
			go func() {
				f, _, err := VerifiedRaised(p, c.flags, 0, func(Reached) error { return nil }, tr.raise())
				done <- result{f, err}
			}()
			select {
			case res := <-done:
				if res.err != nil {
					t.Fatal(res.err)
				}
				defer res.f.Close()
				if !peerHeld.Load() {
					// An open of a FIFO waits for its peer, and this one
					// came back before the peer had begun to open: the
					// non-blocking open's answer was kept.
					t.Error("the fifo's open did not wait for its peer")
				}
				var st syscall.Stat_t
				if err := syscall.Fstat(fdOf(t, res.f), &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
					t.Errorf("the open reached mode %o (%v), want the fifo", st.Mode, err)
				}
				if fl, err := fcntlFlags(fdOf(t, res.f)); err != nil || fl&syscall.O_NONBLOCK != 0 {
					t.Errorf("the fifo is handed on with flags %#x (%v), want it blocking", fl, err)
				}
			case <-time.After(10 * time.Second):
				// Let the stuck open go, so the test binary is not left with
				// it: a non-blocking peer satisfies it at once.
				if g, err := os.OpenFile(p, peerFlags|syscall.O_NONBLOCK, 0); err == nil {
					defer g.Close()
				}
				t.Fatal("still waiting after 10s — the open waited for its peer inside the hold, " +
					"and the peer for the hold")
			}
			if f := <-peer; f != nil {
				_ = f.Close()
			}
			if !peerHeld.Load() {
				t.Error("the peer never took the hold, so nothing here asked whether it was free")
			}
		})
	}
}

func fcntlFlags(fd int) (int, error) {
	fl, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFL, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(fl), nil
}
