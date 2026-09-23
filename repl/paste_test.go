// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// Bracketed paste, asserted on the bytes.
//
// **The bytes and not the screen, which is the opposite of what most of this
// package's tests do and is deliberate.** Everything here is a sequence the
// terminal acts on and draws nothing for: `\e[?2004h` is a request, and
// `\e[7m` is an attribute. A screen model shows neither, and an assertion on
// the text would pass against a shell that never wrote any of them — which is
// exactly the state #2775 was filed about. The sequences are also not ours to
// choose: they were measured off real bash and real zsh, and a test that named
// anything else would be pinning an invention.
//
// pasting is the style a dialect that brackets answers with, spelled here
// rather than imported: a test in this package names the answer, never the
// shell that gives it. dialect/bash and dialect/zsh pin which shells do.
var pasting = EditorStyle{
	BracketedPaste:     true,
	PastedTextStyle:    "\x1b[7m",
	PastedTextStyleEnd: "\x1b[27m",
}

// chunks hands over one write per chunk, which is how a paste arrives: a
// terminal delivers the whole of it in one read, markers and all.
//
// before runs just before each chunk after the first, so a test can say what
// the screen had to be showing *by then* — which is the whole of the second
// half of #2775. The text landed in the line and nothing was drawn until the
// next keystroke forced a redraw, and a test that looked only at the end of
// the session could not tell that apart from a shell that drew it on time.
type chunks struct {
	s      []string
	i      int
	before func()
}

func (c *chunks) Read(p []byte) (int, error) {
	if c.i >= len(c.s) {
		return 0, io.EOF
	}
	if c.i > 0 && c.before != nil {
		c.before()
	}
	n := copy(p, c.s[c.i])
	if n < len(c.s[c.i]) {
		c.s[c.i] = c.s[c.i][n:]
		return n, nil
	}
	c.i++
	return n, nil
}

// paste is one bracketed paste, markers and all, as a terminal delivers it.
func paste(text string) string { return "\x1b[200~" + text + pasteEndMarker }

// The terminal is asked to bracket pastes for the length of the read, and told
// to stop for the length of the command.
//
// Measured 2026-09-14 against bash 5.3.3 and zsh 5.9.2 through a
// pseudo-terminal: both write `\e[?2004h` around the prompt and `\e[?2004l\r`
// once the line is accepted, and ksh93 writes neither. A terminal brackets a
// paste only for an application that asked, so a shell that never asks is
// handed a paste as plain keystrokes — newlines included, which run it.
func TestTheTerminalIsAskedToBracketAPaste(t *testing.T) {
	for _, c := range []struct {
		name  string
		style EditorStyle
		asks  bool
	}{
		{"a dialect that brackets", pasting, true},
		{"a dialect that does not", zeroAnswers, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			// A terminal state rather than nil: a paste is something a
			// terminal does, and the offer is withheld where there is none —
			// measured, bash writes neither marker on a pipe (#4249). So the
			// session this row is about is one that took a mode.
			e := Shell{Editor: c.style}.newEditor(t.Context(), &terminalState{})
			e.in, e.out = typing("echo hi\r"), &out
			if _, err := e.readLine(drawPrompt("$ ")); err != nil {
				t.Fatal(err)
			}
			drawn := out.String()
			on := strings.Index(drawn, pasteModeOn)
			off := strings.Index(drawn, pasteModeOff)
			if !c.asks {
				if on >= 0 || off >= 0 {
					t.Fatalf("a dialect that does not bracket wrote the mode anyway: %q", drawn)
				}
				return
			}
			switch {
			case on < 0:
				t.Fatalf("the terminal was never asked to bracket a paste: %q", drawn)
			case off < 0:
				t.Fatalf("the request was never taken back, so the command runs with it on: %q", drawn)
			}
			// Around the read, which is what the order says: the request
			// comes before the prompt the line is typed at, and is taken back
			// after the newline that ends it.
			if prompt := strings.Index(drawn, "$ "); on >= prompt {
				t.Errorf("the request came after the prompt rather than before it: %q", drawn)
			}
			if end := strings.LastIndex(drawn, "\r\n"); off <= end {
				t.Errorf("the request was taken back before the line ended rather than after it: %q", drawn)
			}
		})
	}
}

// A newline inside a paste goes in the line and does not submit it.
//
// This is the user-visible half. Without the bracketing a pasted script runs
// line by line as it arrives — measured, ours and real bash behaved identically
// on an *unbracketed* multi-line paste, both running each line as its newline
// came — and the difference is that bash arranges never to receive one that
// way. Measured against bash 5.3.3 with the markers in place: a paste of
// `echo AAA\recho BBB` draws on two rows, runs neither command, and runs both
// when Return is pressed.
func TestANewlineInsideAPasteDoesNotSubmitTheLine(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"a carriage return, as a terminal sends one", paste("echo AAA\recho BBB") + "\r", "echo AAA\necho BBB"},
		{"a line feed", paste("echo AAA\necho BBB") + "\r", "echo AAA\necho BBB"},
		// Measured: bash treats each of the two as a break of its own, so a
		// `\r\n` inside a paste is two rows and not one.
		{"both, which is two breaks and not one", paste("a\r\nb") + "\r", "a\n\nb"},
		// And the marker itself never reaches the line, which is what ksh93
		// does with it — it has no bracketing, so it types `^[[200~`.
		{"the markers are not typed into the line", paste("x") + "\r", "x"},
		// A paste around the cursor, so the insertion is not merely an append.
		{"a paste lands where the cursor is", "echo Z\x02" + paste("AB") + "\r", "echo ABZ"},
		// Text either side of it, typed the ordinary way.
		{"typing goes on after a paste", paste("echo ") + "hi\r", "echo hi"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: pasting}.newEditor(t.Context(), nil)
			e.in, e.out = &chunks{s: []string{c.keys}}, &out
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("the line came back as %q, want %q", got, c.want)
			}
		})
	}
}

// The same newline typed rather than pasted still submits.
//
// The pair that says the editor is choosing by *how the text arrived* rather
// than by what is in it. A Return is a Return; only the run between the markers
// is text.
func TestATypedNewlineStillSubmitsTheLine(t *testing.T) {
	var out strings.Builder
	e := Shell{Editor: pasting}.newEditor(t.Context(), nil)
	e.in, e.out = typing("echo AAA\recho BBB\r"), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo AAA" {
		t.Fatalf("the line came back as %q, want %q — a typed Return no longer submits", got, "echo AAA")
	}
}

// A paste is drawn when it arrives, not when the next keystroke forces a
// redraw.
//
// The second half of #2775, and the reason this needs the paced reader: the
// text did land in the line, and a test that read the screen at the end of the
// session saw it there either way. What was wrong is *when* — the screen still
// read `$ ` until something else was typed.
func TestAPasteIsDrawnWhenItArrives(t *testing.T) {
	for _, c := range []struct {
		name  string
		style EditorStyle
		want  string
	}{
		// Whether the dialect marks it or not, the text is on the screen
		// before the next key is read.
		{"a dialect that marks a paste", pasting, "\x1b[7mecho PASTED\x1b[27m"},
		{"a dialect that does not", zeroAnswers, "echo PASTED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			var byTheNextKey string
			e := Shell{Editor: c.style}.newEditor(t.Context(), nil)
			e.in = &chunks{
				s:      []string{paste("echo PASTED"), "Z\r"},
				before: func() { byTheNextKey = out.String() },
			}
			e.out = &out
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != "echo PASTEDZ" {
				t.Fatalf("the line came back as %q, want %q", got, "echo PASTEDZ")
			}
			if !strings.Contains(byTheNextKey, c.want) {
				t.Errorf("by the time the next key was read the terminal had been sent %q,\nwhich does not contain %q — the paste was not drawn when it arrived",
					byTheNextKey, c.want)
			}
		})
	}
}

// The mark comes off on the next keystroke.
//
// Measured in both shells that draw one: the line is drawn again without the
// reverse video whatever the next key is. A mark that stayed would leave the
// whole line inverse for as long as it was being edited.
func TestThePasteMarkComesOffOnTheNextKeystroke(t *testing.T) {
	var out strings.Builder
	var beforeTheKey int
	e := Shell{Editor: pasting}.newEditor(t.Context(), nil)
	e.in = &chunks{
		s:      []string{paste("echo PASTED"), "Z\r"},
		before: func() { beforeTheKey = out.Len() },
	}
	e.out = &out
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	after := out.String()[beforeTheKey:]
	if !strings.Contains(after, "echo PASTEDZ") {
		t.Fatalf("the keystroke after the paste did not draw the line: %q", after)
	}
	if strings.Contains(after, "\x1b[7m") {
		t.Errorf("the line was drawn still marked as pasted after the next keystroke: %q", after)
	}
}

// What of a paste belongs in the line.
//
// Line endings become a newline apiece, and every other control character is
// dropped — see pastedRunes for the measurement behind each. This is the unit
// that says what happens to the bytes; the tests above say what happens to the
// line.
func TestWhatOfAPasteBelongsInTheLine(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"plain text is itself", "echo hi", "echo hi"},
		{"a carriage return is a newline", "a\rb", "a\nb"},
		{"a line feed is a newline", "a\nb", "a\nb"},
		{"a tab is dropped, having no caret notation to draw it in", "a\tb", "ab"},
		{"an escape is dropped, so a paste cannot drive the terminal", "a\x1b[31mb", "a[31mb"},
		{"a control character is dropped", "a\x01b", "ab"},
		{"a delete is dropped", "a\x7fb", "ab"},
		{"characters outside ASCII are kept whole", "echo 日本語", "echo 日本語"},
		{"a byte that is no character is dropped", "a\xffb", "ab"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := string(pastedRunes([]byte(c.in))); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// A newline in the line ends the row it is on.
//
// Only a paste puts one there, and every count in this package is built on
// place: which row the cursor is on, which row a redraw comes back up to,
// where the line ends. A newline counted as an ordinary character would put
// every row after it one out, and the redraw would paint the prompt into the
// middle of the line.
func TestANewlineInTheLineEndsTheRow(t *testing.T) {
	const cols = 20
	line := []rune("echo AAA\necho BBB")
	curRow, curCol, endRow, endCol := place(2, line, len(line), cols)
	if endRow != 1 || endCol != len("echo BBB") {
		t.Errorf("the line ends at row %d column %d, want row 1 column %d", endRow, endCol, len("echo BBB"))
	}
	if curRow != endRow || curCol != endCol {
		t.Errorf("the cursor is at row %d column %d, want the end of the line", curRow, curCol)
	}
	// The character after the newline is at the start of the row below, not at
	// the column the newline was in.
	if row, col := placeAt(2, line, len("echo AAA\n"), cols); row != 1 || col != 0 {
		t.Errorf("the first character of the second row is at row %d column %d, want row 1 column 0", row, col)
	}
}

// And it is spelled for a terminal that is not translating anything.
//
// The line editor takes the terminal fully raw, so a bare line feed moves down
// a row and leaves the cursor in the column it was in — the second row of a
// pasted script would start wherever the first one ended.
func TestANewlineIsDrawnAsAReturnAndALineFeed(t *testing.T) {
	if got := onScreen("a\nb"); got != "a\r\nb" {
		t.Errorf("drew %q, want %q", got, "a\r\nb")
	}
	if got := onScreen("echo hi"); got != "echo hi" {
		t.Errorf("a line with no newline in it was rewritten: %q", got)
	}
}

// And the whole of it through a real terminal, in a real session.
//
// The reader-driven tests above say what the editor does with the bytes. This
// says the bytes reach it and the sequences reach the terminal, which is the
// failure the shipped binary had: a rendered-surface feature can be right in
// every unit test and absent from the thing a person runs. The editor is only
// built where there is a terminal, so raw mode, the reads and the redraw are
// all on the far side of what a reader can exercise.
func TestABracketedPasteThroughATerminal(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) { sh.Editor = pasting })
	// The request is written at the first prompt, before anything is typed.
	waitFor(t, s.screen, pasteModeOn, "the request to bracket a paste")

	// Two commands in one paste. Neither runs as its newline arrives, which is
	// the whole point of the bracketing — the Return below is what runs them.
	s.typeLine(paste("echo AAA\recho BBB"))
	waitFor(t, s.screen, "\x1b[7m", "the paste drawn as pasted")
	if ran := s.ran.String(); strings.Contains(ran, "AAA") {
		t.Fatalf("the paste ran before Return was pressed: %q", ran)
	}
	s.typeKeys("\r")
	waitFor(t, s.ran, "AAA", "the first pasted command's output")
	waitFor(t, s.ran, "BBB", "the second pasted command's output")
	// And the request is taken back for the length of the command, which is
	// what keeps a program the shell runs from being handed markers it never
	// asked for.
	waitFor(t, s.screen, pasteModeOff, "the request taken back")
	s.end()
}
