// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/blairham/sh/driver"
)

// ErrNameStartsAnotherShell is what a run is refused with when the name the
// graded binary is invoked under puts it in a mode the reference's own name
// does not put the reference in.
var ErrNameStartsAnotherShell = errors.New("the graded shell's name puts it in a mode the reference is not in")

// CheckInvocationName is the second precondition beside [CheckBaseNames], and
// it is about a different noun.
//
// CheckBaseNames asks whether the two base names are **equal**, because
// [normalize] replaces each shell's full path and never its base name, so a
// line where a shell names *itself* is compared literally. This asks what the
// name does to the shell *before the first line of a file is read*: argv[0]
// is an input to a front end, and two shells invoked under names that answer
// it differently are two shells in different modes — which is a defect the
// figures cannot show, because a mode is not a line.
//
// The noun is **the name a binary is invoked under**, not the path it lives
// at, and those differ at exactly one place: a harness that renames. That is
// what made #4674 invisible for as long as it lasted. The suite's container
// copied the graded binary in as `/suiteours`; zsh takes its emulation from
// the **first letter** of argv[0] — [driver.EmulationNamed], real zsh's own
// rule — and `s` is `sh`, so every containerized run of `cmd/zsh` had already
// run `emulate sh` while `/bin/zsh` beside it had not. Nothing was broken:
// the image was pinned by digest, both shells ran on one copy of the files,
// one scorer graded them, and the column still measured a shell in one mode
// against a shell in another. `zsh/arith.tests` took C's precedences for five
// rows and then refused `$(( 08 + 1 ))`, at 5/38 lines and status 1, and read
// as a defect list.
//
// Two questions are read off that one word today and both are asked, because
// the failure has the same shape whichever of them moves:
//
//	driver.PosixNamed       the exact word `sh`, in every dialect
//	driver.EmulationNamed   one shell's own convention, by first letter
//
// **The reference is the other side, not the column's dialect name.** What
// makes a comparison sound is that the two shells are in the same mode, and a
// column legitimately graded under a name they share — a shell against itself
// at `/bin/sh` — is not a confound. A dialect this package has no parts for is
// not refused either: a column with no front end of ours reads nothing off the
// name (see [dialectParts]).
func CheckInvocationName(s Suite, ours, reference string) error {
	_, sem, _, _, ok := dialectParts(s.Dialect)
	if !ok {
		return nil
	}
	// The **whole path** on both sides and never a base name, because that is
	// what argv[0] will be: the harness runs each shell by its path, so this
	// asks the front end its own questions with the front end's own input.
	// The two read the word differently and that is measured rather than
	// tidied — EmulationNamed drops the directory and then one dash,
	// PosixNamed drops one dash and then the directory, so `./d/-sh` is sh
	// emulation to zsh and is not the name `sh` to bash. Reducing either to a
	// base name here would answer a question neither front end asks. See
	// driver/emulationname.go.
	if got, want := driver.EmulationNamed([]string{ours}, sem.EmulationOption),
		driver.EmulationNamed([]string{reference}, sem.EmulationOption); got != want {
		return fmt.Errorf("%w: %q starts this shell in %s and %q starts it in %s",
			ErrNameStartsAnotherShell, filepath.Base(ours), emulationWord(got),
			filepath.Base(reference), emulationWord(want))
	}
	if driver.PosixNamed([]string{ours}) != driver.PosixNamed([]string{reference}) {
		return fmt.Errorf("%w: exactly one of %q and %q is the standard's own name for the "+
			"shell, so one of them starts in POSIX mode and the other does not",
			ErrNameStartsAnotherShell, filepath.Base(ours), filepath.Base(reference))
	}
	return nil
}

// emulationWord names the mode for the refusal, and names the absence of one
// as well: both sides of that comparison are printed, so "" has to be a word
// rather than a gap in the sentence.
func emulationWord(mode string) string {
	if mode == "" {
		return "no emulation"
	}
	return mode + " emulation"
}
