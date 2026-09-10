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

// An associative array is declared, never inferred: `typeset -A` puts the
// attribute on the name, and from then on a subscript is a string key where
// the same text on an undeclared name is an arithmetic index.
func TestAssociativeArrays(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{
			"a string subscript stores and reads back",
			`typeset -A m; m[k]=v; printf "%s" "${m[k]}"`, "v",
		},
		{
			// The whole reason the attribute exists: the subscript is taken
			// as written, so `1+1` is a three-character key and not 2.
			"the subscript is a key, not an expression",
			`typeset -A m; m[1+1]=x; printf "%s" "${m[1+1]}"`, "x",
		},
		{
			"values follow key order",
			`typeset -A m; m[b]=2; m[a]=1; printf "[%s]" "${m[@]}"`, "[1][2]",
		},
		{
			"the star form joins into one field",
			`typeset -A m; m[b]=2; m[a]=1; printf "[%s]" "${m[*]}"`, "[1 2]",
		},
		{
			"count and element length are different questions",
			`typeset -A m; m[a]=abc; m[b]=x; printf "%s %s" "${#m[@]}" "${#m[a]}"`, "2 3",
		},
		{
			"a compound literal keys its elements",
			`typeset -A m; m=([x]=1 [y]=2); printf "%s" "${m[x]}${m[y]}"`, "12",
		},
		{
			"a literal as a declaration operand",
			`typeset -A m=([x]=1 [y]=2); printf "%s" "${m[x]}${m[y]}"`, "12",
		},
		{
			"append keeps what is there",
			`typeset -A m=([a]=1); m+=([b]=2); printf "%s" "${#m[@]}${m[a]}${m[b]}"`, "212",
		},
		{
			"plain assignment starts over",
			`typeset -A m=([a]=1 [b]=2); m=([c]=3); printf "%s" "${#m[@]}${m[c]}"`, "13",
		},
		{
			// Two of the three shells with the attribute read bare elements
			// pairwise; the third refuses the shape both others accept.
			"bare elements pair up",
			`typeset -A m; m=(a 1 b 2); printf "%s" "${m[a]}${m[b]}"`, "12",
		},
		{
			"a quoted key holds a space",
			`typeset -A m; m=(["c d"]=s); printf "%s" "${m[c d]}"`, "s",
		},
		{
			"a value keeps its spaces",
			`typeset -A m; m=([a]="x y"); printf "[%s]" "${m[@]}"`, "[x y]",
		},
		{
			"an expanded subscript reads through",
			`typeset -A m; m=([b]=bee); k=b; printf "%s" "${m[$k]}"`, "bee",
		},
		{
			// nil is what says the element was not there: a stored "" is set.
			"a missing key takes the default",
			`typeset -A m; m[a]=1; printf "%s" "${m[z]:-d}"`, "d",
		},
		{
			"assigning through the default operator lands on the key",
			`typeset -A m; : "${m[k]:=v}"; printf "%s" "${m[k]}"`, "v",
		},
		{
			"unset takes one key",
			`typeset -A m; m=([a]=1 [b]=2); unset "m[a]"; printf "%s%s" "${#m[@]}" "${m[a]:-gone}"`, "1gone",
		},
		{
			"unset takes the whole array",
			`typeset -A m; m[a]=1; unset m; printf "%s" "${#m[@]}"`, "0",
		},
		{
			"declaring twice keeps the elements",
			`typeset -A m; m[a]=1; typeset -A m; printf "%s" "${m[a]}"`, "1",
		},
		{
			// The attribute is per name: without it the same text is the
			// arithmetic subscript it always was.
			"an undeclared name stays indexed",
			`a[0]=x; printf "%s" "${a[0]}"`, "x",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `${!m[@]}` is the keys, and it needs a grammar with indirection — the
// strict core has none, because two of the panel reject the spelling.
func TestIndirectSubscriptYieldsTheKeys(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"one key", `typeset -A m; m[k]=v; printf "%s" "${!m[@]}"`, "k"},
		{
			// Sorted, because no shell promises any order and a
			// deterministic one is worth having.
			"keys are sorted",
			`typeset -A m; m[b]=2; m[a]=1; printf "[%s]" "${!m[@]}"`, "[a][b]",
		},
		{
			"the loop that is the reason the form exists",
			`typeset -A m; m[b]=2; m[a]=1; for k in "${!m[@]}"; do printf "%s=%s;" "$k" "${m[$k]}"; done`,
			"a=1;b=2;",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := syntax.Core()
			d.ParamIndirection = true
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			var out bytes.Buffer
			sem := testSemantics()
			r := newTestRunner(t, &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if out.String() != c.want {
				t.Errorf("got %q, want %q", out.String(), c.want)
			}
		})
	}
}

// `typeset -A` inside a function declares a local table, and the *attribute*
// unwinds with the value: the caller's name must not come back reading its
// subscripts as strings.
func TestALocalAssociativeArrayStaysInTheFunction(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the elements stay inside", `f(){ typeset -A m; m[k]=v; }; f; echo "[${#m[@]}]"`, "[0]"},
		{"visible while inside", `f(){ typeset -A m; m[k]=v; echo "[${m[k]}]"; }; f`, "[v]"},
		{
			"an outer table survives",
			`typeset -A m; m[o]=out; f(){ typeset -A m; m[i]=in; }; f; echo "[${m[o]}${m[i]:-}]"`,
			"[out]",
		},
		{
			"without the declaration it is global",
			`typeset -A g; f(){ g[k]=v; }; f; echo "[${g[k]}]"`,
			"[v]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

// A plain `$m` on an associative array asks the same axis an indexed array
// does — and the "one element" answer is the element whose key is `0`, not
// the first key there is, because an associative array has no first.
func TestAssocScalarFollowsTheArrayScalarAxis(t *testing.T) {
	whole := testSemantics()
	whole.ArrayScalarIsTheWholeArray = Yes
	one := testSemantics()
	one.ArrayScalarIsTheWholeArray = No
	one.KeyedTableScalarIsTheFirstValue = No
	// The other answer to the second axis: the first value in the order the
	// table yields, which is what a shell with an ordered table gives.
	first := testSemantics()
	first.ArrayScalarIsTheWholeArray = No
	first.KeyedTableScalarIsTheFirstValue = Yes

	src := `typeset -A m; m[b]=2; m[a]=1; printf "[%s]" "$m"`
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &whole }); got != "[1 2]" {
		t.Errorf("whole-array answer gave %q, want [1 2]", got)
	}
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &one }); got != "[]" {
		t.Errorf("one-element answer without a 0 key gave %q, want []", got)
	}
	withZero := `typeset -A m; m[0]=z; m[a]=1; printf "[%s]" "$m"`
	if got, _ := run(t, withZero, func(r *Runner) { r.Semantics = &one }); got != "[z]" {
		t.Errorf("one-element answer gave %q, want [z] — the key 0, not the lowest key", got)
	}
	// And the second axis moves it off the key `0` and onto the order: the
	// same table with no `0` in it has a first value, where the answer above
	// has nothing at all.
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &first }); got != "[1]" {
		t.Errorf("first-value answer without a 0 key gave %q, want [1]", got)
	}
	if got, _ := run(t, withZero, func(r *Runner) { r.Semantics = &first }); got != "[z]" {
		t.Errorf("first-value answer gave %q, want [z] — the first, which is the 0 key here", got)
	}
}
