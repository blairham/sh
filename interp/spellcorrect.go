// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path/filepath"
	"strings"
)

// Correcting a misspelled path, one component at a time.
//
// bash's `cdspell` is the name over this, and `dirspell` is the same
// correction asked for by the completer instead of by `cd`. They are one
// algorithm and this is the only copy of it: a second corrector written for
// whichever of the two was built later is the failure this repository has paid
// for five times, so the capability is here and the two names are the
// dialect's.
//
// Everything below is measured against bash 5.3.15 on 2026-09-08, at a
// prompt, since the name is interactive-only there and a `-c` probe shows both
// shells doing nothing.

// withinOneEdit reports whether two names are one edit apart: one insertion,
// one deletion, one substitution or one transposition of adjacent characters.
//
// Exactly one, and the threshold is flat rather than scaled by length, which
// is the measurement and not a simplification. Against a directory holding
// `documents`, bash corrects `documnets`, `documets`, `docments`, `dcouments`
// and `documentss` — one transposition, two different deletions, another
// transposition and one insertion — and refuses `doucmnets`, which is two
// transpositions. Nine characters is long enough that a length-scaled budget
// would have allowed the second, so there is no scaling.
//
// Written as an explicit walk rather than as a distance matrix because the
// question is not "how far apart" but "is it one", and the answer is decidable
// in a single pass. A matrix would compute a number nothing reads.
//
// Runes rather than bytes: a name is what a person typed, and a mistyped
// accented letter is one edit and not two.
func withinOneEdit(typed, actual string) bool {
	if typed == actual {
		return false // Not a correction. The caller only asks about names that did not resolve.
	}
	a, b := []rune(typed), []rune(actual)
	switch len(a) - len(b) {
	case 0:
		return oneSubstitution(a, b) || oneTransposition(a, b)
	case 1:
		return oneDeletion(a, b)
	case -1:
		return oneDeletion(b, a)
	}
	return false
}

// oneSubstitution reports whether two equal-length names differ in one place.
func oneSubstitution(a, b []rune) bool {
	diff := 0
	for i := range a {
		if a[i] != b[i] {
			diff++
			if diff > 1 {
				return false
			}
		}
	}
	return diff == 1
}

// oneTransposition reports whether two equal-length names differ by swapping
// one adjacent pair — the mistake a person actually makes when typing fast,
// and the one an edit distance without it scores as two.
func oneTransposition(a, b []rune) bool {
	i := 0
	for i < len(a) && a[i] == b[i] {
		i++
	}
	if i+1 >= len(a) || a[i] != b[i+1] || a[i+1] != b[i] {
		return false
	}
	for j := i + 2; j < len(a); j++ {
		if a[j] != b[j] {
			return false
		}
	}
	return true
}

// oneDeletion reports whether dropping one rune from the longer name gives the
// shorter one. It is called both ways round, which is what makes it answer for
// an insertion as well.
func oneDeletion(long, short []rune) bool {
	i := 0
	for i < len(short) && long[i] == short[i] {
		i++
	}
	for j := i; j < len(short); j++ {
		if long[j+1] != short[j] {
			return false
		}
	}
	return true
}

// closestName is the entry of dir that the typed name is one edit from, or
// empty where none is or where the directory cannot be read.
//
// The first such entry wins, and "first" is by name because readDir sorts.
// bash takes whatever its readdir hands back first, which on the measured run
// was the *older* of two equally close candidates — an order no program can
// predict and the same shell will not reproduce after the directory is
// rewritten. Ours is deterministic instead, and the two agree wherever exactly
// one entry is within an edit, which is every case a person actually hits.
// Measured: with `zzz1` and `zzz2` both one insertion from `zzz`, bash chose
// `zzz2` and this chooses `zzz1`.
func (r *Runner) closestName(dir, typed string) string {
	entries, err := r.readDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if withinOneEdit(typed, e.Name()) {
			return e.Name()
		}
	}
	return ""
}

// correctPath is the operand with each component that does not exist replaced
// by the closest name that does, or empty where it cannot be corrected.
//
// The *operand*, not the resolved path, because what a correction produces is
// something to show a person: bash prints `alpha/beta` for `alpah/beta`,
// `/tmp/w/alpha` for `/tmp/w/alpah` and `documents/` for `documnets/`, so the
// shape it was given — relative or absolute, with or without a trailing slash
// — is the shape it comes back in.
//
// Greedy and without backtracking, which is measured rather than chosen. In a
// tree holding `alpha/beta/gamma` and `alpha/betta`, bash corrects
// `alpha/bteta` to `alpha/betta` and then fails `alpha/bteta/gamma` outright
// — having committed to `betta`, it does not go back and try `beta`, even
// though `beta/gamma` exists and would have resolved. So a component is
// corrected once, against what the components before it already decided.
//
// `.` and `..` are left alone: they resolve everywhere and are not names in a
// directory, so a corrector that treated them as candidates would be able to
// "correct" `..` into some entry that happened to be two characters long.
func (r *Runner) correctPath(operand string) string {
	if operand == "" {
		return ""
	}
	parts := strings.Split(operand, "/")
	// The walk needs somewhere to look each component up, and that is this
	// runner's directory for a relative operand and the root for an absolute
	// one — the same resolution `cd` itself does, so a correction is never
	// looked for anywhere `cd` would not have gone.
	at := r.Dir
	if strings.HasPrefix(operand, "/") {
		at = "/"
	}
	corrected := false
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			// An empty part is the leading slash or a trailing one; both are
			// separators rather than names.
			at = filepath.Join(at, part)
			continue
		}
		if _, err := r.stat(filepath.Join(at, part)); err == nil {
			at = filepath.Join(at, part)
			continue
		}
		match := r.closestName(at, part)
		if match == "" {
			return ""
		}
		parts[i] = match
		corrected = true
		at = filepath.Join(at, match)
	}
	if !corrected {
		// Nothing was wrong with the spelling, so whatever stopped `cd` is
		// not something this can fix — a permission, a name that is a file,
		// a symlink that leads nowhere. Answering with the operand unchanged
		// would make the caller print a "correction" that corrected nothing.
		return ""
	}
	return strings.Join(parts, "/")
}

// cdCorrected is `cd`'s second attempt at an operand that did not resolve: the
// corrected operand to show, and where it leads.
//
// Only where the shell has said it wants this and only at a prompt. Both are
// measured — `bash -c 'shopt -s cdspell; cd subdri'` refuses with the ordinary
// diagnostic, so a shell that corrected a script's `cd` would be doing
// something bash does not.
//
// The same resolution the first attempt made, on the corrected text: joined
// against the directory `cd` started in, resolved physically under `-P`, and
// required to be a directory. Anything less would let a correction that landed
// on a *file* report success and then fail to move.
func (r *Runner) cdCorrected(operand, from string, physical bool) (dir, shown string, ok bool) {
	if !r.cdCorrectsSpelling || !r.Interactive || operand == "" {
		return "", "", false
	}
	shown = r.correctPath(operand)
	if shown == "" {
		return "", "", false
	}
	dir = shown
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(from, dir)
	}
	if physical {
		if resolved, err := r.physicalPath(dir); err == nil {
			dir = resolved
		}
	}
	info, err := r.stat(dir)
	if err != nil || !info.IsDir() {
		return "", "", false
	}
	return dir, shown, true
}
