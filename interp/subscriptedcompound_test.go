// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The sentence a declaration utility says about a subscripted operand whose
// value is parenthesized, and the three conditions that decide it. See
// interp/subscriptedcompound.go for the panel each row was measured from.
func TestASubscriptedOperandHoldingACompoundIsSpokenAbout(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      string
	}{
		{
			"the operand written out again",
			`typeset a[1]="(var)"`,
			"warning: a[1]=(var): quoted compound array assignment deprecated",
		},
		{"a blank inside does not matter", `typeset a[1]="(v ar)"`, "warning: a[1]=(v ar):"},
		{"nor an empty pair", `typeset a[1]="()"`, "warning: a[1]=():"},
		{"the appending spelling says so", `typeset a[1]+="(v)"`, "warning: a[1]+=(v):"},
		{"a scalar standing there is not an array", `a=1; typeset a[1]="(v)"`, "warning: a[1]=(v):"},
		// And the conditions that keep it quiet.
		{"no declaration word", `a[1]="(var)"`, ""},
		{"the parentheses are not at the ends", `typeset a[1]=" (v) "`, ""},
		{"nor with text after them", `typeset a[1]="(v)x"`, ""},
		{"an array already standing", `typeset -a a; typeset a[1]="(v)"`, ""},
		{"a table", `typeset -A m; typeset m[k]="(v)"`, ""},
		{"the letter on this very line", `typeset -a a[1]="(v)"`, ""},
		{"a declaration that makes a local", `f() { typeset a[1]="(v)"; }; f`, ""},
		{"even over a local that is not an array", `f() { local a; typeset a[1]="(v)"; }; f`, ""},
		// The one that puts it back on the global cell.
		{"unless the letter says global", `f() { typeset -g a[1]="(v)"; }; f`, "warning: a[1]=(v):"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, func(r *Runner) {
				s := declareElementSemantics()
				s.DeclarationTakesASubscriptedAppendOperand = Yes
				r.Semantics = &s
				r.Diagnostics = &Diagnostics{
					QuotedCompoundArrayAssignmentDeprecated: "warning: %[1]s: quoted compound array assignment deprecated",
				}
			})
			said := strings.Contains(out, "quoted compound array assignment deprecated")
			switch {
			case c.want == "" && said:
				t.Errorf("said something where nothing was measured:\n%s", out)
			case c.want != "" && !strings.Contains(out, c.want):
				t.Errorf("output %q, want it to hold %q", out, c.want)
			}
		})
	}
}

// Nothing is said where the dialect has no wording, which is every column
// but the one that re-reads a parenthesized value at all.
func TestADialectWithNoWordingSaysNothingAboutASubscriptedCompound(t *testing.T) {
	out, _ := runDeclareElement(t, `typeset a[1]="(var)"`, nil)
	if strings.Contains(out, "deprecated") {
		t.Errorf("a dialect with no wording said something:\n%s", out)
	}
}
