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

// `cd -q`, the one option letter beyond `-L` and `-P` that any of the panel
// has, and the letter that stopped a real startup dead: a plugin manager
// wraps every move it makes in an anonymous function so the directory hooks
// stay quiet, and without the letter the whole word became the operand — so
// `cd -q "$dir"` looked for a directory called `-q`, said so, and did not
// move. #1558.
//
// Measured 2026-09-08 across the panel. zsh 5.9.2 moves; bash 5.3.15, that
// binary under argv[0] `sh`, bash 3.2.57, dash and ksh93 every one refuse the
// letter by name and stay where they were.
func TestCdQuietIsAnOptionInTheOneShellThatHasIt(t *testing.T) {
	// resolved says whether `-P` was in the line, and it is asserted rather
	// than left alone because the quiet letter is the sort of thing that is
	// easy to grant by borrowing the neighboring case: a `q` that also set
	// the physical flag, or that marked `-P` as seen, would move to the right
	// directory under every check that only asks where it landed. It would
	// land there by the *other* name.
	for _, c := range []struct {
		name     string
		cmd      string
		resolved bool
	}{
		{"alone", "cd -q link/sub", false},
		{"twice", "cd -q -q link/sub", false},
		{"before -P", "cd -q -P link/sub", true},
		{"after -P", "cd -P -q link/sub", true},
		{"bundled, q first", "cd -qP link/sub", true},
		{"bundled, q second", "cd -Pq link/sub", true},
		// The letter must not eat what follows it: a `-q` that consumed a
		// word would take `link/sub` for its argument and land in HOME.
		{"with -L, which keeps the name", "cd -q -L link/sub", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			sem.CdHasQuietOption = Yes
			sem.CdRefusesUnknownOption = No
			sem.CdLastPathOptionWins = No
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "zsh", Dir: dir,
				Stdout: out, Stderr: errs,
			})
			// The status is echoed rather than read from the runner
			// afterwards: `pwd` is a command too, and its own success would
			// have overwritten a `cd` that failed.
			runCd(t, r, c.cmd+"\necho st=$?\npwd\n")
			if errs.Len() != 0 {
				t.Errorf("%s said %q, want nothing", c.cmd, errs.String())
			}
			if got := out.String(); !strings.Contains(got, "st=0\n") {
				t.Errorf("%s printed %q, want st=0", c.cmd, got)
			}
			// Asserted as arrival rather than as silence. A `cd` that
			// refused quietly and stayed put would pass a check for an empty
			// stderr, which is how the operand-eating shape hid in the first
			// place.
			got := strings.TrimSpace(out.String())
			if !strings.HasSuffix(got, "sub") {
				t.Errorf("%s left %q, want it in sub", c.cmd, got)
			}
			if isResolved := !strings.Contains(got, "link"); isResolved != c.resolved {
				t.Errorf("%s left %q; resolved = %v, want %v", c.cmd, got, isResolved, c.resolved)
			}
		})
	}
}

// The other five columns, where the letter does not exist. They refuse it by
// name, and the name they refuse is `-q` and not the directory: naming the
// operand is exactly the misreading #1558 was filed for, so both halves are
// asserted.
func TestCdQuietIsRefusedByNameWhereTheShellHasNoSuchLetter(t *testing.T) {
	for _, c := range []struct {
		name string
		cmd  string
	}{
		{"alone", "cd -q link/sub"},
		{"before -P", "cd -q -P link/sub"},
		{"bundled ahead of a real letter", "cd -qP link/sub"},
		{"bundled behind one", "cd -Pq link/sub"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			sem.CdHasQuietOption = No
			sem.CdRefusesUnknownOption = Yes
			sem.CdLastPathOptionWins = Yes
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, c.cmd+"\necho st=$?\npwd\n")
			got := errs.String()
			if !strings.Contains(got, "-q") || !strings.Contains(got, "invalid option") {
				t.Errorf("%s said %q, want it to name `-q` as an option", c.cmd, got)
			}
			if strings.Contains(got, "No such file") {
				t.Errorf("%s said %q, want no missing-directory reading", c.cmd, got)
			}
			if got := out.String(); !strings.Contains(got, "st=2\n") {
				t.Errorf("%s printed %q, want st=2", c.cmd, got)
			}
			// And it did not move, which a status alone cannot say.
			if got := strings.TrimSpace(out.String()); strings.HasSuffix(got, "sub") {
				t.Errorf("%s left %q, want it to have stayed put", c.cmd, got)
			}
		})
	}
}

// A shell that reads an unknown letter as an operand still has to read `-q`
// as an *option* when it has one — the two questions are separate, and this
// is the pairing that says so. Without it, a zsh dialect would look right for
// the wrong reason: the operand fallback also leaves `cd -q dir` silent at
// status 1, which is not the same thing as moving.
func TestCdQuietIsAskedBeforeTheUnknownLetterQuestion(t *testing.T) {
	dir, _ := cdTree(t)
	sem := PosixSemantics()
	sem.CdHasQuietOption = No
	// The zsh answer to the *other* question, so the only thing that can
	// send `-q` to the operand is this shell not having the letter.
	sem.CdRefusesUnknownOption = No
	sem.CdLastPathOptionWins = No
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "zsh", Dir: dir,
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, "cd -q link/sub\npwd\n")
	if got := errs.String(); !strings.Contains(got, "-q") || strings.Contains(got, "invalid option") {
		t.Errorf("said %q, want `-q` read as somewhere to go", got)
	}
	if got := strings.TrimSpace(out.String()); strings.HasSuffix(got, "sub") {
		t.Errorf("left %q, want it to have stayed put", got)
	}
}

// A dialect that has not answered says so rather than guessing, and the thing
// it names is `cd -q` — the shape every other unanswered axis takes.
//
// One axis, not two. The question underneath is reached only by this one
// having defaulted to no, so naming both would leave a reader to work out
// which of them decided. `cd -Z` names one; so does this.
//
// The second row is the shipping shape and the one that discriminates:
// PosixSemantics answers *neither* `cd` axis, so plain `sh` with no `-dialect`
// is exactly this. The first row alone could not tell the two readings apart —
// with the unknown-letter question answered, falling through to it produces an
// ordinary `invalid option` and not a second unanswered-axis line, so a count
// of one holds either way. It was written that way first and a mutant walked
// through it.
func TestCdQuietUnansweredIsRefusedByName(t *testing.T) {
	for _, c := range []struct {
		name    string
		refuses Answer
	}{
		{"with the letter question answered", Yes},
		{"with neither answered, as plain `sh` has them", Unspecified},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			sem.CdHasQuietOption = Unspecified
			sem.CdRefusesUnknownOption = c.refuses
			sem.CdLastPathOptionWins = Yes
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, "cd -q link/sub\npwd\n")
			got := errs.String()
			if !strings.Contains(got, "cd -q") {
				t.Errorf("said %q, want it to name `cd -q`", got)
			}
			if strings.Contains(got, "an option `cd` does not have") {
				t.Errorf("said %q, want only the question that decided", got)
			}
			if n := strings.Count(got, unansweredTail); n != 1 {
				t.Errorf("named %d unanswered axes, want 1: %q", n, got)
			}
			if got := strings.TrimSpace(out.String()); strings.HasSuffix(got, "sub") {
				t.Errorf("left %q, want it to have stayed put", got)
			}
		})
	}
}

// The sentence an unanswered axis ends with, so counting them counts axes and
// not lines.
const unansweredTail = "the shells disagree here and no dialect was chosen"

// The letter is asked about only when a `q` is there to ask about. A shell
// with no answer recorded still has to run a plain `cd`, a `cd -P` and a
// `cd --`, which is the same rule TestCdWithOnePathOptionAsksNothing states
// for the other conditional axis.
func TestCdWithoutAQuietLetterAsksNothingAboutOne(t *testing.T) {
	for _, cmd := range []string{"cd link/sub", "cd -P link/sub", "cd -- link/sub", "cd -L link/sub"} {
		t.Run(cmd, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			// Deliberately left unspecified: were it asked here, this would
			// refuse.
			sem.CdHasQuietOption = Unspecified
			sem.CdRefusesUnknownOption = Yes
			sem.CdLastPathOptionWins = Yes
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, cmd+"\npwd\n")
			if errs.Len() != 0 {
				t.Errorf("%s said %q, want nothing to be asked", cmd, errs.String())
			}
			if got := strings.TrimSpace(out.String()); !strings.HasSuffix(got, "sub") {
				t.Errorf("%s left %q, want it in sub", cmd, got)
			}
		})
	}
}

// A directory genuinely called `-q`, which is the case that says the letter
// is read as an option and not merely tolerated. In a shell with the letter,
// `cd -q` with such a directory present still means *quietly, to HOME* — it
// does not fall back to the directory — and `cd -- -q` is how the directory
// is reached. Measured in zsh 5.9.2, where a `-q` directory beside a `-Z` one
// is entered for `cd -Z` and not for `cd -q`.
func TestADirectoryCalledDashQIsReachedThroughTheEndOfOptions(t *testing.T) {
	dir, _ := cdTree(t)
	if err := os.Mkdir(filepath.Join(dir, "-q"), 0o755); err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.CdHasQuietOption = Yes
	sem.CdRefusesUnknownOption = No
	sem.CdLastPathOptionWins = No
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "zsh", Dir: dir,
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, "cd -- -q\npwd\n")
	if errs.Len() != 0 {
		t.Errorf("said %q, want nothing", errs.String())
	}
	if got := strings.TrimSpace(out.String()); !strings.HasSuffix(got, "-q") {
		t.Errorf("cd -- -q left %q, want the directory of that name", got)
	}
}
