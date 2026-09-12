// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The order this shell publishes the letters of `$-` in, and it is the reason
// the axis is a sequence rather than a discipline: this one is neither
// sorted, nor the order the script set the options in, nor capitals apart.
//
// Measured 2026-09-12, the `s` rows by feeding the program on standard input
// and the last through a pseudo-terminal:
//
//	set -f; set -u; set -e              ufe
//	set -a -b -C -e -f -u -v -E -I      ubaCEvIfe
//	set -x -a -C -e -u -v               uaCvxe
//	set -C -e, on stdin                 Cse
//	set -x -C -v -u -a -f -e, on stdin  uaCvxsife
//	-i -c, at a pseudo-terminal         mi
//
// `n` is the one letter here nobody can measure directly: the option it
// stands for stops the `echo` that would read `$-`. Its place is the one the
// rest implies — this shell's `set -o` listing runs `errexit noglob ignoreeof
// interactive monitor noexec stdin xtrace verbose vi emacs noclobber
// allexport notify nounset`, which is this string reversed, and `noexec` sits
// between `stdin` and `interactive` there. The `mi` row is the check on that
// reversal, and nothing else in the panel puts those two that way round.
func TestDollarDashLetterOrder(t *testing.T) {
	if got, want := dash.Semantics().DollarDashLetterOrder, "ubaCEVvxsnmiIfe"; got != want {
		t.Errorf("DollarDashLetterOrder = %q, want %q", got, want)
	}
}

// And the order as this shell writes it. There are no startup letters here —
// the one shell in the panel whose `$-` begins empty — so every letter in
// these rows is one the script set.
func TestDollarDashIsWrittenInThatOrder(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -f; set -u; set -e; echo "[$-]"`, "[ufe]"},
		{`set -C; set -e; echo "[$-]"`, "[Ce]"},
		{`set -a; set -C; set -u; set -v; echo "[$-]"`, "[uaCv]"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
		}
	}
}
