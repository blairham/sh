// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// listSem is the vector a whole-list expansion needs answered before any of
// these questions can be asked: where an array starts, and that a bare name is
// not the whole of one.
func listSem() Semantics {
	s := CoreSemantics()
	s.ArrayBaseIsZero = Yes
	s.ArraysAreSparse = Yes
	s.ArrayScalarIsTheWholeArray = No
	s.ArrayLengthWithoutSubscriptIsCount = No
	s.OperatorDistributesOverTheFieldList = Yes
	s.OperatorDistributesOverStarSubscript = Yes
	s.SplitParamExpansion = Yes
	return s
}

// `count` is the probe every row here uses, because the thing that splits is
// how many fields come back and `printf` writes its format once for none.
const countFields = `n() { printf '%s:' "$#"; printf '<%s>' "$@"; printf '\n'; }; `

// What the **colon** of `${a[@]:-word}` tests on a whole list. Three readings,
// and the discriminating shape is a list holding an empty element — which is
// why every row here has one (#3425).
func TestWholeArrayColonTestAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer WholeArrayColonTestPolicy
		src    string
		want   string
	}{
		// bash 5.3.20: the join is empty only when every element is and
		// there is at most one of them.
		{
			"the join, an empty element first", WholeArrayColonTestReadsTheJoinedValue,
			`a=("" c); n "${a[@]:-x}"`, "2:<><c>",
		},
		{
			"the join, one empty element", WholeArrayColonTestReadsTheJoinedValue,
			`a=(""); n "${a[@]:-x}"`, "1:<x>",
		},
		{
			"the join, two empty elements", WholeArrayColonTestReadsTheJoinedValue,
			`a=("" ""); n "${a[@]:-x}"`, "2:<><>",
		},
		// ksh93u+: the first element alone, however many follow it.
		{
			"the first element, empty first", WholeArrayColonTestReadsTheFirstElement,
			`a=("" c); n "${a[@]:-x}"`, "1:<x>",
		},
		{
			"the first element, empty last", WholeArrayColonTestReadsTheFirstElement,
			`b=(c ""); n "${b[@]:-x}"`, "2:<c><>",
		},
		{
			"the first element, two empty", WholeArrayColonTestReadsTheFirstElement,
			`a=("" ""); n "${a[@]:-x}"`, "1:<x>",
		},
		{
			"the first element under the star spelling", WholeArrayColonTestReadsTheFirstElement,
			`a=("" c); n "${a[*]:-x}"`, "1:<x>",
		},
		// zsh 5.9.2: any element at all is a value under `[@]`, and the join
		// under `[*]`.
		{
			"the element count, one empty element", WholeArrayColonTestCountsTheElementsUnderAt,
			`a=(""); n "${a[@]:-x}"`, "1:<>",
		},
		{
			"the element count, an empty element first", WholeArrayColonTestCountsTheElementsUnderAt,
			`a=("" c); n "${a[@]:-x}"`, "2:<><c>",
		},
		{
			"the star spelling still reads the join", WholeArrayColonTestCountsTheElementsUnderAt,
			`a=(""); n "${a[*]:-x}"`, "1:<x>",
		},
		// The positional list is the same question and the commoner source.
		{
			"positionals, the first element", WholeArrayColonTestReadsTheFirstElement,
			`set -- "" c; n "${@:-x}"`, "1:<x>",
		},
		{
			"positionals, the join", WholeArrayColonTestReadsTheJoinedValue,
			`set -- "" c; n "${@:-x}"`, "2:<><c>",
		},
		{
			"positionals, the element count", WholeArrayColonTestCountsTheElementsUnderAt,
			`set -- ""; n "${@:-x}"`, "1:<>",
		},
		// The alternate, where the readings part the same way — and where a
		// fired `+` on a list that still has elements is one empty field
		// rather than the elements.
		{
			"the alternate under the first element", WholeArrayColonTestReadsTheFirstElement,
			`a=("" c); n "${a[@]:+y}"`, "1:<>",
		},
		{
			"the alternate under the join", WholeArrayColonTestReadsTheJoinedValue,
			`a=("" c); n "${a[@]:+y}"`, "1:<y>",
		},
		{
			"the alternate under the element count", WholeArrayColonTestCountsTheElementsUnderAt,
			`a=(""); n "${a[@]:+y}"`, "1:<y>",
		},
		// Controls. Where the three readings agree the axis is never put, so
		// a runner with no answer still runs these.
		{
			"a list of ordinary values", WholeArrayColonTestUnspecified,
			`h=(a b); n "${h[@]:-x}"`, "2:<a><b>",
		},
		{
			"a list with no elements", WholeArrayColonTestUnspecified,
			`e=(); n "${e[@]:-x}"`, "1:<x>",
		},
		{
			"and an element subscript is not this question", WholeArrayColonTestUnspecified,
			`a=("" c); n "${a[0]:-x}"`, "1:<x>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := listSem()
			sem.WholeArrayColonTest = tc.answer
			out, _ := run(t, countFields+tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want+"\n" {
				t.Errorf("got %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// An unanswered colon test is refused where it decides, and nowhere else.
func TestWholeArrayColonTestUnanswered(t *testing.T) {
	sem := listSem()
	sem.WholeArrayColonTest = WholeArrayColonTestUnspecified
	out, _ := run(t, countFields+`a=("" c); n "${a[@]:-x}"`, func(r *Runner) { r.Semantics = &sem })
	if want := "what the colon of `${a[@]:-word}` tests on a whole list"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to carry %q", out, want)
	}
}

// A **non-global** replacement over a joined field list ends it at the field
// it replaced in, in the one column that joins and has the operator (#3413).
func TestReplacementEndsTheJoinedFieldListAxis(t *testing.T) {
	for _, tc := range []struct {
		name     string
		joins    Answer
		ends     Answer
		src      string
		want     string
		refusal  string
		distinct bool
	}{
		// BusyBox ash 1.37.0.
		{
			name: "the list ends at the field it replaced in", joins: No, ends: Yes,
			src: `set -- xb alpha beta; n "${@/a/-}"`, want: "2:<xb><-lpha>",
		},
		{
			name: "a match in the first field ends it there", joins: No, ends: Yes,
			src: `set -- ab ab ab; n "${@/a/-}"`, want: "1:<-b>",
		},
		{
			name: "a replacement with no replacement word", joins: No, ends: Yes,
			src: `set -- ab ab ab; n "${@/b}"`, want: "1:<a>",
		},
		{
			name: "a field holding a blank", joins: No, ends: Yes,
			src: `set -- 'p q' r; n "${@/q/-}"`, want: "1:<p ->",
		},
		{
			name: "and the unquoted spelling is cut the same", joins: No, ends: Yes,
			src: `set -- 'p q' r; n ${@/q/-}`, want: "2:<p><->",
		},
		// The other answer to the same axis: the join, uncut.
		{
			name: "the join keeps every field", joins: No, ends: No,
			src: `set -- xb alpha beta; n "${@/a/-}"`, want: "3:<xb><-lpha><beta>",
		},
		// Controls. Each is a way the cut could have been made too wide.
		{
			name: "a pattern that matches nothing cuts nothing", joins: No, ends: Yes,
			src: `set -- xb alpha beta; n "${@/zz/-}"`, want: "3:<xb><alpha><beta>",
		},
		{
			name: "the global replacement cuts nothing", joins: No, ends: Yes,
			src: `set -- ab ab ab; n "${@//b/-}"`, want: "3:<a-><a-><a->",
		},
		{
			name: "a trim cuts nothing", joins: No, ends: Yes,
			src: `set -- ab cb; n "${@#a}"`, want: "2:<b><cb>",
		},
		{
			name: "a replacement in the last field cuts nothing", joins: No, ends: Yes,
			src: `set -- ab cb; n "${@/c/-}"`, want: "2:<ab><-b>",
		},
		// And the axis is unreachable where the operator distributes, which
		// is every other column: nothing is joined, so there is no list to
		// cut short.
		{
			name: "the distributing reading is not cut", joins: Yes, ends: Yes,
			src: `set -- xb alpha beta; n "${@/a/-}"`, want: "3:<xb><-lpha><bet->",
		},
		{
			name: "unanswered where it would cut", joins: No, ends: Unspecified,
			src:     `set -- xb alpha beta; n "${@/a/-}"`,
			refusal: "a replacement ending `$@` at the field it replaced in",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := listSem()
			sem.OperatorDistributesOverTheFieldList = tc.joins
			sem.ReplacementEndsTheJoinedFieldList = tc.ends
			out, _ := run(t, countFields+tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refusal != "" {
				if !strings.Contains(out, tc.refusal) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refusal)
				}
				return
			}
			if out != tc.want+"\n" {
				t.Errorf("got %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// An operator written around a `${!v}` whose target is a whole list maps over
// the **elements**, exactly as it does when the target is written out. Every
// row here came back as one joined field (#3243).
func TestAnOperatorOnAnIndirectListMapsOverTheElements(t *testing.T) {
	const setup = `set -- p q r; a=(A B C); at='@'; ea='a[@]'; es='a[*]'; `
	for _, tc := range []struct{ name, src, want string }{
		{"the plain spelling is the control", `n "${!ea}"`, "3:<A><B><C>"},
		{"a prefix trim", `n "${!ea#A}"`, "3:<><B><C>"},
		{"a suffix trim", `n "${!ea%C}"`, "3:<A><B><>"},
		{"a replacement", `n "${!ea/A/z}"`, "3:<z><B><C>"},
		{"a global replacement", `n "${!ea//A/z}"`, "3:<z><B><C>"},
		{"a case change", `n "${!ea,,}"`, "3:<a><b><c>"},
		{"a transformation", `n "${!ea@Q}"`, "3:<'A'><'B'><'C'>"},
		{"a trim on the positional list", `n "${!at#p}"`, "3:<><q><r>"},
		{"a transformation on the positional list", `n "${!at@Q}"`, "3:<'p'><'q'><'r'>"},
		{"a slice of the array", `n "${!ea:1}"`, "2:<B><C>"},
		{"a slice with a length", `n "${!ea:0:2}"`, "2:<A><B>"},
		// `${@:1}` counts from `$1`, so the whole list is the right answer
		// and not a missing slice: measured on bash 5.3.20, `n "${!at:1}"`
		// is `3:<p><q><r>` where `n "${!at:2}"` is two.
		{"a slice of the positional list", `n "${!at:1}"`, "3:<p><q><r>"},
		{"a slice that drops a parameter", `n "${!at:2}"`, "2:<q><r>"},
		{"the default operator yields the list", `n "${!ea-D}"`, "3:<A><B><C>"},
		{"and the alternate yields its word", `n "${!ea+S}"`, "1:<S>"},
		// The joining spelling is the control on the other side: the target
		// names one value, so the operator has one value to act on.
		{"the joining target is one field", `n "${!es}"`, "1:<A B C>"},
		{"and an operator on it is still one field", `n "${!es#A}"`, "1:< B C>"},
		// A target that is not a list is untouched by any of this.
		{"a target naming one element", `v='a[1]'; n "${!v#B}"`, "1:<>"},
		{"a target naming a scalar", `s=hello; w=s; n "${!w#he}"`, "1:<llo>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := listSem()
			sem.IndirectionYieldsName = No
			sem.SubstringOfPositionalsSlicesTheList = Yes
			out, _ := runGrammar(t, countFields+setup+tc.src, func(d *syntax.Dialect) {
				d.ParamIndirection = true
				d.ParamCaseChange = true
				d.ParamTransformations = true
			}, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want+"\n" {
				t.Errorf("got %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// The grammar half: the rewrite is taken only where the indirection really
// resolves to a list, so the shapes beside it keep the answers they had.
func TestTheIndirectListRewriteStaysWhereItWas(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a length is answered before the rewrite", `a=(A B C); ea='a[@]'; echo "${#ea}"`, "4\n"},
		{"an indirection with a subscript of its own", `a=(A B C); n "${!a[1]}"`, "1:<>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := listSem()
			sem.IndirectionYieldsName = No
			out, _ := runGrammar(t, countFields+tc.src, func(d *syntax.Dialect) {
				d.ParamIndirection = true
			}, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
