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

// promptSession is a shell at a prompt on a pseudo-terminal, and the owner of
// the rule above.
//
// The rule is one sentence — **wait for the next prompt before sending a key,
// and the helper does the waiting** — and #1018 is what it cost to have it
// written in three places and meant in two. `driver`'s screen.endSession has
// always owned it. capturingSession owns it since #1014. This one left it to
// every call site, which is how a fifth instance of #635 gets written by
// somebody who copied the fourth.
//
// It cannot be shared as code: these tests are `package repl` because they use
// unexported things, and `driver` imports `repl`, so the direction the reuse
// would have to go is a cycle. What can be shared is the rule, and each helper
// states it and owns it.
//
// atPrompt is what makes owning it safe rather than merely dutiful. Folding
// the wait into finish and leaving the call sites alone hangs **all seven** of
// them — measured, and it is the reason #1018 exists rather than a patch: the
// buffer carries a cursor and has to, because the editor redraws the prompt on
// every keystroke, so a caller that already waited has consumed the prompt and
// a second wait blocks on one that is never drawn again. The flag says whether
// the prompt has been consumed since the last key went out, so finish waits
// exactly when a wait is owed. A call site that waits and one that does not are
// both correct, which is the property the two other helpers do not yet have and
// the one that makes the rule unforgettable.
//
// What this is not: a live bug removed. These sessions hand the Shell a buffer
// for Out and Err and only the terminal for In, so the output a caller waits
// on never crosses the pseudo-terminal, and the #635 window does not open the
// way it does for capturingSession — which gives the Shell the terminal for
// all three. Measured, with the wait taken out of both helpers entirely and
// run on Linux at one core under load: the conduit test fails 6 times in 60,
// and three callers of *this* helper fail 0 times in 200 each. So the wait
// here is preventive and costs one prompt. It is worth having because the
// susceptibility is a property of how a session was wired and not of the rule,
// and the next test to want the editor's drawing on a real terminal will wire
// it the other way — #635 is what happens then, and the fifth instance is the
// one this is meant to prevent rather than to fix.
type promptSession struct {
	t        *testing.T
	control  *os.File
	out      *syncBuffer
	errs     *syncBuffer
	path     string
	done     chan error
	tty      *os.File
	atPrompt bool
	finished bool
}

// send puts keys on the terminal, and records that the shell is no longer
// known to be reading.
func (s *promptSession) send(keys string) {
	s.t.Helper()
	s.atPrompt = false
	if _, err := s.control.WriteString(keys); err != nil {
		s.t.Fatal(err)
	}
}

// waitFor waits for anything that is not a prompt. It says nothing about
// whether the shell is reading again, because output arrives while the
// terminal is still being handed back — which is exactly the window #635 is
// about, and exactly the mistake #1014 fixed in the other helper.
func (s *promptSession) waitFor(want, what string) {
	s.t.Helper()
	waitFor(s.t, s.out, want, what)
}

// waitForPrompt waits for the next prompt, which is drawn after raw mode is
// restored and immediately before the read — an order the two cannot swap.
func (s *promptSession) waitForPrompt(what string) {
	s.t.Helper()
	waitFor(s.t, s.out, "$ ", what)
	s.atPrompt = true
}

// finish ends the session with a ^D, once the shell is reading again.
func (s *promptSession) finish() {
	s.t.Helper()
	if s.finished {
		return
	}
	s.finished = true
	if !s.atPrompt {
		s.waitForPrompt("the prompt that says the shell is reading again")
	}
	s.send("\x04")
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		s.t.Fatal("the shell did not exit on ^D")
	}
	_ = s.tty.Close()
	_ = s.control.Close()
}

// atThePrompt starts a session on a pseudo-terminal with a history of its own.
func atThePrompt(t *testing.T, vars map[string]string, style HistoryStyle, seeded ...string) *promptSession {
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
) *promptSession {
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

	sess := &promptSession{t: t, control: control, out: out, errs: errs, path: path, done: done, tty: tty}
	sess.waitForPrompt("the first prompt")
	return sess
}

// TestASessionEndsWhetherOrNotTheCallerWaitedForThePrompt is the ownership
// rule as something that runs, and it is the half #1018 could not have by
// folding.
//
// Two callers, identical but for one line: one leaves the prompt entirely to
// finish, and one waits for it itself because it wants to assert on what the
// prompt was. Both have to end the session. That is what "the helper owns it"
// has to mean if it is to be worth more than a convention — a rule that only
// works when the caller also knows the rule is not owned anywhere.
//
// Without the flag the second caller hangs for the full deadline: the buffer's
// cursor has moved past the prompt it consumed, and finish's own wait then
// blocks on one that is never drawn again. That is the measured reason the
// naive fold fails, and with the call sites of the day it failed all seven.
func TestASessionEndsWhetherOrNotTheCallerWaitedForThePrompt(t *testing.T) {
	for _, tc := range []struct {
		name           string
		waitsForItself bool
	}{
		{"the caller leaves the prompt to finish", false},
		{"the caller waited for the prompt itself", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := atThePrompt(t, nil, HistoryStyle{})
			// An expression, so the wait on its value cannot be answered by
			// the terminal echoing the line back.
			sess.send("echo mark-$((6*7))\r")
			sess.waitFor("mark-42", "the command's output")
			if tc.waitsForItself {
				sess.waitForPrompt("the prompt after the line ran")
			}
			sess.finish()
		})
	}
}

// `C-r` at a real prompt finds an earlier line and runs it.
func TestTheTerminalLoopSearchesTheHistory(t *testing.T) {
	style := HistoryStyle{
		SearchPrompt:       "(reverse-i-search)`%s': ",
		SearchFailedPrompt: "(failed reverse-i-search)`%s': ",
	}
	sess := atThePrompt(t, nil, style, "echo alpha", "echo beta")

	sess.send("\x12alpha")
	// The wording arrives before the line is accepted, which is the half a
	// test of the returned string cannot see.
	sess.waitFor("(reverse-i-search)`alpha': echo alpha", "the search on screen")
	sess.send("\r")
	sess.waitFor("alpha\n", "the output of the line the search found")
	sess.finish()
}

// The up arrow reaches what an earlier session left, and the search reaches
// past the newest of it.
func TestTheTerminalLoopRecallsWhatWasLoaded(t *testing.T) {
	sess := atThePrompt(t, nil, HistoryStyle{}, "echo alpha", "echo beta")

	sess.send("\x1b[A\x1b[A\r")
	sess.waitFor("alpha\n", "two steps back through the loaded history")
	sess.finish()
}

// The knobs reach the loop, and the two lists they produce are both right: the
// ignored line is neither recallable nor written when the dialect forgets it.
func TestTheTerminalLoopHonorsTheKnobs(t *testing.T) {
	style := HistoryStyle{Control: "HISTCONTROL", Ignore: "HISTIGNORE"}
	vars := map[string]string{"HISTCONTROL": "ignorespace"}
	sess := atThePrompt(t, vars, style, "echo seeded")

	for _, line := range []string{" echo hidden\r", "echo kept\r"} {
		sess.send(line)
		sess.waitForPrompt("the prompt after the line ran")
	}
	// The up arrow skips straight past the hidden line to the kept one.
	sess.send("\x1b[A\x1b[A")
	sess.waitFor("$ echo seeded", "the entry before the kept one, the hidden line having been dropped")
	sess.send("\x03")
	sess.finish()

	raw, err := os.ReadFile(sess.path)
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
	sess := atThePrompt(t, vars, HistoryStyle{},
		"echo one", "echo two", "echo three", "echo four")

	// Four steps back, on a list two long: the fourth and third are
	// reachable and the walk stops rather than wrapping.
	sess.send("\x1b[A\x1b[A\x1b[A\x1b[A\r")
	sess.waitFor("three\n", "the oldest line HISTSIZE allows")
	sess.finish()

	// And the file follows, because HISTFILESIZE defaults to HISTSIZE.
	// Measured: bash 5.3.15 given only `HISTSIZE=2` leaves two lines in the
	// file however many were there before.
	raw, err := os.ReadFile(sess.path)
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
	sess := atThePrompt(t, vars, HistoryStyle{},
		"echo one", "echo two", "echo three", "echo four")

	sess.send("echo added\r")
	sess.waitFor("added\n", "the line running")
	sess.finish()

	raw, err := os.ReadFile(sess.path)
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
	ed := s.newEditor(t.Context(), nil)
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
	sess := atThePromptWith(t, nil, style, prepare, "echo seeded")

	for _, line := range []string{" echo hidden\r", "echo kept\r"} {
		sess.send(line)
		sess.waitForPrompt("the prompt after the line ran")
	}
	// Two steps back reaches the seeded line, the space-led one having been
	// dropped from the list as well as from the file.
	sess.send("\x1b[A\x1b[A")
	sess.waitFor("$ echo seeded", "the entry before the kept one, the space-led line having been dropped")
	sess.send("\x03")
	sess.finish()

	raw, err := os.ReadFile(sess.path)
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
	sess := atThePromptWith(t, vars, style, prepare, "echo seeded")

	sess.send("echo hidden\r")
	sess.waitFor("hidden\n", "the ignored line still running")
	sess.waitFor("$ ", "the prompt after the line ran")
	// One step back is the ignored line itself, which is the half bash does
	// not agree with.
	sess.send("\x1b[A")
	sess.waitFor("$ echo hidden", "the ignored line, still on the arrow")
	sess.send("\x03")
	sess.finish()

	raw, err := os.ReadFile(sess.path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "echo seeded\n"; got != want {
		t.Errorf("the file holds %q, want %q — the ignored line was written", got, want)
	}
}
