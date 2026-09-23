// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A session whose input is a pipe still has a line editor in one dialect, and
// the editing keys are read as keys there rather than as characters of the
// line.
//
// Both answers, because the cost of each is silent in its own way: a shell
// that reads the keys where the real one does not invents a feature — `C-r`
// stops being a command name and starts searching — and one that does not
// where the real one does loses every editing key, which is what this tree did
// (#4249). The transcripts behind the split are in
// interp.Semantics.EditorReadsKeysWhereThereIsNoTerminal.
//
// What each row asserts is that the key **reached the editor**, not that a code
// path was entered: the search has to find an entry and the line it found has
// to run, which is a command's output on standard output and nothing a drawing
// could fake.
func TestAnEditorReadsKeysWhereThereIsNoTerminal(t *testing.T) {
	for _, c := range []struct {
		name, text string
		// ran is standard output, which is where the commands write. Two
		// lines mean the recall ran something.
		editorRan, plainRan string
	}{
		{
			name: "a reverse search finds an entry and runs it",
			// The session types a line, then `C-r`, the text to search for,
			// and a carriage return — which is what the key delivers, a pipe
			// having no terminal to turn it into a newline.
			text:      "echo seeded-one\n\x12seeded\r",
			editorRan: "seeded-one\nseeded-one\n",
			plainRan:  "seeded-one\n",
		},
		{
			name:      "and an up arrow walks the list",
			text:      "echo seeded-one\n\x1b[A\r",
			editorRan: "seeded-one\nseeded-one\n",
			plainRan:  "seeded-one\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, answer := range []struct {
				name   string
				editor bool
				want   string
			}{
				{"with an editor", true, c.editorRan},
				{"without one", false, c.plainRan},
			} {
				t.Run(answer.name, func(t *testing.T) {
					var ran, said strings.Builder
					r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
					r.Stdout = &ran
					s := Shell{
						Runner: r,
						In:     strings.NewReader(c.text),
						Out:    &ran, Err: &said,
						EchoTheLineWithoutATerminal: true,
						EditorWithoutATerminal:      answer.editor,
					}
					if _, err := s.Run(t.Context()); err != nil {
						t.Fatal(err)
					}
					if ran.String() != answer.want {
						t.Errorf("output = %q, want %q (stderr %q)", ran.String(), answer.want, said.String())
					}
				})
			}
		})
	}
}

// What that editor *writes* where there is no terminal, which is the half a
// suite file compares and the half a redrawing editor gets wrong.
//
// Measured 2026-09-22 against bash 5.3.20 given `--norc -i` on a pipe with
// `PS1='P> '`, the two streams kept apart: standard output holds `hi` and
// nothing else, and standard error holds the prompt and the line it read. Three
// things are asserted separately below because each was wrong on its own:
//
//   - the **stream**. Ours drew to standard output, which at a terminal is the
//     same file and on a pipe is the one the commands are using.
//   - **no redraw**. A screen is rewritten because what is on it can be
//     replaced; a transcript keeps every byte, so a rewrite leaves the line in
//     it twice. See editor.echoedTail.
//   - **no bracketed paste**. A paste is something a terminal does, and bash
//     writes neither `\e[?2004h` nor `\e[?2004l` here.
func TestAnEditorWithoutATerminalWritesWhatTheShellWrites(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
	r.Stdout = &ran
	s := Shell{
		Runner: r,
		In:     strings.NewReader("echo hi\n"),
		Out:    &ran, Err: &said,
		EchoTheLineWithoutATerminal: true,
		EditorWithoutATerminal:      true,
		// The dialect that has an editor here is also one that asks for
		// bracketed paste at a terminal, so the row is only about *this*
		// session if the offer is on: with it off the last assertion below
		// would hold for a shell that had never learned to withhold it.
		Editor: EditorStyle{BracketedPaste: true},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := "hi\n"; ran.String() != want {
		t.Errorf("output = %q, want %q — the commands' stream is theirs alone", ran.String(), want)
	}
	if want := "$ echo hi\n$ "; said.String() != want {
		t.Errorf("stderr = %q, want %q", said.String(), want)
	}
	for _, unwanted := range []struct{ name, seq string }{
		{"an erase to the end of the row", "\x1b[K"},
		{"a carriage return", "\r"},
		{"bracketed paste on", "\x1b[?2004h"},
		{"bracketed paste off", "\x1b[?2004l"},
	} {
		if strings.Contains(said.String(), unwanted.seq) {
			t.Errorf("the transcript holds %s (%q): %q", unwanted.name, unwanted.seq, said.String())
		}
	}
}
