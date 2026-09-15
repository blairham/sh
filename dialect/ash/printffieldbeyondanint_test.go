// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A field past the C `int` writes an empty field here and the builtin carries
// on, reporting 1.
//
// Measured 2026-09-15 on BusyBox v1.37.0, through the container route the
// oracle reaches this shell by. Two operands, because the reuse is what says
// the operand was *consumed* rather than the conversion skipped:
//
//	$ busybox sh -c 'printf "[%21474836470s]" a b; echo "|st=$?"'
//	[][]|st=1
//
// This column is the one that turns *above* INT_MAX rather than at it —
// `%2147483647s` is two billion characters here, where bash and dash refuse —
// and its status is what parts it from bash 3.2, which answers `[][]` at 0
// for the same line and is not a dialect here.
func TestAFieldBeyondAnIntIsEmptyHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '[%21474836470s]' a b; echo "|st=$?"`, "[][]|st=1"},
		{`printf '[%4294967306s]' x; echo "|st=$?"`, "[]|st=1"},
		{`printf '[%.21474836470f]' 1; echo "|st=$?"`, "[]|st=1"},
	} {
		out, _ := run(t, tc.src)
		if got := strings.TrimSpace(out); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: said %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}

// A `*` operand past the int is an unreadable number here rather than one out
// of range, and what that leaves behind is a zero rather than an absence.
//
// A width cannot show the difference — absent and zero are the same width —
// so the precision is what this rests on. Measured the same day:
//
//	$ busybox sh -c 'printf "[%.*f]" 21474836470 1'   [1]
//	$ busybox sh -c 'printf "[%.*f]" abc 1'           [1]
//
// Identical, wording included, where bash writes `[1.000000]` for the first
// and `[1]` for the second. The complaint costing nothing is
// PrintfStarComplaintCostsTheStatus rather than anything of this axis's.
func TestAStarOperandBeyondAnIntIsNotANumberHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf 'A[%*s]B' 21474836470 x; echo "|st=$?"`, "A[x]B|st=0"},
		{`printf '[%.*f]' 21474836470 1; echo "|st=$?"`, "[1]|st=0"},
		{`printf '[%.*f]' abc 1; echo "|st=$?"`, "[1]|st=0"},
	} {
		out, _ := run(t, tc.src)
		if got := strings.TrimSpace(out); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: said %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}
