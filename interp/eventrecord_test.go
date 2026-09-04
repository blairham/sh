// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The events are the audit trail, so what they carry is asserted directly
// here rather than through the prose diagnostics: a consumer of the Sink
// never sees stderr, and a field nothing asserts is a field that can quietly
// stop being filled.

// collectEvents runs src and returns every event the sink received.
func collectEvents(t *testing.T, src string, arrange func(*Runner)) []Event {
	t.Helper()
	var mu sync.Mutex
	var events []Event
	sem := PosixSemantics()
	r := &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Events: SinkFunc(func(_ context.Context, e Event) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
		}),
	}
	if arrange != nil {
		arrange(r)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	return events
}

// A denial is an event, not only a sentence on stderr: the record is what a
// permission prompt or an audit trail stands on, and until this test nothing
// asserted the event at all — the deny tests grep the prose.
func TestADenialIsAnEventNotOnlyASentence(t *testing.T) {
	events := collectEvents(t, "/usr/bin/false", func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionExec {
				return Deny
			}
			return Allow
		})
	})
	var denied []Event
	for _, e := range events {
		if e.Kind == EventDenied {
			denied = append(denied, e)
		}
	}
	if len(denied) != 1 {
		t.Fatalf("saw %d denied events, want exactly the refused exec", len(denied))
	}
	e := denied[0]
	if e.Action.Kind != ActionExec || filepath.Base(e.Action.Path) != "false" {
		t.Errorf("denied action = %v %q, want the exec that was refused", e.Action.Kind, e.Action.Path)
	}
	if e.Line != 1 {
		t.Errorf("Line = %d, want the line the command is on", e.Line)
	}
}

// The end of a command carries its status and its position, so a consumer
// can know what failed and where without re-parsing prose diagnostics.
func TestAnEventCarriesStatusAndPosition(t *testing.T) {
	events := collectEvents(t, ":\n/usr/bin/false", nil)
	var end *Event
	for i, e := range events {
		if e.Kind == EventCommandEnd && e.Action.Kind == ActionExec {
			end = &events[i]
		}
	}
	if end == nil {
		t.Fatal("no command-end event for the exec")
	}
	if end.Status != 1 {
		t.Errorf("Status = %d, want the command's own exit status", end.Status)
	}
	if end.Line != 2 {
		t.Errorf("Line = %d, want 2 — the line the command is on, not where the script began", end.Line)
	}
	if end.File != "" {
		t.Errorf("File = %q, want empty for input that came from no file", end.File)
	}
}

// Inside a sourced file, an event names the file and counts lines within it
// — the same answer a diagnostic would give, carried structurally.
func TestAnEventInsideASourcedFileNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "inc.sh")
	if err := os.WriteFile(sourced, []byte(":\n/usr/bin/true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ". "+sourced, nil)
	var start *Event
	for i, e := range events {
		if e.Kind == EventCommandStart {
			start = &events[i]
		}
	}
	if start == nil {
		t.Fatal("no command-start event from inside the sourced file")
	}
	if start.File != sourced {
		t.Errorf("File = %q, want the sourced file %q", start.File, sourced)
	}
	if start.Line != 2 {
		t.Errorf("Line = %d, want 2 — the line within the sourced file", start.Line)
	}
}

// A successful open is recorded, not only a failed one: an audit trail fed
// by the error path alone held every file the shell could not open and none
// it could.
func TestASuccessfulOpenIsAnEvent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out")
	events := collectEvents(t, "echo hi > "+target, nil)
	var opens []Event
	for _, e := range events {
		if e.Kind == EventAccess && e.Action.Kind == ActionOpen {
			opens = append(opens, e)
		}
	}
	if len(opens) != 1 {
		t.Fatalf("saw %d open access events, want the one redirect", len(opens))
	}
	if opens[0].Action.Path != target || !opens[0].Action.Write {
		t.Errorf("access = %q write=%v, want the redirect's own open", opens[0].Action.Path, opens[0].Action.Write)
	}
}

// A probe is recorded as an access of its own kind, whichever answer it got:
// an existence probe is the auditable act whether or not the file exists.
func TestAProbeIsAnEvent(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent")
	events := collectEvents(t, "test -f "+absent, nil)
	var probes []Event
	for _, e := range events {
		if e.Kind == EventAccess && e.Action.Kind == ActionStat {
			probes = append(probes, e)
		}
	}
	if len(probes) != 1 {
		t.Fatalf("saw %d stat access events, want the test's one probe", len(probes))
	}
	if probes[0].Action.Path != absent {
		t.Errorf("probe path = %q, want the file the test asked about, present or not", probes[0].Action.Path)
	}
}
