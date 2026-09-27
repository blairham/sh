// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect that reads a command string whole, and one that reads it a line at
// a time, with everything else about them equal.
//
// The route set says this shell expands no alias in its own `-c` text, and
// that is a stand-in: what the reference does is read the string *whole*, so
// the `alias` on line 1 has not run when line 2 is parsed. The two readings
// agree about a definition the string makes and part company over one the
// shell already had — see interp.Runner.AliasesReadWhole for the measured
// rows.
func readWholeShell(whole bool) driver.Shell {
	sh := shell()
	d := syntax.Core()
	d.AliasesExpandUnlessTold = true
	// Every route but the command string, which is the one column in the
	// panel that answers this way.
	d.ExpandAliasesInProgramText = syntax.RouteFromScriptFile | syntax.RouteOnStandardInput
	sh.Dialect = d
	// Whether `alias` reads options is a dialect's answer and an unanswered
	// vector refuses the word, which would leave the table empty and every
	// row below comparing one silence with another.
	sh.Semantics.AliasParsesOptions = interp.No
	sh.Diagnostics.CommandStringParsedWhole = whole
	// The alias the shell starts with, defined before a line of the program
	// is read — which is the state the two readings disagree about, and the
	// state a `-f` shell of the dialect this models is in.
	sh.Prelude = "alias tt='echo T'\n"
	return sh
}

// TestAnAliasTheShellAlreadyHadExpandsInATextReadWhole is #4747: the route
// set's stand-in was one-sided.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `env -i … -f` with `env -u FPATH`, against
// that shell's own two startup aliases:
//
//	$'unalias which-command\nwhich-command print'    print, 0
//	$'alias which-command=echo\nwhich-command hi'    the value it had before
//	$'unsetopt aliases\nwhich-command print'         print, 0
//	$'alias myal=print\nmyal hi'                     `myal` not found
//	'f() { which-command print; }; f'                print, 0
//
// The fourth row is the control and it is the reason the stand-in existed at
// all. The first three are what separate the two readings, and they say the
// **switch** is frozen with the tables: neither an `unalias`, a redefinition
// nor turning expansion off reaches a parse that has already happened.
func TestAnAliasTheShellAlreadyHadExpandsInATextReadWhole(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		whole, line string
		// unread is a word the run must have been *unable* to find, so a row
		// whose answer is "nothing came out" is a row where the line ran and
		// the word was not replaced rather than a row where nothing happened.
		unread string
	}{
		{
			"a name the shell already had",
			"tt\n",
			"T\n", "", "",
		},
		{
			"a name the text itself defines",
			"alias nn='echo N'\nnn\n",
			"", "", "nn",
		},
		{
			"an unalias the text has not run yet",
			"unalias tt\ntt\n",
			"T\n", "", "",
		},
		{
			"a definition the text makes over one it had",
			"alias tt='echo OVER'\ntt\n",
			"T\n", "", "",
		},
		{
			"inside a function body, which is the same parse",
			"f() { tt; }\nf\n",
			"T\n", "", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := runArgs(t, readWholeShell(true), "testsh", "-c", tc.src)
			if out != tc.whole {
				t.Errorf("read whole wrote %q, want %q", out, tc.whole)
			}
			if tc.unread != "" && !strings.Contains(errs, tc.unread+": not found") {
				t.Errorf("read whole said %q, want the word %q looked for and not found", errs, tc.unread)
			}
			// The control on the other side: a dialect that reads the same
			// text a line at a time and declines the route expands nothing
			// at all, which is what the front end did on both before this.
			out, errs, _ = runArgs(t, readWholeShell(false), "testsh", "-c", tc.src)
			if out != tc.line {
				t.Errorf("read a line at a time wrote %q, want %q", out, tc.line)
			}
			if tc.line == "" && !strings.Contains(errs, "not found") {
				t.Errorf("read a line at a time said %q, want a word looked for and not found", errs)
			}
		})
	}
}

// And the route the dialect does claim is untouched: a file is read as it runs,
// so a definition on line 1 reaches line 2 there.
//
// This is the row that says the change is about the *text read whole* and not
// about aliases in general — a fix that handed every route a frozen copy would
// pass everything above and fail here.
func TestAFileStillExpandsWhatItsOwnFirstLineDefined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("alias nn='echo N'\nnn\ntt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runArgs(t, readWholeShell(true), "testsh", path)
	if want := "N\nT\n"; out != want || code != 0 {
		t.Errorf("a script wrote %q status %d (stderr %q), want %q at 0", out, code, errs, want)
	}
	if strings.Contains(errs, "panic") {
		t.Fatalf("the run panicked: %s", errs)
	}
}
