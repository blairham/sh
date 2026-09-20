// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// An action type this shell cannot generate says so and then contributes
// nothing, leaving its siblings to answer. It used to end the call, which
// took the *implemented* action types down with it (#3899).
//
// Measured 2026-09-20 against bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C
// bash -c '<case>'`:
//
//	alias foo=ls; compgen -A alias                     foo                 0
//	compgen -A alias zzz                               nothing             1
//	alias foo=ls; compgen -A alias -A alias            foo, once           0
//	compgen -A builtin -A function -A alias -A keyword everything it has   0
//	compgen -b -v cd                                   cd                  0
//	compgen -bv cd                                     cd                  0
//
// So an action with no matches is silence at 1 there, which is what an action
// we cannot generate is here — the diagnostic aside, the control flow and the
// statuses are bash's. And the repeated `-A alias` answering one name is why
// the report is folded per spelling rather than written per occurrence.
func TestCompgenDegradesAnActionItCannotGenerate(t *testing.T) {
	t.Run("the siblings still answer", func(t *testing.T) {
		out, st := compgenRun(t, "f1() { :; }\ncompgen -A builtin -A function -A alias -A keyword\n")
		if !strings.Contains(out, "not implemented") {
			t.Errorf("said %q, want the gap named", out)
		}
		for _, want := range []string{"\ncd\n", "\nf1\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("said %q, want %q in it — the implemented actions must "+
					"still answer", out, want)
			}
		}
		if st != 0 {
			t.Errorf("status = %d, want 0 — words were generated (%q)", st, out)
		}
	})
	t.Run("alone, it is a listing with nothing in it", func(t *testing.T) {
		out, st := compgenRun(t, "compgen -A alias\n")
		if !strings.Contains(out, "not implemented") {
			t.Errorf("said %q, want the gap named", out)
		}
		if st != 1 {
			t.Errorf("status = %d, want 1 — nothing to offer, which is bash's "+
				"answer for an action with no matches (%q)", st, out)
		}
	})
	t.Run("said once however often it is asked", func(t *testing.T) {
		out, _ := compgenRun(t, "compgen -A alias -A alias\n")
		if got := strings.Count(out, "not implemented"); got != 1 {
			t.Errorf("said %q, want one report and not %d — bash folds a repeated "+
				"action too", out, got)
		}
	})
	t.Run("a letter is the same gap written short", func(t *testing.T) {
		out, st := compgenRun(t, "compgen -bv cd\n")
		if !strings.Contains(out, "not implemented") {
			t.Errorf("said %q, want the gap named", out)
		}
		if !strings.Contains(out, "\ncd\n") && !strings.HasPrefix(out, "cd\n") {
			t.Errorf("said %q, want `cd` in it — the letter beside it still generates", out)
		}
		if st != 0 {
			t.Errorf("status = %d, want 0 (%q)", st, out)
		}
	})
}

// The other half of the split, and the control: a name or a letter bash does
// not have either is the script's **typo**, and that still ends the call — in
// bash as here. Measured in the same run: `compgen -A nosuch -A builtin cd`
// and `compgen -A builtin -A nosuch cd` are both `invalid action name` at 2
// with no listing, and `compgen -z -b cd` is `invalid option` plus the usage
// line at 2 with no listing.
//
// This is what makes the rows above discriminating. A change that degraded
// *everything* would pass every one of them and fail every one of these.
func TestCompgenStillEndsTheCallForAThingBashDoesNotHaveEither(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a name that is no action, first", "compgen -A nosuch -A builtin cd\n", "invalid action name"},
		{"a name that is no action, second", "compgen -A builtin -A nosuch cd\n", "invalid action name"},
		{"a letter that is no letter", "compgen -z -b cd\n", "invalid option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := compgenRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("said %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "\ncd\n") || strings.HasPrefix(out, "cd\n") {
				t.Errorf("said %q, want no listing — the call ended", out)
			}
			if st != 2 {
				t.Errorf("status = %d, want 2 (%q)", st, out)
			}
		})
	}
}
