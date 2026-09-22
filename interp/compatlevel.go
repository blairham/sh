// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// compatLevel is the compatibility release this shell is being asked to
// behave as, as a two-digit number — 5.1 and 51 are both 51 — and false where
// the dialect has no such parameter, the parameter is unset, or its value is
// not a release this shell would recognize.
//
// A parameter read where it is used rather than a field settled at startup,
// because it is an ordinary variable a script assigns mid-run: `BASH_COMPAT=51`
// takes effect on the next line, and unsetting it takes the modern reading
// back. Measured 2026-09-22 on bash 5.3.20 under `LC_ALL=C`.
//
// The range bash accepts runs from its oldest supported release to its own,
// and a value outside it is complained about and leaves the level unset. The
// complaint is not written here — see #4262, which also covers the `shopt`
// letters that are the same state under a second spelling. What this function
// gets right is the reading, which is what a script's behavior turns on: an
// out-of-range value takes the modern answer in both shells.
func (r *Runner) compatLevel() (int, bool) {
	name := r.sem().CompatibilityLevelVariable
	if name == "" {
		return 0, false
	}
	v, ok := r.getVar(name)
	if !ok || v == "" {
		return 0, false
	}
	// The dot is optional and means nothing beyond legibility: `5.1` and `51`
	// are one level, and that is a removal rather than a decimal reading —
	// `5.10` would be three digits and is not a release.
	digits := strings.ReplaceAll(v, ".", "")
	if len(digits) != 2 {
		return 0, false
	}
	n := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	// The floor is the oldest release with a compatibility reading at all;
	// below it there is nothing to be compatible with. Measured: `0` is out
	// of range and takes the modern answer.
	if n < 31 {
		return 0, false
	}
	return n, true
}

// compatAtMost reports whether this shell is being asked to behave as the
// given release or older, which is the shape every compatibility row takes:
// a behavior changed *in* a release, so the old reading applies at that
// release's level and below.
func (r *Runner) compatAtMost(release int) bool {
	n, ok := r.compatLevel()
	return ok && n <= release
}
