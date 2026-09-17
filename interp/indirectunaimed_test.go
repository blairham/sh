// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runUnaimedIndirection runs src with `${!x}` available and the two axes the
// refusal's cost rides on answered, so a case can say what the refusal left
// behind rather than only that one was made.
func runUnaimedIndirection(t *testing.T, abandons Answer, dg Diagnostics) func(string) (string, int) {
	return func(src string) (string, int) {
		t.Helper()
		sem := namerefAimSemantics()
		sem.FailedExpansionAbandonsTheLine = abandons
		sem.FatalErrorStatusIsOne = Yes
		// The indirection *taken* rather than yielding the name, which is
		// the reading the rows either side of the refusal are written in.
		// The refusal itself is ahead of that axis and a case below asserts
		// it under the other answer.
		sem.IndirectionYieldsName = No
		return runGrammar(t, src, func(d *syntax.Dialect) {
			d.ParamIndirection = true
		}, func(r *Runner) {
			s := sem
			r.Semantics = &s
			r.Diagnostics = &dg
		})
	}
}

// `${!r}` over a name reference with **nothing to point at** is refused, which
// is the one state the expansion's other two refusals cannot reach: the name
// exists, so it is not undeclared, and the cell under it holds nothing, which
// is a control row bash does *not* refuse.
//
// It answered the empty string at status 0 — a silent empty that reads exactly
// like a reference aimed at a name holding nothing. See #3490.
func TestAnIndirectionOverAReferenceThatPointsNowhereIsRefused(t *testing.T) {
	run := runUnaimedIndirection(t, Yes, Diagnostics{
		IndirectionUnaimedReference: "%[1]s: invalid indirect expansion",
	})

	out, _ := run(`typeset -n u
echo "A[${!u}]"
echo after`)
	if !strings.Contains(out, "u: invalid indirect expansion") {
		t.Errorf("got %q, want the reference named and refused", out)
	}
	if strings.Contains(out, "A[") {
		t.Errorf("got %q, want no value — the expansion did not finish", out)
	}

	// The operators do not stand in for it, which is measured rather than
	// assumed: the word behind `-` and `:?` never gets the chance.
	for _, src := range []string{
		`typeset -n u; echo "B[${!u-DEF}]"`,
		`typeset -n u; echo "C[${!u:?msg}]"`,
		`typeset -n u; echo "D[${!u+SET}]"`,
	} {
		out, _ := run(src)
		if !strings.Contains(out, "u: invalid indirect expansion") {
			t.Errorf("%q: got %q, want the refusal in front of the operator", src, out)
		}
		for _, word := range []string{"DEF", "SET", "B[", "C[", "D["} {
			if strings.Contains(out, word) {
				t.Errorf("%q: got %q, want no %q", src, out, word)
			}
		}
	}

	// The states either side of it are untouched, and they are what make this
	// one row rather than a class: a reference that *is* aimed answers with
	// the name it points at, and an element aim answers with the element's.
	for _, tc := range []struct{ name, src, want string }{
		{"aimed at a name", `v=1; typeset -n r=v; echo "[${!r}]"`, "[v]"},
		{"aimed at an element", `a=(x y); typeset -n n2=a[1]; echo "[${!n2}]"`, "[a[1]]"},
		{"a plain name holding a name", `tgt=T; h=tgt; echo "[${!h}]"`, "[T]"},
	} {
		out, st := run(tc.src)
		if !strings.Contains(out, tc.want) || st != 0 {
			t.Errorf("%s: got %q at %d, want %q", tc.name, out, st, tc.want)
		}
	}

	// The refusal is in front of Semantics.IndirectionYieldsName and not
	// behind it: the dialect that reads `${!x}` as the *name* makes the same
	// refusal, and is the only indirection refusal it makes at all.
	asName := func(src string) (string, int) {
		sem := namerefAimSemantics()
		sem.FailedExpansionAbandonsTheLine = Yes
		sem.FatalErrorStatusIsOne = Yes
		sem.IndirectionYieldsName = Yes
		return runGrammar(t, src, func(d *syntax.Dialect) {
			d.ParamIndirection = true
		}, func(r *Runner) {
			r.Semantics = &sem
			r.Diagnostics = &Diagnostics{IndirectionUnaimedReference: "%[1]s: no reference name"}
		})
	}
	out, _ = asName(`typeset -n u; echo "E[${!u}]"`)
	if !strings.Contains(out, "u: no reference name") || strings.Contains(out, "E[") {
		t.Errorf("got %q, want the same refusal under the name reading", out)
	}

	// And with no wording the dialect makes no refusal at all, which is the
	// reading a shell without name references gets.
	silent := runUnaimedIndirection(t, Yes, Diagnostics{})
	out, st := silent(`typeset -n u; echo "[${!u}]"; echo after`)
	if !strings.Contains(out, "[]") || !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q at %d, want the old empty answer where no dialect refuses", out, st)
	}
}

// What the refusal costs the script is the axis a failed expansion already
// asks, and is not a second question here: one dialect gives up the rest of
// the line and runs the next, the other ends the script.
func TestWhatTheUnaimedIndirectionRefusalCostsTheScript(t *testing.T) {
	dg := Diagnostics{IndirectionUnaimedReference: "%[1]s: no reference name"}

	out, st := runUnaimedIndirection(t, Yes, dg)(`typeset -n u
echo "[${!u}]"; echo same-line
echo next-line`)
	if strings.Contains(out, "same-line") {
		t.Errorf("got %q, want the rest of the line given up", out)
	}
	if !strings.Contains(out, "next-line") || st != 0 {
		t.Errorf("got %q at %d, want the script to carry on", out, st)
	}

	out, st = runUnaimedIndirection(t, No, dg)(`typeset -n u
echo "[${!u}]"
echo next-line`)
	if strings.Contains(out, "next-line") {
		t.Errorf("got %q, want the script over", out)
	}
	if st != 1 {
		t.Errorf("got %q at %d, want the script to end at 1", out, st)
	}
}
