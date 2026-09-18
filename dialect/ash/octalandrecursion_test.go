// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"fmt"
	"strings"
	"testing"
)

// Two answers this column holds alone, measured 2026-09-18 inside the pinned
// image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with stdin `/dev/null`, against `cmd/ash` cross-compiled into the same
// container (#3415, #3416).

// An octal escape past a byte is the first two digits' value here, where every
// other column with the construct keeps the low byte. The third digit is read
// and thrown away, so a fourth digit is still text.
func TestAnOctalEscapePastAByteIsTheFirstTwoDigits(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The controls first: a value that fits, and a run whose fourth digit
		// was never part of the escape.
		{`$'\377'`, "\xff"},
		{`$'\1234'`, "S4"},
		// And the column's own answers.
		{`$'\400'`, " "},
		{`$'\401'`, " "},
		{`$'\477'`, "\x27"},
		{`$'\600'`, "\x30"},
		{`$'\777'`, "\x3f"},
		{`$'\4001'`, " 1"},
		{`$'a\400b'`, "a b"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, status := run(t, fmt.Sprintf("printf '%%s' %s\n", tc.src))
			if out != tc.want || status != 0 {
				t.Errorf("printf %s = %q at %d, want %q", tc.src, out, status, tc.want)
			}
		})
	}
}

// A chain of names is followed to its end here, however long it is: what stops
// the lookup is a name coming back on itself and not a count of frames.
func TestALongChainOfNamesIsAValue(t *testing.T) {
	for _, n := range []int{60, 300} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var b strings.Builder
			for i := range n {
				fmt.Fprintf(&b, "v%d=v%d; ", i, i+1)
			}
			fmt.Fprintf(&b, "v%d=7\necho \"chain=$(( v0 ))\"\n", n)
			out, status := run(t, b.String())
			if out != "chain=7\n" || status != 0 {
				t.Errorf("a chain of %d = %q at %d, want \"chain=7\\n\" and 0", n, out, status)
			}
		})
	}
}

// And a loop is refused, with a sentence that names nothing — where the three
// columns that count frames each blame a name.
func TestARecursionLoopIsRefusedAndNamesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a name that points at itself", "x=x\necho \"v=$(( x ))\"\n"},
		{"two names that point at each other", "a=b\nb=a\necho \"v=$(( a+1 ))\"\n"},
		{"a loop through an expression", "a=b+1\nb=a\necho \"v=$(( a ))\"\n"},
		// This shell evaluates the right operand of a short-circuited `&&`
		// (#2605), so the loop is reached even where nothing needs its value.
		{"behind a short circuit", "x=x\necho \"v=$(( 0 && x ))\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := run(t, tc.src)
			if !strings.Contains(out, "expression recursion loop detected") {
				t.Errorf("got %q, want this shell's own sentence", out)
			}
			if strings.Contains(out, "nested too deeply") {
				t.Errorf("got %q, want the sentence to name no variable", out)
			}
			if status != 2 {
				t.Errorf("status %d, want 2", status)
			}
		})
	}
}
