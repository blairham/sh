// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// `$LASTWIDGET` inside a widget: the widget the keystroke before ran, as the
// editor reported it, and from inside the widget whatever the widget last
// called with `zle` — unless the call said `-f nolast`. Measured 2026-10-02 on
// zsh 5.9.2 through a pseudo-terminal; see lastWidgetName.
func TestLastWidgetIsTheWidgetBefore(t *testing.T) {
	r, out := zleRunner(t, `
		f() {
			print -r -- "L0=$LASTWIDGET T=${(t)LASTWIDGET}"
			zle g; print -r -- "L1=$LASTWIDGET"
			zle h; zle g -f nolast a; print -r -- "L2=$LASTWIDGET"
		}
		zle -N f
		g() { print -r -- "G=$LASTWIDGET [$*]"; }
		zle -N g
		h() { :; }
		zle -N h
	`)
	for _, c := range []struct {
		name string
		last repl.LastWidget
		want string
	}{
		{"a typed character", repl.LastWidget{Widget: repl.WidgetSelfInsert, Known: true}, "self-insert"},
		{"the left arrow", repl.LastWidget{Widget: repl.WidgetBackwardChar, Known: true}, "backward-char"},
		{"a widget of the shell's", repl.LastWidget{Function: "f", Known: true}, "f"},
		{"a new line", repl.LastWidget{Accepted: true, Known: true}, "accept-line"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, ok, said := runWidget(t, r, out, "f", repl.Line{Buffer: "ab", Last: c.last})
			if !ok {
				t.Fatal("the widget did not run")
			}
			want := "L0=" + c.want + " T=scalar-local-readonly-special\nG=" + c.want + " []\nL1=g\nG=h [a]\nL2=h\n"
			if said != want {
				t.Errorf("got %q, want %q", said, want)
			}
		})
	}
}

// A flag that is not `nolast` is refused by name.
func TestAWidgetCallFlagOtherThanNolastIsRefused(t *testing.T) {
	r, out := zleRunner(t, `
		f() { zle g -f bogus y; print -r -- "st=$?"; }
		zle -N f
		g() { print -r -- "ran"; }
		zle -N g
	`)
	_, ok, said := runWidget(t, r, out, "f", repl.Line{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "f:zle: 'nolast' expected after -f\nst=1\n"; said != want {
		t.Errorf("got %q, want %q", said, want)
	}
}
