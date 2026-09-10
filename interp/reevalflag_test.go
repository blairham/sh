// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// The re-reading flag reads the value again as shell text.
//
// Every row states the *construct* it is about — a reference, a substitution,
// a backslash — because the flag is one grammar's and its meaning is that
// grammar's, while what a re-read `$name` comes to is this package's own
// question and the same one it answers everywhere else.
func TestTheReevalFlagReadsTheValueAgain(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a parameter reference in the value becomes an expansion",
			`inner=hello; v='$inner'; printf "[%s]" "${(e)v}"`, "[hello]",
		},
		{
			"the braced spelling too",
			`inner=hello; v='${inner}'; printf "[%s]" "${(e)v}"`, "[hello]",
		},
		{
			"and one carrying an operator",
			`inner=hello; v='${inner:-zz}'; printf "[%s]" "${(e)v}"`, "[hello]",
		},
		{
			"a command substitution runs",
			`v='$(echo ran)'; printf "[%s]" "${(e)v}"`, "[ran]",
		},
		{
			"the backquoted spelling too",
			"v='`echo ran`'; printf \"[%s]\" \"${(e)v}\"", "[ran]",
		},
		{
			"an arithmetic substitution is evaluated",
			`v='$((2+3))'; printf "[%s]" "${(e)v}"`, "[5]",
		},
		{
			"the text around the substitution is kept",
			`d=DD; v='pre-$d-post'; printf "[%s]" "${(e)v}"`, "[pre-DD-post]",
		},
		{
			"without the flag the value is the value",
			`inner=hello; v='$inner'; printf "[%s]" "${v}"`, "[$inner]",
		},
		{
			"a reference to nothing is nothing",
			`v='$nosuchname'; printf "[%s]" "${(e)v}"`, "[]",
		},
		{
			// The single most important row: the flag does not loop.
			"the reading runs once and not to a fixed point",
			`inner='$deeper'; deeper=bottom; v='$inner'; printf "[%s]" "${(e)v}"`,
			"[$deeper]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What the re-reading is *not*. Each row is an expansion a word gets and this
// one does not, and each would be a plausible answer from a reader that read
// the text as an ordinary word instead.
func TestTheReevalFlagIsNotAWholeWordExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a leading tilde is a tilde",
			`v='~'; printf "[%s]" "${(e)v}"`, "[~]",
		},
		{
			"and one in front of a slash",
			`v='~/x'; printf "[%s]" "${(e)v}"`, "[~/x]",
		},
		{
			"a brace list is text",
			`v='{x,y}'; printf "[%s]" "${(e)v}"`, "[{x,y}]",
		},
		{
			"a metacharacter is not matched against the filesystem",
			`v='a*'; printf "[%s]" "${(e)v}"`, "[a*]",
		},
		{
			"a single quote is a character and does not protect what is inside it",
			`d=DD; v="'\$d'"; printf "[%s]" "${(e)v}"`, "['DD']",
		},
		{
			"nor does a double quote",
			`d=DD; v='"$d"'; printf "[%s]" "${(e)v}"`, `["DD"]`,
		},
		{
			"a backslash in front of a dollar suppresses it and goes",
			`d=DD; v='\$d'; printf "[%s]" "${(e)v}"`, "[$d]",
		},
		{
			"a backslash in front of a backslash is one backslash",
			`d=DD; v='\\$d'; printf "[%s]" "${(e)v}"`, `[\DD]`,
		},
		{
			"a backslash in front of anything else is text, both of it",
			`v='a\tb'; printf "[%s]" "${(e)v}"`, `[a\tb]`,
		},
		{
			"including in front of a double quote",
			`v='a\"b'; printf "[%s]" "${(e)v}"`, `[a\"b]`,
		},
		{
			"a dollar with nothing usable after it is a dollar",
			`v='$'; printf "[%s]" "${(e)v}"`, "[$]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The re-reading runs last of the value transformations, which is what the
// three rows below pin from three different sides.
func TestTheReevalFlagRunsAfterTheOtherFlags(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The sharpest of the three: the case conversion turned `$d`
			// into `$D`, which names nothing, so the answer is empty rather
			// than the value of `d`.
			"the case conversion runs first, and can destroy the reference",
			`d=DD; v='$d'; printf "[%s]" "${(eU)v}"`, "[]",
		},
		{
			"whichever side of the group the letters are written on",
			`d=DD; v='$d'; printf "[%s]" "${(Ue)v}"`, "[]",
		},
		{
			// And the quoting escaped the `$` that the re-reading then ate,
			// so the round trip is the text itself.
			"the quoting runs first, and the re-reading undoes it",
			`sq=hi; v='$sq'; printf "[%s]" "${(eq)v}"`, "[$sq]",
		},
		{
			"the split runs first, so the flag sees the fields it made",
			`d=DD; v='$d:$d'; printf "[%s]" "${(@es.:.)v}"`, "[DD][DD]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What the re-reading produces is fields, and how many survive is the ordinary
// question about an expansion's result rather than a rule of this flag's.
func TestTheReevalFlagProducesFields(t *testing.T) {
	const count = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{
			"an array reference yields one field per element",
			count + `a=(p q r); v='${a[@]}'; f ${(e)v}`, `3:[p][q][r]`,
		},
		{
			"quoted, they come back together with IFS's first character",
			count + `a=(p q r); v='${a[@]}'; f "${(e)v}"`, `1:[p q r]`,
		},
		{
			"and IFS is what joins them, not a space",
			count + `a=(p q r); v='${a[@]}'; IFS=-; f "${(e)v}"`, `1:[p-q-r]`,
		},
		{
			"nor the join separator the group named",
			count + `a=(p q r); v='${a[@]}'; f "${(ej:_:)v}"`, `1:[p q r]`,
		},
		{
			"a group asking for the fields by name keeps them",
			count + `a=(p q r); v='${a[@]}'; f "${(@e)v}"`, `3:[p][q][r]`,
		},
		{
			"the rejoin is per word, so words the pipeline held stay apart",
			count + `a=(p q); v='${a[@]}:${a[@]}'; f "${(es.:.)v}"`, `2:[p q][p q]`,
		},
		{
			"and every one of them survives when the fields are kept",
			count + `a=(p q); v='${a[@]}:${a[@]}'; f "${(@es.:.)v}"`, `4:[p][q][p][q]`,
		},
		{
			"a reference to nothing is no field at all unquoted",
			count + `v='$nosuchname'; f ${(e)v}`, `0:[]`,
		},
		{
			"and one empty field quoted",
			count + `v='$nosuchname'; f "${(e)v}"`, `1:[]`,
		},
		{
			// Naming the axis and not a shell: whether an unquoted scalar
			// result is split is a dialect question everywhere else, and the
			// re-reading asks it in the ordinary place rather than answering
			// it itself. This suite runs with the axis saying yes.
			"a scalar result is split where the splitting axis says so",
			count + `sp="a b"; v='$sp'; f ${(e)v}`, `2:[a][b]`,
		},
		{
			"and is one field where quoting suppresses the question",
			count + `sp="a b"; v='$sp'; f "${(e)v}"`, `1:[a b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A failure inside the re-read text is a failure of the expansion, so the
// command does not run and the status says so. It needs no code of its own —
// the text goes through the same parser and the same evaluator — and that is
// exactly why it is asserted: a reader that swallowed the failure would print
// an empty field at status 0.
//
// Whole strings, `after` included by its absence: the wording names the
// expansion the reader could not use, and a row asserting only that the word
// "bad" appeared would pass for a refusal that named the wrong one.
//
// The first row is an *empty-name* expansion — the value `${` reaches the
// reader as one — which this suite's dialect refuses and which a dialect that
// allows it reads as empty. An expansion left unterminated is a different
// failure and one this reader does not yet raise; see #1653.
func TestAFailureInTheReevaluatedTextStopsTheCommand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an expansion this dialect cannot read", `v='${'; printf "[%s]" "${(e)v}"; echo after`, "sh: ${}: bad substitution\n"},
		{"an arithmetic error", `v='$((1/0))'; printf "[%s]" "${(e)v}"; echo after`, "sh: division by zero\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the failure to be fatal")
			}
		})
	}
}

// A value that names its own expansion is bounded rather than followed.
//
// The shell being modeled does not terminate on it — `v='${(e)v}'` spins
// there until it is killed — so there is no answer to imitate, and the two
// candidates for what to do instead are a hang and a stack overflow. Neither
// is a diagnostic anyone can act on, and the second takes whatever program
// embeds this runner down with it, so the depth is bounded and the refusal
// says so. The same bound arithmetic keeps for `x=x`.
//
// The second row is the one a per-name guard would pass: neither value names
// itself, and the pair only closes a cycle through the other.
func TestASelfNamingReevalIsBoundedRatherThanFollowed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value naming its own expansion", `v='${(e)v}'; printf "[%s]" "${(e)v}"; echo after`, "sh: ${(e)v}: nested too deeply\n"},
		{"and a pair naming each other", `a='${(e)b}'; b='${(e)a}'; printf "[%s]" "${(e)a}"; echo after`, "sh: ${(e)a}: nested too deeply\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the refusal to be fatal")
			}
		})
	}
}

// And the bound is nowhere near a nesting that means something: a value
// holding `${(e)…}` of another value resolves in two steps and is not touched
// by it. The row exists because a bound set at one would pass the two tests
// above and break this.
func TestReevalNestingThatMeansSomethingIsUntouched(t *testing.T) {
	out, st := runGrammar(t,
		`x=deep; l1='$x'; l2='${(e)l1}'; printf "[%s]" "${(e)l2}"`, escapingFlags, nil)
	if out != "[deep]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[deep]")
	}
}
