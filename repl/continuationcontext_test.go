// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// The table in continuationcontext.go, row by row.
func TestAContinuationGivesTheWordItsContext(t *testing.T) {
	for _, c := range []struct {
		pre             string
		command, quoted bool
	}{
		{"for x in 1\n", false, false},
		{"for x\n", false, false},
		{"case x in\n", false, false},
		{"repeat 2\n", false, false},
		{"echo a \\\n", false, false},
		{"ls |\n", true, false},
		{"echo a &&\n", true, false},
		{"{\n", true, false},
		{"f() {\n", true, false},
		{"echo $(\n", true, false},
		{"if true\n", true, false},
		{"while true\n", true, false},
		{"for x in 1\ndo\n", true, false},
		{"for x in 1; do\n", true, false},
		{"echo \"a\n", false, true},
		{"echo 'a\n", false, true},
		{"echo \"it's\n", false, true},
		{"echo 'a\\'\n", true, false},
		{"echo \\\"a\n", true, false},
	} {
		command, quoted := continuationContext(c.pre)
		if command != c.command || quoted != c.quoted {
			t.Errorf("%q: command %v quoted %v, want %v %v", c.pre, command, quoted, c.command, c.quoted)
		}
	}
}

// `^D` on an empty continuation line lists in zsh and ends input in bash
// (#6242). Here the listing finds nothing — the word is inside a quote an
// earlier line opened — so zsh's answer is a bell, and the read goes on.
func TestControlDAtAContinuationListsInZsh(t *testing.T) {
	read := func(style EditorStyle, keys string) (string, error, int) {
		var out strings.Builder
		e := Shell{Editor: style}.newEditor(t.Context(), nil)
		e.in, e.out = typing(keys), &out
		e.prebuffer = func() string { return "echo \"a\n" }
		e.comp = CompleterFunc(func(Completion) []Candidate { return []Candidate{{Word: "zfile"}} })
		line, err := e.readLine(drawPrompt("dquote> "))
		return line, err, strings.Count(out.String(), bell)
	}
	zsh := EditorStyle{ListOnControlD: true, ControlDAtAContinuationLists: true, CompletionReadsTheContinuation: true}
	line, err, bells := read(zsh, "\x04b\"\r")
	if err != nil || line != "b\"" || bells != 1 {
		t.Errorf("zsh: got %q, %v, %d bells; want the read to go on after one bell", line, err, bells)
	}
	if _, err, _ := read(EditorStyle{}, "\x04b\"\r"); !errors.Is(err, io.EOF) {
		t.Errorf("bash: got %v, want end of input", err)
	}
	// And at the first prompt it is still end of input in zsh.
	var out strings.Builder
	e := Shell{Editor: zsh}.newEditor(t.Context(), nil)
	e.in, e.out = typing("\x04"), &out
	e.prebuffer = func() string { return "" }
	if _, err := e.readLine(drawPrompt("$ ")); !errors.Is(err, io.EOF) {
		t.Errorf("zsh at the first prompt: got %v, want end of input", err)
	}
}
