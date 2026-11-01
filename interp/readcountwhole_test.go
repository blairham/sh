// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestReadCountFillsTheFirstNameWhole: both answers of the axis over the same
// lines. Yes hands the counted text to the first name whole and leaves the
// rest; No splits it as an uncounted read would.
func TestReadCountFillsTheFirstNameWhole(t *testing.T) {
	t.Parallel()
	const src = `y=keep; read -n 3 x y <<< 'p q rs'; echo "[$x] [$y]"; read -n 4 z <<< '  pq'; echo "[$z]"`
	for _, c := range []struct {
		a    Answer
		want string
	}{
		{Yes, "[p q] [keep]\n[  pq]\n"},
		{No, "[p] [q]\n[pq]\n"},
	} {
		s := testSemantics()
		s.ReadCountFillsTheFirstNameWhole = c.a
		out, st := run(t, src, withSem(s))
		if out != c.want || st != 0 {
			t.Errorf("answered %v: got %q status %d, want %q", c.a, out, st, c.want)
		}
	}
	t.Run("an uncounted read never asks", func(t *testing.T) {
		s := testSemantics()
		s.ReadCountFillsTheFirstNameWhole = Unspecified
		if out, st := run(t, `read x y <<< 'p q'; echo "[$x] [$y]"`, withSem(s)); out != "[p] [q]\n" || st != 0 {
			t.Errorf("got %q status %d", out, st)
		}
	})
}
