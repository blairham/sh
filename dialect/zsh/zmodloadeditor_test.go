// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What an interactive shell on a terminal has loaded that a script has not,
// measured against zsh 5.9.2 in a real pty on 2026-09-22 (#4233).
//
// The pty is what makes these mean anything. Off a pipe, `zsh -f -i -c` is 1
// for both modules in zsh and here alike, so a probe written that way agrees
// with the panel and separates nothing; on a pty the same invocation is 0 for
// both. See zmodload.go's zmodloadEditorLoads for the whole table.

// At a prompt, two modules are there that nothing asked for.
func TestZmodloadModulesAnInteractiveShellHasLoaded(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(), Interactive: true, Terminal: true,
	}, `zmodload -e zsh/zle
print -r -- "zle=$?"
zmodload -e zsh/complete
print -r -- "complete=$?"
zmodload -e zsh/zleparameter
print -r -- "zleparameter=$?"
zmodload`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "zle=0\ncomplete=0\nzleparameter=1\nzsh/complete\nzsh/main\nzsh/zle\n"
	if out != want || st != 0 {
		t.Errorf("at a prompt = %q (status %d), want %q", out, st, want)
	}
}

// **A terminal decides it, not a prompt loop**, which is the half the issue
// this comes from had the other way round. Interactive with nothing to edit is
// a script's answer.
func TestZmodloadEditorModulesNeedATerminalAndNotJustAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		terminal    bool
	}{
		{"neither", false, false},
		{"interactive on a pipe", true, false},
		{"a terminal and no prompt", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Interactive: tc.interactive, Terminal: tc.terminal,
			}, `zmodload -e zsh/zle
print -r -- "zle=$?"
zmodload -e zsh/complete
print -r -- "complete=$?"
zmodload`)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			want := "zle=1\ncomplete=1\nzsh/main\n"
			if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// **`zsh/zle` is there before the first startup file and `zsh/complete` is
// not**, which is the whole reason the two have separate rules. An rc file is
// where a plugin binds its widgets: `add-zle-hook-widget` guards itself with
// `zmodload -e zsh/zle`, so a shell that only had the module by the time the
// prompt was drawn would refuse every one of them and be no better off.
func TestZmodloadZleIsLoadedInsideAStartupFileAndCompleteIsNot(t *testing.T) {
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Dir: t.TempDir(), Interactive: true, Terminal: true,
		Stdout: &buf, Stderr: &buf,
	})
	rc := preset.Parse(t, `zmodload -e zsh/zle
print -r -- "zle=$?"
zmodload -e zsh/complete
print -r -- "complete=$?"`)
	if _, err := r.RunStartupFile(context.Background(), rc, "/home/person/.zshrc"); err != nil {
		t.Fatalf("run startup file: %v", err)
	}
	want := "zle=0\ncomplete=1\n"
	if got := buf.String(); got != want {
		t.Errorf("inside a startup file = %q, want %q", got, want)
	}
}

// And the same shell answers for `zsh/complete` once the startup file is
// behind it, which is the pair that makes the rule a moment rather than a
// property of the shell.
func TestZmodloadCompleteIsLoadedOnceTheStartupFileIsDone(t *testing.T) {
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Dir: t.TempDir(), Interactive: true, Terminal: true,
		Stdout: &buf, Stderr: &buf,
	})
	rc := preset.Parse(t, `zmodload -e zsh/complete
print -r -- "during=$?"`)
	if _, err := r.RunStartupFile(context.Background(), rc, "/home/person/.zshrc"); err != nil {
		t.Fatalf("run startup file: %v", err)
	}
	after := preset.Parse(t, `zmodload -e zsh/complete
print -r -- "after=$?"`)
	if _, err := r.Run(context.Background(), after); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "during=1\nafter=0\n"
	if got := buf.String(); got != want {
		t.Errorf("across the startup file = %q, want %q", got, want)
	}
}
