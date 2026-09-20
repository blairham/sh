// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `a[]=6` — a plain assignment whose brackets hold nothing at all — is
// refused, the name is left exactly as it was, and the rest of the line is
// given up.
//
// Measured 2026-09-20 against bash 5.3.20, `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash g.sh` over a script file with standard input on the null
// device: every shape below writes `g.sh: line N: <name>[]: bad array
// subscript`, the command it stood in does not run, and the next top-level
// line reads 1.
//
// Before this the subscript was dropped while the word was parsed, so the
// assignment arrived as a bare `a=6` — which in this dialect replaces element
// **zero** — and an unseeded `$i` in `a[$i]=6` was indistinguishable from it.
// Status 0, nothing said, and the array's first element gone (#3949).
func TestAnEmptyAssignmentSubscriptIsRefusedAndWritesNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The array is untouched — all three elements, in order, and no
		// fourth. A check on element zero alone would pass while the value
		// landed somewhere else.
		{
			"a=(1 2 3); a[]=6; echo unreached\n" + `echo "a=[${a[@]}] n=${#a[@]}"`,
			"bash: line 1: a[]: bad array subscript\na=[1 2 3] n=3\n",
		},
		// A name nothing has set stays unset, rather than being brought into
		// being holding the value.
		{
			"u[]=6; echo unreached\n" + `echo "set=[${u+yes}] u=[${u[@]}]"`,
			"bash: line 1: u[]: bad array subscript\nset=[] u=[]\n",
		},
		// The append spelling is the same refusal and not a different one.
		{
			"b=(1 2 3); b[]+=6; echo unreached\n" + `echo "b=[${b[@]}]"`,
			"bash: line 1: b[]: bad array subscript\nb=[1 2 3]\n",
		},
		// And a declared table, whose brackets hold a key rather than an
		// expression, is refused before the key is looked at: no empty key
		// is created, and the one the script did store is still there alone.
		{
			"declare -A m; m[k]=v; m[]=9; echo unreached\n" + `echo "keys=[${!m[@]}] vals=[${m[@]}]"`,
			"bash: line 1: m[]: bad array subscript\nkeys=[k] vals=[v]\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The status the refusal leaves, read on the very next line: 1, and the
// command the assignment stood in is given up whole.
func TestAnEmptyAssignmentSubscriptLeavesOneAndGivesUpTheCommand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2 3); a[]=6
echo "st=$?"`, "bash: line 1: a[]: bad array subscript\nst=1\n"},
		// The enclosing function goes with the line, and the script carries
		// on at the next top-level command.
		{
			"a=(1 2 3); f() { a[]=6; echo in-f; }; f\n" + `echo "st=$? a=[${a[@]}]"`,
			"bash: line 1: a[]: bad array subscript\nst=1 a=[1 2 3]\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The neighbors that must not have moved. Each is a subscript that is *not*
// written empty, and each writes element zero at status 0 — which is the
// answer the empty one used to give, so a refusal that reached any of them
// would be this fix in the wrong place.
func TestTheSubscriptsThatAreNotWrittenEmpty(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Quotes between the brackets are a subscript holding the empty
		// string, and the arithmetic reads that as zero.
		{`c=(1 2 3); c[""]=6; echo "st=$? c=[${c[@]}]"`, "st=0 c=[6 2 3]\n"},
		// So is a blank.
		{`d=(1 2 3); d[ ]=6; echo "st=$? d=[${d[@]}]"`, "st=0 d=[6 2 3]\n"},
		// And a subscript that *arrived* empty from an expansion is not this
		// question at all: it is the arithmetic reading of an empty
		// expression, which this dialect answers with zero.
		{`e=(1 2 3); i=; e[$i]=6; echo "st=$? e=[${e[@]}]"`, "st=0 e=[6 2 3]\n"},
		{`g=(1 2 3); g[0]=6; echo "st=$? g=[${g[@]}]"`, "st=0 g=[6 2 3]\n"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// Two orderings around the refusal, each measured against bash 5.3.20 on
// 2026-09-20 and each one a place the refusal could plausibly have gone
// instead.
func TestWhatIsAnsweredBeforeAnEmptyAssignmentSubscript(t *testing.T) {
	// The right-hand side runs first and its value is thrown away, so only
	// the store is refused: `SUBRAN` reaches standard error ahead of the
	// complaint.
	src := "a=(1 2 3); a[]=$(echo SUBRAN >&2; echo v)\n" + `echo "a=[${a[@]}]"`
	want := "SUBRAN\nbash: line 1: a[]: bad array subscript\na=[1 2 3]\n"
	if out, st := runBash(t, t.TempDir(), src); out != want || st != 0 {
		t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
	}
	// And the brackets are answered in front of the **freeze**: a frozen
	// name gets the subscript complaint and not `r: readonly variable`, and
	// the freeze alone leaves the name unset, so nothing was written.
	src = "readonly r\nr[]=6\n" + `echo "st=$? set=[${r+yes}]"`
	want = "bash: line 2: r[]: bad array subscript\nst=1 set=[]\n"
	if out, st := runBash(t, t.TempDir(), src); out != want || st != 0 {
		t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
	}
}

// A declaration utility's own operand is a question one construct further
// out, and it must not pick up this sentence: bash refuses `declare a[]=(x
// y)` as a bad **name** — “\`a[]': not a valid identifier“ — which is the
// wording its `declare 'a[b c]'=v` neighbor already carries. Asserted for
// what it is not, because the operand's own answer is not this issue's.
func TestADeclarationOperandDoesNotTakeTheAssignmentSentence(t *testing.T) {
	src := `declare a[]=(9 9)`
	out, _ := runBash(t, t.TempDir(), src)
	if strings.Contains(out, "bad array subscript") {
		t.Errorf("%s = %q, want the declaration's own refusal and not the assignment's", src, out)
	}
}
