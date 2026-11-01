// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAKindLetterUntiesAScriptTie pins that a declaration asking one half of
// a script's tie to become a kind it is not takes the pair apart and removes
// the other half, and that a letter leaving the half's kind alone keeps the
// tie — the controls, without which a shell that untied on every letter would
// pass.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// from a script file under `env -i PATH=/usr/bin:/bin`), output compared byte
// for byte (#6387).
func TestAKindLetterUntiesAScriptTie(t *testing.T) {
	t.Parallel()
	const show = `; print -r -- "[${(t)foo}] [${(t)FOO}]"`
	for _, tc := range []struct{ name, src, want string }{
		{"array letter on the scalar half", "typeset -T FOO foo; typeset -a FOO" + show + "; typeset -p FOO", "[] [array]\ntypeset -a FOO=(  )\n"},
		{"integer letter on the scalar half", "typeset -T FOO foo; typeset -i FOO" + show, "[] [integer]\n"},
		{"with a value", "typeset -T FOO=a:b foo; typeset -i FOO=3" + show + `; print -r -- $FOO`, "[] [integer]\n3\n"},
		{"table letter on the scalar half", "typeset -T FOO=a:b foo; typeset -A FOO" + show, "[] [association]\n"},
		{"float letter on the array half", "typeset -T FOO foo; typeset -E foo" + show, "[float] []\n"},
		{"integer letter on the array half", "typeset -T FOO=3 foo; typeset -i foo; typeset -p foo; echo ${+FOO}", "typeset -i foo=0\n0\n"},
		{"plus taking the array half's kind away", "typeset -T FOO=a:b foo; typeset +a foo" + show, "[scalar] []\n"},
		{"the numeric words", "typeset -T FOO=a:b foo; integer FOO=4" + show, "[] [integer]\n"},
		{"global from a function", "typeset -T FOO=a:b foo; f() { typeset -g -i FOO=2; }; f" + show, "[] [integer]\n"},
		{
			"the partner is gone, not emptied", "typeset -T FOO=a:b foo; typeset -a FOO; echo ${+foo}; foo=(x); typeset -p foo FOO",
			"0\ntypeset -a foo=( x )\ntypeset -a FOO=(  )\n",
		},

		{"a call's own tie, by typeset", "f() { typeset -T FOO=a:b foo; typeset -A foo" + show + "; }; f", "[association-local] []\n"},
		{"a call's own tie, by local", "f() { typeset -T FOO foo; local -i FOO" + show + "; }; f", "[] [integer-local]\n"},

		// The controls: no kind changes, so the tie stays.
		{"plus of a kind the array half has not", "typeset -T FOO=a:b foo; typeset +i foo; typeset +A foo" + show, "[array-tied] [scalar-tied]\n"},
		{"array letter on the array half", "typeset -T FOO=a:b foo; typeset -a foo" + show, "[array-tied] [scalar-tied]\n"},
		{"plus of a kind the half has not", "typeset -T FOO=a:b foo; typeset +i FOO" + show, "[array-tied] [scalar-tied]\n"},
		{"a letter that is not a kind", "typeset -T FOO=a:b foo; typeset -x FOO; typeset -U foo" + show, "[array-tied-unique] [scalar-tied-export]\n"},
		{"a local stands in front", "typeset -T FOO=a:b foo; f() { local -i FOO" + show + "; }; f" + show, "[array-tied] [integer-local]\n[array-tied] [scalar-tied]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestAReadonlyPartnerRefusesTheUntie is the partner's own refusal, which
// comes before the converted half is reached: `a:b` is never read as
// arithmetic, so the sentence names `foo` rather than the math, and the
// script ends there. Measured as above.
func TestAReadonlyPartnerRefusesTheUntie(t *testing.T) {
	t.Parallel()
	got, _ := runZshOnPath(t, t.TempDir(), "typeset -T FOO=a:b foo; typeset -r foo; typeset -i FOO; echo after")
	if want := "zsh:1: read-only variable: foo\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
