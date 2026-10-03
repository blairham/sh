// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// indexedSubscriptKeepsItsQuoting reports whether an indexed array's
// subscript, written with a quote or a backslash in it, is handed to the
// arithmetic the way the inside of `$(( ))` is: expanded as a double-quoted
// string, so an apostrophe and a backslash before an ordinary character stay
// characters of the expression. See
// Semantics.IndexedSubscriptKeepsItsQuoting.
//
// Asked only where the written subscript holds one of those characters and
// the name is not an associative array — an indexed one, a scalar, or one not
// set yet, which a store makes indexed — so the common path asks nothing: a
// subscript with no quoting in it comes to the same text either way.
func (r *Runner) indexedSubscriptKeepsItsQuoting(written, name string) bool {
	if name == "" || !strings.ContainsAny(written, `'\`) {
		return false
	}
	if _, assoc := r.assocFor(r.throughNameref(name)); assoc {
		// A key, which is SubscriptIsAQuotingContext's question.
		return false
	}
	return r.ask(r.sem().IndexedSubscriptKeepsItsQuoting,
		"an indexed array's subscript keeping its quoting for the arithmetic")
}
