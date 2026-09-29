// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `-p` beside an attribute letter and **no operand**: the listing narrowed to
// the names that letter selects, written in `-p`'s own form.
//
// One helper rather than a copy in each word's loop, because the two words that
// reach it have to give the same answer and a second copy is how one of them
// stops doing so — `typeset -pT` and `local -pT` write the same table in the
// reference, and the table is `declare -p`'s rather than either word's own.
//
// The second result is whether this line *was* that shape, so a caller can fall
// through to its own listing when it was not.
//
// Only with no operand. Operands are a selection and not a filter — `declare -pi
// name` writes the name whether or not it carries the letter — so the operand
// form belongs to declarePrint. See Runner.attributeFilter for the letters and
// for the three readings of a line that writes two of them.
func (r *Runner) filteredPrintListing(f declareFlags) (int, bool) {
	if !f.print || !f.attributeLetterWritten() {
		return 0, false
	}
	keep, answered := r.attributeFilter(f)
	if !answered {
		return r.status, true
	}
	if keep == nil {
		// A letter this dialect spells and this engine records nothing for.
		// It is still an attribute to select on and no name carries it, so
		// the listing is empty rather than whole — the same answer
		// declarationListing gives.
		return 0, true
	}
	return r.declarePrintFiltered(nil, r.sem().DeclareListing, true, keep, true), true
}
