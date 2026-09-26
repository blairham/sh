// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// What a shell was called is the front end's fact, and this is the fifth
// question read off that one word — after `$0`, the alias route, login-ness
// and PosixNamed.
//
// It is *not* PosixNamed under another name. That one is the core's: it reads
// the exact word `sh` in every dialect and starts the mode the standard
// describes, and the spec's reasoning for reading no more than that word
// stands — taking one shell's extras there would put a shell called `bash`
// into POSIX mode. This is one shell's own convention for its own binary,
// which is why the letters arrive from the dialect rather than being written
// here, and why the front end knows nothing about what a mode means: it reads
// a name, gets a word back, and hands that word to the builtin that already
// implements `--emulate`.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), by copying
// the reference binary under each name and running it — `go version -m` says
// *not a Go executable* for it. A symlink and a copy answer alike, which is
// the control on the instrument: both `./ln/sh` and a copied `./sh` report
// `emulate` → `sh` and both answer `cd` with no HOME at status 1.
//
//	sh shell shx sx s b bash bsh   ->  sh
//	k ksh                          ->  ksh
//	c csh                          ->  csh
//	zsh xsh ash dash fish mysh SH  ->  zsh
//
// The rule is keyed on the **first letter** and on nothing else. Two rows
// hold that noun fixed and vary everything around it: `b` enters sh emulation
// with no `s` in the word at all, and `xsh` does not enter it while
// containing one — so "the basename is `sh`" and "the basename begins with
// `sh`" are both wrong, and they are wrong on exactly the rows a grid of
// sensible names would never reach. `SH` is the third: the letter is read
// case-sensitively, so the same word in the other case is zsh.

// EmulationNamed reports the emulation argv[0] asks this shell to start in,
// or the empty string where the name says nothing.
//
// The name is argv[0]'s **basename**, with one leading dash removed and then
// one leading EmulationOption.NameDropsInitial letter; the first letter of
// what is left is looked up in EmulationOption.NameInitials.
//
// The basename comes first here and last in PosixNamed, which is measured
// rather than tidied: `./d/-sh` is sh emulation in zsh — the path is dropped
// and the dash then strips — while the same word is not `sh` to bash, which
// strips the dash from the whole of argv[0] and then finds `-sh` at the end
// of it. Both shells agree on `-x/sh`, which is why one reading looked like
// it would do for both.
func EmulationNamed(argv []string, e interp.EmulationOption) string {
	if len(argv) == 0 || e.NameInitials == "" {
		return ""
	}
	name := argv[0]
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimPrefix(name, "-")
	if e.NameDropsInitial != "" && name != "" && strings.IndexByte(e.NameDropsInitial, name[0]) >= 0 {
		name = name[1:]
	}
	if name == "" {
		return ""
	}
	for _, pair := range strings.Fields(e.NameInitials) {
		letter, mode, ok := strings.Cut(pair, "=")
		if !ok || len(letter) != 1 || mode == "" {
			continue
		}
		if letter[0] == name[0] {
			return mode
		}
	}
	return ""
}

// emulationFromName folds the name's mode into the invocation, so that the
// one call that applies an emulation applies this one too.
//
// Only where `--emulate` did not already say, and that is measured in both
// directions rather than assumed from precedence: with the reference copied to
// a file called `sh`, `--emulate zsh` gives a shell that reports `zsh` and
// answers `cd` with no HOME at 0, and under its own name `--emulate sh` gives
// one that reports `sh` and answers 1. The option wins outright and does not
// merely add to the name.
//
// Outright is the word. `--emulate fish` — a mode no shell knows, which the
// builtin passes over in silence — leaves a binary called `sh` reporting
// `zsh`, so the option does not fall back to the name when its word means
// nothing. Writing the option at all is what puts the name aside, which is
// what leaving in.emulating alone here does.
func (sh Shell) emulationFromName(argv []string, in *source) {
	if in.emulating {
		return
	}
	if mode := EmulationNamed(argv, sh.Semantics.EmulationOption); mode != "" {
		in.emulation, in.emulating = mode, true
	}
}
