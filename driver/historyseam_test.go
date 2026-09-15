// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason promptproviders_test.go is: the thing asserted is
// that an unexported builder reads a field, and the session it builds exists
// only where there is a terminal.
package driver

import (
	"context"
	"testing"
	"time"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// A history tool a binary attaches reaches the session.
//
// The seam itself is repl's and has been for a while. What #804 was asking for
// is somewhere to *attach* one, and until this the fields were on repl.Shell
// only — so a binary composing a shell through this package, which is every
// binary here and the one arrangement this package exists to make possible,
// had no way to reach them short of writing a second front end.
//
// Worth a test for the reason the prompt providers are: the session is built
// only where there is a terminal, so a field dropped on the way across looks
// exactly like a binary that attached nothing.
func TestAHistoryToolReachesTheSession(t *testing.T) {
	var recorded []repl.HistoryEntry
	recorder := repl.HistoryRecorderFunc(func(e repl.HistoryEntry) {
		recorded = append(recorded, e)
	})
	source := repl.HistorySourceFunc(func(context.Context) []string {
		return []string{"from the tool"}
	})
	sh := Shell{
		Name:             "testsh",
		HistoryRecorders: []repl.HistoryRecorder{recorder},
		HistorySources:   []repl.HistorySource{source},
	}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)

	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if len(front.HistoryRecorders) != 1 {
		t.Fatalf("the session carries %d recorders, want 1", len(front.HistoryRecorders))
	}
	if len(front.HistorySources) != 1 {
		t.Fatalf("the session carries %d sources, want 1", len(front.HistorySources))
	}
	// Exercised rather than counted: a field that arrived holding the wrong
	// function is a wiring fault a length cannot see.
	front.HistoryRecorders[0].Record(repl.HistoryEntry{Command: "echo hi", At: time.Unix(1, 0)})
	if len(recorded) != 1 || recorded[0].Command != "echo hi" {
		t.Errorf("the recorder that arrived was told %v, want the one line", recorded)
	}
	if got := front.HistorySources[0].Lines(t.Context()); len(got) != 1 || got[0] != "from the tool" {
		t.Errorf("the source that arrived answered %v, want the tool's line", got)
	}
}

// And a binary that attaches none leaves the session's history to the file.
func TestAShellWithoutAHistoryToolCarriesNoneToTheSession(t *testing.T) {
	sh := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if n := len(front.HistoryRecorders); n != 0 {
		t.Errorf("the session carries %d recorders, want none", n)
	}
	if n := len(front.HistorySources); n != 0 {
		t.Errorf("the session carries %d sources, want none", n)
	}
}
