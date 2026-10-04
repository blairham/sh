// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestAStartupFileIsAFileForTheQuoteRule holds the one route that is neither
// the invocation's nor a builtin's: a startup file is read from a path
// whatever route the program itself came by, so a dialect that ends an
// unterminated quote at the end of a *command string* must not end one at the
// end of `$ENV`.
//
// Written against the command-string route deliberately. That is the route
// the flag is on, so a front end that let the invocation's route stand for
// the startup file's would close the quote here and the file would run — a
// truncated startup file then changing the shell's behavior silently, which
// is the shape of #1424 one door further in.
func TestAStartupFileIsAFileForTheQuoteRule(t *testing.T) {
	sem := withStartupFile()
	sh, out, errs := startupShell(t, sem)
	sh.Dialect.CloseQuotesAtEOF = syntax.RouteFromCommandString
	sh.Diagnostics = interp.Diagnostics{
		Location:          interp.LocationLineWord,
		SyntaxErrorStatus: 9,
	}
	t.Setenv(startupVar, scriptAt(t, "FROM_FILE=\"yes\n"))
	code := MainArgs(sh, []string{"testsh", "-c", `echo "[${FROM_FILE-unset}]"`})
	if !strings.Contains(errs.String(), "unterminated double quote") {
		t.Errorf("stderr %q, want the startup file's quote refused", errs.String())
	}
	// A startup file that will not parse costs that file and not the
	// program (#5869), so the program runs and shows what the file did. Both
	// halves move when the route does: with the file taken as a command
	// string the quote closes, `FROM_FILE="yes` becomes an assignment of
	// "yes\n", and the program prints that instead.
	if want := "[unset]\n"; out.String() != want {
		t.Errorf("out %q, want %q: the startup file was refused", out.String(), want)
	}
	if code != 0 {
		t.Errorf("status %d, want the program's own 0", code)
	}
}
