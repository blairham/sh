// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `read` and `printf -v` reach one store through one operand, and in the
// column where the refusal ends the shell they part on the number it leaves:
// the store through `printf -v` leaves 0 and the one through `read` leaves 1.
//
// Only readable through a subshell's status or the shell's own exit from a
// `-c` string, since nothing after the refusal runs either way (#3497, #3518).

// The unevaluable subscript at both builtins.
func TestARefusedStoreLeavesTheBuiltinsOwnNumber(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"printf", `( printf -v 'a[1/0]' X )`, "next=0"},
		{"read", `( { read 'a[1/0]'; } <<EOF` + "\nY\nEOF\n)", "next=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeRefusalRun(t, Yes, "a=(1 2 3)\n"+c.src+"\n"+`echo "next=$? a=[${a[*]}]"`)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
			if !strings.Contains(out, "a=[1 2 3]") {
				t.Errorf("out %q wrote through a refused store", out)
			}
		})
	}
}

// And the **empty** subscript answers the same way, which is what makes this
// one field about the builtin rather than one per refusal.
func TestARefusedEmptySubscriptLeavesTheBuiltinsOwnNumber(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"printf", `( printf -v 'a[]' X )`, "next=0"},
		{"read", `( { read 'a[]'; } <<EOF` + "\nY\nEOF\n)", "next=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := storeRefusalRun(t, Yes, "a=(1 2 3)\n"+c.src+"\n"+`echo "next=$?"`)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
		})
	}
}

// Answered No, `printf -v` leaves what `read` leaves — so the field moves one
// builtin and is not a rule about the store.
func TestARefusedPrintfStoreLeavesOneWhereTheDialectSaysSo(t *testing.T) {
	out, _ := storeRefusalRun(t, No,
		"a=(1 2 3)\n"+`( printf -v 'a[1/0]' X )`+"\n"+`echo "next=$?"`)
	if !strings.Contains(out, "next=1") {
		t.Errorf("out %q, want the refusal's own 1", out)
	}
}

// The zero is raised back to 1 in the three places every refusal's is, which
// is the same function the declaration's store reads — so `printf -v` does not
// get a second copy of that rule.
func TestARefusedPrintfStoresZeroIsRaisedBackLikeAnyOther(t *testing.T) {
	out, _ := storeRefusalRun(t, Yes,
		"a=(1 2 3)\n"+`( printf -v 'a[1/0]' X && echo yes )`+"\n"+`echo "next=$?"`)
	if !strings.Contains(out, "next=1") {
		t.Errorf("out %q did not raise the zero out of an && list", out)
	}
	if strings.Contains(out, "yes") {
		t.Errorf("out %q ran past the give-up", out)
	}
}

// storeRefusalRun answers what the store needs and makes the refusal the fatal
// one, which is the only branch whose number a caller can read.
func storeRefusalRun(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.PrintfAssignsWithV = Yes
		s.BadSubscriptToAnOutputOperand = BadSubscriptEndsTheScript
		s.EmptyArithSubscript = EmptyArithSubscriptIsInvalid
		s.FatalErrorStatusIsOne = Yes
		s.StoreRefusalThroughPrintfLeavesZero = answer
	}, Diagnostics{}, src, RouteUnspecified)
}
