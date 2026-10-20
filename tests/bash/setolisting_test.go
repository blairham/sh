// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The other side of #2698, and the reason it is an axis rather than a
// correction: here the listing happens **where it stands**, in the sign's own
// form. `set -o -o` is two listings and `set +o -o` is one of each — measured
// on bash 5.3.15, 2026-09-15 — where ksh93 writes one listing, after the
// whole parse, in the form the last of them decided.
//
// Asserted rather than left to the zero value, because a shell that quietly
// started deferring would still print a listing and would still be green on
// every row about what is *in* one.
func TestTheOptionListingHappensWhereItStands(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		minus, plus int
	}{
		{name: "two bare -o are two listings", src: "set -o -o", minus: 2},
		{name: "one of each is one of each", src: "set +o -o", minus: 1, plus: 1},
		{name: "a bare -o on its own", src: "set -o", minus: 1},
		{name: "a bare +o on its own", src: "set +o", plus: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0: %q", st, out)
			}
			// The two forms here are `option<tab>on|off` rows and
			// re-inputtable `set -o name` lines, so one line of each is
			// enough to count listings.
			if got := strings.Count(out, "\nbraceexpand"); got != tc.minus {
				t.Errorf("two-column listings = %d, want %d: %q", got, tc.minus, out)
			}
			if got := strings.Count(out, "set -o braceexpand"); got != tc.plus {
				t.Errorf("re-input listings = %d, want %d: %q", got, tc.plus, out)
			}
		})
	}
}
