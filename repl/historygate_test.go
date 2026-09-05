// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/interp"
)

// gateFor refuses every action aimed at one path and records what it was
// asked, which is the whole of what a session's policy looks like from here.
type gateFor struct {
	path  string
	asked []interp.Action
}

func (g *gateFor) Allow(_ context.Context, a interp.Action) interp.Decision {
	g.asked = append(g.asked, a)
	if a.Path == g.path {
		return interp.Deny
	}
	return interp.Allow
}

// The history is a file the *session* opens rather than one a command opens,
// and it is still inside the boundary — because HISTFILE is a shell variable,
// so the path is one a typed line can aim.
//
// Reading it and writing it are separate accesses and the write is the one
// that matters: a refused read costs a session its recall, a refused write is
// a policy stopping the shell from putting what was typed somewhere it said
// not to.
func TestARefusedHistoryFileIsNeitherReadNorWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(path, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &gateFor{path: path}
	h := historyFile{path: path, size: 10, bound: boundary.Boundary{Gate: g}}

	if got := h.load(t.Context()); got != nil {
		t.Errorf("load returned %q, want a refused history to read as none", got)
	}
	if err := h.save(t.Context(), []string{"echo typed"}); err != nil {
		t.Fatal(err)
	}
	// The file is untouched: what was there is there, and nothing was added.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo earlier\n" {
		t.Errorf("history = %q, want it unwritten", body)
	}
	// And the policy was asked twice — the read and the write — with the
	// write marked as one, since a policy may allow reading a file it will
	// not let the shell append to.
	var reads, writes int
	for _, a := range g.asked {
		if a.Kind != interp.ActionOpen || a.Path != path {
			continue
		}
		if a.Write {
			writes++
		} else {
			reads++
		}
	}
	if reads != 1 || writes != 1 {
		t.Errorf("gate saw %d reads and %d writes, want one of each", reads, writes)
	}
}

// A session with no policy opens its history exactly as it always did, which
// is the default every shell that never asked for one keeps.
func TestAnUngatedHistoryIsReadAndWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(path, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := historyFile{path: path, size: 10}

	if got := h.load(t.Context()); len(got) != 1 || got[0] != "echo earlier" {
		t.Errorf("load returned %q, want the earlier line", got)
	}
	if err := h.save(t.Context(), []string{"echo typed"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo earlier\necho typed\n" {
		t.Errorf("history = %q, want the new line appended", body)
	}
}

// Every file the session opens for itself names the run it belongs to.
//
// The history and the block store are the two, and both are assembled here
// from the same Shell as the Runner's own seams. A session identity threaded
// to one and not the other produces exactly the state this whole field exists
// to end: two records of one session that describe the same commands and
// cannot be lined up.
func TestTheSessionsOwnFilesCarryTheRunsIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")
	if err := os.WriteFile(path, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []interp.Event
	sink := interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) })
	r := newTestRunner(map[string]string{
		"HISTFILE":     path,
		"HISTFILESIZE": "10",
		blocks.DirVar:  filepath.Join(dir, "blocks"),
	})
	s := Shell{Runner: r, Events: sink, Session: "SESSIONUNDERTEST"}

	if lines := s.historyFile().load(t.Context()); len(lines) != 1 {
		t.Fatalf("history read as %q, want the earlier line", lines)
	}
	store := s.blocksStore()
	defer func() { _ = store.Close() }()
	if err := store.Record(t.Context(), blocks.Record{
		V: blocks.Version, ID: event.NewID(time.Unix(1_757_000_000, 0)), Command: "true",
	}, blocks.Output{}); err != nil {
		t.Fatal(err)
	}
	if store.Session() != "SESSIONUNDERTEST" {
		t.Errorf("the store writes as session %q, want the run's", store.Session())
	}

	if len(got) == 0 {
		t.Fatal("the session opened nothing, so the assertion is vacuous")
	}
	var paths []string
	for _, e := range got {
		paths = append(paths, e.Action.Path)
		if e.Session != "SESSIONUNDERTEST" {
			t.Errorf("the record for %s says session %q, want the run's",
				e.Action.Path, e.Session)
		}
		if e.Action.ID == "" {
			t.Errorf("the record for %s carries no action id", e.Action.Path)
		}
	}
	if !slices.Contains(paths, path) {
		t.Errorf("opened %q, want the history file among them", paths)
	}
}
