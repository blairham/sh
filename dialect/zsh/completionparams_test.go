// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// TestACompletionWidgetFromItsKeyHasTheWidgetParameters: the route a key
// bound to a `zle -C` widget takes, RunCompletion, gives the function the
// widget parameters, the line ones read-only. Measured 2026-10-04 through a
// pseudo-terminal against zsh 5.9.2, typing `éx` and the key: the function
// sees `W=cw t=scalar-local-readonly-special B=éx C=2`, `BUFFER=zz` is
// `read-only variable: BUFFER` and the function stops there, so `ran` stays
// unset (#5999). The function ran bare before, with `$WIDGET` empty and the
// assignment accepted.
func TestACompletionWidgetFromItsKeyHasTheWidgetParameters(t *testing.T) {
	r, out := zleRunner(t, `cf() { print -r -- "W=$WIDGET t=${(t)BUFFER} B=$BUFFER C=$CURSOR L=$LBUFFER"; BUFFER=zz; ran=widget }
zle -C cw complete-word cf
`)
	zsh.RunCompletion(r, t.Context(), "cw", repl.Completion{Line: "éx", Point: len("éx"), Start: 0, Word: "éx"})
	got := out.String()
	if want := "W=cw t=scalar-local-readonly-special B=éx C=2 L=éx\n"; !strings.HasPrefix(got, want) {
		t.Errorf("the function said %q, want it to begin %q", got, want)
	}
	if !strings.Contains(got, "read-only variable: BUFFER") {
		t.Errorf("the assignment was not refused: %q", got)
	}
	if v, set := r.GetVar("ran"); set {
		t.Errorf("the function went on past the refusal: ran=%q", v)
	}
	if _, set := r.GetVar("WIDGET"); set {
		t.Error("$WIDGET outlived the call")
	}
	assertNoError(t, r)
}

// assertNoError checks the refusal was taken back: a session that kept it
// would skip every command after the key.
func assertNoError(t *testing.T, r *interp.Runner) {
	t.Helper()
	got, _ := runZshVars(t, r, `print -r -- after`)
	if got != "after\n" {
		t.Errorf("a command after the completion printed %q, want %q", got, "after\n")
	}
}
