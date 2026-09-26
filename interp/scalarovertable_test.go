// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.ScalarStoredOverATableIsRefused, both ways round over one source.
//
// `typeset -A m=([k]=v); m=x` is a scalar landing on a name that is holding a
// table. One reading refuses it and gives up the input, leaving the table
// exactly as it was; the other goes on to
// Semantics.ScalarAssignedOverACompoundReplacesTheName and writes the key `0`.
//
// The listing is the instrument and not `$m`, for the reason
// scalarovercompound_test.go gives: a bare read spells the same thing under
// more than one of the answers, and only the listing says whether the table is
// still there.
func scalarOverTableSem(refuses Answer) Semantics {
	s := testSemantics()
	s.ScalarStoredOverATableIsRefused = refuses
	return s
}

const scalarOverTableRefusal = "m: attempt to set associative array to scalar"

// The two answers for each of the two spellings, which is the pair that says
// the axis is keyed on the **name's kind** and not on the operator.
func TestAScalarStoredOverATable(t *testing.T) {
	for _, c := range []struct {
		name, decl, store string
		refused, taken    string
	}{
		{
			"a plain assignment",
			`typeset -A m=([k]=v)`, `m=x`,
			`declare -A m=([k]="v" )`,
			`declare -A m=([0]="x" [k]="v" )`,
		},
		{
			"an append",
			`typeset -A m=([k]=v)`, `m+=x`,
			`declare -A m=([k]="v" )`,
			`declare -A m=([0]="x" [k]="v" )`,
		},
		{
			"an append onto the base key, which the taking answer joins",
			`typeset -A m=([0]=pre)`, `m+=x`,
			`declare -A m=([0]="pre" )`,
			`declare -A m=([0]="prex" )`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.decl + "; " + c.store + "; typeset -p m; echo after"
			out, st := run(t, src, withSem(scalarOverTableSem(Yes)))
			if !strings.Contains(out, scalarOverTableRefusal) || st == 0 {
				t.Errorf("refusing: got %q status %d, want the refusal at non-zero", out, st)
			}
			if strings.Contains(out, "after") {
				t.Errorf("refusing: got %q, want the input given up", out)
			}
			// The table is left exactly as it was, which a subshell is what
			// makes readable here: the refusal ends that child and the file
			// above it runs on. What it costs outside one is the assertion
			// above; the dialect suites carry the measured `eval` shape.
			out, st = run(t, c.decl+"; ("+c.store+"); typeset -p m",
				withSem(scalarOverTableSem(Yes)))
			if !strings.HasSuffix(strings.TrimSpace(out), c.refused) || st != 0 {
				t.Errorf("refusing, contained: got %q status %d, want it to end %q at 0", out, st, c.refused)
			}

			out, st = run(t, src, withSem(scalarOverTableSem(No)))
			want := c.taken + "\nafter"
			if got := strings.TrimSpace(out); got != want || st != 0 {
				t.Errorf("taking: got %q status %d, want %q at 0", got, st, want)
			}
		})
	}
}

// **The noun is the name holding a table**, and these are the cases that hold
// the refusing answer fixed and move one other thing. Every one of them is
// taken, and a rule written on a wider noun — "a compound name", "an
// assignment to a declared name" — refuses one of them.
func TestOnlyABareScalarStoreOverATableIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"an ordinary array, which is the pair that makes this its own axis",
			`a=(1 2); a=x; typeset -p a`,
			`declare -a a=([0]="x" [1]="2")`,
		},
		{
			"a subscript, which reaches the element it names",
			`typeset -A m=([k]=v); m[k]=x; typeset -p m`,
			`declare -A m=([k]="x" )`,
		},
		{
			"a name that is no longer holding one",
			`typeset -A m=([k]=v); unset m; m=x; typeset -p m`,
			`declare -- m="x"`,
		},
		{
			"a name that never held one",
			`s=plain; s=x; typeset -p s`,
			`declare -- s="x"`,
		},
		{
			"a compound store, which is not a scalar one",
			`typeset -A m=([k]=v); m=(); typeset -p m`,
			`declare -A m=()`,
		},
		{
			"a compound append",
			`typeset -A m=([k]=v); m+=([j]=w); typeset -p m`,
			`declare -A m=([j]="w" [k]="v" )`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src, withSem(scalarOverTableSem(Yes)))
			if got := strings.TrimSpace(out); got != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", got, st, c.want)
			}
		})
	}
}

// It is asked wherever a scalar is **stored** and not only at an assignment
// statement, which is the reach
// Semantics.ScalarAssignedOverACompoundReplacesTheName already has and the
// reason both live at the store. Each of these sets a name by a route that is
// not an assignment at all.
func TestEveryScalarStoreOverATableIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a for loop's variable", `typeset -A m=([k]=v); for m in x y; do :; done; echo after`},
		{"read", "typeset -A m=([k]=v)\nread m <<'IN'\nx\nIN\necho after"},
		{"an assigning expansion", `typeset -A m=([k]=v); : ${m:=x}; echo after`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, withSem(scalarOverTableSem(Yes)))
			if !strings.Contains(out, scalarOverTableRefusal) {
				t.Errorf("got %q, want the refusal", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the input given up", out)
			}
		})
	}
}

// The refusal is written **before** anything is stored, which is what makes
// "the table is left standing" a claim about order rather than about a
// restore: a store that wrote the key first and complained second would list
// the same table with one extra key in it.
func TestARefusedScalarStoreOverATableWritesNothing(t *testing.T) {
	out, st := run(t,
		`typeset -A m=([k]=v); (m=x); printf '[%s][%s][%s]' "${#m[@]}" "${m[k]}" "${m[0]-ABSENT}"`,
		withSem(scalarOverTableSem(Yes)))
	if want := "[1][v][ABSENT]"; !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf("got %q status %d, want it to end %q at 0", out, st, want)
	}
}

// The wording is the dialect's and the name is its one verb, so a dialect that
// words it differently still says which variable.
func TestTheRefusalTakesTheDialectsWording(t *testing.T) {
	sem := scalarOverTableSem(Yes)
	d := Diagnostics{ScalarStoredOverATable: "cannot make %s a scalar"}
	out, st := run(t, `typeset -A m=([k]=v); m=x`, func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &d
	})
	if want := "cannot make m a scalar"; !strings.Contains(out, want) || st == 0 {
		t.Errorf("got %q status %d, want %q at non-zero", out, st, want)
	}
}

// Unanswered is said rather than guessed, for both spellings — the append
// reaches the field from Runner.assign and the plain store from
// Runner.scalarOverCompound, and a route that skipped the question would store
// in silence.
func TestAnUnansweredScalarStoreOverATableSaysSo(t *testing.T) {
	for _, src := range []string{
		`typeset -A m=([k]=v); m=x; typeset -p m`,
		`typeset -A m=([k]=v); m+=x; typeset -p m`,
	} {
		out, _ := run(t, src, withSem(scalarOverTableSem(Unspecified)))
		if !strings.Contains(out, "a scalar store over a name holding a table being refused") {
			t.Errorf("%s: got %q, want the axis named", src, out)
		}
		// And nothing was stored under it, which is the half a route that
		// skipped the question would have got wrong in silence.
		if strings.Contains(out, "x") {
			t.Errorf("%s: got %q, want no value stored", src, out)
		}
	}
}
