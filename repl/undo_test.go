// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"strings"
	"testing"
)

// `^_`, and `^X^U` for the same thing.
//
// Measured under a pty against bash 5.3.15, bash 3.2.57 and zsh 5.9.2, one
// keystroke at a time, with the resulting line read out of the shell's own
// history file. Where the cursor ended up is measured the same way, by typing
// a `Z` after the undo and seeing where it landed.

// The two shells agree about the case undo exists for: a kill taken back.
func TestUndoingAKill(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{
			// The one that makes a kill safe to press. `^Y` cannot answer
			// this on its own — it puts the text back at the cursor, and
			// after `^U` the cursor is not where the text was.
			"a mistaken ^U comes back whole",
			": one two\x15\x1f\r", ": one two",
		},
		{
			// And with the cursor where it was, so what is typed next lands
			// at the end of the line rather than in the middle of it.
			"and the cursor is back at the end of it",
			": one two\x15\x1fZ\r", ": one twoZ",
		},
		{
			"a word kill comes back",
			": one two\x17\x1f\r", ": one two",
		},
		{
			"^X^U is the same key",
			": one two\x17\x18\x15\r", ": one two",
		},
		{
			// A yank is a change like any other, so the text it put in goes
			// away again rather than being yanked a second time.
			"a yank comes back off",
			": one two\x17\x19\x1fZ\r", ": one Z",
		},
		{
			// Nothing has changed on this line, so there is nothing to take
			// back. Both shells leave the line alone rather than reaching
			// into the line before.
			"nothing to undo does nothing",
			"\x1f\x1f: one\r", ": one",
		},
		{
			// A key that left the line as it found it is not a change, and
			// must not cost an undo that appears to do nothing: `^K` at the
			// end of the line kills nothing, so the `^_` after it has to
			// reach past it to the `^W`.
			"a key that changed nothing does not cost an undo",
			": one two\x17\x0b\x1f\r", ": one two",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, style := range []EditorStyle{zeroAnswers, otherAnswers} {
				if got := typedStyled(t, style, c.keys); got != c.want {
					t.Errorf("%q gave %q, want %q", c.keys, got, c.want)
				}
			}
		})
	}
}

// How much of the line one press takes back, which the two shells answer
// differently.
//
// bash takes back the whole run of typing and zsh takes back one keystroke of
// it. A run and not the line: a keystroke that is not typing ends one, so the
// `d` typed after a `^B` is a change of its own in both.
func TestHowMuchOneUndoTakesBack(t *testing.T) {
	for _, c := range []struct {
		name, keys  string
		bash, zshed string
	}{
		{
			// `: abcdef` is eight characters typed; one `^_` leaves bash with
			// nothing and zsh with seven of them.
			"a run of typing",
			": abcdef\x1fZ\r", "Z", ": abcdeZ",
		},
		{
			"two presses",
			": abcdef\x1f\x1fZ\r", "Z", ": abcdZ",
		},
		{
			// A cursor movement ends the run, so what comes back is the `d`
			// and not the typing in front of it.
			"a movement ends the run",
			": abc\x02d\x1fZ\r", ": abZc", ": abZc",
		},
		{
			// Deletes are their own changes in both: three backspaces and one
			// `^_` puts one character back.
			"a backspace is one change",
			": one two\x7f\x7f\x7f\x1f\r", ": one t", ": one t",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.bash {
				t.Errorf("%q taking back a whole run gave %q, want %q", c.keys, got, c.bash)
			}
			if got := typedStyled(t, otherAnswers, c.keys); got != c.zshed {
				t.Errorf("%q taking back one keystroke gave %q, want %q", c.keys, got, c.zshed)
			}
		})
	}
}

// The `M-.` walk, taken back — the same field, asked of a different key.
//
// Two presses and one `^_`: bash goes back in front of the first press and zsh
// goes back to what the first press put in.
func TestUndoingAnMDotWalk(t *testing.T) {
	behind := []string{": a1 a2", ": b1 b2"}
	const keys = ": X\x1b.\x1b.\x1fZ\r"
	if got, want := typedAfter(t, zeroAnswers, behind, keys), ": XZ"; got != want {
		t.Errorf("taking back the whole walk gave %q, want %q", got, want)
	}
	if got, want := typedAfter(t, otherAnswers, behind, keys), ": Xb2Z"; got != want {
		t.Errorf("taking back one press gave %q, want %q", got, want)
	}
}

// Where the cursor lands, which the two shells also answer differently.
//
// bash puts it after the text the undo has just put back; zsh puts it back
// where it was when the change was made. They agree on a backward kill, where
// those are the same place, and part company on a kill that went forwards.
func TestWhereTheCursorLandsAfterAnUndo(t *testing.T) {
	for _, c := range []struct {
		name, keys  string
		bash, zshed string
	}{
		{
			// `^A`, `^K` empties the line from the start; the undo puts it
			// back and bash is at the end of it, zsh at the start.
			"a kill from the start of the line",
			": one two\x01\x0b\x1fZ\r", ": one twoZ", "Z: one two",
		},
		{
			// The same kill from the middle: bash ends up at the end of the
			// line and zsh where the kill happened.
			"a kill from the middle",
			": one two\x02\x02\x02\x0b\x1fZ\r", ": one twoZ", ": one Ztwo",
		},
		{
			// A forward delete of one character.
			"a delete under the cursor",
			": one two\x01\x04\x1fZ\r", ":Z one two", "Z: one two",
		},
		{
			// And a yank taken back, which removes text rather than putting
			// any in: both leave the cursor where the removal was.
			"a yank taken back",
			": one two\x17\x01\x19\x1fZ\r", "Z: one ", "Z: one ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.bash {
				t.Errorf("%q after the restored text gave %q, want %q", c.keys, got, c.bash)
			}
			if got := typedStyled(t, otherAnswers, c.keys); got != c.zshed {
				t.Errorf("%q back where it was gave %q, want %q", c.keys, got, c.zshed)
			}
		})
	}
}

// The stack belongs to the line and not to the session.
//
// Measured: `^_` at a fresh prompt does nothing in both shells, however much
// was edited on the line before it.
func TestUndoDoesNotReachPastAnAcceptedLine(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context())
	e.in, e.out = strings.NewReader(": one two\x17\r\x1f: after\r"), &out
	for _, want := range []string{": one ", ": after"} {
		got, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("read %q, want %q", got, want)
		}
	}
}

// A key that has begun and not finished is where an editor wedges, and ^C is
// the way out of one.
//
// Measured, all three real shells abandon the line on ^C however far into a
// key sequence they are — their editors leave the terminal's ISIG on, so the
// kernel makes that keystroke a signal. This one takes the terminal fully raw,
// so it has to read the byte and mean the same thing by it.
func TestControlCGetsOutOfAHalfTypedKey(t *testing.T) {
	for _, c := range []struct{ name, keys string }{
		{"after a bare Escape", "\x1b\x03"},
		{"part-way through a control sequence", "\x1b[\x03"},
		{"between the parameters and the final byte", "\x1b[1;5\x03"},
		{"after SS3", "\x1bO\x03"},
		{"inside an old-style mouse report", "\x1b[M \x03"},
		{"after the ^X prefix", "\x18\x03"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{}.newEditor(t.Context())
			e.in, e.out = strings.NewReader(": one two"+c.keys), &out
			line, err := e.readLine(drawPrompt("$ "))
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("read %q, %v; want the line abandoned", line, err)
			}
		})
	}
}

// The `^X` prefix reads the byte after it whatever that byte is.
//
// Measured, `^X` then `q` puts nothing in the line in either shell — the same
// claim escape.go makes about a control sequence, for the other prefix: a key
// this does not act on has to be dropped whole, or the shell types it.
func TestTheControlXPrefixTypesNothing(t *testing.T) {
	for _, keys := range []string{": abc\x18q\r", ": abc\x18\x18\r", ": abc\x18\x1b\r"} {
		if got, want := typedStyled(t, zeroAnswers, keys), ": abc"; got != want {
			t.Errorf("%q gave %q, want %q", keys, got, want)
		}
	}
}

// The rest of a key comes from the same place its first byte did.
//
// A byte handed back by the search mode is the next key's first byte and never
// the second, so this is an invariant rather than a case that arises: reading
// the remainder of a sequence straight from the reader gives the same answers
// today and is a second source of input for the next mode that wants one.
func TestTheRestOfAKeyIsReadFromTheSamePlaceAsItsFirstByte(t *testing.T) {
	e := Shell{}.newEditor(t.Context())
	e.in = strings.NewReader("b")
	e.pushBack('.')
	if got, res := e.readByte(); res != keyContinues || got != '.' {
		t.Errorf("readByte gave %q, %v; want the byte that was pushed back", got, res)
	}
	if got, res := e.readByte(); res != keyContinues || got != 'b' {
		t.Errorf("readByte gave %q, %v; want the reader's own byte", got, res)
	}
}
