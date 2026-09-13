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

// Whether an alias may stand in for a word the grammar reserves is decided
// while a line is *read*, so it is a dialect field rather than a semantics
// axis — and POSIX mode moves it. That makes the front end the only place the
// two can meet, exactly as it is for a grammar-reaching `set` option and for
// alias expansion itself.
//
// The observable is one line of output. The body ends in the word it shadows,
// so both readings run and the difference is `took` rather than a parse
// failure: `took` means the alias won, its absence means the loop did.

const reservedAliasProgram = "alias for='echo took;for'\nfor x in a\ndo echo \"[$x]\"\ndone\n"

// reservedAliasShell expands aliases on every route and lets one stand in for
// a reserved word, which is the answer two shells in the panel give. The
// `posix` option name is declared so a script can enter and leave the mode;
// whether a shell *has* the name is a dialect's answer and the mode is the
// core's, which is the split this rests on.
func reservedAliasShell() driver.Shell {
	sh := shell()
	d := syntax.Core()
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	d.AliasesExpandReservedWords = true
	sh.Dialect = d
	sem := interp.CoreSemantics()
	sem.AliasParsesOptions = interp.No
	sh.Semantics = sem
	sh.Register = func(r *interp.Runner) { r.AddSetOptions("posix") }
	return sh
}

// aliasWon says whether the reserved-word alias stood in, and fails the test
// if the loop did not run at all — which is what separates "the mode took the
// alias away" from "the program stopped working".
func aliasWon(t *testing.T, out, errs string, code int) bool {
	t.Helper()
	if code != 0 || !strings.Contains(out, "[a]") {
		t.Fatalf("the loop did not run: %q status %d, stderr %q", out, code, errs)
	}
	return strings.Contains(out, "took")
}

func TestPosixModeDecidesWhetherAnAliasStandsInForAReservedWord(t *testing.T) {
	// Without the mode, the dialect's own answer: the alias wins.
	t.Run("the dialect's own answer", func(t *testing.T) {
		out, errs, code := runArgs(t, reservedAliasShell(), "testsh", "-c", reservedAliasProgram)
		if !aliasWon(t, out, errs, code) {
			t.Errorf("the alias did not stand in: %q", out)
		}
	})

	// Entered by a script, which is the route that reaches the *rest* of the
	// program: the parser is handed the new dialect for every line after the
	// one that moved it.
	t.Run("entered by the script", func(t *testing.T) {
		out, errs, code := runArgs(t, reservedAliasShell(), "testsh", "-c",
			"set -o posix\n"+reservedAliasProgram)
		if aliasWon(t, out, errs, code) {
			t.Errorf("the mode did not protect the word: %q", out)
		}
	})

	// And left again, which is what makes it a mode rather than a build. The
	// answer put back is the shell's own, not the standard's opposite.
	t.Run("left again by the script", func(t *testing.T) {
		out, errs, code := runArgs(t, reservedAliasShell(), "testsh", "-c",
			"set -o posix\nset +o posix\n"+reservedAliasProgram)
		if !aliasWon(t, out, errs, code) {
			t.Errorf("leaving the mode did not hand the word back: %q", out)
		}
	})
}

// The two doors that are open before a line of the program has been read.
//
// This is the case the front end got wrong. It watched the runner's Dialect
// pointer for a change and seeded that watch with the pointer the runner
// already held — so a mode the *startup* sequence had entered compared equal
// on the first turn of the loop, and the program's own first line was parsed
// with the dialect the program was built from. Every later line got the mode
// and the first did not, which is the one line these two routes are about.
func TestPosixModeFromTheInvocationReachesTheProgramsOwnFirstLine(t *testing.T) {
	for _, c := range []struct {
		name string
		argv []string
	}{
		{"the option on the command line", []string{"testsh", "-o", "posix", "-c", reservedAliasProgram}},
		{"the shell's own name", []string{"sh", "-c", reservedAliasProgram}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runArgs(t, reservedAliasShell(), c.argv...)
			if aliasWon(t, out, errs, code) {
				t.Errorf("the mode did not reach the program: %q", out)
			}
		})
	}

	// A script file is the route a shebang takes, and it must answer the
	// same: the loop below is the program's first line there too.
	t.Run("a script file under the name", func(t *testing.T) {
		out, errs, code := runArgs(t, reservedAliasShell(), "sh",
			writeScript(t, reservedAliasProgram))
		if aliasWon(t, out, errs, code) {
			t.Errorf("the mode did not reach the script: %q", out)
		}
	})
}
