// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package blocks stores a command and what came of it as one record.
//
// A block is one thing a person asked for and everything that came of it: the
// line as typed, where and when it ran, how long it took, what it exited with,
// and — when a session is capturing it — what it printed. docs/design/blocks.md
// is the design; the parts a reader of this file needs are repeated here.
//
// It is the *at-rest* form of the unit interp's Sink streams live, and
// deliberately not a second event type. A block is the closure of a
// command-start/command-end pair plus what the writer the caller supplied saw,
// so `status` here is the status on EventCommandEnd and the output is the
// writer the front end handed the Runner. Two fields have no event to come
// from and both are the front end's rather than the interpreter's: the line
// *as typed*, which only a prompt has — an event carries argv after expansion —
// and a session identity, which nothing in the event stream has at all.
//
// The written form is JSON Lines, one object per line, for the reason the
// event stream chose it: a stream is appendable, tailable, greppable, and
// survives a torn write with the loss of one record. The index is opened the
// way cmd/sh opens an audit file — O_APPEND, 0600, unbuffered, one write per
// record straight through — because the two files have the same problem: a
// trail that erases the previous run, or that loses what a buffer held when
// the shell died, is not a trail.
//
// The shape follows the event schema deliberately rather than resembling it by
// accident. `v` names the version and carries the same stability rules; a
// consumer ignores fields it does not recognize and refuses a version it does
// not know; and the field names that mean the same thing are spelled the same
// way. Two differences are on purpose:
//
//   - There is no `seq`. A stream's sequence number is a total order of
//     *emission* and explicitly not a causal one, which is right for a log and
//     is the wrong thing to lean on here. A block's order is the order of the
//     file, and its identity is an id that sorts by the time it started.
//
//   - The timestamp is `start` rather than `time`. An event's `time` is when
//     the *record* was written; a block's is when the command began, and it
//     has a duration beside it. Reusing the name for the other meaning is the
//     one thing the stability rules forbid outright.
//
// # Two files, never one
//
// The plain-line history file is untouched. This index lives beside it and is
// never a replacement, because the two answer different questions — the line
// file is the recall list the up arrow walks, and this is the record of what
// happened. Each is independently disposable, which is what makes adding this
// one safe.
package blocks

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"time"
)

// Version is the schema version every record carries.
//
// The stability rules are the event schema's, by reference rather than by
// restatement, because a block and an event are the same unit: a consumer that
// has learned one has learned both. It changes only for a change that would
// break a conforming consumer, a field name means one thing forever, new
// fields may be added within a version and a consumer must ignore what it does
// not recognize, and an absent field means the zero value.
const Version = 1

// StreamsMerged is what `streams` says while both of a command's streams go to
// one body in write order, which is what the person saw.
//
// The field exists from the first version so that separating them later is an
// added value rather than a version bump. That is the additive rule being used
// rather than described.
const StreamsMerged = "merged"

// Record is one block as it goes on the wire.
//
// Start, DurationMs and Status are written even at zero, because zero is
// meaningful for all three: a status of 0 is success, and a command that took
// no measurable time still ran. Everything about output is omitted when
// nothing was kept, and a reader treats its absence as "no output kept" — the
// same state a block whose body file has since been deleted is in. There is
// one state to handle rather than two.
type Record struct {
	V       int    `json:"v"`
	ID      string `json:"id"`
	Session string `json:"session"`
	// Command is the line as typed, before expansion. `echo $HOME` is recorded
	// as `echo $HOME`, which is what a person asked for and what an event
	// cannot supply.
	Command string `json:"command"`
	// Cwd is where the line was accepted, which is before it ran: a line that
	// says `cd /tmp` was typed somewhere else.
	Cwd        string    `json:"cwd"`
	Start      time.Time `json:"start"`
	DurationMs int64     `json:"durationMs"`
	Status     int       `json:"status"`
	// Output is the body's path relative to the store, stored rather than
	// derived from the id. The layout is therefore an implementation detail
	// that can change without a version bump, and a store whose bodies were
	// moved by hand still resolves.
	Output string `json:"output,omitempty"`
	// OutputBytes is the *true* total the command wrote, which is larger than
	// the body when Truncated is set. A consumer is never guessing.
	OutputBytes int64  `json:"outputBytes,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Streams     string `json:"streams,omitempty"`
}

// idEncoding is base32 with the extended-hex alphabet and no padding.
//
// The alphabet is 0-9 then A-V, which is ordered, so sorting the encoded
// strings sorts the bytes underneath them. That is the whole reason for
// choosing it over standard base32, whose alphabet starts at A and puts the
// digits last: an id has a timestamp at the front, and this is what makes
// `sort` over the index chronological.
var idEncoding = base32.HexEncoding.WithPadding(base32.NoPadding)

// idLength is how long an id always is: sixteen bytes in base32 is 26
// characters. Fixed rather than a range, which is what lets a short decimal
// number mean something else without ambiguity — see Store.Find.
const idLength = 26

// NewID makes an id for a block or a session: eight bytes of Unix nanoseconds,
// big-endian, then eight random bytes, encoded as 26 characters.
//
// Four properties, each of which was a requirement. It sorts by time, per
// idEncoding. It is unique across sessions without coordination, because two
// shells writing in the same nanosecond differ in 64 random bits — and nothing
// having to hold a counter is what makes an append-only multi-session store
// possible at all. It is safe as a filename on every filesystem, being
// uppercase throughout so a case-folding one cannot collide two of them. And
// it needs nothing outside the standard library.
//
// A content hash was the other candidate and is wrong for this: two identical
// commands run an hour apart are two blocks, and when and where they ran is as
// much of a block as what was typed.
func NewID(t time.Time) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(t.UnixNano()))
	// crypto/rand.Read never returns an error — it panics on a broken system
	// rather than reporting one — so there is nothing to handle here.
	_, _ = rand.Read(b[8:])
	return idEncoding.EncodeToString(b[:])
}

// decode reads one line back, reporting whether it is a record this version
// understands.
//
// Three ways a line is not one, and all three are skipped rather than reported
// — a store is a file several sessions append to, so a reader that failed on
// one bad line would be a reader that a single torn write could disable.
//
//  1. It is not JSON. A record is written with one Write under O_APPEND, so
//     that means a concurrent writer tore it or something else wrote here.
//  2. Its version is one we do not know. The event schema's rule is that a
//     consumer refuses a version it does not recognize, and this is that rule:
//     an old shell reading a newer store skips what it cannot promise to read
//     correctly and still shows everything it can. Unknown *fields* are the
//     opposite case and are ignored, which encoding/json already does — that
//     is what makes an added field a non-breaking change.
//  3. It has no status. Every record this package writes has one, because a
//     block always exited with something; a record without one is truncated or
//     foreign, and reading it would silently report a *success*. This is the
//     same care the event schema takes by making its status a pointer, arrived
//     at from the other side: there a status is present only for the end of a
//     command, so absence is ordinary and a pointer distinguishes it from zero.
//     Here presence is universal, so the field can be a plain int that is
//     always written, and the pointer is needed only on the way back in.
func decode(line string) (Record, bool) {
	var probe struct {
		V      *int `json:"v"`
		Status *int `json:"status"`
	}
	if err := json.Unmarshal([]byte(line), &probe); err != nil {
		return Record{}, false
	}
	if probe.V == nil || *probe.V > Version || probe.Status == nil {
		return Record{}, false
	}
	var r Record
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		return Record{}, false
	}
	return r, true
}

// encode renders a record as the single line it is written as.
//
// One line by definition, so no indentation, and no HTML escaping: escaping
// < > & would mangle every command line that redirects, which is most of the
// ones worth keeping.
func encode(r Record) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
