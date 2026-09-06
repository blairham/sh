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
	"github.com/blairham/sh/interp"
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
	return atThePromptWith(t, vars, style, nil, seeded...)
}

// atThePromptWith is the same session with one more seam: prepare is handed
// the Runner before the loop starts, for a test that needs the shell to know
// something a variable cannot say — an option namespace is the case this was
// added for, since a dialect's options are installed on the Runner and not
// passed in beside it.
func atThePromptWith(
	t *testing.T,
	vars map[string]string,
	style HistoryStyle,
	prepare func(*interp.Runner),
	seeded ...string,
) (*os.File, *syncBuffer, *syncBuffer, string, func()) {
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
	if prepare != nil {
		prepare(r)
	}
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
	ed := s.newEditor(t.Context())
	if ed.searchPrompt != "<search %s>" || ed.searchFailed != "<failed %s>" {
		t.Errorf("wording is %q and %q, want it carried across", ed.searchPrompt, ed.searchFailed)
	}
	if !ed.searchBelow {
		t.Error("the placement was dropped on the way to the editor")
	}
}

// A rule the dialect spells as an option reaches the loop, and reaches it
// through the shell's own option namespace.
//
// zsh's `HIST_IGNORE_SPACE` is the case. A variable would not do here: the
// namespace is installed on the Runner by the dialect, so a session that read
// the option from anywhere else would pass every unit test in this package and
// still ignore what `setopt` was told.
//
// Measured on 2026-09-06, zsh 5.9.2 with a scratch HOME, ZDOTDIR and HISTFILE:
// with `setopt HIST_IGNORE_SPACE`, the space-led line is in neither `fc -l`
// nor the file — the same answer bash gives `HISTCONTROL=ignorespace`, which
// is why the up-arrow assertion below is the one the bash-shaped test makes.
func TestTheTerminalLoopHonorsAnOptionSpelledKnob(t *testing.T) {
	style := HistoryStyle{
		IgnoreSpaceOption:            "HIST_IGNORE_SPACE",
		IgnoreDupsOption:             "HIST_IGNORE_DUPS",
		Ignore:                       "HISTORY_IGNORE",
		IgnoreIsOnePattern:           true,
		PatternIgnoredStaysInSession: true,
	}
	// A namespace shaped like a dialect's: it answers about the names it has
	// and reports every other name unknown.
	namespace := func(name string) (bool, bool) {
		switch name {
		case "HIST_IGNORE_SPACE":
			return true, true
		case "HIST_IGNORE_DUPS":
			return false, true
		}
		return false, false
	}
	prepare := func(r *interp.Runner) { r.SetOptionNamespace(namespace) }
	control, out, _, path, finish := atThePromptWith(t, nil, style, prepare, "echo seeded")

	for _, line := range []string{" echo hidden\r", "echo kept\r"} {
		if _, err := control.WriteString(line); err != nil {
			t.Fatal(err)
		}
		waitFor(t, out, "$ ", "the prompt after the line ran")
	}
	// Two steps back reaches the seeded line, the space-led one having been
	// dropped from the list as well as from the file.
	if _, err := control.WriteString("\x1b[A\x1b[A"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ echo seeded", "the entry before the kept one, the space-led line having been dropped")
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

// And the pattern knob in the same dialect gives the other answer: the line is
// out of the file and still on the up arrow.
//
// This is the axis end to end, and it is the pair that says the axis belongs
// to the knob rather than to the shell — one style, one session, two rules,
// two different fates for the line.
//
// Measured on 2026-09-06, zsh 5.9.2: with `HISTORY_IGNORE='echo hidden'`,
// `fc -l` lists `echo hidden` as entry 5 and the file written by `fc -W` goes
// straight from `echo one` to `echo two`.
func TestTheTerminalLoopKeepsAPatternIgnoredLineOnTheArrow(t *testing.T) {
	style := HistoryStyle{
		IgnoreSpaceOption:            "HIST_IGNORE_SPACE",
		Ignore:                       "HISTORY_IGNORE",
		IgnoreIsOnePattern:           true,
		PatternIgnoredStaysInSession: true,
	}
	vars := map[string]string{"HISTORY_IGNORE": "echo hidden"}
	prepare := func(r *interp.Runner) {
		r.SetOptionNamespace(func(string) (bool, bool) { return false, false })
	}
	control, out, _, path, finish := atThePromptWith(t, vars, style, prepare, "echo seeded")

	if _, err := control.WriteString("echo hidden\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "hidden\n", "the ignored line still running")
	waitFor(t, out, "$ ", "the prompt after the line ran")
	// One step back is the ignored line itself, which is the half bash does
	// not agree with.
	if _, err := control.WriteString("\x1b[A"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ echo hidden", "the ignored line, still on the arrow")
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "$ ", "the prompt after the line was abandoned")
	finish()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "echo seeded\n"; got != want {
		t.Errorf("the file holds %q, want %q — the ignored line was written", got, want)
	}
}
