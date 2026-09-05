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

// TestARefusedInvocationOptionExitsTheDialectsStatus is #483: the front end
// exited its own `usageStatus` for a refused option *letter*, so a dialect
// answering 1 exited 2 — while the same dialect's `-o nosuchoption` already
// exited 1, because only the long spelling could carry an answer back.
//
// Both spellings in one table, since the point is that they agree.
func TestARefusedInvocationOptionExitsTheDialectsStatus(t *testing.T) {
	shellWith := func(status int) driver.Shell {
		sem := interp.PosixSemantics()
		// Not fatal, so that what is being measured is the status the front
		// end returns rather than the one a dying script leaves behind.
		sem.BadSetOptionNameFatal = interp.No
		return driver.Shell{
			Name:        "testsh",
			Dialect:     syntax.Core(),
			Semantics:   sem,
			Diagnostics: interp.Diagnostics{SetInvalidOptionStatus: status},
		}
	}
	for _, c := range []struct {
		name string
		argv []string
	}{
		{"a letter", []string{"testsh", "-q", "-c", "echo hi"}},
		{"a letter in a bundle", []string{"testsh", "-eq", "-c", "echo hi"}},
		{"a name", []string{"testsh", "-o", "nosuchoption", "-c", "echo hi"}},
		{"a letter with a script route behind it", []string{"testsh", "-q"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, want := range []int{1, 2, 3} {
				var out, errs strings.Builder
				sh := shellWith(want)
				sh.Stdout, sh.Stderr = &out, &errs
				if got := driver.MainArgs(sh, c.argv); got != want {
					t.Errorf("status %d, want the dialect's %d (stderr %q)", got, want, errs.String())
				}
				if out.String() != "" {
					t.Errorf("ran %q, want a refused option to stop the shell before anything runs", out.String())
				}
				if errs.String() == "" {
					t.Error("said nothing, want the refusal reported")
				}
			}
		})
	}
}

// TestARefusedInvocationOptionDefaultsToTwo: a dialect with no answer of its
// own gets 2, which is what three of the panel report and what the front end
// used to report for everybody.
func TestARefusedInvocationOptionDefaultsToTwo(t *testing.T) {
	for _, argv := range [][]string{
		{"testsh", "-q", "-c", "echo hi"},
		{"testsh", "-o", "nosuchoption", "-c", "echo hi"},
	} {
		var out, errs strings.Builder
		sh := driver.Shell{Name: "testsh", Dialect: syntax.Core(), Stdout: &out, Stderr: &errs}
		if got := driver.MainArgs(sh, argv); got != 2 {
			t.Errorf("%q gave %d, want 2 (stderr %q)", argv, got, errs.String())
		}
	}
}
