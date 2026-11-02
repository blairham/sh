// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A compound literal here is one shape or the other and never a mixture:
// either every element names a subscript, or none does and a bracket in front
// of an element is text. The *first* element decides which.
//
// Every other shell in the panel places a subscripted element wherever it
// stands and continues the bare ones from it, so `a=(x [3]=y z)` is three
// elements there. We read subscripts everywhere, which made `a=(p [1]=A)` a
// keyed table of two entries — one of them keyed `p` and holding nothing —
// where this shell has a two-element word list (#2505).
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, read back with `typeset -p a`.
func TestTheFirstElementDecidesWhetherSubscriptsAreRead(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// Subscripted throughout: the subscripts are keys and the literal
		// builds the keyed table, which is the shape that already worked.
		{`a=([1]=A [2]=B); typeset -p a`, "typeset -A a=([1]=A [2]=B)\n"},
		// A bare element in front makes every bracket in the literal text.
		{`a=(p [1]=A); typeset -p a`, "typeset -a a=(p '[1]=A')\n"},
		// However far along the bracketed element stands.
		{`a=(p q [1]=A); typeset -p a`, "typeset -a a=(p q '[1]=A')\n"},
		{`a=(p [1]=A [2]=B); typeset -p a`, "typeset -a a=(p '[1]=A' '[2]=B')\n"},
		// A quoted head was never a subscript, so this is a word list by the
		// ordinary rule and not by the shape rule — the control that keeps
		// the two apart.
		{`a=("[1]=A" p); typeset -p a`, "typeset -a a=('[1]=A' p)\n"},
		// A bare element the expansion produced decides the shape just as a
		// written one does: what is read is the word, not its source.
		{`a=($(echo p) [1]=A); typeset -p a`, "typeset -a a=(p '[1]=A')\n"},
	} {
		if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the shape reaches the lexer, because the blanks inside a bracket are
// characters only while a subscript is being read.
//
// `a=([1 2]=A)` is the one key `1 2`, and the same brackets after a bare
// element are three words. The second row is what a fix that only taught the
// *store* about the shape would still get wrong: the element count is
// decided before the store ever sees it.
func TestABareFirstElementStopsTheBlanksBeingCharacters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`a=([1 2]=A); typeset -p a`, "typeset -A a=(['1 2']=A)\n"},
		{`a=(p [1 2]=A); typeset -p a`, "typeset -a a=(p '[1' '2]=A')\n"},
	} {
		if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A bare element *after* a subscripted one is a syntax error, and it is the
// grammar's refusal rather than the store's: the offending element is named
// as it was written, with the quotes off and any expansion unexpanded, and it
// is refused whatever the name was holding.
//
// The `$x` and `$(echo p)` rows are what say so — an expansion that has not
// happened cannot have been refused for its value — and the `typeset -A` row
// says the refusal comes first, since that name's own complaint about being
// given an indexed literal never gets to run.
func TestABareElementAfterASubscriptedOneIsASyntaxError(t *testing.T) {
	d := ksh.Dialect()
	for _, tc := range []struct{ src, want string }{
		{"a=([1]=A p)\n", "syntax error at line 1: `p' unexpected"},
		{"a=([1]=A \"b\")\n", "syntax error at line 1: `b' unexpected"},
		{"a=([1]=A $x)\n", "syntax error at line 1: `$x' unexpected"},
		{"a=([1]=A $(echo p))\n", "syntax error at line 1: `$(echo p)' unexpected"},
		{"a=([1]=A [2]=B p q)\n", "syntax error at line 1: `p' unexpected"},
		// The appending spelling is shaped by the same rule — #2505 as filed
		// had it exempt, and it is not.
		{"a+=([1]=A p)\n", "syntax error at line 1: `p' unexpected"},
		// A declared keyed name does not escape it either: the refusal comes
		// first, so that name's own complaint about being given an indexed
		// literal never gets to run.
		{"typeset -A a=([1]=A p)\n", "syntax error at line 1: `p' unexpected"},
		// Nor does the literal an element is given of its own.
		{"a[1]=([2]=z p)\n", "syntax error at line 1: `p' unexpected"},
		// The literal's own newlines do not end the rule, and the bare
		// element is blamed on the line it stands on.
		{"a=([1]=A\np)\n", "syntax error at line 2: `p' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := diagTable(ksh.Diagnostics()).ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
	// And the shapes it must not fire on, which are what say the rule is
	// "bare after subscripted" and not "a bracket and a bare word in one
	// literal".
	for _, src := range []string{
		"a=(p [1]=A)\n",
		"a=(p q [1]=A [2]=B)\n",
		"a=(\"[1]=A\" p)\n",
		"a=([1]=A [2]=B)\n",
		"a=(p q r)\n",
		"a=()\n",
		"a+=(p [1]=A)\n",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// A literal that settled on the word-list shape does not leave the lexer
// there: the next literal spans its subscript's blanks again.
//
// A restore rather than a clear, and the difference is visible — clearing
// instead leaves `n` holding two words.
func TestTheWordListShapeDoesNotOutliveItsLiteral(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=(p [1 2]=A); n=([3 4]=B); typeset -p a; typeset -p n`)
	want := "typeset -a a=(p '[1' '2]=A')\ntypeset -A n=(['3 4']=B)\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q at 0", out, st, want)
	}
}

// A literal whose brackets are text expands each element once and not twice.
//
// The failure this pins is invisible in the value: splitting `[$((i++))]=v`
// into halves, expanding them, and then throwing the halves away for the word
// gives the same two words and leaves `i` at 2. Measured at 1.
func TestAWordListElementIsExpandedOnce(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`i=0; a=(p [$((i++))]=v); echo "i=$i n=${#a[@]}"`, "i=1 n=2\n"},
		{`i=0; a=([$((i++))]=v); echo "i=$i n=${#a[@]}"`, "i=1 n=1\n"},
	} {
		if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// An append whose name is already holding an indexed array cannot turn it
// into a keyed one, so the subscripted reading is given up there and the
// elements go in as the words they were written as.
//
// This is the reachable consequence #2505 was filed for, and it is a *store*
// rule rather than the grammar one above: an unset name, a scalar and a
// declared keyed table all read the subscripts, and only the indexed array
// refuses. We marked the name associative and stored the keyed reading over
// the top, so `a=(p q r); a+=([1]=Z)` answered `typeset -A a=([1]=Z)` — three
// elements gone, silently, at status 0.
func TestAnAppendOverAnIndexedArrayKeepsTheElementsAsWords(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`a=(p q r); a+=([1]=Z); typeset -p a`, "typeset -a a=(p q r '[1]=Z')\n"},
		{`a=(p q r); a+=([5]=Z); typeset -p a`, "typeset -a a=(p q r '[5]=Z')\n"},
		{`a=(p q r); a+=([1]+=Z); typeset -p a`, "typeset -a a=(p q r '[1]+=Z')\n"},
		// And the three names that do not refuse, which are what say the rule
		// is the indexed array's and not the operator's.
		{`unset a; a+=([1]=Z [2]=Y); typeset -p a`, "typeset -A a=([1]=Z [2]=Y)\n"},
		{`unset a; a+=([1]+=Z); typeset -p a`, "typeset -A a=([1]=Z)\n"},
		{`typeset -A m=([k]=v); m+=([j]=w); typeset -p m`, "typeset -A m=([j]=w [k]=v)\n"},
		// A replacing literal is not an append and starts over, so it reads
		// the subscripts however much the name was holding.
		{`a=(p q r); a=([1]=Z); typeset -p a`, "typeset -A a=([1]=Z)\n"},
	} {
		if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// What refuses the subscripted reading is an indexed array *holding an
// element*, not any indexed array: one that never held one reads the
// subscripts of an append, and reads them as its own — expressions placing
// values, the name left indexed — rather than as keys (#7323).
//
// The key is the array's history and not its count. An array emptied by
// `unset` has `${#e[@]}` 0 as well and still keeps the text, so those rows read
// the values back rather than the listing: where the kept word goes after an
// emptied array is a different question from whether it is kept.
//
// Measured 2026-11-02 from a script file under `env -i PATH=/usr/bin:/bin` on
// ksh93u+ 2012-08-01.
func TestAnAppendOverAnIndexedArrayHoldingNoElementReadsTheSubscripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// The issue's four rows: the declared array that has never held an
		// element, and the three controls that already agreed.
		{`typeset -a e; e+=([3]=y); typeset -p e`, "typeset -a e=([3]=y)\n"},
		{`unset e; e+=([3]=y); typeset -p e`, "typeset -A e=([3]=y)\n"},
		{`e=(7 8); e+=([3]=y); typeset -p e`, "typeset -a e=(7 8 '[3]=y')\n"},
		{`e=s; e+=([3]=y); typeset -p e`, "typeset -A e=([0]=s [3]=y)\n"},
		// Holding no element, however it came to: an empty literal ends the
		// declared-only state without putting an element in.
		{`typeset -a e=(); e+=([3]=y); typeset -p e`, "typeset -a e=([3]=y)\n"},
		{`typeset -a e; e+=(); e+=([3]=y); typeset -p e`, "typeset -a e=([3]=y)\n"},
		// Having held one and lost it is not the same state.
		{`e=(7); unset 'e[0]'; e+=([3]=y); printf '<%s>\n' "${e[@]}"`, "<[3]=y>\n"},
		{`typeset -a e; e[0]=q; unset 'e[0]'; e+=([3]=y); printf '<%s>\n' "${e[@]}"`, "<[3]=y>\n"},
		// Every element is read, and the subscripts are expressions rather
		// than keys: an unset name in one is 0.
		{`typeset -a e; e+=([3]=y [5]=z); typeset -p e`, "typeset -a e=([3]=y [5]=z)\n"},
		{`typeset -a e; e+=([1+1]=y); typeset -p e`, "typeset -a e=([2]=y)\n"},
		{`typeset -a e; e+=([k]=y); typeset -p e`, "typeset -a e=(y)\n"},
		{`typeset -a e; e+=([3]=y [3]+=z); typeset -p e`, "typeset -a e=([3]=yz)\n"},
		{`typeset -ia e; e+=([3]=1+1); typeset -p e`, "typeset -a -i e=([3]=2)\n"},
		// Once the append has placed an element the array holds one, so the
		// next append keeps its text, after the highest subscript.
		{`typeset -a e; e+=([3]=y); e+=([5]=z); typeset -p e`, "typeset -a e=([3]=y [4]='[5]=z')\n"},
		// A literal that is not subscripted throughout is a word list by the
		// grammar's rule, which no store rule reaches.
		{`typeset -a e; e+=(x [3]=y); typeset -p e`, "typeset -a e=(x '[3]=y')\n"},
		// The first element decides as it does for nesting: a literal of its
		// own leading the list still builds a table.
		{`typeset -a e; e+=([3]=y [4]=(p q)); typeset -p e`, "typeset -a e=([3]=y [4]=(p q) )\n"},
		{`typeset -a e; e+=([3]=(p q) [4]=y); typeset -p e`, "typeset -A e=([3]=(p q) [4]=y)\n"},
		{`typeset -a e; e+=([3]=(p q)); typeset -p e`, "typeset -A e=([3]=(p q) )\n"},
	} {
		if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
