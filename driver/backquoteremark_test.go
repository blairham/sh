// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A remark a dialect makes only while it is *not* going to run the program
// (#1466).
//
// One shell in the panel remarks on every backquote substitution it reads,
// and it says so under `-n` and never otherwise — the same file run writes
// nothing at all. That is the front end's to know, because the interpreter
// holds the option and the parser holds the remark and only this package
// holds both. The tests name the wording and the rule, never the shell.

// backquoteSaying is the diagnostics of a dialect that remarks on the older
// command substitution and carries the line inside the sentence.
func backquoteSaying() interp.Diagnostics {
	return interp.Diagnostics{
		BackquoteObsolete:     "warning: line %[1]d: `...` obsolete, use $(...)",
		RemarkNamesItsOwnLine: true,
	}
}

func backquoteScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bq.sh")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestABackquoteRemarkIsHeldBackWhileRunning(t *testing.T) {
	const body = "x=`echo hi`\ny=$(echo there)`echo b`\nprintf '%s' \"$x$y\"\n"
	path := backquoteScript(t, body)

	sh := shell()
	sh.Diagnostics = backquoteSaying()

	// Checked and not run: one remark per backquote, each naming its own
	// line, and the location in front of it says only who is speaking.
	_, errs, code := runArgs(t, sh, "testsh", "-n", path)
	want := path + ": warning: line 1: `...` obsolete, use $(...)\n" +
		path + ": warning: line 2: `...` obsolete, use $(...)\n"
	if errs != want || code != 0 {
		t.Errorf("-n wrote %q code %d, want %q at 0", errs, code, want)
	}

	// Run instead, and the same parse produces the same remarks and none of
	// them is said.
	out, errs, code := runArgs(t, sh, "testsh", path)
	if out != "hithereb" || errs != "" || code != 0 {
		t.Errorf("running wrote %q / %q code %d, want the output alone", out, errs, code)
	}
}

// The same program off standard input, which is the route that says the
// remark once rather than twice: the reader retires a parser when it goes
// looking for more input, and a retire that then finds none used to leave the
// last remark in two places at once.
func TestABackquoteRemarkOnStandardInputIsSaidOnce(t *testing.T) {
	sh := shell()
	sh.Diagnostics = backquoteSaying()
	sh.Stdin = strings.NewReader("x=`echo hi`\n")
	_, errs, code := runArgs(t, sh, "testsh", "-n")
	want := "testsh: warning: line 1: `...` obsolete, use $(...)\n"
	if errs != want || code != 0 {
		t.Errorf("wrote %q code %d, want %q at 0", errs, code, want)
	}
}

// A dialect with no wording says nothing on either route, which is five of
// the six columns — silence is the answer here rather than a missing feature.
func TestNoBackquoteWordingSaysNothing(t *testing.T) {
	path := backquoteScript(t, "x=`echo hi`\nprintf '%s' \"$x\"\n")
	sh := shell()
	for _, argv := range [][]string{{"testsh", "-n", path}, {"testsh", path}} {
		_, errs, code := runArgs(t, sh, argv...)
		if errs != "" || code != 0 {
			t.Errorf("%v wrote %q code %d, want nothing at 0", argv[1:], errs, code)
		}
	}
}
