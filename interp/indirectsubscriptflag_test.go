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

// Semantics.IndirectionIsTheSubscriptFlag: the `!` of `${!name}` read as
// zsh's `(k)` expansion flag rather than as an indirection or as the name.
//
// Tests name the axis and never a shell.

// subscriptFlagState is what every row below reads.
const subscriptFlagState = `s=SVAL; SVAL=DEEP; n=s; empty=` + "\n" +
	`a=(zero one two)` + "\n" +
	`typeset -A m; m[ka]=va; m[kb]=vb` + "\n" +
	`typeset -A w; w[k]=tgt; tgt=HELLO` + "\n"

// subscriptFlagRun runs one script with the axis at the given answer.
//
// The dialect carries the indirection, the flag group and subscripts, because
// the rows write all three — and a `(k)` written *in the source* is what the
// rewritten sigil is compared against.
func subscriptFlagRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamIndirection = true
	d.ParamExpansionFlags = true
	f, err := syntax.Parse(subscriptFlagState+src+"\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	sem := testSemantics()
	sem.IndirectionIsTheSubscriptFlag = a
	sem.IndirectionYieldsName = No
	r := newTestRunner(t, &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out})
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), st
}

// The twenty written expansions the axis was measured over, each as the pair
// of spellings the measurement compared: the `!` sigil and the `(k)` flag.
//
// The grid varies the parameter's **kind** (scalar, indexed array, table,
// unset), the **subscript** (none, a specific one, out of range, negative,
// arithmetic, `[@]`, `[*]`) and the **operator** (a trim, a slice, a `-`
// word), which is the noun the rule is keyed on.
var subscriptFlagRows = []struct {
	name string
	// written is the text after the sigil or the flag, so `s` becomes
	// `${!s}` and `${(k)s}`.
	written string
}{
	{"a scalar", "s"},
	{"a scalar naming another", "n"},
	{"a scalar holding nothing", "empty"},
	{"a name nothing set", "nosuch"},
	{"a bare array", "a"},
	{"a bare table", "m"},
	{"the first element", "a[0]"},
	{"a later element", "a[1]"},
	{"an element out of range", "a[9]"},
	{"a negative subscript", "a[-1]"},
	{"a subscript that is an expression", "a[1+1]"},
	{"a key the table has", "m[ka]"},
	{"a key the table has not", "m[zz]"},
	{"an array's whole-list spelling", "a[*]"},
	{"a table's whole-list spelling", "m[*]"},
	// The two rows the listing turns on, and they are the sharpest in the
	// grid: a table answers with its **keys** and an ordinary array with its
	// **values**, so a reading that answered "the subscripts" for both would
	// pass every other row here and fail these.
	{"an array's listing", "a[@]"},
	{"a table's listing", "m[@]"},
	{"an operator after a table's listing", "w[@]#H"},
	{"a slice after a table's listing", "w[@]:1:2"},
	{"a word behind a missing key", "m[zz]-MIS"},
	{"a word behind an element out of range", "a[9]-MIS"},
	{"a trim over an array's listing", "a[@]#z"},
}

// TestTheSigilIsTheSubscriptFlagWhereTheAxisSaysSo is the axis's own row, and
// it is the measurement written as a test: each shape is asked twice in one
// script, once with the `!` and once with the flag the `!` stands for.
func TestTheSigilIsTheSubscriptFlagWhereTheAxisSaysSo(t *testing.T) {
	for _, row := range subscriptFlagRows {
		t.Run(row.name, func(t *testing.T) {
			src := `printf "[%s]" "${!` + row.written + `}"; echo` + "\n" +
				`printf "[%s]" "${(k)` + row.written + `}"; echo`
			out, st := subscriptFlagRun(t, Yes, src)
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 2 || st != 0 {
				t.Fatalf("out %q status %d, want two lines at 0", out, st)
			}
			if lines[0] != lines[1] {
				t.Errorf("${!%s} = %q, ${(k)%s} = %q — the sigil is the flag",
					row.written, lines[0], row.written, lines[1])
			}
		})
	}
}

// And the control the grid needs: the flag is **not** the plain expansion, so
// the rows above are saying something.
//
// Without this every row is equally well explained by "the `!` does nothing at
// all", which is true of the four rows with no subscript and false of the
// rest — the shape of a grid keyed on the wrong noun.
func TestTheSubscriptFlagIsNotThePlainExpansion(t *testing.T) {
	for _, tc := range []struct{ name, written, sigil, plain string }{
		{"the first element", "a[0]", "[0]", "[zero]"},
		{"a key the table has", "m[ka]", "[ka]", "[va]"},
		{"a table's whole-list spelling", "m[*]", "[ka kb]", "[va vb]"},
		{"an operator after a table's listing", "w[@]#H", "[k]", "[tgt]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := subscriptFlagRun(t, Yes,
				`printf "[%s]" "${!`+tc.written+`}"`)
			if out != tc.sigil {
				t.Errorf("${!%s} = %q, want %q", tc.written, out, tc.sigil)
			}
			out, _ = subscriptFlagRun(t, Yes,
				`printf "[%s]" "${`+tc.written+`}"`)
			if out != tc.plain {
				t.Errorf("${%s} = %q, want %q", tc.written, out, tc.plain)
			}
		})
	}
}

// With the axis at No the same text is an **indirection**, which is what says
// the axis is live rather than the rewrite being the only reading there is.
func TestWithoutTheAxisTheSigilIsAnIndirection(t *testing.T) {
	// `n` holds `s`, and `s` holds SVAL: the indirection reads twice and the
	// flag reads the subscript, so the two answers cannot be confused.
	if out, _ := subscriptFlagRun(t, No, `printf "[%s]" "${!n}"`); out != "[SVAL]" {
		t.Errorf("with the axis No: ${!n} = %q, want the indirection's [SVAL]", out)
	}
	if out, _ := subscriptFlagRun(t, Yes, `printf "[%s]" "${!n}"`); out != "[s]" {
		t.Errorf("with the axis Yes: ${!n} = %q, want the flag's [s]", out)
	}
	// And the listing, which the two answers also part on: the indirection
	// reads the array's values as names, and the flag answers the keys.
	if out, _ := subscriptFlagRun(t, Yes, `printf "[%s]" "${!m[@]}"`); out != "[ka][kb]" {
		t.Errorf("with the axis Yes: ${!m[@]} = %q, want the keys", out)
	}
}

// The prefix listing is not this axis's, and the axis is not asked for it:
// the shell this reading belongs to has no such spelling, and the two that do
// answer No. A dialect that has never answered still reads `${!ZQ_@}`.
func TestThePrefixListingDoesNotAskTheAxis(t *testing.T) {
	d := syntax.Core()
	d.ParamIndirection = true
	d.ParamIndirectionPrefixListing = true
	f, err := syntax.Parse("ZQ_a=1; ZQ_b=2; echo ${!ZQ_@}\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	// Deliberately unanswered: the point is that nothing here asks.
	sem := testSemantics()
	sem.IndirectionIsTheSubscriptFlag = Unspecified
	r := newTestRunner(t, &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "ZQ_a ZQ_b" {
		t.Errorf("got %q, want the prefix listing with the axis never asked", got)
	}
}

// The rewrite is made on a **copy**: the tree is the program, and the same
// node is expanded again under whatever answer is in force then.
//
// **One tree, two answers**, which is the only shape that can see it: a test
// that parses afresh for each answer hands the second run a node the first
// never touched, and a node written through is then invisible. The tree here
// is parsed once and run twice, the second time with the axis the other way.
func TestTheRewriteLeavesTheTreeAlone(t *testing.T) {
	d := syntax.Core()
	d.ParamIndirection = true
	d.ParamExpansionFlags = true
	f, err := syntax.Parse(subscriptFlagState+`printf "[%s]" "${!n}"`+"\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	answer := func(a Answer) string {
		t.Helper()
		var out bytes.Buffer
		sem := testSemantics()
		sem.IndirectionIsTheSubscriptFlag = a
		sem.IndirectionYieldsName = No
		r := newTestRunner(t, &Runner{Semantics: &sem, Dialect: &d, Stdout: &out, Stderr: &out})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run: %v", err)
		}
		return out.String()
	}
	if got := answer(Yes); got != "[s]" {
		t.Fatalf("the flag's run = %q, want [s]", got)
	}
	// The same tree, the other answer. A rewrite that had edited the node
	// would answer `[s]` again — the flag is still on it — where the
	// indirection reads `n`, finds `s`, and reads that.
	if got := answer(No); got != "[SVAL]" {
		t.Errorf("the indirection's run over the same tree = %q, want [SVAL] — "+
			"the rewrite wrote through the tree", got)
	}
	// And back, so the order is not what decided it.
	if got := answer(Yes); got != "[s]" {
		t.Errorf("the flag's run again = %q, want [s]", got)
	}
}
