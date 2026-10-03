// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// `zle name -n num`, `-N` and `-K keymap`, and the `$NUMERIC` and `$KEYMAP`
// they set for the call. Every line is zsh 5.9.2's, measured 2026-10-02
// through a pseudo-terminal (#5495).
func TestAWidgetCallSetsNumericAndKeymap(t *testing.T) {
	r, out := zleRunner(t, `
		g() { print -r -- "G[$*] N=${NUMERIC-u} T=${(t)NUMERIC} K=$KEYMAP TK=${(t)KEYMAP}"; }
		zle -N g
		f() {
			print -r -- "F N=${NUMERIC-u}"; zle g; zle g -n 3 a; print -r -- "after N=${NUMERIC-u}"
			zle g -N b; zle g -K vicmd c; print -r -- "afterK K=$KEYMAP"
			zle g -K nosuch d; print st=$?; zle g -n x e; print st=$?
			zle g -n; print st=$?; zle g -K; print st=$?
			zle g -n 0 h; zle g -n -2 i; zle g -Nw j; zle g -K menuselect k; print st=$?
		}
		zle -N f
	`)
	_, ok, said := runWidget(t, r, out, "f", repl.Line{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	const kt = " K=main TK=scalar-local-readonly-special\n"
	want := "F N=u\n" +
		"G[] N=u T=" + kt +
		"G[a] N=3 T=integer-local-special" + kt +
		"after N=u\n" +
		"G[b] N=u T=" + kt +
		"G[c] N=u T= K=vicmd TK=scalar-local-readonly-special\n" +
		"afterK K=main\n" +
		"st=1\n" +
		"G[e] N=0 T=integer-local-special" + kt +
		"st=0\n" +
		"f:zle:4: number expected after -n\nst=1\n" +
		"f:zle:4: keymap expected after -K\nst=1\n" +
		"G[h] N=0 T=integer-local-special" + kt +
		"G[i] N=-2 T=integer-local-special" + kt +
		"G[j] N=u T=" + kt +
		"st=1\n"
	if said != want {
		t.Errorf("got %q\nwant %q", said, want)
	}
	_, _, said = runWidget(t, r, out, "g", repl.Line{ViCommand: true})
	if want := "G[] N=u T= K=vicmd TK=scalar-local-readonly-special\n"; said != want {
		t.Errorf("in vi command mode: got %q, want %q", said, want)
	}
}

// A count typed before the key is `$NUMERIC` in the widget, and the call
// options move it as before: with ESC 5 typed, `zle g` sees 5, `-N` 1 and
// `-n 3` 3, and the widget's own is 5 again after. Measured 2026-10-02 on zsh
// 5.9.2 through a pseudo-terminal (#5498).
func TestACountTypedBeforeTheKeyIsNumeric(t *testing.T) {
	r, out := zleRunner(t, `
		g() { print -rn -- "${NUMERIC-u} "; }
		zle -N g
		f() { print -rn -- "${NUMERIC-u} ${(t)NUMERIC} "; zle g; zle g -N; zle g -n 3; zle g -K vicmd; print -r -- "${NUMERIC-u}"; }
		zle -N f
	`)
	five, minus := 5, -1
	for _, c := range []struct {
		n    *int
		want string
	}{
		{&five, "5 integer-local-special 5 1 3 5 5\n"},
		{&minus, "-1 integer-local-special -1 1 3 -1 -1\n"},
		{nil, "u  u u 3 u u\n"},
	} {
		if _, _, said := runWidget(t, r, out, "f", repl.Line{Numeric: c.n}); said != c.want {
			t.Errorf("got %q, want %q", said, c.want)
		}
	}
}
