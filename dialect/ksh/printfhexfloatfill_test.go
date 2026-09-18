// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A `%a`'s zero fill goes in *front* of the `0x` here, where the two columns
// that also have the conversion put it between the prefix and the digits. It
// is a placement and not an arithmetic — both produce a field exactly as wide
// as the width asked for — which is what parts it from the alternate-prefix
// axis, and the same shell answers the two differently.
//
// Measured 2026-09-17 against ksh93u+ 2012-08-01 under LC_ALL=C: `%030a` of
// 1.5 is eleven zeros and then this shell's whole nineteen-character
// `0x1.800000000000p+0`, which the twelve-digit default gives it (#3089).
func TestAHexFloatZeroFillGoesInFrontOfThePrefix(t *testing.T) {
	out, st := answersRun(t, `printf '[%030a]' 1.5`)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "[000000000000x1.800000000000p+0]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	// The controls: `-` and a space fill are unanimous, so the question is
	// the `0` flag's and not the width's.
	out, _ = answersRun(t, `printf '[%-25a][%25a]' 1.5 1.5`)
	for _, want := range []string{"[0x1.800000000000p+0      ]", "[      0x1.800000000000p+0]"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
	}
}

// And past the width Go's `fmt` renders the answer is the same, because the
// conversion lays out its own field: the ceiling took the width away and the
// fill then landed in front of the prefix on one side and inside it on the
// other, which is two answers to one question.
func TestAWideHexFloatIsLaidOutTheSameWay(t *testing.T) {
	out, st := answersRun(t, "printf '%010000020a' 1.5 | { head -c 4; }; echo")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if got := strings.TrimRight(out, "\n"); got != "0000" {
		t.Errorf("first bytes %q, want %q", got, "0000")
	}
	out, _ = answersRun(t, "printf '%010000020a' 1.5 | wc -c")
	if got := strings.TrimSpace(out); got != "10000020" {
		t.Errorf("%s bytes, want 10000020", got)
	}
}
