// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The other side of the axis, and the half that must not move: C ignores the
// `0` flag where a precision is written on an integer conversion, and this
// column complies. Measured 2026-09-17 against bash 5.3.20 under LC_ALL=C.
func TestTheZeroFlagIsIgnoredPastAPrecision(t *testing.T) {
	for _, tc := range []struct{ spec, arg, want string }{
		{"%+05.0d", "7", "   +7"},
		{"%#05.0o", "7", "   07"},
		{"%08.3d", "7", "     007"},
		{"%08.3x", "7", "     007"},
		{"%08.0d", "0", "        "},
		{"%08.10d", "7", "0000000007"},
	} {
		out, st := answersRun(t, "printf '[%"+"s]' \"$(printf '"+tc.spec+"' "+tc.arg+")\"")
		if st != 0 {
			t.Errorf("%s of %s: status %d: %q", tc.spec, tc.arg, st, out)
			continue
		}
		if want := "[" + tc.want + "]"; !strings.Contains(out, want) {
			t.Errorf("%s of %s: got %q, want it to contain %q", tc.spec, tc.arg, out, want)
		}
	}
}

// A width past what Go's `fmt` renders is laid out by this shell instead, and
// the two sides of that ceiling have to agree: a field with a precision on it
// is filled with blanks there as much as here. It filled with zeros, because
// the wide path read the `0` flag without C's rule about it (#3067).
func TestTheWideFieldObeysTheSameZeroFlagRule(t *testing.T) {
	out, st := answersRun(t, `printf '%010000020.3d' 7 | cut -c1-4`)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if got := strings.TrimRight(out, "\n"); got != "    " {
		t.Errorf("first four bytes = %q, want four blanks", got)
	}
}
