// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"
)

// KSH_OPTION_PRINT changes the **shape** of the two bare listings and nothing
// else about them. On, `setopt` and `unsetopt` each write every option in the
// table as `name<pad>on|off` instead of naming the deviations and their
// complement, and the two then print the same 185 rows as each other and as
// `set -o`.
//
// That is one sentence with one noun in it — **this option's own state** —
// and the noun is the thing these cases hold still, because the obvious
// alternative reading agrees with it nearly everywhere. `emulate ksh` is what
// a script reaches this through, ksh's default for the option is the only one
// in the table that turns it on, and so "the shape is ksh's" and "the shape
// is this option's" give the same answer on every row where nobody has
// touched the option. TestTheListingShapeIsKeyedOnTheOptionAndNotOnTheMode
// below is the deliberate pair that parts them.
//
// Measured 2026-09-25 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh` — `go version -m` says *not a Go executable* — with
// `-f -c`. `setopt kshoptionprint; setopt`, the same with `unsetopt`, and the
// same with `set -o` are three byte-identical 185-line listings; the state
// word begins at column 23 on every one of them, and the longest name written
// is 21 characters.
//
// Two rows of the reference's own listing are left out of the expectations
// here and it is not a license to weaken them. `rcs` is read off what the
// *front end* was told, so a `-f` binary writes `norcs` where this runner is
// the library's and reads it on; and `hashcmds` carries this shell's own
// state as its table default rather than zsh's, which is a standing
// difference on `set -o` too and is filed separately. The binary's answer for
// both, against real zsh over one copy of one file, is pinned in
// share/suite/zsh/options.tests.
func TestKshOptionPrintRewritesBothBareListingsIntoTheLongForm(t *testing.T) {
	dir := t.TempDir()
	setopt, st := runZsh(t, dir, "setopt kshoptionprint\nsetopt\n")
	if st != 0 {
		t.Fatalf("setopt status %d", st)
	}
	rows := listingLines(setopt)
	if len(rows) != 185 {
		t.Errorf("the long form writes %d rows, want 185", len(rows))
	}
	for _, row := range rows {
		name, state, ok := strings.Cut(row, " ")
		if !ok {
			t.Fatalf("row %q is one word, so it is still the short shape", row)
		}
		if got := strings.TrimLeft(state, " "); got != "on" && got != "off" {
			t.Errorf("row %q ends in %q, want on or off", row, got)
		}
		// The column, which is the half a row-by-row name check would miss:
		// `%-22s` and `name + " " + state` produce the same set of names and
		// only one of them is the reference's shape.
		if len(row) != 22+len(strings.TrimLeft(state, " ")) {
			t.Errorf("row %q puts the state at column %d, want 23", row, 22+1)
		}
		if len(name) > 21 {
			t.Errorf("name %q is longer than the column", name)
		}
	}

	// The direction a bare name means is the whole difference between the two
	// builtins, and this form has no direction — so they print the same table
	// rather than complementary halves of one.
	unsetopt, st := runZsh(t, dir, "setopt kshoptionprint\nunsetopt\n")
	if st != 0 {
		t.Fatalf("unsetopt status %d", st)
	}
	if unsetopt != setopt {
		t.Errorf("`unsetopt` under the long form differs from `setopt`:\n%s", firstDiff(setopt, unsetopt))
	}

	// And it is `set -o`'s table rather than a second one beside it, which is
	// what says the two cannot drift apart over a width or an ordering.
	seto, st := runZsh(t, dir, "setopt kshoptionprint\nset -o\n")
	if st != 0 {
		t.Fatalf("set -o status %d", st)
	}
	if seto != setopt {
		t.Errorf("the long form differs from `set -o`:\n%s", firstDiff(setopt, seto))
	}
}

// **The option decides the shape; the mode decides the contents.** The two
// rows below hold the option's state fixed and move the mode, and neither
// shape moves with it: a mode that is not ksh prints the long form while the
// option is on, and ksh itself prints the short one while it is off.
//
// This is the pair the wide grid would have missed. Every unemulated row and
// every `emulate ksh` row agrees under both readings; only a mode and an
// option that disagree about ksh tell them apart, and there are exactly two
// such cases.
func TestTheListingShapeIsKeyedOnTheOptionAndNotOnTheMode(t *testing.T) {
	for _, c := range []struct {
		name, src string
		long      bool
	}{
		{"a mode that is not ksh, with the option on", "emulate sh\nsetopt kshoptionprint\nsetopt\n", true},
		{"ksh itself, with the option off", "emulate ksh\nunsetopt kshoptionprint\nsetopt\n", false},
		// The two controls, where the readings coincide. Without them a
		// failure of the pair above could be a listing stuck in one shape.
		{"ksh with the option at ksh's own default", "emulate ksh\nsetopt\n", true},
		{"zsh with the option at zsh's own default", "emulate zsh\nsetopt\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if st != 0 {
				t.Fatalf("status %d", st)
			}
			rows := listingLines(out)
			got := len(rows) == 185 && strings.Contains(rows[0], " ")
			if got != c.long {
				t.Errorf("the listing is %s (%d rows, first %q), want %s",
					shapeName(got), len(rows), first(rows), shapeName(c.long))
			}
		})
	}
}

// The two rules compose rather than one replacing the other: the shape is
// this option's and the baseline every row is measured against is still the
// mode's (#4517). Holding the *state* of a pair fixed and moving the mode
// inverts both halves of their rows inside a listing whose shape has not
// moved at all.
//
// Measured on zsh 5.9.2: `promptpercent` reads on and `promptsubst` off under
// `emulate zsh` and under `emulate sh` alike — neither is reset by a bare
// emulation — while sh's defaults for the two are the opposite of zsh's.
func TestTheLongFormStillTakesItsBaselineFromTheMode(t *testing.T) {
	for _, c := range []struct{ mode, want string }{
		{"emulate zsh", "nopromptpercent       off\npromptsubst           off"},
		{"emulate sh", "promptpercent         on\nnopromptsubst         on"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.mode+"\nsetopt kshoptionprint\nsetopt\n")
			if st != 0 {
				t.Fatalf("status %d", st)
			}
			var got []string
			for _, row := range listingLines(out) {
				if name, _, ok := strings.Cut(row, " "); ok && isPromptRow(name) {
					got = append(got, row)
				}
			}
			if strings.Join(got, "\n") != c.want {
				t.Errorf("%s then the long form writes\n%q\nwant\n%q",
					c.mode, strings.Join(got, "\n"), c.want)
			}
		})
	}
}

// The long form belongs to the **bare** command. Naming an option is a
// request to move one and prints nothing in either shape, which is what says
// this is a listing's rendering rather than a second thing `setopt` does.
func TestNamingAnOptionPrintsNothingInEitherShape(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"the long form", "setopt kshoptionprint\nsetopt autocd\nunsetopt autocd\n"},
		{"the short form", "setopt autocd\nunsetopt autocd\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != "" || st != 0 {
				t.Errorf("%s printed %q at status %d, want nothing at 0", c.name, out, st)
			}
		})
	}
}

// `set +o` is not one of the surfaces this option reaches. It writes
// re-inputtable `set ±o name` lines in both states of it, and the only row
// that moves is the one naming the option itself — measured on zsh 5.9.2,
// where turning it on changes `set +o kshoptionprint` to `set -o
// kshoptionprint` and leaves the other 184 lines alone.
//
// This is the row that says the rewrite is the *state* listing's rather than
// every listing's, which a case that only ever looked at `setopt` could not
// distinguish.
func TestSetPlusOKeepsItsOwnShape(t *testing.T) {
	dir := t.TempDir()
	off, st := runZsh(t, dir, "set +o\n")
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	on, st := runZsh(t, dir, "setopt kshoptionprint\nset +o\n")
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	want := strings.Replace(off, "set +o kshoptionprint\n", "set -o kshoptionprint\n", 1)
	if want == off {
		t.Fatalf("`set +o` never names kshoptionprint, so this case is measuring nothing")
	}
	if on != want {
		t.Errorf("`set +o` under the option =\n%s", firstDiff(want, on))
	}
}

func shapeName(long bool) string {
	if long {
		return "the long form"
	}
	return "the short form"
}

func first(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0]
}

// firstDiff names the first line two listings disagree on, which is more use
// than 185 lines of each.
func firstDiff(want, got string) string {
	a, b := listingLines(want), listingLines(got)
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := "", ""
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return fmt.Sprintf("line %d: want %q\n         got  %q", i+1, x, y)
		}
	}
	return "no differing line, so the two differ only in length"
}
