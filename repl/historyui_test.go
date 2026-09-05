// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// The history surface through the loop that actually runs, on a real
// pseudo-terminal.
//
// The editor's own tests feed it bytes, which proves the mode and says nothing
// about whether `Run` wires it up: a search that works perfectly on an editor
// nothing installs looks exactly like a working feature until someone presses
// `C-r`. The terminal loop is also the only one with a history at all — the
// piped loop has no editor — so this is the only place the two halves meet.
//
// Every wait is on the *next prompt* rather than on a duration. The shell puts
// the terminal back in its own line discipline to run a line and takes it into
// raw mode afterwards, and the kernel drops what is queued but unread across
// that change, so input sent any earlier is simply gone (#635).

// atThePrompt starts a session on a pseudo-terminal with a history of its own.
func atThePrompt(t *testing.T, vars map[string]string, style HistoryStyle, seeded ...string) (*os.File, *syncBuffer, *syncBuffer, string, func()) {
	t.Helper()
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}

	path := filepath.Join(t.TempDir(), "hist")
	if len(seeded) > 0 {
		if err := os.WriteFile(path, []byte(strings.Join(seeded, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if vars == nil {
		vars = map[string]string{}
	}
	vars["HISTFILE"], vars["PS1"] = path, "$ "

	out, errs := &syncBuffer{}, &syncBuffer{}
	r := newTestRunner(vars)
	r.Stdout, r.Stderr = out, errs
	s := Shell{Runner: r, In: tty, Out: out, Err: errs, Name: "sh", History: style}

	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context())
		done <- err
	}()

	waitFor(t, out, "$ ", "the first prompt")
	finish := func() {
		t.Helper()
		if _, err := control.WriteString("\x04"); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("the shell did not exit on ^D")
		}
		_ = tty.Close()
		_ = control.Close()
	}
	return control, out, errs, path, finish
}

// `C-r` at a real prompt finds an earlier line and runs it.
func TestTheTerminalLoopSearchesTheHistory(t *testing.T) {
	style := HistoryStyle{
		SearchPrompt:       "(reverse-i-search)`%s': ",
		SearchFailedPrompt: "(failed reverse-i-search)`%s': ",
	}
	control, out, _, _, finish := atThePrompt(t, nil, style, "echo alpha", "echo beta")

	if _, err := control.WriteString("\x12alpha"); err != nil {
		t.Fatal(err)
	}
	// The wording arrives before the line is accepted, which is the half a
	// test of the returned string cannot see.
	waitFor(t, out, "(reverse-i-search)`alpha': echo alpha", "the search on screen")
	if _, err := control.WriteString("\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "alpha\n", "the output of the line the search found")
	waitFor(t, out, "$ ", "the prompt after the line ran")
	finish()
}

// The up arrow reaches what an earlier session left, and the search reaches
// past the newest of it.
func TestTheTerminalLoopRecallsWhatWasLoaded(t *testing.T) {
	control, out, _, _, finish := atThePrompt(t, nil, HistoryStyle{}, "echo alpha", "echo beta")

	if _, err := control.WriteString("\x1b[A\x1b[A\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "alpha\n", "two steps back through the loaded history")
	waitFor(t, out, "$ ", "the prompt after the line ran")
	finish()
}

// The knobs reach the loop, and the two lists they produce are both right: the
// ignored line is neither recallable nor written when the dialect forgets it.
func TestTheTerminalLoopHonorsTheKnobs(t *testing.T) {
	style := HistoryStyle{Control: "HISTCONTROL", Ignore: "HISTIGNORE"}
	vars := map[string]string{"HISTCONTROL": "ignorespace"}
	control, out, _, path, finish := atThePrompt(t, vars, style, "echo seeded")

	for _, line := range []string{" echo hidden\r", "echo kept\r"} {
		if _, err := control.WriteString(line); err != nil {
			t.Fatal(err)
		}
		waitFor(t, out, "$ ", "the prompt after the line ran")
	}
	// The up arrow skips straight past the hidden line to the kept one.
	if _, err := control.WriteString("\x1b[A\x1b[A"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ echo seeded", "the entry before the kept one, the hidden line having been dropped")
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ ", "the prompt after the line was abandoned")
	finish()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "echo seeded\necho kept\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}
}

// HISTSIZE bounds what the session can recall, and the file keeps what the
// session could not.
//
// Measured: bash 5.3.15 with `HISTSIZE=2` and four lines in the file lets the
// up arrow reach two of them and stops there.
func TestTheTerminalLoopBoundsWhatItRecalls(t *testing.T) {
	vars := map[string]string{"HISTSIZE": "2"}
	control, out, _, path, finish := atThePrompt(t, vars, HistoryStyle{},
		"echo one", "echo two", "echo three", "echo four")

	// Four steps back, on a list two long: the fourth and third are
	// reachable and the walk stops rather than wrapping.
	if _, err := control.WriteString("\x1b[A\x1b[A\x1b[A\x1b[A\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "three\n", "the oldest line HISTSIZE allows")
	waitFor(t, out, "$ ", "the prompt after the line ran")
	finish()

	// And the file follows, because HISTFILESIZE defaults to HISTSIZE.
	// Measured: bash 5.3.15 given only `HISTSIZE=2` leaves two lines in the
	// file however many were there before.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "echo four\necho three\n"; got != want {
		t.Errorf("the file holds %q, want %q", got, want)
	}
}

// The two bounds are two questions, and saying so keeps the file.
//
// Measured: bash 5.3.15 with `HISTSIZE=2 HISTFILESIZE=100` recalls two lines
// and leaves every earlier one on disk. Without the second variable it leaves
// two, which is the case above — so this is the test that would catch one
// variable being read for both.
func TestABigFileBoundKeepsWhatASmallHistsizeCannotRecall(t *testing.T) {
	vars := map[string]string{"HISTSIZE": "2", "HISTFILESIZE": "100"}
	control, out, _, path, finish := atThePrompt(t, vars, HistoryStyle{},
		"echo one", "echo two", "echo three", "echo four")

	if _, err := control.WriteString("echo added\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "added\n", "the line running")
	waitFor(t, out, "$ ", "the prompt after the line ran")
	finish()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "echo one\necho two\necho three\necho four\necho added\n"
	if string(raw) != want {
		t.Errorf("the file holds %q, want %q", raw, want)
	}
}

// What a dialect says about the search reaches the editor.
//
// The same hazard the driver's own wiring test was written for, one layer
// down: the editor is built only where there is a terminal, so a field dropped
// on the way here looks exactly like a dialect that did not answer. The
// placement flag is the one that would go unnoticed longest — the wording
// still arrives, and it is merely in the wrong place.
func TestTheDialectsSearchReachesTheEditor(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(nil),
		History: HistoryStyle{
			SearchPrompt:       "<search %s>",
			SearchFailedPrompt: "<failed %s>",
			SearchBelowTheLine: true,
		},
	}
	ed := s.newEditor()
	if ed.searchPrompt != "<search %s>" || ed.searchFailed != "<failed %s>" {
		t.Errorf("wording is %q and %q, want it carried across", ed.searchPrompt, ed.searchFailed)
	}
	if !ed.searchBelow {
		t.Error("the placement was dropped on the way to the editor")
	}
}
