// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

func vars(m map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { v, ok := m[name]; return v, ok }
}

// Where the history goes, and how a session is told not to keep one.
func TestWhereTheHistoryLives(t *testing.T) {
	h := historyFrom(vars(nil), "/home/someone")
	if h.path != filepath.Join("/home/someone", ".sh_history") {
		t.Errorf("default is %q, want one under the home directory", h.path)
	}
	if h.size != defaultHistorySize {
		t.Errorf("default size is %d, want %d", h.size, defaultHistorySize)
	}
	h = historyFrom(vars(map[string]string{"HISTFILE": "/tmp/elsewhere"}), "/home/someone")
	if h.path != "/tmp/elsewhere" {
		t.Errorf("HISTFILE gave %q", h.path)
	}
	// An empty HISTFILE is how a session says to keep nothing, and is not the
	// same as an unset one.
	if h := historyFrom(vars(map[string]string{"HISTFILE": ""}), "/home/someone"); h.path != "" {
		t.Errorf("empty HISTFILE gave %q, want no history", h.path)
	}
	// With no home and no HISTFILE there is nowhere to put it.
	if h := historyFrom(vars(nil), ""); h.path != "" {
		t.Errorf("no home gave %q, want no history", h.path)
	}
	h = historyFrom(vars(map[string]string{"HISTFILESIZE": "5"}), "/home/someone")
	if h.size != 5 {
		t.Errorf("HISTFILESIZE gave %d, want 5", h.size)
	}
	// A size that is not a number leaves the default rather than zero, which
	// would quietly turn the history off.
	if h := historyFrom(vars(map[string]string{"HISTFILESIZE": "lots"}), "/home"); h.size != defaultHistorySize {
		t.Errorf("a bad HISTFILESIZE gave %d, want the default", h.size)
	}
}

// A session reads what earlier ones left and appends only what it added.
func TestHistorySurvivesTheSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100}

	if got := h.load(t.Context()); got != nil {
		t.Errorf("a missing file gave %q, want nothing and no complaint", got)
	}
	if err := h.save(t.Context(), []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	got := h.load(t.Context())
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("loaded %q", got)
	}
	// The second session appends its own and does not rewrite the file, so
	// two shells open at once both keep what they typed.
	if err := h.save(t.Context(), []string{"three"}); err != nil {
		t.Fatal(err)
	}
	if got := h.load(t.Context()); len(got) != 3 || got[2] != "three" {
		t.Errorf("loaded %q, want three lines", got)
	}
	// The file is not readable by everyone: it is a record of what someone
	// typed.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode is %o, want 600", perm)
	}
}

// Only the last HISTFILESIZE lines are kept, so a file that has grown is
// trimmed on the way in rather than read whole.
func TestHistoryIsTrimmedToItsSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := (historyFile{path: path, size: 100}).save(t.Context(), []string{"a", "b", "c", "d"}); err != nil {
		t.Fatal(err)
	}
	got := historyFile{path: path, size: 2}.load(t.Context())
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Errorf("loaded %q, want the last two", got)
	}
}

// A history turned off reads and writes nothing at all.
func TestHistoryTurnedOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 0}
	if err := h.save(t.Context(), []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("wrote a file for a history that is off")
	}
	if got := (historyFile{}).load(t.Context()); got != nil {
		t.Errorf("loaded %q with no path", got)
	}
	// And a history with nowhere to go saves nothing, quietly. An empty
	// HISTFILE means "keep none", not "fail on the way out" — the error
	// would arrive as the shell exits, which is the worst moment for one.
	if err := (historyFile{size: defaultHistorySize}).save(t.Context(), []string{"one"}); err != nil {
		t.Errorf("saving with no path gave %v, want it quietly skipped", err)
	}
}

// Blank lines are not kept — they are what a history is most often cluttered
// with and are never worth walking back through.
func TestHistorySkipsBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := os.WriteFile(path, []byte("one\n\n   \ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := historyFile{path: path, size: 100}.load(t.Context())
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("loaded %q, want the two real lines", got)
	}
}

// A very long line is kept rather than losing the file: a pasted command can
// be far longer than a scanner's default.
func TestHistoryKeepsALongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	long := "echo " + strings.Repeat("x", 200000)
	if err := (historyFile{path: path, size: 10}).save(t.Context(), []string{long, "after"}); err != nil {
		t.Fatal(err)
	}
	got := historyFile{path: path, size: 10}.load(t.Context())
	if len(got) != 2 || got[0] != long || got[1] != "after" {
		t.Errorf("loaded %d lines, want the long one and the next", len(got))
	}
}

// A multi-line construct is one entry, not one per line.
//
// Recalling `do echo $i` on its own is recalling something that cannot run,
// and a `for` loop appearing four times over is a history nobody can walk
// back through.
func TestAMultiLineCommandIsOneEntry(t *testing.T) {
	e := &editor{}
	sh := Shell{Runner: newTestRunner(nil), Dialect: syntax.Core()}
	var pending strings.Builder
	// Typed as it would be at a prompt: the first two lines leave the
	// construct unfinished and the third completes it.
	for _, line := range []string{"for i in 1 2", "do echo n=$i", "done"} {
		_, _, _, ready := sh.accept(&pending, e.remember, line)
		if ready != (line == "done") {
			t.Fatalf("%q: ready=%v", line, ready)
		}
	}
	if len(e.history) != 1 {
		t.Fatalf("history is %q, want one entry", e.history)
	}
	if !strings.Contains(e.history[0], "for i in 1 2") ||
		!strings.Contains(e.history[0], "done") {
		t.Errorf("the entry is %q, want the whole construct", e.history[0])
	}
	// A one-line command is remembered too, and the pending text does not
	// leak into it.
	if _, _, _, ready := sh.accept(&pending, e.remember, "echo after"); !ready {
		t.Fatal("a complete line was not ready")
	}
	if len(e.history) != 2 || e.history[1] != "echo after" {
		t.Errorf("history is %q, want the second entry alone", e.history)
	}
	// And it survives a round trip through the file, newlines and all... it
	// does not: the file is a line per entry, so a multi-line command comes
	// back as several. Stated rather than asserted the other way, because it
	// is a real limit of the format and not a thing this test wants.
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100}
	if err := h.save(t.Context(), e.history); err != nil {
		t.Fatal(err)
	}
	// Three lines from the loop and one from the command after it.
	if got := h.load(t.Context()); len(got) != 4 {
		t.Errorf("loaded %q, want the four lines a line-per-entry file gives back", got)
	}
}
