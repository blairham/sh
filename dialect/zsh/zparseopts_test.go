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
// `-M` makes one description store under another's, and the two halves of a
// match come apart there: which description decides whether an argument is
// taken, and which decides the shape it lands in. Measured 2026-09-08 against
// zsh 5.9.2 in a 131-case differential run; the other five shells of the panel
// answer `command not found` at 127, so there is nothing here for a dialect
// axis to disagree about.
func TestZparseoptsMapsOneDescriptionOntoAnother(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "the-equals-half-names-a-description-instead-of-an-array",
		snippet: `set -- -a -b; zparseopts -M -a arr a=b b; echo "st=$? arr=[${(j:|:)arr}] b=[${(j:|:)b}]"`,
		want:    "st=0 arr=[-a] b=[]\n",
	}, {
		name:    "and-without-the-letter-the-same-line-names-an-array",
		snippet: `set -- -a -b; zparseopts -a arr a=b b; echo "st=$? arr=[${(j:|:)arr}] b=[${(j:|:)b}]"`,
		want:    "st=0 arr=[-b] b=[-a]\n",
	}, {
		name:    "a-name-no-description-has-is-still-an-array",
		snippet: `set -- -a; zparseopts -M -a arr a=foo; echo "st=$? arr=[${(j:|:)arr}] foo=[${(j:|:)foo}]"`,
		want:    "st=0 arr=[] foo=[-a]\n",
	}, {
		name:    "links-follow-through-a-chain",
		snippet: `set -- -a v; zparseopts -M a:=b b:=c c:=q; echo "st=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-a|v]\n",
	}, {
		name:    "the-first-spelling-and-the-last-argument-share-one-slot",
		snippet: `set -- -a v -b w; zparseopts -M a:=b b:=q; echo "st=$? q=[${(j:|:)q}]"; set -- -b w -a v; zparseopts -M a:=b b:=q; echo "other=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-a|w]\nother=0 q=[-b|v]\n",
	}, {
		name:    "a-later-match-without-an-argument-shortens-the-slot",
		snippet: `set -- -b w -a; zparseopts -M a=b b:=q; echo "st=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-b]\n",
	}, {
		name:    "a-joined-target-spells-the-element-its-own-way",
		snippet: `set -- -a v; zparseopts -M a:=b b:-=q; echo "st=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-bv]\n",
	}, {
		name:    "the-target-decides-the-shape-and-the-match-decides-the-argument",
		snippet: `set -- -a v -b; zparseopts -D -M a:=b b=q; echo "st=$? q=[${(j:|:)q}] argv=[${(j:|:)@}]"; set -- -a v; zparseopts -D -M a=b b:=q; echo "other=$? q=[${(j:|:)q}] argv=[${(j:|:)@}]"`,
		want:    "st=0 q=[-a] argv=[]\nother=0 q=[-a] argv=[v]\n",
	}, {
		name:    "the-plus-of-the-target-decides-and-not-the-sources",
		snippet: `set -- -a v1 -b w -a v2; zparseopts -M a+:=b b:=q; echo "st=$? q=[${(j:|:)q}]"; set -- -a v1 -b w -a v2; zparseopts -M a:=b b+:=q; echo "plus=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-a|v2]\nplus=0 q=[-a|v1|-b|w|-a|v2]\n",
	}, {
		// The `+` on the source above is read for nothing, and this is where
		// that is observable: the slot still keeps the first spelling. A
		// reading that took `+` from the source would spell this `-b`, which
		// the row above cannot tell apart because its last match happens to
		// be the first one's option again.
		name:    "a-source-with-a-plus-does-not-unpin-the-first-spelling",
		snippet: `set -- -a v1 -b w; zparseopts -M a+:=b b:=q; echo "st=$? q=[${(j:|:)q}]"; set -- -b w -a v1; zparseopts -M a+:=b b:=q; echo "other=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-a|w]\nother=0 q=[-b|v1]\n",
	}, {
		name:    "an-accumulating-joined-target-spells-every-element-its-own-way",
		snippet: `set -- -a v1 -b w -a v2; zparseopts -M a:=b b+:-=q; echo "st=$? q=[${(j:|:)q}]"`,
		want:    "st=0 q=[-bv1|-bw|-bv2]\n",
	}, {
		name:    "the-association-is-keyed-by-the-target",
		snippet: `set -- -a v -b w; zparseopts -M -A h a:=b b:; echo "st=$? ${(kv)h}"; set -- -a v -b w; zparseopts -M -A g a:=b b+:; echo "plus=$? ${(kv)g}"`,
		want:    "st=0 -b w\nplus=0 -b vw\n",
	}, {
		name:    "the-manual-s-own-example",
		snippet: `set -- -a -b1 -b2 -c3; zparseopts -A bar -M a=foo b+: c:=b; echo "st=$? foo=[${(j:|:)foo}] ${(kv)bar}"`,
		want:    "st=0 foo=[-a] -a  -b 123\n",
	}})
}

// A description aliased to itself, and a cycle, which are two different
// answers rather than one.
func TestZparseoptsRefusesACycleAndNotASelfMapping(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "a-cycle-of-two-names-the-description-that-closes-it",
		snippet: `set -- -a; zparseopts -M -a arr a=b b=a 2>&1; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "zsh:zparseopts:1: cyclic option mapping: b=a\nst=1 arr=[]\n",
	}, {
		name:    "a-longer-cycle-names-the-link-that-returns",
		snippet: `zparseopts -M -a arr a=b b=c c=a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: cyclic option mapping: c=a\nst=1\n",
	}, {
		name:    "entered-from-outside-it-is-still-the-closing-link",
		snippet: `zparseopts -M -a arr x=a a=b b=a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: cyclic option mapping: b=a\nst=1\n",
	}, {
		name:    "the-description-is-quoted-as-it-was-written",
		snippet: `zparseopts -M -a arr a:=b b:=a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: cyclic option mapping: b:=a\nst=1\n",
	}, {
		name:    "a-self-mapping-stores-in-no-array-at-all",
		snippet: `set -- -a -b; zparseopts -M -a arr a=a b; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[-b]\n",
	}, {
		name:    "and-still-reaches-the-association-under-its-own-name",
		snippet: `set -- -a; zparseopts -M -a arr -A h a=a; echo "st=$? arr=[${(j:|:)arr}] ${(kv)h}"`,
		want:    "st=0 arr=[] -a \n",
	}, {
		name:    "a-cycle-is-refused-before-the-missing-array-is",
		snippet: `zparseopts -M a=b b=a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: cyclic option mapping: b=a\nst=1\n",
	}, {
		name:    "and-after-a-description-that-has-nowhere-to-put-anything",
		snippet: `zparseopts -M c a=b b=a 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: no default array defined: c\nst=1\n",
	}})
}

// Where a repeated option's answer *sits*, which is not where a reading of
// "only the last occurrence survives" puts it. Nothing to do with `-M` — it is
// the rule `-M` generalizes, and it was wrong here until `-M` was built.
func TestZparseoptsKeepsARepeatedOptionInItsFirstPlace(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "the-slot-stays-put-and-the-argument-is-replaced",
		snippet: `set -- -a v1 -c z -a v2; zparseopts -a arr a: c:; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[-a|v2|-c|z]\n",
	}, {
		name:    "and-a-flag-does-not-move-to-the-end-either",
		snippet: `set -- -a -c -a; zparseopts -a arr a c; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[-a|-c]\n",
	}, {
		name:    "a-joined-element-is-rewritten-in-place",
		snippet: `set -- -av1 -c -av2; zparseopts -a arr a:- c; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[-av2|-c]\n",
	}})
}

// The default array is emptied whether or not a description names it, which is
// the half of `-K` that is invisible until nothing matches.
func TestZparseoptsEmptiesTheDefaultArrayNobodyNamed(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "no-description-uses-it-and-it-is-emptied-anyway",
		snippet: `arr=(old); zparseopts -a arr a=q; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[]\n",
	}, {
		name:    "and-dash-k-is-what-keeps-it",
		snippet: `arr=(old); zparseopts -K -a arr a=q; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[old]\n",
	}})
}

// An array name that is no name at all. The check is not static: it fires
// where the array would be written, which is why `-K` with nothing to store is
// silent and the same line with something to store is not.
//
// Real zsh *aborts* a non-interactive shell after this sentence, so the `echo`
// after each of these never runs there. The sentence and the status are
// reproduced and the fatality is not — the same trade the builtin's own
// comment records — which is the one respect in which these wants are not real
// zsh's transcript.
func TestZparseoptsRefusesAnArrayNameWhereItWritesIt(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "a-missing-argument-is-refused-before-the-name-is",
		snippet: `set -- -a; zparseopts -a arr "a:=" 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: missing argument for option: -a\nst=1\n",
	}, {
		name:    "an-equals-with-nothing-after-it-names-an-array-called-nothing",
		snippet: `set -- -a; zparseopts -a arr "a=" 2>&1; echo "st=$?"`,
		want:    "zsh:1: not an identifier: \nst=1\n",
	}, {
		name:    "and-a-description-with-no-equals-at-all-is-silent",
		snippet: `set -- -a; zparseopts -a arr a; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[-a]\n",
	}, {
		name:    "nothing-written-is-nothing-checked",
		snippet: `zparseopts -K -a arr "a=1bad" 2>&1; echo "st=$?"; set -- -a; zparseopts -K -a arr "a=1bad" 2>&1; echo "wrote=$?"`,
		want:    "st=0\nzsh:1: not an identifier: 1bad\nwrote=1\n",
	}, {
		name:    "the-missing-array-complaint-comes-first-whatever-the-order",
		snippet: `zparseopts "a=1bad" b 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: no default array defined: b\nst=1\n",
	}, {
		// And an `=` with nothing after it satisfies that check as much as
		// one with a name after it, which is the other side of the empty
		// array name: the complaint is about `b` and not about `a=`.
		name:    "an-empty-equals-still-counts-as-having-named-an-array",
		snippet: `zparseopts "a=" b 2>&1; echo "st=$?"`,
		want:    "zsh:zparseopts:1: no default array defined: b\nst=1\n",
	}, {
		name:    "the-descriptions-are-scanned-last-first-and-the-default-after-them",
		snippet: `zparseopts -a arr "a=1bad" "b=2bad" 2>&1; echo "st=$?"; zparseopts -a "1bad" "a=2bad" b 2>&1; echo "other=$?"`,
		want:    "zsh:1: not an identifier: 2bad\nst=1\nzsh:1: not an identifier: 2bad\nother=1\n",
	}, {
		name:    "removal-still-happened",
		snippet: `set -- -a v; zparseopts -D -a arr "a=1bad" 2>&1; echo "st=$? argv=[${(j:|:)@}]"`,
		want:    "zsh:1: not an identifier: 1bad\nst=1 argv=[v]\n",
	}, {
		name:    "an-alias-may-name-a-description-that-is-no-identifier",
		snippet: `set -- --b -a; zparseopts -M -a arr a=-b -b; echo "st=$? arr=[${(j:|:)arr}]"`,
		want:    "st=0 arr=[--b]\n",
	}})
}

// A refusal leaves the store as it found it (#1535). The assertion is
// `typeset -p` — the name's *type* and its contents, not the diagnostic —
// because the damage this is about is silent and is read several lines later:
// a name a function declared `local -a` holding a string answers every later
// `${opts[@]}` and `${#opts}` as a string, at status 0, with nothing said.
//
// Every want here is real zsh's own, byte for byte, including the location
// inside the function.
func TestZparseoptsRefusesWithoutTouchingItsTarget(t *testing.T) {
	runZparseoptsCases(t, []zparseoptsCase{{
		name:    "a-declared-array-is-still-an-empty-array",
		snippet: `f() { local -a opts; zparseopts -a opts "X:::y" 2>&1; print -r -- "st=$? n=${#opts[@]}"; typeset -p opts; }; f -X`,
		want:    "f:zparseopts: invalid option description: X:::y\nst=1 n=0\ntypeset -a opts=(  )\n",
	}, {
		name:    "an-array-holding-elements-keeps-them",
		snippet: `f() { local -a opts=(p q); zparseopts -F -a opts X 2>&1; print -r -- "st=$?"; typeset -p opts; }; f -Z`,
		want:    "f:zparseopts: bad option: -Z\nst=1\ntypeset -a opts=( p q )\n",
	}, {
		name:    "an-unset-name-stays-unset",
		snippet: `f() { zparseopts -a opts "X:::y" 2>&1; print -r -- "st=$? [${opts-UNSET}]"; }; f -X`,
		want:    "f:zparseopts: invalid option description: X:::y\nst=1 [UNSET]\n",
	}, {
		name:    "a-scalar-stays-a-scalar",
		snippet: `f() { local opts=str; zparseopts -F -a opts X 2>&1; print -r -- "st=$?"; typeset -p opts; }; f -Z`,
		want:    "f:zparseopts: bad option: -Z\nst=1\ntypeset opts=str\n",
	}, {
		name:    "the-new-refusal-is-the-same",
		snippet: `f() { local -a opts=(p q); zparseopts -M -a opts a=b b=a 2>&1; print -r -- "st=$?"; typeset -p opts; }; f -a`,
		want:    "f:zparseopts: cyclic option mapping: b=a\nst=1\ntypeset -a opts=( p q )\n",
	}, {
		name:    "and-the-line-the-issue-was-found-on-now-parses",
		snippet: `f() { local -a opts; zparseopts -D -E -M -a opts X; print -r -- "st=$? [${opts[(r)-X]}]"; typeset -p opts; }; f -X`,
		want:    "st=0 [-X]\ntypeset -a opts=( -X )\n",
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
