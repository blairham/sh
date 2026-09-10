// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package fdset

import (
	"errors"
	"syscall"
	"time"
)

// ReadableNow reports whether a read on fd would return at once, waiting no
// time at all for the answer.
//
// The descriptor is not read from, which is the whole point: the caller wants
// to know whether input is waiting and a read is what would consume it.
//
// Every answer this cannot get is "yes". A descriptor past what a set can
// name cannot be asked about, and neither can one the call itself failed on,
// and in both cases the honest fallback is the one that says "go ahead" — the
// caller then does the read it was going to do, and finds out.
func ReadableNow(fd int) bool {
	var set syscall.FdSet
	if fd/nfdbits >= len(set.Bits) {
		return true
	}
	tv := syscall.Timeval{Sec: 0, Usec: 0}
	for {
		set = syscall.FdSet{}
		add(&set, fd)
		err := selectSets(fd+1, &set, nil, nil, &tv)
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived while the question was being asked. The
			// question is unchanged, so it is asked again.
			continue
		}
		if err != nil {
			return true
		}
		return has(&set, fd)
	}
}

// ReadableWithin is ReadableNow with a deadline: it waits up to d for a read
// on fd to become one that would not block, and reports whether it became one.
//
// The third answer this has and ReadableNow does not is *whether the question
// could be asked at all*. A caller that only wants to avoid blocking can treat
// "cannot ask" as "go ahead" and find out from the read; a caller implementing
// a builtin whose whole contract is a timeout cannot, because a shell that
// reported a timeout it never waited for, or read where it was asked to wait,
// is wrong in a way no later call recovers from.
//
// A negative or zero d is the same look ReadableNow takes, which is what
// `-t 0` means: ask, do not wait.
func ReadableWithin(fd int, d time.Duration) (ready, asked bool) {
	var set syscall.FdSet
	if fd < 0 || fd/nfdbits >= len(set.Bits) {
		return false, false
	}
	deadline := time.Now().Add(d)
	for {
		left := time.Until(deadline)
		if d <= 0 || left < 0 {
			left = 0
		}
		tv := timeval(left)
		set = syscall.FdSet{}
		add(&set, fd)
		err := selectSets(fd+1, &set, nil, nil, &tv)
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived mid-wait. The deadline is absolute, so the
			// question is asked again for what is left of it rather than for
			// the whole of it again.
			if d <= 0 {
				return false, true
			}
			continue
		}
		if err != nil {
			return false, false
		}
		return has(&set, fd), true
	}
}

// Wait waits until terminal has a byte or one of fds is readable, and answers
// with the descriptors among fds that are.
//
// terminal is the descriptor a key arrives on, or negative where the caller's
// input is not a descriptor at all. That is the difference between waiting and
// looking: with a terminal to wait on, this blocks until *something* happens,
// which is what lets a session sit at an idle prompt costing nothing while a
// watcher is armed. Without one there is nothing to block on that would ever
// wake for a keystroke, so it looks once and returns, and a caller that has to
// do its own read is no worse off than it was.
//
// terminalReady is the whole of what the caller needs to know about the
// terminal, and answering it separately is deliberate: a key somebody pressed
// must be able to get through a descriptor that is *permanently* readable. The
// read end of a pipe whose writer has gone is exactly that, and a caller that
// served descriptors first would never reach the key.
func Wait(terminal int, fds []int) (ready []int, terminalReady bool, err error) {
	var set syscall.FdSet
	limit := len(set.Bits) * nfdbits
	watched := make([]int, 0, len(fds))
	highest := -1
	for _, fd := range fds {
		// Out of range, or closed, or never opened. Dropped here rather than
		// left for the kernel to complain about, because the complaint is
		// about the *call* and not about the descriptor: one dead entry would
		// otherwise cost every live one beside it its turn, which is the bug
		// where a caller's second watcher silently stops working because the
		// first one's descriptor was closed.
		if fd < 0 || fd >= limit || !live(fd) {
			continue
		}
		add(&set, fd)
		watched = append(watched, fd)
		highest = max(highest, fd)
	}
	waiting := terminal >= 0 && terminal < limit
	if waiting {
		add(&set, terminal)
		highest = max(highest, terminal)
	}
	if highest < 0 {
		return nil, false, nil
	}
	// A nil timeout blocks; a zero one looks and returns. See above.
	var timeout *syscall.Timeval
	if !waiting {
		timeout = &syscall.Timeval{Sec: 0, Usec: 0}
	}
	if err := selectSets(highest+1, &set, nil, nil, timeout); err != nil {
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived while we waited. Nothing is ready and nothing
			// is wrong: the caller goes back to reading, which is where it
			// already handles an interrupted read.
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, fd := range watched {
		if has(&set, fd) {
			ready = append(ready, fd)
		}
	}
	return ready, waiting && has(&set, terminal), nil
}

// The two operations on a descriptor set. Neither is range-checked: every
// caller above has already decided the descriptor is one the set can name.
func add(set *syscall.FdSet, fd int) {
	set.Bits[fd/nfdbits] |= 1 << (uint(fd) % nfdbits)
}

func has(set *syscall.FdSet, fd int) bool {
	return set.Bits[fd/nfdbits]&(1<<(uint(fd)%nfdbits)) != 0
}

// live reports whether a descriptor is open in this process.
//
// The cheapest question that distinguishes the two, and it changes nothing
// about the descriptor. See Wait for why it is asked at all.
func live(fd int) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
	return errno == 0
}

// Ready waits until a descriptor is ready in the way the set it was named in
// asks about, and answers which ones are, set by set.
//
// The general form of the two questions above, and the reason it is here
// rather than beside its caller: read, write and error are one call to the
// kernel with three sets in it, and a caller that asked them one at a time
// would be waiting on the first while the second became ready.
//
// timeout is how long to wait, and **nil means forever** — the distinction a
// builtin whose timeout is optional has to be able to make, since a zero
// duration already means "look and come back". A wait with nothing in any set
// and no timeout is a wait that never ends, which is what a caller asking for
// exactly that has asked for.
//
// A descriptor that is not open, or is past what a set can name, is **dropped
// rather than complained about**, and it is dropped before the call rather
// than after it. The kernel's answer to a set holding one dead entry is a
// complaint about the *call*, so every live descriptor beside it would lose
// its turn — the same reasoning [Wait] gives, and the same bug: a caller's
// second watcher silently stops working because the first one's descriptor was
// closed. A caller that wanted to hear about a bad descriptor still does, by
// the descriptor never being reported ready.
func Ready(read, write, except []int, timeout *time.Duration) (r, w, e []int, err error) {
	var probe syscall.FdSet
	limit := len(probe.Bits) * nfdbits
	var sets [3]syscall.FdSet
	given := [3][]int{read, write, except}
	var watched [3][]int
	highest := -1
	for i, fds := range given {
		for _, fd := range fds {
			if fd < 0 || fd >= limit || !live(fd) {
				continue
			}
			add(&sets[i], fd)
			watched[i] = append(watched[i], fd)
			highest = max(highest, fd)
		}
	}
	deadline := time.Time{}
	if timeout != nil {
		deadline = time.Now().Add(*timeout)
	}
	for {
		var tv *syscall.Timeval
		if timeout != nil {
			left := time.Until(deadline)
			if left < 0 {
				left = 0
			}
			v := timeval(left)
			tv = &v
		}
		// A fresh copy each turn: select rewrites the sets it is given, so
		// the ones built above are the question and these are the answer.
		asked := sets
		callErr := selectSets(highest+1, &asked[0], &asked[1], &asked[2], tv)
		if errors.Is(callErr, syscall.EINTR) {
			// A signal arrived mid-wait. The deadline is absolute, so what is
			// left of it is what the question is asked for; a wait with no
			// deadline is asked again whole, which is what "forever" means.
			continue
		}
		if callErr != nil {
			return nil, nil, nil, callErr
		}
		for i := range watched {
			for _, fd := range watched[i] {
				if has(&asked[i], fd) {
					switch i {
					case 0:
						r = append(r, fd)
					case 1:
						w = append(w, fd)
					default:
						e = append(e, fd)
					}
				}
			}
		}
		return r, w, e, nil
	}
}
