// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/repl"
)

// `typeset -p` on the parameters a widget is given: the widget's own, so
// without `-g` in the widget and with it in a function the widget calls, and
// no name at all once the widget is done. Measured 2026-10-02 on zsh 5.9.2
// through a pseudo-terminal; see widgetParameterDeclarations.
func TestTypesetPrintsAWidgetsParameters(t *testing.T) {
	r, out := zleRunner(t, `
		g() { typeset -p BUFFER region_highlight; }
		w() {
			POSTDISPLAY=pd; region_highlight=("0 1 bold")
			typeset -p BUFFER CURSOR LBUFFER RBUFFER POSTDISPLAY WIDGET region_highlight KEYS_QUEUED_COUNT
			g
		}
		zle -N w
	`)
	_, ok, said := runWidget(t, r, out, "w", repl.Line{Buffer: "ab", Cursor: 2})
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := `typeset BUFFER=ab
typeset -i10 CURSOR=2
typeset LBUFFER=ab
typeset RBUFFER=''
typeset POSTDISPLAY=pd
typeset -r WIDGET=w
typeset -a region_highlight=( '0 1 bold' )
typeset -i10 -r KEYS_QUEUED_COUNT=0
typeset -g BUFFER=ab
typeset -g -a region_highlight=( '0 1 bold' )
`
	if said != want {
		t.Errorf("in the widget:\n got %q\nwant %q", said, want)
	}
	out.Reset()
	f, err := parseZsh(`typeset -p BUFFER region_highlight`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "zsh:typeset:1: no such variable: BUFFER\nzsh:typeset:1: no such variable: region_highlight\n"; got != want {
		t.Errorf("after the widget:\n got %q\nwant %q", got, want)
	}
}
