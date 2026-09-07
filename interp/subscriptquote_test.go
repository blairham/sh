// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether an array subscript is a quoting context.
//
// Two readings of the same characters. Where it is one, the key is the text
// *inside* the quotes and escapes, which is bash's and ksh93's answer; where
// it is not, the key is the subscript exactly as written — substitutions
// performed and every other character kept — which is zsh's. Measured
// 2026-09-07 against bash 5.3.15, that build invoked as `sh`, ksh93u+ and zsh
// 5.9.2; bash 3.2.57 has no associative arrays and dash has no arrays at all.
//
// Every row stores under one spelling and reads with the other, because a key
// that is the same string under both readings hides the question entirely —
// which is why this was silent: `m[user]=x` is `user` either way, and the
// wrong answer only appears once a quote is written.

// keyRun runs src with the axis answered and nothing else moved.
func keyRun(t *testing.T, src string, quoting interp.Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *interp.Semantics) {
		s.SubscriptIsAQuotingContext = quoting
	})
}

func TestASubscriptIsAQuotingContextOrIsTakenAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, removed, asWritten string }{
		{
			"a quoted key, written quoted and read bare",
			`typeset -A m; m["k"]=W; printf "[%s]" "${m[k]}"`,
			`[W]`, `[]`,
		},
		{
			"and read with the quotes it was written with",
			`typeset -A m; m["k"]=W; kk='"k"'; printf "[%s]" "${m[$kk]}"`,
			`[]`, `[W]`,
		},
		{
			"a bare key, read quoted",
			`typeset -A m; m[k]=P; printf "[%s]" "${m["k"]}"`,
			`[P]`, `[]`,
		},
		{
			// The crisp form: the substitution happens under both readings
			// and only the quote characters around it differ, so this is a
			// rule about quoting and not about expansion.
			"a substitution in the subscript, quoted and bare",
			`typeset -A q; q[k]=K; v=k; printf "[%s][%s]" "${q[$v]}" "${q["$v"]}"`,
			`[K][K]`, `[K][]`,
		},
		{
			// Written on both sides rather than substituted on one, which
			// keeps this row about the *source* text's quoting: a value's
			// own backslash is a separate question, and one this
			// implementation answers wrongly (#1222).
			"a backslash in the key",
			`typeset -A m; m[a\b]=B; printf "[%s][%s]" "${m[ab]}" "${m[a\b]}"`,
			`[B][B]`, `[][B]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := keyRun(t, tc.src, interp.Yes); out != tc.removed || st != 0 {
				t.Errorf("a quoting context: got %q status %d, want %q at 0", out, st, tc.removed)
			}
			if out, st := keyRun(t, tc.src, interp.No); out != tc.asWritten || st != 0 {
				t.Errorf("as written: got %q status %d, want %q at 0", out, st, tc.asWritten)
			}
		})
	}
}

// The axis is asked only where the two readings differ, so a key with no
// quote and no backslash in it — which is nearly every key a script writes —
// never demands a dialect for the question.
//
// Left unanswered on purpose: an unanswered axis that is asked says so and
// refuses, so the values coming back at status 0 are the whole assertion.
func TestAPlainKeyNeverAsksTheSubscriptQuotingAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare key", `typeset -A m; m[k]=P; printf "[%s]" "${m[k]}"`, `[P]`},
		{"a substituted key", `typeset -A m; v=k; m[$v]=P; printf "[%s]" "${m[$v]}"`, `[P]`},
		{"the whole array", `typeset -A m; m[a]=1; printf "%d" "${#m[@]}"`, `1`},
		{"a key that is not there", `typeset -A m; printf "[%s]" "${m[nope]:-gone}"`, `[gone]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := keyRun(t, tc.src, interp.Unspecified)
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("asked the axis where it changes nothing: %q", out)
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And it is asked where they differ, which is the other half of the claim.
func TestAnUnansweredSubscriptQuotingAxisIsRefusedByName(t *testing.T) {
	out, _ := keyRun(t, `typeset -A m; m[k]=P; printf "[%s]" "${m["k"]}"`, interp.Unspecified)
	if !strings.Contains(out, "an array subscript being a quoting context") {
		t.Errorf("got %q, want the axis refused by name", out)
	}
}

// The two things that stay unanimous and must not move with the axis.
//
// A *bare* `@` or `*` is the whole array under both readings; a quoted one is
// a key, so it looks one up and finds nothing. And no shell in the panel
// space-trims a key, which is the row this implementation used to answer
// wrongly: with `p[s]=T`, `${p[ s ]}` looks up three characters and is empty
// in bash, ksh93 and zsh alike.
func TestTheWholeArraySpellingAndTheBlanksAreNotTheAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare at is the whole array", `typeset -A n; n[a]=1; n[b]=2; printf "%d" "${#n[@]}"`, `2`},
		{"a bare star too", `typeset -A n; n[a]=1; n[b]=2; set -- "${n[*]}"; printf "%d" "$#"`, `1`},
		{"a quoted at is a key", `typeset -A n; n[a]=1; printf "[%s]" "${n["@"]}"`, `[]`},
		{"a key keeps its blanks", `typeset -A p; p[s]=T; printf "[%s][%s]" "${p[ s ]}" "${p[s]}"`, `[][T]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []interp.Answer{interp.Yes, interp.No} {
				out, st := keyRun(t, tc.src, a)
				if out != tc.want || st != 0 {
					t.Errorf("axis=%v: got %q status %d, want %q at 0", a, out, st, tc.want)
				}
			}
		})
	}
}

// The set of characters a backslash escapes inside a pattern.
//
// Empty means every character, which is bash's, bash 3.2's, bash as `sh`'s,
// dash's and ksh93's: a raw `bet\a` matches `beta` there. zsh names a set —
// its pattern metacharacters — and a backslash before anything outside it is
// a literal backslash with the character after it standing on its own.
//
// Measured by handing the matcher a raw backslash, which is the only way to
// ask: quote removal spends an escape written in the source before the matcher
// sees it, so a `case` pattern spelled `bet\a` is `beta` in all six and says
// nothing about this. A *substituted* pattern asks it in the five shells that
// match the result of an expansion, and `setopt globsubst` asks it in the one
// that does not.
func TestAPatternEscapeReachesEveryCharacterOrASet(t *testing.T) {
	// zsh's set, which is what makes the rows below the shell's answer rather
	// than a set invented here.
	const zshSet = `-=!*?[]()|^~#<>\`

	escRun := func(t *testing.T, src, set string) (string, int) {
		t.Helper()
		return axisRun(t, src, func(s *interp.Semantics) {
			s.GlobExpansionResults = interp.Yes
			s.PatternEscapeReaches = set
		})
	}
	for _, tc := range []struct{ name, src, every, someSet string }{
		{
			"before a character that needed no escaping",
			`p='bet\a'; case beta in $p) echo yes;; *) echo no;; esac`,
			"yes\n", "no\n",
		},
		{
			"and then the backslash is a character of the pattern",
			`p='bet\a'; case 'bet\a' in $p) echo yes;; *) echo no;; esac`,
			"no\n", "yes\n",
		},
		{
			"before a metacharacter it reaches either way",
			`p='be\*'; case 'be*' in $p) echo yes;; *) echo no;; esac`,
			"yes\n", "yes\n",
		},
		{
			"so the metacharacter is not live either way",
			`p='be\*'; case bex in $p) echo yes;; *) echo no;; esac`,
			"no\n", "no\n",
		},
		{
			// The backslash is in zsh's own set, which is what keeps a
			// glob-escaped value literal: a doubled one is one character.
			"before another backslash it reaches either way",
			`p='x\\y'; case 'x\y' in $p) echo yes;; *) echo no;; esac`,
			"yes\n", "yes\n",
		},
		{
			// Parameter expansion's matcher, which is a second construction
			// of the options and has to carry the same answer. The pattern
			// arrives raw here for the same reason it does in a `case`
			// subject above: the expansion's result is matched as a pattern,
			// which is the axis this row's setup answers yes.
			"and the trims ask it too, not only case and [[ ]]",
			`v=beta; p='bet\a'; printf "[%s]" "${v#$p}"`,
			"[]", "[beta]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := escRun(t, tc.src, ""); out != tc.every || st != 0 {
				t.Errorf("every character: got %q status %d, want %q at 0", out, st, tc.every)
			}
			if out, st := escRun(t, tc.src, zshSet); out != tc.someSet || st != 0 {
				t.Errorf("a set: got %q status %d, want %q at 0", out, st, tc.someSet)
			}
		})
	}
}
