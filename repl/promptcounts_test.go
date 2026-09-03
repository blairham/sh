// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"regexp"
	"strings"
	"testing"
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
	if got := s.escapes(`<!\! #\#>`); got != "<!4 #1>" {
		t.Errorf("drew %q, want <!4 #1>", got)
	}
	s.counts.history++
	s.counts.command++
	if got := s.escapes(`<!\! #\#>`); got != "<!5 #2>" {
		t.Errorf("drew %q, want <!5 #2>", got)
	}
	// Without either loop having run there is still a number to draw.
	bare := Shell{Style: s.Style}
	if got := bare.escapes(`<!\! #\#>`); got != "<!1 #1>" {
		t.Errorf("drew %q, want <!1 #1>", got)
	}
}

// The two loops keep the numbers, and they keep them differently: only one of
// them has a history file behind it.
func TestBothLoopsCountTheLines(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "n=1\nn=2\n")
	r := newTestRunner(map[string]string{"PS1": `<\!:\#>`})
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
	r := newTestRunner(map[string]string{"PS1": `<\j>`})
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
	if got := terminalName(nil); got != "" {
		t.Errorf("nil gave %q, want nothing", got)
	}
	if got := lookupTerminal(readerFile(t, "x")); got != "" {
		t.Errorf("a regular file gave %q, want nothing", got)
	}
}

// prompts runs a session that is not a terminal and returns everything it
// prompted with, which is the only way to drive the loop from a test.
func prompts(t *testing.T, name, ps1, typed string) string {
	t.Helper()
	var out, errs strings.Builder
	in := readerFile(t, typed)
	r := newTestRunner(map[string]string{name: ps1, "PS2": ""})
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
