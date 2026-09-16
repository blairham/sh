// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A function's own attributes — #3192.
//
// Here rather than in the substrate because the whole notion is this
// dialect's: `readonly -f` is an option ksh93, dash and BusyBox ash end the
// script over, and zsh reads the `-F` half as a float's precision. The core
// keeps the two tables and the listing shape; what the letters are, and that
// there are any, is Semantics.FunctionAttributeLetters.
//
// Every want below is bytes from bash 5.3.20, measured 2026-09-16 and agreed
// by 3.2.57.

// The letters print, and a listing of the whole table carries them.
func TestAFunctionsAttributesReachTheListing(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, "a(){ :; }\nb(){ :; }\nc(){ :; }\nd(){ :; }\n"+
		"readonly -f b\nexport -f c\nreadonly -f d; export -f d\ndeclare -F\n")
	want := "declare -f a\ndeclare -fr b\ndeclare -fx c\ndeclare -frx d\n"
	if out != want || st != 0 {
		t.Errorf("declare -F = %q (status %d), want %q", out, st, want)
	}
}

// With no operand the letters filter, and they are a union rather than an
// intersection — which is the half a probe with one attributed function could
// not tell apart.
func TestTheAttributeLettersFilterAsAUnion(t *testing.T) {
	setup := "a(){ :; }\nb(){ :; }\nc(){ :; }\nd(){ :; }\n" +
		"readonly -f b\nexport -f c\nreadonly -f d; export -f d\n"
	for _, tc := range []struct{ line, want string }{
		{"declare -Fr", "declare -fr b\ndeclare -frx d\n"},
		{"declare -Fx", "declare -fx c\ndeclare -frx d\n"},
		// All three, and not the one that holds both.
		{"declare -Frx", "declare -fr b\ndeclare -fx c\ndeclare -frx d\n"},
		// And the whole table when no letter narrows it, so the rows above
		// are a narrowing rather than a listing that happens to be short.
		{"declare -F", "declare -f a\ndeclare -fr b\ndeclare -fx c\ndeclare -frx d\n"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), setup+tc.line+"\n")
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.line, out, st, tc.want)
			}
		})
	}
}

// A body listing of the whole table writes the attribute line under a body
// that has one; a name asked for by operand gets the body alone.
func TestABodyListingCarriesTheAttributeLine(t *testing.T) {
	setup := "b(){ :; }\nreadonly -f b\n"
	whole, st := runBash(t, t.TempDir(), setup+"declare -f\n")
	if st != 0 || !strings.HasSuffix(whole, "declare -fr b\n") {
		t.Errorf("declare -f = %q (status %d), want it to end with the attribute line", whole, st)
	}
	named, st := runBash(t, t.TempDir(), setup+"declare -f b\n")
	if st != 0 || strings.Contains(named, "declare -fr") {
		t.Errorf("declare -f b = %q (status %d), want the body alone", named, st)
	}
	if named == whole {
		// The two shapes have to differ, or one of the assertions above is
		// passing on the other's output.
		t.Errorf("the named and whole-table listings are the same text: %q", named)
	}
}

// With an operand the same letters set rather than filter, in either order
// and under either word.
func TestTheAttributeLettersSetWithAnOperand(t *testing.T) {
	for _, line := range []string{
		"declare -fr e", "declare -rf e", "typeset -fr e", "declare -Fr e", "readonly -f e",
	} {
		t.Run(line, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), "e(){ :; }\n"+line+"\ndeclare -F\n")
			want := "declare -fr e\n"
			if out != want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", line, out, st, want)
			}
		})
	}
}

// And the freeze refuses, in both directions, at 1 and not fatally.
func TestAFrozenFunctionIsNeitherRedefinedNorUnset(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"b(){ echo orig; }\nreadonly -f b\nb(){ echo new; }\necho st=$?\nunset -f b\necho st=$?\nb\n")
	for _, want := range []string{
		"b: readonly function\n",
		"unset: b: cannot unset: readonly function\n",
		// Both refusals at 1, and the body untouched: a refusal that had
		// bound the new definition would print `new`.
		"st=1\n",
		"orig\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("said %q, want %q in it", out, want)
		}
	}
	if strings.Contains(out, "new") {
		t.Errorf("said %q, want the refused definition never to have bound", out)
	}
	if strings.Count(out, "st=1\n") != 2 {
		t.Errorf("said %q, want both refusals at 1", out)
	}
	if st != 0 {
		t.Errorf("status %d, want the script to carry on", st)
	}
}

// `readonly -f` refuses a name that is not a function, and refuses it in
// `readonly`'s words rather than `export`'s.
func TestFreezingSomethingThatIsNotAFunction(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "readonly -f nosuchfn_zz\necho st=$?\n")
	want := "readonly: nosuchfn_zz: not a function\n"
	if !strings.Contains(out, want) || !strings.Contains(out, "st=1\n") || st != 0 {
		t.Errorf("said %q (status %d), want %q and st=1", out, st, want)
	}
}

// `readonly -f` and `export -f` with no name are the listings `declare -fr`
// and `declare -fx` write, byte for byte.
func TestTheBareListingsAgreeWithTheDeclarationForm(t *testing.T) {
	setup := "a(){ :; }\nb(){ :; }\nreadonly -f a\nexport -f b\n"
	for _, tc := range []struct{ bare, declaration string }{
		{"readonly -f", "declare -fr"},
		{"readonly -pf", "declare -fr"},
		{"export -f", "declare -fx"},
	} {
		t.Run(tc.bare, func(t *testing.T) {
			got, st := runBash(t, t.TempDir(), setup+tc.bare+"\n")
			want, wst := runBash(t, t.TempDir(), setup+tc.declaration+"\n")
			if got != want || st != wst {
				t.Errorf("%s = %q (status %d), want %s's %q (status %d)",
					tc.bare, got, st, tc.declaration, want, wst)
			}
			if !strings.Contains(want, "declare -f") {
				t.Fatalf("%s wrote %q, which carries no attribute line to compare", tc.declaration, want)
			}
		})
	}
}
