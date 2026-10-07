// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// zsh's vi insert keymap, against the table in viinsertkeys.go (#6272).
func TestZshViInsertKeymap(t *testing.T) {
	read := func(style EditorStyle, keys string) (string, int) {
		t.Helper()
		var out strings.Builder
		e := Shell{Editor: style}.newEditor(t.Context(), nil)
		e.in, e.out = typing(keys), &out
		e.vi = func() bool { return true }
		line, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatalf("%q: %v", keys, err)
		}
		return line, strings.Count(out.String(), bell)
	}
	zsh := EditorStyle{ZshViInsertKeymap: true}
	for _, c := range []struct {
		name, keys, want string
		bells            int
	}{
		{"control keys type themselves", "ab\x01\x02\x05\x06\x0b\x0e\x0f\x10\x13\x14\x19\x1c\x1d\x1e\x1f\r", "ab\x01\x02\x05\x06\x0b\x0e\x0f\x10\x13\x14\x19\x1c\x1d\x1e\x1f", 0},
		{"Backspace stops where insert mode began, and rings", "aa\x1bAb\x7f\x7f\r", "aa", 1},
		{"^H the same", "aa\x1bAb\x08\x08\r", "aa", 1},
		{"Backspace at the start of a fresh line rings", "\x7f\r", "", 1},
		{"^U kills back to where insert mode began, then nothing", "aa\x1bAb\x15\x15\r", "aa", 0},
		{"^U on a fresh line kills it all", "aa bb\x15\r", "", 0},
		{"^W stops where insert mode began, silently", "aa bb\x1bAcc dd\x17\x17\x17\r", "aa bb", 0},
		{"^W's word is vi's", "echo a.bc\x17\r", "echo a.", 0},
		{"^W takes the blanks after a word", "echo ab  \x17\r", "echo ", 0},
		{"^X is unbound and rings", "ab\x18\r", "ab", 1},
		// The cursor put on the `b` by way of command mode: this reader hands
		// over a byte at a time, so an Escape here is always the mode switch
		// and an arrow cannot be typed. The pty test has the arrows.
		{"^D with something typed lists and deletes nothing", "echo b\x1biaX\x04\r", "echo aXb", 1},
		{"^Q is vi-quoted-insert", "ab\x11\x16\r", "ab\x16", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, bells := read(zsh, c.keys)
			if got != c.want || bells != c.bells {
				t.Errorf("got %q with %d bells, want %q with %d", got, bells, c.want, c.bells)
			}
		})
	}
	// Without the field vi insert mode is the shared table, as bash's is.
	if got, _ := read(EditorStyle{}, "ab\x02X\r"); got != "aXb" {
		t.Errorf("without the field ^B is backward-char: got %q", got)
	}
}

// The control keys a dialect's vi insert mode types as they are, against
// bash 5.3.20's rows in EditorStyle.ViInsertTypesTheseKeys (#6301): each goes
// in the line, and a key not listed keeps its action.
func TestViInsertTypesTheKeysTheDialectLists(t *testing.T) {
	style := EditorStyle{ViInsertTypesTheseKeys: "\x01\x02\x05\x06\x07\x0b\x0c\x0f\x18\x1c\x1d\x1e"}
	for _, c := range []struct{ name, keys, want string }{
		{"^A", "ab\x01Z\r", "ab\x01Z"},
		{"^B", "ab\x02Z\r", "ab\x02Z"},
		{"^K", "ab\x0bZ\r", "ab\x0bZ"},
		{"^X", "ab\x18Z\r", "ab\x18Z"},
		{"^^", "ab\x1eZ\r", "ab\x1eZ"},
		{"^T is not one of them", "ab\x14Z\r", "baZ"},
		{"and in command mode ^B is not typed", "ab\x1b\x02iZ\r", "aZb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := viTyped(t, style, c.keys); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	if got := viTyped(t, EditorStyle{}, "ab\x02Z\r"); got != "aZb" {
		t.Errorf("without the field ^B moves back: got %q", got)
	}
}
