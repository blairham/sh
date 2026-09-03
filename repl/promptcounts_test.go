// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The two numbers look alike and are not.
//
// Measured with three lines already in the history file: bash drew 4 for the
// history number at the first prompt and 1 for the command number, because
// the history carries across sessions and the count of commands does not.
func TestTheHistoryNumberAndTheCommandNumber(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(nil),
		Style: PromptStyle{
			Escape: '\\',
			Codes:  map[rune]PromptField{'!': FieldHistoryNumber, '#': FieldCommandNumber},
		},
		counts: &counts{history: 3},
	}
	// Both are the number the *next* line will have, so a prompt drawn
	// before anything is typed says one more than has happened.
	if got := s.render(`<!\! #\#>`); got != "<!4 #1>" {
		t.Errorf("drew %q, want <!4 #1>", got)
	}
	s.counts.history++
	s.counts.command++
	if got := s.render(`<!\! #\#>`); got != "<!5 #2>" {
		t.Errorf("drew %q, want <!5 #2>", got)
	}
	// Without either loop having run there is still a number to draw.
	bare := Shell{Style: s.Style}
	if got := bare.render(`<!\! #\#>`); got != "<!1 #1>" {
		t.Errorf("drew %q, want <!1 #1>", got)
	}
}

// The two loops keep the numbers, and they keep them differently: only one of
// them has a history file behind it.
func TestBothLoopsCountTheLines(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "n=1\nn=2\n")
	r := newTestRunner(map[string]string{
		"PS1": `<\!:\#>`, "HISTFILE": filepath.Join(t.TempDir(), "history"),
	})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Escape: '\\',
			Codes:  map[rune]PromptField{'!': FieldHistoryNumber, '#': FieldCommandNumber},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Three prompts for two lines, the last one asking for a third.
	if got := errs.String(); got != "<1:1><2:2><3:3>" {
		t.Errorf("prompts = %q, want <1:1><2:2><3:3>", got)
	}
}

// A construct typed over several lines is one entry in the history and one
// command, not one of each per line.
//
// Measured: `: before`, a four-line for loop, then `: after` drew 1, 2 and 3
// in real bash — not 1, 6, 7. accept already kept the history that way,
// remembering the whole accumulated text as one line; only the numbering
// disagreed with it.
func TestAConstructIsOneEntryHoweverManyLinesItTook(t *testing.T) {
	if got := prompts(t, "PS1", `<\!:\#>`, ": before\nfor i in 1\ndo\n:\ndone\n: after\n"); got != "<1:1><2:2><3:3><4:4>" {
		t.Errorf("prompts = %q, want the loop counted once", got)
	}
}

// A line that will not parse is remembered and is not a command, so the two
// numbers come apart within one session.
//
// Measured: bash draws `!3 #2` at the prompt after a line that would not
// parse, and `!4 #3` after the next one that would.
func TestALineThatWillNotParseIsHistoryAndNotACommand(t *testing.T) {
	if got := prompts(t, "PS1", `<\!:\#>`, ": one\nfor\n: three\n"); got != "<1:1><2:2><3:2><4:3>" {
		t.Errorf("prompts = %q, want the bad line remembered and not run", got)
	}
}

// An empty line is neither.
func TestAnEmptyLineIsNeither(t *testing.T) {
	if got := prompts(t, "PS1", `<\!:\#>`, ": one\n\n: three\n"); got != "<1:1><2:2><2:2><3:3>" {
		t.Errorf("prompts = %q, want the empty line to count for nothing", got)
	}
}

// A job the shell is still looking after is counted, and one that has finished
// is not: it stays in the table until its notice has been given, and counting
// it would say something was running for exactly as long as it takes to say
// that it is not.
func TestTheCountOfJobs(t *testing.T) {
	var out, errs strings.Builder
	// Long enough that both prompts see it running, short enough that it is
	// gone soon after the test is.
	in := readerFile(t, "sleep 2 &\n:\n")
	r := newTestRunner(map[string]string{
		"PS1": `<\j>`, "HISTFILE": filepath.Join(t.TempDir(), "history"),
	})
	r.Stdout = &out
	r.JobControl = true
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Escape: '\\',
			Codes:  map[rune]PromptField{'j': FieldJobCount},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Nothing, then the job, and it is still there for the prompt after it.
	if got := errs.String(); !strings.HasPrefix(got, "<0><1>") {
		t.Errorf("prompts = %q, want <0><1> to start", got)
	}
}

// Asked of something that is not a terminal there is no name to give, and a
// prompt that draws it draws nothing rather than the file's own name — which
// is `/dev/stdin` whatever is behind it.
func TestTheTerminalNameOfSomethingThatIsNotOne(t *testing.T) {
	if got := lookupTerminal(nil); got != "" {
		t.Errorf("nil gave %q, want nothing", got)
	}
	if got := lookupTerminal(readerFile(t, "x")); got != "" {
		t.Errorf("a regular file gave %q, want nothing", got)
	}
}

// And drawn through the code that asks for it, so that the answer reaching
// the prompt is what is graded and not only the lookup.
//
// /dev/null is a character device with a device number like any other, which
// is how a terminal's name can be looked up where there is no terminal.
func TestDrawingTheTerminalName(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	s := Shell{
		In:     f,
		counts: &counts{},
		Style: PromptStyle{
			Escape: '\\',
			Codes:  map[rune]PromptField{'l': FieldTerminalName},
		},
	}
	if got := s.render(`<\l>`); got != "<null>" {
		t.Errorf("drew %q, want <null>", got)
	}
	// Asked once: the answer is kept for the session.
	s.counts.tty = "changed"
	if got := s.render(`<\l>`); got != "<changed>" {
		t.Errorf("drew %q, want the kept answer", got)
	}
}

// prompts runs a session that is not a terminal and returns everything it
// prompted with, which is the only way to drive the loop from a test.
func prompts(t *testing.T, name, ps1, typed string) string {
	t.Helper()
	var out, errs strings.Builder
	in := readerFile(t, typed)
	r := newTestRunner(map[string]string{
		name: ps1, "PS2": "",
		// Pointed at a temporary file. The numbering starts where the
		// history left off, so a test that did not say would count this
		// machine's own history and would say something different on every
		// machine — and would be reading the user's file to do it.
		"HISTFILE": filepath.Join(t.TempDir(), "history"),
	})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Escape: '\\',
			Codes: map[rune]PromptField{
				'!': FieldHistoryNumber, '#': FieldCommandNumber, 'j': FieldJobCount,
			},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Only the prompts: a line that will not parse is reported to the same
	// stream, and what is being counted here is the prompting.
	return strings.Join(promptRe.FindAllString(errs.String(), -1), "")
}

var promptRe = regexp.MustCompile(`<[0-9]+:[0-9]+>`)

// What one accepted line does to the two numbers, asked of the one place that
// decides it.
//
// Both loops call this. They used to each hold their own copy, and a mutation
// of the terminal loop's copy survived every test here — because every test
// here drives the other loop. The note on beforeReading warned about exactly
// that, having already happened once.
func TestWhatAnAcceptedLineCounts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		blank, parsed bool
		history, cmd  int
	}{
		{"a command", false, true, 1, 1},
		{"a line that will not parse", false, false, 1, 0},
		{"an empty line", true, true, 0, 0},
		{"an empty line that would not parse either", true, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c counts
			c.accepted(tc.blank, tc.parsed)
			if c.history != tc.history || c.command != tc.cmd {
				t.Errorf("history %d command %d, want %d and %d",
					c.history, c.command, tc.history, tc.cmd)
			}
		})
	}
}

// The numbering starts where the history file left off, in either loop.
//
// Measured: bash given `-i` on a pipe, with three lines in HISTFILE, draws
// `!4 #1` at its first prompt — the history carries across sessions whether
// or not there is an editor to recall it with.
func TestTheNumberingStartsFromTheHistoryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")
	if err := os.WriteFile(path, []byte("old one\nold two\nold three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs strings.Builder
	in := readerFile(t, ": one\n")
	r := newTestRunner(map[string]string{"PS1": `<\!:\#>`, "PS2": "", "HISTFILE": path})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Escape: '\\',
			Codes:  map[rune]PromptField{'!': FieldHistoryNumber, '#': FieldCommandNumber},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := errs.String(); got != "<4:1><5:2>" {
		t.Errorf("prompts = %q, want <4:1> then <5:2>", got)
	}
}

// The terminal's name is found by its device number, so anything in /dev with
// one can be looked up — which is how this is tested without a terminal.
func TestLookingUpADeviceByItsNumber(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if got := lookupTerminal(f); got != "null" {
		t.Errorf("looked up %s and got %q, want null", os.DevNull, got)
	}
}

// A job that has finished is not one the shell is looking after.
//
// It stays in the table until its notice has been given, which is what makes
// this worth stating: counting it would say something was running for exactly
// as long as it takes to say that it is not.
func TestAFinishedJobIsNotCounted(t *testing.T) {
	r := newTestRunner(nil)
	r.JobControl = true
	var out strings.Builder
	r.Stdout = &out
	f := syntax.NewParser("sleep 0 &\n", syntax.Core()).Parse()
	if err := r.RunPart(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	jobs := r.Jobs()
	if len(jobs) == 0 {
		t.Fatal("no job was started")
	}
	// Not asked while it runs: `sleep 0` can be over before the question is,
	// and asserting that it is still going is asserting a race. It failed on
	// Linux for exactly that reason. What this test is about is the other
	// half — a job that has finished — and that is arranged by waiting.
	s := Shell{Runner: r}
	for _, j := range jobs {
		j.Wait()
	}
	// Still in the table, and no longer running.
	if len(r.Jobs()) == 0 {
		t.Fatal("the job left the table before its notice was given")
	}
	if got := s.liveJobs(); got != 0 {
		t.Errorf("live jobs = %d after it finished, want 0", got)
	}
}
