// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recordingHistory keeps every entry it was told.
type recordingHistory struct{ got []HistoryEntry }

func (r *recordingHistory) Record(e HistoryEntry) { r.got = append(r.got, e) }

// recorded drives one accepted line through the session's recording rules and
// hands back what the file collected and what a recorder was told.
func recorded(t *testing.T, s Shell, rec *recordingHistory, lines ...string) ([]string, *syncBuffer) {
	t.Helper()
	errs := &syncBuffer{}
	s.Err, s.Name = errs, "sh"
	s.HistoryRecorders = []HistoryRecorder{rec}
	ed := &editor{}
	var added []string
	remember := s.recording(ed, &added)
	for _, line := range lines {
		remember(line)
	}
	return added, errs
}

// What a recorder is told, as one value: the line as typed, where the shell
// was, when, and which run it was.
//
// The whole entry rather than the command alone, so a field that stopped being
// filled in is a failure rather than an untested field.
func TestARecorderIsToldTheLineAndWhereItWasTyped(t *testing.T) {
	at := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	r := newTestRunner(nil)
	r.Dir = "/where/it/was/typed"
	rec := &recordingHistory{}
	s := Shell{
		Runner: r, Session: "session-1",
		Clock: func() time.Time { return at },
	}

	added, _ := recorded(t, s, rec, "echo hi")

	want := []HistoryEntry{{
		Command: "echo hi", Dir: "/where/it/was/typed", At: at, Session: "session-1",
	}}
	if !reflect.DeepEqual(rec.got, want) {
		t.Errorf("the recorder was told\n got %+v\nwant %+v", rec.got, want)
	}
	// And the file still collected it, because a recorder is an addition.
	if !reflect.DeepEqual(added, []string{"echo hi"}) {
		t.Errorf("the file collected %q, want %q", added, []string{"echo hi"})
	}
}

// The rules a line has already passed by the time a recorder sees it.
//
// This is the load-bearing half of putting the seam where it is. A recorder
// fed anywhere else would need its own copy of each of these, and the one that
// matters is the first: a session that refuses to write a credential to its
// own file and then hands it to a history tool has moved the leak rather than
// stopped it.
func TestARecorderIsToldWhatTheFileIsToldAndNothingElse(t *testing.T) {
	for _, c := range []struct {
		name  string
		vars  map[string]string
		lines []string
		want  []string
	}{
		{
			name:  "a blank line is nothing happening",
			lines: []string{"echo one", "   ", "", "echo two"},
			want:  []string{"echo one", "echo two"},
		},
		{
			name:  "a line carrying a credential is refused",
			lines: []string{"echo one", "export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY", "echo two"},
			want:  []string{"echo one", "echo two"},
		},
		{
			name:  "a line the session was told to ignore",
			vars:  map[string]string{"HISTIGNORE": "secret*"},
			lines: []string{"echo one", "secret thing", "echo two"},
			want:  []string{"echo one", "echo two"},
		},
		{
			name:  "a line beginning with a space, where that is the rule",
			vars:  map[string]string{"HISTCONTROL": "ignorespace"},
			lines: []string{"echo one", " echo hidden", "echo two"},
			want:  []string{"echo one", "echo two"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := &recordingHistory{}
			s := Shell{Runner: newTestRunner(c.vars), History: bashishHistory}
			added, _ := recorded(t, s, rec, c.lines...)

			var told []string
			for _, e := range rec.got {
				told = append(told, e.Command)
			}
			if !reflect.DeepEqual(told, c.want) {
				t.Errorf("the recorder was told %q, want %q", told, c.want)
			}
			// And the two agree, which is the property rather than a
			// coincidence of this table: what a recorder hears is what the
			// file keeps.
			if !reflect.DeepEqual(added, c.want) {
				t.Errorf("the file collected %q, want the same %q", added, c.want)
			}
		})
	}
}

// Every recorder is told, because a recorder is not answering a question.
func TestEveryRecorderIsTold(t *testing.T) {
	first, second := &recordingHistory{}, &recordingHistory{}
	s := Shell{Runner: newTestRunner(nil), HistoryRecorders: []HistoryRecorder{first, nil, second}}
	ed := &editor{}
	var added []string
	s.recording(ed, &added)("echo hi")

	for name, r := range map[string]*recordingHistory{"first": first, "second": second} {
		if len(r.got) != 1 || r.got[0].Command != "echo hi" {
			t.Errorf("the %s recorder was told %+v, want one entry for `echo hi`", name, r.got)
		}
	}
}

// A recorder that panics costs its own record and nothing else: the file still
// has the line, the recorders after it are still told, and the session goes on.
func TestARecorderThatPanicsCostsOnlyItsOwnRecord(t *testing.T) {
	after := &recordingHistory{}
	errs := &syncBuffer{}
	s := Shell{
		Runner: newTestRunner(nil), Err: errs, Name: "sh",
		HistoryRecorders: []HistoryRecorder{
			HistoryRecorderFunc(func(HistoryEntry) { panic("a bug in a history tool") }),
			after,
		},
	}
	ed := &editor{}
	var added []string
	s.recording(ed, &added)("echo hi")

	if !reflect.DeepEqual(added, []string{"echo hi"}) {
		t.Errorf("the file collected %q, want %q", added, []string{"echo hi"})
	}
	if len(after.got) != 1 {
		t.Errorf("the recorder after the one that panicked was told %+v, want one entry", after.got)
	}
	if !strings.Contains(errs.String(), "a bug in a history tool") {
		t.Errorf("the panic was not reported: %q", errs.String())
	}
}

// A source's lines come before the file's, so the file is the recent end of
// the walk and a tool's database is the older half.
func TestASourcesLinesComeBeforeTheFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := os.WriteFile(path, []byte("from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Shell{
		Runner: newTestRunner(map[string]string{"HISTFILE": path}),
		HistorySources: []HistorySource{
			HistorySourceFunc(func(context.Context) []string { return []string{"older one", "older two"} }),
			nil,
			HistorySourceFunc(func(context.Context) []string { return []string{"older three"} }),
		},
	}
	got := s.recalled(t.Context(), s.historyFile())
	want := []string{"older one", "older two", "older three", "from the file"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the session recalls\n got %q\nwant %q", got, want)
	}
}

// A session with no sources recalls the file and nothing else, which is what
// every session did before this seam existed.
func TestASessionWithoutSourcesRecallsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Shell{Runner: newTestRunner(map[string]string{"HISTFILE": path})}
	if got := s.recalled(t.Context(), s.historyFile()); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("the session recalls %q, want %q", got, []string{"one", "two"})
	}
}

// A source that panics costs its own lines and does not stop the session
// starting — which is the whole reason it is guarded.
func TestASourceThatPanicsDoesNotStopTheSession(t *testing.T) {
	errs := &syncBuffer{}
	s := Shell{
		Runner: newTestRunner(map[string]string{"HISTFILE": ""}), Err: errs, Name: "sh",
		HistorySources: []HistorySource{
			HistorySourceFunc(func(context.Context) []string { panic("a bug in a history tool") }),
			HistorySourceFunc(func(context.Context) []string { return []string{"still here"} }),
		},
	}
	got := s.recalled(t.Context(), s.historyFile())
	if !reflect.DeepEqual(got, []string{"still here"}) {
		t.Errorf("the session recalls %q, want %q", got, []string{"still here"})
	}
	if !strings.Contains(errs.String(), "a bug in a history tool") {
		t.Errorf("the panic was not reported: %q", errs.String())
	}
}

// Both halves through a real terminal, in a real session.
//
// This is the wiring, and it is invisible from anywhere else: the editor and
// its history exist only where there is a terminal, so a field dropped between
// the Shell and the loop looks exactly like a front end that supplied nothing.
//
// The up arrow reaches a line no session ever typed — it came from the source
// — and running it proves the line is real rather than merely drawn. The
// recorder then has to have been told about it.
func TestASourceAndARecorderReachARealSession(t *testing.T) {
	rec := &recordingHistory{}
	dir := t.TempDir()
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Dir = dir
		sh.HistorySources = []HistorySource{
			HistorySourceFunc(func(context.Context) []string { return []string{"echo from-the-tool"} }),
		}
		sh.HistoryRecorders = []HistoryRecorder{rec}
	})

	// Up, then Return: the line the source supplied, run.
	s.typeLine("\x1b[A\n")
	waitFor(t, s.ran, "from-the-tool", "the recalled line's output")
	s.end()

	var told []string
	for _, e := range rec.got {
		told = append(told, e.Command)
	}
	if !reflect.DeepEqual(told, []string{"echo from-the-tool"}) {
		t.Errorf("the recorder was told %q, want %q", told, []string{"echo from-the-tool"})
	}
	if rec.got[0].Dir != dir {
		t.Errorf("the recorder was told the directory %q, want the shell's %q", rec.got[0].Dir, dir)
	}
}
