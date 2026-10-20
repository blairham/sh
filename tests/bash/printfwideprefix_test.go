// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strconv"
	"strings"
	"testing"
)

// A width past what Go's `fmt` renders is laid out by this shell, and the two
// sides of that ceiling have to agree about where a zero fill goes. Past it
// the width was taken out of the spec, the bare `0x7` came back, and the fill
// landed in *front* of the prefix — which is neither reading of the
// alternate-prefix axis, since both of those put it between the prefix and
// the digits.
//
// Measured 2026-09-17 against bash 5.3.20 under LC_ALL=C: `%#010000020x` of 7
// is `0x` and then ten million zeros, 10000020 bytes, and `%#010000009x` —
// one below the ceiling, so it never took the wide path — is the same shape
// (#3089).
func TestAWideAlternatePrefixKeepsItsFillInside(t *testing.T) {
	for _, tc := range []struct {
		spec, want string
		bytes      int
	}{
		{"%#010000020x", "0x000000", 10000020},
		{"%#010000009x", "0x000000", 10000009},
		{"%#010000020X", "0X000000", 10000020},
	} {
		out, st := answersRun(t, "printf '"+tc.spec+"' 7 | { head -c 8; } ; echo")
		if st != 0 {
			t.Errorf("%s: status %d: %q", tc.spec, st, out)
			continue
		}
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s: first bytes %q, want %q", tc.spec, got, tc.want)
		}
		out, _ = answersRun(t, "printf '"+tc.spec+"' 7 | wc -c")
		if got := strings.TrimSpace(out); got != strconv.Itoa(tc.bytes) {
			t.Errorf("%s: %s bytes, want %d", tc.spec, got, tc.bytes)
		}
	}
}

// And the same question of `%a`, whose renderer lays out its own field: the
// width never reaches `fmt` there at all, so the ceiling must not take it
// away. Measured the same day: `%010000020a` of 1.5 is `0x` then the fill in
// bash 5.3.20, and `%030a` — far below the ceiling — is the same shape.
func TestAWideHexFloatKeepsItsFillInside(t *testing.T) {
	out, st := answersRun(t, `printf '[%030a]' 1.5`)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "[0x00000000000000000000001.8p+0]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	out, _ = answersRun(t, "printf '%010000020a' 1.5 | { head -c 4; }; echo")
	if got := strings.TrimRight(out, "\n"); got != "0x00" {
		t.Errorf("first bytes %q, want %q", got, "0x00")
	}
}
