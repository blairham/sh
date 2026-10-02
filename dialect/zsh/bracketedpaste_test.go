// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `zle_bracketed_paste` is there where the editor is loaded and nowhere else.
// Measured 2026-10-02 on zsh 5.9.2: an ordinary array at a prompt on a
// terminal holding the two sequences, absent under `-fic` with no terminal,
// and created by `zmodload zsh/zle` in a script — once, so a second load does
// not put back what a script unset.
func TestTheBracketedPasteParameterIsTheEditorsOwn(t *testing.T) {
	for _, c := range []struct {
		name                  string
		interactive, terminal bool
		src, want             string
	}{
		{
			"at a prompt", true, true,
			`print -r -- ${(t)zle_bracketed_paste} ${(qqqq)zle_bracketed_paste}`,
			"array $'\\033[?2004h' $'\\033[?2004l'\n",
		},
		{"interactive with no terminal", true, false, `print -r -- ${+zle_bracketed_paste}`, "0\n"},
		{"a script", false, false, `print -r -- ${+zle_bracketed_paste}`, "0\n"},
		{
			"a script that loads the editor", false, false,
			`zmodload zsh/zle; print -r -- ${(t)zle_bracketed_paste}; unset zle_bracketed_paste; zmodload zsh/zle; print -r -- ${+zle_bracketed_paste}`,
			"array\n0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{
				Dir: t.TempDir(), Interactive: c.interactive, Terminal: c.terminal,
			}, c.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
