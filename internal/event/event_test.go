// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package rather than event_test, and for one reason: the clock an Encoder
// stamps records with is unexported on purpose — an audit trail whose times a
// caller can choose is not one — so the only honest way to make these
// deterministic is from inside.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/interp"
)

// fixedTime is what the injected clock returns. Any instant would do; this one
// is written out in full so the expected wire form below is readable.
var fixedTime = time.Date(2026, 9, 5, 11, 2, 3, 1, time.UTC)

func testEncoder(w *bytes.Buffer) *Encoder {
	return newEncoder(w, func() time.Time { return fixedTime })
}

// TestTheWireNamesAreTheInterpreterNames is stability rule 2 asserted against
// the thing that would break it.
//
// The names on the wire are what a consumer matches on, so they cannot be a
// second table maintained alongside interp's — a table that drifts is worse
// than no table, because the drift is silent and lands in somebody's parser.
// Of takes them from the String methods, and this pins that it does.
func TestTheWireNamesAreTheInterpreterNames(t *testing.T) {
	t.Parallel()
	events := []interp.EventKind{
		interp.EventCommandStart, interp.EventCommandEnd,
		interp.EventDenied, interp.EventError, interp.EventAccess,
	}
	wantEvents := []string{"command-start", "command-end", "denied", "error", "access"}
	for i, k := range events {
		r := Of(interp.Event{Kind: k})
		if r.Event != wantEvents[i] {
			t.Errorf("event %d: got %q, want %q", k, r.Event, wantEvents[i])
		}
	}
	actions := []interp.ActionKind{
		interp.ActionExec, interp.ActionOpen, interp.ActionStat,
		interp.ActionReadDir, interp.ActionSignal, interp.ActionInherit,
	}
	wantActions := []string{"exec", "open", "stat", "read-dir", "signal", "inherit"}
	for i, k := range actions {
		r := Of(interp.Event{Action: interp.Action{Kind: k}})
		if r.Action != wantActions[i] {
			t.Errorf("action %d: got %q, want %q", k, r.Action, wantActions[i])
		}
	}
}

// TestZeroIsWrittenWhereZeroMeansSomething is stability rule 5, which is the
// one a plain omitempty struct gets wrong.
//
// `kill -0` is the existence probe and its signal number is 0; a command that
// succeeded ends with status 0. Both would vanish under omitempty, and a
// consumer reading an absent field as the zero value could not tell "this
// event has no status" from "this command succeeded" — which is the
// difference between a command that ran and one that never started.
func TestZeroIsWrittenWhereZeroMeansSomething(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	ctx := t.Context()
	enc.Emit(ctx, interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionSignal, PID: 4321, Signal: 0},
	})
	enc.Emit(ctx, interp.Event{
		Kind: interp.EventCommandEnd, Status: 0,
		Action: interp.Action{Kind: interp.ActionExec, Path: "/bin/true"},
	})
	lines := records(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d records, want 2", len(lines))
	}
	if got, ok := lines[0]["signal"]; !ok || got != float64(0) {
		t.Errorf("signal: got %v (present %v), want 0 present", got, ok)
	}
	if got, ok := lines[0]["pid"]; !ok || got != float64(4321) {
		t.Errorf("pid: got %v (present %v), want 4321", got, ok)
	}
	if _, ok := lines[0]["status"]; ok {
		t.Error("an access carries no status, but one was written")
	}
	if got, ok := lines[1]["status"]; !ok || got != float64(0) {
		t.Errorf("status: got %v (present %v), want 0 present", got, ok)
	}
	if _, ok := lines[1]["signal"]; ok {
		t.Error("an exec carries no signal, but one was written")
	}
}

// TestAnAbsentFieldIsAnAbsentField pins the other half of rule 5: everything
// that does not apply is omitted rather than written as a null or a zero, so a
// record stays small and a consumer can test for presence.
func TestAnAbsentFieldIsAnAbsentField(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	testEncoder(&buf).Emit(t.Context(), interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
		Line:   3,
	})
	line := strings.TrimSpace(buf.String())
	for _, field := range []string{"args", "write", "pid", "signal", "status", "error", "file"} {
		if strings.Contains(line, `"`+field+`"`) {
			t.Errorf("%q appears in a record that has no %s: %s", field, field, line)
		}
	}
	// Line is written even at zero — it is always meaningful, and a consumer
	// showing "where" needs to distinguish line 0 from a missing line.
	for _, field := range []string{"v", "seq", "time", "event", "action", "path", "line"} {
		if !strings.Contains(line, `"`+field+`"`) {
			t.Errorf("%q is missing from %s", field, line)
		}
	}
}

// TestAStreamIsNumberedAndStamped is what makes a stream a stream.
//
// Seq is for gap detection: a consumer holding 1..40 and then 42 knows it lost
// one, which no timestamp can tell it. So the numbers have to start at 1 and
// step by exactly one, and a hole is the only thing that ever produces a hole.
func TestAStreamIsNumberedAndStamped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	for range 3 {
		enc.Emit(t.Context(), interp.Event{Kind: interp.EventAccess})
	}
	for i, r := range records(t, &buf) {
		if got := r["seq"]; got != float64(i+1) {
			t.Errorf("record %d: seq %v, want %d", i, got, i+1)
		}
		if got := r["v"]; got != float64(Version) {
			t.Errorf("record %d: v %v, want %d", i, got, Version)
		}
		if got := r["time"]; got != fixedTime.Format(time.RFC3339Nano) {
			t.Errorf("record %d: time %v, want %v", i, got, fixedTime.Format(time.RFC3339Nano))
		}
	}
}

// TestOneRecordIsOneLine is the whole of the JSON Lines claim: a reader splits
// on newlines and hands each piece to a JSON decoder. A record that wrapped —
// because something indented it, or because a path held a raw newline — would
// break every such reader.
func TestOneRecordIsOneLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	ctx := t.Context()
	enc.Emit(ctx, interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionOpen, Path: "/tmp/a\nb"},
	})
	enc.Emit(ctx, interp.Event{
		Kind:   interp.EventCommandStart,
		Action: interp.Action{Kind: interp.ActionExec, Path: "/bin/echo", Args: []string{"echo", "hi"}},
	})
	got := strings.Count(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if got != 1 {
		t.Fatalf("2 records produced %d embedded newlines, want 1 separator: %q", got, buf.String())
	}
}

// TestPathsAreNotHTMLEscaped is a papercut that would otherwise reach every
// consumer. encoding/json escapes <, > and & by default, for a browser; a
// shell writes all three constantly — a redirect is spelled with two of them —
// and a path or an argument arriving as > is a record nobody can grep.
func TestPathsAreNotHTMLEscaped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	testEncoder(&buf).Emit(t.Context(), interp.Event{
		Kind:   interp.EventCommandStart,
		Action: interp.Action{Kind: interp.ActionExec, Path: "/bin/sh", Args: []string{"sh", "-c", "a > b & c"}},
	})
	if strings.Contains(buf.String(), `\u00`) {
		t.Errorf("something was escaped for HTML: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "a > b & c") {
		t.Errorf("the argument did not survive: %s", buf.String())
	}
}

// TestAnUnknownFieldDoesNotBreakAConsumer is stability rule 3, demonstrated
// rather than asserted about ourselves.
//
// The rule binds consumers, so what this pins is that the shape permits it: a
// consumer decoding into a struct of the fields it cares about reads a record
// carrying fields it has never heard of, and gets what it asked for.
func TestAnUnknownFieldDoesNotBreakAConsumer(t *testing.T) {
	t.Parallel()
	line := `{"v":1,"seq":7,"time":"2026-09-05T11:02:03.000000001Z","event":"denied",` +
		`"action":"open","path":"/etc/shadow","line":3,"invented-later":{"a":[1,2]}}`
	var got struct {
		V     int    `json:"v"`
		Event string `json:"event"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("a record with an unknown field did not decode: %v", err)
	}
	if got.V != Version || got.Event != "denied" || got.Path != "/etc/shadow" {
		t.Errorf("got %+v", got)
	}
}

// TestAnErrorIsCarriedAsItsText keeps the wire form free of Go.
//
// interp.Event.Err is an error value, which nothing outside this process can
// read. A consumer wants the sentence, and only the sentence — an error's type
// is an implementation detail of the interpreter and would be a promise this
// schema cannot keep.
func TestAnErrorIsCarriedAsItsText(t *testing.T) {
	t.Parallel()
	r := Of(interp.Event{Kind: interp.EventError, Err: errors.New("no such file")})
	if r.Error != "no such file" {
		t.Errorf("got %q, want %q", r.Error, "no such file")
	}
	if Of(interp.Event{Kind: interp.EventAccess}).Error != "" {
		t.Error("an event with no error carried one")
	}
}

// TestOfCarriesNoStreamState is why Seq and Time are the Encoder's.
//
// A Record built from an event alone has neither, and says so with zero values
// rather than inventing a number or reading a clock. A consumer that wants the
// structure without a stream — mapping one event to one protocol message — gets
// exactly the event.
func TestOfCarriesNoStreamState(t *testing.T) {
	t.Parallel()
	r := Of(interp.Event{Kind: interp.EventAccess})
	if r.Seq != 0 || !r.Time.IsZero() {
		t.Errorf("Of invented stream state: seq=%d time=%v", r.Seq, r.Time)
	}
}

// TestSignalsCarryTheirTargetVerbatim pins that a process group survives.
//
// kill(2) takes a negative pid to mean a process group, and interp passes it
// through unchanged for exactly that reason. A schema that normalized the sign
// would turn "signal everything in job 1" into "signal one process", which is
// the difference an audit trail exists to record.
func TestSignalsCarryTheirTargetVerbatim(t *testing.T) {
	t.Parallel()
	r := Of(interp.Event{
		Kind:   interp.EventDenied,
		Action: interp.Action{Kind: interp.ActionSignal, PID: -900, Signal: syscall.SIGKILL},
	})
	if r.PID == nil || *r.PID != -900 {
		t.Errorf("pid: got %v, want -900", r.PID)
	}
	if r.Signal == nil || *r.Signal != int(syscall.SIGKILL) {
		t.Errorf("signal: got %v, want %d", r.Signal, syscall.SIGKILL)
	}
}

// TestAnEncoderIsWrittenToFromEveryGoroutine is the contract interp.Sink
// states, asserted against the implementation of one.
//
// A background job reports from the goroutine running it and so does each half
// of a pipeline. An unguarded encoder interleaves two records into one
// unreadable line and races on the counter besides, and neither failure is
// visible in a single-threaded test. Under -race this is the assertion; the
// sequence check is what catches a lost increment if it ever runs without.
func TestAnEncoderIsWrittenToFromEveryGoroutine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			enc.Emit(context.Background(), interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
			})
		}()
	}
	wg.Wait()
	seen := map[float64]bool{}
	for _, r := range records(t, &buf) {
		seq, ok := r["seq"].(float64)
		if !ok {
			t.Fatalf("a record has no sequence number: %v", r)
		}
		if seen[seq] {
			t.Errorf("sequence number %v was issued twice", seq)
		}
		seen[seq] = true
	}
	if len(seen) != n {
		t.Errorf("got %d records, want %d", len(seen), n)
	}
}

// records decodes the buffer as JSON Lines, which is also an assertion: a
// stream that is not one line per object fails here.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q did not decode: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

// An id is the length the disambiguation in Find relies on, and it is made of
// the alphabet a filename can hold without quoting.
func TestAnIDIsFixedWidthAndFilenameSafe(t *testing.T) {
	id := NewID(time.Unix(1_757_000_000, 12345))
	if len(id) != IDLength {
		t.Fatalf("id %q is %d characters, want %d", id, len(id), IDLength)
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'A' || r > 'V') {
			t.Fatalf("id %q holds %q, which is not base32hex", id, r)
		}
	}
}

// Sorting the strings sorts them by time, which is the whole reason for
// choosing base32hex over standard base32 — its alphabet is ordered.
//
// The times are far enough apart to be unambiguous and close enough that only
// the low bytes of the timestamp differ, which is the case that would break if
// the encoding were not order-preserving.
func TestIDsSortIntoTimeOrder(t *testing.T) {
	base := time.Unix(1_757_000_000, 0)
	var ids []string
	var want []string
	for i := range 20 {
		id := NewID(base.Add(time.Duration(i) * time.Millisecond))
		ids = append(ids, id)
		want = append(want, id)
	}
	// Shuffled by sorting a copy: the input was already in order, so a sort
	// that did nothing would pass. Reverse first.
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	sort.Strings(ids)
	for i := range ids {
		if ids[i] != want[i] {
			t.Fatalf("sorted position %d is %q, want %q — the encoding is not order-preserving",
				i, ids[i], want[i])
		}
	}
}

// Two shells in the same nanosecond still write different ids, which is what
// makes an append-only store with no coordination possible.
func TestIDsInTheSameInstantDiffer(t *testing.T) {
	at := time.Unix(1_757_000_000, 7)
	seen := map[string]bool{}
	for range 1000 {
		id := NewID(at)
		if seen[id] {
			t.Fatalf("id %q came back twice for one instant", id)
		}
		seen[id] = true
	}
}

// The two identity fields go on the wire under the names a consumer joins on.
//
// Both were added within version 1 rather than as a version bump, which is rule
// 3 being used rather than described. What they add is the ability to line up
// two records of one run: the session says which shell, and the action id says
// which action within it.
func TestARecordCarriesItsSessionAndActionID(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	enc.Emit(t.Context(), interp.Event{
		Kind:    interp.EventCommandStart,
		Session: "SESSION",
		Action:  interp.Action{ID: "12", Kind: interp.ActionExec, Path: "/bin/echo"},
	})
	lines := records(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("got %d records, want 1", len(lines))
	}
	if got := lines[0]["session"]; got != "SESSION" {
		t.Errorf("session = %v, want the Runner's", got)
	}
	if got := lines[0]["actionId"]; got != "12" {
		t.Errorf("actionId = %v, want the action's", got)
	}
	// Not seq. The two are different numbers with different meanings and the
	// schema says so: seq orders emission within one stream and differs on
	// every record, and this names one action and repeats on every record about
	// it. A consumer that joined on seq would join a command's start to
	// whatever happened to be emitted next.
	if got := lines[0]["seq"]; got != float64(1) {
		t.Errorf("seq = %v, want the stream's own counter untouched", got)
	}
}

// An action's id repeats across every record about that action, and the stream
// counter does not.
//
// This is the property a consumer relies on, asserted against the two fields
// together because either one alone reads as plausible: a start and an end that
// share an actionId and differ in seq is the shape, and any other combination
// is a schema that cannot be joined or cannot be replayed.
func TestOneActionsRecordsShareAnIDAndDifferInSeq(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	a := interp.Action{ID: "4", Kind: interp.ActionExec, Path: "/bin/false"}
	enc.Emit(t.Context(), interp.Event{Kind: interp.EventCommandStart, Action: a})
	enc.Emit(t.Context(), interp.Event{Kind: interp.EventCommandEnd, Action: a, Status: 1})
	lines := records(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d records, want 2", len(lines))
	}
	if lines[0]["actionId"] != lines[1]["actionId"] {
		t.Errorf("actionIds are %v and %v, want one action to have one id",
			lines[0]["actionId"], lines[1]["actionId"])
	}
	if lines[0]["seq"] == lines[1]["seq"] {
		t.Errorf("both records have seq %v, want the stream's own order", lines[0]["seq"])
	}
}

// A run with no identity writes no identity, which is rule 5.
//
// An embedder that set no Session and a Runner nobody was watching both produce
// events with nothing to say here, and saying nothing is the honest record: a
// consumer reads the absent field as empty and knows this stream cannot be
// joined, rather than being handed a placeholder that looks like an id.
func TestAnIdentitylessRunWritesNoIdentity(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	enc.Emit(t.Context(), interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionStat, Path: "/tmp/x"},
	})
	lines := records(t, &buf)
	if _, ok := lines[0]["session"]; ok {
		t.Error("a run with no session wrote one")
	}
	if _, ok := lines[0]["actionId"]; ok {
		t.Error("an action with no id wrote one")
	}
}

// A consumer written before these fields existed still reads a record that has
// them, which is the whole reason this was an added field and not a version 2.
//
// The consumer here is deliberately the *original* field set, spelled out
// rather than referenced, so that it cannot quietly grow the new fields and
// stop testing anything. Both existing consumers of this schema are of exactly
// this shape — they decode into a struct of the fields they know and ignore the
// rest — so if this passes, neither needed a change.
func TestAConsumerWrittenBeforeTheIdentityFieldsStillReads(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	enc := testEncoder(&buf)
	enc.Emit(t.Context(), interp.Event{
		Kind:    interp.EventDenied,
		Session: "SESSION",
		Action: interp.Action{
			ID: "12", Kind: interp.ActionOpen, Path: "/etc/shadow", Write: true,
		},
		Line: 3, File: "script.sh",
	})
	var old struct {
		V      int       `json:"v"`
		Seq    int64     `json:"seq"`
		Time   time.Time `json:"time"`
		Event  string    `json:"event"`
		Action string    `json:"action"`
		Path   string    `json:"path"`
		Args   []string  `json:"args"`
		Write  bool      `json:"write"`
		PID    *int      `json:"pid"`
		Signal *int      `json:"signal"`
		Status *int      `json:"status"`
		Error  string    `json:"error"`
		Line   int       `json:"line"`
		File   string    `json:"file"`
	}
	if err := json.Unmarshal(buf.Bytes(), &old); err != nil {
		t.Fatalf("a record with the identity fields did not decode: %v", err)
	}
	if old.V != Version {
		t.Errorf("v = %d, want %d — an added field must not bump the version", old.V, Version)
	}
	if old.Event != "denied" || old.Action != "open" || old.Path != "/etc/shadow" ||
		!old.Write || old.Line != 3 || old.File != "script.sh" || old.Seq != 1 {
		t.Errorf("a version 1 consumer read %+v, want every original field unchanged", old)
	}
}
