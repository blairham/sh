// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/interp"
)

// The observer role: interp.Sink, remoted.
//
// It is the second and last of the two roles, and it is the cheap one, because
// the serialization already exists. internal/event is the JSON Lines schema at
// version 1 with its five stability rules, written as a shared contract rather
// than as a feature of any one consumer — an audit file reads it, an agent
// protocol reads it, and this is the third. So the role adds a notification and
// a buffer and no new vocabulary at all.
//
// # Why this role is remotable where the gate role is not
//
// The two look symmetrical from a distance and they are not, and the whole of
// the difference is in the signature. Gate.Allow returns a Decision; Sink.Emit
// returns nothing. docs/design/plugins.md refuses the gate role because a
// remote decision has no third answer available — fail closed and a hung plugin
// bricks the shell, fail open and killing the plugin is a privilege escalation.
// An observer is never asked anything, so there is no answer to be missing.
// What a slow observer costs is a record, and a lost record is a thing this
// schema can *say*: seq exists so that "a consumer that has records 1..40 and
// then 42 knows it lost one".
//
// That is the trade this file is built around, and it is the reason a bounded
// buffer that drops is honest here and would be a lie in the gate role.
//
// # Nothing here may block the shell
//
// A Sink is called synchronously on the goroutine that emitted, which is the
// interpreter's own — mid-command, mid-pipeline, mid-background-job. So Emit
// does a conversion, takes a lock it holds for a handful of instructions, and
// makes one non-blocking channel send. It never touches the plugin, never
// touches the connection and has no path on which it can wait for a peer.
//
// The plugin is fed by a goroutine of this file's own, and what that goroutine
// blocks on — a write to the plugin's standard input — is a place the design
// already accounts for: a plugin that lets that stream back up is a plugin that
// is not reading its own requests either, so it had already stopped serving the
// protocol. The host's answer is the one it already had, which is that Close
// takes the stream away.

// observerBuffer is how many records may be waiting for the plugin.
//
// A number rather than "enough", because a bound that is not a number is not a
// bound. What it has to cover is a plugin that is momentarily busy while a
// command emits a burst: an ordinary command is two records, but a glob stats
// every entry it descends past and a PATH search stats a candidate per
// directory, and each of those is a record. A thousand is comfortably more than
// any single command a person types produces and is a few hundred kilobytes at
// worst, which is a cost a shell can carry without asking.
//
// When it is not enough the consumer is told, which is the property that makes
// choosing a number defensible at all. It is not a guess about how fast plugins
// are; it is a guess about how long a plugin may stall before the gap in seq
// stops being an inconvenience and starts being the record.
const observerBuffer = 1024

// observer is one plugin's view of the event stream.
//
// The channel is the whole of the flow control. There is no deadline anywhere
// in this file and there must not be: a deadline on a record would report
// something other than what happened — #493's rule, and the same one
// repl.Completer's doc comment applies to a Tab — where a drop reports exactly
// what happened, in a field the schema already has.
type observer struct {
	h *Host

	// mu covers seq and the send together, and the together is the point. Two
	// goroutines emitting would otherwise number their records under the lock
	// and enqueue them in the other order, and a consumer would see seq going
	// backwards on a stream whose own documentation calls it a total order of
	// emission. Holding it across the send is free because the send cannot
	// block: it has a default.
	mu   sync.Mutex
	seq  int64
	recs chan event.Record

	// closing is shut by Close and says the shell is ending: deliver what has
	// already been numbered and then go.
	//
	// Delivered rather than abandoned, and this is a correction to
	// docs/design/plugins.md, which said the write channel is "abandoned on
	// shutdown rather than drained". That rule is about not letting a plugin
	// choose how long a shutdown takes, and it is right about that; taken
	// literally here it would make the role useless for the commonest shell
	// there is. `sh -c cmd` emits every record it will ever emit and then
	// exits, so an observer that abandoned its buffer would see almost nothing,
	// and the records it lost would be the ones about the command that was
	// actually run.
	//
	// The rule is kept where it bites: the drain is bounded by flushWait, and
	// what the bound protects against is exactly what the rule was written
	// against — a plugin that has stopped reading, where the write blocks and
	// no amount of waiting helps. Close takes the stream away after it, which
	// is the unblocking event the host controls.
	closing chan struct{}
	// done is closed when the feed goroutine has gone, so Close can wait for
	// it. A goroutine per plugin that outlived its host is the leak class #690
	// is this repository's standing example of.
	done chan struct{}

	// now is the clock, injectable only from a test in this package. A stream
	// whose times a caller could choose would be the same mistake
	// event.Encoder refuses to make.
	now func() time.Time
}

// newObserver makes one. The feed is started by the caller, which is the
// handshake, once the plugin's surface has been accepted.
func newObserver(h *Host) *observer {
	return &observer{
		h:       h,
		recs:    make(chan event.Record, observerBuffer),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
		now:     time.Now,
	}
}

// Emit is interp.Sink. It runs on whichever goroutine emitted the event and it
// does not block, ever.
//
// The order of what it does matters in one place and is otherwise arbitrary.
// The sequence number is taken *before* the send is attempted, so a record that
// is dropped has still consumed its number and the gap is visible to the
// plugin. Numbering on the way out instead would produce a stream that is
// contiguous and incomplete, which is a stream that lies — and it is the one
// mutation of this file that changes nothing a casual test can see.
func (o *observer) Emit(_ context.Context, e interp.Event) {
	select {
	case <-o.done:
		// The feed has gone: the plugin is dead or the host is closing. There
		// is nobody to number records for, and an unread buffer that goes on
		// filling is memory held for a reader that will never come.
		//
		// Not covered, and correctly: removing it is a mutant that survives,
		// because what it saves is a conversion and a lock per event on a
		// stream nobody is reading and it changes no answer anywhere. It is a
		// cost rather than a behavior, which is the one kind of line a test
		// should not be written to pin.
		return
	default:
	}
	r := event.Of(e)
	// Args is the one field that is a slice, and this Sink is the first one in
	// the repository that outlives the call to Emit — event.Encoder marshals
	// on the caller's goroutine and traceSink formats on it, so neither ever
	// held a reference to the interpreter's memory afterwards. Copied rather
	// than trusted: interp.Sink says nothing about how long an Event stays
	// readable, so anything that keeps one has to keep its own.
	if r.Args != nil {
		r.Args = append([]string(nil), r.Args...)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.seq++
	r.Seq, r.Time = o.seq, o.now()
	select {
	case o.recs <- r:
	default:
		// Dropped, deliberately and silently. There is nowhere to complain to
		// that is not the shell's own standard error, and a shell that
		// chattered because a plugin was slow would be worse than the lost
		// record — the gap in seq is the complaint, delivered to the only
		// party that can do anything about it.
	}
}

// feed is the goroutine that puts records on the wire.
//
// It exists so that Emit does not, and everything about it follows from that:
// the marshaling, the write and any waiting for the plugin happen here, on a
// goroutine the interpreter is not running on.
//
// At shutdown it delivers what is already numbered and then goes — see
// closing, which is where the reasoning for draining rather than abandoning
// is. Nothing here is bounded, because the bound belongs to the caller: Close
// waits for this goroutine and then takes the stream away, which is what a
// plugin that stopped reading can be answered with and a clock cannot.
func (o *observer) feed() {
	defer close(o.done)
	for {
		select {
		case r := <-o.recs:
			if !o.put(r) {
				return
			}
		case <-o.closing:
			for {
				select {
				case r := <-o.recs:
					if !o.put(r) {
						return
					}
				default:
					// Dry. Nothing new can usefully be numbered — the shell is
					// ending — so there is nothing left to wait for.
					return
				}
			}
		}
	}
}

// put writes one record and says whether the stream is still there.
//
// Stopping on a failed write is not covered either, and for a related reason:
// carrying on regardless is a mutant that survives, because the loop is fed by
// events rather than by itself, so what it costs is one failed write per
// record on a stream that is gone and not a spin. It stops because there is
// nothing left to do, not because continuing would break anything.
func (o *observer) put(r event.Record) bool {
	if err := o.h.conn.Notify(MethodEvent, r); err != nil {
		// The stream is gone. Not reported as the plugin's death: this is very
		// often *because* of it, and die keeps the first reason for the reason
		// it keeps it.
		o.h.die(err)
		return false
	}
	return true
}

// Sink is the interp.Sink that feeds this plugin's observer role, or nil if it
// did not declare one.
//
// Nil rather than a sink that discards, and it is the difference between two
// sentences a front end says: a nil Sink is what interp already means by "no
// consumer", so a plugin that did not ask for the event stream costs the shell
// nothing at all rather than costing it a conversion per action into a channel
// nobody reads.
//
// The typed-nil hazard is why this is a method and not a field. Returning
// h.obs directly when it is nil would hand back an interface holding a nil
// *observer, which is not nil, and every caller's `if sink != nil` would be
// wrong.
func (h *Host) Sink() interp.Sink {
	if h.obs == nil {
		return nil
	}
	return h.obs
}
