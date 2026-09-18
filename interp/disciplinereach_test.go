// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// How far a variable's hooks reach. Two questions that look like one — which
// reads run a `.get`, and how long a `.set` goes on watching a name — and a
// third about the cell a prefix in front of a call writes (#3121, #3161,
// #3162).

// runDisciplined runs src with the hooks on and the two constructs these
// cases need to say what they mean: `[[ ]]`, whose `-v` is a set-ness test
// with no value in it, and the `function` keyword.
func runDisciplined(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, disciplineGrammar, func(r *Runner) {
		sem := disciplineSemantics()
		sem.PrefixToAKeywordFunctionIsScopedToTheCall = No
		r.Semantics = &sem
	})
}

// disciplineGrammar is the three constructs these cases need to say what they
// mean: `[[ -v ]]`, which is a set-ness test with no value in it, the
// `function` keyword, whose two spellings one column scopes differently, and
// an array literal, so a container can be removed.
func disciplineGrammar(d *syntax.Dialect) {
	d.DoubleBracket = true
	d.ParameterIsSetTest = true
	d.FunctionKeyword = true
	d.ArrayLiteral = true
	// And the dot in a name, which is what lets a hook read and write the
	// parameter it is entered with.
	d.DottedName = true
}

func disciplineSemantics() Semantics {
	sem := CoreSemantics()
	sem.PunctuatedFunctionNameIsRefused = Yes
	sem.DisciplineFunctionIsAVariableHook = Yes
	return sem
}

// A **set-ness test** asks the store, so the value's producer is not run for
// it. The left column is the whole of what keeps that narrow.
func TestASetnessTestDoesNotRunTheValuesProducer(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		fires     bool
	}{
		{"a colon-less alternate", `echo "[${x+S}]"`, false},
		{"one with an empty operand", `echo "[${x+}]"`, false},
		{"a bracketed set-ness test", `[[ -v x ]]; echo "st=$?"`, false},
		{"the colon form, which tests the value", `echo "[${x:+S}]"`, true},
		{"a default, which yields the value", `echo "[${x-D}]"`, true},
		{"a length, which counts it", `echo "[${#x}]"`, true},
		{"a trim, which shapes it", `echo "[${x#V}]"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runDisciplined(t, "function x.get { echo GET; }\nx=V\n"+tc.src)
			if got := strings.Contains(out, "GET"); got != tc.fires {
				t.Errorf("out %q, want the hook to fire=%v", out, tc.fires)
			}
		})
	}
	// And the set-ness test still answers, so the rows above are about the
	// hook and not about a read that stopped happening.
	out, _ := runDisciplined(t, "function x.get { echo GET; }\nx=V\necho \"[${x+S}][${y+S}]\"")
	if !strings.Contains(out, "[S][]") {
		t.Errorf("out %q does not answer the set-ness test", out)
	}
	// A test written *inside* one is its own question, which is why the mark
	// is restored rather than cleared.
	out, _ = runDisciplined(t, "function x.get { echo GET; }\nx=V\necho \"[${x+${x}}]\"")
	if strings.Count(out, "GET") != 1 || !strings.Contains(out, "[V]") {
		t.Errorf("out %q, want exactly one read from the inner expansion", out)
	}
}

// An `unset` that removes a variable takes its hooks with it: the binding is
// to the variable and not to the name.
func TestUnsettingAVariableDiscardsItsDisciplines(t *testing.T) {
	out, _ := runDisciplined(t, `s=1
function s.set { echo SET; }
function plain { :; }
unset s
s=2
echo "after=[$s]"`)
	if strings.Contains(out, "SET") {
		t.Errorf("out %q still fires a hook the removal took away", out)
	}
	if !strings.Contains(out, "after=[2]") {
		t.Errorf("out %q lost the store the hook was watching", out)
	}
	// A name **nothing has set** is the row that says this is the variable
	// going rather than the `unset` word: there was nothing to remove, so
	// nothing is unbound.
	out, _ = runDisciplined(t, "function s.set { echo SET; }\nunset s\ns=1")
	if !strings.Contains(out, "SET") {
		t.Errorf("out %q unbound a hook over a removal that removed nothing", out)
	}
	// And the definition is what binds, so writing it again after the
	// removal binds it again.
	out, _ = runDisciplined(t, `s=1
function s.set { echo FIRST; }
unset s
s=2
function s.set { echo SECOND; }
s=3`)
	if strings.Contains(out, "FIRST") || !strings.Contains(out, "SECOND") {
		t.Errorf("out %q, want only the hook defined after the removal", out)
	}
	// The `.unset` hook runs first, on the value it is about to lose — and
	// a container reaches it, which a scalar read of the store does not.
	out, _ = runDisciplined(t, "m=(p q)\nfunction m.unset { echo MUNSET; }\nunset m\necho done")
	if !strings.Contains(out, "MUNSET") {
		t.Errorf("out %q does not run the removal's hook for a container", out)
	}
}

// A prefix in front of a `function`-form function may write a cell belonging
// to the **call**, and the append is what makes that visible: there is
// nothing in a fresh cell to append to.
func TestAPrefixToAKeywordFunctionCanBelongToTheCall(t *testing.T) {
	src := `s=base
function s.set { echo SET; }
function s.append { echo APP; }
function kf { echo "kf=[$s]"; }
s+=5 kf
echo "after=[$s]"`
	scoped := func(r *Runner) {
		sem := disciplineSemantics()
		sem.PrefixToAKeywordFunctionIsScopedToTheCall = Yes
		sem.AssignmentPrefixPersistsAfterAFunction = Yes
		r.Semantics = &sem
	}
	out, _ := runGrammar(t, src, disciplineGrammar, scoped)
	if !strings.Contains(out, "kf=[5]") {
		t.Errorf("out %q appends to a cell the call was supposed to make fresh", out)
	}
	if !strings.Contains(out, "after=[base]") {
		t.Errorf("out %q leaves the call's own cell behind", out)
	}
	// Nothing was watching that cell, so the store fires no hook — which is
	// the same rule the persisting answer reads from the other side.
	if strings.Contains(out, "SET") || strings.Contains(out, "APP") {
		t.Errorf("out %q fires a hook for a store into a cell the call just made", out)
	}
	// The other answer, which is the control: the same source with the cell
	// the shell's own appends to what the shell holds, fires the event the
	// operator names, and — under the persisting answer beside it — keeps it.
	out, _ = runDisciplined(t, src)
	if !strings.Contains(out, "kf=[base5]") || !strings.Contains(out, "APP") {
		t.Errorf("out %q, want the shell's own cell appended to and the hook fired", out)
	}
}
