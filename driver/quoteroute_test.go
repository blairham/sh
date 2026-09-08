// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether an unterminated quote ends at the end of the input is a grammar
// answer that depends on *how the program arrived*, and attaching the route
// is the front end's job. One shell in the panel closes the quote at the end
// of a command string and refuses the same text in a file; a dialect flag
// applied to every route ran a truncated script, discarded everything after
// the quote and reported success (#1424).
//
// Nothing here names that shell — the flag vector is what the front end
// reads, and driver_test may not name a real one.

func quoteRouteShell() driver.Shell {
	sh := shell()
	sh.Dialect.CloseQuotesAtEOF = syntax.RouteFromCommandString
	sh.Diagnostics = interp.Diagnostics{
		Location:          interp.LocationLineWord,
		SyntaxErrorStatus: 9,
	}
	return sh
}

// TestAQuoteClosedOnACommandStringIsRefusedOnTheOtherRoutes drives all three
// invocation routes with one text.
func TestAQuoteClosedOnACommandStringIsRefusedOnTheOtherRoutes(t *testing.T) {
	const src = "echo one\necho \"abc\n"

	t.Run("a command string closes it and runs the command", func(t *testing.T) {
		out, errs, code := runArgs(t, quoteRouteShell(), "testsh", "-c", src)
		// abc and a newline: the end of input stands in for the closing
		// mark, so the newline the text ended on is *inside* the quote and
		// is part of the word. That is what the shell that does this prints,
		// and it is why the row is worth asserting rather than only the
		// status — a rule that merely stopped refusing would print `abc`.
		if out != "one\nabc\n\n" {
			t.Errorf("output %q, want one, then abc carrying the newline it swallowed", out)
		}
		if errs != "" {
			t.Errorf("stderr %q, want nothing said", errs)
		}
		if code != 0 {
			t.Errorf("status %d, want 0", code)
		}
	})

	t.Run("a script file refuses it", func(t *testing.T) {
		out, errs, code := runArgs(t, quoteRouteShell(), "testsh", writeScript(t, src))
		if out != "one\n" {
			t.Errorf("output %q, want only what stood before the quote", out)
		}
		if !strings.Contains(errs, "unterminated double quote") {
			t.Errorf("stderr %q, want the quote named", errs)
		}
		if code != 9 {
			t.Errorf("status %d, want the syntax error's 9", code)
		}
	})

	t.Run("a program on standard input refuses it", func(t *testing.T) {
		out, errs, code := runStdinProgram(t, quoteRouteShell(), src)
		if out != "one\n" {
			t.Errorf("output %q, want only what stood before the quote", out)
		}
		if !strings.Contains(errs, "unterminated double quote") {
			t.Errorf("stderr %q, want the quote named", errs)
		}
		if code != 9 {
			t.Errorf("status %d, want the syntax error's 9", code)
		}
	})
}

// TestASessionsInputIsACommandString is the fourth way in, and the one whose
// route is settled by what the type *is* rather than by an argument vector:
// every input a session runs is a command string, so a dialect that ends an
// unterminated quote at the end of one ends it here.
//
// Asserted because nothing else would notice it going missing. A session
// takes no operands, so there is no route to read off the invocation and the
// answer has to be stated — and the failure is silent in the other direction
// here, the quote being refused where the shell it speaks for would have run
// the command.
func TestASessionsInputIsACommandString(t *testing.T) {
	t.Parallel()
	var out, errs strings.Builder
	sh := driver.Shell{Name: "sh", Dir: t.TempDir(), Stdout: &out, Stderr: &errs, KeepProcess: true}
	sh.Dialect = syntax.Core()
	sh.Dialect.CloseQuotesAtEOF = syntax.RouteFromCommandString
	s, code := driver.NewSession(sh)
	if code != 0 || s == nil {
		t.Fatalf("NewSession: status %d", code)
	}
	if status := s.Run(t.Context(), `echo "abc`); status != 0 {
		t.Fatalf("status %d, stderr %q — want the quote closed and the command run", status, errs.String())
	}
	if got := out.String(); got != "abc\n" {
		t.Errorf("output %q, want %q", got, "abc\n")
	}
	// The same, through the dialect that reads an input whole before running
	// any of it: a second parse of the same text by a second call, and a
	// route attached to one of the two is a route attached to neither.
	out.Reset()
	errs.Reset()
	whole := driver.Shell{Name: "sh", Dir: t.TempDir(), Stdout: &out, Stderr: &errs, KeepProcess: true}
	whole.Dialect = syntax.Core()
	whole.Dialect.CloseQuotesAtEOF = syntax.RouteFromCommandString
	whole.Diagnostics.CommandStringParsedWhole = true
	sw, code := driver.NewSession(whole)
	if code != 0 || sw == nil {
		t.Fatalf("NewSession: status %d", code)
	}
	if status := sw.Run(t.Context(), `echo "abc`); status != 0 {
		t.Fatalf("whole-first: status %d, stderr %q", status, errs.String())
	}
	if got := out.String(); got != "abc\n" {
		t.Errorf("whole-first: output %q, want %q", got, "abc\n")
	}
	// And the same input refuses where the dialect names no route at all,
	// which is what says the session is answering the flag rather than
	// ignoring it.
	out.Reset()
	errs.Reset()
	strict := driver.Shell{Name: "sh", Dir: t.TempDir(), Stdout: &out, Stderr: &errs, KeepProcess: true}
	strict.Dialect = syntax.Core()
	s2, code := driver.NewSession(strict)
	if code != 0 || s2 == nil {
		t.Fatalf("NewSession: status %d", code)
	}
	if status := s2.Run(t.Context(), `echo "abc`); status == 0 {
		t.Errorf("status 0 and output %q, want a refusal", out.String())
	}
}

// TestAGrammarChangeMidProgramKeepsTheRoute.
//
// A run-time option can replace the runner's dialect, and the front end hands
// the replacement to the parser for the lines after it. A replacement is a
// *language* and carries no route, so taking it as written turned the route's
// answer off half way down a program — the first line lenient and the rest
// strict, in one file, for no reason a reader could see.
//
// Driven from a command string, which is the direction that discriminates: on
// every other route both answers refuse and the bug is invisible.
func TestAGrammarChangeMidProgramKeepsTheRoute(t *testing.T) {
	sh := quoteRouteShell()
	sh.Register = func(r *interp.Runner) {
		r.Register("groups-on", func(r *interp.Runner, _ context.Context, _ []string) int {
			r.SetMatchOption(interp.QuantifiedGroupsEverywhere, true)
			return 0
		})
	}
	out, errs, code := runArgs(t, sh, "testsh", "-c", "groups-on\necho \"abc\n")
	if code != 0 || errs != "" {
		t.Errorf("status %d stderr %q, want the quote still closed after the toggle", code, errs)
	}
	if out != "abc\n\n" {
		t.Errorf("output %q, want abc and the newline the quote swallowed", out)
	}
}

// TestTheWholeFirstParseIsOnTheRouteToo.
//
// One dialect reads a command string whole before running any of it, so the
// failure is reported before anything has run. That is a second parse of the
// same text through a different call, and a route attached to one of the two
// is a route attached to neither: the whole-first parse would refuse a quote
// the line-by-line one then closed, and the shell would report a syntax error
// for a program it was about to run.
//
// Diagnostics.CommandStringParsedWhole is the switch, set here rather than
// named as a shell's — the front end reads the vector.
func TestTheWholeFirstParseIsOnTheRouteToo(t *testing.T) {
	sh := quoteRouteShell()
	sh.Diagnostics.CommandStringParsedWhole = true
	out, errs, code := runArgs(t, sh, "testsh", "-c", "echo one\necho \"abc\n")
	if code != 0 || errs != "" {
		t.Errorf("status %d stderr %q, want the whole-first parse to accept it", code, errs)
	}
	if out != "one\nabc\n\n" {
		t.Errorf("output %q, want one, then abc carrying the newline it swallowed", out)
	}
	// And the same shell refuses it from a file, which is what says the
	// whole-first parse is answering the route rather than always accepting.
	_, errs, code = runArgs(t, sh, "testsh", writeScript(t, "echo one\necho \"abc\n"))
	if code != 9 || !strings.Contains(errs, "unterminated double quote") {
		t.Errorf("from a file: status %d stderr %q, want the refusal", code, errs)
	}
}
