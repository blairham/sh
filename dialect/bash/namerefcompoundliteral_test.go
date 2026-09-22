// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The `n` letter beside a **parenthesized** value is refused in the sentence
// a reference over an array already has, and the literal lands anyway.
// Measured on bash 5.3.20, 2026-09-22.
func TestTheReferenceLetterBesideACompoundLiteral(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the refusal and the status",
			`typeset -n x=(a b) 2>/dev/null; echo "st=$?"`,
			"st=1\n",
		},
		{
			"and the value still lands",
			`typeset -n x=(a b) 2>/dev/null; typeset -p x`,
			"declare -a x=([0]=\"a\" [1]=\"b\")\n",
		},
		{
			"the array letter beside it changes nothing",
			`typeset -na y=(a b) 2>/dev/null; echo "st=$?"; typeset -p y`,
			"st=1\ndeclare -a y=([0]=\"a\" [1]=\"b\")\n",
		},
		{
			"an empty literal is the same refusal",
			`typeset -n z=() 2>/dev/null; echo "st=$?"; typeset -p z`,
			"st=1\ndeclare -a z=()\n",
		},
		{
			"a table literal too",
			`typeset -A m; typeset -n m=([k]=v) 2>/dev/null; typeset -p m`,
			"declare -A m=([k]=\"v\" )\n",
		},
		{
			"over a name already holding an array, the literal replaces it",
			`arr=(zero); typeset -n arr=(one two) 2>/dev/null; typeset -p arr`,
			"declare -a arr=([0]=\"one\" [1]=\"two\")\n",
		},
		{
			"a reference already aimed writes through instead",
			`typeset -n q=v; typeset -n q=(a b); echo "st=$?"; typeset -p v`,
			"st=0\ndeclare -a v=([0]=\"a\" [1]=\"b\")\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// An array letter written over a reference that has never been aimed takes
// the reference away here, where ksh93 leaves both letters standing —
// Semantics.ArrayLetterOverAnUnaimedReferenceDropsIt.
func TestAnArrayLetterOverAnUnaimedReference(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the indexed letter", `typeset -n foo; typeset -a foo; typeset -p foo`, "declare -a foo\n"},
		{"the table letter", `typeset -n foo; typeset -A foo; typeset -p foo`, "declare -A foo\n"},
		{
			"and an element then lands in it",
			`typeset -n foo; typeset -a foo; foo[0]=7; typeset -p foo`,
			"declare -a foo=([0]=\"7\")\n",
		},
		{
			"an aimed reference sends the letter to its target instead",
			`typeset -n b=bar; typeset -a b; typeset -p b; typeset -p bar`,
			"declare -n b=\"bar\"\ndeclare -a bar\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// `unset -n` takes the **variable** away, letters and all — it is not
// `typeset +n`, which leaves the target's name behind as a value. Measured on
// bash 5.3.20, 2026-09-22.
func TestUnsetWithTheReferenceLetterTakesTheWholeVariable(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the name is gone",
			`typeset -nx r=v; unset -n r; typeset -p r 2>/dev/null; echo "st=$?"`,
			"st=1\n",
		},
		{
			"and the export attribute went with it",
			`typeset -nx r=v; unset -n r; r=plain; typeset -p r`,
			"declare -- r=\"plain\"\n",
		},
		{
			"so no child is told about it",
			`typeset -nx r=v; unset -n r; r=plain; env | grep -c '^r=' || true`,
			"0\n",
		},
		{
			"where `+n` leaves the target's name as an ordinary value",
			`typeset -nx r=v; typeset +n r; typeset -p r`,
			"declare -x r=\"v\"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// A reference carrying the export letter reaches a child as its **target's
// name**, and the integer letter a write through one folds by is the
// target's. Measured on bash 5.3.20, 2026-09-22.
func TestWhatTravelsThroughAReference(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"an exported reference is in the environment as its target",
			`var=foo; typeset -nx ref=var; env | grep '^ref='`,
			"ref=var\n",
		},
		{
			"an element target keeps its subscript there",
			`typeset -a a=(x y); typeset -nx ref='a[1]'; env | grep '^ref='`,
			"ref=a[1]\n",
		},
		{
			"a reference aimed at nothing reaches no child",
			`typeset -nx ref; env | grep -c '^ref=' || true`,
			"0\n",
		},
		{
			"the append folds by the target's integer letter",
			`typeset -i v=1; typeset -n r=v; r+=2; typeset -p v`,
			"declare -i v=\"3\"\n",
		},
		{
			"and by the array's, through an element",
			`typeset -ai a; typeset -n r='a[1]'; r=12; r+=4; typeset -p a`,
			"declare -ai a=([1]=\"16\")\n",
		},
		{
			"and by the table's",
			`typeset -A m; typeset -i m; typeset -n r='m[k]'; r=1; r+=2; typeset -p m`,
			"declare -Ai m=([k]=\"3\" )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// bash has two sentences for a target it will not aim a reference at, and the
// empty word is the one that parts them.
func TestTheEmptyReferenceTarget(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the empty word is an ordinary bad name",
			`typeset -n r= 2>&1 | sed 's/.*typeset/typeset/'`,
			"typeset: `': not a valid identifier\n",
		},
		{
			"a word that is not a name keeps the letter's own sentence",
			`typeset -n r=1x 2>&1 | sed 's/.*typeset/typeset/'`,
			"typeset: `1x': invalid variable name for name reference\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// The silent refusal `typeset -in` gives takes back a name it brought into
// being, and an `unset` name is not one it found standing. Measured on bash
// 5.3.20, 2026-09-22.
func TestTheSilentReferenceRefusalTakesBackAnUnsetName(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a name nothing ever set",
			`typeset -in b=v 2>/dev/null; typeset -p b 2>/dev/null; echo "st=$?"`,
			"st=1\n",
		},
		{
			"a name an unset took away",
			`b=1; unset b; typeset -in b=v 2>/dev/null; typeset -p b 2>/dev/null; echo "st=$?"`,
			"st=1\n",
		},
		{
			"and one `unset -n` took away",
			`typeset -ai a=(1); typeset -n b='a[0]'; unset -n b
typeset -in b='a[0]' 2>/dev/null; typeset -p b 2>/dev/null; echo "st=$?"`,
			"st=1\n",
		},
		{
			"where a name that really is there keeps its letters",
			`b=kept; typeset -in b=v 2>/dev/null; typeset -p b`,
			"declare -i b=\"kept\"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
