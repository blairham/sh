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

// The other remark a dialect makes only while it is *not* going to run the
// program (#2409), and the one that says `-n` is a **lint mode with rules**
// in that shell rather than a parse check that happens to warn.
//
// The tests name the wording and the route, never the shell.

// spacingSaying is the diagnostics of a dialect that remarks on two operators
// written with no blank between them and carries the line inside the
// sentence.
func spacingSaying() interp.Diagnostics {
	return interp.Diagnostics{
		OperatorsNotSeparated: "warning: line %[1]d: use space or tab to " +
			"separate operators %[2]s and %[3]s",
		RemarkNamesItsOwnLine: true,
	}
}

func spacingScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestASpacingRemarkIsHeldBackWhileRunning is the whole of what #2409 turned
// out to be, and it is the half two rounds of measurement had wrong.
//
// The program parses, runs and exits 0 — there is no refusal for the remark
// to ride on — and the shell still says nothing about it when it is going to
// run. Only the checked route prints, and that route lives here.
func TestASpacingRemarkIsHeldBackWhileRunning(t *testing.T) {
	const body = "printf a\n(:);(:)\n"
	path := spacingScript(t, body)

	sh := shell()
	sh.Diagnostics = spacingSaying()

	_, errs, code := runArgs(t, sh, "testsh", "-n", path)
	want := path + ": warning: line 2: use space or tab to separate operators ; and (\n"
	if errs != want || code != 0 {
		t.Errorf("-n wrote %q code %d, want %q at 0", errs, code, want)
	}

	out, errs, code := runArgs(t, sh, "testsh", path)
	if out != "a" || errs != "" || code != 0 {
		t.Errorf("running wrote %q / %q code %d, want the output alone", out, errs, code)
	}
}

// `-c` and standard input are runs too, so both are silent — which is the
// measurement the issue's own table was missing, having been taken entirely
// under one flag.
func TestTheRunningRoutesAreAllSilent(t *testing.T) {
	sh := shell()
	sh.Diagnostics = spacingSaying()

	_, errs, code := runArgs(t, sh, "testsh", "-c", "(:);(:)")
	if errs != "" || code != 0 {
		t.Errorf("-c wrote %q code %d, want nothing at 0", errs, code)
	}

	sh.Stdin = strings.NewReader("(:);(:)\n")
	_, errs, code = runArgs(t, sh, "testsh")
	if errs != "" || code != 0 {
		t.Errorf("stdin wrote %q code %d, want nothing at 0", errs, code)
	}
}

// Checked from standard input says it once, the same seam the backquote
// remark found: a retire that then finds no more input used to leave the last
// remark in two places at once.
func TestASpacingRemarkOnStandardInputIsSaidOnce(t *testing.T) {
	sh := shell()
	sh.Diagnostics = spacingSaying()
	sh.Stdin = strings.NewReader("(:);(:)\n")
	_, errs, code := runArgs(t, sh, "testsh", "-n")
	want := "testsh: warning: line 1: use space or tab to separate operators ; and (\n"
	if errs != want || code != 0 {
		t.Errorf("wrote %q code %d, want %q at 0", errs, code, want)
	}
}

// TestASpacingRemarkComesBeforeTheRefusal is the shape #2409 was filed from:
// the warning, then the syntax error, in that order, and the status is the
// refusal's.
func TestASpacingRemarkComesBeforeTheRefusal(t *testing.T) {
	path := spacingScript(t, "if |; then :; fi\n")
	sh := shell()
	sh.Diagnostics = spacingSaying()
	_, errs, code := runArgs(t, sh, "testsh", "-n", path)
	warn := path + ": warning: line 1: use space or tab to separate operators | and ;\n"
	if !strings.HasPrefix(errs, warn) {
		t.Errorf("wrote %q, want it to start with %q", errs, warn)
	}
	if strings.Count(errs, "\n") != 2 || code == 0 {
		t.Errorf("wrote %q code %d, want the warning then the refusal", errs, code)
	}
}

// A dialect with no wording says nothing on either route, which is five of
// the six columns.
func TestNoSpacingWordingSaysNothing(t *testing.T) {
	path := spacingScript(t, "printf a\n(:);(:)\n")
	sh := shell()
	for _, argv := range [][]string{{"testsh", "-n", path}, {"testsh", path}} {
		_, errs, code := runArgs(t, sh, argv...)
		if errs != "" || code != 0 {
			t.Errorf("%v wrote %q code %d, want nothing at 0", argv[1:], errs, code)
		}
	}
}
