// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason promptproviders_test.go is: the thing asserted is
// that an unexported builder reads a dialect's answer, and the prompt it
// builds exists only where there is a terminal.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The status a refused line leaves at a prompt is the dialect's own, and it is
// the same answer the dialect gives a script.
//
// Two routes, one table. A prompt that had a parse status of its own would be
// a second front end, which is the thing driver exists to prevent — and the
// number is not a constant across shells, so there is nothing to hard-code:
// measured, it is 2 in bash and dash, 3 in ksh93 and 1 in zsh (#1299).
func TestTheFrontEndCarriesTheDialectsParseFailureStatus(t *testing.T) {
	sh := Shell{
		Name:        "testsh",
		Diagnostics: interp.Diagnostics{SyntaxErrorStatus: 5, ForNameStatus: 6},
	}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)

	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if front.ParseFailureStatus == nil {
		t.Fatal("the prompt was given no answer for a refused line")
	}
	// Two failures with two different answers, so a wiring that reached for
	// the syntax status directly — or for any single number — is visible.
	// The second is a failure this parser finds while reading and the panel
	// finds while running, which is exactly why the dialect answers it with
	// the error in hand rather than with one number.
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		{"a syntax error", "fi", 5},
		{"a for loop whose name is not one", "for 1x in a; do :; done", 6},
	} {
		_, err := syntax.Parse(c.src, sh.Dialect)
		if err == nil {
			t.Fatalf("%s: %q parsed", c.name, c.src)
		}
		if got := front.ParseFailureStatus(err); got != c.want {
			t.Errorf("%s: the prompt answers %d, want %d", c.name, got, c.want)
		}
		if got, want := front.ParseFailureStatus(err), sh.Diagnostics.StatusForParseError(err); got != want {
			t.Errorf("%s: the prompt answers %d and a script exits %d — the two routes disagree", c.name, got, want)
		}
	}
}
