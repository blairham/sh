// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The `set -o` listing happens **once**, at the end of the option parse, in
// the form the last `-o` or `+o` decided — and a `-o` that declined its word
// counts as a `+o`. Measured on ksh93u+ 2012-08-01, 2026-09-13 and again
// 2026-09-15; every other column lists where it stands, in the sign's own
// form (#2698).
//
// Two things the immediate reading cannot express, and both have a row here.
// It is **deferred**: `set -o -e -o` lists once with `errexit on` already in
// the listing, so the listing is written after the whole parse. And it is
// **once**: `set +o -o` is one listing where bash writes two.
func TestTheOptionListingHappensOnceAtTheEndOfTheParse(t *testing.T) {
	// The two forms, told apart by their first line. The re-input line is a
	// `set` command a script could feed back; the table has a header.
	const (
		table = "Current option settings"
		input = "set --default"
	)
	for _, tc := range []struct {
		name, src, want string
		// listings counts how many times the wanted form appears, because
		// "once" is half of what is being pinned.
		listings int
	}{
		{name: "a bare -o is the table", src: "set -o", want: table, listings: 1},
		{name: "a bare +o is the re-input line", src: "set +o", want: input, listings: 1},
		{
			// The `-o` declined `-e`, so it counts as a `+o` — and the
			// option it declined the word for is already in the listing.
			name: "a declined word makes it the re-input form",
			src:  "set -o -e", want: input, listings: 1,
		},
		{name: "and `--` is a declined word too", src: "set -o --", want: input, listings: 1},
		{
			name: "which still leaves the operands behind it",
			src:  `set -o -- a b; echo "n=$#"`, want: input, listings: 1,
		},
		{
			// Last one wins, and there is still only one listing.
			name: "the last sign decides: a declined -o then a +o",
			src:  "set -o +o", want: input, listings: 1,
		},
		{
			name: "a declined +o then a bare -o is the table, once",
			src:  "set +o -o", want: table, listings: 1,
		},
		{
			name: "and so is a declined -o then a bare one",
			src:  "set -o -e -o", want: table, listings: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0: %q", st, out)
			}
			if got := strings.Count(out, tc.want); got != tc.listings {
				t.Errorf("%q appeared %d times, want %d: %q", tc.want, got, tc.listings, out)
			}
			// And the other form never appears beside it, which is what
			// "one listing" means rather than "at least one".
			other := input
			if tc.want == input {
				other = table
			}
			if strings.Contains(out, other) {
				t.Errorf("both forms were written: %q", out)
			}
		})
	}
}

// Deferred means the listing sees the options the *whole* parse left, not the
// ones standing when the `-o` was read.
func TestTheDeferredListingHoldsWhatTheWholeParseLeft(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, "set -o -e -o")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "errexit                  on") {
		t.Errorf("the listing does not hold `errexit on`: %q", out)
	}
}

// And a parse that ended early writes no listing at all: the deferral is not
// a delay, it is a listing that never happens.
func TestAParseThatFailedWritesNoListing(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, "set -o -Z")
	if err != nil {
		t.Fatal(err)
	}
	if st == 0 {
		t.Errorf("status = 0, want a refusal: %q", out)
	}
	if strings.Contains(out, "Current option settings") || strings.Contains(out, "set --default") {
		t.Errorf("a failed parse still listed: %q", out)
	}
	if !strings.Contains(out, "-Z: unknown option") {
		t.Errorf("out = %q, want the unknown-option refusal", out)
	}
}
