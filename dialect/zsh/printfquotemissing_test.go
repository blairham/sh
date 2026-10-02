// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `%q` with no operand left writes nothing here, where an empty operand is
// quoted. See interp.Semantics.PrintfQuoteOfNoArgumentIsEmpty.
//
// Measured 2026-10-02 on zsh 5.9.2 (#5153).
func TestPrintfQuoteOfNoArgumentIsEmpty(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`printf '[%q]\n'`, "[]\n"},
		{`printf '[%q]\n' ''`, "['']\n"},
		{`printf '[%q] [%q]\n' a`, "[a] []\n"},
		{`printf '%q%q\n' 你你`, "你你\n"},
	} {
		out, _, errs := runZshUTF8(t, c.src)
		if out != c.want || errs != "" {
			t.Errorf("%s: got %q, %q, want %q", c.src, out, errs, c.want)
		}
	}
}
