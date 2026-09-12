// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// zeroTimeoutRun runs src with in as the shell's input and the given answer
// for what `read -t 0` asks of a stream.
func zeroTimeoutRun(t *testing.T, in *os.File, style ReadZeroTimeoutStyle, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.ReadOptions = "rd:n:N:t:u:"
	sem.ReadZeroTimeout = style
	// Both shells with a *reading* style also leave the name alone when a
	// read runs out of time — a `-t 0` that gives up reaches the same arm a
	// `-t 0.2` that expired does, and measured, ksh93 and zsh answer it the
	// same way there as here. ReadTimeoutKeepsWhatArrived is asked in
	// readoptions_test.go, where both answers are exercised; fixing it here
	// keeps these tests about the style.
	sem.ReadTimeoutKeepsWhatArrived = No
	// And what a real deadline bounds, for the same reason: the control
	// below writes one, and this file is about the zero.
	sem.ReadTimeoutBoundsReadability = No
	r := newTestRunner(t, &Runner{
		Stdin: in, Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// waiting is a stream with a line already in it; empty is a stream nothing has
// been written to and whose writer is still open, so a read on it would wait;
// ended is a stream whose writer has gone, so a read on it returns at once
// with the end of input.
func waiting(t *testing.T, text string) *os.File {
	t.Helper()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close(); _ = wr.Close() })
	if _, err := wr.WriteString(text); err != nil {
		t.Fatal(err)
	}
	return rd
}

func empty(t *testing.T) *os.File {
	t.Helper()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close(); _ = wr.Close() })
	return rd
}

func ended(t *testing.T) *os.File {
	t.Helper()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close() })
	if err := wr.Close(); err != nil {
		t.Fatal(err)
	}
	return rd
}

// TestZeroTimeoutIsThreeQuestionsNotOne: `read -t 0` is not a deadline that
// has already passed, and the three answers differ in what they do to the
// stream as much as in what they report.
//
// The discriminating column is the third: a partial line waiting with the rest
// of it still to come. One style reads nothing there, one gives up and keeps
// nothing, and one commits to finishing the line.
//
// The middle column is measured rather than deduced, and it moved: a `-t 0`
// with nothing waiting runs out of time, and running out of time leaves the
// name alone in both shells with a reading style — `v=[old]`, not `v=[]`.
// The last column is an end of input rather than a timeout, and there every
// shell assigns, so a variable that held something is emptied.
func TestZeroTimeoutIsThreeQuestionsNotOne(t *testing.T) {
	const src = `v=old; read -t 0 v; echo "st=$? v=[$v]"`
	for _, c := range []struct {
		name                               string
		style                              ReadZeroTimeoutStyle
		lineWaiting, nothingWaiting, atEnd string
	}{
		{
			"a poll reads nothing and reports what it found",
			ReadZeroTimeoutPolls,
			"st=0 v=[old]", "st=1 v=[old]", "st=0 v=[old]",
		},
		{
			"taking what is waiting reads it",
			ReadZeroTimeoutTakesWhatIsWaiting,
			"st=0 v=[hello]", "st=1 v=[old]", "st=1 v=[]",
		},
		{
			"finishing what it started reads it too",
			ReadZeroTimeoutFinishesWhatItStarted,
			"st=0 v=[hello]", "st=1 v=[old]", "st=1 v=[]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, tc := range []struct {
				what string
				in   *os.File
				want string
			}{
				{"a line waiting", waiting(t, "hello\nworld\n"), c.lineWaiting},
				{"nothing waiting", empty(t), c.nothingWaiting},
				{"the stream ended", ended(t), c.atEnd},
			} {
				if got, _ := zeroTimeoutRun(t, tc.in, c.style, src); strings.TrimSpace(got) != tc.want {
					t.Errorf("%s: got %q, want %q", tc.what, strings.TrimSpace(got), tc.want)
				}
			}
		})
	}
}

// TestAPollLeavesTheInputForTheNextRead is the half a status cannot show, and
// the reason a poll is not merely a read with a short deadline: the byte it
// reported is still there. A poll that consumed it would leave a script
// watching for input eating its own input a character at a time.
func TestAPollLeavesTheInputForTheNextRead(t *testing.T) {
	const src = `read -t 0 v; echo "st=$? v=[$v]"; read w; echo "w=[$w]"`
	got, _ := zeroTimeoutRun(t, waiting(t, "hello\nworld\n"), ReadZeroTimeoutPolls, src)
	if want := "st=0 v=[]\nw=[hello]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// The styles that read take the line, so the next read gets the one
	// after it. Same input, same script, a different answer to one question.
	got, _ = zeroTimeoutRun(t, waiting(t, "hello\nworld\n"), ReadZeroTimeoutTakesWhatIsWaiting, src)
	if want := "st=0 v=[hello]\nw=[world]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestAPartialLineSeparatesTheTwoStylesThatRead: with `ab` waiting and the
// rest of the line still to come, one style gives up — leaving the variable
// exactly as it was, because giving up is a timeout and not an end of input —
// while the other commits to the line it began.
//
// The two cases have separate streams rather than one with a timer on it,
// because a timer is what would make this a race: the style that gives up
// must meet a stream where the rest never comes, and the style that commits
// must meet one where it does. Neither waits on a clock — the second blocks
// until the writer speaks, which is the behavior under test.
func TestAPartialLineSeparatesTheTwoStylesThatRead(t *testing.T) {
	for _, c := range []struct {
		name        string
		style       ReadZeroTimeoutStyle
		restArrives bool
		want        string
	}{
		{"giving up touches no name", ReadZeroTimeoutTakesWhatIsWaiting, false, "st=1 v=[old]"},
		{"committing finishes the line", ReadZeroTimeoutFinishesWhatItStarted, true, "st=0 v=[abc]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rd, wr, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rd.Close(); _ = wr.Close() })
			if _, err := wr.WriteString("ab"); err != nil {
				t.Fatal(err)
			}
			if c.restArrives {
				if _, err := wr.WriteString("c\n"); err != nil {
					t.Fatal(err)
				}
			}
			// A style that wrongly commits to a line whose rest never
			// arrives blocks forever, so this waits on a bound and fails
			// rather than hanging the package. The bound is generous: no
			// correct answer here waits at all.
			done := make(chan string, 1)
			go func() {
				got, _ := zeroTimeoutRun(t, rd, c.style, `v=old; read -t 0 v; echo "st=$? v=[$v]"`)
				done <- strings.TrimSpace(got)
			}()
			select {
			case got := <-done:
				if got != c.want {
					t.Errorf("got %q, want %q", got, c.want)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("still reading: the style waited for input it should not have")
			}
		})
	}
}

// TestAnUnansweredZeroTimeoutIsRefused: no dialect chosen means no answer, and
// the substrate says so rather than picking one of the three.
func TestAnUnansweredZeroTimeoutIsRefused(t *testing.T) {
	out, st := zeroTimeoutRun(t, waiting(t, "hello\n"), ReadZeroTimeoutUnspecified, `read -t 0 v`)
	if st != 2 {
		t.Errorf("status = %d, want the refusal's 2", st)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal naming the disagreement", out)
	}
	if !strings.Contains(out, "read -t 0") {
		t.Errorf("said %q, want the question named", out)
	}
}

// TestAnOrdinaryTimeoutIsStillADeadline is the control: only a zero asks the
// question above, and every other `-t` is a deadline the read races against.
func TestAnOrdinaryTimeoutIsStillADeadline(t *testing.T) {
	// Unspecified would refuse a zero; a real timeout must not reach the
	// question at all, so this passing is what says the ask is confined.
	out, _ := zeroTimeoutRun(t, waiting(t, "hello\n"), ReadZeroTimeoutUnspecified,
		`read -t 5 v; echo "st=$? v=[$v]"`)
	if want := "st=0 v=[hello]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
