// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// `M-Delete` and `^W` are two widgets (#6284): in a dialect whose `^W` kills
// back to a blank, the widget `M-Delete` runs still kills back over a word,
// and so does a key bound to it by name.
func TestBackwardKillWordIsNotTheControlWKill(t *testing.T) {
	table := map[string]Binding{"\x18b": {Widget: WidgetBackwardKillWord}, "\x18w": {Widget: WidgetKillWordBefore}}
	for _, c := range []struct{ name, keys, want string }{
		{"M-Delete", "echo foo-bar\x1b\x7f\r", "echo foo-"},
		{"bound by name", "echo foo-bar\x18b\r", "echo foo-"},
		{"^W", "echo foo-bar\x17\r", "echo "},
		{"^W's widget bound by name", "echo foo-bar\x18w\r", "echo "},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := Shell{KeyBindings: func(Keymap) map[string]Binding { return table }}.newEditor(t.Context(), nil)
			e.in, e.out = typing(c.keys), &strings.Builder{}
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
