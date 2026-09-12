// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The order this shell publishes the letters of `$-` in: the lowercase ones
// sorted, then the uppercase ones, then the letter naming the route it was
// invoked by.
//
// Measured on bash 5.3.15, 2026-09-12, and bash 3.2.57 and bash-as-`sh` agree
// on every row:
//
//	set -f; set -u; set -e            efhuBc
//	set -C                            hBCc
//	set -C -e                         ehBCc
//	set -a                            ahBc
//	set -e -C, program on stdin       ehBCs
//	-i -c, at a pseudo-terminal       himBHc
//
// The last two are what say the trailing letter belongs to the *route* and
// not to `c` in particular: `i` and `m` sort in with the lowercase letters
// where `s` does not. Which of `c` and `s` leads is not measurable here,
// because this shell never shows both — see CommandStringShowsSInDollarDash.
func TestDollarDashLetterOrder(t *testing.T) {
	if got, want := bash.Semantics().DollarDashLetterOrder, "aefhilmntuvxBCEHTcs"; got != want {
		t.Errorf("DollarDashLetterOrder = %q, want %q", got, want)
	}
}

// And the order as this shell writes it, rather than as a string in a field.
// The startup letters are `hB`, so each row is a letter the script set
// finding its place among letters the shell already held.
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
