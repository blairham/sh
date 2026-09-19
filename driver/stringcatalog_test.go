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

// #3003: an invocation option asking the shell to list the strings the program
// marked for translation rather than run it.
//
// Named for the axis rather than for a shell, as the help option next door is.
// What is under test is the *route* half — which invocations answer it, which
// of the two forms wins, and what a parse failure does.
func catalogShell(opt interp.StringCatalogOption) driver.Shell {
	d := syntax.Core()
	d.DollarDoubleQuote = true
	sem := interp.PosixSemantics()
	sem.StringCatalogOption = opt
	dg := interp.Diagnostics{InvocationBadLongOption: "%[1]s: invalid option"}
	return driver.Shell{
		Name:        "testsh",
		Dialect:     d,
		Semantics:   sem,
		Diagnostics: dg,
	}
}

func catalogScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestAStringCatalogOptionIsAnsweredFromTheVector(t *testing.T) {
	opt := interp.StringCatalogOption{
		Spellings:               "-D --dump-strings",
		PortableObjectSpellings: "--dump-po-strings",
	}

	t.Run("each marked string, and nothing else", func(t *testing.T) {
		// The control is inside the row: the unmarked string must not be
		// listed, or an option that printed every quoted word would pass.
		path := catalogScript(t, "s.sh", "echo \"plain\"\necho $\"marked\"\n")
		var out, errs strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &errs
		if r := driver.MainArgs(sh, []string{"testsh", "-D", path}); r != 0 {
			t.Errorf("status %d, want 0 (err %q)", r, errs.String())
		}
		if got, want := out.String(), "\"marked\"\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
	})

	t.Run("the long spelling of the same option", func(t *testing.T) {
		path := catalogScript(t, "s.sh", "echo $\"one\"\n")
		var out strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &out
		if r := driver.MainArgs(sh, []string{"testsh", "--dump-strings", path}); r != 0 {
			t.Errorf("status %d, want 0", r)
		}
		if got, want := out.String(), "\"one\"\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
	})

	t.Run("a catalog entry names the file and the line", func(t *testing.T) {
		path := catalogScript(t, "s.sh", "echo x\n\necho $\"deep\"\n")
		var out strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &out
		if r := driver.MainArgs(sh, []string{"testsh", "--dump-po-strings", path}); r != 0 {
			t.Errorf("status %d, want 0", r)
		}
		want := "#: " + path + ":3\nmsgid \"deep\"\nmsgstr \"\"\n"
		if out.String() != want {
			t.Errorf("stdout %q, want %q", out.String(), want)
		}
	})

	t.Run("a string holding a newline is written in pieces", func(t *testing.T) {
		path := catalogScript(t, "s.sh", "echo $\"a\nb\"\n")
		var out strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &out
		if r := driver.MainArgs(sh, []string{"testsh", "--dump-po-strings", path}); r != 0 {
			t.Errorf("status %d, want 0", r)
		}
		want := "#: " + path + ":1\nmsgid \"\"\n\"a\\n\"\n\"b\"\nmsgstr \"\"\n"
		if out.String() != want {
			t.Errorf("stdout %q, want %q", out.String(), want)
		}
	})

	t.Run("the portable form wins in either order", func(t *testing.T) {
		path := catalogScript(t, "s.sh", "echo $\"one\"\n")
		for _, argv := range [][]string{
			{"testsh", "--dump-strings", "--dump-po-strings", path},
			{"testsh", "--dump-po-strings", "--dump-strings", path},
		} {
			var out strings.Builder
			sh := catalogShell(opt)
			sh.Stdout, sh.Stderr = &out, &out
			if r := driver.MainArgs(sh, argv); r != 0 {
				t.Errorf("%v: status %d", argv, r)
			}
			want := "#: " + path + ":1\nmsgid \"one\"\nmsgstr \"\"\n"
			if out.String() != want {
				t.Errorf("%v: stdout %q, want %q", argv, out.String(), want)
			}
		}
	})

	t.Run("a command string is listed too", func(t *testing.T) {
		// The row that parts this option from the one that writes the whole
		// program back, where a command string wins outright. Measured.
		var out strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &out
		if r := driver.MainArgs(sh, []string{"testsh", "-D", "-c", "echo $\"inc\""}); r != 0 {
			t.Errorf("status %d, want 0", r)
		}
		if got, want := out.String(), "\"inc\"\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
	})

	t.Run("a parse failure lists what was read", func(t *testing.T) {
		path := catalogScript(t, "bad.sh", "echo $\"ok\"\nfor in\n")
		var out, errs strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &errs
		if r := driver.MainArgs(sh, []string{"testsh", "-D", path}); r == 0 {
			t.Errorf("status 0 for input that would not parse")
		}
		if got, want := out.String(), "\"ok\"\n"; got != want {
			t.Errorf("stdout %q, want %q", got, want)
		}
		if errs.String() == "" {
			t.Errorf("no diagnostic for a failed parse")
		}
	})

	t.Run("a file that will not open is not a listing", func(t *testing.T) {
		var out, errs strings.Builder
		sh := catalogShell(opt)
		sh.Stdout, sh.Stderr = &out, &errs
		missing := filepath.Join(t.TempDir(), "nope.sh")
		if r := driver.MainArgs(sh, []string{"testsh", "-D", missing}); r == 0 {
			t.Errorf("status 0 for a file that is not there")
		}
		if out.String() != "" {
			t.Errorf("stdout %q, want nothing", out.String())
		}
	})

	t.Run("both listings compose, strings first", func(t *testing.T) {
		// Measured: asked for the strings and for the program, the shell
		// writes both — strings, then program — in either order, and one
		// diagnostic where the input would not parse. They are not two
		// spellings of one request, and nothing here arbitrates between
		// them.
		sh := catalogShell(opt)
		sh.Semantics.ScriptListingOption = interp.ScriptListingOption{
			Spellings:          "--pretty-print",
			ParseFailureStatus: 1,
		}
		path := catalogScript(t, "s.sh", "echo $\"one\"\n")
		for _, argv := range [][]string{
			{"testsh", "--dump-strings", "--pretty-print", path},
			{"testsh", "--pretty-print", "--dump-strings", path},
		} {
			var out strings.Builder
			run := sh
			run.Stdout, run.Stderr = &out, &out
			run.Register = func(r *interp.Runner) {
				r.SetScriptListingLayout(syntax.Layout{TranslatedWordWrittenPlain: true})
			}
			if got := driver.MainArgs(run, argv); got != 0 {
				t.Errorf("%v: status %d", argv, got)
			}
			if want := "\"one\"\necho \"one\""; out.String() != want {
				t.Errorf("%v: stdout %q, want %q", argv, out.String(), want)
			}
		}
	})

	t.Run("refused where the dialect names none", func(t *testing.T) {
		path := catalogScript(t, "s.sh", "echo $\"one\"\n")
		var out, errs strings.Builder
		sh := catalogShell(interp.StringCatalogOption{})
		sh.Stdout, sh.Stderr = &out, &errs
		if r := driver.MainArgs(sh, []string{"testsh", "--dump-strings", path}); r != 2 {
			t.Errorf("status %d, want 2", r)
		}
		if !strings.Contains(errs.String(), "--dump-strings: invalid option") {
			t.Errorf("stderr %q", errs.String())
		}
	})
}
