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
	// The two sizes are two questions. HISTSIZE bounds what the session can
	// recall; HISTFILESIZE bounds what the file keeps, and defaults to
	// HISTSIZE's value rather than to the substrate's own.
	h = historyFrom(vars(map[string]string{"HISTSIZE": "5"}), "/home/someone")
	if h.size != 5 {
		t.Errorf("HISTSIZE gave %d, want 5", h.size)
	}
	if h.file != 5 {
		t.Errorf("HISTFILESIZE defaulted to %d, want HISTSIZE's 5", h.file)
	}
	h = historyFrom(vars(map[string]string{"HISTSIZE": "5", "HISTFILESIZE": "9"}), "/home/someone")
	if h.size != 5 || h.file != 9 {
		t.Errorf("sizes are %d recalled and %d on disk, want 5 and 9", h.size, h.file)
	}
	// A size that is not a number leaves the default rather than zero, which
	// would quietly turn the history off — and zero is the one value that
	// does, so reading nonsense as zero would be the destructive reading.
	if h := historyFrom(vars(map[string]string{"HISTSIZE": "lots"}), "/home"); h.size != defaultHistorySize {
		t.Errorf("a bad HISTSIZE gave %d, want the default", h.size)
	}
	if h := historyFrom(vars(map[string]string{"HISTFILESIZE": "-3"}), "/home"); h.file != defaultHistorySize {
		t.Errorf("a negative HISTFILESIZE gave %d, want the default", h.file)
	}
	if h := historyFrom(vars(map[string]string{"HISTSIZE": "0"}), "/home"); h.size != 0 {
		t.Errorf("HISTSIZE=0 gave %d, want a session that recalls nothing", h.size)
	}
}

// A session reads what earlier ones left and appends only what it added.
func TestHistorySurvivesTheSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100, file: 100}

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

// Only the last HISTSIZE lines reach the session, so a file longer than the
// list a person can walk is trimmed on the way in rather than read whole.
func TestHistoryIsTrimmedToItsSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := (historyFile{path: path, size: 100, file: 100}).save(t.Context(), []string{"a", "b", "c", "d"}); err != nil {
		t.Fatal(err)
	}
	got := historyFile{path: path, size: 2, file: 2}.load(t.Context())
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Errorf("loaded %q, want the last two", got)
	}
}

// A history turned off reads and writes nothing at all.
func TestHistoryTurnedOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 0, file: 0}
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
	if err := (historyFile{size: defaultHistorySize, file: defaultHistorySize}).save(t.Context(), []string{"one"}); err != nil {
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
	got := historyFile{path: path, size: 100, file: 100}.load(t.Context())
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("loaded %q, want the two real lines", got)
	}
}

// A very long line is kept rather than losing the file: a pasted command can
// be far longer than a scanner's default.
func TestHistoryKeepsALongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	long := "echo " + strings.Repeat("x", 200000)
	if err := (historyFile{path: path, size: 10, file: 10}).save(t.Context(), []string{long, "after"}); err != nil {
		t.Fatal(err)
	}
	got := historyFile{path: path, size: 10, file: 10}.load(t.Context())
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
	h := historyFile{path: path, size: 100, file: 100}
	if err := h.save(t.Context(), e.history); err != nil {
		t.Fatal(err)
	}
	// Three lines from the loop and one from the command after it.
	if got := h.load(t.Context()); len(got) != 4 {
		t.Errorf("loaded %q, want the four lines a line-per-entry file gives back", got)
	}
}

// HISTFILESIZE is a bound on the file rather than a suggestion.
//
// Measured on 2026-09-05: bash 5.3.15 with four lines in the file, HISTFILESIZE=3
// and two lines typed leaves exactly the last three. The append is still the
// ordinary path; the rewrite happens only when the file is over the bound.
func TestTheFileIsBroughtBackUnderItsBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100, file: 3}
	if err := h.save(t.Context(), []string{"a", "b", "c", "d"}); err != nil {
		t.Fatal(err)
	}
	if got := h.load(t.Context()); strings.Join(got, "|") != "b|c|d" {
		t.Errorf("the file holds %q, want the last three", got)
	}
	// A second session appends and is trimmed again, so the bound holds
	// across sessions rather than only within one.
	if err := h.save(t.Context(), []string{"e"}); err != nil {
		t.Fatal(err)
	}
	if got := h.load(t.Context()); strings.Join(got, "|") != "c|d|e" {
		t.Errorf("the file holds %q, want the last three after a second session", got)
	}
	// Still not readable by everyone. The rewrite goes through a temporary
	// file, and a temporary file created carelessly is how a private file
	// becomes a public one.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode after a rewrite is %o, want 600", perm)
	}
	// And nothing is left behind beside it.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, want only the history", len(entries))
	}
}

// A file already under the bound is left exactly as it was, rather than
// rewritten for the sake of it: the append is the normal path, and two shells
// exiting at once both keep their lines because of it.
func TestAFileUnderItsBoundIsNotRewritten(t *testing.T) {
	// Landing exactly on the bound rather than under it, which is where an
	// off-by-one would put the rewrite: two lines, then a third, in a file
	// allowed three.
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100, file: 3}
	if err := h.save(t.Context(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.save(t.Context(), []string{"c"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Sys() == nil || after.Sys() == nil {
		t.Skip("no file identity to compare on this platform")
	}
	if !os.SameFile(before, after) {
		t.Error("an append replaced the file, which is what loses another shell's lines")
	}
	if got := h.load(t.Context()); strings.Join(got, "|") != "a|b|c" {
		t.Errorf("the file holds %q, want all three", got)
	}
}

// HISTFILESIZE=0 writes nothing, and HISTSIZE=0 leaves the session with
// nothing to write.
func TestZeroTurnsTheWritingOff(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    historyFile
	}{
		{name: "HISTSIZE=0", h: historyFile{size: 0, file: 100}},
		{name: "HISTFILESIZE=0", h: historyFile{size: 100, file: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.h.path = filepath.Join(t.TempDir(), "hist")
			if err := tc.h.save(t.Context(), []string{"one"}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(tc.h.path); err == nil {
				t.Error("a file was written for a session told to keep nothing")
			}
		})
	}
}
