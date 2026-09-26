// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// Whether a doubled `'` inside a single-quoted string is one literal quote is
// decided while a word is *read*, so it is a dialect field rather than a
// semantics axis — and one dialect spells it as an option a running script
// switches. That makes the front end the only place the two can meet, exactly
// as it is for POSIX mode and for the option that moves the pattern grammar.
//
// The observable is one line of output: `pair` means the word was read with
// the flag on, `nopair` means it was not.

const doubledQuoteWord = "printf '%s\\n' 'no''pair'\n"

// doubledQuoteShell has a builtin that moves the flag, because no core `set
// -o` name reaches it: the flag exists for a dialect's own option namespace
// and the question here is the front end's, not that dialect's.
func doubledQuoteShell() driver.Shell {
	sh := shell()
	sh.Register = func(r *interp.Runner) {
		r.Register("pairquotes", func(r *interp.Runner, _ context.Context, args []string) int {
			r.SetDoubledQuoteInSingleQuotes(len(args) == 0 || args[0] != "off")
			return 0
		})
	}
	return sh
}

func TestABuiltinCanDecideHowTheNextLineReadsAQuotedRun(t *testing.T) {
	// The flag is off until something asks for it, so the two runs either
	// side of the word are the run itself.
	t.Run("nothing asked for it", func(t *testing.T) {
		out, errs, code := runArgs(t, doubledQuoteShell(), "testsh", doubledQuoteScript(t, doubledQuoteWord))
		if code != 0 || out != "nopair\n" {
			t.Errorf("got %q status %d, stderr %q, want %q", out, code, errs, "nopair\n")
		}
	})

	// A script file, which is the route the reader takes a line at a time:
	// the line after the builtin is read the new way.
	t.Run("a script file reads the next line the new way", func(t *testing.T) {
		out, errs, code := runArgs(t, doubledQuoteShell(), "testsh",
			doubledQuoteScript(t, "pairquotes\n"+doubledQuoteWord))
		if code != 0 || out != "no'pair\n" {
			t.Errorf("got %q status %d, stderr %q, want %q", out, code, errs, "no'pair\n")
		}
	})

	// And back again, which is what makes it a switch rather than a build.
	t.Run("and back again", func(t *testing.T) {
		out, errs, code := runArgs(t, doubledQuoteShell(), "testsh",
			doubledQuoteScript(t, "pairquotes\npairquotes off\n"+doubledQuoteWord))
		if code != 0 || out != "nopair\n" {
			t.Errorf("got %q status %d, stderr %q, want %q", out, code, errs, "nopair\n")
		}
	})

	// The control that says the reading is what moved and not the *word*: a
	// line already read is read the old way, however the line ends. This is
	// the row a probe written with a single line would have got wrong — and
	// it is measured behavior rather than an implementation detail, since
	// the reference shell answers the same way.
	t.Run("a line already read keeps its reading", func(t *testing.T) {
		out, errs, code := runArgs(t, doubledQuoteShell(), "testsh",
			doubledQuoteScript(t, "pairquotes; "+doubledQuoteWord))
		if code != 0 || out != "nopair\n" {
			t.Errorf("got %q status %d, stderr %q, want %q", out, code, errs, "nopair\n")
		}
	})
}

// doubledQuoteScript writes src to a script file and hands back its path, so
// the rows above read as one call each. A file rather than `-c` because the
// route is the point: this front end reads a file a line at a time.
func doubledQuoteScript(t *testing.T, src string) string {
	t.Helper()
	return writeScript(t, src)
}
