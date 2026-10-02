// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The editor's terminal operations reach a transformation under their termcap
// names, and nothing else in the stream does. The names are the ones zsh
// 5.9.2 passes, measured 2026-10-02; see termcaptransform.go.
func TestTheEditorsOperationsAreTransformedByTermcapName(t *testing.T) {
	show := func(code, arg string) (string, bool) {
		if arg != "" {
			return "<" + code + ":" + arg + ">", true
		}
		return "<" + code + ">", true
	}
	for _, c := range []struct{ in, want string }{
		{"\x1b[J", "<cd>"},
		{"\x1b[K", "<ce>"},
		{"\b\b", "<le><le>"},
		{"\x1b[18D\x1b[18C", "<LE:18><RI:18>"},
		{"\x1b[1D", "<le>"},
		{"\x1b[A\x1b[3A\x1b[2B", "<up><UP:3><DO:2>"},
		{"\x1b[0m\x1b[27m\x1b[24m\x1b[J% ", "\x1b[0m\x1b[27m\x1b[24m<cd>% "},
		{"\r\n", "\r\n"},
		{"\x1b[2J\x1b[1K\x1b[?2004h\x1b[38;5;1m", "\x1b[2J\x1b[1K\x1b[?2004h\x1b[38;5;1m"},
		{"plain", "plain"},
		{"\x1b[", "\x1b["},
	} {
		if got := transformTermcapSequences(c.in, show); got != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
	none := func(string, string) (string, bool) { return "", false }
	if got := transformTermcapSequences("a\x1b[Jb", none); got != "a\x1b[Jb" {
		t.Errorf("with nothing installed: got %q", got)
	}
}

// And through the editor, which is where every one of them goes out: a
// transformation that blanks them all leaves the prompt and the line and none
// of the control sequences, which is what zsh's own zle tests install.
func TestTheEditorWritesTheTransformationInPlaceOfItsSequences(t *testing.T) {
	var out strings.Builder
	style := EditorStyle{ClearBeforeThePrompt: "\x1b[0m\x1b[J"}
	e := Shell{Editor: style}.newEditor(t.Context(), &terminalState{})
	var seen []string
	e.transformTermcap = func(code, arg string) (string, bool) {
		seen = append(seen, code+arg)
		return "", true
	}
	e.in, e.out = typing("ab\x02X\r"), &out
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	drawn := out.String()
	for _, seq := range []string{"\x1b[J", "\x1b[K", "\b", "\x1b[D"} {
		if strings.Contains(drawn, seq) {
			t.Errorf("%q went out untransformed: %q", seq, drawn)
		}
	}
	if !strings.Contains(drawn, "\x1b[0m") || !strings.Contains(drawn, "$ ") {
		t.Errorf("the attribute or the prompt was lost: %q", drawn)
	}
	if len(seen) == 0 {
		t.Errorf("the transformation was never asked: %q", drawn)
	}
}

// A highlighter's codes are attributes and never reach the transformation,
// even where they spell a cursor movement: `ESC[3Dm` is a foreground's ending
// under `zle_highlight`'s `fg_default_code:D`, and zsh writes it as it is.
func TestAHighlightersCodesAreNotTransformed(t *testing.T) {
	blank := func(string, string) (string, bool) { return "", true }
	in := "\x1b[D" + highlightGuard + "\x1b[3Dm" + highlightGuard + "x\x1b[K"
	if got, want := transformTermcapSequences(in, blank), "\x1b[3Dmx"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	none := func(string, string) (string, bool) { return "", false }
	if got, want := transformTermcapSequences(in, none), "\x1b[D\x1b[3Dmx\x1b[K"; got != want {
		t.Errorf("with nothing installed: got %q, want %q", got, want)
	}
}

// And through the editor: what a highlighter asks for goes out as it is with
// a transformation installed, guards and all taken away.
func TestTheEditorKeepsAHighlightersCodesFromTheTransformation(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), &terminalState{})
	e.transformTermcap = func(string, string) (string, bool) { return "", true }
	e.highlighter = HighlighterFunc(func(string) []Highlight {
		return []Highlight{{Start: 1, Style: "\x1b[3Dm", Point: true}}
	})
	e.in, e.out = typing("ab\r"), &out
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	drawn := out.String()
	if !strings.Contains(drawn, "a\x1b[3Dmb") || strings.Contains(drawn, highlightGuard) {
		t.Errorf("the highlighter's code was not written as it is: %q", drawn)
	}
}
