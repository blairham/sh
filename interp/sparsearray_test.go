// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An array is a map from subscript to value, and whether an unassigned
// subscript is an *element* is the dialect's to say.
//
// It was a list, so the gap was real: `a=(x); a[5]=y` was six elements, four
// of them empty, `${#a[@]}` counted them, and `for i in "${!a[@]}"` visited
// four subscripts nobody had assigned. That is nobody's answer — one reading
// gives two elements and the other five.
func TestWhetherAGapIsElements(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sparse Answer
		want   string
	}{
		{"a gap is nothing", Yes, "n=2 all=[x y] keys=[0 5]"},
		{"a gap is empty elements", No, "n=6 all=[x     y] keys=[0 1 2 3 4 5]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := arrays(t, `a=(x); a[5]=y; echo "n=${#a[@]} all=[${a[@]}] keys=[${!a[@]}]"`, tc.sparse)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("out = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// `unset a[i]` removes a subscript, which is the same question from the other
// side — and it had been doing nothing at all, because the subscript was read
// as part of the name and a name that was never in the table was deleted.
func TestUnsettingOneElement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sparse Answer
		want   string
	}{
		{"leaves a hole", Yes, "n=2 all=[p r] keys=[0 2]"},
		{"leaves an empty element", No, "n=3 all=[p  r] keys=[0 1 2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := arrays(t, `a=(p q r); unset "a[1]"; echo "n=${#a[@]} all=[${a[@]}] keys=[${!a[@]}]"`, tc.sparse)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("out = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Asked only where an array has a gap. A contiguous one reads the same either
// way, and that is almost every array there is — so refusing them all would
// be refusing to have arrays.
func TestTheGapQuestionIsAskedOnlyWhereThereIsOne(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"no gap", `a=(p q r); echo "${#a[@]}"`, false},
		{"a gap", `a=(x); a[5]=y; echo "${#a[@]}"`, true},
		{"a gap made by unsetting", `a=(p q r); unset "a[1]"; echo "${#a[@]}"`, true},
		{"an array of one", `a=(p); echo "${a[@]}"`, false},
		{"an empty array", `a=(); echo "${#a[@]}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				// Every other array axis answered, so a refusal here is
				// about the gap and not about the base or the scalar view.
				sem.ArrayBaseIsZero = Yes
				sem.ArrayScalarIsTheWholeArray = No
				// `unset a[0]` names an element here, which is the thing
				// under test rather than a question this asks.
				sem.UnsetTakesASubscript = Yes
				r.Semantics = &sem
			})
			if got := strings.Contains(out, "no element at all"); got != tc.refuses {
				t.Errorf("refused = %v, want %v (out %q)", got, tc.refuses, out)
			}
		})
	}
}

// Appending goes after the highest subscript, not after the count: an array
// with a gap in it has a longer reach than it has elements.
func TestAppendingGoesAfterTheHighestSubscript(t *testing.T) {
	out := arrays(t, `a=(x); a[5]=y; a+=(z); echo "${!a[@]}"`, Yes)
	if strings.TrimSpace(out) != "0 5 6" {
		t.Errorf("subscripts = %q, want the new element after the last", out)
	}
}

// A plain `$a` is the first element by *subscript* and not by the order the
// subscripts were written in, which a list gave for free and a map has to be
// asked for.
func TestTheScalarViewIsTheLowestSubscript(t *testing.T) {
	out := arrays(t, `a[5]=y; a[0]=x; echo "$a"`, Yes)
	if strings.TrimSpace(out) != "x" {
		t.Errorf("scalar = %q, want the lowest subscript's value", out)
	}
}

// `unset a[i]` takes the subscript the *script* wrote, which is not the
// position it is stored at wherever the dialect counts from one.
func TestUnsettingUsesTheDialectsOwnSubscript(t *testing.T) {
	out, st := run(t, `a=(p q r); unset "a[2]"; echo "[${a[@]}]"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArraysAreSparse = No
		sem.UnsetTakesASubscript = Yes
		// Counted from one, so `a[2]` is the middle element and not the last.
		sem.ArrayBaseIsZero = No
		sem.ArrayScalarIsTheWholeArray = No
		r.Semantics = &sem
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if strings.TrimSpace(out) != "[p  r]" {
		t.Errorf("out = %q, want the middle element gone", strings.TrimSpace(out))
	}
}

// arrays runs src in a grammar that has `${!a[@]}`, which the strict core
// does not — the subscripts are half of what this is about, so they have to
// be sayable.
func arrays(t *testing.T, src string, sparse Answer) string {
	t.Helper()
	d := syntax.Core()
	d.ParamIndirection = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem := CoreSemantics()
	sem.ArraysAreSparse = sparse
	sem.ArrayBaseIsZero = Yes
	sem.ArrayScalarIsTheWholeArray = No
	sem.UnsetTakesASubscript = Yes
	sem.IndirectionYieldsName = No
	r := &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}
