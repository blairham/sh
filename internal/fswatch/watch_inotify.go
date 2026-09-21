// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fswatch

import (
	"os"
	"sync"
	"syscall"

	"github.com/blairham/sh/internal/fdset"
)

// The inotify half. One descriptor carries every watch, and each directory
// added to it is a watch descriptor on that one.
//
// The mask is everything that can mean "what I computed is no longer true".
// A repository's branch moves by a file being renamed into place, its index
// by the same, and a `.git` that is removed or moved is as much a change as
// either — a watcher that only heard about writes would hold a stale answer
// for ever afterwards.
const inotifyEvents = syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_DELETE_SELF |
	syscall.IN_MODIFY | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO |
	syscall.IN_MOVE_SELF | syscall.IN_ATTRIB | syscall.IN_CLOSE_WRITE

// Watcher reports that something it was pointed at has changed.
type Watcher struct {
	notify func()

	// stop is the self-pipe that ends the wait. The inotify descriptor is
	// selectable, so the loop waits on both at once and closing the pipe is
	// what gets it out — closing the inotify descriptor underneath a blocked
	// wait is a descriptor another goroutine has been handed by then.
	stopR, stopW *os.File

	mu     sync.Mutex
	fd     int
	closed bool
	done   chan struct{}
}

// Open starts a watcher that calls notify whenever a directory added to it
// changes. notify runs on the watcher's own goroutine and must not block.
func Open(notify func()) (*Watcher, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	stopR, stopW, err := os.Pipe()
	if err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	w := &Watcher{notify: notify, stopR: stopR, stopW: stopW, fd: fd, done: make(chan struct{})}
	go w.wait()
	return w, nil
}

// Add points the watcher at one directory. A path that is not there is not an
// error: a repository has files that exist only during an operation, and a
// caller naming one is saying "tell me if it turns up", which this cannot do
// and must not fail over.
func (w *Watcher) Add(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if _, err := os.Stat(path); err != nil {
		return nil //nolint:nilerr // an absent path is nothing to watch, not a failure
	}
	if _, err := syscall.InotifyAddWatch(w.fd, path, inotifyEvents); err != nil {
		return err
	}
	return nil
}

// wait is the watcher's goroutine.
func (w *Watcher) wait() {
	defer close(w.done)
	buf := make([]byte, 16*(syscall.SizeofInotifyEvent+syscall.NAME_MAX+1))
	for {
		ready, _, _, err := fdset.Ready([]int{w.fd, int(w.stopR.Fd())}, nil, nil, nil)
		if err != nil {
			return
		}
		stop, pending := false, false
		for _, fd := range ready {
			if fd == int(w.stopR.Fd()) {
				stop = true
				continue
			}
			pending = true
		}
		if pending {
			// Read once. select said there is something there, so this does
			// not block, and whatever is left over makes the descriptor
			// readable again on the next pass — which costs one more call
			// and never a missed event.
			if _, err := syscall.Read(w.fd, buf); err != nil && err != syscall.EINTR {
				return
			}
		}
		if stop {
			return
		}
		if pending && w.notify != nil {
			// One call however many events were in the buffer. Nothing here
			// reports what changed, so a caller told twice about one rewrite
			// would do the same work twice for no more information.
			w.notify()
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
	_ = w.stopR.Close()
	return syscall.Close(w.fd)
}
