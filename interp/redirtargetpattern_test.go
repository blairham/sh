// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A pattern in a redirection's target — see
// Semantics.RedirectTargetTakesPathnameExpansion.
//
// POSIX says a non-interactive shell does not pathname-expand the word after
// a redirection operator, and four of the seven columns follow it. The two
// that do not both stop in POSIX mode, which is the row that makes it an
// axis. Measured 2026-09-16 in a directory holding exactly `only-one.txt`.
//
// The output side is asserted beside the input side in every row that has
// one, because that is where the reading costs something: a shell that
// matches truncates a file the script never named, and a shell that does not
// creates one called `only-*.txt`.

// patternDir is a directory holding exactly one file a `only-*` pattern
// matches, plus a second name it does not.
func patternDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only-one.txt"), []byte("CONTENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func patternRun(t *testing.T, dir, src string, expands Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.RedirectTargetTakesPathnameExpansion = expands
		r.Semantics, r.Dir = &sem, dir
	})
}

func TestAPatternWrittenIntoARedirectionTargetIsMatchedOrNot(t *testing.T) {
	t.Run("matched, the input side", func(t *testing.T) {
		dir := patternDir(t)
		out, st := patternRun(t, dir, `cat < only-*.txt`, Yes)
		if out != "CONTENT\n" || st != 0 {
			t.Errorf("out=%q st=%d, want the matching file read", out, st)
		}
	})
	t.Run("not matched, the input side", func(t *testing.T) {
		dir := patternDir(t)
		out, st := patternRun(t, dir, `cat < only-*.txt`, No)
		if st == 0 || !strings.Contains(out, "only-*.txt") {
			t.Errorf("out=%q st=%d, want the literal name refused", out, st)
		}
		if strings.Contains(out, "CONTENT") {
			t.Errorf("out=%q, want the matching file left alone", out)
		}
	})
	// The half that costs something rather than only reporting differently.
	t.Run("matched, the output side truncates the match", func(t *testing.T) {
		dir := patternDir(t)
		if _, st := patternRun(t, dir, `printf 'X\n' > only-*.txt`, Yes); st != 0 {
			t.Fatalf("status %d", st)
		}
		if got := readFile(t, dir, "only-one.txt"); got != "X\n" {
			t.Errorf("only-one.txt = %q, want it written through the pattern", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "only-*.txt")); err == nil {
			t.Error("a file called `only-*.txt` was created as well as the match written")
		}
	})
	t.Run("not matched, the output side creates the name", func(t *testing.T) {
		dir := patternDir(t)
		if _, st := patternRun(t, dir, `printf 'X\n' > only-*.txt`, No); st != 0 {
			t.Fatalf("status %d", st)
		}
		if got := readFile(t, dir, "only-one.txt"); got != "CONTENT\n" {
			t.Errorf("only-one.txt = %q, want the match untouched", got)
		}
		if got := readFile(t, dir, "only-*.txt"); got != "X\n" {
			t.Errorf("only-*.txt = %q, want a file of that name written", got)
		}
	})
	// A pattern that arrived through an expansion takes the same answer,
	// which is why one axis covers both: `e="only-*.txt"; cat < $e` reads
	// the match in zsh and refuses in ksh93, dash and BusyBox ash.
	t.Run("a pattern out of a variable", func(t *testing.T) {
		dir := patternDir(t)
		if out, st := patternRun(t, dir, `e='only-*.txt'; cat < $e`, Yes); out != "CONTENT\n" || st != 0 {
			t.Errorf("out=%q st=%d, want the match read", out, st)
		}
		out, st := patternRun(t, dir, `e='only-*.txt'; cat < $e`, No)
		if st == 0 || strings.Contains(out, "CONTENT") {
			t.Errorf("out=%q st=%d, want the literal name refused", out, st)
		}
	})
	// The control, and the reason `> out-*.txt` read as agreement: a pattern
	// that matches nothing is the same word under both answers, and the axis
	// is not even asked for it.
	t.Run("a pattern that matches nothing", func(t *testing.T) {
		for _, a := range []Answer{Yes, No} {
			dir := patternDir(t)
			if _, st := patternRun(t, dir, `printf 'X\n' > nomatch-*.txt`, a); st != 0 {
				t.Fatalf("%v: status %d", a, st)
			}
			if got := readFile(t, dir, "nomatch-*.txt"); got != "X\n" {
				t.Errorf("%v: nomatch-*.txt = %q, want the name as written", a, got)
			}
		}
	})
}

// Field splitting is the other half, and it moves with the matching in every
// column measured: bash in POSIX mode writes a file called `a b` where its
// own mode calls the same redirection ambiguous.
func TestARedirectionTargetIsSplitOnlyWhereItIsAlsoMatched(t *testing.T) {
	dir := t.TempDir()
	out, st := run(t, `e="a b"; printf 'X\n' > $e`, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.RedirectTargetTakesPathnameExpansion = No
		sem.SplitParamExpansion = Yes
		r.Semantics, r.Dir = &sem, dir
	})
	if st != 0 || out != "" {
		t.Errorf("out=%q st=%d, want the two-word name written", out, st)
	}
	if got := readFile(t, dir, "a b"); got != "X\n" {
		t.Errorf("`a b` = %q, want one file of that name", got)
	}
	// A target that expanded to *nothing* is still nothing. The collapse
	// above replaces each view's contents and not its count, so the reading
	// that calls no words ambiguous still does — a collapse that wrote one
	// empty word instead would open a file with no name.
	out, st = run(t, `e=; printf 'X\n' > $e`, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.RedirectTargetTakesPathnameExpansion = No
		sem.SplitParamExpansion = Yes
		dg := Diagnostics{AmbiguousRedirect: "%[1]s: ambiguous redirect"}
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, t.TempDir()
	})
	if st == 0 || !strings.Contains(out, "ambiguous redirect") {
		t.Errorf("out=%q st=%d, want an empty target still ambiguous", out, st)
	}
	// And the count is still the other axis's: brace expansion makes two
	// words before either question is reached, so the same reading still
	// calls `> {c,d}` ambiguous.
	out, st = run(t, `printf 'X\n' > {c,d}`, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.RedirectTargetTakesPathnameExpansion = No
		sem.BraceExpansion = Yes
		dg := Diagnostics{AmbiguousRedirect: "%[1]s: ambiguous redirect"}
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, t.TempDir()
	})
	if st == 0 || !strings.Contains(out, "ambiguous redirect") {
		t.Errorf("out=%q st=%d, want the brace target still ambiguous", out, st)
	}
}

// An axis nothing answered refuses rather than taking a side, and names
// itself — but only where the two readings differ, so an ordinary target and
// a pattern that matched nothing ask it nothing.
func TestAnUnansweredPatternAxisRefusesOnlyWhereItDecides(t *testing.T) {
	dir := patternDir(t)
	out, st := run(t, `cat < only-*.txt`, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = No
		// Back to unanswered: the POSIX preset answers this one, the
		// standard being explicit about it, so a test about no dialect
		// having chosen has to take that answer back out.
		sem.RedirectTargetTakesPathnameExpansion = Unspecified
		r.Semantics, r.Dir = &sem, dir
	})
	if st == 0 || !strings.Contains(out, "field-split and matched as a pattern") {
		t.Errorf("out=%q st=%d, want the axis named", out, st)
	}
	if strings.Contains(out, "CONTENT") {
		t.Errorf("out=%q, want the command not run after the refusal", out)
	}
	out, st = run(t, `cat < only-one.txt`, func(r *Runner) {
		sem := permissive()
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.RedirectTargetTakesPathnameExpansion = Unspecified
		r.Semantics, r.Dir = &sem, dir
	})
	if st != 0 || out != "CONTENT\n" {
		t.Errorf("out=%q st=%d, want an ordinary target to ask nothing", out, st)
	}
}
