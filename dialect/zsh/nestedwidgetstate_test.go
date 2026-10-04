// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// nestingEditor is an editor that runs another widget in the middle of an
// action — what repl did with `zle-line-pre-redraw` from every redraw before
// #5864, and what any editor may still do: RunWidget is the one entry point,
// and nothing about it says it is never re-entered.
type nestingEditor struct {
	stubEditor
	r     *interp.Runner
	inner string
}

func (e *nestingEditor) Perform(_ repl.Widget, in repl.Line) (repl.Line, bool) {
	// The inner widget's own line is dropped on purpose: what this measures
	// is what its *ending* does to the caller, so the editor hands back the
	// line exactly as the non-nesting editor below does.
	zsh.RunWidget(e.r, repl.WithActions(context.Background(), e), e.inner, repl.Line{Buffer: "inner", Cursor: 2})
	return in, true
}

// A widget call inside another puts the caller's state back when it ends,
// rather than clearing it. Every parameter a widget is given is read after
// the nested call and compared with the same widget run under an editor
// that nests nothing — so the want is this shell's own answer without the
// nesting, and the comparison covers whatever the outer widget prints
// rather than a list somebody remembered to write down.
//
// Measured against zsh 5.9.2 on 2026-10-04 through a pseudo-terminal: after
// `zle .self-insert` from a `self-insert` replacement, every one of these
// reads what it read before, with `LASTWIDGET` the called widget's name. Before
// #5864 the end of the nested call cleared them all, so the caller saw no
// `$WIDGET`, no line, an unset `PENDING` — the `bad math expression`
// zsh-autosuggestions printed on every key — and was refused its next `zle`.
func TestANestedWidgetCallPutsTheCallersStateBack(t *testing.T) {
	const src = `
		outer() {
			POSTDISPLAY=pd
			region_highlight=('0 1 bold')
			local -i KEYS_QUEUED_COUNT
			zle .forward-char
			print -r -- "W=[$WIDGET] LW=[$LASTWIDGET] B=[$BUFFER] C=[$CURSOR] L=[$LBUFFER] R=[$RBUFFER]"
			print -r -- "P=[$PENDING] K=[$KEYS_QUEUED_COUNT] KM=[$KEYMAP] N=[${NUMERIC-unset}] PD=[$POSTDISPLAY] RH=[$region_highlight]"
			print -r -- "T=[${(t)BUFFER} ${(t)CURSOR} ${(t)WIDGET} ${(t)LASTWIDGET} ${(t)region_highlight} ${(t)PENDING} ${(t)KEYS_QUEUED_COUNT} ${(t)POSTDISPLAY}]"
			(( $PENDING > 0 || $KEYS_QUEUED_COUNT > 0 )); print -r -- "rc=$?"
			typeset -p BUFFER CURSOR
			BUFFER+=Z
			zle other; print -r -- "zle=$?"
		}
		zle -N outer
		other() { :; }
		zle -N other
		pre() { local seen=$WIDGET$BUFFER; }
		zle -N zle-line-pre-redraw pre
		cf() { :; }
		zle -C completer complete-word cf
	`
	in := repl.Line{Buffer: "ab", Cursor: 1}

	control, controlOut := zleRunner(t, src)
	wantLine, ok, want := runWidget(t, control, controlOut, "outer", in)
	if !ok {
		t.Fatal("the control did not run")
	}
	// The control has to have said something worth comparing against: a
	// widget that ran nothing would print nothing on both sides and agree.
	if !strings.Contains(want, "W=[outer]") || !strings.Contains(want, "zle=0") || wantLine.Buffer != "abZ" {
		t.Fatalf("the control is not a widget that ran: %q, line %q", want, wantLine.Buffer)
	}

	// A plain widget nested, and a completion one — whose line parameters are
	// read-only, which must not outlast it: `BUFFER+=Z` in the caller is the
	// row that says so.
	for _, inner := range []string{"zle-line-pre-redraw", "completer"} {
		t.Run(inner, func(t *testing.T) {
			r, out := zleRunner(t, src)
			ed := &nestingEditor{r: r, inner: inner}
			line, ok := zsh.RunWidget(r, repl.WithActions(context.Background(), ed), "outer", in)
			said := out.String()
			if !ok {
				t.Fatal("the widget did not run")
			}
			if said != want {
				t.Errorf("after a nested call the caller read\n%s\nwhere with no nesting it reads\n%s", said, want)
			}
			if line != wantLine {
				t.Errorf("line = %+v, want %+v", line, wantLine)
			}
			// And once the outermost call is over, nothing is left open: a
			// script between two keystrokes finds no widget parameters and
			// may not call a widget.
			out.Reset()
			f, err := syntax.Parse(`print -r -- "[${+BUFFER}] [${+WIDGET}] [${+PENDING}] [${+region_highlight}]"; zle other; print -r -- "zle=$?"`, zsh.Dialect())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got, wantAfter := out.String(), "[0] [0] [0] [0]\nzsh:zle:1: widgets can only be called when ZLE is active\nzle=1\n"; got != wantAfter {
				t.Errorf("after the call a script reads %q, want %q", got, wantAfter)
			}
		})
	}
}
