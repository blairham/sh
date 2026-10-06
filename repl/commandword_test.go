// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The command word which-command and run-help ask about, against the table in
// commandword.go. Every row was measured against zsh 5.9.2 with `M-?`.
func TestTheCommandWordTheCursorIsIn(t *testing.T) {
	for _, c := range []struct {
		line   string
		cursor int
		want   string
	}{
		{"echo abc def", 10, "echo"},
		{"ls -l; echo x", 5, "ls"},
		{"ls -l;echo x", 6, "echo"},
		{"ls -l; echo x", 13, "echo"},
		{"ls; ", 4, "ls"},
		{"FOO=1 ls -l", 11, "ls"},
		{"a=1 b=2", 7, ""},
		{"", 0, ""},
		{"time ls", 7, "ls"},
		{"nocorrect ls", 12, "ls"},
		{"! ls", 4, "ls"},
		{"if true; then ls; fi", 20, "fi"},
		{"if true; then ls", 16, "ls"},
		{"for i in a; do ls", 17, "ls"},
		{"builtin echo x", 14, "builtin"},
		{"noglob ls", 9, "noglob"},
		{"- ls", 4, "-"},
		{`echo "a;b" x`, 12, "echo"},
		{"echo 'a|b'", 10, "echo"},
		{"echo x && ls", 12, "ls"},
		{"echo $(ls)", 9, "echo"},
		{"echo `ls`", 8, "echo"},
		{"{ ls }", 6, "ls"},
		{`ls a\;b cat`, 11, "ls"},
		{"(echo x)", 8, "echo"},
	} {
		if got := commandWord([]rune(c.line), c.cursor); got != c.want {
			t.Errorf("%q at %d: got %q, want %q", c.line, c.cursor, got, c.want)
		}
	}
}

// which-command and run-help put the line aside and run the question in its
// place, and execute-named-cmd runs a widget by its name.
func TestTheCommandKeys(t *testing.T) {
	style := EditorStyle{WideEmacsKeymap: true, PrefixArgument: true, WhichCommandWord: "which-command", RunHelpWord: "run-help"}
	named := func() map[string]Binding {
		return map[string]Binding{
			"up-case-word":    {Widget: WidgetUpCaseWord},
			".up-case-word":   {Widget: WidgetUpCaseWord},
			"up-line":         {Widget: WidgetUpLine},
			"transpose-words": {Widget: WidgetTransposeWords},
			"quote-line":      {Widget: WidgetQuoteLine},
		}
	}
	for _, c := range []struct {
		name, keys    string
		first, second string
	}{
		{"M-? asks which-command and hands the line back", "echo abc def\x02\x02\x1b?X\r", "which-command echo", "echo abc dXef"},
		{"M-h asks run-help", "FOO=1 ls -l\x1bhX\r", "run-help ls", "FOO=1 ls -lX"},
		{"M-H too", "ls -l\x1bHX\r", "run-help ls", "ls -lX"},
		{"M-? on nothing rings and leaves the line", "a=1\x1b?X\r\r", "a=1X", ""},
		{"M-x runs a widget by name", "echo abc def\x02\x02\x02\x02\x02\x1bxup-case-word\rX\r\r", "echo abCX def", ""},
		{"a prefix one name begins with is that name", "echo abc def\x02\x02\x02\x02\x02\x1bxup-ca\rX\r\r", "echo abCX def", ""},
		{"Tab completes it", "echo abc def\x02\x02\x02\x02\x02\x1bxup-ca\t\rX\r\r", "echo abCX def", ""},
		{"a space is a hyphen", "echo abc def\x02\x02\x02\x02\x02\x1bxup case\rX\r\r", "echo abCX def", ""},
		{"the dotted name", "echo abc def\x02\x02\x02\x02\x02\x1bx.up-case-word\rX\r\r", "echo abCX def", ""},
		{"no such name rings and waits; ^G gives up", "echo abc\x1bxnosuch\r\x07X\r\r", "echo abcX", ""},
		{"an ambiguous prefix waits", "echo abc\x1bxup-\r\x07X\r\r", "echo abcX", ""},
		{"Backspace and ^U", "echo abc def\x02\x02\x02\x02\x02\x1bxzz\x7f\x15transpose-words\rX\r\r", "abc echoX def", ""},
		{"the count reaches the widget", "echo abc def ghi\x01\x1b2\x1bxup-case-word\rX\r\r", "ECHO ABCX def ghi", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: style, NamedWidgets: named}.newEditor(t.Context(), nil)
			e.in, e.out = typing(c.keys), &out
			for i, want := range []string{c.first, c.second} {
				got, err := e.readLine(drawPrompt("$ "))
				if err != nil {
					t.Fatalf("read %d: %v", i+1, err)
				}
				if got != want {
					t.Errorf("read %d = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}
