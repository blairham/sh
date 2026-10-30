// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package sigbook keeps the book of which channels in this program have asked
// os/signal for which signals, so that whatever has had to take a signal off
// the whole process can put back what everybody else had.
//
// A child inherits only one disposition from the process that starts it, and
// that is SIG_IGN: exec resets every handled signal to its default. So a shell
// whose child is owed an ignore it does not hold itself — one a subshell set,
// an asynchronous list's INT and QUIT — has to put the ignore on the
// process for the length of the fork. signal.Ignore is the only way to, and it
// takes every subscription the process has for the signal away with it: the
// trap handlers of every Runner in the program, the interactive session's ^C
// catcher, a terminal's resize watcher. os/signal cannot say who those were
// afterwards, so this book can.
//
// Every subscription in this module that lives longer than a single call goes
// through Notify and Stop here, and PutBack is what the end of such a window
// calls. A channel subscribed only for the length of one call — a bridge, a
// drain that is stopped on the next line — has nothing to put back and goes to
// os/signal directly.
package sigbook

import (
	"os"
	"os/signal"
	"sync"
)

// book is every channel's standing subscriptions. One for the process, because
// a disposition is the process's and so is what an ignore takes away.
var book struct {
	mu   sync.Mutex
	subs map[chan<- os.Signal]map[os.Signal]bool
}

// Notify is signal.Notify, written down.
//
// No signals is not "every signal" here, as it is to os/signal: nothing in
// this program asks for that, and a book entry that stood for every signal
// would put a subscription back on signals nobody asked for.
func Notify(c chan<- os.Signal, sigs ...os.Signal) {
	if len(sigs) == 0 {
		return
	}
	book.mu.Lock()
	defer book.mu.Unlock()
	signal.Notify(c, sigs...)
	if book.subs == nil {
		book.subs = map[chan<- os.Signal]map[os.Signal]bool{}
	}
	want := book.subs[c]
	if want == nil {
		want = map[os.Signal]bool{}
		book.subs[c] = want
	}
	for _, sig := range sigs {
		want[sig] = true
	}
}

// Stop is signal.Stop, written down: c is off every signal and out of the book.
func Stop(c chan<- os.Signal) {
	book.mu.Lock()
	defer book.mu.Unlock()
	signal.Stop(c)
	delete(book.subs, c)
}

// Forget takes sigs out of c's entry without calling os/signal, for a caller
// that has just had the process take them away and no longer wants them back:
// a shell that owns its process and has ignored or reset the signal itself.
func Forget(c chan<- os.Signal, sigs ...os.Signal) {
	book.mu.Lock()
	defer book.mu.Unlock()
	want := book.subs[c]
	for _, sig := range sigs {
		delete(want, sig)
	}
	if len(want) == 0 {
		delete(book.subs, c)
	}
}

// PutBack subscribes every channel in the book again to each of sigs it had
// asked for, for a caller that has just ignored them on the whole process for
// a moment and is handing them back.
//
// Notify on a subscription that is still there changes nothing, so a signal
// the window never took is safe to name.
func PutBack(sigs ...os.Signal) {
	book.mu.Lock()
	defer book.mu.Unlock()
	for c, want := range book.subs {
		var again []os.Signal
		for _, sig := range sigs {
			if want[sig] {
				again = append(again, sig)
			}
		}
		if len(again) > 0 {
			signal.Notify(c, again...)
		}
	}
}
