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

// The array letter written on the same line takes the subscript out of the
// operand: the value is re-read into the base name, and neither the
// subscript nor the append spelling decides anything. See
// Runner.letterDropsTheSubscript for the panel.
func TestAnArrayLetterDropsASubscriptedCompound(t *testing.T) {
	// Read back through the elements rather than through a listing, which is
	// a question of its own with axes of its own.
	const read = `; echo "[${a[0]-}] [${a[1]-}] [${a[2]-}] n=${#a[@]}"`
	for _, c := range []struct {
		name, src, want string
	}{
		{"the value lands from the base", `typeset -a a[1]="(v)"`, `[v] [] [] n=1`},
		{"as a whole literal", `typeset -a a[1]="(v w)"`, `[v] [w] [] n=2`},
		{"whatever the subscript was", `typeset -a a[2]="(v)"`, `[v] [] [] n=1`},
		{"replacing what stood there", `typeset -a a=(z); typeset -a a[1]="(v)"`, `[v] [] [] n=1`},
		{"the append spelling too", `typeset -a a[1]+="(v)"`, `[v] [] [] n=1`},
		// The controls: an ordinary value keeps its subscript, and so does a
		// parenthesized one with no letter on the line.
		{"an ordinary value keeps it", `typeset -a a[1]=plain`, `[] [plain] [] n=1`},
		{"and so does one with no letter", `typeset a[1]="(v)"`, `[] [(v)] [] n=1`},
		{"the shape test still applies", `typeset -a a[1]=" (v) "`, `[] [ (v) ] [] n=1`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src+read, func(r *Runner) {
				s := declareElementSemantics()
				s.DeclarationTakesASubscriptedAppendOperand = Yes
				s.DeclarationRereadsAParenthesizedValue = Yes
				s.ScalarOverACompoundIsAnInconsistentType = No
				r.Semantics = &s
				r.Diagnostics = &Diagnostics{}
			})
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
		})
	}
}

// And nothing is dropped in a dialect that does not re-read a parenthesized
// value at all: there is nothing for the letter to drop the subscript *for*,
// so the characters go where the subscript says.
func TestADialectThatDoesNotRereadKeepsTheSubscript(t *testing.T) {
	out, _ := run(t, `typeset -a a[1]="(v)"; echo "[${a[0]-}] [${a[1]-}]"`, func(r *Runner) {
		s := declareElementSemantics()
		s.DeclarationRereadsAParenthesizedValue = No
		s.ScalarOverACompoundIsAnInconsistentType = No
		r.Semantics = &s
		r.Diagnostics = &Diagnostics{}
	})
	if want := `[] [(v)]`; strings.TrimSpace(out) != want {
		t.Errorf("output %q, want %q", strings.TrimSpace(out), want)
	}
}
