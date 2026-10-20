// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/repl"
)

// TestZleMessageGoesToTheEditor: `zle -M STRING` hands the string to the
// editor at status 0, and the arity is checked before the widget context —
// measured 2026-10-04 against zsh 5.9.2 (#5942).
func TestZleMessageGoesToTheEditor(t *testing.T) {
	r, out := zleRunner(t, `w() { zle -M "hello msg"; a=$?; zle -M -- -x; b=$?; zle -M 2>/dev/null; c=$?; zle -M x y 2>/dev/null; d=$?; print -r -- "$a $b $c $d" }
zle -N w
`)
	ed := &stubEditor{}
	_, ok, said, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if !ok || said != "0 0 1 1\n" {
		t.Errorf("ok=%v said %q, want %q", ok, said, "0 0 1 1\n")
	}
	if want := []string{"hello msg", "-x"}; !slices.Equal(ed.messages, want) {
		t.Errorf("the editor was shown %q, want %q", ed.messages, want)
	}
	for src, want := range map[string]string{
		"zle -M hi":  "zsh:zle:1: can only be called from widget function\n",
		"zle -M":     "zsh:zle:1: not enough arguments for -M\n",
		"zle -M a b": "zsh:zle:1: too many arguments for -M\n",
	} {
		if got, st := runZshVars(t, r, src); got != want || st != 1 {
			t.Errorf("%s: %q at %d, want %q at 1", src, got, st, want)
		}
	}
}
