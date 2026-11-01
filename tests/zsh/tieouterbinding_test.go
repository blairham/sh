// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAGlobalUntieRemovesThePartnerBeneathALocal pins that `typeset -g`
// untying a script's tie from inside a call removes the *tied* partner — the
// binding a local of the call displaced — and leaves the local standing.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// from a script file under `env -i PATH=/usr/bin:/bin`), output compared byte
// for byte (#6469).
func TestAGlobalUntieRemovesThePartnerBeneathALocal(t *testing.T) {
	t.Parallel()
	const tie = "typeset -T FOO=a:b foo; "
	const show = `print -r -- "[${(t)foo}] [${(t)FOO}]"`
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar local", tie + "f() { local foo; typeset -g -a FOO; " + show + "; }; f; " + show, "[scalar-local] [array]\n[] [array]\n"},
		{"an array local", tie + "f() { local -a foo; typeset -g -a FOO; " + show + "; }; f; " + show, "[array-local] [array]\n[] [array]\n"},
		{
			"gone, not emptied", tie + "f() { local foo=(l); typeset -g -a FOO; print -r -- $foo; }; f; echo ${+foo}; typeset -p foo; echo st=$?",
			"l\n0\nzsh:typeset:1: no such variable: foo\nst=1\n",
		},
		{"the scalar half as the partner", tie + "f() { local FOO; typeset -g -i foo; }; f; " + show + "; echo ${+FOO}", "[integer] []\n0\n"},
		{"the export goes with it", tie + "export FOO; f() { local FOO; typeset -g -i foo; }; f; FOO=n; /usr/bin/env | grep -c '^FOO='", "0\n"},
		{"a caller's local", tie + "g() { local foo; f; print -r -- \"g [${(t)foo}]\"; }; f() { typeset -g -a FOO; }; g; " + show, "g [scalar-local]\n[] [array]\n"},
		{"a literal on the line", tie + "f() { local foo; typeset -g FOO=(x y); }; f; " + show + "; print -r -- $FOO", "[] [array]\nx y\n"},
		{"a frozen partner", tie + "typeset -r foo; f() { local foo; typeset -g -a FOO; echo in; }; f; echo after", "f: read-only variable: foo\n"},
		// The control: a kind the half already has keeps the tie, local or not.
		{"no kind change", tie + "f() { local foo; typeset -g FOO; }; f; " + show, "[array-tied] [scalar-tied]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestALocalTieOverAnOuterTieIsAFreshPair pins that a local tie inside a call
// is a new pair whatever its names were tied to outside the call: it starts
// empty, either order and a different partner are taken, and the outer pair is
// tied again on return. The last rows are the controls — a tie the call made
// itself, and a `-g` one, are the same pair tied again.
//
// Measured as above (#6470).
func TestALocalTieOverAnOuterTieIsAFreshPair(t *testing.T) {
	t.Parallel()
	const tie = "typeset -T FOO=a:b foo; "
	const after = `; print -r -- "[$FOO] [${(t)FOO}] [${(t)foo}]"; foo=(z w); print -r -- $FOO`
	const back = "[a:b] [scalar-tied] [array-tied]\nz:w\n"
	for _, tc := range []struct{ name, src, want string }{
		{"the same pair", tie + `f() { local -T FOO foo; echo "[$FOO] [$foo] [${#foo}]"; }; f` + after, "[] [] [0]\n" + back},
		{"by typeset", tie + `f() { typeset -T FOO foo; echo "[$FOO] [${#foo}] [${(t)FOO}]"; }; f` + after, "[] [0] [scalar-local-tied]\n" + back},
		{"listed", tie + "f() { local -T FOO foo; typeset -p FOO foo; }; f", "typeset -T FOO foo=(  )\ntypeset -aT FOO foo=(  )\n"},
		{"another separator", tie + `f() { local -T FOO foo ";"; foo=(p q); echo $FOO; }; f` + after, "p;q\n" + back},
		{"the other way round", tie + `f() { local -T foo FOO; FOO=(p q); echo "[$foo]"; }; f` + after, "[p:q]\n" + back},
		{"a new partner for the scalar", tie + `f() { local -T FOO bar; FOO=x:y; echo "[$bar] [${(t)foo}]"; }; f` + after, "[x y] [array-tied]\n" + back},
		{"a new partner for the array", tie + `f() { local -T BAR foo; BAR=x:y; echo "[$foo] [${(t)FOO}]"; }; f` + after, "[x y] [scalar-tied]\n" + back},
		{"a caller's local tie", tie + `g() { local -T FOO foo; FOO=g:h; f; echo "g [$FOO]"; }; f() { local -T FOO foo; echo "[$FOO] [${#foo}]"; }; g` + after, "[] [0]\ng [g:h]\n" + back},
		{"with a value", tie + `f() { local -T FOO=x:y foo; echo "[$FOO] [${#foo}]"; }; f` + after, "[x:y] [2]\n" + back},

		{"the call's own tie again", `f() { typeset -UT FOO=a:a foo; typeset -T FOO foo; typeset -p FOO; }; f; FOO=x:y; echo ${#foo}`, "typeset -UT FOO foo=( a )\n0\n"},
		{"a global tie again", tie + `f() { typeset -gT FOO foo; echo "[$FOO]"; }; f` + after, "[a:b]\n" + back},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
