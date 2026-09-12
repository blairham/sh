// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The order this shell publishes the letters of `$-` in: `i` and `c` lead,
// the rest of the lowercase letters are sorted, then the uppercase ones, and
// `l` comes last of all.
//
// Measured on ksh93u+, 2026-09-12, the interactive rows through a
// pseudo-terminal with a scratch home directory:
//
//	set -f; set -u; set -e   cefhsuB
//	set -C                   chsBC
//	set -a                   cahsB
//	-l -c                    chsBl
//	-i -c                    icmsBE
//	-il -c                   icmsBEl
//	-i -Cc                   icmsBCE
//
// Two letters sit outside the sort, in opposite directions, which is why this
// is neither "sorted" nor "sorted with capitals last": `i` stands in front of
// a `c` it sorts after, and `l` stands behind capitals it sorts before.
func TestDollarDashLetterOrder(t *testing.T) {
	if got, want := ksh.Semantics().DollarDashLetterOrder, "icaefhmnstuvxBCEHTl"; got != want {
		t.Errorf("DollarDashLetterOrder = %q, want %q", got, want)
	}
}

// And the order as this shell writes it. The startup letters are `hB`; the
// route letters `c` and `s` come from Runner.Route and are not in play here.
func TestDollarDashIsWrittenInThatOrder(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -f; set -u; set -e; echo "[$-]"`, "[efhuB]"},
		{`set -C; echo "[$-]"`, "[hBC]"},
		{`set -C; set -e; echo "[$-]"`, "[ehBC]"},
		{`set -a; echo "[$-]"`, "[ahB]"},
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
