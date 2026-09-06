// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zparseopts`, measured against zsh 5.9.2 (2026-09-06) with a scratch HOME
// and no startup files. Every want below is a byte-for-byte transcript of
// what the real builtin wrote for the same snippet, taken from a 143-case
// differential run.
//
// The assertions are whole strings rather than substrings on purpose. This
// builtin's failure mode is not a crash: it is a caller handed plausible
// variables at status 0, so a check that the array merely *contains* the
// option would pass for a parser that put the argument in the wrong element,
// dropped the second occurrence, or left the option in `$@`.

// zparseoptsCase is one snippet and the whole of what the real builtin wrote.
type zparseoptsCase struct {
	name    string
	snippet string
	want    string
	// status is the shell's own exit status, which is 0 for everything that
	// does not stop the script — a builtin's refusal included, since the
	// snippets print their own `st=`.
	status int
}

func runZparseoptsCases(t *testing.T, cases []zparseoptsCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.snippet)
			if out != tc.want || st != tc.status {
				t.Errorf("%s\n = %q (status %d)\nwant %q", tc.snippet, out, st, tc.want)
			}
		})
	}
}

// Where an option's argument goes, which is the whole of the spec grammar and
// the thing a caller reads by index.
func TestZparseoptsPutsAnArgumentWhereTheDescriptionSaysTo(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "a-flag-is-one-element-and-the-word-stays",
		snippet: `set -- -a foo; zparseopts a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] argv=[-a|foo]\n",
	}, {
		name:    "a-mandatory-argument-is-an-element-of-its-own",
		snippet: `set -- -a val rest; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a|val] argv=[rest]\n",
	}, {
		name:    "the-dash-minus-form-joins-it-to-the-option",
		snippet: `set -- -a val r; zparseopts -D a:-=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-aval] argv=[r]\n",
	}, {
		name:    "an-optional-argument-joins-it-too",
		snippet: `set -- -aval rest; zparseopts -D a::=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-aval] argv=[rest]\n",
	}, {
		// The surprising one, and the reason "optional" is not "in the same
		// word": a following word *is* taken.
		name:    "an-optional-argument-reaches-the-next-word",
		snippet: `set -- -a rest more; zparseopts -D a::=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-arest] argv=[more]\n",
	}, {
		name:    "but-not-one-that-looks-like-an-option",
		snippet: `set -- -a -b; zparseopts -D a::=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[-a] y=[-b]\n",
	}, {
		// Where a mandatory argument differs from an optional one: it takes
		// whatever is next, `--` included.
		name:    "a-mandatory-argument-takes-even-a-double-dash",
		snippet: `set -- -a -- z; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a|--] argv=[z]\n",
	}, {
		name:    "and-a-missing-one-is-a-refusal-that-changes-nothing",
		snippet: `set -- -a; zparseopts -D a:=x 2>&1; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want: "zsh:zparseopts:1: missing argument for option: -a\n" +
			"st=1 x=[] argv=[-a]\n",
	}})
}

// Which occurrence survives, and in what order — the part a per-array rule
// would get wrong.
func TestZparseoptsKeepsTheLastOccurrenceOfEachDescription(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "without-plus-only-the-last",
		snippet: `set -- -a v1 -a v2; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "st=0 x=[-a|v2]\n",
	}, {
		name:    "with-plus-every-one",
		snippet: `set -- -a v1 -a v2 -a v3; zparseopts -D a+:=x; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "st=0 x=[-a|v1|-a|v2|-a|v3]\n",
	}, {
		// Two descriptions sharing one array: the pruning is per description
		// and the array is still in command-line order, so a rule that kept
		// "the last occurrence" per *array* would answer `(-b)` here.
		name:    "two-descriptions-share-an-array-in-command-line-order",
		snippet: `set -- -a -b c; zparseopts -D -a arr a b; echo "st=$? arr=[${(j:|:)arr}] argv=[${(j:|:)@}]"`,
		want:    "st=0 arr=[-a|-b] argv=[c]\n",
	}, {
		// The manual's own example, which exercises both rules at once.
		name: "the-manuals-example",
		snippet: `set -- -a -bx -c y -cz baz -cend; zparseopts a=foo b:=bar c+:=bar` +
			`; echo "st=$? foo=[${(j:|:)foo}] bar=[${(j:|:)bar}] argv=[${(j:|:)@}]"`,
		want: "st=0 foo=[-a] bar=[-b|x|-c|y|-c|z] argv=[-a|-bx|-c|y|-cz|baz|-cend]\n",
	}})
}

// Where parsing stops, which decides what a function passes on to something
// else.
func TestZparseoptsStopsWhereTheDescriptionsRunOut(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "at-the-first-word-no-description-covers",
		snippet: `set -- x -a y; zparseopts -D a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[] argv=[x|-a|y]\n",
	}, {
		name:    "unless-dash-e-says-to-keep-going",
		snippet: `set -- x -a y; zparseopts -D -E a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] argv=[x|y]\n",
	}, {
		// Always at `-` or `--`, `-E` or not, and `-D` removes the one it
		// stopped at — but only without `-E`.
		name:    "always-at-a-double-dash-which-dash-d-removes",
		snippet: `set -- -a -- -b; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] y=[] argv=[-b]\n",
	}, {
		name:    "and-dash-e-leaves-the-double-dash-in-place",
		snippet: `set -- -a -- -b; zparseopts -D -E a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] y=[] argv=[--|-b]\n",
	}, {
		name:    "a-bare-dash-stops-it-the-same-way",
		snippet: `set -- -a - -b; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] argv=[-b]\n",
	}, {
		name:    "and-without-dash-d-nothing-is-removed-at-all",
		snippet: `set -- -a v; zparseopts -A o -- a:; echo "st=$? ${(kv)o} argv=[${(j:|:)@}]"`,
		want:    "st=0 -a v argv=[-a|v]\n",
	}})
}

// `-F` is the validating form, and what it does *not* do is the point: an
// unknown option is a refusal, and nothing is written or removed.
func TestZparseoptsDashFRefusesAnUnknownOptionAndWritesNothing(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "an-unknown-option-is-quietly-a-stop-without-dash-f",
		snippet: `set -- -z r; zparseopts -D a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[] argv=[-z|r]\n",
	}, {
		// The one that says `-F` discards: `-a` matched before `-z` was
		// reached, and `x` is still empty.
		name:    "and-a-refusal-that-discards-what-had-matched-with-it",
		snippet: `set -- -a -z; zparseopts -D -F a=x 2>&1; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "zsh:zparseopts:1: bad option: -z\nst=1 x=[] argv=[-a|-z]\n",
	}, {
		// A plain word is not option-like, so `-F` has nothing to say about
		// it — it is an ordinary stop.
		name:    "a-plain-word-is-not-what-dash-f-complains-about",
		snippet: `set -- q -a; zparseopts -D -F a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[] argv=[q|-a]\n",
	}})
}

// How a description's name is spelled, and which one wins when two overlap.
func TestZparseoptsMatchesTheLongestOrTheLastDescription(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "one-dash-is-added-to-the-name-as-written",
		snippet: `set -- --foo v r; zparseopts -D -- -foo:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[--foo|v] argv=[r]\n",
	}, {
		name:    "so-a-plain-name-is-a-single-dash-option",
		snippet: `set -- -foo bar; zparseopts -D -- foo=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-foo] argv=[bar]\n",
	}, {
		// No GNU `=` handling, deliberately: the argument is `=v`.
		name:    "an-equals-sign-is-part-of-the-argument",
		snippet: `set -- --foo=v r; zparseopts -D -- -foo:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[--foo|=v] argv=[r]\n",
	}, {
		name:    "flags-that-overlap-give-it-to-the-longest-name",
		snippet: `set -- --foobar; zparseopts -D -- -foobar=y -foo=x; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[] y=[--foobar]\n",
	}, {
		// And when one of them takes an argument the rule changes to "the
		// last written", which is observable only because it is
		// order-dependent — these two differ in nothing else.
		name:    "but-an-argument-in-the-overlap-gives-it-to-the-last-written",
		snippet: `set -- --foobar; zparseopts -D -- -foo:=x -foobar=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[] y=[--foobar]\n",
	}, {
		name:    "the-same-two-written-the-other-way-round",
		snippet: `set -- --foobar; zparseopts -D -- -foobar=y -foo:=x; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[--foo|bar] y=[]\n",
	}, {
		name:    "flags-cluster-in-one-word",
		snippet: `set -- -ab r; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a] y=[-b] argv=[r]\n",
	}, {
		name:    "a-special-character-in-a-name-is-escaped",
		snippet: `set -- -a:b; zparseopts -D -- "a\:b=x"; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		want:    "st=0 x=[-a:b] argv=[]\n",
	}})
}

// `-A` and `-K`: the association, and what survives a call.
func TestZparseoptsWritesAnAssociationAndDashKKeepsWhatWasThere(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "a-flag-is-a-key-with-an-empty-value",
		snippet: `set -- -a; zparseopts -D -A o -- a; echo "st=$? [${(kv)o}]"`,
		want:    "st=0 [-a ]\n",
	}, {
		name:    "and-an-argument-is-the-value-however-it-was-spelled",
		snippet: `set -- -av; zparseopts -D -A o -- a::; echo "st=$? [${(kv)o}]"`,
		want:    "st=0 [-a v]\n",
	}, {
		// Two occurrences under `+` concatenate in the association where
		// they accumulate as elements in an array.
		name:    "plus-concatenates-in-an-association",
		snippet: `set -- -a 1 -a 2; zparseopts -D -A o -- a+:; echo "st=$? ${(kv)o}"`,
		want:    "st=0 -a 12\n",
	}, {
		name:    "an-array-nothing-matched-is-emptied",
		snippet: `x=(old); set -- -b; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[] y=[-b]\n",
	}, {
		name:    "unless-dash-k-says-to-keep-it",
		snippet: `x=(old); set -- -b; zparseopts -D -K a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "st=0 x=[old] y=[-b]\n",
	}, {
		// And `-K` on an array that *did* match still replaces the whole
		// thing — the letter is about arrays nothing matched, not about
		// accumulating into one that did.
		name:    "and-dash-k-still-replaces-an-array-that-matched",
		snippet: `x=(old); set -- -a; zparseopts -D -K a+=x; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "st=0 x=[-a]\n",
	}, {
		// An association is the other way round: `-K` keeps its individual
		// elements.
		name:    "an-association-keeps-its-other-elements-under-dash-k",
		snippet: `typeset -A o; o[z]=1; set -- -a; zparseopts -D -K -A o a b; echo "st=$? ${(kv)o}"`,
		want:    "st=0 -a  z 1\n",
	}, {
		name:    "and-loses-them-without-it",
		snippet: `typeset -A o; o[k]=1; set -- -a; zparseopts -D -A o a; echo "st=$? o=[${(kv)o}]"`,
		want:    "st=0 o=[-a ]\n",
	}})
}

// The refusals, whole lines with their locations — which is half of what each
// one says.
func TestZparseoptsRefusals(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "nothing-to-parse-against",
		snippet: `set -- -a; zparseopts -D 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: missing option descriptions\nst=1\n",
	}, {
		name:    "nowhere-to-put-what-it-finds",
		snippet: `zparseopts -D a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: no default array defined: a\nst=1\n",
	}, {
		// The letters are not stackable, so an unknown one becomes a
		// description and names itself in that complaint rather than in a
		// `bad option`. That is a fact about the syntax: `-DEK` cannot be
		// told from a description of `--DEK`.
		name:    "an-unknown-letter-is-read-as-a-description",
		snippet: `zparseopts -Q a=x 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: no default array defined: -Q\nst=1\n",
	}, {
		name:    "and-so-is-a-stack-of-real-ones",
		snippet: `set -- -a; zparseopts -DQ a=x 2>&1; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "zsh:zparseopts:1: no default array defined: -DQ\nst=1 x=[]\n",
	}, {
		name:    "a-description-with-no-option-in-it-is-not-an-error",
		snippet: `zparseopts -D -a arr "=x" "=y"; echo "st=$?"`,
		want:    "st=0\n",
	}, {
		// And it matches *nothing* rather than everything, which is what an
		// unguarded prefix test would make of an empty name: `-z` is still
		// an option no description covers, so the parse stops there and
		// leaves it. The mutant that drops the guard does not answer this
		// wrongly — it does not answer at all, because a zero-length match
		// never advances past the word.
		name:    "and-it-matches-nothing-rather-than-everything",
		snippet: `set -- -z r; zparseopts -D -a arr "=x"; echo "st=$? arr=[${(j:|:)arr}] argv=[${(j:|:)@}]"`,
		want:    "st=0 arr=[] argv=[-z|r]\n",
	}, {
		name:    "a-third-colon-is-not-a-form",
		snippet: `set -- -a v; zparseopts -D a:::=x 2>&1; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "zsh:zparseopts:1: invalid option description: a:::=x\nst=1 x=[]\n",
	}, {
		name:    "the-same-option-described-twice",
		snippet: `set -- -a; zparseopts -D a=x a=y 2>&1; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		want:    "zsh:zparseopts:1: option defined more than once: a\nst=1 x=[] y=[]\n",
	}, {
		name:    "an-array-letter-with-no-name-after-it",
		snippet: `zparseopts -D -A 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: missing array name\nst=1\n",
	}})
}

// `-M` is refused by name rather than built. See the constant in
// zparseopts.go for why: two measurements of it need two different rules, so
// a parser that picked one would bind the other case wrongly at status 0.
func TestZparseoptsRefusesTheMappingLetterByName(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "dash-m",
		snippet: `set -- -a v; zparseopts -D -M a:=x 2>&1; echo "st=$? x=[${(j:|:)x}]"`,
		want:    "zsh:zparseopts:1: -M is not implemented yet\nst=1 x=[]\n",
	}})
}

// The two builtins are registrations of this dialect and not the substrate's,
// which the feature model is the cheapest place to see: `zmodload zsh/zutil`
// asks the runner whether it has each of the module's four builtins by name,
// so a registration that never happened would show up there — and `whence -w`
// says the same thing one name at a time, `zregexparse` included.
func TestZutilLoadsWithThreeOfItsFourBuiltins(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil 2>&1
print -r -- "st=$?"
whence -w zparseopts zformat zstyle zregexparse`)
	want := "st=0\n" +
		"zparseopts: builtin\nzformat: builtin\nzstyle: builtin\nzregexparse: none\n"
	// 1 is `whence -w`'s answer for the name that is not there, which is the
	// half of this that says the load above is not a stale table claiming a
	// builtin this shell has not got.
	if out != want || st != 1 {
		t.Errorf("zutil feature answer = %q (status %d), want %q", out, st, want)
	}
}
