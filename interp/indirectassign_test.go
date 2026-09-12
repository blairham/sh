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

// indirectAssignRun is flagsRun with the operator that always assigns and the
// array grammar the element rows need, on the one array base the flag's own
// dialect has. The axes are named rather than a shell: `(P)` beside an
// assignment exists where arrays start at one, and a row reading `a[2]` of a
// two-element array would be about the base and not about the flag anywhere
// else.
func indirectAssignRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	alwaysAssigning(&d)
	// The `${(P)+n}` row asks whether the *element* is set, which needs the
	// `+` between the braces and the name to be grammar at all.
	d.ParamSetTestFlag = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ArrayBaseIsZero = No
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Dialect: &d, Semantics: &sem, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The `(P)` flag moves the *assignment* one step along, not only the read.
//
// `(P)` says the base is the name of the parameter the expansion is about, so
// an operator that assigns in the same expansion writes that parameter. This
// shell read the flag on the way out only: `x=tgt; ${(P)x::=new}` substituted
// `new`, left `tgt` alone and stored `new` in `x` — a plausible word at status
// 0, and the failure this codebase minds most.
//
// It had a second half, which is why the row about a *number* is here. A real
// startup writes `${(P)2::=$value}` from a function whose `$2` holds the name
// to write; assigning to the name instead created a parameter called `2`, and
// every later `$2` in a function called with one argument read it. A plugin
// manager's "were two components given?" test then answered yes, and eighteen
// autoloaded functions were looked for in a directory built out of the wrong
// halves (#1672).
//
// Named for the flag rather than for a shell, per the rule in AGENTS.md.
// Measured on zsh 5.9.2, the only shell in the panel with the flag; the same
// rows are in the corpus under `param/expansion-flags-indirection-*`.
func TestTheIndirectionFlagAssignsToTheParameterItNamed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the value lands on the parameter the name resolves to",
			`x=tgt; tgt=old; printf "[%s][%s][%s]" "${(P)x::=new}" "$tgt" "$x"`,
			"[new][new][tgt]",
		},
		{
			"the conditional test is about the resolved parameter",
			`x=tgt; tgt=old; printf "[%s][%s]" "${(P)x:=new}" "$tgt"`,
			"[old][old]",
		},
		{
			"and it fires when that parameter is the one that is unset",
			`x=tgt; unset tgt; printf "[%s][%s]" "${(P)x:=new}" "$tgt"`,
			"[new][new]",
		},
		{
			"the colon-less assignment fires on unset alone",
			`x=tgt; tgt=; printf "[%s][%s]" "${(P)x=new}" "$tgt"`,
			"[][]",
		},
		{
			"and on nothing else",
			`x=tgt; unset tgt; printf "[%s][%s]" "${(P)x=new}" "$tgt"`,
			"[new][new]",
		},
		{
			"a base with nothing to resolve leaves the assignment on the name written",
			`unset x nm; printf "[%s][%s][%s]" "${(P)x::=nm}" "$x" "$nm"`,
			"[nm][nm][]",
		},
		{
			"the other letters act on what is substituted and not on what is stored",
			`x=tgt; tgt=old; printf "[%s][%s]" "${(UP)x::=new}" "$tgt"`,
			"[NEW][new]",
		},
		{
			"the base's own subscript belongs to the resolution and not to the target",
			`arr=(tgt zz); tgt=o; printf "[%s][%s][%s]" "${(P)arr[1]::=new}" "$tgt" "${arr[1]}"`,
			"[new][new][tgt]",
		},
		{
			"a number the name resolved to is not a parameter called by that digit",
			`x=tgt; tgt=old; printf "[%s]" "${(P)x::=1}"; f() { printf "[%s][%s]" "$#" "$2"; }; f one`,
			"[1][1][]",
		},

		// The controls: the same operators with no flag group at all were
		// already right, so a case failing only above is a case about the
		// route and not about the operators.
		{
			"without the flag the assignment is the name written",
			`x=tgt; tgt=old; printf "[%s][%s][%s]" "${x::=new}" "$x" "$tgt"`,
			"[new][new][old]",
		},
		{
			"and a group without the flag is too",
			`x=tgt; tgt=old; printf "[%s][%s][%s]" "${(U)x::=new}" "$x" "$tgt"`,
			"[NEW][new][old]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectAssignRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q: out=%q status=%d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The resolved text is a parameter *reference* and not only a name, so a
// subscript in it reaches one element.
//
// This is the half the startup needs: its table of times is written as
// `${(P)$name::=$t}` with `$name` holding `ZI[mtime-side]`. The read is
// asserted beside the write on purpose — a shell that assigned to the element
// while still reading the whole text as a flat name would answer the read
// empty and then fire `:=` over a key that was already there.
func TestTheIndirectionFlagAssignsToAnElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an association's key, read through the same name",
			`typeset -A m; m=(k old); n="m[k]"; printf "[%s][%s][%s]" "${(P)n}" "${(P)n::=new}" "${m[k]}"`,
			"[old][new][new]",
		},
		{
			"a key that is there is not overwritten by the conditional operator",
			`typeset -A m; m=(k old); n="m[k]"; printf "[%s][%s]" "${(P)n:=new}" "${m[k]}"`,
			"[old][old]",
		},
		{
			"a key that is not there is",
			`typeset -A m; n="m[k]"; printf "[%s][%s]" "${(P)n:=new}" "${m[k]}"`,
			"[new][new]",
		},
		{
			"and the set test asks the element",
			`typeset -A m; m=(k v); n="m[k]"; g="m[nosuch]"; printf "[%s][%s]" "${(P)+n}" "${(P)+g}"`,
			"[1][0]",
		},
		{
			"an array element by index",
			`a=(p q); i="a[2]"; printf "[%s][%s][%s]" "${(P)i}" "${(P)i::=Z}" "${a[*]}"`,
			"[q][Z][p Z]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectAssignRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q: out=%q status=%d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The name check is asked of what the indirection came to, not of what was
// written.
//
// `x` is a perfectly good identifier, so a shell asking one step too early
// sees nothing wrong and assigns to `x`. Asked one step later it names the
// text the value holds and ends the script — which is the second assertion:
// a refusal carried as a status would leave the word after it behind.
//
// All three assigning operators, because the *direct* spelling asks the
// question of `::=` alone (see assignableTarget, and #1541 for why it is
// narrow there) and the flag's route asks it of every one — measured, `x=;
// ${(P)x:=new}` and `x=; ${(P)x=new}` are both `not an identifier: `.
func TestTheIndirectionFlagRefusesWhatItResolvedTo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"two words are no name", `x="a b"; echo "${(P)x::=new}"`, "not an identifier: a b"},
		{"nor is a character no parameter answers to", `x="#"; echo "${(P)x::=new}"`, "not an identifier: #"},
		{"a set but empty base is the empty name", `x=; echo "${(P)x::=new}"`, "not an identifier: "},
		{"the conditional operator asks it too", `x=; echo "${(P)x:=new}"`, "not an identifier: "},
		{"and the colon-less one", `x=; echo "${(P)x=new}"`, "not an identifier: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectAssignRun(t, tc.src+`; echo AFTER`)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%q: out=%q, want %q in it", tc.src, out, tc.want)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("%q: out=%q, want the script ended there", tc.src, out)
			}
			if st == 0 {
				t.Errorf("%q: status 0, want the refusal to be fatal", tc.src)
			}
		})
	}
}

// A write through `(P)` goes through the resolved text read as the parameter
// expansion it **spells**, which is the same node the read side uses.
//
// It used to be taken apart by hand into a name and one arithmetic subscript.
// A search failed in the arithmetic, and a *range* was silently wrong: `1,2`
// reached the reader as one expression, the comma operator answered its right
// operand, and the write landed on element 2 instead of replacing the span —
// a plausible array back at status 0 (#2169).
func TestAWriteThroughAReferenceReachesEverySubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a search names the element",
			`a=(p q); v='a[(r)q]'; : ${(P)v::=Z}; printf "[%s]" "${a[@]}"`,
			"[p][Z]",
		},
		{
			"and its index form names the same one",
			`a=(p q); v='a[(i)q]'; : ${(P)v::=Z}; printf "[%s]" "${a[@]}"`,
			"[p][Z]",
		},
		{
			// The silent one: the span is replaced, so the array shrinks.
			"a range names a span",
			`x=(p q r); v='x[1,2]'; : ${(P)v::=Z}; printf "[%s]" "${x[@]}"`,
			"[Z][r]",
		},
		{
			"a range that grows the array",
			`x=(p q r); v='x[2,3]'; : ${(P)v::=Z}; printf "[%s]" "${x[@]}"`,
			"[p][Z]",
		},
		{
			// The control, which already worked: one element by index.
			"one element by index",
			`x=(p q r); v='x[2]'; : ${(P)v::=Z}; printf "[%s]" "${x[@]}"`,
			"[p][Z][r]",
		},
		{
			// And a character of a scalar, the other control.
			"a character of a string",
			`s=abc; v='s[2]'; : ${(P)v::=Z}; printf "[%s]" "$s"`,
			"[aZc]",
		},
		{
			"a key on a table",
			`typeset -A m; m[k]=old; w='m[k]'; : ${(P)w::=Z}; printf "[%s]" "${m[k]}"`,
			"[Z]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectSpanRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// indirectSpanRun is indirectAssignRun with the two answers a subscript that
// names a *span* depends on, which the rows above are about: the comma is a
// range, and a span on the left replaces the elements it covers.
func indirectSpanRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	alwaysAssigning(&d)
	d.ArraySubscriptFlags = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ArrayBaseIsZero = No
	sem.SubscriptCommaIsARange = Yes
	sem.SubscriptExpressionStopsAtASeparator = Yes
	sem.ScalarSubscriptIsACharacter = Yes
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Dialect: &d, Semantics: &sem, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}
