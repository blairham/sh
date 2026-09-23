// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// What this shell was *launched* holding reaches the names a dialect registered
// for it, once, before the first line — on every route into the shell.
//
// The front end's half of interp.Runner.SetInheritedParameterAction. The store's
// seam cannot cover this: a name that arrives in the environment is answered out
// of the inherited entries and is never written, so nothing a script does is a
// message about it. The one route a person actually uses for a startup
// parameter is exactly this one — `NAME=value prog` (#4267).
//
// Named after no shell, per this package's rule: the parameter here is invented
// for the test and the claim is about the front end.
func TestTheFrontEndDeliversWhatTheShellWasLaunchedHolding(t *testing.T) {
	const (
		name           = "DRIVER_TEST_STARTUP_PARAMETER"
		optionListName = "DRIVER_TEST_OPTION_LIST"
	)
	sh := func(out, errs *bytes.Buffer) driver.Shell {
		s := shell()
		s.Stdout, s.Stderr = out, errs
		// A location style, so that the name DiagnoseAsTheShell writes is
		// visible at all: the substrate's own Diagnostics is LocationNone,
		// which writes no prefix anywhere. The style says how a place is
		// spelled and names no shell.
		s.Diagnostics.Location = interp.LocationLineWord
		s.Register = func(r *interp.Runner) {
			// An option namespace of this test's own, so the ordering claim
			// below has two complaints to order. The name is invented; the
			// namespace is the core's.
			r.SetShellOptions(optionListName)
			r.SetInheritedParameterAction(name, func(rr *interp.Runner, value string) {
				rr.DiagnoseAsTheShell("%s: %s: heard at startup\n", name, value)
			})
		}
		return s
	}
	script := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(script, []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const want = "testsh: " + name + ": brought: heard at startup\n"

	t.Run("carried", func(t *testing.T) {
		t.Setenv(name, "brought")
		for _, route := range []struct {
			name string
			argv []string
		}{
			{"-c", []string{"testsh", "-c", "echo ran"}},
			{"a script file", []string{"testsh", script}},
		} {
			t.Run(route.name, func(t *testing.T) {
				var out, errs bytes.Buffer
				if code := driver.MainArgs(sh(&out, &errs), route.argv); code != 0 {
					t.Fatalf("status %d, stderr %q", code, errs.String())
				}
				if errs.String() != want {
					t.Errorf("stderr = %q, want %q", errs.String(), want)
				}
				if strings.TrimSpace(out.String()) != "ran" {
					t.Errorf("stdout = %q, want the script to have run", out.String())
				}
			})
		}
	})

	// Absent is not the same as empty, and neither is a message about a value:
	// a name the environment did not carry is not delivered at all.
	t.Run("not carried", func(t *testing.T) {
		// t.Setenv first, so the suite puts whatever was there back; then
		// away, because absent is not empty.
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		var out, errs bytes.Buffer
		driver.MainArgs(sh(&out, &errs), []string{"testsh", "-c", "echo ran"})
		if errs.String() != "" {
			t.Errorf("stderr = %q, want nothing said", errs.String())
		}
	})

	// Carried empty *is* delivered, because "inherited and empty" is a value a
	// dialect may have something to say about.
	t.Run("carried empty", func(t *testing.T) {
		t.Setenv(name, "")
		var out, errs bytes.Buffer
		driver.MainArgs(sh(&out, &errs), []string{"testsh", "-c", "echo ran"})
		if got := "testsh: " + name + ": : heard at startup\n"; errs.String() != got {
			t.Errorf("stderr = %q, want %q", errs.String(), got)
		}
	})

	// And it is delivered before the inherited option list, which is measured
	// on the shell that has both — see
	// interp.Runner.ApplyInheritedParameters. The list is the core's own
	// namespace, so an unknown name in it complains here too.
	t.Run("before the inherited option list", func(t *testing.T) {
		t.Setenv(name, "brought")
		t.Setenv(optionListName, "nosuchoption")
		var out, errs bytes.Buffer
		driver.MainArgs(sh(&out, &errs), []string{"testsh", "-c", "echo ran"})
		lines := strings.Split(strings.TrimRight(errs.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("stderr = %q, want two lines", errs.String())
		}
		if lines[0] != strings.TrimRight(want, "\n") {
			t.Errorf("first line = %q, want the parameter's %q", lines[0], want)
		}
		if !strings.Contains(lines[1], "nosuchoption") {
			t.Errorf("second line = %q, want the option list's complaint", lines[1])
		}
	})
}
