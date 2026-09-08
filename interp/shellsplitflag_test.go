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

		// Composition, each row placing the split against one other step.
		{"the length is taken before the split", `v="a b  c"; printf "[%s]" "${(Z+n+)#v}"; echo`, "[6]\n"},
		{"an array is joined and then split", `a=(x "y z"); printf "[%s]" ${(Z+n+)a}; echo`, "[x][y][z]\n"},
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
