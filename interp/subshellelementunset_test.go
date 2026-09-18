// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// In one column an element removed from an array a subshell has only been
// **looking at** takes the whole array with it, for the rest of that subshell.
// The parent's copy is untouched either way, so the window is narrow — and
// the direction is this shell keeping data the real one loses (#3517).

// The axis itself, and the parent's array behind it.
func TestAnElementUnsetInASubshellEmptiesAnArrayItOnlyInherited(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`( unset 'a[1]'; echo "in=[${a[*]}] n=${#a[@]}" )` + "\n" +
		`echo "after=[${a[*]}]"`
	for _, c := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"emptied", Yes, "in=[] n=0"},
		{"the one element", No, "in=[1 3] n=2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := subshellUnsetRun(t, c.answer, src)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
			// The parent is untouched under both answers, which is what
			// makes this about the subshell's view rather than about the
			// removal.
			if !strings.Contains(out, "after=[1 2 3]") {
				t.Errorf("out %q reached the parent's array", out)
			}
		})
	}
}

// Four controls say what the answer is about, and each one removes a
// candidate: the inherited copy is fine on its own, a write is fine, a write
// *before* the unset makes the unset ordinary, and an array the subshell made
// itself is its own.
func TestWhatAnInheritedArrayIsForThisAnswer(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the copy alone", "a=(1 2 3)\n" + `( echo "in=[${a[*]}]" )`, "in=[1 2 3]"},
		{"a write", "a=(1 2 3)\n" + `( a[0]=9; echo "in=[${a[*]}]" )`, "in=[9 2 3]"},
		{"a write first", "a=(1 2 3)\n" + `( a[0]=9; unset 'a[1]'; echo "in=[${a[*]}]" )`, "in=[9 3]"},
		{"an array of its own", `( b=(1 2 3); unset 'b[1]'; echo "in=[${b[*]}]" )`, "in=[1 3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := subshellUnsetRun(t, Yes, c.src)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
		})
	}
}

// It is **indexed arrays only**: a table's key removal takes the one key, and
// a name that is not an array at all is whatever it already was.
//
// Asserted on the values rather than on `${#m[@]}`, because the column this
// is measured from answers that count 2 inside a subshell and 1 outside it
// with the same one key standing either way — a stale count of its own, in
// the same family as the bookkeeping #3511 is about, and not what this axis
// decides.
func TestATablesKeyIsNotEmptiedBySettingOneOfItsKeysAside(t *testing.T) {
	out, _ := subshellUnsetRun(t, Yes, "typeset -A m; m[k]=v; m[j]=w\n"+
		`( unset 'm[k]'; echo "in=[${m[j]}][${m[k]-gone}]" )`)
	if !strings.Contains(out, "in=[w][gone]") {
		t.Errorf("out %q emptied a table", out)
	}
}

// And it is **two contexts** rather than every subshell: an explicit `( … )`
// and a `$( … )`. Measured 2026-09-17, the column that empties leaves the
// array alone in a background job, at either end of a pipeline and in a
// process substitution — the contexts it forks — and a `( … )` standing as a
// background job's whole body is that fork rather than a subshell of it.
func TestWhichContextsArraysAreAViewIn(t *testing.T) {
	const unset = `unset 'a[1]'; echo "in=[${a[*]}]"`
	for _, c := range []struct{ name, src, want string }{
		{"a subshell", "( " + unset + " )", "in=[]"},
		{"a command substitution", `v=$(` + unset + `); echo "$v"`, "in=[]"},
		{"a subshell inside a pipeline's element", "echo x | { ( " + unset + " ); }", "in=[]"},
		{"a background job", "{ " + unset + "; } &\nwait", "in=[1 3]"},
		{"a subshell as a background job's body", "( " + unset + " ) &\nwait", "in=[1 3]"},
		{"a pipeline's element", "echo x | { " + unset + "; }", "in=[1 3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := subshellUnsetRun(t, Yes, "a=(1 2 3)\n"+c.src)
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
		})
	}
}

// And a **nested** subshell answers for itself: the inner one empties its own
// view and the outer one still has all three.
func TestANestedSubshellEmptiesOnlyItsOwnView(t *testing.T) {
	out, _ := subshellUnsetRun(t, Yes, "a=(1 2 3)\n"+
		`( ( unset 'a[1]'; echo "in2=[${a[*]}]" ); echo "in1=[${a[*]}]" )`)
	if !strings.Contains(out, "in2=[]") {
		t.Errorf("out %q did not empty the inner subshell's view", out)
	}
	if !strings.Contains(out, "in1=[1 2 3]") {
		t.Errorf("out %q reached the outer subshell's array", out)
	}
}

// Nothing of this happens outside a subshell, which is the other half of the
// name: the same line at the top level removes the one element.
func TestAnElementUnsetOutsideASubshellIsUntouchedByTheAnswer(t *testing.T) {
	out, _ := subshellUnsetRun(t, Yes, "a=(1 2 3)\nunset 'a[1]'\n"+`echo "top=[${a[*]}]"`)
	if !strings.Contains(out, "top=[1 3]") {
		t.Errorf("out %q emptied an array at the top level", out)
	}
}

// subshellUnsetRun answers what an `unset` of an element needs and nothing
// else, so a row varies the one answer.
func subshellUnsetRun(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.ArrayBaseIsZero = Yes
		s.UnsetTakesASubscript = Yes
		s.UnsetSubscriptSkippedWhenNameUnset = No
		s.UnsetArraySpan = UnsetArraySpanIsAnExpression
		s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = answer
		s.FatalErrorStatusIsOne = Yes
	}, Diagnostics{}, src, RouteUnspecified)
}
