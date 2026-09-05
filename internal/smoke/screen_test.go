// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"io"
	"strings"
	"testing"
	"time"
)

// The fix from #635, stated where it can be checked without a race.
//
// A session draws the same prompt after every line, so "wait for the prompt"
// searched over everything drawn is answered by the *first* one for the rest of
// the run. A suite that then types is typing at a shell that may still be
// running the previous command, with the terminal in its own line discipline
// and nothing reading — and the kernel drops what is queued but unread when the
// discipline changes back. That is how a keystroke goes missing, and it is why
// this is about a cursor rather than about a longer deadline.
func TestAWaitIsNotSatisfiedByAMarkItAlreadySaw(t *testing.T) {
	drawn := &Screen{}
	drawn.Draw(promptAnchor)

	if !drawn.Seek(promptAnchor) {
		t.Fatal("the first prompt was not found at all")
	}
	if drawn.Seek(promptAnchor) {
		t.Error("the same prompt answered twice, so a later wait would not wait")
	}

	// The line is echoed, it runs, and the shell prompts again.
	drawn.Draw("echo pipe-42\r\nPIPE-42\r\n" + promptAnchor)
	if !drawn.Seek("PIPE-42") {
		t.Error("the output of the line was not found")
	}
	if !drawn.Seek(promptAnchor) {
		t.Error("the second prompt was not found")
	}
	if drawn.Seek(promptAnchor) {
		t.Error("a third prompt was found where only two were drawn")
	}

	// And nothing is forgotten: the whole screen is what a failure prints,
	// which a buffer cleared between waits could not report.
	if got := drawn.Text(); !strings.Contains(got, "echo pipe-42") {
		t.Errorf("the screen is %q, want everything drawn kept for the diagnostic", got)
	}
}

// The cursor must not make a wait miss text that had not been drawn when the
// wait began, which is every real use of it.
func TestAWaitReadsWhatArrivesAfterIt(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	drawn := Watch(pr)

	go func() {
		for _, part := range []string{promptAnchor, "./ticker\r\n", "tick-42\r\n", promptAnchor} {
			time.Sleep(time.Millisecond)
			if _, err := io.WriteString(pw, part); err != nil {
				return
			}
		}
	}()

	if err := drawn.Await(promptAnchor, budget); err != nil {
		t.Fatalf("the first prompt: %v", err)
	}
	if err := drawn.Await("tick-42", budget); err != nil {
		t.Fatalf("the output: %v", err)
	}
	if err := drawn.Await(promptAnchor, budget); err != nil {
		t.Fatalf("the prompt after it: %v", err)
	}
}

// The cursor is what a wait is built on, so the wait itself is checked and not
// only the search under it.
//
// The second prompt is arranged to arrive late, and the assertion is on the
// lower bound: a wait satisfied by the prompt it had already seen returns
// before that prompt exists. Load can only make the elapsed time longer, which
// is the direction that keeps this from being a flake of its own.
func TestAWaitBlocksUntilTheMarkIsDrawnAgain(t *testing.T) {
	const late = 50 * time.Millisecond

	drawn := &Screen{}
	drawn.Draw(promptAnchor)
	if err := drawn.Await(promptAnchor, budget); err != nil {
		t.Fatalf("the first prompt: %v", err)
	}

	go func() {
		time.Sleep(late)
		drawn.Draw("tick-42\r\n" + promptAnchor)
	}()

	start := time.Now()
	if err := drawn.Await(promptAnchor, budget); err != nil {
		t.Fatalf("the second prompt: %v", err)
	}
	if waited := time.Since(start); waited < late {
		t.Errorf("the wait returned after %v, before the second prompt was drawn at %v", waited, late)
	}
}

// A wait that gives up says what it was waiting for and what was on screen
// instead, because a report of ten failures with no evidence is a report
// nobody can act on.
func TestAWaitThatGivesUpSaysWhatItSaw(t *testing.T) {
	drawn := &Screen{}
	drawn.Draw("command not found\r\n")
	err := drawn.Await("never-drawn", time.Millisecond)
	if err == nil {
		t.Fatal("a wait for something never drawn returned no error")
	}
	for _, want := range []string{"never-drawn", "command not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message is %q, want it to carry %q", err, want)
		}
	}
}

// Several marks, and which one arrived.
//
// The session cannot know in advance whether the prompt it set took effect, so
// the first wait of a run has to accept either and report which — that answer
// is itself one of the rows.
func TestAWaitOnSeveralMarksAnswersWhichArrived(t *testing.T) {
	drawn := &Screen{}
	drawn.Draw("bash-5.3$ ")
	got, err := drawn.AwaitAny(budget, promptAnchor, "$ ")
	if err != nil {
		t.Fatalf("neither prompt was found: %v", err)
	}
	if got != "$ " {
		t.Errorf("answered %q, want the default prompt that was actually drawn", got)
	}
}

// Silence is an assertion too, and it is the only one a stopped job can make.
func TestQuietIsFalseAsSoonAsTheMarkIsDrawn(t *testing.T) {
	drawn := &Screen{}
	if !drawn.Quiet("tick-42", 20*time.Millisecond) {
		t.Error("an empty screen was not quiet")
	}
	drawn.Draw("tick-42\r\n")
	if drawn.Quiet("tick-42", 20*time.Millisecond) {
		t.Error("a screen with the mark on it was reported quiet")
	}
	// And the cursor moved past it, so the next silence is a real one.
	if !drawn.Quiet("tick-42", 20*time.Millisecond) {
		t.Error("the same mark answered twice, so a job that stopped would look alive forever")
	}
}

// What a failure prints is readable, and what an assertion reads is not
// touched.
//
// The two are deliberately different. Grading stripped text would pass for a
// shell whose every line was a control character; printing raw text produces a
// report nobody reads. So the cleaning is on the diagnostic only, and this is
// where that is stated.
func TestReadableCleansTheDiagnosticAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a redraw", "\r\x1b[Kecho x", "echo x"},
		{
			// The editor draws the whole line again after every keystroke,
			// so a diagnostic that did not replay the returns is forty
			// times as long and says nothing the last redraw does not.
			name: "a line typed one key at a time",
			in:   "$ s\r\x1b[K$ sm\r\x1b[K$ smokefunc\r\n",
			want: "$ smokefunc",
		},
		{
			// And the overwrite is an overwrite, not a truncation: a shorter
			// redraw with no erase leaves the tail of the longer one, which
			// is what the terminal itself would show.
			name: "an overwrite without an erase",
			in:   "abcdef\rxy",
			want: "xycdef",
		},
		{"a clear", "\x1b[H\x1b[2Jhello", "hello"},
		{"a two-byte sequence", "\x1bMhello", "hello"},
		{"a newline is kept as a mark", "one\ntwo", "one⏎two"},
		{"a control byte is spelled out", "a\x1ab", `a\x1ab`},
		{"the suspend character", "\x03", `\x03`},
		{"ordinary text is untouched", "tick-42", "tick-42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Readable(tc.in); got != tc.want {
				t.Errorf("Readable(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
