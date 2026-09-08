// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether an alias expands is a question about the *invocation*, so the front
// end is the only place it can be asked: the parser holds the table and this
// package is the one that knows which of the three routes the program came by.
//
// One dialect in the panel answers two routes and not the third, which is why
// this is a set rather than a flag — but the flag is what a test here may
// name, and which shell answers what belongs in its own package.

const aliasProgram = "alias hi='echo aliased'\nhi\n"

// aliasShell is a shell whose dialect expands aliases on exactly the routes
// given, with the one axis `alias` itself asks answered so the test is about
// the route and nothing else.
func aliasShell(routes syntax.ProgramRoutes) driver.Shell {
	sh := shell()
	d := syntax.Core()
	d.ExpandAliases = routes
	sh.Dialect = d
	sem := interp.CoreSemantics()
	sem.AliasParsesOptions = interp.No
	sh.Semantics = sem
	return sh
}

// runStdinArgs invokes the front end with the program on a real descriptor,
// which is the only way the standard-input route is reached: `-s` says the
// program is there, and the shell reads it as it runs.
func runStdinArgs(t *testing.T, sh driver.Shell, src string) string {
	t.Helper()
	path := writeScript(t, src)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sh.Stdin = f
	out, _, _ := runArgs(t, sh, "testsh", "-s")
	return out
}

func TestAliasExpansionFollowsTheInvocationRoute(t *testing.T) {
	for _, c := range []struct {
		name   string
		routes syntax.ProgramRoutes
		// want[route] says whether the alias expanded on that route.
		wantCommandString, wantScriptFile, wantStandardInput bool
	}{
		{"no route", syntax.RouteOnNoRoute, false, false, false},
		{"every route", syntax.RouteOnEveryRoute, true, true, true},
		// The shape one shell in the panel actually has, and the one a
		// boolean cannot hold: the two routes a real script uses, and not
		// the argument.
		{
			"a file and standard input but not a command string",
			syntax.RouteFromScriptFile | syntax.RouteOnStandardInput,
			false, true, true,
		},
		{"a command string alone", syntax.RouteFromCommandString, true, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh := aliasShell(c.routes)

			out, _, _ := runArgs(t, sh, "testsh", "-c", aliasProgram)
			if got := strings.Contains(out, "aliased"); got != c.wantCommandString {
				t.Errorf("-c: expanded=%v (%q), want %v", got, out, c.wantCommandString)
			}

			out, _, _ = runArgs(t, sh, "testsh", writeScript(t, aliasProgram))
			if got := strings.Contains(out, "aliased"); got != c.wantScriptFile {
				t.Errorf("script file: expanded=%v (%q), want %v", got, out, c.wantScriptFile)
			}

			out = runStdinArgs(t, sh, aliasProgram)
			if got := strings.Contains(out, "aliased"); got != c.wantStandardInput {
				t.Errorf("standard input: expanded=%v (%q), want %v", got, out, c.wantStandardInput)
			}
		})
	}
}

// The numbering of a program read in pieces has to carry the lines an alias
// body added, because they are in no piece of text to be counted.
//
// The standard-input route is the one that reads in pieces, and it is
// reachable only through a real descriptor — which is why this is here and
// not in the parser's own tests.
func TestABodyNewlineShiftsLaterLinesOnStandardInputToo(t *testing.T) {
	const src = "alias two='echo one\necho body'\ntwo\necho \"LINENO=$LINENO\"\n"

	sh := aliasShell(syntax.RouteOnEveryRoute)
	d := sh.Dialect
	d.AliasBodyCountsLines = true
	sh.Dialect = d
	// The last line is physically 4, and the body's newline puts it at 5.
	if out := runStdinArgs(t, sh, src); !strings.Contains(out, "LINENO=5") {
		t.Errorf("counting: got %q, want LINENO=5", out)
	}

	d.AliasBodyCountsLines = false
	sh.Dialect = d
	if out := runStdinArgs(t, sh, src); !strings.Contains(out, "LINENO=4") {
		t.Errorf("not counting: got %q, want LINENO=4", out)
	}
}
