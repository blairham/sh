// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"testing"
	"time"

	"github.com/blairham/sh/interp"
)

// How long the editor waits for the rest of a multi-character key sequence,
// in the dialect that keeps the length in a parameter.
//
// Nothing waited at all before this: `escapeIsTheModeSwitch` asked whether a
// byte was *there* and took no time over the answer, on the argument that a
// terminal writes an escape sequence in one write and that there was no honest
// number to invent. The parameter is the number, which is what #4995 is.
//
// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 —
// `zsh -f -i`, `bindkey -v`, `echo hello` typed, an Escape, a gap, then `[D`,
// the tail of a left arrow. Waited through, the three bytes are the arrow and
// the line runs as `echo hello`; given up on, the Escape was the mode switch
// and `D` kills to the end of the line, so it runs as `echo hell`. Every row
// below was re-run against this shell's own binary through the same driver and
// every one agrees.
//
//	KEYTIMEOUT  gap     runs as
//	40          0.05s   echo hello
//	40          1.00s   echo hell
//	200         1.00s   echo hello
//	1           0.05s   echo hell
//	200         3.00s   echo hell
//	1000        4.00s   echo hello
//	0           4.00s   echo hello
//	-5          4.00s   echo hello
//	soon        0.05s   echo hello
//
// **Rows two and three hold the gap still and move only the parameter, and so
// do rows one and four.** They are the pair that says the *number* is read: a
// grid that varied only the gap would agree, on every row, with a timer
// hard-coded to the reference's own four tenths, and would have proved nothing
// about the parameter at all.
//
// **And zero does not mean "no wait".** It waits indefinitely, and so does
// every negative and so does a value that is not a number — the reference
// declares the name `integer`, so `KEYTIMEOUT=soon` stores 0. The obvious
// reading of zero is the opposite of the measured one, which is why those
// three rows are here and why the four-second gap is in them: a merely long
// wait and an unbounded one are told apart by the gap being longer than any
// number the parameter held.
func TestHowLongTheEditorWaitsForTheRestOfAKeySequence(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		set         bool
		want        time.Duration
		forever     bool
	}{
		// Hundredths of a second, which is the unit the grid measured: 200
		// waits through a one-second gap and not through a three-second one.
		{"the default forty is four tenths", "40", true, 400 * time.Millisecond, false},
		{"two hundred is two seconds", "200", true, 2 * time.Second, false},
		{"one is a hundredth", "1", true, 10 * time.Millisecond, false},
		{"a thousand is ten seconds", "1000", true, 10 * time.Second, false},
		// The three rows whose obvious reading is the wrong one.
		{"zero waits indefinitely", "0", true, 0, true},
		{"and so does a negative", "-5", true, 0, true},
		{"and so does a value that is not a number", "soon", true, 0, true},
		{"blanks around a number are ignored", " 40 ", true, 400 * time.Millisecond, false},
		// An unset parameter is not a parameter holding zero. The dialect
		// named it and this session has not got it, so there is no
		// instruction to follow and the editor waits no time — which is what
		// it did before there was a wait.
		{"an unset parameter waits no time at all", "", false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Shell{Editor: EditorStyle{KeySequenceWaitParameter: "KEYTIMEOUT"}}
			s.Runner = &interp.Runner{}
			if tc.set {
				s.Runner.Vars = map[string]string{"KEYTIMEOUT": tc.value}
			}
			read := s.keySequenceWait()
			if read == nil {
				t.Fatal("the dialect names KEYTIMEOUT and no reader was made")
			}
			got, forever := read()
			if got != tc.want || forever != tc.forever {
				t.Errorf("KEYTIMEOUT=%q -> %v,forever=%v; want %v,forever=%v",
					tc.value, got, forever, tc.want, tc.forever)
			}
		})
	}
}

// A dialect that names no parameter waits no time at all.
//
// **This is the zero value and it is the point of spelling the field as a
// name.** Three of the four dialects have no line editor wait to speak of and
// bash's is a readline variable rather than a shell parameter, so they name
// nothing here — and naming nothing has to leave them exactly where they were
// before this existed, rather than handing them a default somebody would then
// have to measure. A field holding a duration could not have done that.
func TestADialectThatNamesNoKeySequenceWaitWaitsNoTime(t *testing.T) {
	s := &Shell{}
	s.Runner = &interp.Runner{Vars: map[string]string{"KEYTIMEOUT": "200"}}
	if read := s.keySequenceWait(); read != nil {
		t.Errorf("a dialect naming no parameter was given a reader anyway")
	}
	// And the editor's own view of a nil reader, which is the value the
	// Escape question actually consults.
	e := &editor{}
	if d, forever := e.keySequenceWait(); d != 0 || forever {
		t.Errorf("a nil reader = %v,forever=%v; want 0,forever=false", d, forever)
	}
}

// A session with no Runner has nothing to read the parameter out of.
func TestTheKeySequenceWaitNeedsARunnerToReadFrom(t *testing.T) {
	s := &Shell{Editor: EditorStyle{KeySequenceWaitParameter: "KEYTIMEOUT"}}
	if read := s.keySequenceWait(); read != nil {
		t.Errorf("a session with no Runner was given a reader anyway")
	}
}

// The wait itself, over a real pipe: the question `escapeIsTheModeSwitch`
// asks, with a descriptor that does or does not get a byte in time.
//
// **This is the half the parameter table above cannot reach.** That table
// grades the reading of `$KEYTIMEOUT` and would go on passing with the number
// read and thrown away; these rows grade the wiring, by putting a descriptor
// under the editor and watching whether the answer changes with when the byte
// arrives. The pty differential in the commit message is the same question
// asked of a whole shell; this is it asked of the one function.
//
// The bounds are deliberately loose. What is being pinned is *that* the wait
// happens and roughly how long, not the scheduler's accuracy: a row that
// failed because a runner was busy for forty milliseconds would be a worse
// test than no row at all.
func TestTheEscapeQuestionWaitsForTheRestOfTheSequence(t *testing.T) {
	// escapeIsTheModeSwitch is true when the Escape stands alone, and false
	// when something follows it inside the wait.
	newPipe := func(t *testing.T) (*os.File, *os.File) {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Close(); w.Close() })
		return r, w
	}
	editorOver := func(r *os.File, wait func() (time.Duration, bool)) *editor {
		return &editor{inFd: func() int { return int(r.Fd()) }, keyWait: wait}
	}

	t.Run("nothing follows within the wait, so the Escape is the mode switch", func(t *testing.T) {
		r, _ := newPipe(t)
		e := editorOver(r, func() (time.Duration, bool) { return 150 * time.Millisecond, false })
		start := time.Now()
		if !e.escapeIsTheModeSwitch() {
			t.Errorf("an Escape with nothing behind it did not switch mode")
		}
		if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
			t.Errorf("the question took %v, which is less than the wait it was given", elapsed)
		}
	})

	t.Run("a byte arrives inside the wait, so the Escape begins a sequence", func(t *testing.T) {
		r, w := newPipe(t)
		e := editorOver(r, func() (time.Duration, bool) { return 3 * time.Second, false })
		go func() {
			time.Sleep(50 * time.Millisecond)
			w.Write([]byte("["))
		}()
		start := time.Now()
		if e.escapeIsTheModeSwitch() {
			t.Errorf("an Escape with a byte behind it switched mode anyway")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("the question waited %v after the byte had arrived", elapsed)
		}
	})

	t.Run("a dialect that names no parameter waits no time at all", func(t *testing.T) {
		// The zero value, and the row that says nothing inherits a wait: the
		// byte below arrives long after this has already answered.
		r, w := newPipe(t)
		e := editorOver(r, nil)
		go func() {
			time.Sleep(200 * time.Millisecond)
			w.Write([]byte("["))
		}()
		start := time.Now()
		if !e.escapeIsTheModeSwitch() {
			t.Errorf("an editor with no wait waited for the byte anyway")
		}
		if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
			t.Errorf("an editor with no wait took %v", elapsed)
		}
	})

	t.Run("zero waits indefinitely, so a late byte is still the sequence", func(t *testing.T) {
		// `KEYTIMEOUT=0` — measured to wait past a four-second gap. A
		// quarter of a second is enough to tell it from every bounded row
		// above without making the suite wait for one.
		r, w := newPipe(t)
		e := editorOver(r, func() (time.Duration, bool) { return 0, true })
		go func() {
			time.Sleep(250 * time.Millisecond)
			w.Write([]byte("["))
		}()
		if e.escapeIsTheModeSwitch() {
			t.Errorf("an unbounded wait gave up before the byte arrived")
		}
	})

	t.Run("a descriptor the question cannot be put about switches mode", func(t *testing.T) {
		// A descriptor past what a descriptor set can name. The honest
		// fallback is the one ReadableNow takes for the same case — go
		// ahead — which here means treating the Escape as the mode switch,
		// because a mode switch is recoverable by pressing `i` and a
		// swallowed Escape is not.
		e := &editor{
			inFd:    func() int { return 1 << 20 },
			keyWait: func() (time.Duration, bool) { return 3 * time.Second, false },
		}
		start := time.Now()
		if !e.escapeIsTheModeSwitch() {
			t.Errorf("a descriptor nothing could be asked about began a sequence")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("a descriptor nothing could be asked about was waited on for %v", elapsed)
		}
	})

	t.Run("a byte already in hand is never waited on", func(t *testing.T) {
		// inputPending answers first: bytes taken off the terminal sit where
		// the kernel cannot see them, which is the split this function has
		// always had.
		r, _ := newPipe(t)
		e := editorOver(r, func() (time.Duration, bool) { return 3 * time.Second, false })
		e.held[0] = byte(0x5b)
		e.heldLen = 1
		start := time.Now()
		if e.escapeIsTheModeSwitch() {
			t.Errorf("an Escape with a byte already in hand switched mode")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("a byte already in hand was waited on for %v", elapsed)
		}
	})
}
