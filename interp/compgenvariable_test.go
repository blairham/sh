// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `compgen -v` writes the names the shell holds a **value** for, and
// `-A variable` is the same action under its long name.
//
// What decides a row is a value and not a declaration, which is the whole of
// what separates this list from the one `declare -p` writes: a name brought
// into being by a declaration carrying nothing is a row there and is not a
// completion candidate here. The compound sibling of that is the same answer
// — an array a declaration made and nothing wrote to is out, and one an
// **empty literal** assigned is in, because something assigned it.
//
// See Runner.compgenVariableNames for the measurements (#4069).
func TestCompgenVariableListsNamesThatHoldAValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", "zzv=1\ncompgen -v zzv\n", "zzv\n"},
		{"an empty scalar counts", "zzv=\ncompgen -v zzv\n", "zzv\n"},
		{"an array with elements", "zzv=(a b)\ncompgen -v zzv\n", "zzv\n"},
		{"an array an empty literal assigned", "zzv=()\ncompgen -v zzv\n", "zzv\n"},
		{"nothing of that name at all", "compgen -v zzv\n", ""},
		{"the long spelling is the same action", "zzv=1\ncompgen -A variable zzv\n", "zzv\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := compgenRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A value a declaration **displaced** is still the shell's, so the name is
// still a candidate — and the placeholder an `unset` leaves on a local with
// nothing underneath it is not one. The declarations carry a value so that
// this suite is not stopped by the axis over a valueless one, which is not
// what it is about — the placeholder is the same either way.
//
// The two rows together are what #4069 read the other way round. Its own
// case has a global under the local, so the row it saw was the global's; the
// second row here is the control that separates them, and it is the reason
// this shell does not have to make the placeholder a candidate to answer the
// issue's snippet the way bash does.
func TestCompgenVariableSeesAValueADeclarationDisplaced(t *testing.T) {
	out, _ := compgenRun(t, "zzv=global\nf() { local zzv=L; unset zzv; compgen -v zzv; }\nf\n")
	if want := "zzv\n"; out != want {
		t.Errorf("a global under the placeholder = %q, want %q", out, want)
	}
	out, _ = compgenRun(t, "f() { local zznov=L; unset zznov; compgen -v zznov; }\nf\n")
	if out != "" {
		t.Errorf("a placeholder over nothing = %q, want no row", out)
	}
}

// The order is the shell's rather than the command line's, and the variables
// sit between the functions and the filenames — see compgenActionOrder.
func TestCompgenVariableIsGeneratedAfterTheFunctions(t *testing.T) {
	out, _ := compgenRun(t, "zzf() { :; }\nzzv=1\ncompgen -v -A function zz\n")
	if want := "zzf\nzzv\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	out, _ = compgenRun(t, "zzf() { :; }\nzzv=1\ncompgen -A function -v zz\n")
	if want := "zzf\nzzv\n"; out != want {
		t.Errorf("written the other way round = %q, want %q — the order is the shell's", out, want)
	}
}

// A name the shell was launched with and never assigned is a candidate too:
// it is in none of the tables the walk above reads and a child is told about
// it all the same.
func TestCompgenVariableListsAnInheritedName(t *testing.T) {
	out, _ := compgenRunEnv(t, []string{"ZZENV=1"}, "compgen -v ZZ\n")
	if want := "ZZENV\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// And one the script has taken away is gone from it, which is what says
	// the row is the name being *held* rather than the launch string.
	out, _ = compgenRunEnv(t, []string{"ZZENV=1"}, "unset ZZENV\ncompgen -v ZZ\n")
	if out != "" {
		t.Errorf("after unset = %q, want no row", out)
	}
}

// Nothing to offer is a failure rather than an empty success, which is the
// rule every action here already follows.
func TestCompgenVariableWithNoMatchFails(t *testing.T) {
	out, st := compgenRun(t, "compgen -v zznosuchname\n")
	if out != "" || st != 1 {
		t.Errorf("got %q status %d, want silence at 1", out, st)
	}
	if strings.Contains(out, "not implemented") {
		t.Errorf("got %q, want the letter generated rather than refused", out)
	}
}
