// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAScriptTieHalfKeepsItsKindAgainstAValue pins that each half of a
// script's tie holds its kind against a value, as the shell's own pairs do:
// an array written to the scalar half is refused on every road that writes a
// whole array, and a plain value over the array half becomes its one element.
// A declaration carrying the literal unties instead, and a local in front is
// an ordinary name — the controls, without which a shell refusing every array
// literal near a tie would pass.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// from a script file under `env -i PATH=/usr/bin:/bin`), output compared byte
// for byte (#6468).
func TestAScriptTieHalfKeepsItsKindAgainstAValue(t *testing.T) {
	t.Parallel()
	const tie = "typeset -T FOO=a:b foo; "
	const show = `; print -r -- "[${(t)foo}] [${(t)FOO}] [$FOO]"`
	const refused = "zsh:1: FOO: attempt to assign array value to non-array\n"
	for _, tc := range []struct{ name, src, want string }{
		{"a bare literal", tie + "FOO=(x); echo after", refused},
		{"an appended literal", tie + "FOO+=(x); echo after", refused},
		{"an empty literal", tie + "FOO=(); echo after", refused},
		{"set -A", tie + "set -A FOO x y; echo after", refused},
		{"set +A", tie + "set +A FOO x; echo after", refused},
		{"read -A", tie + "read -A FOO <<< 'x y'; echo after", refused},
		{"the (A) flag", tie + ": ${(A)FOO::=x y}; echo after", refused},
		{"in a function", tie + "f() { FOO=(x); }; f; echo after", "f: FOO: attempt to assign array value to non-array\n"},
		{"a call's own tie", "f() { typeset -T FOO=a:b foo; FOO=(x); }; f; echo after", "f: FOO: attempt to assign array value to non-array\n"},
		{"the partner shadowed", tie + "f() { local foo; FOO=(x); }; f; echo after", "f: FOO: attempt to assign array value to non-array\n"},
		{"a plain value over the array half", tie + "foo=plain" + show + "; echo ${#foo}", "[array-tied] [scalar-tied] [plain]\n1\n"},

		// The declarations, which untie rather than refuse.
		{"typeset with the literal", tie + "typeset FOO=(x)" + show, "[] [array] [x]\n"},
		{"export with the literal", tie + "export FOO=(x)" + show, "[] [array-export] [x]\n"},
		{"a call's own tie, by local", "f() { typeset -T FOO=a:b foo; local FOO=(x)" + show + "; }; f", "[] [array-local] [x]\n"},
		{"a frozen partner", tie + "typeset -r foo; typeset FOO=(x); echo after", "zsh:1: read-only variable: foo\n"},

		// The controls.
		{"an untied scalar converts", "x=s; x=(a); print -r -- ${(t)x}", "array\n"},
		{"a local in front", tie + `f() { local FOO; FOO=(x); print -r -- ${(t)FOO}; }; f` + show, "array-local\n[array-tied] [scalar-tied] [a:b]\n"},
		{"a fresh local with the literal", tie + `f() { local FOO=(x); print -r -- ${(t)FOO}; }; f` + show, "array-local\n[array-tied] [scalar-tied] [a:b]\n"},
		{"the array half takes an array", tie + "foo=(p q)" + show, "[array-tied] [scalar-tied] [p:q]\n"},
		{"the shell's own array half", "set -A path x y; print -r -- $PATH", "x:y\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestAFixedScalarSlotRefusesAnArrayOnEveryRoad is the same store refusal
// over the slots that had it on the bare literal only: one of the shell's own
// scalars and a private. Measured as above.
func TestAFixedScalarSlotRefusesAnArrayOnEveryRoad(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{"set -A HOME x y; echo after", "zsh:1: HOME: attempt to assign array value to non-array\n"},
		{"read -A HOME <<< x; echo after", "zsh:1: HOME: attempt to assign array value to non-array\n"},
		{": ${(A)PATH::=x y}; echo after", "zsh:1: PATH: attempt to assign array value to non-array\n"},
		{"zmodload zsh/param/private; f() { private v; set -A v x; }; f; echo after", "f: v: attempt to assign array value to non-array\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
