// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// **The termcap `me` is `sgr` with everything off and the character set taken
// out, where that is the reset `sgr0` is** (#5289). Each row is one entry's
// `sgr0`, `sgr` and `rmacs` as zsh 5.9.2 reads them from its database, and the
// `$termcap[me]` it answers, measured 2026-10-01 with `${(qqqq)…}`. screen and
// linux share `sgr0` and `rmacs` and part on `sgr` alone; vt220's padded
// `rmacs` is in neither string; and an entry with no `sgr` keeps `sgr0`.
func TestTheTermcapExitAttributes(t *testing.T) {
	for _, c := range []struct{ name, sgr0, sgr, rmacs, want string }{
		{"xterm", "\x1b(B\x1b[m", "%?%p9%t\x1b(0%e\x1b(B%;\x1b[0%?%p6%t;1%;%?%p5%t;2%;%?%p2%t;4%;%?%p1%p3%|%t;7%;%?%p4%t;5%;%?%p7%t;8%;m", "\x1b(B", "\x1b[0m"},
		{"screen", "\x1b[m\x0f", "\x1b[0%?%p6%t;1%;%?%p1%t;3%;%?%p2%t;4%;%?%p3%t;7%;%?%p4%t;5%;%?%p5%t;2%;m%?%p9%t\x0e%e\x0f%;", "\x0f", "\x1b[0m"},
		{"vt100", "\x1b[m\x0f$<2>", "\x1b[0%?%p1%p6%|%t;1%;%?%p2%t;4%;%?%p1%p3%|%t;7%;%?%p4%t;5%;m%?%p9%t\x0e%e\x0f%;$<2>", "\x0f", "\x1b[0m"},
		{"vt220", "\x1b[m\x1b(B", "\x1b[0%?%p6%t;1%;%?%p2%t;4%;%?%p4%t;5%;%?%p1%p3%|%t;7%;m%?%p9%t\x1b(0%e\x1b(B%;$<2>", "\x1b(B$<4>", "\x1b[0m\x1b(B"},
		{"ansi", "\x1b[0;10m", "\x1b[0;10%?%p1%t;7%;%?%p2%t;4%;%?%p3%t;7%;%?%p4%t;5%;%?%p6%t;1%;%?%p7%t;8%;%?%p9%t;11%;m", "\x1b[10m", "\x1b[0m"},
		{"linux", "\x1b[m\x0f", "\x1b[0;10%?%p1%t;7%;%?%p2%t;4%;%?%p3%t;7%;%?%p4%t;5%;%?%p5%t;2%;%?%p6%t;1%;m%?%p9%t\x0e%e\x0f%;", "\x0f", "\x1b[m\x0f"},
		{"no sgr", "\x1b[m", "", "", "\x1b[m"},
	} {
		if got := termcapExitAttributes(c.sgr0, c.sgr, c.rmacs); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
