// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// Whether a kill joins the one before it is decided by the widget call
// before it, not by the keystroke (#5918), on a real terminal.
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file. Each row types `aa bb cc`, presses the keys, and
// yanks into an emptied line with ^Xy:
//
//	^Xa                  two kills inside one widget     one two
//	^W ^Xm               a key's kill, then a widget's   bb cc
//	^Xn                  a widget call between kills     bb
//	^W ^Xb               copy-region-as-kill between     bb
//	^Xk ^W               a widget, then a key's kill     bb
//	^Xf                  CUTBUFFER assigned between      bb Qcc
//	^W ESC 2 ^W          a count between two kill keys   aa bb cc
//
// Before the fix this shell joined by keystroke and wrote `one `, `bb cc`,
// `bb `, `bb X`, `bb cc`, `bb ` and `aa bb ` — the first, fourth, fifth,
// sixth and seventh rows wrong. The second and third came out right because
// the keystroke and the call before happened to agree.
func TestAKillJoinsByTheCallBeforeIt(t *testing.T) {
	control, screen := widgetSession(t, `y() { BUFFER=; zle yank; local got=$BUFFER; BUFFER=; print -r -- "YANK<$got>END" }
zle -N y; bindkey '^Xy' y
bindkey '^W' backward-kill-word
k() { zle backward-kill-word }; zle -N k; bindkey '^Xk' k
a() { BUFFER='one two'; CURSOR=7; zle backward-kill-word; zle backward-kill-word }; zle -N a; bindkey '^Xa' a
m() { zle k }; zle -N m; bindkey '^Xm' m
n() { zle k; zle backward-kill-word }; zle -N n; bindkey '^Xn' n
b() { zle copy-region-as-kill X; zle backward-kill-word }; zle -N b; bindkey '^Xb' b
f() { zle backward-kill-word; CUTBUFFER=Q$CUTBUFFER; zle backward-kill-word }; zle -N f; bindkey '^Xf' f
`)
	for _, row := range []struct{ name, keys, want string }{
		{"two kills inside one widget", "\x18a", "YANK<one two>END"},
		{"a key's kill, then a widget's", "aa bb cc\x17\x18m", "YANK<bb cc>END"},
		{"a widget call between kills", "aa bb cc\x18n", "YANK<bb >END"},
		{"copy-region-as-kill between", "aa bb cc\x17\x18b", "YANK<bb >END"},
		{"a widget, then a key's kill", "aa bb cc\x18k\x17", "YANK<bb >END"},
		{"CUTBUFFER assigned between", "aa bb cc\x18f", "YANK<bb Qcc>END"},
		{"a count between two kill keys", "aa bb cc\x17\x1b2\x17", "YANK<aa bb cc>END"},
	} {
		if _, err := control.WriteString(row.keys + "\x18y"); err != nil {
			t.Fatalf("%s: typing: %v", row.name, err)
		}
		if err := screen.Await(row.want, widgetBudget); err != nil {
			t.Fatalf("%s: the yank is not %s\n%v\n%q", row.name, row.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
