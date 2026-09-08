// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// `${(Z:opts:)v}` splits a value the way the shell splits a command line.
//
// Every row is measured on zsh 5.9.2, the one shell in the panel whose
// grammar has the flag; the other five call `${(Z+n+)v}` a bad substitution
// when they reach it or refuse it while reading, which is the three-way split
// recorded for every one of these flags.
//
// The fields are asserted rather than counted. A splitter that hands the
// whole value back as one field, one that splits it and loses the quotes, and
// one that is right all exit 0 and differ only in what they produce — and
// `[a][b][c]` says which of the three ran where `n=3` does not.
func TestTheShellSplitFlagSplitsAValueLikeACommandLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"blanks separate", `v="a b  c"; printf "[%s]" ${(Z+n+)v}; echo`, "[a][b][c]\n"},
		{"quotes are kept in the word", `v="a 'b c' d"; printf "[%s]" ${(Z+n+)v}; echo`, "[a]['b c'][d]\n"},
		// Quoted, so the backslash is asserted as it was written: unquoted,
		// whether it survives is the separate question of whether this
		// runner globs the result of an expansion, which is not this row's.
		{"an escaped blank holds a word together", `v='a b\ c d'; printf "[%s]" "${(Z+n+)v}"; echo`, `[a][b\ c][d]` + "\n"},
		{"a substitution is one word, unrun", `v='a $(echo x y) b'; printf "[%s]" ${(Z+n+)v}; echo`, "[a][$(echo x y)][b]\n"},
		{"operators are words of their own", `v='echo a|b; c && d'; printf "[%s]" ${(Z+n+)v}; echo`, "[echo][a][|][b][;][c][&&][d]\n"},
		{"an unclosed quote is the rest of it", `v="a 'b"; printf "[%s]" ${(Z+n+)v}; echo`, "[a]['b]\n"},

		// The option letters, on one value that separates all four readings.
		{"no comment rule", "v=$'a # hi\nb'; printf \"[%s]\" ${(Z+n+)v}; echo", "[a][#][hi][b]\n"},
		{"c keeps the comment whole", "v=$'a # hi\nb'; printf \"[%s]\" ${(Z+cn+)v}; echo", "[a][# hi][b]\n"},
		{"C drops it", "v=$'a # hi\nb'; printf \"[%s]\" ${(Z+Cn+)v}; echo", "[a][b]\n"},
		{"c wins where both are written", "v='a # hi'; printf \"[%s]\" ${(Z+Cc+)v}; echo", "[a][# hi]\n"},
		{"without n a newline is a semicolon", "v=$'a\nb'; printf \"[%s]\" ${(Z+C+)v}; echo", "[a][;][b]\n"},
		{"one word per newline", "v=$'a\n\n\nb'; printf \"[%s]\" ${(Z+C+)v}; echo", "[a][;][;][;][b]\n"},
		{"a hash inside quotes is not a comment", `v="a 'x #y' b"; printf "[%s]" ${(Z+Cn+)v}; echo`, "[a]['x #y'][b]\n"},

		// An empty option list turns the flag off rather than splitting with
		// no options: the value comes back unchanged, both blanks and all.
		{"an empty option list is a no-op", `v="a  b"; printf "[%s]" "${(Z::)v}"; echo`, "[a  b]\n"},
		{"where the same value with an option splits", `v="a  b"; printf "[%s]" "${(Z+n+)v}"; echo`, "[a][b]\n"},

		// The delimiters, including the matched pairs.
		{"a colon delimiter", `v="a b"; printf "[%s]" ${(Z:n:)v}; echo`, "[a][b]\n"},
		{"a bracket pair", `v="a b"; printf "[%s]" ${(Z[n])v}; echo`, "[a][b]\n"},
		{"an angle pair", `v="a b"; printf "[%s]" ${(Z<n>)v}; echo`, "[a][b]\n"},

		// Nothing to split. Quoted the empty field at the edge survives, and
		// unquoted it does not — the same rule `(f)` and `(s)` follow.
		{"an empty value quoted is one empty field", `v=""; set -- "${(Z+n+)v}"; printf "n=%d" $#; echo`, "n=1\n"},
		{"and unquoted is none", `v=""; set -- ${(Z+n+)v}; printf "n=%d" $#; echo`, "n=0\n"},
		{"blanks alone are the same", `v="   "; set -- "${(Z+n+)v}"; printf "n=%d" $#; echo`, "n=1\n"},
		{"so is a value that is only a dropped comment", `v="#only"; set -- "${(Z+Cn+)v}"; printf "n=%d" $#; echo`, "n=1\n"},
		// The empty field is the *expansion's*, not any one word's: an
		// element that splits to nothing contributes no field at all, and
		// only a result with nothing in it anywhere comes to the one.
		{"an element that splits to nothing is no field", `a=('' x); set -- "${(@Z+n+)a}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=1[x]\n"},
		{"an empty array quoted is still one empty field", `a=(); set -- "${(@Z+n+)a}"; printf "n=%d" $#; echo`, "n=1\n"},
		{"and unquoted it is none", `a=(); set -- ${(Z+n+)a}; printf "n=%d" $#; echo`, "n=0\n"},
		{"where the same array without the split flag has no field", `a=(); set -- "${(@)a}"; printf "n=%d" $#; echo`, "n=0\n"},

		// Composition, each row placing the split against one other step.
		{"the length is taken before the split", `v="a b  c"; printf "[%s]" "${(Z+n+)#v}"; echo`, "[6]\n"},
		// An array is *not* joined first, which is where this split parts
		// company with `(f)` and `(s)`: each element is read as a command
		// line of its own, so a quote opened in one does not reach the next.
		// The quoted rows are the same array under the join that quoting
		// itself asks for, and the `@` row is that join declined again.
		{"an array is split element by element", `a=('"x' 'y"'); printf "[%s]" ${(Z+n+)a}; echo`, `["x][y"]` + "\n"},
		{"quoted the array joins before the split", `a=('"x' 'y"'); printf "[%s]" "${(Z+n+)a}"; echo`, `["x y"]` + "\n"},
		{"and the fields flag declines that join", `a=('"x' 'y"'); printf "[%s]" "${(@Z+n+)a}"; echo`, `["x][y"]` + "\n"},
		{"where a separator split does join first", `a=("a:b" "c:d"); printf "[%s]" ${(s.:.)a}; echo`, "[a][b c][d]\n"},
		{"an array of ordinary words", `a=(x "y z"); printf "[%s]" ${(Z+n+)a}; echo`, "[x][y][z]\n"},
		{"the quoting flag runs first", `v="a|b"; printf "[%s]" "${(Z+n+q)v}"; echo`, `[a\|b]` + "\n"},
		{"and the unquoting flag does too", `v="'a|b'"; printf "[%s]" "${(Z+n+Q)v}"; echo`, "[a][|][b]\n"},
		{"the ordering flag runs after", `v="b  a"; printf "[%s]" "${(oZ+n+)v}"; echo`, "[a][b]\n"},
		{"the case flags compose", `v="a  B"; printf "[%s]" "${(UZ+n+)v}"; echo`, "[A][B]\n"},
		{"a line split composes with it", "v=$'a:b\nc'; printf \"[%s]\" \"${(fZ+n+)v}\"; echo", "[a:b][c]\n"},
		{"and a separator split does", `v="a:b  c"; printf "[%s]" "${(s.:.Z+n+)v}"; echo`, "[a][b][c]\n"},
		{"an operator's result is what is split", `printf "[%s]" ${(Z+n+)nope:-"p q"}; echo`, "[p][q]\n"},
		{"an unset name is nothing at all", `printf "[%s]" ${(Z+n+)nosuch}; echo`, "[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// `${(z)v}` is the same split with no option letters, and this asserts that
// it *is* the same one rather than a second splitter that agrees on `a b`.
//
// Every row is measured on zsh 5.9.2 against the `(Z)` row beside it, so a
// divergence shows up here as two rows disagreeing rather than as one row
// nobody compared. The rows that would pass under a fresh word scanner and
// fail under a wrong one are the quoting, the operators and the substitution:
// a scanner that split on blanks answers `[a]['b][c']` where both spellings
// of this flag answer `[a]['b c']`.
func TestTheArgumentlessShellSplitFlagIsTheSameSplit(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"blanks separate", `v="a b  c"; printf "[%s]" ${(z)v}; echo`, "[a][b][c]\n"},
		{"quotes are kept in the word", `v="a 'b c' d"; printf "[%s]" ${(z)v}; echo`, "[a]['b c'][d]\n"},
		{"double quotes too", `v='a "b c" d'; printf "[%s]" ${(z)v}; echo`, `[a]["b c"][d]` + "\n"},
		{"and a dollar-quoted word", `v="a $'x y' b"; printf "[%s]" ${(z)v}; echo`, "[a][$'x y'][b]\n"},
		{"an escaped blank holds a word together", `v='a b\ c d'; printf "[%s]" "${(z)v}"; echo`, `[a][b\ c][d]` + "\n"},
		{"a quote inside a word does not open one", `v="ab'c d'ef"; printf "[%s]" ${(z)v}; echo`, "[ab'c d'ef]\n"},
		{"a substitution is one word, unrun", `v='a $(echo x y) b'; printf "[%s]" ${(z)v}; echo`, "[a][$(echo x y)][b]\n"},
		{"a backquote is too", "v='a `echo x y` b'; printf \"[%s]\" ${(z)v}; echo", "[a][`echo x y`][b]\n"},
		{"and an arithmetic expansion", `v='a $((1+2)) b'; printf "[%s]" ${(z)v}; echo`, "[a][$((1+2))][b]\n"},
		{"operators are words of their own", `v='echo a|b; c && d'; printf "[%s]" ${(z)v}; echo`, "[echo][a][|][b][;][c][&&][d]\n"},
		{"a redirection keeps its descriptor", `v='a > f 2>&1'; printf "[%s]" ${(z)v}; echo`, "[a][>][f][2>&][1]\n"},
		{"a here-document opener reads no body", "v=$'a <<EOF\nbody\nEOF'; printf \"[%s]\" ${(z)v}; echo", "[a][<<][EOF][;][body][;][EOF]\n"},
		{"an unclosed quote is the rest of it", `v="a 'b"; printf "[%s]" ${(z)v}; echo`, "[a]['b]\n"},
		{"an unclosed double quote too", `v='a "b'; printf "[%s]" ${(z)v}; echo`, `[a]["b]` + "\n"},
		{"a hash where a word begins is a word", "v=$'a # hi\nb'; printf \"[%s]\" ${(z)v}; echo", "[a][#][hi][;][b]\n"},
		{"a hash inside a word is not", `v="a#b"; printf "[%s]" ${(z)v}; echo`, "[a#b]\n"},
		{"a newline is a semicolon of its own", "v=$'a\n\n\nb'; printf \"[%s]\" ${(z)v}; echo", "[a][;][;][;][b]\n"},
		{"tabs are blanks", "v=$'\ta  \tb '; printf \"[%s]\" ${(z)v}; echo", "[a][b]\n"},

		// It is the no-letters case of the same flag, which is the whole of
		// the fold: a `Z` argument written behind it still reaches the same
		// splitter, so the letters take effect on a group the bare spelling
		// opened.
		{"a Z argument behind it still reaches the split", `v="a # h"; printf "[%s]" ${(zZ+C+)v}; echo`, "[a]\n"},

		// Nothing to split, the same edge rule the argument spelling has.
		{"an empty value quoted is one empty field", `v=""; set -- "${(z)v}"; printf "n=%d" $#; echo`, "n=1\n"},
		{"and unquoted is none", `v=""; set -- ${(z)v}; printf "n=%d" $#; echo`, "n=0\n"},
		{"blanks alone are the same", `v="   "; set -- ${(z)v}; printf "n=%d" $#; echo`, "n=0\n"},
		{"an empty array quoted is still one empty field", `a=(); set -- "${(@z)a}"; printf "n=%d" $#; echo`, "n=1\n"},
		{"an element that splits to nothing is no field", `a=('' x); set -- "${(@z)a}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=1[x]\n"},

		// Composition, each row placing the split against one other step.
		{"the quoting flag runs first", `v="a|b"; printf "[%s]" "${(zq)v}"; echo`, `[a\|b]` + "\n"},
		{"and the unquoting flag does too", `v="'a|b'"; printf "[%s]" "${(zQ)v}"; echo`, "[a][|][b]\n"},
		{"the ordering flag runs after, written first", `v="b  a"; printf "[%s]" "${(oz)v}"; echo`, "[a][b]\n"},
		{"and written second", `v="b  a"; printf "[%s]" "${(zo)v}"; echo`, "[a][b]\n"},
		{"the dedup flag runs after", `v="p p q"; printf "[%s]" ${(zu)v}; echo`, "[p][q]\n"},
		{"the numeric order runs after", `v="10 9 2"; printf "[%s]" ${(zn)v}; echo`, "[2][9][10]\n"},
		{"the join runs before, so its separator is split again", `a=(x y); printf "[%s]" "${(zj.|.)a}"; echo`, "[x][|][y]\n"},
		{"a line split composes with it", "v=$'a:b\nc'; printf \"[%s]\" ${(fz)v}; echo", "[a:b][c]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The `@` flag beside the split, in both written orders and both quotings.
//
// Both orders are in the startup this was built for — `${(z@)m[k]}` and
// `${(@z)a}` — and they are the same answer, which is worth an assertion
// rather than an assumption: a split that consulted the letters positionally
// would answer them differently, and the one that broke would be whichever
// order the tests happened not to cover.
func TestTheArgumentlessShellSplitFlagBesideTheFieldsFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"quoted, split first", `v="a b"; set -- "${(z@)v}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[a][b]\n"},
		{"quoted, fields first", `v="a b"; set -- "${(@z)v}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[a][b]\n"},
		{"quoted, neither", `v="a b"; set -- "${(z)v}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[a][b]\n"},
		{"unquoted, split first", `v="a b"; set -- ${(z@)v}; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[a][b]\n"},
		{"unquoted, fields first", `v="a b"; set -- ${(@z)v}; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[a][b]\n"},
		{"over an array, split first", `a=("x y" "p q"); set -- "${(z@)a}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=4[x][y][p][q]\n"},
		{"over an array, fields first", `a=("x y" "p q"); set -- "${(@z)a}"; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=4[x][y][p][q]\n"},
		// An operator's result is what is split, which is the shape the
		// startup this was built for writes — `${(z@)m[k]:-$m2[k]}`, the
		// association and its default both graded in the corpus, where the
		// subscripts have a real dialect under them.
		{"an operator's result is what is split", `set -- ${(z@)nope:-"p q"}; printf "n=%d" $#; printf "[%s]" "$@"; echo`, "n=2[p][q]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A group may write both spellings, and then the option letters are what the
// order decides: a `Z` argument adds its letters, a `z` clears the ones in
// front of it. Measured on zsh 5.9.2 on `$'a # h\nb'`, where `C` and `n` each
// change the answer visibly and the two are independent.
//
// The two-argument rows are the ones that caught a real defect: reading the
// last `Z` argument alone answered `[a][#][h][b]` for `${(Z+C+Z+n+)v}`, the
// dropped comment lost, at status 0.
func TestTheTwoShellSplitSpellingsAccumulateTheirOptionLetters(t *testing.T) {
	const v = "v=$'a # h\nb'; "
	for _, tc := range []struct{ name, src, want string }{
		{"two arguments union", `printf "[%s]" ${(Z+C+Z+n+)v}; echo`, "[a][b]\n"},
		{"in either order", `printf "[%s]" ${(Z+n+Z+C+)v}; echo`, "[a][b]\n"},
		{"an argument behind the bare letter counts", `printf "[%s]" ${(zZ+n+)v}; echo`, "[a][#][h][b]\n"},
		{"the bare letter behind an argument clears it", `printf "[%s]" ${(Z+n+z)v}; echo`, "[a][#][h][;][b]\n"},
		{"and clears only what stands in front of it", `printf "[%s]" ${(Z+C+zZ+n+)v}; echo`, "[a][#][h][b]\n"},
		{"a trailing one clears the lot", `printf "[%s]" ${(Z+C+Z+n+z)v}; echo`, "[a][#][h][;][b]\n"},
		{"the bare letter twice is once", `printf "[%s]" ${(zz)v}; echo`, "[a][#][h][;][b]\n"},
		{"an empty argument adds nothing", `printf "[%s]" ${(Z+n+Z::)v}; echo`, "[a][#][h][b]\n"},
		{"nor does it turn the split off", `printf "[%s]" ${(zZ::)v}; echo`, "[a][#][h][;][b]\n"},
		{"where alone it is the whole no-op", `printf "[%s]" "${(Z::)v}"; echo`, "[a # h\nb]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, v+tc.src, selectingWithFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// An option letter the flag does not have is refused by *position*, as an
// error in the flags, where a flag letter this interpreter has not built is
// refused by *name*. Both shapes are asserted whole, because the whole point
// of the pair is that they are different complaints and a reader has to be
// able to tell which one they got.
func TestTheShellSplitFlagRefusesAnUnknownOptionLetter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an unknown letter", `v=x; printf "[%s]" ${(Z:q:)v}`,
			"sh: error in flags near position 6 in '${(Z:q:)v}'\n",
		},
		{
			"one behind a good one", `v=x; printf "[%s]" ${(Z+nx+)v}`,
			"sh: error in flags near position 7 in '${(Z+nx+)v}'\n",
		},
		{
			"no argument at all", `v=x; printf "[%s]" ${(Z)v}`,
			"sh: error in flags near position 5 in '${(Z)v}'\n",
		},
		{
			"an argument that never closes", `v=x; printf "[%s]" ${(Z+n)v}`,
			"sh: error in flags near position 5 in '${(Z+n)v}'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the option letter refused")
			}
		})
	}
}
