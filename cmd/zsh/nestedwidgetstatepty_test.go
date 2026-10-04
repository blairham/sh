// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A widget that calls another keeps its own state, on a real terminal with a
// `zle-line-pre-redraw` widget defined — which is every session that loads a
// highlighter or zsh-history-substring-search.
//
// Before #5864 the editor ran the pre-redraw widget from the redraw that
// `zle .self-insert` and `zle -R` make, in the middle of the calling widget,
// and the end of that call cleared everything: `$WIDGET`, `$BUFFER`,
// `PENDING`, `KEYS_QUEUED_COUNT`, `POSTDISPLAY` and `region_highlight` came
// back empty, so zsh-autosuggestions' `(( $PENDING > 0 || … ))` failed on
// every key — and the line the widget handed back was empty, so Return ran
// nothing and `exit` could not end the session.
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file, typing `echo ok$((6*7)`, ^G, `)`, Return:
//
//	NESTED W=[self-insert] LW=[.self-insert] B=[echo ok$((6*7))] C=[15] P=[0] K=[0]
//	KM=[main] PD=[pd] RH=1 T=[scalar-local-special scalar-local-readonly-special
//	array-local-special integer-local-readonly-special] rc=1 mid=0
//
// on one line, and `mid=0` is the half that is not about state at all: zsh
// runs **no** pre-redraw widget from inside another widget. Before the fix
// this shell wrote every bracket empty, `rc=2` and `mid=2`.
func TestANestedWidgetCallLeavesTheCallersStateAlone(t *testing.T) {
	control, screen := widgetSession(t, `pres=0
pre() { (( pres++ )) }
zle -N zle-line-pre-redraw pre
arm() { armed=1 }
zle -N arm
bindkey '^G' arm
w() {
  if (( ! armed )); then zle .self-insert; zle -R; return; fi
  armed=0
  POSTDISPLAY=pd
  region_highlight=('0 1 bold')
  local -i KEYS_QUEUED_COUNT
  local -i before=$pres
  zle .self-insert
  zle -R
  (( $PENDING > 0 || $KEYS_QUEUED_COUNT > 0 ))
  local rc=$?
  print -r -- "NESTED W=[$WIDGET] LW=[$LASTWIDGET] B=[$BUFFER] C=[$CURSOR] P=[$PENDING] K=[$KEYS_QUEUED_COUNT] KM=[$KEYMAP] PD=[$POSTDISPLAY] RH=${#region_highlight} T=[${(t)BUFFER} ${(t)WIDGET} ${(t)region_highlight} ${(t)PENDING}] rc=$rc mid=$(( pres - before ))"
  POSTDISPLAY=
}
zle -N self-insert w
`)
	if _, err := control.WriteString("echo ok$((6*7)\a)"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	// Every bracket is filled in by the shell, so none of this row can be
	// satisfied by the terminal echoing what was typed.
	const want = "NESTED W=[self-insert] LW=[.self-insert] B=[echo ok$((6*7))] C=[15] P=[0] K=[0] KM=[main] PD=[pd] RH=1 " +
		"T=[scalar-local-special scalar-local-readonly-special array-local-special integer-local-readonly-special] " +
		"rc=1 mid=0"
	if err := screen.Await(want, widgetBudget); err != nil {
		t.Fatalf("the calling widget's state after a nested call is not\n%s\n%v\n%q", want, err,
			smoke.LastLines(screen.Text(), 6))
	}
	// And the line typed through that widget is the line that runs — the
	// half a person saw first, as a Return that did nothing. The echo of
	// the typed line carries `ok$((6*7))` and never `ok42`.
	if _, err := control.WriteString("\n"); err != nil {
		t.Fatalf("pressing Return: %v", err)
	}
	if err := screen.Await("ok42\r\n", widgetBudget); err != nil {
		t.Fatalf("the line typed through the widget did not run: %v\n%q", err,
			smoke.LastLines(screen.Text(), 6))
	}
}

// The same for a `zle -F` handler, which is how an inline suggestion arrives
// asynchronously: its `zle -R` runs no pre-redraw widget either, and a widget
// it calls afterwards still runs.
//
// Measured 2026-10-04 against zsh 5.9.2 the same way: `HANDLER rc=0 mid=0`.
// Before #5864 this shell wrote `HANDLER rc=1 mid=1` — the pre-redraw widget
// ran from the handler's redraw, and the end of it told the rest of the
// handler that the editor was not running, so `zle inner` was refused.
func TestAHandlersRedrawRunsNoPreRedrawWidget(t *testing.T) {
	_, screen := widgetSession(t, `pres=0
pre() { (( pres++ )) }
zle -N zle-line-pre-redraw pre
inner() { : }
zle -N inner
h() {
  local x
  read -u $1 x
  zle -F $1
  local -i before=$pres
  zle -R
  zle inner
  print -r -- "HANDLER rc=$? mid=$(( pres - before ))"
}
exec {fd}< <(print go)
zle -F $fd h
`)
	const want = "HANDLER rc=0 mid=0"
	if err := screen.Await(want, widgetBudget); err != nil {
		t.Fatalf("the handler wrote something other than %q: %v\n%q", want, err,
			smoke.LastLines(screen.Text(), 6))
	}
}
