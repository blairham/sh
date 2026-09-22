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

// The `-p` word puts the attribute line back on a listing a name asked for,
// which is the one form that writes a body and an attribute line together —
// #4190.
//
// `declare -fp f` is the shape a state capture reads back, so a body alone
// there is a listing saying a frozen function is an ordinary one, silently and
// at status 0. Every want below is bytes from bash 5.3.20 under LC_ALL=C,
// measured 2026-09-22 from a script file.
func TestThePrintWordCarriesTheAttributeLineOnANamedListing(t *testing.T) {
	// One of each letter and one holding none, so a row that wrote the line
	// for every name would fail on `a` and a row that wrote none would fail
	// on the other three.
	setup := "a(){ :; }\nb(){ :; }\nc(){ :; }\nd(){ :; }\n" +
		"readonly -f b\ndeclare -ft c\nexport -f d\n"
	body := func(n string) string { return n + " () \n{ \n    :\n}\n" }
	for _, tc := range []struct{ line, want string }{
		// The body and then the line, in that order.
		{"declare -fp b", body("b") + "declare -fr b\n"},
		{"declare -fp c", body("c") + "declare -ft c\n"},
		{"declare -fp d", body("d") + "declare -fx d\n"},
		// A function holding none gets no line at all, where `-Fp` writes
		// `declare -f a` for the same name — so the line is the function's
		// attributes rather than the form's decoration.
		{"declare -fp a", body("a")},
		{"declare -Fp a", "declare -f a\n"},
		// The letters come off in the field's order whatever the command
		// line said, and a name may hold more than one.
		{"readonly -f c\ndeclare -fp c", body("c") + "declare -frt c\n"},
		// Two names, each followed by its own line rather than both lines
		// collected at the end.
		{"declare -fp b c", body("b") + "declare -fr b\n" + body("c") + "declare -ft c\n"},
		// The word may be spelled first, and `typeset` is the same builtin.
		{"declare -pf b", body("b") + "declare -fr b\n"},
		{"typeset -fp b", body("b") + "declare -fr b\n"},
		// Without it the body stands alone, which is the contrast the fix
		// had to keep: only the `-p` form changed.
		{"declare -f b", body("b")},
	} {
		t.Run(tc.line, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), setup+tc.line+"\n")
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.line, out, st, tc.want)
			}
		})
	}
}

// A name that is not there is still reported, and still leaves 1 behind, with
// the attribute line of the name that was there written before it.
func TestThePrintWordOnAMissingNameBesideAnAttributedOne(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "b(){ :; }\nreadonly -f b\ndeclare -fp b nosuch\n")
	want := "b () \n{ \n    :\n}\ndeclare -fr b\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("declare -fp b nosuch = %q, want it to start with %q", out, want)
	}
	if !strings.Contains(out, "nosuch: not found") {
		t.Errorf("declare -fp b nosuch = %q, want the missing name reported", out)
	}
	if st != 1 {
		t.Errorf("declare -fp b nosuch left %d behind, want 1", st)
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
