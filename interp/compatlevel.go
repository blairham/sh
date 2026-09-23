// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// CompatibilityLevel reads a value of the parameter named by
// Semantics.CompatibilityLevelVariable as a release, as a two-digit number —
// `5.1` and `51` are both 51 — and false where the text is not a release this
// shell would recognize.
//
// The spelling is exactly `NN` or `N.N` and the dot means nothing beyond
// legibility, which is a removal rather than a decimal reading: `5.10` would
// be three digits and is not a release. It is narrower than stripping every
// dot, and the difference is measured rather than tidy — on bash 5.3.20 under
// `LC_ALL=C`, `.44`, `44.`, `044`, ` 44` and `4.10` are each out of range
// where `4.4` is level 44.
//
// The **floor** is here because it is a statement about the idea rather than
// about one shell: below the oldest release that has a compatibility reading
// at all there is nothing to be compatible with. The **ceiling** is not here,
// because it is the reading shell's own release — a fact a dialect holds and
// the substrate must not name. Leaving it out changes no reading: a level
// above every threshold a row can carry answers the modern way, which is what
// an out-of-range value does in the shell that has the parameter. The
// complaint is the dialect's too; see dialect/bash/compat.go.
//
// Exported so that the complaint and the reading cannot come apart. A dialect
// deciding whether to complain about a value is asking this same question,
// and a second copy of these rules is how one of them would go on accepting a
// spelling the other had stopped taking.
func CompatibilityLevel(value string) (int, bool) {
	var digits string
	switch {
	case len(value) == 2:
		digits = value
	case len(value) == 3 && value[1] == '.':
		digits = value[:1] + value[2:]
	default:
		return 0, false
	}
	n := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	// Measured: `0` and `30` are out of range and take the modern answer.
	if n < CompatibilityFloor {
		return 0, false
	}
	return n, true
}

// CompatibilityFloor is the oldest release a compatibility level can name.
//
// A dialect needs it to say where the range it complains outside of starts,
// and the reading above needs it to refuse a number below it; one constant so
// the two ends of the range are not written down twice.
const CompatibilityFloor = 31

// compatLevel is the compatibility release this shell is being asked to
// behave as, and false where the dialect has no such parameter, the parameter
// is unset, or its value is not a release this shell would recognize.
//
// A parameter read where it is used rather than a field settled at startup,
// because it is an ordinary variable a script assigns mid-run: `BASH_COMPAT=51`
// takes effect on the next line, and unsetting it takes the modern reading
// back. Measured 2026-09-22 on bash 5.3.20 under `LC_ALL=C`.
func (r *Runner) compatLevel() (int, bool) {
	name := r.sem().CompatibilityLevelVariable
	if name == "" {
		return 0, false
	}
	v, ok := r.getVar(name)
	if !ok {
		return 0, false
	}
	return CompatibilityLevel(v)
}

// compatAtMost reports whether this shell is being asked to behave as the
// given release or older, which is the shape every compatibility row takes:
// a behavior changed *in* a release, so the old reading applies at that
// release's level and below.
func (r *Runner) compatAtMost(release int) bool {
	n, ok := r.compatLevel()
	return ok && n <= release
}
