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
