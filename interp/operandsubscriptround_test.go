// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subscript that reaches a *builtin* as text — `unset 'a[$k]'`, `printf -v
// 'c[$k]'`, `read 'b[$k]'`, `test -v 'g[$k]'` — and the second round it gets
// where the dialect makes one.
//
// Three axes rather than one, because the panel splits them: one column
// rounds at every surface here, another rounds at all of them but `unset`,
// and a third rounds at none. And a switch across all three, because one
// column lets a script turn the round off by name and none of them lets a
// script move the two neighboring surfaces — a declaration's operand and the
// `[[ -v ]]` keyword — which round under axes of their own.
//
// Tests name axes and wordings, never shells.

// roundGrammar is what these operands need: brackets in a subscript, a
// declaration utility to make the table with, the `-v` operator, and the
// here-string `read` is measured through.
func roundGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
	d.ParameterIsSetTest = true
	d.DoubleBracket = true
	d.Herestring = true
}

// roundAxes answers the three axes and the two neighbors, and says whether
// the session has asked the round to stop.
//
// The neighbors are answered `Yes` throughout rather than left alone,
// because the assertion that matters most here is that the switch does *not*
// reach them: a switch wired one level too wide would move them too, and a
// test that left them off could not tell.
func roundAxes(unset, output, isSet Answer, stopped bool) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.UnsetExpandsAFlatSubscript = unset
		s.OutputOperandExpandsAFlatSubscript = output
		s.TestIsSetExpandsAFlatSubscript = isSet
		s.DeclarationOperandExpandsItsSubscript = Yes
		s.ConditionIsSetExpandsAFlatSubscript = Yes
		// The three the probes need to exist at all, none of which is the
		// subject: a quoted key has to be readable for `${m['$k']}` to say
		// which of the two keys was written, `printf` has to have the letter
		// that stores, and a subscripted operand has to reach the
		// declaration for the neighbor control below to have a row.
		s.SubscriptIsAQuotingContext = Yes
		s.PrintfAssignsWithV = Yes
		s.SubscriptedAssignmentPrefix = SubscriptedPrefixStoresTheElement
		r.Semantics = &s
		if stopped {
			r.SetExpandsAnOperandsSubscriptAgain(false)
		}
	}
}

// The three surfaces, each under its own axis, with the other two answered
// the opposite way so that a row can only be moved by the axis it names.
func TestABuiltinsOperandRoundsItsSubscriptByAxis(t *testing.T) {
	const table = "typeset -A m; k='x y'; m[$k]=hello; "
	for _, tc := range []struct {
		name     string
		src      string
		rounded  string
		standing string
		axis     func(Answer) func(*Runner)
	}{
		{
			// `unset` names an element to take away rather than one to write,
			// and is the surface the columns part on.
			name:     "unset",
			src:      table + `unset 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`,
			rounded:  "[GONE]",
			standing: "[hello]",
			axis: func(a Answer) func(*Runner) {
				return roundAxes(a, No, No, false)
			},
		},
		{
			name:     "read through a name operand",
			src:      "typeset -A m; k='x y'; " + `read 'm[$k]' <<<Z; printf '[%s][%s]' "${m[$k]-U}" "${m['$k']-U}"`,
			rounded:  "[Z][U]",
			standing: "[U][Z]",
			axis: func(a Answer) func(*Runner) {
				return roundAxes(No, a, No, false)
			},
		},
		{
			name:     "printf through a name operand",
			src:      "typeset -A m; k='x y'; " + `printf -v 'm[$k]' P; printf '[%s][%s]' "${m[$k]-U}" "${m['$k']-U}"`,
			rounded:  "[P][U]",
			standing: "[U][P]",
			axis: func(a Answer) func(*Runner) {
				return roundAxes(No, a, No, false)
			},
		},
		{
			name:     "the is-set operator",
			src:      table + `if test -v 'm[$k]'; then printf T; else printf F; fi`,
			rounded:  "T",
			standing: "F",
			axis: func(a Answer) func(*Runner) {
				return roundAxes(No, No, a, false)
			},
		},
		{
			// The same operator in its bracket spelling, which is the same
			// builtin and must not have drifted.
			name:     "the is-set operator spelled with brackets",
			src:      table + `if [ -v 'm[$k]' ]; then printf T; else printf F; fi`,
			rounded:  "T",
			standing: "F",
			axis: func(a Answer) func(*Runner) {
				return roundAxes(No, No, a, false)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runGrammar(t, tc.src, roundGrammar, tc.axis(Yes)); out != tc.rounded || st != 0 {
				t.Errorf("rounding: %q (status %d), want %q at 0", out, st, tc.rounded)
			}
			if out, st := runGrammar(t, tc.src, roundGrammar, tc.axis(No)); out != tc.standing || st != 0 {
				t.Errorf("standing: %q (status %d), want %q at 0", out, st, tc.standing)
			}
		})
	}
}

// The control the rows above need: an operand with nothing in it for a round
// to do reaches the same element under either answer.
//
// Without it, a shell that had simply stopped reading a quoted subscript
// would pass the standing half of every row — the element would survive
// `unset` and the key would not be found — for a reason that has nothing to
// do with the round.
func TestAnOperandWithNothingToExpandIsTheSameElementEitherWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unset", "typeset -A m; m[plain]=hello; unset 'm[plain]'; printf '[%s]'" + ` "${m[plain]-GONE}"`, "[GONE]"},
		{"read", "typeset -A m; read 'm[plain]' <<<Z; printf '[%s]'" + ` "${m[plain]-U}"`, "[Z]"},
		{"printf", "typeset -A m; printf -v 'm[plain]' P; printf '[%s]'" + ` "${m[plain]-U}"`, "[P]"},
		{"is-set on a key that is there", "typeset -A m; m[plain]=hello; if test -v 'm[plain]'; then printf T; else printf F; fi", "T"},
		{"is-set on a key that is not", "typeset -A m; m[plain]=hello; if test -v 'm[nope]'; then printf T; else printf F; fi", "F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No} {
				out, st := runGrammar(t, tc.src, roundGrammar, roundAxes(a, a, a, false))
				if out != tc.want || st != 0 {
					t.Errorf("%v: %q (status %d), want %q at 0", a, out, st, tc.want)
				}
			}
		})
	}
}

// The switch withholds the round at all three surfaces at once, and a shell
// that was never told about it rounds wherever its axes say.
func TestASessionCanWithholdABuiltinsSecondRound(t *testing.T) {
	const table = "typeset -A m; k='x y'; m[$k]=hello; "
	for _, tc := range []struct{ name, src, rounding, withheld string }{
		{"unset", table + `unset 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`, "[GONE]", "[hello]"},
		{"read", "typeset -A m; k='x y'; " + `read 'm[$k]' <<<Z; printf '[%s][%s]' "${m[$k]-U}" "${m['$k']-U}"`, "[Z][U]", "[U][Z]"},
		{"printf", "typeset -A m; k='x y'; " + `printf -v 'm[$k]' P; printf '[%s][%s]' "${m[$k]-U}" "${m['$k']-U}"`, "[P][U]", "[U][P]"},
		{"is-set", table + `if test -v 'm[$k]'; then printf T; else printf F; fi`, "T", "F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runGrammar(t, tc.src, roundGrammar, roundAxes(Yes, Yes, Yes, false)); out != tc.rounding {
				t.Errorf("permitted: %q, want %q", out, tc.rounding)
			}
			if out, _ := runGrammar(t, tc.src, roundGrammar, roundAxes(Yes, Yes, Yes, true)); out != tc.withheld {
				t.Errorf("withheld: %q, want %q", out, tc.withheld)
			}
		})
	}
}

// And it reaches those three and nothing else.
//
// This is the row the switch exists to be scoped by, and it is measured
// rather than tidy: the one column that names the option leaves a
// declaration's operand and the `[[ -v ]]` keyword rounding with the option
// set. A switch wired one level up — at the shared round rather than at the
// three sites — would move them too, and every other test in this file would
// still pass.
//
// The keyword is the row that can be asked here. The other neighbor, a
// declaration's subscripted operand, needs more of a vector than this file
// builds and is asserted where the whole vector is real — see
// TestTheOptionLeavesTheNeighbouringSurfacesRounding in dialect/bash.
func TestWithholdingTheRoundLeavesTheNeighbouringSurfacesAlone(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the is-set keyword",
			`typeset -A m; k='x y'; m[$k]=hello; if [[ -v 'm[$k]' ]]; then printf T; else printf F; fi`,
			"T",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stopped := range []bool{false, true} {
				out, st := runGrammar(t, tc.src, roundGrammar, roundAxes(Yes, Yes, Yes, stopped))
				if out != tc.want || st != 0 {
					t.Errorf("withheld=%v: %q (status %d), want %q at 0", stopped, out, st, tc.want)
				}
			}
		})
	}
}
