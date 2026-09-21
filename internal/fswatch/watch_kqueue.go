// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fswatch

import (
	"os"
	"sync"
	"syscall"

	"github.com/blairham/sh/internal/fdset"
)

// The kqueue half. A directory is watched by holding a descriptor on it and
// asking the kernel to report the vnode events that mean its contents moved.
//
// NOTE_WRITE is the one that matters and the rest are there because a
// directory can go away underneath a watch: a repository whose `.git` is
// renamed or removed has changed as surely as one whose index was rewritten,
// and a watcher that only heard about writes would hold a stale answer for
// ever afterwards.
const vnodeEvents = syscall.NOTE_WRITE | syscall.NOTE_DELETE | syscall.NOTE_RENAME |
	syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB

// Watcher reports that something it was pointed at has changed.
type Watcher struct {
	notify func()

	// stop is the self-pipe that ends the wait. A kqueue descriptor is
	// selectable, so the loop waits on both at once and closing the pipe is
	// what gets it out — closing the kqueue underneath a blocked wait is a
	// descriptor reused by another goroutine in the time between.
	stopR, stopW *os.File

	mu     sync.Mutex
	kq     int
	held   []*os.File
	closed bool
	done   chan struct{}
}

// Open starts a watcher that calls notify whenever a directory added to it
// changes. notify runs on the watcher's own goroutine and must not block.
func Open(notify func()) (*Watcher, error) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	stopR, stopW, err := os.Pipe()
	if err != nil {
		_ = syscall.Close(kq)
		return nil, err
	}
	w := &Watcher{notify: notify, stopR: stopR, stopW: stopW, kq: kq, done: make(chan struct{})}
	go w.wait()
	return w, nil
}

// Add points the watcher at one directory. A path that is not there is not an
// error: a repository has files that exist only during an operation, and a
// caller naming one is saying "tell me if it turns up", which this cannot do
// and must not fail over.
func (w *Watcher) Add(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil //nolint:nilerr // an absent path is nothing to watch, not a failure
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		_ = f.Close()
		return os.ErrClosed
	}
	var change syscall.Kevent_t
	syscall.SetKevent(&change, int(f.Fd()), syscall.EVFILT_VNODE, syscall.EV_ADD|syscall.EV_CLEAR)
	change.Fflags = vnodeEvents
	if _, err := syscall.Kevent(w.kq, []syscall.Kevent_t{change}, nil, nil); err != nil {
		_ = f.Close()
		return err
	}
	// Held for the life of the watcher: the registration is against the open
	// descriptor, so closing the file would take the watch with it.
	w.held = append(w.held, f)
	return nil
}

// wait is the watcher's goroutine.
func (w *Watcher) wait() {
	defer close(w.done)
	for {
		ready, _, _, err := fdset.Ready([]int{w.kq, int(w.stopR.Fd())}, nil, nil, nil)
		if err != nil {
			return
		}
		stop := false
		pending := false
		for _, fd := range ready {
			if fd == int(w.stopR.Fd()) {
				stop = true
				continue
			}
			pending = true
		}
		if pending {
			w.drain()
		}
		if stop {
			return
		}
		if pending && w.notify != nil {
			// One call however many events came out. Nothing here reports
			// what changed, so a caller told twice about one rewrite would
			// do the same work twice for no more information.
			w.notify()
		}
	}
}

// drain takes whatever is queued off the kqueue without waiting, which is
// what makes the descriptor stop being readable.
func (w *Watcher) drain() {
	events := make([]syscall.Kevent_t, 16)
	var zero syscall.Timespec
	for {
		n, err := syscall.Kevent(w.kq, nil, events, &zero)
		if n <= 0 || err != nil {
			return
		}
		if n < len(events) {
			return
		}
	}
}

// Close ends the watch. Safe to call twice, which is what lets a caller close
// on the way out of a failure as well as on the ordinary path.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	_ = w.stopW.Close()
	w.mu.Unlock()

	<-w.done

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, f := range w.held {
		_ = f.Close()
	}
	w.held = nil
	_ = w.stopR.Close()
	return syscall.Close(w.kq)
}
