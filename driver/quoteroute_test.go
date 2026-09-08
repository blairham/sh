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
