// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `zle_bracketed_paste` comes into being when the line editor's module loads,
// and only then. Measured 2026-10-02 on zsh 5.9.2 through a
// pseudo-terminal; see zleboot.go (#5497).
func TestTheBracketedPasteParameterIsTheEditorsOwn(t *testing.T) {
	const kind = `print -r -- ${(t)zle_bracketed_paste} ${(qqqq)zle_bracketed_paste}`
	for _, c := range []struct {
		name                  string
		interactive, terminal bool
		zleOff                bool
		src, want             string
	}{
		{"at a prompt", true, true, false, kind, "array $'\\033[?2004h' $'\\033[?2004l'\n"},
		{"at a prompt with the editor off", true, true, true, `print -r -- ${+zle_bracketed_paste}`, "0\n"},
		{
			"the editor off, then a builtin of its module", true, true, true,
			`bindkey -l >/dev/null; print -r -- ${+zle_bracketed_paste}`, "1\n",
		},
		{"interactive with no terminal", true, false, false, `print -r -- ${+zle_bracketed_paste}`, "0\n"},
		{"a script", false, false, false, `print -r -- ${+zle_bracketed_paste}`, "0\n"},
		{
			"a script that loads the editor", false, false, false,
			`zmodload zsh/zle; print -r -- ${(t)zle_bracketed_paste}; unset zle_bracketed_paste; zmodload zsh/zle; bindkey -l >/dev/null; print -r -- ${+zle_bracketed_paste}`,
			"array\n0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			r := preset.Runner(dialecttest.Base{
				Dir: t.TempDir(), Interactive: c.interactive, Terminal: c.terminal,
				Stdout: &out, Stderr: &out,
			})
			if c.zleOff {
				if _, err := r.Run(context.Background(), preset.Parse(t, "unsetopt zle")); err != nil {
					t.Fatal(err)
				}
			}
			zsh.BeforeStartupFiles(r)
			if _, err := r.Run(context.Background(), preset.Parse(t, c.src)); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := out.String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The editor starting a line loads the module where nothing had: `setopt
// zle` under `-fiV +Z` brings the parameter at the next line, and a script
// that unset it after the load does not get it back from the next line.
func TestStartingALineLoadsTheEditorsModuleOnce(t *testing.T) {
	var out bytes.Buffer
	r := preset.Runner(dialecttest.Base{
		Dir: t.TempDir(), Interactive: true, Terminal: true, Stdout: &out, Stderr: &out,
	})
	run := func(src string) {
		if _, err := r.Run(context.Background(), preset.Parse(t, src)); err != nil {
			t.Fatal(err)
		}
	}
	run("unsetopt zle")
	zsh.BeforeStartupFiles(r)
	zsh.StartLine(r)
	run(`print -r -- ${+zle_bracketed_paste}; setopt zle`)
	zsh.StartLine(r)
	run(`print -r -- ${+zle_bracketed_paste}; unset zle_bracketed_paste`)
	zsh.StartLine(r)
	run(`print -r -- ${+zle_bracketed_paste}`)
	if got, want := out.String(), "0\n1\n0\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
