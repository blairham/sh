// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The `-h` letter written on a `typeset -T` line reaches **both** halves, and
// a tie declared over an already-hidden name does not keep the hide — see
// Runner.declareTie and Runner.dropHideInScope (#4876).
//
// The two rows fail in opposite directions, which is why they are one test:
// carrying the letter through without also dropping the one a name already
// carried would leave the second row exactly as wrong as it was.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME.
func TestTheHideLetterOnATieLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the letter on the tie's own line reaches both halves",
			`typeset -hT TT tt; print "${(t)TT}|${(t)tt}"`,
			"scalar-tied-hide|array-tied-hide\n",
		},
		{
			"a tie over an already-hidden name drops the hide",
			`typeset -h TT; typeset -T TT tt; print "${(t)TT}|${(t)tt}"`,
			"scalar-tied|array-tied\n",
		},
		// The controls. The first says the letter is not simply missing from
		// a tied name, and the next two say a tie line does carry other
		// letters to both halves — so neither row above is "a tie takes no
		// attributes".
		{
			"the letter after the tie reaches the named half alone",
			`typeset -T TT tt; typeset -h TT; print "${(t)TT}|${(t)tt}"`,
			"scalar-tied-hide|array-tied\n",
		},
		{
			"the hide-value letter on a tie line reaches both",
			`typeset -HT TT tt; print "${(t)TT}|${(t)tt}"`,
			"scalar-tied-hideval|array-tied-hideval\n",
		},
		{
			"the unique letter on a tie line reaches both",
			`typeset -UT TT tt; print "${(t)TT}|${(t)tt}"`,
			"scalar-tied-unique|array-tied-unique\n",
		},
		{
			"the letter on an untied name is untouched",
			`typeset -h v=1; print "${(t)v}"`,
			"scalar-hide\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A local of one half of one of the shell's **own** ties displaces the other
// half with it — that is what keeps the pair a pair — and the half nobody
// named does not describe as `local` (#4875).
//
// The fact was already written down in interp/tielocal.go's own table, which
// records `${(t)path}` as `array-tied-special` inside `f(){ local PATH=/x }`.
// Nothing had asked the word about it, which is why the two disagreed for as
// long as they did.
func TestThePartnerOfABuiltInTieIsNotLocal(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"from the scalar end",
			`f(){ typeset PATH; print "${(t)path}" }; f`,
			"array-tied-special\n",
		},
		{
			"from the array end",
			`f(){ typeset path; print "${(t)PATH}" }; f`,
			"scalar-tied-export-special\n",
		},
		// The controls. The half a declaration *names* is local in both
		// shells, and so is one an enclosing call named — so this is not
		// "a tied name is never local".
		{
			"the half the declaration named",
			`f(){ typeset PATH; print "${(t)PATH}" }; f`,
			"scalar-local-tied-special\n",
		},
		{
			"a caller's real local reaches the callee",
			`g(){ typeset PATH; print "${(t)path}" }
f(){ typeset path; g }
f`,
			"array-local-tied-special\n",
		},
		{
			"a caller's mirror does not",
			`g(){ print "${(t)path}" }
f(){ typeset PATH; g }
f`,
			"array-tied-special\n",
		},
		{
			"the partner declared in its own right, after the mirror",
			`f(){ typeset PATH; typeset path; print "${(t)path}" }; f`,
			"array-local-tied-special\n",
		},
		{
			"and nothing standing at all",
			`f(){ print "${(t)path}" }; f`,
			"array-tied-special\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != 0 {
				t.Errorf("%s = %q status %d, want it to end %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The values the mirror is *for* are untouched by the word above: a local of
// the scalar half still drives the array half inside the call, writing the
// array half still drives the scalar, and the caller gets both back.
//
// Its own test because a fix keyed on "the partner is not this call's" could
// have been written by not shadowing the partner at all, and that would pass
// every row of the test above and break every row of this one.
func TestTheMirrorStillCarriesTheValues(t *testing.T) {
	out, st := answersRun(t, `f(){ typeset PATH=/x; print "in=${#path} ${path[1]}" }
f
print "out=${path[1]} n=${#path}"`)
	want := "in=1 /x\nout=/usr/bin n=2\n"
	if out != want || st != 0 {
		t.Errorf("a local of the scalar half = %q status %d, want %q", out, st, want)
	}
	out, st = answersRun(t, `f(){ typeset PATH=/x; path=(/q /r); print "in=$PATH" }
f
print "out=$PATH"`)
	want = "in=/q:/r\nout=/usr/bin:/bin\n"
	if out != want || st != 0 {
		t.Errorf("writing the mirrored half = %q status %d, want %q", out, st, want)
	}
}

// A frozen tie refuses an array literal on its array half, because the value
// a tie declaration carries is the **scalar's** — see
// Runner.freezeWithoutDeferring (#4874).
func TestAFrozenTieRefusesALiteralOnItsArrayHalf(t *testing.T) {
	out, st := answersRun(t, `typeset -rT TT tt=(a b); print "reached ${#tt}"`)
	if !strings.Contains(out, "read-only variable: tt") {
		t.Errorf("a frozen tie with an array literal = %q, want the freeze to refuse it", out)
	}
	if strings.Contains(out, "reached") || st == 0 {
		t.Errorf("a frozen tie with an array literal = %q status %d, want the script ended",
			out, st)
	}
}

// And the two controls that keep it from being "a frozen tie refuses a
// value": the scalar half's value is this declaration's own and is taken
// through the freeze, and the same literal with no `-r` is stored.
func TestAFrozenTieTakesTheScalarHalfsValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the scalar half under a freeze", `typeset -rT TT=x:y tt; print "${#tt} [$TT] ${(t)tt}"`, "2 [x:y] array-readonly-tied\n"},
		{"the array half with no freeze", `typeset -T TT tt=(a b); print "${#tt} [$TT] ${(t)tt}"`, "2 [a:b] array-tied\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A declaration of the half the mirror displaced is that name's first, not a
// redeclaration, so it writes nothing back — see Runner.shadow, where the two
// answers `fresh` used to carry part company (#4890).
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME.
func TestADeclarationOverATieMirrorIsNotARedeclaration(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the array half after the scalar was declared",
			`f(){ typeset PATH; typeset path; print "${(t)path}" }; f`,
			"array-local-tied-special\n",
		},
		{
			"the scalar half after the array was declared",
			`f(){ typeset path; typeset PATH; print "${(t)path}" }; f`,
			"array-local-tied-special\n",
		},
		// The controls, and they are the whole of what keeps the listing rule
		// itself intact: a genuine second declaration still writes the name
		// back, on a tied name and an ordinary one alike, and so does one at
		// the top level where there is no binding to be making.
		{
			"a real redeclaration of the tied name still lists",
			`f(){ typeset path; typeset path; print "${(t)path}" }; f`,
			"path=(  )\narray-local-tied-special\n",
		},
		{
			"a real redeclaration of an ordinary name still lists",
			`f(){ typeset v; typeset v; print "${(t)v}" }; f`,
			"v=''\nscalar-local\n",
		},
		{
			"and of the scalar half",
			`f(){ typeset PATH; typeset PATH; print "${(t)PATH}" }; f`,
			"PATH=''\nscalar-local-tied-special\n",
		},
		{
			"the top level lists, having no binding to make",
			`typeset PATH; typeset path; print "${(t)path}"`,
			"PATH=/usr/bin:/bin\npath=( /usr/bin /bin )\narray-tied-special\n",
		},
		{
			"one declaration on its own writes nothing",
			`f(){ typeset path; print "${(t)path}" }; f`,
			"array-local-tied-special\n",
		},
		// And the exemption is spent by the declaration that uses it: the
		// mark the mirror left is consumed, so a *third* line naming either
		// half is a redeclaration again and lists once — not twice, which is
		// what a shell that never cleared the mark would do, and not never,
		// which is what one that read the mark for every later line would.
		{
			"a second declaration of the displaced half lists again",
			`f(){ typeset PATH; typeset path; typeset path; print "${(t)path}" }; f`,
			"path=( '' )\narray-local-tied-special\n",
		},
		{
			"and the partner, whose own mark the first line spent",
			`f(){ typeset PATH; typeset path; typeset PATH; print "${(t)PATH}" }; f`,
			"PATH=''\nscalar-local-tied-special\n",
		},
		// A second of the shell's own pairs, which says the rule is about the
		// mirror and not about `path` in particular.
		{
			"the FPATH pair writes nothing either",
			`f(){ typeset FPATH; typeset fpath; print "${(t)fpath}" }; f`,
			"array-local-tied-special\n",
		},
		// A tie the *script* made is not one the shell mirrors, so nothing
		// here is exempt and nothing here changes: the control that says the
		// exemption did not widen to every tie.
		{
			"a script's own tie is untouched by any of this",
			`typeset -T TT tt; f(){ typeset TT; typeset tt; print "${(t)tt}" }; f`,
			"scalar-local\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the value the mirror left is still standing, which is what says this is
// a *listing* fix and not the cell being made fresh. A declaration over a
// mirror keeps what the partner put there; a first declaration of the name
// with no mirror in front of it does not.
func TestADeclarationOverATieMirrorKeepsTheValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"over the mirror", `f(){ typeset PATH=/x; typeset path; print "n=$#path [$path]" }; f`, "n=1 [/x]\n"},
		{"with no mirror", `f(){ typeset path; print "n=$#path [$path]" }; f`, "n=0 []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `local` at the top level reaches the shadow that makes no binding at all,
// which is the second place the redeclaration answer cannot be read off
// `fresh` — there is no scope, so nothing is fresh and everything is standing
// already.
//
// Its own test because `local` is the one word that gets there: every other
// caller of the shadow with no scope carries an attribute letter, so it never
// asks the listing question. Without these rows the branch was a reading
// nothing could contradict.
func TestLocalAtTheTopLevelListsAHeldName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a second local of the same name", `local v; local v; print "${(t)v}"`, "v=''\nscalar\n"},
		{"a local over a name already holding", `v=hi; local v; print "${(t)v}"`, "v=hi\nscalar\n"},
		{
			"and the mirror pair, which has no binding to be making either",
			`local PATH; local path; print "${(t)path}"`,
			"PATH=/usr/bin:/bin\npath=( /usr/bin /bin )\narray-tied-special\n",
		},
		{
			"while inside a function the same pair writes nothing",
			`f(){ local PATH; local path; print "${(t)path}" }; f`,
			"array-local-tied-special\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
