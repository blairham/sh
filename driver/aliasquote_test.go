// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A quote an alias body opens, from the outside: what the shell prints, and
// what it exits with (#2685).
//
// The parser's own tests say what the tokens become. This says what a person
// running the script sees, which is where both halves of the defect were
// visible: one direction refused a script every shell in the panel runs, and
// the other ran a command the author never wrote.
//
// The script route, because it is the one all seven columns expand aliases on
// without an option — zsh expands none under `-c` — and because the second
// direction is about the input running out, which a one-line string cannot
// show.
func quotingAliasShell() driver.Shell {
	sh := shell()
	d := syntax.Core()
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	sh.Dialect = d
	sem := interp.CoreSemantics()
	sem.AliasParsesOptions = interp.No
	sh.Semantics = sem
	return sh
}

func TestAQuoteAnAliasBodyOpensReachesTheRestOfTheScript(t *testing.T) {
	const src = "alias q='echo \"'\necho one\nq hello\"\necho two\n"
	out, errs, code := runArgs(t, quotingAliasShell(), "testsh", writeScript(t, src))
	if want := "one\n hello\ntwo\n"; out != want {
		t.Errorf("out %q, want %q (stderr %q)", out, want, errs)
	}
	if code != 0 {
		t.Errorf("status %d, want 0 — every column in the panel runs this", code)
	}
}

// The other direction. The construct swallows the rest of the file, so the
// line behind it never runs, and the shell says so and exits nonzero — 2 in
// bash 5.3, bash 3.2, that binary as `sh`, dash and BusyBox ash, 1 in zsh and
// 3 in ksh93. Before this it exited 0 having printed `x` and `two`.
func TestAQuoteAnAliasBodyLeavesOpenStopsTheScript(t *testing.T) {
	const src = "alias a='echo \"x'\necho one\na\necho two\n"
	out, errs, code := runArgs(t, quotingAliasShell(), "testsh", writeScript(t, src))
	if want := "one\n"; out != want {
		t.Errorf("out %q, want %q — the rest of the file is inside the quote", out, want)
	}
	if code == 0 {
		t.Errorf("status 0, want a refusal: nothing in the panel runs this")
	}
	if !strings.Contains(errs, "quote") && !strings.Contains(errs, "unmatched") &&
		!strings.Contains(errs, "matching") {
		t.Errorf("said %q, want a word about the quote that was never closed", errs)
	}
}
