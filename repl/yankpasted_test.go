// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A yank is drawn as a paste is in zsh, and plainly in bash (#6271). See
// EditorStyle.YankIsDrawnAsPasted.
func TestAYankIsDrawnAsPasted(t *testing.T) {
	draw := func(style EditorStyle, now func() (string, string), vi bool, keys string) string {
		t.Helper()
		var out strings.Builder
		e := Shell{Editor: style, PastedTextStyle: now}.newEditor(t.Context(), nil)
		e.in, e.out, e.width = typing(keys), &out, func() int { return 80 }
		e.vi = func() bool { return vi }
		if _, err := e.readLine(drawPrompt("$ ")); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	zsh := EditorStyle{YankIsDrawnAsPasted: true, PastedTextStyle: "\x1b[7m", PastedTextStyleEnd: "\x1b[27m"}
	if got := draw(zsh, nil, false, "echo ab\x17\x19\r"); !strings.Contains(got, "\x1b[7mab\x1b[27m") {
		t.Errorf("zsh: the yank was not drawn in standout:\n%q", got)
	}
	// vi's `p` the same.
	if got := draw(zsh, nil, true, "echo ab\x17\x1bp\r"); !strings.Contains(got, "\x1b[7mab\x1b[27m") {
		t.Errorf("zsh: vi's p was not drawn in standout:\n%q", got)
	}
	// The live style wins over the fixed one: `zle_highlight=(paste:none)`.
	none := func() (string, string) { return "", "" }
	if got := draw(zsh, none, false, "echo ab\x17\x19\r"); strings.Contains(got, "\x1b[7m") {
		t.Errorf("zsh, paste:none: the yank was drawn in standout:\n%q", got)
	}
	underline := func() (string, string) { return "\x1b[4m", "\x1b[24m" }
	if got := draw(zsh, underline, false, "echo ab\x17\x19\r"); !strings.Contains(got, "\x1b[4mab\x1b[24m") {
		t.Errorf("zsh, paste:underline: got\n%q", got)
	}
	// bash draws a yank plainly, and still marks a paste.
	bash := EditorStyle{PastedTextStyle: "\x1b[7m", PastedTextStyleEnd: "\x1b[27m"}
	if got := draw(bash, nil, false, "echo ab\x17\x19\r"); strings.Contains(got, "\x1b[7m") {
		t.Errorf("bash: the yank was drawn in standout:\n%q", got)
	}
}
