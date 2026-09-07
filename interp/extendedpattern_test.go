// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The pattern operators one shell keeps behind an option, named as the option
// and never as the shell. Which `setopt` name reaches it is dialect/zsh's
// business and is asserted there.
//
// docs/spec/grammar/patterns.md, "One shell's extended pattern operators".

// The whole point of the option: with it off every one of these characters is
// ordinary text, and two of the rows below are the *opposite* answer from the
// one the same pattern gets with it on. A matcher that read them
// unconditionally would break every pattern holding a `#`.
func TestTheOperatorsAreOrdinaryTextWithTheOptionOff(t *testing.T) {
	for _, tc := range []struct{ src, off, on string }{
		{`[[ 'a#' == a# ]] && echo hit || echo miss`, "hit", "miss"},
		{`[[ aaa == a# ]] && echo hit || echo miss`, "miss", "hit"},
		{`[[ '^y' == ^x ]] && echo hit || echo miss`, "miss", "hit"},
		{`[[ abc == ^x* ]] && echo hit || echo miss`, "miss", "hit"},
		// Off, the tilde is a character the pattern has to find; on, it takes
		// everything matching `*b*` back out of what `a*` found.
		{`[[ 'a~b' == a*~*b* ]] && echo hit || echo miss`, "hit", "miss"},
		// `(#i)` with the option off is a group holding one alternative, so
		// the pattern is the four characters `#ia` and then `bc`.
		{`[[ '#iabc' == (#i)abc ]] && echo hit || echo miss`, "hit", "miss"},
		{`[[ ABC == (#i)abc ]] && echo hit || echo miss`, "miss", "hit"},
	} {
		for _, on := range []bool{false, true} {
			want := tc.off
			if on {
				want = tc.on
			}
			out, _ := runExtendedOperators(t, tc.src, on, nil)
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("on=%v %s = %q, want %q", on, tc.src, got, want)
			}
		}
	}
}

// runExtendedOperators runs src with the option in the state named, and with
// the bare alternation on for the parser *and* the runner — `(#i)` and a
// closure over a group are only distinguishable from literal parentheses in a
// dialect that reads groups at all, and the matcher asks the runner rather
// than the parse.
func runExtendedOperators(t *testing.T, src string, on bool, more func(*Runner)) (string, int) {
	t.Helper()
	enable := func(d *syntax.Dialect) {
		d.PatternAlternation = true
		// The second flag is what lets a `case` arm's pattern open with a
		// `(#…)` at all: without it the `#` behind the arm's paren starts a
		// comment and the arm eats the rest of the line.
		d.GlobQualifiers = true
	}
	return runGrammar(t, src, enable, func(r *Runner) {
		d := syntax.Core()
		enable(&d)
		r.Dialect = &d
		r.SetMatchOption(ExtendedPatternOperators, on)
		if more != nil {
			more(r)
		}
	})
}

func TestTheCaseFlagsFoldOnlyLiterals(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// `(#i)`: a literal folds, and so does an escaped one.
		{`[[ ABC == (#i)abc ]] && echo hit || echo miss`, "hit"},
		{`[[ B == (#i)\b ]] && echo hit || echo miss`, "hit"},
		{`[[ ABC == (#i)?bc ]] && echo hit || echo miss`, "hit"},
		// A bracket expression and a character class do not. This is the
		// row that separates the flag from the run-time fold, which does
		// reach both — measured, real zsh refuses both of these.
		{`[[ ABC == (#i)[abc][abc][abc] ]] && echo hit || echo miss`, "miss"},
		{`[[ ABC == (#i)[[:lower:]][[:lower:]][[:lower:]] ]] && echo hit || echo miss`, "miss"},
		// `(#I)` turns it back off, and only from where it stands.
		{`[[ ABCdef == (#i)abc(#I)def ]] && echo hit || echo miss`, "hit"},
		{`[[ ABCDEF == (#i)abc(#I)def ]] && echo hit || echo miss`, "miss"},
		// `(#l)` folds one way: a lowercase letter in the pattern takes
		// either case, an uppercase one takes only itself.
		{`[[ ABC == (#l)abc ]] && echo hit || echo miss`, "hit"},
		{`[[ abc == (#l)ABC ]] && echo hit || echo miss`, "miss"},
		{`[[ AbC == (#l)abc ]] && echo hit || echo miss`, "hit"},
		// An empty group says nothing and matches.
		{`[[ abc == (#)abc ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A flag reaches to the end of the group it stands in and no further.
func TestAFlagsReachEndsWithItsGroup(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ ABCd == ((#i)abc)d ]] && echo hit || echo miss`, "hit"},
		{`[[ ABCD == ((#i)abc)d ]] && echo hit || echo miss`, "miss"},
		{`[[ ABC == ((#i)abc|zz) ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestAClosureRepeatsOneItem(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Zero or more, and the item is the one character in front of it.
		{`[[ abbb == ab# ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == ab# ]] && echo hit || echo miss`, "hit"},
		{`[[ a == ab# ]] && echo hit || echo miss`, "hit"},
		// Never a run of them, which is what "one item" means.
		{`[[ abab == ab# ]] && echo hit || echo miss`, "miss"},
		// One or more.
		{`[[ ab == ab## ]] && echo hit || echo miss`, "hit"},
		{`[[ a == ab## ]] && echo hit || echo miss`, "miss"},
		// A group, a bracket expression and a `?` are all items.
		{`[[ ababab == (ab)# ]] && echo hit || echo miss`, "hit"},
		{`[[ abab == [ab]# ]] && echo hit || echo miss`, "hit"},
		{`[[ abc == ?# ]] && echo hit || echo miss`, "hit"},
		// The count spelling, with either side left out.
		{`[[ aaa == a(#c3) ]] && echo hit || echo miss`, "hit"},
		{`[[ aa == a(#c3) ]] && echo hit || echo miss`, "miss"},
		{`[[ aaaa == a(#c3) ]] && echo hit || echo miss`, "miss"},
		{`[[ aaa == a(#c2,4) ]] && echo hit || echo miss`, "hit"},
		{`[[ aaaa == a(#c2,3) ]] && echo hit || echo miss`, "miss"},
		{`[[ aaaa == a(#c2,) ]] && echo hit || echo miss`, "hit"},
		{`[[ aa == a(#c,3) ]] && echo hit || echo miss`, "hit"},
		{`[[ aa == a(#c,) ]] && echo hit || echo miss`, "hit"},
		{`[[ b == a(#c0)b ]] && echo hit || echo miss`, "hit"},
		{`[[ ababab == (ab)(#c3) ]] && echo hit || echo miss`, "hit"},
		{`[[ abab == [ab](#c4) ]] && echo hit || echo miss`, "hit"},
		// A `#` with nothing in front of it is the character itself.
		{`[[ '#foo' == \#foo ]] && echo hit || echo miss`, "hit"},
		// And a quoted one is not an operator either, which is the marking
		// a pattern operand carries for text that was quoted.
		{`[[ 'a#b' == "a#b" ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == "a#b" ]] && echo hit || echo miss`, "miss"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestTheNegationTurnsTheRestOfItsBranch(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ abc == ^x* ]] && echo hit || echo miss`, "hit"},
		{`[[ xa == ^x* ]] && echo hit || echo miss`, "miss"},
		{`[[ abc == ^a* ]] && echo hit || echo miss`, "miss"},
		// It starts where it stands rather than at the front of a pattern.
		{`[[ ab == a^x ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == a^b ]] && echo hit || echo miss`, "miss"},
		// Two of them cancel.
		{`[[ ab == ^^x ]] && echo hit || echo miss`, "miss"},
		{`[[ x == ^^x ]] && echo hit || echo miss`, "hit"},
		// Inside a group, and inside one arm of an alternation.
		{`[[ ab == (^x*) ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == (^x|zz) ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestTheExclusionTakesEverySideAgainstTheWholeSubject(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ acd == a*~*b* ]] && echo hit || echo miss`, "hit"},
		{`[[ abd == a*~*b* ]] && echo hit || echo miss`, "miss"},
		// The left side still has to match: nothing here is excluded, and
		// the answer is still a miss.
		{`[[ xyz == a*~*b* ]] && echo hit || echo miss`, "miss"},
		// Two exclusions, both against the subject rather than each other.
		{`[[ acd == a*~*b*~*d ]] && echo hit || echo miss`, "miss"},
		{`[[ ace == a*~*b*~*d ]] && echo hit || echo miss`, "hit"},
		// It binds tighter than `|`: the `zz` arm is not excluded.
		{`[[ zz == (a*~*b*|zz) ]] && echo hit || echo miss`, "hit"},
		{`[[ ac == (a*~*b*) ]] && echo hit || echo miss`, "hit"},
		// A tilde with nothing on one side is the character itself.
		{`[[ 'a~' == a~ ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == a~ ]] && echo hit || echo miss`, "miss"},
		// And one inside a bracket expression is a member of it rather than
		// an operator, which is what the scan has to step over.
		{`[[ '~' == [a~] ]] && echo hit || echo miss`, "hit"},
		{`[[ a == [a~] ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Every surface that matches a pattern, because a flag honored in one place
// and dropped in another is the same silent bug wearing a different hat.
func TestTheOperatorsReachEverySurface(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ABC", "abd", "aaa", "b1"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"case", `case ABC in (#i)abc) echo hit;; *) echo miss;; esac`, "hit"},
		{"case closure", `case aaa in a#) echo hit;; *) echo miss;; esac`, "hit"},
		{"trim prefix", `v=ABCd; echo "[${v#(#i)abc}]"`, "[d]"},
		{"trim suffix", `v=dABC; echo "[${v%(#i)abc}]"`, "[d]"},
		{"trim closure", `v=aaab; echo "[${v##a#}]"`, "[b]"},
		{"replace", `v=xABCy; echo "[${v/(#i)abc/Q}]"`, "[xQy]"},
		{"replace negation", `v=abc; echo "[${v/^b/Q}]"`, "[Q]"},
		{"exclusion in a trim", `v=abc; echo "[${v##a*~*c}]"`, "[c]"},
		{"glob, flag", `echo (#i)ab*`, "ABC abd"},
		{"glob, negation", `echo ^a*`, "ABC b1"},
		{"glob, closure", `echo a#`, "aaa"},
		// The same word quoted is not a pattern, which is the mark a field
		// carries through the walk.
		{"glob, quoted closure", `echo "a#"`, "a#"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, func(r *Runner) { r.Dir = dir })
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// The two position-aware flags that are assertions rather than options.
//
// `(#s)` asks that the match be standing at the start of the subject and
// `(#e)` at its end, so the matcher has to carry where it is — which is the
// mechanism the whole family is waiting on, and which nothing else in a
// pattern needs.
//
// docs/spec/grammar/patterns.md, "Where a match is standing".
func TestTheAnchorsAskWhereTheMatchIsStanding(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The trivial reading, which is also a control: an anchor at the
		// edge it names holds.
		{`[[ ab == (#s)ab ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == ab(#e) ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == (#s)ab(#e) ]] && echo hit || echo miss`, "hit"},
		{`[[ '' == (#s)(#e) ]] && echo hit || echo miss`, "hit"},
		// Anywhere else it simply does not hold. It is a match that cannot
		// happen and not a pattern that cannot be read, which is what keeps
		// the status 1 rather than the 2 a rejected pattern earns.
		{`[[ ab == a(#s)b ]] && echo hit || echo miss`, "miss"},
		{`[[ ab == a(#e)b ]] && echo hit || echo miss`, "miss"},
		{`[[ ab == (#s)(#e)ab ]] && echo hit || echo miss`, "miss"},
		// Which is what lets the other arm of an alternation carry the
		// match. Both of these are the assertion failing and something else
		// succeeding, and an implementation that made a misplaced anchor an
		// error would fail them.
		{`[[ ab == (a|(#s))b ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == ((#s)a|b)b ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == (a(#e)|a)b ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == ((#s)|x)ab ]] && echo hit || echo miss`, "hit"},
		// A `*` that has consumed nothing is still at the start, and one
		// that has consumed everything is at the end.
		{`[[ ab == *(#s)ab ]] && echo hit || echo miss`, "hit"},
		{`[[ ab == ab(#e)* ]] && echo hit || echo miss`, "hit"},
		{`[[ aab == a*(#s)ab ]] && echo hit || echo miss`, "miss"},
		// Through a closure, which re-enters the matcher at a position of
		// its own.
		{`[[ aaa == a#(#e) ]] && echo hit || echo miss`, "hit"},
		{`[[ aaab == a#(#e) ]] && echo hit || echo miss`, "miss"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Neither anchor may share its flag group, which is the shell's own rule and
// not a convenience: the mixed spellings are rejected patterns rather than
// unimplemented ones, so they take the surface's bad-pattern status and not
// the refusal's.
func TestAnAnchorStandsAloneInItsGroup(t *testing.T) {
	for _, src := range []string{
		`[[ AB == (#is)ab ]]`,
		`[[ AB == (#si)ab ]]`,
		`[[ ab == (#se)ab ]]`,
		`[[ ab == (#ss)ab ]]`,
	} {
		out, st := runExtendedOperators(t, src, true, nil)
		if !strings.Contains(out, "bad pattern") {
			t.Errorf("%s = %q, want the pattern rejected", src, out)
		}
		if strings.Contains(out, "not implemented") {
			t.Errorf("%s called a rejected pattern unimplemented: %q", src, out)
		}
		if st == 0 {
			t.Errorf("%s exited 0, want the script abandoned", src)
		}
	}
	// Written as two groups the same letters are fine, which is what says
	// the rule is about the group and not about the letters.
	out, _ := runExtendedOperators(t, `[[ AB == (#i)(#s)ab ]] && echo hit || echo miss`, true, nil)
	if got := strings.TrimSpace(out); got != "hit" {
		t.Errorf("`(#i)(#s)ab` = %q, want hit", got)
	}
}

// An anchor asks about the *subject*, not about the piece a surface handed the
// matcher — which is the whole reason the position is threaded rather than
// derived from the string in hand. A trim tries the prefixes of its value one
// at a time, and each of those is at the start of the subject and reaches its
// end only when it is the whole of it.
func TestAnAnchorNamesTheSubjectAndNotThePiece(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abcd; echo "[${v#(#s)ab}]"`, "[cd]"},
		{`v=abcd; echo "[${v#ab(#e)}]"`, "[abcd]"},
		{`v=abcd; echo "[${v#abcd(#e)}]"`, "[]"},
		{`v=abcd; echo "[${v%cd(#e)}]"`, "[ab]"},
		{`v=abcd; echo "[${v%(#s)cd}]"`, "[abcd]"},
		{`v=abcd; echo "[${v%(#s)abcd}]"`, "[]"},
		{`v=abcd; echo "[${v##*(#e)}]"`, "[]"},
		// A replacement scans every span of the value, so the anchors are
		// what keep it to one end. Without a position these would replace
		// every X rather than the one that is there.
		{`v=XbXcX; echo "[${v//(#s)X/-}]"`, "[-bXcX]"},
		{`v=XbXcX; echo "[${v//X(#e)/-}]"`, "[XbXc-]"},
		{`v=XbXcX; echo "[${v//X(#s)/-}]"`, "[XbXcX]"},
		{`v=aXbXc; echo "[${v//(#s)X/-}]"`, "[aXbXc]"},
		// An empty match is a position, and only one position is the start.
		{`v=abc; echo "[${v//(#s)/-}]"`, "[-abc]"},
		{`v=abc; echo "[${v//(#e)/-}]"`, "[abc-]"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The flags that *report* a position rather than only asking about one, and
// the parameters they report it into.
//
// docs/spec/grammar/patterns.md, "Reporting where a match landed".
func TestTheReportingFlagsFillTheirParameters(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The shape and the convention: the arrays run in group order, and
		// the bounds are one-based with the end being the index of the
		// *last* character.
		{`[[ abc == (#b)(a)(b)c ]]; echo "[${match[1]}${match[2]}][${mbegin[1]}${mbegin[2]}][${mend[1]}${mend[2]}]"`, "[ab][12][12]"},
		{`[[ abc == (#b)a(b*) ]]; echo "[${match[1]}][${mbegin[1]}][${mend[1]}]"`, "[bc][2][3]"},
		// An empty group has an end one below its begin, which is what the
		// inclusive convention says and what a script can rely on.
		{`[[ ac == (#b)(a)(b|)c ]]; echo "[${match[2]}][${mbegin[2]}][${mend[2]}]"`, "[][2][1]"},
		// Numbering is by opening parenthesis, outermost first.
		{`[[ abc == (#b)((a)(b))c ]]; echo "[${match[1]}|${match[2]}|${match[3]}]"`, "[ab|a|b]"},
		// A group that never participated is empty with -1 for both bounds
		// — the row that says why a dropped flag was never acceptable, since
		// empty is a real answer here.
		{`[[ ac == (#b)(a)((b))#c ]]; echo "[${match[2]}][${mbegin[2]}][${mend[2]}]"`, "[][-1][-1]"},
		// Even down an arm the subject never took: the number is a fact
		// about the pattern text and not about the path.
		{`[[ abc == (#b)((x)|a(b)c) ]]; echo "[${match[1]}|${match[2]}|${match[3]}][${mbegin[2]}]"`, "[abc||b][-1]"},
		// A group under a closure reports its last repetition.
		{`[[ abab == (#b)(ab)# ]]; echo "[${match[1]}][${mbegin[1]}]"`, "[ab][3]"},
		// The scope of the two switches is the group they stand in.
		{`[[ abc == (#b)(a)(#B)(b)c ]]; echo "[${match[1]}][${match[2]}]"`, "[a][]"},
		{`[[ abc == (#b)(a)((#B)(b))(c) ]]; echo "[${match[2]}][${match[3]}]"`, "[b][c]"},
		// `(#m)` is its own parameter set and composes with `(#b)`.
		{`[[ abc == (#m)a* ]]; echo "[$MATCH][$MBEGIN][$MEND]"`, "[abc][1][3]"},
		{`[[ abc == (#m)(#b)(a)b* ]]; echo "[$MATCH][${match[1]}]"`, "[abc][a]"},
		{`[[ abc == (#m)(#M)a* ]]; echo "[$MATCH]"`, "[]"},
		{`[[ abc == (#bB)(a)bc ]]; echo "[${match[1]}]"`, "[]"},
	} {
		out, _ := runReportingFlags(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// runReportingFlags is runExtendedOperators with the one further axis these
// rows depend on answered: `$match` is an array, and which number its first
// element answers to is a dialect's choice rather than this file's. The flags
// report **one-based** positions, so a test reading `${match[1]}` has to say
// which base it is reading in or it would be asserting the axis by accident.
func runReportingFlags(t *testing.T, src string) (string, int) {
	t.Helper()
	return runExtendedOperators(t, src, true, func(r *Runner) {
		r.Semantics.ArrayBaseIsZero = No
	})
}

// Nothing is written where the pattern asked for nothing, or where it asked
// and did not match.
//
// This is the other half of the module rule the refusal was protecting: a
// publish that always ran would *empty* the parameters a script had filled
// itself, which is a wrong answer in the opposite direction.
func TestAMatchThatAsksForNothingWritesNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`match=(zz); [[ abc == (#b)abc ]]; echo "[${match[1]}]"`, "[zz]"},
		{`match=(zz); [[ abc == (#b)(x)zz ]]; echo "[${match[1]}]"`, "[zz]"},
		{`match=(zz); [[ abc == (a)bc ]]; echo "[${match[1]}]"`, "[zz]"},
		{`MATCH=zz; [[ abc == (#m)xyz ]]; echo "[$MATCH]"`, "[zz]"},
		{`MATCH=zz; [[ abc == a* ]]; echo "[$MATCH]"`, "[zz]"},
		{`match=(zz); v=abc; echo "[${v//(#b)(q)/X}][${match[1]}]"`, "[abc][zz]"},
	} {
		out, _ := runReportingFlags(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Which combination of arm and length wins decides nothing about *whether* a
// pattern matches and everything about what it reports, so it has its own
// rows: a written arm beats a longer one, and within an arm the group takes
// as much as it can and still leave the rest a match.
func TestWhichSplitAGroupReports(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ abc == (#b)(a|ab)* ]]; echo "[${match[1]}]"`, "[a]"},
		{`[[ abc == (#b)(ab|a)* ]]; echo "[${match[1]}]"`, "[ab]"},
		{`[[ ab == (#b)(|a)(b|ab) ]]; echo "[${match[1]}|${match[2]}]"`, "[|ab]"},
		{`[[ ab == (#b)(a|)(b|ab) ]]; echo "[${match[1]}|${match[2]}]"`, "[a|b]"},
		{`[[ aabab == (#b)(a*)b ]]; echo "[${match[1]}]"`, "[aaba]"},
		{`[[ abcabc == (#b)(*)(abc) ]]; echo "[${match[1]}|${match[2]}]"`, "[abc|abc]"},
		{`[[ aaa == (#b)(a#)(a#)(a#) ]]; echo "[${match[1]}|${match[2]}|${match[3]}]"`, "[aaa||]"},
	} {
		out, _ := runReportingFlags(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A replacement reads the parameters *its own* match wrote, which means the
// replacement text is expanded once per match rather than once per
// expansion. A `with` joined before the scan cannot say either of these.
func TestAReplacementIsExpandedPerMatch(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abcd; echo "[${v//(#b)(b)(c)/<${match[1]}-${match[2]}>}]"`, "[a<b-c>d]"},
		{`v=abcb; echo "[${v//(#b)(b)/<${match[1]}${mbegin[1]}>}]"`, "[a<b2>c<b4>]"},
		{`v=abcd; echo "[${v//(#m)[bc]/<$MATCH:$MBEGIN:$MEND>}]"`, "[a<b:2:2><c:3:3>d]"},
		{`v=abcd; echo "[${v/(#b)(b*)/<${match[1]}>}]"`, "[a<bcd>]"},
		{`v=abc; echo "[${v/#(#b)(a)/<${match[1]}>}]"`, "[<a>bc]"},
		{`v=abc; echo "[${v/%(#b)(c)/<${match[1]}>}]"`, "[ab<c>]"},
		// And the parameters are left holding the *last* match.
		{`v=abcb; echo "[${v//(#b)(b)/Q}][${mbegin[1]}]"`, "[aQcQ][4]"},
	} {
		out, _ := runReportingFlags(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The positions a script reads are **characters**, which is a different
// number from the byte offset the matcher works in the moment a subject
// holds anything above ASCII.
func TestReportedPositionsAreCharacters(t *testing.T) {
	src := `v=aébc; echo "[${v//(#m)?/<$MATCH:$MBEGIN:$MEND>}]"`
	out, _ := runExtendedOperators(t, src, true, func(r *Runner) {
		r.Semantics.ArrayBaseIsZero = No
		r.Semantics.MultibyteEncodingIsHonored = Yes
		r.Env = append(r.Env, "LC_ALL=en_US.UTF-8")
	})
	want := "[<a:1:1><é:2:2><b:3:3><c:4:4>]"
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("%s = %q, want %q", src, got, want)
	}
}

// The load-bearing half. A flag this matcher does not implement says so by
// name and stops, because `(#b)` also fills `$match` and a dropped one would
// leave a script reading an empty value rather than seeing a refusal.
func TestAnUnimplementedFlagIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{`[[ abd == (#a1)abc ]]`, "(#a)"},
		{`[[ abc == (#u)abc ]]`, "(#u)"},
		{`[[ abc == (#U)abc ]]`, "(#U)"},
		// A flag reached through an arm of an alternation is reached.
		{`[[ abc == ((#u)abc|zz) ]]`, "(#u)"},
		// And through every other surface, not only a condition.
		{`case abc in (#u)abc) echo hit;; esac`, "(#u)"},
		{`v=abc; echo "${v#(#u)a}"`, "(#u)"},
		{`echo (#u)a*`, "(#u)"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, nil)
		want := "the " + tc.name + " pattern flag is not implemented"
		if !strings.Contains(out, want) {
			t.Errorf("%s = %q, want a refusal naming %s", tc.src, out, want)
		}
	}
	// It stops the script rather than reporting and carrying on, which is
	// the difference between a refusal and a complaint.
	out, st := runExtendedOperators(t, `[[ abc == (#u)abc ]]; echo AFTER`, true, nil)
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("a refused flag = %q status %d, want the script abandoned", out, st)
	}
	// And no arm runs, which is the half a status cannot show: a refusal
	// that let the match answer for itself would run one of these bodies.
	out, _ = runExtendedOperators(t, `case abc in (#u)abc) echo hit;; *) echo miss;; esac`, true, nil)
	if strings.Contains(out, "hit") || strings.Contains(out, "miss") {
		t.Errorf("a refused `case` pattern ran an arm: %q", out)
	}
	// With the option off the same text is a group and nothing is refused.
	out, _ = runExtendedOperators(t, `[[ '#uabc' == (#u)abc ]] && echo hit; echo AFTER`, false, nil)
	if !strings.Contains(out, "hit") || !strings.Contains(out, "AFTER") {
		t.Errorf("with the option off = %q, want hit and AFTER", out)
	}
}

// A letter no shell has is the shell's own complaint rather than ours, and
// the status it exits with belongs to the surface the pattern stood in.
func TestALetterNoShellHasIsABadPattern(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
	}{
		{`[[ abc == (#Z)abc ]]`, 2},
		{`case abc in (#Z)abc) echo hit;; esac`, 0},
		{`v=abc; echo "${v#(#Z)a}"`, 1},
		{`echo (#Z)a*`, 1},
		// A closure with no item in front of it is the same complaint.
		{`[[ abc == *# ]]`, 2},
		{`[[ abc == ab### ]]`, 2},
		{`[[ aaa == (#c3) ]]`, 2},
	} {
		out, st := runExtendedOperators(t, tc.src, true, nil)
		if !strings.Contains(out, "bad pattern") || st != tc.status {
			t.Errorf("%s = %q status %d, want a bad-pattern refusal at %d",
				tc.src, out, st, tc.status)
		}
	}
}

// A `#` at the front of a pattern has nothing to repeat, and is the same
// refusal. It can only be reached from a value, because a `#` where a word
// begins is a comment — so this asks the axis for a live expansion rather
// than writing the pattern down.
func TestAClosureAtTheFrontOfAPatternIsABadPattern(t *testing.T) {
	live := func(r *Runner) {
		s := *r.Semantics
		s.GlobExpansionResults = Yes
		r.Semantics = &s
	}
	out, st := runExtendedOperators(t, `p='#foo'; [[ '#foo' == $p ]]`, true, live)
	if !strings.Contains(out, "bad pattern") || st != 2 {
		t.Errorf("a live `#foo` = %q status %d, want a bad-pattern refusal at 2", out, st)
	}
	// And dead, the same four characters are text that matches itself.
	dead := func(r *Runner) {
		s := *r.Semantics
		s.GlobExpansionResults = No
		r.Semantics = &s
	}
	out, st = runExtendedOperators(t, `p='#foo'; [[ '#foo' == $p ]] && echo hit || echo miss`, true, dead)
	if strings.TrimSpace(out) != "hit" || st != 0 {
		t.Errorf("a dead `#foo` = %q status %d, want hit", out, st)
	}
}

// A metacharacter that arrived from a value is not one. The same answer the
// dialect already gives for `*`, asked of the three new characters.
func TestTheOperatorsAreNotReadInTheResultOfAnExpansion(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`p="a#b"; [[ ab == $p ]] && echo hit || echo miss`, "miss"},
		{`p="a#b"; [[ 'a#b' == $p ]] && echo hit || echo miss`, "hit"},
		{`p="^x"; [[ ab == $p ]] && echo hit || echo miss`, "miss"},
		{`p="^x"; [[ '^x' == $p ]] && echo hit || echo miss`, "hit"},
		{`p="a*~*b*"; [[ acd == $p ]] && echo hit || echo miss`, "miss"},
	} {
		out, _ := runExtendedOperators(t, tc.src, true, func(r *Runner) {
			// The axis that says an expansion's metacharacters are dead is
			// the one this rides on, and it is the same one `*` asks.
			s := *r.Semantics
			s.GlobExpansionResults = No
			r.Semantics = &s
		})
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
