// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"strings"
	"testing"
)

// viQuotedEditor is an editor in vi insert mode with a terminal eighty
// columns wide, so that the placeholder has somewhere to be drawn.
func viQuotedEditor(t *testing.T, style EditorStyle, keys string, out *strings.Builder) *editor {
	t.Helper()
	e := Shell{Editor: style}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), out
	e.vi = func() bool { return true }
	e.width = func() int { return 80 }
	return e
}

// `^V` in vi insert mode, against the table in quotedinsert.go (#6251).
func TestViQuotedInsertTakesTheNextKeyAsItIs(t *testing.T) {
	style := EditorStyle{ViQuotedInsert: true}
	for _, c := range []struct{ name, keys, want string }{
		{"a letter", "ab\x16x\r", "abx"},
		{"a control character", "ab\x16\x01\r", "ab\x01"},
		{"Return is not accepted", "ab\x16\r\r", "ab\r"},
		{"ESC is typed, and insert mode stays", "ab\x16\x1bx\r", "ab\x1bx"},
		{"a character of more than one byte", "ab\x16é\r", "abé"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := viQuotedEditor(t, style, c.keys, &out)
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}

	// The caret is drawn while it waits and is not typed. Nothing else in
	// this line draws one.
	var out strings.Builder
	if got, err := viQuotedEditor(t, style, "ab\x16x\r", &out).readLine(drawPrompt("$ ")); err != nil || got != "abx" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !strings.Contains(out.String(), "^") {
		t.Errorf("no caret drawn while ^V waited:\n%q", out.String())
	}

	// Without the field the key does nothing, as it always did, and draws
	// nothing.
	out.Reset()
	if got, err := viQuotedEditor(t, EditorStyle{}, "ab\x16x\r", &out).readLine(drawPrompt("$ ")); err != nil || got != "abx" {
		t.Errorf("without the field: got %q, %v", got, err)
	}
	if strings.Contains(out.String(), "^") {
		t.Errorf("without the field a caret was drawn:\n%q", out.String())
	}

	// And emacs editing's quoted-insert is untouched by it: no caret there.
	out.Reset()
	e := Shell{Editor: EditorStyle{WideEmacsKeymap: true, ViQuotedInsert: true}}.newEditor(t.Context(), nil)
	e.in, e.out, e.width = typing("ab\x16x\r"), &out, func() int { return 80 }
	if got, err := e.readLine(drawPrompt("$ ")); err != nil || got != "abx" {
		t.Errorf("emacs: got %q, %v", got, err)
	}
	if strings.Contains(out.String(), "^") {
		t.Errorf("emacs quoted-insert drew a caret:\n%q", out.String())
	}
}

// `^V ^C` rings and abandons the line in zsh, in both keymaps, where `^C`
// alone abandons it silently; in bash it is a `^C` typed. Measured against
// zsh 5.9.2 and bash 5.3.20 through a pty. See
// EditorStyle.QuotedInsertAbandonsOnControlC.
func TestQuotedInsertAndControlC(t *testing.T) {
	for _, c := range []struct {
		name string
		vi   bool
	}{{"emacs", false}, {"vi insert", true}} {
		t.Run(c.name+", zsh", func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: EditorStyle{
				WideEmacsKeymap: true, ViQuotedInsert: true, QuotedInsertAbandonsOnControlC: true,
			}}.newEditor(t.Context(), nil)
			e.in, e.out, e.width = typing("ab\x16\x03"), &out, func() int { return 80 }
			e.vi = func() bool { return c.vi }
			if _, err := e.readLine(drawPrompt("$ ")); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("got %v, want the line abandoned", err)
			}
			if n := strings.Count(out.String(), bell); n != 1 {
				t.Errorf("rang %d times, want 1:\n%q", n, out.String())
			}
		})
		t.Run(c.name+", bash", func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: EditorStyle{WordKeys: true, QuotedInsertInViInsert: true}}.newEditor(t.Context(), nil)
			e.in, e.out, e.width = typing("ab\x16\x03\r"), &out, func() int { return 80 }
			e.vi = func() bool { return c.vi }
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil || got != "ab\x03" {
				t.Errorf("got %q, %v, want the ^C typed", got, err)
			}
			if strings.Contains(out.String(), bell) {
				t.Errorf("rang:\n%q", out.String())
			}
		})
	}
}
