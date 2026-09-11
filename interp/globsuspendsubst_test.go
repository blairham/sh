// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Suspending pathname expansion is a rule about a **word**, and a command
// substitution inside that word is a **program**.
//
// The two are easy to conflate because the suspension lives on the runner:
// `expandWordNoSplit` — an assignment's value, a `[[ ]]` operand, a
// redirection target — switches globbing off so that `n=*` stores the
// character, and until #1962 the commands run inside a substitution in the
// same word inherited that. So `v=$(echo *)` answered the pattern instead of
// the listing, at status 0 with nothing said.
//
// Measured 2026-09-11 in a directory holding `aa` and `bb`: `v=aa bb` in
// bash 5.3.15, dash, ksh93 and zsh 5.9.2 — a unanimous panel, so there is no
// axis here — where this shell left `v=*`.
//
// The tests name the containment rather than a shell, which is the rule for
// this package: what is being pinned is that the flag stops at the boundary
// of a program, the same containment `Runner.nestedLength` and
// `expandingQuoting` already carry for `${#…}` and for quoting.
func TestGlobbingSuspendedForAWordDoesNotReachTheProgramsInIt(t *testing.T) {
	dir := fileDir(t, "aa", "bb")
	inDir := func(r *Runner) { r.Dir = dir }
	for _, tc := range []struct{ name, src, want string }{
		{
			// The line the issue is written around.
			"an assignment's substitution globs",
			`v=$(echo *); echo "v=$v"`, "v=aa bb",
		},
		{
			// And the word's own asterisk still does not, which is the half a
			// fix that simply stopped suspending would have broken: `n=*` is
			// the character, in every shell in the panel.
			"the assignment's own pattern is still text",
			`n=*; echo "n=$n"`, "n=*",
		},
		{
			// Both in one word, so the restore is observed rather than
			// assumed: the flag has to come back on when the substitution
			// ends or the asterisks around it would list the directory too.
			"the suspension comes back when the program ends",
			`v=*$(echo *)*; echo "v=$v"`, "v=*aa bb*",
		},
		{
			// A body with more than an expansion in it, which is the shape
			// that makes it obvious these are ordinary commands.
			"a body of several commands globs throughout",
			`v=$( x=1; echo * ); echo "v=$v"`, "v=aa bb",
		},
		{
			// An arithmetic operand suspends it for the same reason an
			// assignment does, and leaked it the same way: this answered 1.
			"an arithmetic operand's substitution globs",
			`n=$(( $(echo * | wc -w) )); echo "n=$n"`, "n=2",
		},
		{
			// A redirection target is the third context, and the name it
			// writes to is the substitution's answer rather than the pattern.
			"a redirection target's substitution globs",
			`echo hi > $(echo aa); cat aa`, "hi",
		},
		{
			// Nesting restores rather than clears: the inner assignment
			// suspends again inside the outer program, and the substitution
			// inside *that* clears again.
			"an assignment inside the program suspends again on its own",
			`v=$(x=$(echo *); echo "[$x]"); echo "v=$v"`, "v=[aa bb]",
		},
		{
			// The backquoted spelling is the same construct and goes through
			// the same place; a fix applied to one spelling alone would leave
			// the other answering the pattern.
			"the backquoted spelling is the same program",
			"v=`echo *`; echo \"v=$v\"", "v=aa bb",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, inDir)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The other direction, and the reason the fix is at the program's boundary
// rather than at the places that suspend.
//
// A pattern operand does *not* suspend globbing — #1955 narrowed at the
// expansion for exactly this reason — so a substitution standing where a
// pattern goes already globs inside itself and matches, here and in the panel.
// Clearing the flag around the program has to leave that alone.
func TestASubstitutionStandingWhereAPatternGoesStillGlobsInside(t *testing.T) {
	dir := fileDir(t, "aa", "bb")
	inDir := func(r *Runner) { r.Dir = dir }
	for _, tc := range []struct{ name, src, want string }{
		{
			"the listing is the pattern the case matches against",
			`case "aa bb" in $(echo *)) echo match;; *) echo no;; esac`, "match",
		},
		{
			// And the same operand with `noglob` on, where the whole option
			// is off and nothing inside is expected to list.
			"with globbing off the pattern is the text",
			`set -o noglob; case '*' in $(echo *)) echo match;; *) echo no;; esac`, "match",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, inDir)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
