// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `unset` of a name a **producer** answers for takes the producer with the
// parameter, so the name is an ordinary one afterwards — #4764.
//
// Tests name seams and never shells. The producer here is a table the test
// keeps beside the runner, which is what a dialect's view of its own alias or
// function table is: a reading of something else the shell holds.

// producedTableRunner registers `view` as a produced table whose contents are
// the map the caller still owns, so a row can say whether the *thing* survived
// the parameter going.
func producedTableRunner(t *testing.T, table map[string]string, out *strings.Builder) *Runner {
	t.Helper()
	dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
	sem := Semantics{FatalErrorStatusIsOne: Yes}
	r := newTestRunner(t, &Runner{
		Stdout: out, Stderr: out, Diagnostics: &dg, Semantics: &sem, Name: "testsh",
	})
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		a := AssocArray{}
		for k, v := range table {
			a[k] = Scalar(v)
		}
		return a
	})
	r.SetDynamicAssocWriter("view", func(_ *Runner, key, value string, set bool) {
		if !set {
			delete(table, key)
			return
		}
		table[key] = value
	})
	return r
}

func runProduced(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}

// The two halves that used to disagree: the name's own reading of itself said
// it was gone and the view behind it went on answering every lookup.
func TestUnsetOfAProducedTableTakesTheProducerWithIt(t *testing.T) {
	table := map[string]string{"k": "v"}
	var out strings.Builder
	r := producedTableRunner(t, table, &out)
	runProduced(t, r, `echo "before=[${view[k]}]"
unset view
echo "after=[${view[k]}]"`)
	const want = "before=[v]\nafter=[]\n"
	if out.String() != want {
		t.Errorf("= %q, want %q", out.String(), want)
	}
	// And the control that says what an `unset` of this shape must **not**
	// do: the thing the view was a view of is still there. Delivering the
	// removal to the writer, which is what a produced *array* gets, would
	// have emptied it — and for a shell's alias or function table that is
	// every alias and every function in the shell.
	if table["k"] != "v" {
		t.Errorf("the table behind the view is %v, want the entry still in it", table)
	}
}

// And the name is an ordinary one afterwards, in both directions: a scalar
// stored under it reads back as itself, and the producer is not consulted.
func TestAStoreAfterUnsettingAProducedTableIsOrdinary(t *testing.T) {
	table := map[string]string{"k": "v"}
	var out strings.Builder
	r := producedTableRunner(t, table, &out)
	runProduced(t, r, `unset view
view=string
echo "v=[$view]"`)
	const want = "v=[string]\n"
	if out.String() != want {
		t.Errorf("= %q, want %q", out.String(), want)
	}
}

// The control on the whole of it: without the `unset`, every row above
// answers the other way. A suite that only ran the unset rows could not tell
// the producer having been taken away from its never having been registered.
func TestTheProducedTableAnswersWhileItIsThere(t *testing.T) {
	table := map[string]string{"k": "v"}
	var out strings.Builder
	r := producedTableRunner(t, table, &out)
	// The keyed read alone: `${view+…}` on a table reaches two axes this
	// runner answers for nobody, and a control must not be the thing that
	// draws a diagnostic.
	runProduced(t, r, `echo "k=[${view[k]}] other=[${view[nosuch]}]"`)
	const want = "k=[v] other=[]\n"
	if out.String() != want {
		t.Errorf("= %q, want %q", out.String(), want)
	}
}
