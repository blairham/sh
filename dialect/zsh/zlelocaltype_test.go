// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// The line parameters say `local` in their type, because a widget call opens
// them for the length of the call and zsh reports that the way it reports any
// name a scope owns.
//
// This is what a plugin asks before it trusts the parameter: a highlighter or
// a wrapper that tests for `local` in `${(t)…}` and does not find it decides
// it is not running under a real ZLE and quietly does nothing. The behavior
// was already right before #2493 — the parameters are opened for the call and
// a script outside one finds them unset — so the whole of the divergence was
// what the shell *said*.
//
// Measured against zsh 5.9.2 inside an ordinary `zle -N` widget.
func TestTheLineParametersSayLocalInTheirType(t *testing.T) {
	for _, c := range []struct {
		name string
		want string
	}{
		{"BUFFER", "scalar-local-special"},
		{"LBUFFER", "scalar-local-special"},
		{"RBUFFER", "scalar-local-special"},
		{"CURSOR", "scalar-local-special"},
		// #2493 lists WIDGET with LBUFFER and RBUFFER as "the same shape"
		// and says in the same breath that it was not probed separately. It
		// is not the same shape: a widget may not assign the name of the
		// widget it is running, so WIDGET carries `readonly` as well, and
		// the issue's own text gives that spelling for a readonly line
		// parameter. The `local` word — which is what #2493 is about — is
		// there either way.
		{"WIDGET", "scalar-local-readonly-special"},
		{"region_highlight", "array-local-special"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, out := zleRunner(t, fmt.Sprintf(`
				kind() { print -r -- "[${(t)%s}]"; }
				zle -N kind
			`, c.name))
			_, ok, said := runWidget(t, r, out, "kind", repl.Line{Buffer: "abc"})
			if !ok {
				t.Fatal("the widget did not run")
			}
			if want := "[" + c.want + "]"; !strings.Contains(said, want) {
				t.Errorf("${(t)%s} in a widget: got %q, want %s", c.name, said, want)
			}
		})
	}
}

// The word is the *call's*, not the name's: outside a widget these names are
// not parameters at all, so there is no type to carry a `local` in.
//
// This is the control that makes the test above mean something. A shell that
// answered `scalar-local-special` unconditionally — by writing the word into
// the spelling rather than by opening a scope — would pass every row up there
// and fail this one.
func TestTheTypeIsTheCallsAndNotTheNames(t *testing.T) {
	for _, name := range []string{"BUFFER", "CURSOR", "region_highlight"} {
		t.Run(name, func(t *testing.T) {
			got := zleParam(t, fmt.Sprintf(`print -r -- "[${(t)%s}] [${+%s}]"`, name, name))
			if !strings.Contains(got, "[] [0]") {
				t.Errorf("${(t)%s} outside a widget: got %q, want an unset name with no type", name, got)
			}
		})
	}
}
