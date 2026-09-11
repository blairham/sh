// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A replacement scans the value for somewhere else to match, and a match that
// ended at the end of the value leaves nowhere.
//
// The empty match waiting at the final stop is not a second match, which
// matters only for a pattern that can match empty at all: `${v//*/X}` on a
// non-empty value is one X in every shell in the panel and was two here — the
// `*` took the whole value and then the position it stopped at was matched
// again (#1341). The neighbours below are the control: they say the operator
// still replaces everywhere it should, and that a pattern which cannot reach
// the end is untouched by this.
func TestAMatchEndingAtTheValuesEndIsTheLastOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a star takes the value once", `v=abcd; printf "[%s]" "${v//*/X}"`, "[X]"},
		{"a star after a literal", `v=abcd; printf "[%s]" "${v//b*/X}"`, "[aX]"},
		{"a star reaching the end from the middle", `v=abcd; printf "[%s]" "${v//c*/X}"`, "[abX]"},
		// The pattern matches everywhere and never reaches the end, so every
		// unit is replaced and the count is the length: a rule that stopped
		// at the first match would show up here.
		{"one unit at a time", `v=abcd; printf "[%s]" "${v//?/X}"`, "[XXXX]"},
		{"every occurrence of a literal", `v=abcabc; printf "[%s]" "${v//b/X}"`, "[aXcaXc]"},
		// The last occurrence ends at the end of the value, and it is still
		// replaced — the rule ends the scan after that match rather than
		// declining it.
		{"a literal at the very end", `v=abcb; printf "[%s]" "${v//b/X}"`, "[aXcX]"},
		{"a star that empties the value", `v=abcd; printf "[%s]" "${v//*/}"`, "[]"},
		// A star over an empty value has nothing before the final stop, so
		// the one match there is the only one and is taken.
		{"a star over an empty value", `v=; printf "[%s]" "${v//*/X}"`, "[X]"},
		// The single-replacement and anchored spellings share the scan and
		// must be unmoved by a rule about the end of it.
		{"the single spelling", `v=abcd; printf "[%s]" "${v/*/X}"`, "[X]"},
		{"anchored at the front", `v=abcd; printf "[%s]" "${v/#*/X}"`, "[X]"},
		{"anchored at the end", `v=abcd; printf "[%s]" "${v/%*/X}"`, "[X]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, patternGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
