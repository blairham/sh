// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/syntax"
)

// recordingProvider keeps what it was told and answers with fixed text.
type recordingProvider struct {
	told []PromptInfo
	text string
}

func (p *recordingProvider) Prompt(info PromptInfo) string {
	p.told = append(p.told, info)
	return p.text
}

// A provider's text is drawn before the prompt parameter's, in the order the
// providers were given, and the whole thing is measured as one.
//
// The width is the assertion that matters as much as the text: the editor
// places the cursor by it, so a prompt whose text grew and whose count did not
// draws every line one column to the left of where it belongs.
func TestProvidersAreDrawnBeforeThePromptParameter(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		PromptProviders: []PromptProvider{
			PromptProviderFunc(func(PromptInfo) string { return "one" }),
			PromptProviderFunc(func(PromptInfo) string { return "two" }),
		},
	}
	var pending strings.Builder
	got := s.beforeReading(&pending)
	if got.text != "onetwo$ " {
		t.Errorf("the prompt is %q, want %q", got.text, "onetwo$ ")
	}
	if got.cells != len("onetwo$ ") {
		t.Errorf("the prompt is %d cells, want %d", got.cells, len("onetwo$ "))
	}
}

// The same at the continuation prompt, and the provider is told which it is.
func TestProvidersAreDrawnAtTheContinuationPromptAndToldSo(t *testing.T) {
	p := &recordingProvider{text: "seg"}
	s := Shell{
		Runner:          newTestRunner(map[string]string{"PS2": "> "}),
		PromptProviders: []PromptProvider{p},
	}
	var pending strings.Builder
	pending.WriteString("for i in a b\n")
	got := s.beforeReading(&pending)
	if got.text != "seg> " {
		t.Errorf("the continuation prompt is %q, want %q", got.text, "seg> ")
	}
	if len(p.told) != 1 || !p.told[0].Continued {
		t.Errorf("the provider was told %+v, want one call with Continued set", p.told)
	}
}

// Nothing is added when nothing was contributed, so a session without
// providers draws exactly the prompt it drew before this seam existed.
func TestASessionWithoutProvidersDrawsOnlyTheParameter(t *testing.T) {
	s := Shell{Runner: newTestRunner(map[string]string{"PS1": "$ "})}
	var pending strings.Builder
	if got := s.beforeReading(&pending); got.text != "$ " {
		t.Errorf("the prompt is %q, want %q", got.text, "$ ")
	}
}

// A nil in the list is skipped rather than a crash.
func TestANilProviderInTheListIsSkipped(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		PromptProviders: []PromptProvider{
			nil, PromptProviderFunc(func(PromptInfo) string { return "here" }),
		},
	}
	var pending strings.Builder
	if got := s.beforeReading(&pending); got.text != "here$ " {
		t.Errorf("the prompt is %q, want %q", got.text, "here$ ")
	}
}

// A provider marking part of its text non-printing has that part drawn and not
// counted, which is the whole of why NonPrinting exists.
//
// Both halves asserted: the escape bytes are on the wire, and the width is of
// the letters alone. A prompt that colored itself without saying so would draw
// the same and count five columns too many.
func TestNonPrintingTextIsDrawnAndNotCounted(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		PromptProviders: []PromptProvider{
			PromptProviderFunc(func(PromptInfo) string {
				return NonPrinting("\x1b]0;a title\a") + "seg" + NonPrinting("\x1b[0m")
			}),
		},
	}
	var pending strings.Builder
	got := s.beforeReading(&pending)
	if want := "\x1b]0;a title\aseg\x1b[0m$ "; got.text != want {
		t.Errorf("the prompt bytes are %q, want %q", got.text, want)
	}
	if want := len("seg$ "); got.cells != want {
		t.Errorf("the prompt is %d cells, want %d", got.cells, want)
	}
}

// NonPrinting leaves the empty string alone, because a provider building its
// text conditionally hands it one often and a marker pair around nothing is
// two bytes to strip for no reason.
func TestNonPrintingLeavesTheEmptyStringAlone(t *testing.T) {
	if got := NonPrinting(""); got != "" {
		t.Errorf("NonPrinting(%q) = %q, want %q", "", got, "")
	}
	if got := NonPrinting("x"); got != "\x01x\x02" {
		t.Errorf("NonPrinting(%q) = %q, want %q", "x", got, "\x01x\x02")
	}
}

// A provider that panics costs its own segment and nothing else: the prompt
// still arrives, the providers after it still contribute, and the session goes
// on.
//
// The process is the session — a shell open for hours, with its variables, its
// jobs and its directory — and a decoration that could end it would be a worse
// bargain than the decoration is worth. This is the same guard a typed line
// already runs behind.
func TestAProviderThatPanicsCostsOnlyItsOwnSegment(t *testing.T) {
	errs := &syncBuffer{}
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		Err:    errs,
		Name:   "sh",
		PromptProviders: []PromptProvider{
			PromptProviderFunc(func(PromptInfo) string { return "before" }),
			PromptProviderFunc(func(PromptInfo) string { panic("a bug in a segment") }),
			PromptProviderFunc(func(PromptInfo) string { return "after" }),
		},
	}
	var pending strings.Builder
	got := s.beforeReading(&pending)
	if got.text != "beforeafter$ " {
		t.Errorf("the prompt is %q, want %q", got.text, "beforeafter$ ")
	}
	if !strings.Contains(errs.String(), "a bug in a segment") {
		t.Errorf("the panic was not reported: %q", errs.String())
	}
}

// What a provider is told about the last command, asserted as one value.
//
// The facts are the block store's, taken by closeBlock, so this drives the
// same call the loops drive rather than setting the fields by hand.
func TestAProviderIsToldWhatTheLastCommandWas(t *testing.T) {
	clock := &steppingClock{}
	r := newTestRunner(nil)
	r.Dir = "/where/it/ran"
	p := &recordingProvider{}
	s := Shell{
		Runner: r, PromptProviders: []PromptProvider{p},
		Clock: clock.now, counts: &counts{},
	}

	// Nothing has run yet.
	var pending strings.Builder
	s.beforeReading(&pending)

	b := s.beginBlock("echo hi")
	clock.step = 1500 * time.Millisecond
	s.closeBlock(t.Context(), nil, nil, b)
	s.beforeReading(&pending)

	// A blank line is not a command and must not replace it, which is the
	// rule the block store already follows: pressing return should not clear
	// the duration of the command being looked at.
	s.closeBlock(t.Context(), nil, nil, s.beginBlock("   "))
	s.beforeReading(&pending)

	want := []PromptInfo{
		{Dir: "/where/it/ran"},
		{Dir: "/where/it/ran", Command: "echo hi", Duration: 1500 * time.Millisecond},
		{Dir: "/where/it/ran", Command: "echo hi", Duration: 1500 * time.Millisecond},
	}
	if !reflect.DeepEqual(p.told, want) {
		t.Errorf("the provider was told\n got %+v\nwant %+v", p.told, want)
	}
}

// steppingClock moves forward by whatever the test last asked for, so a
// duration is a fact of the test rather than of how fast the machine is.
type steppingClock struct {
	at   time.Time
	step time.Duration
}

func (c *steppingClock) now() time.Time {
	c.at = c.at.Add(c.step)
	return c.at
}

// The status a provider is told is the shell's `$?`.
func TestAProviderIsToldTheExitStatus(t *testing.T) {
	r := newTestRunner(nil)
	r.SetExitStatus(3)
	p := &recordingProvider{}
	s := Shell{Runner: r, PromptProviders: []PromptProvider{p}}
	var pending strings.Builder
	s.beforeReading(&pending)
	if len(p.told) != 1 || p.told[0].Status != 3 {
		t.Errorf("the provider was told %+v, want a status of 3", p.told)
	}
}

// The job count a provider is told is what the shell is still looking after,
// and a job that has finished is not one.
//
// Arranged by waiting rather than by asking while it runs: `sleep 0` can be
// over before the question is, and asserting that it is still going is
// asserting a race — which failed on Linux once already, for the sibling
// assertion in promptcounts_test.go.
func TestAProviderIsToldHowManyJobsTheShellIsLookingAfter(t *testing.T) {
	r := newTestRunner(nil)
	r.JobControl = true
	r.Stdout = &syncBuffer{}
	f := syntax.NewParser("sleep 0 &\n", syntax.Core()).Parse()
	if err := r.RunPart(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	jobs := r.Jobs()
	if len(jobs) == 0 {
		t.Fatal("no job was started")
	}

	p := &recordingProvider{}
	s := Shell{Runner: r, PromptProviders: []PromptProvider{p}}
	var pending strings.Builder
	s.beforeReading(&pending)

	for _, j := range jobs {
		j.Wait()
	}
	s.beforeReading(&pending)

	if len(p.told) != 2 {
		t.Fatalf("the provider was told %d times, want 2", len(p.told))
	}
	if p.told[0].Jobs != 1 {
		t.Errorf("with a job running the provider was told %d jobs, want 1", p.told[0].Jobs)
	}
	if p.told[1].Jobs != 0 {
		t.Errorf("after it finished the provider was told %d jobs, want 0", p.told[1].Jobs)
	}
}

// A Shell with no Runner still draws a prompt and reports a status of zero
// rather than panicking.
func TestAShellWithoutARunnerReportsNoStatus(t *testing.T) {
	if got := (Shell{}).exitStatus(); got != 0 {
		t.Errorf("the status is %d, want 0", got)
	}
}

// A provider's bytes reach a real terminal, escape sequences included, and the
// markers that said which of them were escape sequences do not.
//
// This is the assertion a reader-driven test cannot make and an end-to-end
// test that strips ANSI would pass without: what is checked is the exact byte
// sequence on the wire. A marker reaching the screen is a control character
// nobody asked for, and an escape sequence not reaching it is a prompt that is
// silently plain.
func TestAProvidersBytesReachTheTerminalAndItsMarkersDoNot(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.PromptProviders = []PromptProvider{
			PromptProviderFunc(func(info PromptInfo) string {
				if info.Continued {
					return ""
				}
				return NonPrinting("\x1b[31m") + "P" + NonPrinting("\x1b[0m")
			}),
		}
	})
	// typeLine waits for the first prompt, so by the time it returns the
	// whole of it has been drawn.
	s.typeLine("echo ok\n")
	waitFor(t, s.ran, "ok", "the command's output")
	s.end()

	// The first thing on the screen, exactly: the provider's colored mark and
	// then the parameter. Anchored at the start of the stream rather than
	// searched for, because a search would be answered by a prompt that came
	// out right on the fourth redraw.
	const wantFirst = "\x1b[31mP\x1b[0m[1]"
	if got := s.screen.String(); !strings.HasPrefix(got, wantFirst) {
		t.Errorf("the first prompt drawn was\n got %q\nwant a prefix of %q", firstBytes(got), wantFirst)
	}
	if i := strings.IndexAny(s.screen.String(), markStart+markEnd); i >= 0 {
		t.Errorf("a non-printing marker reached the terminal at byte %d: %q", i, s.screen.String())
	}
}

// firstBytes is enough of the screen to see the first prompt in a failure.
func firstBytes(s string) string {
	if len(s) > 60 {
		return s[:60]
	}
	return s
}
