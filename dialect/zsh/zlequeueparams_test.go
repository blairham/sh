// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// How much input is waiting behind this keystroke, which a widget asks with
// `PENDING` and `KEYS_QUEUED_COUNT`. Both are 0 here and both have to
// *exist*, because the thing that reads them reads them in arithmetic.
//
// zsh-autosuggestions opens `_zsh_autosuggest_modify` with
// `(( $PENDING > 0 || $KEYS_QUEUED_COUNT > 0 ))`. An unset name is not a zero
// there — it leaves the expression with no left operand — so every character
// typed at a real prompt printed `bad math expression: operand expected at
// `> 0 || 0 > 0 '` and the suggestion never arrived (#4211).
//
// Measured on zsh 5.9.2 through a pseudo-terminal, 2026-09-22, from inside a
// widget bound to `^M`: `PENDING=[0] KQC=[0]`.
func TestTheQueueParametersAreZeroRatherThanAbsent(t *testing.T) {
	for _, name := range []string{"PENDING", "KEYS_QUEUED_COUNT"} {
		t.Run(name, func(t *testing.T) {
			r, out := zleRunner(t, fmt.Sprintf(`
				kind() { print -r -- "[${%s}] [${+%s}]"; }
				zle -N kind
			`, name, name))
			_, ok, said := runWidget(t, r, out, "kind", repl.Line{Buffer: "abc"})
			if !ok {
				t.Fatal("the widget did not run")
			}
			if !strings.Contains(said, "[0] [1]") {
				t.Errorf("$%s in a widget: got %q, want a set name worth 0", name, said)
			}
		})
	}
}

// The shape the plugin actually writes, which is the row that says the fix
// reaches arithmetic and not only `${…}`. A name that expands to the empty
// string would pass the test above if it were written with `${…-0}` and
// fails here, which is the point of spending a second suite on it.
func TestTheQueueParametersReadAsArithmetic(t *testing.T) {
	r, out := zleRunner(t, `
		kind() {
			# The plugin's own line 307, which is why only PENDING was
			# missing an operand in the message: a widget may shadow one of
			# these with a local of its own, read-only special though the
			# name is, and real zsh lets it — zsh-autosuggestions would not
			# run at all otherwise.
			local -i KEYS_QUEUED_COUNT
			if (( $PENDING > 0 || $KEYS_QUEUED_COUNT > 0 )); then
				print -r -- "[pending]"
			else
				print -r -- "[idle]"
			fi
		}
		zle -N kind
	`)
	_, ok, said := runWidget(t, r, out, "kind", repl.Line{Buffer: "abc"})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if !strings.Contains(said, "[idle]") {
		t.Errorf("the plugin's own test in a widget: got %q, want %q", said, "[idle]")
	}
	if strings.Contains(said, "bad math expression") {
		t.Errorf("the arithmetic still refuses: %q", said)
	}
}

// `integer-local-readonly-special`, which is neither of the two shapes this
// file already had: the line parameters are scalar and writable, WIDGET is
// scalar and read-only. Measured in the same pseudo-terminal run that gave
// the values — and that run reported `scalar-local-special` for BUFFER and
// `scalar-local-readonly-special` for WIDGET, the words
// zlelocaltype_test.go already asserts, which is what says it was reading
// the shell it claimed to be reading.
func TestTheQueueParametersCarryTheirType(t *testing.T) {
	for _, name := range []string{"PENDING", "KEYS_QUEUED_COUNT"} {
		t.Run(name, func(t *testing.T) {
			r, out := zleRunner(t, fmt.Sprintf(`
				kind() { print -r -- "[${(t)%s}]"; }
				zle -N kind
			`, name))
			_, ok, said := runWidget(t, r, out, "kind", repl.Line{Buffer: "abc"})
			if !ok {
				t.Fatal("the widget did not run")
			}
			if want := "[integer-local-readonly-special]"; !strings.Contains(said, want) {
				t.Errorf("${(t)%s} in a widget: got %q, want %s", name, said, want)
			}
		})
	}
}

// The control that makes the rows above mean something: outside a widget
// these are not parameters at all, exactly as the line parameters are not.
// A shell that answered 0 unconditionally would pass every row up there and
// fail this one.
func TestTheQueueParametersBelongToTheCall(t *testing.T) {
	for _, name := range []string{"PENDING", "KEYS_QUEUED_COUNT"} {
		t.Run(name, func(t *testing.T) {
			got := zleParam(t, fmt.Sprintf(`print -r -- "[${(t)%s}] [${+%s}]"`, name, name))
			if !strings.Contains(got, "[] [0]") {
				t.Errorf("${(t)%s} outside a widget: got %q, want an unset name with no type", name, got)
			}
		})
	}
}
