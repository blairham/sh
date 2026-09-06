// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// The three flags that change what `${#…}` counts.
//
// Every row is a measurement on zsh 5.9.2 under `LC_ALL=C`, and the data is
// chosen so that the three answers differ: `a=(abc de f)` separates `c` from
// the element count, and `"a b  c"` separates `w` from `W` by the empty field
// between the two spaces. Single-word data would have hidden all of it.
func TestTheLengthFlags(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The plain answers first, so the modifiers are read against them.
		{"without a flag a list counts elements", `a=(abc de f); printf "[%s]" "${#a}"`, "[3]"},
		{"and a scalar counts characters", `v="a b  c"; printf "[%s]" "${#v}"`, "[6]"},
		// `c` counts the characters of the words joined, so the separators
		// are in the total: `abc de f` is 8 and not 6.
		{"c counts the joined characters", `a=(abc de f); printf "[%s]" "${(c)#a}"`, "[8]"},
		{"an empty element still costs its separator", `a=(abc "" f); printf "[%s]" "${(c)#a}"`, "[6]"},
		{"one word has no separator", `v=abc; printf "[%s]" "${(c)#v}"`, "[3]"},
		{"and no words have no characters", `a=(); printf "[%s]" "${(c)#a}"`, "[0]"},
		// The separator `c` counts is a space, and a `j` argument replaces
		// it. `$IFS` does not — which is the reading a shared join
		// separator would have got wrong.
		{"a j argument is the separator counted", `a=(abc de f); printf "[%s]" "${(cj.--.)#a}"`, "[10]"},
		{"and IFS is not", `IFS=:; a=(abc de f); printf "[%s]" "${(c)#a}"`, "[8]"},
		// A `$IFS` of the same *length* as a space cannot tell the two
		// apart, which is what makes this row the one that does: set and
		// empty, `$IFS`'s first character is no character at all, and the
		// count is still 8.
		{"not even when it has no first character", `IFS=""; a=(abc de f); printf "[%s]" "${(c)#a}"`, "[8]"},
		// `w` counts words and `W` counts the empty ones too.
		{"w counts words", `v="a b  c"; printf "[%s]" "${(w)#v}"`, "[3]"},
		{"W counts the empty one between them", `v="a b  c"; printf "[%s]" "${(W)#v}"`, "[4]"},
		{"leading and trailing whitespace is trimmed for w", `v=" a "; printf "[%s]" "${(w)#v}"`, "[1]"},
		{"but not for W", `v=" a "; printf "[%s]" "${(W)#v}"`, "[3]"},
		{"whitespace alone is no word", `v="  "; printf "[%s]" "${(w)#v}"`, "[0]"},
		{"and three empty ones", `v="  "; printf "[%s]" "${(W)#v}"`, "[3]"},
		{"an empty value is no word either way", `v=""; printf "[%s]" "${(w)#v}${(W)#v}"`, "[00]"},
		// An array is counted an element at a time and the totals added,
		// which a join would have got one word wrong.
		{"an array counts each element", `a=(abc de f); printf "[%s]" "${(w)#a}${(W)#a}"`, "[33]"},
		{"an empty element counts nothing", `a=(abc "" f); printf "[%s]" "${(w)#a}${(W)#a}"`, "[22]"},
		{"and no elements nothing at all", `a=(); printf "[%s]" "${(w)#a}${(W)#a}"`, "[00]"},
		// A non-whitespace IFS separator does not collapse, so the two
		// agree — and a trailing one still opens a field, which ordinary
		// word splitting absorbs.
		{"a non-whitespace IFS separator delimits singly", `IFS=:; v="a::b"; printf "[%s]" "${(w)#v}${(W)#v}"`, "[33]"},
		{"and a trailing one opens a field", `IFS=:; v="a:"; printf "[%s]" "${(w)#v}${(W)#v}"`, "[22]"},
		{"a leading one too", `IFS=:; v=":a:"; printf "[%s]" "${(w)#v}${(W)#v}"`, "[33]"},
		{"separators alone are all fields", `IFS=:; v=":::"; printf "[%s]" "${(w)#v}${(W)#v}"`, "[44]"},
		// It is the trailing *run* that decides, not the last byte: a run
		// holding one non-whitespace separator opens a field however much
		// whitespace follows it.
		{"a mixed run with a separator in it counts", `IFS=": "; v="a: "; printf "[%s]" "${(w)#v}"`, "[2]"},
		{"written the other way round as well", `IFS=": "; v="a :"; printf "[%s]" "${(w)#v}"`, "[2]"},
		{"and whitespace on its own does not", `IFS=": "; v="a "; printf "[%s]" "${(w)#v}"`, "[1]"},
		{"a run of both is one delimiter for w", `IFS=": "; v="a  :  b"; printf "[%s]" "${(w)#v}"`, "[2]"},
		{"and six fields for W", `IFS=": "; v="a  :  b"; printf "[%s]" "${(W)#v}"`, "[6]"},
		{"IFS set to empty splits nothing", `IFS=""; v="a b"; printf "[%s]" "${(w)#v}${(W)#v}"`, "[11]"},
		// An explicit separator is a string split literally, and it is a
		// *different* rule: `w` collapses runs of it and drops a leading
		// empty field while keeping a trailing one, and an empty value is
		// still one field where under IFS it is none.
		{"s gives w its separator", `v="a:b::c"; printf "[%s]" "${(ws.:.)#v}"`, "[3]"},
		{"where W counts every field", `v="a:b::c"; printf "[%s]" "${(Ws.:.)#v}"`, "[4]"},
		{"a leading empty field is dropped", `v=":a:"; printf "[%s]" "${(ws.:.)#v}${(Ws.:.)#v}"`, "[23]"},
		{"separators alone are one field", `v="::"; printf "[%s]" "${(ws.:.)#v}${(Ws.:.)#v}"`, "[13]"},
		{"and so is an empty value", `v=""; printf "[%s]" "${(ws.:.)#v}${(Ws.:.)#v}"`, "[11]"},
		{"an empty separator counts characters", `v=abc; printf "[%s]" "${(ws::)#v}${(Ws::)#v}"`, "[33]"},
		{"and an empty value is still one field", `v=""; printf "[%s]" "${(ws::)#v}${(Ws::)#v}"`, "[11]"},
		{"f is a separator as much as s is", `v=$'a\nb\n'; printf "[%s]" "${(fw)#v}${(fW)#v}"`, "[33]"},
		{"and IFS reads the same value differently", `v=$'a\nb\n'; printf "[%s]" "${(w)#v}${(W)#v}"`, "[23]"},
		{"an array with an explicit separator counts per element", `a=(a: :b); printf "[%s]" "${(ws.:.)#a}${(Ws.:.)#a}"`, "[34]"},
		// The last of the three written wins, which is measured rather than
		// assumed: on `a b` the answers 2 and 3 are `w`'s and `c`'s.
		{"cw is w", `v="a b"; printf "[%s]" "${(cw)#v}"`, "[2]"},
		{"wc is c", `v="a b"; printf "[%s]" "${(wc)#v}"`, "[3]"},
		{"Wc is c", `v="a b"; printf "[%s]" "${(Wc)#v}"`, "[3]"},
		{"cW is W", `v="a b"; printf "[%s]" "${(cW)#v}"`, "[2]"},
		{"and on an array the same way round", `a=(abc de f); printf "[%s]" "${(cw)#a}${(wc)#a}"`, "[38]"},
		// None of the three does anything to a value, which is what says
		// they modify the length step and nothing else.
		{"c on a value is the plain expansion", `a=(abc de f); printf "[%s]" "${(@c)a}"`, "[abc][de][f]"},
		{"and so is w", `v="a b"; printf "[%s]" "${(w)v}"`, "[a b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The length is taken before the double-quoted join, and that is a defect
// this change fixed rather than a rule it added.
//
// `"${(U)#a}"` answered 8 on the branch point — the length of `ABC DE F`,
// which is the join having already happened — where the shell with the
// construct answers 3. Nothing in the flag group had asked for a join; rule
// 5 does it to every quoted list, and the length step was reading what it
// left. A `j` separator makes it visible from the other side: it changes the
// join and does not change the count.
func TestALengthIsTakenBeforeTheQuotedJoin(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a quoted list still counts elements", `a=(abc de f); printf "[%s]" "${(U)#a}"`, "[3]"},
		{"a join separator does not reach the count", `a=(abc de f); printf "[%s]" "${(Uj.-.)#a}"`, "[3]"},
		{"and (@) answers the same", `a=(abc de f); printf "[%s]" "${(@U)#a}"`, "[3]"},
		{"as does the unquoted form", `a=(abc de f); printf "[%s]" ${(U)#a}`, "[3]"},
		// A split does not reach it either: the length is asked before rule
		// 11, so `${(s.:.)#v}` is the characters of the value and not the
		// fields the separator would have made.
		{"nor does a split", `v=":a:"; printf "[%s]" "${(s.:.)#v}"`, "[3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
