// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// Six refusals and one trace, each worded or placed as BusyBox does.
// Measured 2026-10-04 in the digest-pinned alpine image, BusyBox v1.37.0
// (#5723).
func TestBuiltinRefusalsAreWordedAsBusyBoxWordsThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want, not string }{
		{"kill names itself for a job it cannot find", "kill %9\n", "kill: line 1: %9: no such job", ""},
		{"bg refuses an option word first", "bg --version\n", "bg: line 1: illegal option --", "no such job"},
		{"fg too", "fg -x\n", "fg: line 1: illegal option -x", ""},
		{"an emptied match operand is a missing operand", "p=; [[ abc =~ $p ]]\n", "=~: argument expected", ""},
		{"the bracket form has no match operator", "[ abc =~ ]\n", "=~: unknown operand", ""},
		{"a reserved word is quoted in a trace", "set -x; f() { echo in; }; f\n", "+ echo 'in'", ""},
		{"an assignment's value is not", "set -x; a=in\n", "+ a=in", "'in'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%q said %q, want %q in it", tc.src, out, tc.want)
			}
			if tc.not != "" && strings.Contains(out, tc.not) {
				t.Errorf("%q said %q, want no %q in it", tc.src, out, tc.not)
			}
		})
	}
}
