// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `typeset -U` on a **tied** name, where one side is a joined string and the
// other a list of elements.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`,
// from a script file. See interp/tiedunique.go for the rule and #4495 for the
// suite chunk that found it.
//
// The four rows of the reduction, pinned: the reference answers `l o c a l`
// to the array before the letter arrives, `l:o:c:a l o c a` after it, and
// lists the pair as `typeset -UT SCALAR array=( l o c a )` and
// `typeset -aT SCALAR array=( l o c a )`.
func TestTheUniqueLetterOnATiedScalar(t *testing.T) {
	dir := t.TempDir()
	const reduction = `typeset -T SCALAR=l:o:c:a:l array
print $array
typeset -U SCALAR
print $SCALAR $array
typeset -p SCALAR array`
	out, st := runZsh(t, dir, reduction)
	want := "l o c a l\n" +
		"l:o:c:a l o c a\n" +
		"typeset -UT SCALAR array=( l o c a )\n" +
		"typeset -aT SCALAR array=( l o c a )\n"
	if out != want || st != 0 {
		t.Errorf("the reduction = %q at %d, want %q at 0", out, st, want)
	}
}

// The rule is keyed on **the name that is written**, and these are the rows
// that say so rather than merely agreeing with it.
//
// The first pair holds the written name fixed at the scalar and moves only
// where the letter sits; the second holds the letter fixed on the array and
// moves the written name. A reading that made `-U` a property of the *pair*
// answers the first row of each pair correctly and the second one wrong, and
// a reading that put the whole attribute on the array half gets the two
// scalar writes backwards.
func TestTheWrittenNameDecidesWhichSideIsUniquified(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the letter on the scalar, writing the scalar",
			`typeset -T S s; typeset -U S; S=m:n:m; print "$S [${s[@]}]"`,
			"m:n [m n]\n",
		},
		{
			"the letter on the array, writing the scalar",
			`typeset -T S s; typeset -U s; S=m:n:m; print "$S [${s[@]}]"`,
			"m:n:m [m n m]\n",
		},
		{
			"the letter on the array, writing the array",
			`typeset -T S s; typeset -U s; s=(p q p); print "$S [${s[@]}]"`,
			"p:q [p q]\n",
		},
		{
			"the letter on the scalar, writing the array",
			`typeset -T S s; typeset -U S; s=(p q p); print "$S [${s[@]}]"`,
			"p:q:p [p q p]\n",
		},
		// And with the letter on both names, every write deduplicates —
		// which is the control that says the two rows above are about where
		// the letter is and not about the scalar being special.
		{
			"the letter on both, writing the scalar",
			`typeset -T S s; typeset -U S s; S=m:n:m; print "$S [${s[@]}]"`,
			"m:n [m n]\n",
		},
		{
			"the letter on both, writing the array",
			`typeset -T S s; typeset -U S s; s=(p q p); print "$S [${s[@]}]"`,
			"p:q [p q]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What the attribute does to the value a name is already holding, which is the
// half #4495 was missing, and the rest of the letter's rows on a tie.
func TestTheUniqueLetterOnATieInDetail(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The first occurrence is the one kept and it does not move, on the
		// scalar side as on the array side.
		{
			"the first occurrence stands",
			`typeset -T S s; typeset -U S; S=3:1:2:3; print "$S"; S=1:3:1; print "$S"`,
			"3:1:2\n1:3\n",
		},
		// The fields are the tie's, so the separator it was declared with is
		// what names them.
		{
			"the tie's own separator",
			"typeset -T S s '#'; typeset -U S; S='a#b#a'; print \"$S [${s[@]}]\"",
			"a#b [a b]\n",
		},
		// An empty field is a value like any other: five fields deduplicate
		// to four, and the two empties become one.
		{
			"an empty field is a value",
			`typeset -T S s; typeset -U S; S=a::b::c; print "$S ${#s}"`,
			"a::b:c 4\n",
		},
		// The letter arriving with the tie applies to the tie's own value.
		{
			"the letter written on the tie's own line",
			`typeset -TU S=a:b:a s; print "$S [${s[@]}]"`,
			"a:b [a b]\n",
		},
		// An append to the scalar is a write like any other.
		{
			"an append to the scalar",
			`typeset -T S=a:b s; typeset -U S; S+=:a; print "$S [${s[@]}]"`,
			"a:b [a b]\n",
		},
		// And an append to the *array* is not, because the letter is on the
		// other name.
		{
			"an append to the array",
			`typeset -T S=a:b s; typeset -U S; s+=(a); print "$S [${s[@]}]"`,
			"a:b:a [a b a]\n",
		},
		// `+U` takes the letter off and leaves the elements that are there.
		{
			"the plus form",
			`typeset -T S=a:b:a s; typeset -U S; typeset +U S; S=m:n:m; print "$S [${s[@]}]"`,
			"m:n:m [m n m]\n",
		},
		// Export goes to the scalar alone, so the listing writes the letter
		// on an `export` line and the array keeps its own.
		{
			"an exported tie",
			`typeset -xT S=a:b:a s; typeset -U S; typeset -p S; typeset -p s`,
			"export -UT S s=( a b )\ntypeset -aT S s=( a b )\n",
		},
		// A subshell's letter is the subshell's.
		{
			"inside a subshell",
			`typeset -T S=a:b:a s; (typeset -U S; print "$S"); print "$S"`,
			"a:b\na:b:a\n",
		},
		// The control: a scalar that is not half of a tie has no fields, so
		// the letter has nothing to say about it.
		{
			"an untied scalar",
			`v=a:b:a; typeset -U v; print "$v"; v=c:d:c; print "$v"`,
			"a:b:a\nc:d:c\n",
		},
		// And the other control, the plain array the letter was written for.
		{
			"an untied array",
			`a=(1 1 2); typeset -U a; print "[${a[@]}]"`,
			"[1 2]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A tie starts both names over: every attribute either name carried before is
// dropped and only export survives.
//
// It is here rather than in a file of its own because it is what keeps the
// letters above meaning what they say — `typeset -U S; typeset -T S=a:b:a s`
// is `a:b:a` precisely because the tie threw that `-U` away before the value
// reached it, where the same value under a `-U` written *on* the tie is
// `a:b`. See interp/tiedscalar.go.
func TestATieStartsBothNamesOver(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the unique letter, and the value that proves it went",
			`typeset -U S; typeset -T S=a:b:a s; print "$S"; typeset -p S`,
			"a:b:a\ntypeset -T S s=( a b a )\n",
		},
		{"an upper letter", `typeset -u S; typeset -T S s; typeset -p S`, "typeset -T S s=( '' )\n"},
		{"an integer letter", `typeset -i S=5; typeset -T S s; typeset -p S`, "typeset -T S s=( 5 )\n"},
		{"a traced letter", `typeset -t S; typeset -T S s; typeset -p S`, "typeset -T S s=( '' )\n"},
		// The array half answers the same way.
		{
			"the array half",
			`typeset -U s; typeset -T S=a:b:a s; print "$S"; typeset -p s`,
			"a:b:a\ntypeset -aT S s=( a b a )\n",
		},
		// Export is the one that survives, on either half.
		{"export on the scalar", `typeset -x S=v; typeset -T S s; typeset -p S`, "export -T S s=( v )\n"},
		{"export on the array", `typeset -x s; typeset -T S s; typeset -p s`, "typeset -axT S s=(  )\n"},
		// And re-declaring the *same* pair keeps what it has, which is what
		// says the rule is about a name arriving at a tie rather than about
		// the letter `T` being written again.
		{
			"re-declaring the same pair",
			`typeset -TU S=a:b:a s; typeset -T S s; typeset -p S`,
			"typeset -UT S s=( a b )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
