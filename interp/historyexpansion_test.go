// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// promptRunner is a Runner whose dialect has an expander and starts a prompt
// with it on, which is what bash and zsh were measured doing.
func promptRunner(t *testing.T) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.HistoryExpansion = Yes
	sem.HistoryExpansionAtAPrompt = Yes
	return newTestRunner(t, &Runner{Semantics: &sem})
}

// The order the front end has to keep: the dialect's default goes on before
// the startup files, and an rc file that turns the feature *off* is not undone
// afterwards. `set +H` in a .bashrc is how a person who does not want `!!` says
// so, and a default applied later would put it straight back.
func TestAnRcFileCanTurnHistoryExpansionOff(t *testing.T) {
	r := promptRunner(t)
	r.StartInteractiveHistory()
	if !r.HistoryExpansion() {
		t.Fatal("the dialect's default did not reach the session")
	}
	// The rc file speaks.
	r.SetHistoryExpansion(false)
	// And a second application of the default — the shape of the bug this
	// guards, where the two orders were swapped — must change nothing.
	r.StartInteractiveHistory()
	if r.HistoryExpansion() {
		t.Error("the default put back the state the rc file turned off")
	}
}

// And the mirror: a session whose dialect starts with the expander off — which
// is ksh93, measured — still reaches it through `set -H`.
func TestADialectThatStartsOffStillTakesTheRequest(t *testing.T) {
	sem := PosixSemantics()
	sem.HistoryExpansion = Yes
	sem.HistoryExpansionAtAPrompt = No
	r := newTestRunner(t, &Runner{Semantics: &sem})
	r.StartInteractiveHistory()
	if r.HistoryExpansion() {
		t.Fatal("a dialect that starts off started on")
	}
	r.SetHistoryExpansion(true)
	if !r.HistoryExpansion() {
		t.Error("`set -H` did not reach the state")
	}
	// Recording is the other state and is on for a prompt in every dialect
	// that has a history at all — measured, ksh93 records and does not expand.
	if !r.HistoryRecording() {
		t.Error("a prompt is not recording")
	}
}

// `histchars` is read out of the parameter rather than assumed, and an empty
// value turns the expander off without moving the option. Both were measured
// before anything read the parameter — see docs/spec/history.md.
func TestHistcharsIsReadFromTheParameter(t *testing.T) {
	r := promptRunner(t)
	r.StartInteractiveHistory()
	if got := r.HistoryChars().String(); got != "!^#" {
		t.Errorf("default histchars = %q, want !^#", got)
	}
	r.Vars = map[string]string{"histchars": ",%@"}
	if got := r.HistoryChars().String(); got != ",%@" {
		t.Errorf("histchars = %q, want ,%%@", got)
	}
	// And the line that follows is the point of reading it at all.
	res, err := r.ExpandHistory("echo ,,", []string{"echo one two"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := "echo echo one two"; res.Line != want {
		t.Errorf("got %q, want %q", res.Line, want)
	}
}

// With the state off ExpandHistory hands the line straight back, so the one
// place that decides is the Runner and a front end may call unconditionally.
func TestExpandHistoryIsInertWithTheStateOff(t *testing.T) {
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{Semantics: &sem})
	res, err := r.ExpandHistory("echo !!", []string{"echo one"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Line != "echo !!" || res.Changed {
		t.Errorf("got %q changed=%v, want the line untouched", res.Line, res.Changed)
	}
}
