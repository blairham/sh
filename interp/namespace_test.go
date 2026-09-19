// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// runNamespace runs a snippet with the grammar flag on and nothing else
// changed, so every row is a statement about the construct rather than about
// a preset. The wordings a refusal needs are the fallbacks the code carries,
// which is what lets a dialect-free runner be asked at all.
func runNamespace(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.NamespaceBlock = true
		// A member is a name with a dot in it, so the dialect that has the
		// block needs the dialect that reads one — which is the same shell,
		// and is why every row below sets both.
		d.DottedName = true
		d.ParamIndirection = true
		d.ArrayLiteral = true
		d.ArraySubscript = true
		d.DeclarationUtilities = map[string]bool{"typeset": true}
	}, nil)
}

// A namespace is a name-resolution region over a compound, and the read half
// and the write half are one rule: a bare name reads the member where there
// is one and the plain name otherwise, and a write always makes a member.
//
// The four rows below are the ones that make it a region rather than a
// scope — a scope that *hid* the outer name would fail the first, and a scope
// that was discarded at the closing brace would fail the third.
func TestANamespaceBodyReadsThroughAndWritesIn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=OUTER; namespace ns { echo "[${x-unset}]"; }`, "[OUTER]\n"},
		{`x=OUTER; namespace ns { x=IN; }; echo "[$x]"`, "[OUTER]\n"},
		{`namespace ns { x=1; }; namespace ns { echo "[${x-unset}]"; }`, "[1]\n"},
		{`namespace ns { x=1; }; namespace n2 { echo "[${x-unset}]"; }`, "[unset]\n"},
		// The member is an ordinary name spelled with a leading dot, which is
		// the store this shell already had for a compound's members.
		{`namespace ns { x=1; }; echo "[${.ns.x-unset}]"`, "[1]\n"},
		{`namespace ns { x=1; }; echo "[${ns.x-unset}]"`, "[unset]\n"},
		// A write from outside, through the member's own name, is seen by a
		// later block: the store is the region and there is no second copy.
		{`namespace ns { x=1; }; .ns.x=2; namespace ns { echo "[$x]"; }`, "[2]\n"},
		{`namespace ns { x=1; }; unset .ns.x; echo "[${.ns.x-unset}]"`, "[unset]\n"},
		// And `unset` of the namespace takes the members with it, the way it
		// does for a compound variable's.
		{`namespace ns { x=1; }; unset .ns; echo "[${.ns.x-unset}]"`, "[unset]\n"},
	} {
		if out, st := runNamespace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The fall-through is live and reaches from outside too, which is what says
// the region is one rule and not a copy made at the opening brace.
func TestANamespaceReadsThroughToThePlainName(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`gv=GLOBAL; namespace ns { y=1; }; echo "[${.ns.gv-unset}]"`, "[GLOBAL]\n"},
		{`gv=GLOBAL; namespace ns { y=1; }; echo "[${.ns.zzz-unset}]"`, "[unset]\n"},
		{`gv=A; namespace ns { y=1; }; gv=B; echo "[${.ns.gv}]"`, "[B]\n"},
		{`gv=G; namespace ns { gv=M; }; echo "[$gv][${.ns.gv}]"`, "[G][M]\n"},
		// And a namespace the shell has never seen is an ordinary dotted
		// name, which is what keeps the fall-through from reaching every
		// `.a.b` a script writes.
		{`gv=G; echo "[${.nope.gv-unset}]"`, "[unset]\n"},
	} {
		if out, st := runNamespace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The region is **lexical**, and these are the two rows that say so. A
// function defined outside and called from inside reads the caller's names; a
// function defined inside and called from outside reads the namespace's. A
// region saved and restored around a call would answer the first wrong, and
// one attached to the call site would answer the second wrong.
func TestANamespaceRegionIsLexical(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			`x=OUTER; g(){ echo "[${x-unset}]"; x=FROMG; }; namespace ns { g; }; echo "[$x]"`,
			"[OUTER]\n[FROMG]\n",
		},
		{`namespace ns { x=1; f(){ echo "[${x-unset}]"; }; }; .ns.f`, "[1]\n"},
		{`namespace ns { x=1; }; h(){ echo "[${x-unset}]"; }; namespace ns { h; }`, "[unset]\n"},
		// A function defined inside is stored under the member name, so it is
		// callable through it and not by its bare name.
		{`namespace ns { f(){ echo hi; }; }; .ns.f`, "hi\n"},
		{`namespace ns { f(){ echo IN; }; f; }`, "IN\n"},
		{`g(){ echo OUT; }; namespace ns { g; }`, "OUT\n"},
		// And its writes are the region's.
		{`namespace ns { f(){ y=IN; }; }; .ns.f; echo "[${y-unset}][${.ns.y-unset}]"`, "[unset][IN]\n"},
	} {
		if out, st := runNamespace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// Namespaces are flat however the blocks nest, because the region is replaced
// rather than appended to.
func TestNamespacesAreFlat(t *testing.T) {
	const src = `namespace a { namespace b { x=1; }; }; echo "[${.a.b.x-unset}][${.b.x-unset}]"`
	if out, st := runNamespace(t, src); out != "[unset][1]\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "[unset][1]\n")
	}
}

// Everything a name can hold goes in the region, not only a scalar: the
// declaration commands, a loop's variable, an array, and an export — which is
// the row that says a namespace is not a way to reach the environment.
func TestANamespaceHoldsEveryKindOfName(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`namespace ns { typeset t=1; }; echo "[${.ns.t-unset}]"`, "[1]\n"},
		{`namespace ns { a=(p q); }; echo "[${.ns.a[1]-unset}]"`, "[q]\n"},
		{`namespace ns { for i in a b; do :; done; }; echo "[${.ns.i-unset}][${i-unset}]"`, "[b][unset]\n"},
		{`namespace ns { typeset -x E=1; }; echo "[${.ns.E-unset}]"`, "[1]\n"},
	} {
		if out, st := runNamespace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A subshell that opens a namespace leaves nothing behind, which falls out of
// the members being ordinary names and the set of namespaces being a table a
// clone owns.
func TestANamespaceMadeInASubshellDoesNotLeak(t *testing.T) {
	const src = `( namespace ns { x=1; } ); echo "[${.ns.x-unset}]"`
	if out, st := runNamespace(t, src); out != "[unset]\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "[unset]\n")
	}
}

// A word that is no name is refused when the clause runs, not when the file
// is read, and which sentence is used is decided by the word. See
// Diagnostics.NamespaceNameNotAVariable for the rows.
func TestANamespaceNameIsJudgedWhenTheClauseRuns(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`namespace .ns { x=1; }; echo after`, ".ns: is not an identifier\n"},
		{`namespace a.b { x=1; }; echo after`, "a.b: is not an identifier\n"},
		{`namespace 1x { x=1; }; echo after`, ".1x: invalid variable name\n"},
		{`namespace a-b { x=1; }; echo after`, ".a-b: invalid variable name\n"},
		{`namespace a[1] { x=1; }; echo after`, ".a[1]: cannot be an array\n"},
	} {
		// Fatal: the `echo` never runs, which is what makes this a stage and
		// not a wording. The status and the name in front of the sentence are
		// the dialect's ordinary fatal ones and are asserted in dialect/ksh,
		// where the column that has the construct is.
		out, st := runNamespace(t, c.src)
		if !strings.HasSuffix(out, c.want) || strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s\n got %q at %d\nwant it to end %q and stop", c.src, out, st, c.want)
		}
	}
}

// The keys of a namespace are the members it holds **and** the names it reads
// through to, which is the listing half of the fall-through. Measured on the
// column that has the construct: `gv=GLOBAL; namespace ns { x=1; }` then
// `${!.ns.@}` answers the shell's own parameters and `gv` and `x`, every one
// of them spelled `.ns.…`.
//
// Asserted as a membership rather than as the whole line, because which
// parameters a shell has of its own is not this construct's question and
// differs by dialect.
func TestANamespaceListsWhatItReadsThroughTo(t *testing.T) {
	out, st := runNamespace(t, `gv=G; namespace ns { x=1; }; echo "[${!.ns.@}]"`)
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	for _, want := range []string{".ns.gv", ".ns.x"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q does not list %s", out, want)
		}
	}
	// And a namespace nothing declared lists nothing, which is what keeps
	// this off every dotted name a script writes.
	if out, _ := runNamespace(t, `gv=G; echo "[${!.nope.@}]"`); out != "[]\n" {
		t.Errorf("an undeclared namespace listed %q, want []", out)
	}
}
