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

// #3266: an invocation option asking the shell to write the program back
// instead of running it.
//
// Named for the axis rather than for a shell, as the help option next door is.
// What is under test here is the *route* half — which invocations the option
// applies to, and what a parse failure does — and not the arrangement, which is
// the dialect's and is graded in its own package.
func listingShell(t *testing.T, opt interp.ScriptListingOption) driver.Shell {
	t.Helper()
	sem := interp.PosixSemantics()
	sem.ScriptListingOption = opt
	dg := interp.Diagnostics{InvocationBadLongOption: "%[1]s: invalid option"}
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: dg,
		// The arrangement reaches the listing through the runner, exactly as
		// a function listing's does — see interp.Runner.SetScriptListingLayout
		// and the reason the layout is not on the vector.
		Register: func(r *interp.Runner) { r.SetScriptListingLayout(listed()) },
	}
}

// listed is the arrangement these tests grade against: enough of one to tell a
// listing from a run, and no shell's.
func listed() syntax.Layout {
	return syntax.Layout{
		Indent:                                  "  ",
		Nested:                                  true,
		Lines:                                   true,
		Separator:                               ";",
		KeywordTerminator:                       ";",
		BraceOpenSuffix:                         " ",
		StatementsShareALineOutsideADeclaration: true,
		FileFollowsTheSourceUnits:               true,
		TrailingBlankLine:                       true,
	}
}

func listingScriptAt(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestAScriptListingOptionIsAnsweredFromTheVector(t *testing.T) {
	opt := interp.ScriptListingOption{Spellings: "--list", ParseFailureStatus: 1}

	t.Run("a script operand is written back and never run", func(t *testing.T) {
		path := listingScriptAt(t, "s.sh", "echo a\necho b\n")
		var out, errs strings.Builder
		sh := listingShell(t, opt)
		sh.Stdout, sh.Stderr = &out, &errs
		r := driver.MainArgs(sh, []string{"testsh", "--list", path})
		if r != 0 {
			t.Errorf("status %d, want 0 (err %q)", r, errs.String())
		}
		if got, want := out.String(), "echo a\necho b\n\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
	})

	t.Run("a command string wins over the option", func(t *testing.T) {
		// Measured on the shell that has one: `--pretty-print -c 'echo hi'`
		// writes `hi`. The option is not refused and not answered — the
		// route simply decides.
		var out strings.Builder
		sh := listingShell(t, opt)
		sh.Stdout, sh.Stderr = &out, &out
		if r := driver.MainArgs(sh, []string{"testsh", "--list", "-c", "echo hi"}); r != 0 {
			t.Errorf("status %d, want 0", r)
		}
		if got, want := out.String(), "hi\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
	})

	t.Run("a parse failure writes what was read", func(t *testing.T) {
		// The control this route needs: silence would pass a check that only
		// looked at the status, and what the shell being modeled does is
		// write every unit it parsed before the failure.
		path := listingScriptAt(t, "bad.sh", "echo ok\nfor in\n")
		var out, errs strings.Builder
		sh := listingShell(t, opt)
		sh.Stdout, sh.Stderr = &out, &errs
		if r := driver.MainArgs(sh, []string{"testsh", "--list", path}); r != 1 {
			t.Errorf("status %d, want 1", r)
		}
		if got, want := out.String(), "echo ok\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
		if errs.String() == "" {
			t.Errorf("no diagnostic for a failed parse")
		}
	})

	t.Run("a file that will not open is not a listing", func(t *testing.T) {
		var out, errs strings.Builder
		sh := listingShell(t, opt)
		sh.Stdout, sh.Stderr = &out, &errs
		missing := filepath.Join(t.TempDir(), "nope.sh")
		if r := driver.MainArgs(sh, []string{"testsh", "--list", missing}); r == 0 {
			t.Errorf("status 0 for a file that is not there")
		}
		if out.String() != "" {
			t.Errorf("stdout %q, want nothing", out.String())
		}
	})

	t.Run("refused where the dialect names none", func(t *testing.T) {
		path := listingScriptAt(t, "s.sh", "echo a\n")
		var out, errs strings.Builder
		sh := listingShell(t, interp.ScriptListingOption{})
		sh.Stdout, sh.Stderr = &out, &errs
		if r := driver.MainArgs(sh, []string{"testsh", "--list", path}); r != 2 {
			t.Errorf("status %d, want 2", r)
		}
		if !strings.Contains(errs.String(), "--list: invalid option") {
			t.Errorf("stderr %q", errs.String())
		}
	})
}

// The arrangement reaches the listing from the runner and not from the vector,
// which is the one thing about the wiring a route test can check cheaply: a
// listing written with the zero Layout is the same text as one written with an
// arrangement, so a front end that forgot to ask would pass every row above.
func TestTheListingUsesTheRunnersArrangement(t *testing.T) {
	path := listingScriptAt(t, "s.sh", "if true; then echo a; echo b; fi\n")
	var out strings.Builder
	sh := listingShell(t, interp.ScriptListingOption{Spellings: "--list"})
	sh.Stdout, sh.Stderr = &out, &out
	// No arrangement at all on this one, which is what makes the row above
	// evidence: the two produce different text from the same file.
	sh.Register = nil
	if r := driver.MainArgs(sh, []string{"testsh", "--list", path}); r != 0 {
		t.Fatalf("status %d: %q", r, out.String())
	}
	// Nothing set an arrangement, so the zero Layout keeps the source's own
	// line structure: one line in, one line out.
	if got, want := out.String(), "if true; then echo a; echo b; fi"; got != want {
		t.Errorf("stdout %q, want %q", got, want)
	}
}
