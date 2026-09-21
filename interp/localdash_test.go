// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `local -` saves the shell's `set` table for the life of the call.
//
// Measured 2026-09-15, `env -i PATH=/usr/bin:/bin`, over `-c` and a script
// file alike, with `f() { local -; set -f; }; f; echo $-`:
//
//	bash 5.3.15   the `f` is gone     restored
//	dash          the `f` is gone     restored, and `local` takes no letters
//	                                  there at all, so this is not one of them
//	BusyBox ash   the `f` is gone     restored
//	bash 3.2.57   refused as a bad name — the form arrived between the two
//	zsh 5.9.2     `-` is a parameter there, so `local -` declares it
//	ksh93         no `local` builtin, so the form cannot be written
//
// Three of the four that have `local` agree and the fourth reads the same two
// characters as something else, which is why this is an axis rather than a
// fix. See Semantics.LocalDashSavesTheShellOptions.

func savesOptions(yes Answer) func(*Runner) {
	sem := permissive()
	sem.LocalDashSavesTheShellOptions = yes
	// `-f` is a letter two shells spend on something else, and these tests
	// are not about that — see Semantics.SetFTurnsOffGlobbing. Answered so
	// that the letter moves an option at all.
	sem.SetFTurnsOffGlobbing = Yes
	return withSem(sem)
}

func TestLocalDashPutsTheOptionsBack(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"an option the body turns on does not outlive the call",
			`f() { local -; set -u; echo "in=$-"; }; f; echo "after=$-"`,
			"in=u\nafter=\n",
		},
		{
			"and one it turns off comes back on",
			`set -f; f() { local -; set +f; echo "in=$-"; }; f; echo "after=$-"`,
			"in=\nafter=f\n",
		},
		{
			"it declares names beside itself",
			`f() { local - x=2; echo "in=$x"; }; x=1; f; echo "after=$x"`,
			"in=2\nafter=1\n",
		},
		{
			"a nested call saves and restores its own",
			`g() { local -; set -u; echo "g=$-"; }
			 f() { local -; set -f; g; echo "f=$-"; }
			 f; echo "after=$-"`,
			"g=fu\nf=f\nafter=\n",
		},
		{
			"a return unwinds it like everything else the call saved",
			`f() { local -; set -f; return 3; }; f; echo "st=$? opts=$-"`,
			"st=3 opts=\n",
		},
		{
			"and so does an inner function that never returns normally",
			`f() { local -; set -f; g; }; g() { return 4; }; f; echo "opts=$-"`,
			"opts=\n",
		},
		{
			// A subshell inside a function body shares its caller's scope
			// stack, so a save hung there would put the caller's table back
			// when the caller returned — and the subshell's own options
			// never leave it anyway. Measured on bash 5.3.15: both lines
			// below match.
			"a subshell inside the body saves nothing on the caller's scope",
			`f() { ( local -; set -u ); echo "in=$-"; }; f; echo "after=$-"`,
			"in=\nafter=\n",
		},
		{
			"and the call's own save is untouched by it",
			`f() { local -; set -u; ( local -; set -f ); echo "in=$-"; }; f; echo "after=$-"`,
			"in=u\nafter=\n",
		},
		{
			"on its own it is not a declaration and succeeds",
			`f() { local -; echo "st=$?"; }; f`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, savesOptions(Yes))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Where the dialect reads `-` as a name, the operand falls through to the
// declaration it was — nothing about this change may reach that shell, and
// the options a body sets stay set.
func TestWhereTheDashIsANameItStaysOne(t *testing.T) {
	out, _ := run(t, `f() { local -; set -u; }; f; echo "after=$-"`, savesOptions(No))
	// What "a name" then means is the dialect's: the substrate's own answer
	// is that it is not a valid one, and zsh — the shell this answer is for —
	// has a parameter by that name and declares it. Either way nothing here
	// saves an option table, which is what this asserts.
	if !strings.Contains(out, "not a valid identifier") {
		t.Errorf("got %q, want the operand handled as the name it is there", out)
	}
	if strings.Contains(out, "after=") {
		t.Errorf("got %q, want the refusal to have ended the call, as a bad name does", out)
	}
}

// The axis is asked only where the operand was written: an ordinary `local`
// runs under a Runner holding no answer at all.
func TestAnOrdinaryLocalNeverAsks(t *testing.T) {
	sem := permissive()
	sem.LocalDashSavesTheShellOptions = Unspecified

	out, st := run(t, `f() { local x=1; echo "x=$x"; }; f`, withSem(sem))
	if want := "x=1\n"; out != want || st != 0 {
		t.Errorf("ordinary: got %q (status %d), want %q at 0", out, st, want)
	}

	out, st = run(t, `f() { local -; }; f`, withSem(sem))
	if st == 0 {
		t.Errorf("the operand: status 0 and %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "`local -`") {
		t.Errorf("the operand: %q does not name the axis", out)
	}
}

// The save is a row in the call's own listing.
//
// A bare `local` lists what this call made local, and the option table it
// saved is one of them — measured 2026-09-21, `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, from a script file, on bash 5.3.20. See
// localDashListingRow for the rows, and BareLocalListing for whose listing
// this is: the answer that writes the call's locals is the only one that
// reaches the row at all, because the shell that lists every parameter reads
// `-` as a parameter name and never makes the save (#4048).
func TestLocalDashListsAsOneOfTheCallsLocals(t *testing.T) {
	listsLocals := func(r *Runner) {
		sem := permissive()
		sem.LocalDashSavesTheShellOptions = Yes
		sem.BareLocalListing = BareLocalListsLocals
		sem.DeclareListing = DeclareListingClustered
		sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
		r.Semantics = &sem
	}
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"on its own it is the whole listing",
			`f() { local -; local; }; f`,
			"local -\n",
		},
		{
			"and beside a name it is the first row",
			`g() { local -; local x=1; local; }; g`,
			"local -\ndeclare -- x=\"1\"\n",
		},
		{
			"written after the name, and still first",
			`g() { local x=1; local -; local; }; g`,
			"local -\ndeclare -- x=\"1\"\n",
		},
		{
			"asked for twice, it is one row",
			`g() { local -; local -; local x=1; local; }; g`,
			"local -\ndeclare -- x=\"1\"\n",
		},
		{
			"the save belongs to the call that made it and not to one it calls",
			`g() { local -; h; }; h() { local; }; g`,
			"",
		},
		{
			"a call that saved nothing writes no row",
			`g() { local x=1; local; }; g`,
			"declare -- x=\"1\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, listsLocals)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
