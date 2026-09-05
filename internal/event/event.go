// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package event writes an interpreter's event stream down in a stable,
// front-end-neutral form.
//
// This is the Sink half of the two seams docs/design.md describes, and it is a
// shared contract rather than a feature of any one consumer: a sandbox's audit
// log reads it, an agent protocol's session updates read it, an assistant
// assembling context reads it. Nothing here names a protocol, a front end, a
// dialect or a shell, and nothing may — the moment one consumer's vocabulary
// leaks into the record, the others are reading a translation.
//
// It is a *serialization* of interp.Event and not a second event type. interp
// owns what an event is; this owns how one is written down, which is the split
// that lets the wire form carry a version and a sequence number without
// putting either on the value interp passes around.
//
// The written form is JSON Lines — one object per line. A stream is
// appendable, tailable, greppable, and survives truncation with the loss of
// one record; a single JSON array would have to be closed to be valid, which a
// log that is still being written cannot be.
//
// docs/design/sandboxing.md carries the schema table and the stability rules
// in full. The five that a consumer relies on are repeated on Record below,
// because a consumer reads this file and may not read that one.
package event

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/blairham/sh/interp"
)

// Version is the schema version every record carries.
//
// It changes only for a change that would break a conforming consumer. Adding
// a field, an event name or an action name does not, because a consumer is
// required to ignore what it does not recognize — see Record.
const Version = 1

// Record is one event as it goes on the wire.
//
// The stability rules, which are the contract:
//
//  1. V identifies the schema. A consumer checks it and refuses a version it
//     does not know.
//  2. A field name means one thing forever. A name is never reused for a
//     different meaning; a field that becomes wrong is abandoned rather than
//     repurposed.
//  3. New fields may be added within a version, so a consumer must ignore
//     fields it does not recognize. A consumer that fails on an unknown field
//     has broken the contract rather than found a bug.
//  4. New Event and Action names may be added within a version, so a consumer
//     must not fail on a name it does not know — recording it and carrying on
//     is the useful default. ActionSignal arriving after the other five kinds
//     is the worked example, and docs/design.md tells that story.
//  5. An absent field means the zero value. Fields are omitted rather than
//     written as null, except where zero is meaningful — a status of 0 is
//     success and a signal of 0 is the existence probe, so those are pointers
//     and are written whenever they apply.
//
// Event and Action are the names interp's own String methods produce, so the
// wire names and the Go names cannot drift apart.
type Record struct {
	V   int   `json:"v"`
	Seq int64 `json:"seq"`
	// Time is when the record was written, which is a function call after the
	// action: a Sink is called synchronously on the goroutine that emitted.
	// Stating the mechanism is better than implying a precision the record
	// does not have.
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	Action string    `json:"action"`
	Path   string    `json:"path,omitempty"`
	Args   []string  `json:"args,omitempty"`
	Write  bool      `json:"write,omitempty"`
	// PID and Signal are set together, for a signal and nothing else. Pointers
	// because both are meaningful at zero: 0 is a signal that delivers
	// nothing, which is `kill -0` asking whether a process is there.
	PID    *int `json:"pid,omitempty"`
	Signal *int `json:"signal,omitempty"`
	// Status is the exit status, for the end of a command and nothing else. A
	// pointer for the same reason: 0 is success rather than absence.
	Status *int   `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	// Line is the source line a diagnostic about this action would name, and
	// File is the file that line is in — the sourced file while one runs, the
	// script otherwise, absent when the input was a command string or standard
	// input.
	Line int    `json:"line"`
	File string `json:"file,omitempty"`
}

// Of converts an event to its wire form, without a sequence number or a time.
//
// Those two belong to a stream rather than to an event — see Encoder — so a
// consumer that wants the structure without a stream gets a Record whose Seq
// is 0 and whose Time is the zero time, which is honest about having neither.
func Of(e interp.Event) Record {
	r := Record{
		V:      Version,
		Event:  e.Kind.String(),
		Action: e.Action.Kind.String(),
		Path:   e.Action.Path,
		Args:   e.Action.Args,
		Write:  e.Action.Write,
		Line:   e.Line,
		File:   e.File,
	}
	if e.Action.Kind == interp.ActionSignal {
		pid, sig := e.Action.PID, int(e.Action.Signal)
		r.PID, r.Signal = &pid, &sig
	}
	if e.Kind == interp.EventCommandEnd {
		status := e.Status
		r.Status = &status
	}
	if e.Err != nil {
		r.Error = e.Err.Error()
	}
	return r
}

// Encoder is an interp.Sink that writes each event as a line of JSON.
//
// It numbers and timestamps what it writes, which is the whole reason a stream
// is a thing rather than a bag of events:
//
// Seq gives a total order of *emission*, and must not be read as a causal one.
// A Sink is called from every goroutine a shell has — a background job and
// each half of a pipeline run on their own — so two records a step apart may
// be unrelated. What it is for is replay and gap detection: a consumer holding
// 1..40 and then 42 knows it lost one, which a timestamp alone cannot tell it.
//
// The mutex is the contract and not caution, for the same reason. Two
// goroutines writing one line each into one io.Writer interleave them into one
// unreadable line, and the sequence numbers would race besides.
type Encoder struct {
	mu  sync.Mutex
	enc *json.Encoder
	// now is injectable so a test is deterministic. Nothing else sets it: a
	// stream whose clock a caller could change would be an audit trail whose
	// times a caller could choose.
	now func() time.Time
	seq int64
}

// NewEncoder writes records to w.
func NewEncoder(w io.Writer) *Encoder {
	return newEncoder(w, time.Now)
}

func newEncoder(w io.Writer, now func() time.Time) *Encoder {
	enc := json.NewEncoder(w)
	// No indentation and no HTML escaping. A record is one line by definition,
	// and escaping < > & would mangle every path and every argument that
	// contains one — a redirect is written with them.
	enc.SetEscapeHTML(false)
	return &Encoder{enc: enc, now: now}
}

// Emit writes one event. It is interp.Sink.
func (e *Encoder) Emit(_ context.Context, ev interp.Event) {
	r := Of(ev)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	r.Seq, r.Time = e.seq, e.now()
	// A record that cannot be written is dropped. Complaining would mean a
	// diagnostic per action, and the only place to complain to may be the
	// stream that just failed. A consumer that needs to know a stream broke
	// finds the gap in Seq, which is what the numbering is for.
	_ = e.enc.Encode(r)
}
