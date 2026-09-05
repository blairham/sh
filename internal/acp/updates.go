// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/blairham/sh/interp"
)

// What the shell did, as the protocol says it.
//
// Two things flow to a client while a turn runs: what the commands *wrote*,
// which is bytes on the runner's own writers, and what the shell *did*, which
// is the event stream. They are deliberately separate — interp.Event carries
// no output, because the streams are the caller's own writers and copying
// every byte through the event path would hand a consumer a second copy of
// what it already holds.

// tracker matches the events of one action to the tool call that reports it.
//
// It exists because interp.Event carries no identity for the action it belongs
// to, so a start and an end are matched by *ordering* — and ordering is
// exactly what concurrency breaks, since a background job and each half of a
// pipeline emit from their own goroutines. A fingerprint of what the action
// was, with a queue per fingerprint, is right whenever two identical commands
// are not in flight at once, and picks the wrong one of them when they are.
//
// The failure is cosmetic: the wrong tool call is marked complete, and no
// decision changes. It is still wrong, and the fix is a per-action id on the
// event — see docs/design/acp.md, which asks for one rather than inventing a
// second event schema here.
type tracker struct {
	mu   sync.Mutex
	n    int
	open map[string][]string
}

// fingerprint is what an action is, as a key.
//
// The action and nothing else. The event also carries the line and the file it
// came from, which would discriminate better, and they are deliberately left
// out: the gate is consulted with an Action alone, before any event exists, so
// a key that included them could never match the tool call a permission
// request opened to the events for the action it approved.
func fingerprint(e interp.Event) string {
	a := e.Action
	var b strings.Builder
	b.WriteString(a.Kind.String())
	b.WriteByte(0)
	b.WriteString(a.Path)
	b.WriteByte(0)
	b.WriteString(strings.Join(a.Args, "\x00"))
	if a.Kind == interp.ActionOpen {
		b.WriteByte(0)
		b.WriteString(strconv.FormatBool(a.Write))
	}
	if a.Kind == interp.ActionSignal {
		b.WriteByte(0)
		b.WriteString(strconv.Itoa(a.PID))
		b.WriteByte(0)
		b.WriteString(strconv.Itoa(int(a.Signal)))
	}
	return b.String()
}

// begin opens a tool call for an action and returns its id.
func (t *tracker) begin(e interp.Event) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.n++
	id := "call-" + strconv.Itoa(t.n)
	if t.open == nil {
		t.open = map[string][]string{}
	}
	k := fingerprint(e)
	t.open[k] = append(t.open[k], id)
	return id
}

// head is the oldest open tool call for an action, left open.
func (t *tracker) head(e interp.Event) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := t.open[fingerprint(e)]
	if len(q) == 0 {
		return "", false
	}
	return q[0], true
}

// end closes the oldest open tool call for an action and returns its id.
func (t *tracker) end(e interp.Event) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := fingerprint(e)
	q := t.open[k]
	if len(q) == 0 {
		return "", false
	}
	id := q[0]
	if len(q) == 1 {
		delete(t.open, k)
	} else {
		t.open[k] = q[1:]
	}
	return id, true
}

// toolKind is what kind of tool call an action reads as, in the protocol's own
// vocabulary. Its list is longer than this; these are the ones an action of
// ours can honestly be.
func toolKind(a interp.Action) string {
	switch a.Kind {
	case interp.ActionExec:
		return KindExecute
	case interp.ActionOpen:
		if a.Write {
			return KindEdit
		}
		return KindRead
	case interp.ActionStat, interp.ActionReadDir:
		return KindRead
	case interp.ActionSignal, interp.ActionInherit:
		return KindOther
	}
	return KindOther
}

// title is what a person reads in the client's list of what happened.
func title(a interp.Action) string {
	switch a.Kind {
	case interp.ActionExec:
		if len(a.Args) > 0 {
			return strings.Join(a.Args, " ")
		}
		return a.Path
	case interp.ActionOpen:
		if a.Write {
			return "write " + a.Path
		}
		return "read " + a.Path
	case interp.ActionStat:
		return "stat " + a.Path
	case interp.ActionReadDir:
		return "list " + a.Path
	case interp.ActionSignal:
		return "signal " + strconv.Itoa(int(a.Signal)) + " to " + strconv.Itoa(a.PID)
	case interp.ActionInherit:
		return "inherited " + a.Path
	}
	return a.Kind.String()
}

// describe is the structured action, handed over as the tool call's raw input.
//
// The same facts the audit record carries, in the same words: the vocabulary
// is interp's, and both consumers of the event stream use it rather than each
// inventing a spelling. Nothing is invented here and nothing is dropped.
func describe(a interp.Action) map[string]any {
	m := map[string]any{"action": a.Kind.String()}
	if a.Path != "" {
		m["path"] = a.Path
	}
	if len(a.Args) > 0 {
		m["args"] = a.Args
	}
	if a.Kind == interp.ActionOpen {
		m["write"] = a.Write
	}
	if a.Kind == interp.ActionSignal {
		m["pid"] = a.PID
		m["signal"] = int(a.Signal)
	}
	return m
}

// locations is where a client should look to follow along, which is a path for
// everything that has one and nothing for a signal.
func locations(e interp.Event) []ToolCallLocation {
	if e.Action.Kind == interp.ActionSignal || e.Action.Path == "" {
		return nil
	}
	// The line is the script's rather than the file's when the action came
	// from a command string, and a zero line is omitted by the encoding.
	return []ToolCallLocation{{Path: e.Action.Path}}
}

// chunker turns the bytes a command wrote into content blocks, without ever
// splitting a character in half.
//
// A Writer is handed whatever size of write the command made, and a multi-byte
// character can straddle two of them. Sending the halves would put invalid
// UTF-8 into a JSON string, where the encoder replaces it with a replacement
// character — so `echo café` split at the wrong byte would arrive as mojibake
// rather than as text. The incomplete tail is held back until the rest of it
// arrives.
type chunker struct {
	mu   sync.Mutex
	tail []byte
	emit func(string)
}

func (c *chunker) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	buf := p
	if len(c.tail) > 0 {
		buf = append(c.tail, p...)
		c.tail = nil
	}
	// Hold back a final rune that has not all arrived. RuneError with a width
	// of 1 is a byte that cannot begin a rune at all, which is not a partial
	// character and is passed through for the encoder to deal with.
	if keep := incompleteTail(buf); keep > 0 {
		c.tail = append([]byte(nil), buf[len(buf)-keep:]...)
		buf = buf[:len(buf)-keep]
	}
	if len(buf) > 0 {
		c.emit(string(buf))
	}
	return n, nil
}

// Flush sends whatever is held back, which at the end of a turn is a truncated
// character rather than an incomplete one and is better shown than dropped.
func (c *chunker) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.tail) > 0 {
		c.emit(string(c.tail))
		c.tail = nil
	}
}

// incompleteTail is how many bytes at the end of b are the start of a
// character whose remaining bytes have not arrived.
//
// The scan goes back to the last byte that can begin a character — at most
// three, since no encoding of a rune is longer than four bytes — and compares
// what the lead byte says the character's length is against how much of it is
// here. A byte that begins nothing valid is not a partial character and is
// passed through, because holding it back would wait for bytes that are never
// coming.
func incompleteTail(b []byte) int {
	for i := len(b) - 1; i >= 0 && i > len(b)-utf8.UTFMax; i-- {
		if !utf8.RuneStart(b[i]) {
			continue
		}
		n := runeLen(b[i])
		if n == 0 || len(b)-i >= n {
			return 0
		}
		return len(b) - i
	}
	return 0
}

// runeLen is how many bytes the character beginning with c occupies, or 0 if c
// does not begin one.
func runeLen(c byte) int {
	switch {
	case c < 0x80:
		return 1
	case c&0xE0 == 0xC0:
		return 2
	case c&0xF0 == 0xE0:
		return 3
	case c&0xF8 == 0xF0:
		return 4
	}
	return 0
}
