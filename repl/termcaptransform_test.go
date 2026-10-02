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
