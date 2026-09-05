// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An action has an identity, and the same one everywhere it appears.
//
// This is what neither ordering nor a fingerprint could supply. A Gate is
// consulted about an action and then one to three events are emitted about it,
// and a consumer that has to pair the consultation with the events — an agent
// protocol asking a person's permission, a front end closing the record it
// opened — had nothing to pair them on. Ordering does not answer it, because a
// background job and each half of a pipeline emit from their own goroutines, so
// a command's end is very often not the record after its start.
//
// Every kind is exercised, deliberately. A kind that arrived without an id
// would be a kind whose actions look joinable to a consumer and are not, which
// is worse than one that plainly carries nothing.

// recorder is a Gate and a Sink that keeps what it was told, guarded because
// both are called from every goroutine a shell has.
type recorder struct {
	mu     sync.Mutex
	gated  []Action
	events []Event
}

func (rec *recorder) Allow(_ context.Context, a Action) Decision {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.gated = append(rec.gated, a)
	return Allow
}

func (rec *recorder) Emit(_ context.Context, e Event) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.events = append(rec.events, e)
}

// kinds returns the events whose action was of one kind.
func (rec *recorder) kinds(k ActionKind) []Event {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var out []Event
	for _, e := range rec.events {
		if e.Action.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

// runRecorded runs a script under a recorder and hands back what it saw.
func runRecorded(t *testing.T, src string, setup func(*Runner)) *recorder {
	t.Helper()
	rec := &recorder{}
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Session: "SESSIONUNDERTEST",
		Gate:    rec, Events: rec,
	})
	if setup != nil {
		setup(r)
	}
	t.Cleanup(r.CleanUp)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

// Every kind of action carries an id, and no kind is exempt.
//
// The table names all six on purpose. A seventh kind added without reaching
// this list is a kind whose events cannot be joined to the consultation that
// allowed them, and nothing else in the suite would notice: the events still
// arrive and the script still runs.
func TestEveryKindOfActionCarriesAnID(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, []byte(":\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pid, _ := aLiveProcess(t)

	for _, tc := range []struct {
		name, src string
		kind      ActionKind
		setup     func(*Runner)
	}{
		{"exec", "/bin/echo hi", ActionExec, nil},
		{"open", "/bin/echo hi > " + filepath.Join(dir, "out"), ActionOpen, nil},
		{"stat", "test -f " + present, ActionStat, nil},
		{"read-dir", "echo " + dir + "/*", ActionReadDir, nil},
		{"signal", "kill -0 " + itoa(pid), ActionSignal, func(r *Runner) {
			r.SignalGroup = func(int, syscall.Signal) error { return nil }
		}},
		{"inherit", ":", ActionInherit, func(r *Runner) {
			r.InheritedFiles = []*os.File{inherited(t, "hello\n")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := runRecorded(t, tc.src, tc.setup)
			got := rec.kinds(tc.kind)
			if len(got) == 0 {
				t.Fatalf("no %v event: the script %q did not produce one", tc.kind, tc.src)
			}
			for _, e := range got {
				if e.Action.ID == "" {
					t.Errorf("a %v event carries no id: %+v", tc.kind, e.Action)
				}
				if e.Session != "SESSIONUNDERTEST" {
					t.Errorf("a %v event says session %q, want the Runner's",
						tc.kind, e.Session)
				}
			}
		})
	}
}

// The id the gate was shown is the id the events carry.
//
// This is the whole promise. A permission request answered about one action and
// records written about another that merely looks the same is exactly the
// failure a fingerprint match had: the wrong record gets closed, and nothing
// says so.
func TestTheGateAndTheEventsAgreeOnTheID(t *testing.T) {
	rec := runRecorded(t, "/bin/echo hi", nil)
	rec.mu.Lock()
	defer rec.mu.Unlock()

	var execID string
	for _, a := range rec.gated {
		if a.Kind == ActionExec {
			execID = a.ID
		}
	}
	if execID == "" {
		t.Fatal("the gate was never shown an exec with an id")
	}
	var starts, ends int
	for _, e := range rec.events {
		if e.Action.Kind != ActionExec {
			continue
		}
		if e.Action.ID != execID {
			t.Errorf("%v event carries id %q, want the gate's %q",
				e.Kind, e.Action.ID, execID)
		}
		switch e.Kind {
		case EventCommandStart:
			starts++
		case EventCommandEnd:
			ends++
		}
	}
	if starts != 1 || ends != 1 {
		t.Errorf("saw %d starts and %d ends, want one of each to share the id", starts, ends)
	}
}

// Two actions never share an id, including across a subshell.
//
// A subshell is a cloned Runner, so a counter copied rather than shared would
// hand the parent's next action and the child's next action the same number —
// two different actions that look like one, which is worse than no id at all
// because a consumer would join them.
func TestNoTwoActionsShareAnID(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"in sequence", "/bin/echo one; /bin/echo two; /bin/echo three"},
		{"across a subshell", "( /bin/echo one ); /bin/echo two"},
		{"across a background job", "/bin/echo one & wait; /bin/echo two"},
		{"across a pipeline", "/bin/echo one | /bin/cat"},
		{"across a command substitution", "x=$(/bin/echo one); /bin/echo two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := runRecorded(t, tc.src, func(r *Runner) { r.Dir = dir })
			rec.mu.Lock()
			defer rec.mu.Unlock()
			seen := map[string]Action{}
			for _, a := range rec.gated {
				if a.ID == "" {
					t.Fatalf("the gate was shown an action with no id: %+v", a)
				}
				if prev, ok := seen[a.ID]; ok {
					t.Errorf("id %q was given to two actions: %+v and %+v", a.ID, prev, a)
				}
				seen[a.ID] = a
			}
			if len(seen) < 2 {
				t.Fatalf("only %d gated action(s): the script proves nothing about collisions", len(seen))
			}
		})
	}
}

// Either seam on its own is enough to be numbered.
//
// Numbering is skipped only for a Runner nobody is watching at all, because the
// id exists to appear in a record and a shell with neither seam produces none —
// the same fast path a stat in a PATH search takes, where the cost has to stay a
// nil check. What must not happen is a shell that has one of the two going
// unnumbered: an audit trail with no gate is the ordinary way to watch a shell,
// and its records are exactly the ones somebody joins.
func TestEitherSeamAloneIsEnoughToBeNumbered(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*recorder, *Runner)
	}{
		{"a sink and no gate", func(rec *recorder, r *Runner) { r.Events = rec }},
		{"a gate and no sink", func(rec *recorder, r *Runner) { r.Gate = rec }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
				Session: "SESSIONUNDERTEST",
			})
			tc.build(rec, r)
			f, err := syntax.Parse("/bin/echo hi", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			rec.mu.Lock()
			defer rec.mu.Unlock()
			saw := false
			for _, a := range rec.gated {
				saw = true
				if a.ID == "" {
					t.Errorf("the gate was shown %+v with no id", a)
				}
			}
			for _, e := range rec.events {
				saw = true
				if e.Action.ID == "" {
					t.Errorf("a %v event carries no id: %+v", e.Kind, e.Action)
				}
			}
			if !saw {
				t.Fatal("the seam saw nothing, so the assertion is vacuous")
			}
		})
	}
}
