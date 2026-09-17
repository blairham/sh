// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `printf -v` writes to the element a subscripted output parameter names, and
// refuses a frozen one — the same two things `read` does with the same
// operand, through the same two functions (#2298, #3469).

func printfElementSem(assigns Answer) Semantics {
	s := testSemantics()
	s.PrintfAssignsWithV = assigns
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	return s
}

// The store, in both containers. It went through the plain variable store
// before this, so the name was a *scalar* spelled with brackets in it: the
// array a script then read was untouched and the builtin still reported 0.
func TestPrintfWithVReachesAnElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an indexed element",
			`q=(1 2 3); printf -v 'q[1]' '%s' B; echo "[${q[*]}] ${#q[@]}"`,
			"[1 B 3] 3",
		},
		{
			"an element past the end",
			`q=(1 2); printf -v 'q[5]' '%s' E; echo "[${q[*]}] ${#q[@]}"`,
			"[1 2 E] 3",
		},
		{
			"a keyed element",
			`typeset -A m; printf -v 'm[k]' '%s' C; typeset -p m`,
			`declare -A m=([k]="C" )`,
		},
		{
			"a keyed element that was already there",
			`typeset -A m=([k]=old); printf -v 'm[k]' '%s' D; typeset -p m`,
			`declare -A m=([k]="D" )`,
		},
		// The control: a name with no subscript is the plain store it always
		// was, which is what says the route was widened and not swapped.
		{"a scalar", `printf -v s '%s' A; echo "[$s]"`, "[A]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(printfElementSem(Yes)))
			if got := strings.TrimSpace(out); got != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q", got, st, tc.want)
			}
		})
	}
}

// And a frozen name is refused with nothing written, which is the half that
// has to be asked before the format runs: `printf -v` collects its output
// instead of printing it, so a refusal after the fact would have formatted
// and thrown away.
func TestPrintfWithVRefusesAFrozenElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an indexed element of a frozen array",
			`a=(1 2); readonly a; printf -v 'a[0]' '%s' Y; echo "st=$? [${a[*]}]"`,
			"st=1 [1 2]",
		},
		{
			"a keyed element of a frozen table",
			`typeset -A m=([k]=v); readonly m; printf -v 'm[k]' '%s' Z; echo "st=$? [${m[k]}]"`,
			"st=1 [v]",
		},
		{
			"a frozen scalar",
			`readonly s=1; printf -v s '%s' X; echo "st=$? [$s]"`,
			"st=1 [1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfElementSem(Yes)
			// The freeze refusal carries two axes of its own, and the rows
			// here are about the *element* rather than about either: they
			// take the reading that keeps the line, so the `echo` after the
			// refusal is there to be read.
			sem.ReadonlyRefusalInABuiltinIsFatal = No
			out, _ := run(t, tc.src, withSem(sem))
			if got := strings.TrimSpace(out); !strings.HasSuffix(got, tc.want) {
				t.Errorf("got %q, want it to end in %q", got, tc.want)
			}
		})
	}
}
