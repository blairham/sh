// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// subGrammar is the grammar this construct needs, named by the constructs and
// not by a shell: a subscript to put a group in, the group itself, and the
// brace-less spelling one row exercises.
func subGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	d.BareSubscript = true
}

// runSub runs src with that grammar and the two answers the measured shell
// gives that a search depends on: arrays are indexed from one, and an
// expansion's result is not a pattern — the second so that a row showing a
// substituted value's metacharacters *are* live in a subscript is showing
// this construct's own rule rather than a dialect's.
func runSub(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, subGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		// The measured shell's answer, and the row that says a subscript is
		// never matched against the filesystem: with it off, a subscript
		// that globbed would quietly come back as itself and no test could
		// tell.
		sem.GlobNoMatchIsError = Yes
		r.Semantics = &sem
	})
}

// runAssoc is runSub with the two more answers an *association* needs: the
// declaration attribute's grammar, the expansion flag group so that `(k)`,
// `(v)` and `(o)` can be written in front of a search, and the measured
// answer to whether a subscript quotes — it does not, which is what lets a
// key hold a space without the quotes becoming part of it.
func runAssoc(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		subGrammar(d)
		d.ParamExpansionFlags = true
		d.ParamElementSelection = true
		d.ArrayLiteral = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		sem.GlobNoMatchIsError = Yes
		sem.SubscriptIsAQuotingContext = No
		// A quoted operator on a joined subscript applies to the joined text
		// rather than to each element, which is the measured answer and the
		// axis an operator written on a search reaches.
		sem.OperatorDistributesOverStarSubscript = No
		r.Semantics = &sem
	})
}

// The array every row below searches. Five elements, with `beta` twice so
// that first and last are different answers, and every element ending in `a`
// so that a pattern has more than one match to choose between.
const subArray = `a=(alpha beta gamma beta delta); `

// The four selecting flags and their two no-match answers.
//
// The exact element and the exact index, never "no error": a subscript-flag
// bug returns a plausible element at status 0, so a row that asserted only
// the status would pass while the shell handed a script the wrong directory.
func TestASubscriptFlagGroupSelectsTheElementItNames(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"r is the first match", `${a[(r)*a]}`, "alpha"},
		{"R is the last match", `${a[(R)*a]}`, "delta"},
		{"i is the first index", `${a[(i)be*]}`, "2"},
		{"I is the last index", `${a[(I)be*]}`, "4"},
		{"r with no match is nothing", `${a[(r)zz]}`, ""},
		{"R with no match is nothing", `${a[(R)zz]}`, ""},
		{"i with no match is one past the end", `${a[(i)zz]}`, "6"},
		{"I with no match is one before the start", `${a[(I)zz]}`, "0"},
		{"the pattern is a pattern", `${a[(r)*mm*]}`, "gamma"},
		{"and e makes it a string", `${a[(re)*mm*]}`, ""},
		{"e finds what it is equal to", `${a[(re)gamma]}`, "gamma"},
		{"e reaches the index flags too", `${a[(ie)be*]}`, "6"},
		{"the last selecting flag wins", `${a[(ir)be*]}`, "beta"},
		{"and it wins the other way", `${a[(ri)be*]}`, "2"},
		{"n asks for the nth match", `${a[(rn:2:)*a]}`, "beta"},
		{"n counts backwards for R", `${a[(In:2:)*a]}`, "4"},
		{"n is an expression", `${a[(rn:1+1:)*ta]}`, "beta"},
		{"n below one is one", `${a[(in:0:)*a]}`, "1"},
		{"n past the last match misses", `${a[(in:9:)*a]}`, "6"},
		{"b moves the start forwards", `${a[(ib:3:)*a]}`, "3"},
		{"b moves it backwards for I", `${a[(Ib:2:)*a]}`, "2"},
		{"b counts back from the end", `${a[(ib:-2:)*a]}`, "4"},
		{"b below the first element is the first", `${a[(ib:0:)*a]}`, "1"},
		{"b past the end searches nothing", `${a[(ib:6:)*a]}`, "6"},
		{"and nothing in reverse either", `${a[(Ib:6:)*a]}`, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, subArray+`printf "[%s]" "`+tc.src+`"`)
			if want := "[" + tc.want + "]"; out != want {
				t.Errorf("%s = %q, want %q", tc.src, out, want)
			}
			if status != 0 {
				t.Errorf("%s: status %d, want 0", tc.src, status)
			}
		})
	}
}

// A group with none of the four selecting flags leaves the subscript read
// exactly as it would have been without a group — with one exception, which
// is measured and has a test of its own below: `@` and `*` stop naming the
// whole array as soon as anything opens a group.
func TestASubscriptFlagGroupThatSelectsNothingChangesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an empty group", `${a[()2]}`, "beta"},
		{"a bare e", `${a[(e)2]}`, "beta"},
		{"an expression behind one", `${a[(e)1+1]}`, "beta"},
		{"a name behind one is zero, so no element", `${a[(e)beta]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, subArray+`printf "[%s]" "`+tc.src+`"`)
			if want := "[" + tc.want + "]"; out != want {
				t.Errorf("%s = %q, want %q", tc.src, out, want)
			}
			if status != 0 {
				t.Errorf("%s: status %d, want 0", tc.src, status)
			}
		})
	}
}

// The operand is the subscript's text with its substitutions performed and
// nothing else touched, which is one rule with three consequences.
func TestASubscriptSearchOperandIsTheTextAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a substituted value's metacharacters are live",
			`a=(alpha beta gamma); g='be*'; printf "[%s]" "${a[(r)$g]}"`,
			"[beta]",
		},
		{
			"a substitution reaches the operand at all",
			`a=(alpha beta gamma); d=beta; printf "[%s]" "${a[(r)$d]}"`,
			"[beta]",
		},
		{
			"quotes are matched rather than removed",
			`b=('"beta"' beta); printf "[%s]" "${b[(r)"beta"]}"`,
			`["beta"]`,
		},
		{
			"and the value inside them is still substituted",
			`b=('"beta"' beta); h=beta; printf "[%s]" "${b[(r)"$h"]}"`,
			`["beta"]`,
		},
		{
			"exact matching sees them too",
			`b=('"beta"' beta); printf "[%s]" "${b[(re)"beta"]}"`,
			`["beta"]`,
		},
		{
			"single quotes are matched as well",
			`b=("'beta'" beta); printf "[%s]" "${b[(r)'beta']}"`,
			`['beta']`,
		},
		{
			"a metacharacter across two spans is still not a filename",
			`b=('(e)beta' beta); printf "[%s]" "${b[(re)(e)beta]}"`,
			`[(e)beta]`,
		},
		{
			"a backslash escapes a metacharacter",
			`b=(bex 'be*'); printf "[%s]" "${b[(r)be\*]}" "${b[(r)be*]}"`,
			`[be*][bex]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// A name holding nothing and an array holding nothing are different answers,
// which is the row a search over the empty case would otherwise get right by
// accident.
func TestASubscriptSearchOverNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an unset name is not searched", `printf "[%s]" "${nope[(i)x]}"`, "[]"},
		{"nor for an element", `printf "[%s]" "${nope[(r)x]}"`, "[]"},
		{"a declared empty array is", `b=(); printf "[%s]" "${b[(i)x]}"`, "[1]"},
		{"in both directions", `b=(); printf "[%s]" "${b[(I)x]}"`, "[0]"},
		{"and holds no element", `b=(); printf "[%s]" "${b[(r)x]}"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// The positional parameters are searched like any other list.
func TestASubscriptSearchReachesThePositionalParameters(t *testing.T) {
	grammar := func(d *syntax.Dialect) {
		subGrammar(d)
		// `@` is not a name, so a subscript on it is a grammar question of
		// its own and the flag group rides on that answer rather than
		// widening it.
		d.SpecialParamSubscript = true
	}
	out, status := runGrammar(t, `printf "[%s]" "${@[(r)b]}" "${@[(i)c]}"`, grammar,
		func(r *Runner) {
			sem := *r.Semantics
			sem.ArrayBaseIsZero = No
			sem.GlobExpansionResults = No
			sem.SplitParamExpansion = No
			r.Semantics = &sem
			r.Params = []string{"a", "b", "c"}
			d := syntax.Core()
			grammar(&d)
			r.Dialect = &d
		})
	if out != "[b][3]" {
		t.Errorf("= %q, want %q", out, "[b][3]")
	}
	if status != 0 {
		t.Errorf("status %d, want 0", status)
	}
}

// The brace-less spelling reaches the same reading, which is the half a lexer
// change carries: without it the parenthesis ends the word and the file does
// not parse.
func TestASubscriptSearchWithoutBraces(t *testing.T) {
	out, status := runSub(t, subArray+`printf "[%s]" "$a[(r)*a]" "$a[(i)be*]" "$a[(re)be*]"`)
	if want := "[alpha][2][]"; out != want {
		t.Errorf("= %q, want %q", out, want)
	}
	if status != 0 {
		t.Errorf("status %d, want 0", status)
	}
}

// What is read and not carried is refused by name, and the refusal abandons
// the word rather than answering with a plausible element.
func TestASubscriptFlagThisImplementationDoesNotCarryIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, names string }{
		{`printf "[%s]" "${a[(w)beta]}"`, "(w)"},
		{`printf "[%s]" "${a[(f)beta]}"`, "(f)"},
		{`printf "[%s]" "${a[(p)beta]}"`, "(p)"},
		{`printf "[%s]" "${a[(k)beta]}"`, "(k)"},
		{`printf "[%s]" "${a[(K)beta]}"`, "(K)"},
		{`printf "[%s]" "${a[(s:,:)beta]}"`, "(s)"},
		{`printf "[%s]" "${a[(rw)beta]}"`, "(w)"},
	} {
		out, status := runSub(t, subArray+tc.src)
		if !strings.Contains(out, tc.names+" subscript flag is not implemented") {
			t.Errorf("%s: output %q does not refuse %s by name", tc.src, out, tc.names)
		}
		if !strings.Contains(out, "a[") {
			t.Errorf("%s: output %q does not name the subscript", tc.src, out)
		}
		if status == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
	}
}

// A search over a target this does not carry is refused the same way, and the
// refusal replaces a *silent* wrong answer: a scalar answered out of the
// one-element list it is read as, and a character position is not an element.
func TestASubscriptSearchOverATargetThisDoesNotCarryIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, why string }{
		{
			"a scalar",
			`s="one two"; printf "[%s]" "${s[(r)two]}"`,
			"for a scalar",
		},
		{
			"and a scalar searched for an index",
			`s=hello; printf "[%s]" "${s[(i)l]}"`,
			"for a scalar",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if !strings.Contains(out, "subscript flag is not implemented "+tc.why) {
				t.Errorf("output %q does not refuse %s", out, tc.why)
			}
			if status == 0 {
				t.Error("status 0, want a failure")
			}
		})
	}
	// A group that selects nothing is not a search, so it does not reach the
	// refusal and the key is still looked up.
	out, status := runSub(t, `typeset -A h; h[k1]=v1; printf "[%s]" "${h[()k1]}"`)
	if out != "[v1]" || status != 0 {
		t.Errorf("= %q status %d, want [v1] 0", out, status)
	}
}

// The shape zi.zsh writes six times, end to end: the answer decides whether a
// directory is prepended to a path that already holds it.
func TestTheShapeAPluginManagerAsksThisWith(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"already on the path, so nothing is added",
			`p=(/u/bin /pfx/bin /x); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"keep",
		},
		{
			"not on it, so it is",
			`p=(/u/bin /x); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"add",
		},
		{
			"and a prefix of an entry is not the entry",
			`p=(/pfx/bindir); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"add",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// The one thing a group changes even when it selects nothing: `@` and `*` are
// no longer the whole array, so they reach the arithmetic and fail there.
//
// Measured on the shell that has the construct, and a wrong answer here is
// silent in the worst way — the whole array where the shell gives an error is
// a value a script will happily use.
func TestAFlagGroupTakesTheWholeArraySpellingsAway(t *testing.T) {
	for _, src := range []string{`${a[()@]}`, `${a[()*]}`, `${a[(e)@]}`, `${a[(r)@]}`} {
		out, _ := runSub(t, subArray+`printf "[%s]" "`+src+`"`)
		if strings.Contains(out, "alpha beta") || strings.Contains(out, "[alpha][beta]") {
			t.Errorf("%s = %q, want no array", src, out)
		}
	}
	// And without a group they still are, which is what says the clause is
	// about the group rather than about the brackets.
	out, status := runSub(t, subArray+`printf "[%s]" "${a[*]}"`)
	if want := "[alpha beta gamma beta delta]"; out != want || status != 0 {
		t.Errorf("${a[*]} = %q status %d, want %q 0", out, status, want)
	}
}

// The association every row below searches. The keys are deliberately not in
// sorted order as written, so a row that came back in the order it was
// assigned would be visible; and the values repeat, so `r` and `R` have more
// than one match to choose between.
const subAssoc = `typeset -A m=(gamma one alpha two beta one); `

// A search over an association is a different construct wearing the same four
// letters, and the difference is the whole of what this asserts: the letters
// select *keys* rather than indices, and the case of the letter is how many
// matches come back rather than which end the search started from.
//
// Measured against zsh 5.9.2. The order the several matches come back in is
// this implementation's own — sorted, for the reason AssocArray.keys() gives
// — so the rows that have several assert the *set* through a sort of their
// own where the shell's hash order would otherwise be pinned here.
func TestASubscriptSearchOverAnAssociationSelectsKeys(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"i is the first matching key", `${m[(i)alpha]}`, "alpha"},
		{"and it is a key and not an index", `${m[(i)*a]}`, "alpha"},
		{"I is every matching key", `${m[(I)*a]}`, "alpha beta gamma"},
		{"r searches the values", `${m[(r)one]}`, "one"},
		{"and a key is not a value", `${m[(r)alpha]}`, ""},
		{"R is every matching value", `${m[(R)one]}`, "one one"},
		{"a literal key with no glob in it", `${m[(I)beta]}`, "beta"},
		{"a glob matching one", `${m[(I)g*]}`, "gamma"},
		{"a glob matching none", `${m[(I)zz*]}`, ""},
		{"an empty pattern matches no key here", `${m[(I)]}`, ""},
		{"i with no match is nothing, not an index", `${m[(i)zz]}`, ""},
		{"I with no match is nothing either", `${m[(I)zz]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, subAssoc+`printf "[%s]" "`+tc.src+`"`)
			if out != "["+tc.want+"]" || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, "["+tc.want+"]")
			}
		})
	}
}

// A key that holds a glob character is found by a group that says the operand
// is literal, and the same group without `(e)` finds both. The pair is what
// says `(e)` is read on this path at all — one row alone passes for an
// implementation that ignores it.
func TestAnExactSearchOverAnAssociation(t *testing.T) {
	const m = `typeset -A m=(aa 1 'a*' 2); `
	for _, tc := range []struct{ name, src, want string }{
		{"exact finds the key spelled with the star", `${m[(Ie)a*]}`, "a*"},
		{"and a pattern finds both", `${m[(I)a*]}`, "a* aa"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, m+`printf "[%s]" "`+tc.src+`"`)
			if out != "["+tc.want+"]" || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, "["+tc.want+"]")
			}
		})
	}
}

// The two modifiers the ordered array's search reads are *ignored* over an
// association, which is measured and not assumed: with three matching keys,
// asking for the third match and starting at the second both still answer
// with the first.
//
// Asserted because ignoring them is the kind of thing that looks like an
// oversight and would be "fixed" into a divergence.
func TestNthAndBeginAreIgnoredOverAnAssociation(t *testing.T) {
	const m = `typeset -A m=(aa 1 ab 2 ac 3); `
	for _, tc := range []struct{ name, src, want string }{
		{"nth is ignored", `${m[(in:3:)a*]}`, "aa"},
		{"begin is ignored", `${m[(ib:2:)a*]}`, "aa"},
		{"and neither moves the every-match answer", `${m[(In:2:)a*]}`, "aa ab ac"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, m+`printf "[%s]" "`+tc.src+`"`)
			if out != "["+tc.want+"]" || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, "["+tc.want+"]")
			}
		})
	}
}

// The search names several elements, so the three readings that ask whether a
// subscript is a list have to agree. They did not before: a count that was a
// width, a quoted expansion that was one field per match, and a flag group
// handed one word made of all of them.
func TestAnAssociationSearchIsAList(t *testing.T) {
	const m = `typeset -A m=('a b' 1 'c d' 2); `
	for _, tc := range []struct{ name, src, want string }{
		{"the length is the match count", `printf "[%s]" "${#m[(I)*]}"`, "[2]"},
		{"and zero where nothing matched", `printf "[%s]" "${#m[(I)zz]}"`, "[0]"},
		{"quoted, the matches are joined", `printf "[%s]" "${m[(I)*]}"`, "[a b c d]"},
		{"unquoted, one field each", `printf "[%s]" ${m[(I)*]}`, "[a b][c d]"},
		{"quoted with @, one field each", `printf "[%s]" "${(@)m[(I)*]}"`, "[a b][c d]"},
		{"no match quoted is one empty field", `set -- "${m[(I)zz]}"; printf "[%s]" "$#"`, "[1]"},
		{"no match unquoted is no field", `set -- ${m[(I)zz]}; printf "[%s]" "$#"`, "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, m+tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
		})
	}
}

// A search with no match is *set* with no elements rather than unset, which is
// the opposite of what the same group over an ordered array answers and is
// measured on both sides. A `-` alternative firing here is the silent shape:
// a script reading `${m[(I)pat]-default}` would take the default for a table
// that simply has no such hook.
func TestAnAssociationSearchWithNoMatchIsSet(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an association search answers even with no match", `typeset -A m=(a 1); printf "[%s]" "${m[(I)zz]-none}"`, "[]"},
		{"a plain missing key does not", `typeset -A m=(a 1); printf "[%s]" "${m[zz]-none}"`, "[none]"},
		{"and an ordered array's search does not either", `a=(x y); printf "[%s]" "${a[(r)zz]-none}"`, "[none]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
		})
	}
}

// Which half of each matched pair comes back is the *expansion's* flag group,
// not the search's, and the answer is the same whichever letter searched.
func TestTheKeyAndValueFlagsOverAnAssociationSearch(t *testing.T) {
	const m = `typeset -A m; m[a]=1; m[b]=2; `
	for _, tc := range []struct{ name, src, want string }{
		{"k over a key search is the keys", `${(k)m[(I)*]}`, "a b"},
		{"v over a key search is the values", `${(v)m[(I)*]}`, "1 2"},
		{"k and v interleave", `${(kv)m[(I)*]}`, "a 1 b 2"},
		{"k over a value search is still the keys", `${(k)m[(R)*]}`, "a b"},
		{"v over a value search is still the values", `${(v)m[(R)*]}`, "1 2"},
		{"and a single match takes the same rule", `${(kv)m[(i)*]}`, "a 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, m+`printf "[%s]" "`+tc.src+`"`)
			if out != "["+tc.want+"]" || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, "["+tc.want+"]")
			}
		})
	}
}

// An operator written on a search follows the rule `${a[*]}` already follows,
// and the two halves of it are opposite: quoted, the matches are joined and
// the operator applies once to the joined text; unquoted, it applies to each.
//
// Measured on zsh 5.9.2 with three matching keys, and both halves are needed —
// a shell that distributed in both would answer `a b c` where the quoted
// spelling is `a pb pc`, and one that joined in both would answer one field
// where the unquoted spelling is three.
//
// It is here rather than in the corpus because the quoted answer says which
// match came *first*, and the corpus may not pin a key order neither shell
// promises. Against this implementation's own sorted order it is exact.
func TestAnOperatorOnAnAssociationSearch(t *testing.T) {
	const m = `typeset -A m=(pa 1 pb 2 pc 3); `
	for _, tc := range []struct{ name, src, want string }{
		{"quoted, joined then trimmed once", `printf "[%s]" "${m[(I)p*]#p}"`, "[a pb pc]"},
		{"unquoted, trimmed one at a time", `printf "[%s]" ${m[(I)p*]#p}`, "[a][b][c]"},
		{"and @ keeps the fields through quotes", `printf "[%s]" "${(@)m[(I)p*]#p}"`, "[a][b][c]"},
		{"a filter reads the joined text in quotes", `printf "[%s]" "${m[(I)p*]:#pa}"`, "[pa pb pc]"},
		{"and a slice slices the list", `printf "[%s]" "${m[(I)p*]:1}"`, "[pb pc]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, m+tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
		})
	}
}

// The shape the plugin manager writes twenty-six times, sorted by a flag
// group of its own — which is the reading that needs the search to be a list
// rather than one word, and the one a join would silently answer with a
// single unsorted field.
func TestTheAssociationSearchAPluginManagerAsksWith(t *testing.T) {
	const exts = `typeset -A e=('z-annex subcommand:wait' w 'z-annex subcommand:load' l other o); `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a literal hook name that is there",
			`printf "[%s]" "${e[(I)z-annex subcommand:wait]}"`,
			"[z-annex subcommand:wait]",
		},
		{
			"one that is not",
			`printf "[%s]" "${e[(I)z-annex subcommand:nope]}"`,
			"[]",
		},
		{
			"and every subcommand, sorted, one field each",
			`printf "[%s]" ${(o)e[(I)z-annex subcommand:*]}`,
			"[z-annex subcommand:load][z-annex subcommand:wait]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, exts+tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
		})
	}
}

// The letters this still does not carry over an association are still refused
// by name. `(k)` and `(K)` are not searches at all there — measured, `${m[(k)a]}`
// is the value at the key `a` and `${m[(k)*]}` is nothing, because the star is
// a key nobody assigned — so building the search did not build them, and the
// guarantee that says so is asserted rather than assumed.
func TestTheSubscriptFlagsStillUnbuiltOverAnAssociation(t *testing.T) {
	for _, tc := range []struct{ src, names string }{
		{`typeset -A m=(a 1); printf "[%s]" "${m[(k)a]}"`, "(k)"},
		{`typeset -A m=(a 1); printf "[%s]" "${m[(K)a]}"`, "(K)"},
		{`typeset -A m=(a 1); printf "[%s]" "${m[(w)a]}"`, "(w)"},
	} {
		out, status := runAssoc(t, tc.src)
		if !strings.Contains(out, tc.names+" subscript flag is not implemented") {
			t.Errorf("%s: output %q does not refuse %s by name", tc.src, out, tc.names)
		}
		if status == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
	}
}
