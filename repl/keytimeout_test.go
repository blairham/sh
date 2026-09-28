// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
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
