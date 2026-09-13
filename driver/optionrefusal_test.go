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
// Both spellings in one table, and since #2629 each carries **its own**
// answer: the two statuses are set to different numbers here, so a front end
// that read one field for both spellings fails on whichever half it did not
// take. That is not a hypothetical shape — BusyBox ash reports 1 for the name
// and 2 for the letter.
func TestARefusedInvocationOptionExitsTheDialectsStatus(t *testing.T) {
	shellWith := func(letter, name int) driver.Shell {
		sem := interp.PosixSemantics()
		// Not fatal, so that what is being measured is the status the front
		// end returns rather than the one a dying script leaves behind.
		sem.BadSetOptionNameFatal = interp.No
		sem.BadSetOptionLetterFatal = interp.No
		return driver.Shell{
			Name:      "testsh",
			Dialect:   syntax.Core(),
			Semantics: sem,
			Diagnostics: interp.Diagnostics{
				SetInvalidOptionNameStatus:   name,
				SetInvalidOptionLetterStatus: letter,
			},
		}
	}
	for _, c := range []struct {
		name   string
		argv   []string
		letter bool
	}{
		{"a letter", []string{"testsh", "-q", "-c", "echo hi"}, true},
		{"a letter in a bundle", []string{"testsh", "-eq", "-c", "echo hi"}, true},
		{"a name", []string{"testsh", "-o", "nosuchoption", "-c", "echo hi"}, false},
		{"a letter with a script route behind it", []string{"testsh", "-q"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Two spellings, two fields, and never the same number in both:
			// whichever this invocation is, the other field holds the answer
			// that must *not* come back.
			for _, want := range []int{1, 2, 3} {
				other := want%3 + 1
				letter, name := want, other
				if !c.letter {
					letter, name = other, want
				}
				var out, errs strings.Builder
				sh := shellWith(letter, name)
				sh.Stdout, sh.Stderr = &out, &errs
				if got := driver.MainArgs(sh, c.argv); got != want {
					t.Errorf("status %d, want this spelling's own %d and not the other's %d (stderr %q)", got, want, other, errs.String())
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

// TestDecliningAnInvocationOptionNameCanReportSuccess is #2639: one shell
// writes the complaint for a refused `set -o` name on its command line,
// declines to run what it was given, and exits **0**.
//
// The two halves are asserted together because either alone is the wrong
// answer: a shell that ran the command string would be accepting the option,
// and one that exited nonzero would be every other column. The letter by the
// same route is asserted beside it and must *not* move, since that is the
// measurement that made this an axis about one refusal rather than about this
// shell's front end.
func TestDecliningAnInvocationOptionNameCanReportSuccess(t *testing.T) {
	shell := func(zero interp.Answer) driver.Shell {
		sem := interp.PosixSemantics()
		sem.BadSetOptionNameFatal = interp.No
		sem.BadSetOptionLetterFatal = interp.No
		sem.BadSetOptionNameAtInvocationExitsZero = zero
		return driver.Shell{
			Name:      "testsh",
			Dialect:   syntax.Core(),
			Semantics: sem,
			Diagnostics: interp.Diagnostics{
				SetInvalidOptionNameStatus:   1,
				SetInvalidOptionLetterStatus: 2,
			},
		}
	}
	for _, c := range []struct {
		name string
		zero interp.Answer
		argv []string
		want int
	}{
		{"the name, where declining is not a failure", interp.Yes, []string{"testsh", "-o", "nosuchoption", "-c", "echo hi"}, 0},
		{"the name, where it is", interp.No, []string{"testsh", "-o", "nosuchoption", "-c", "echo hi"}, 1},
		{"the letter, which the axis does not reach", interp.Yes, []string{"testsh", "-q", "-c", "echo hi"}, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := shell(c.zero)
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != c.want {
				t.Errorf("status %d, want %d (stderr %q)", got, c.want, errs.String())
			}
			if out.String() != "" {
				t.Errorf("ran %q, want the shell to decline whatever status it reports", out.String())
			}
			if errs.String() == "" {
				t.Error("said nothing, want the refusal spoken even where it reports success")
			}
		})
	}
}
