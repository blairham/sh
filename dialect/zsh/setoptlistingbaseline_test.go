// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A bare `setopt` prints the option names that **deviate from a default**, and
// the default it means is the one belonging to the mode the shell is
// *emulating* rather than zsh's own.
//
// That is one sentence with one noun in it — **the mode's default** — and the
// noun is what these cases are written to hold still, because the wrong noun
// reads exactly like the right one almost everywhere. Three readings agree on
// every row of a shell that has not emulated anything:
//
//   - the listing is the names that are **on** — false, and `nohashdirs` in a
//     plain shell already says so;
//   - the listing is the names away from **zsh's** default — what this shell
//     did, and #4517;
//   - the listing is the names away from **this mode's** default — zsh's.
//
// The second and the third part company only under an emulation, which is why
// every case below sets one.
//
// Measured 2026-09-25 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh` — `go version -m` says *not a Go executable* — with
// `-f -c` and the emulation on the line above the listing.
//
// **One name is dropped from the expectations here and it is not a license to
// weaken them.** `rcs` is a [recordedOver] name whose base state is whatever
// the *front end* was told, so real zsh under `-f` reads it off and prints
// `norcs` in the first, third and fifth rows below, while this runner is the
// library's rather than the binary's and reads it on. Every other name in
// every row is the reference's own, byte for byte; the binary's `-f` answer,
// `norcs` included, is pinned against real zsh in share/suite/zsh/options.tests,
// which runs both shells over one copy of the file.
func TestTheBareListingComparesAgainstTheEmulationsOwnDefaults(t *testing.T) {
	for _, c := range []struct{ name, mode, want string }{
		// The control, and the reason this instrument can be trusted to
		// produce agreement rather than only a difference: under `emulate
		// zsh` the two readings coincide, and both shells print the two rows
		// a `zsh -f` shell deviates by.
		{"zsh is the mode with no column of its own", "emulate zsh", "nohashdirs\n"},
		{"a strict zsh emulation leaves nothing away from zsh's own", "emulate -R zsh", ""},
		// The eight names a bare `emulate sh` does *not* reset — they are in
		// emulationStrictReset — whose sh default differs from zsh's. Their
		// states are exactly what they were before the emulation; it is the
		// baseline underneath them that moved.
		{
			"a bare sh emulation", "emulate sh",
			"banghist\nnohashdirs\nnointeractivecomments\nnotify\n" +
				"promptpercent\nnopromptsubst\nnormstarsilent\n",
		},
		// And the strict form resets those eight too, so nothing is left away
		// from sh's defaults. The pair is the second control: **the mode is
		// held fixed at sh** across these two rows, so the baseline is the
		// same in both and only the states moved. A listing keyed on what the
		// emulation *wrote* would print nothing in the row above as well.
		{"a strict sh emulation", "emulate -R sh", ""},
		{
			"a bare csh emulation", "emulate csh",
			"noextendedhistory\nnohashdirs\nnotify\n",
		},
		{"a strict csh emulation", "emulate -R csh", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.mode+"\nsetopt\n")
			if out != c.want || st != 0 {
				t.Errorf("%s then setopt =\n%q\nwant\n%q\nat status 0, got %d", c.mode, out, c.want, st)
			}
		})
	}
}

// The same baseline decides `unsetopt`, which is the complement and not a
// second rule: 185 names, and every one is in exactly one of the two lists.
func TestTheTwoListingsPartitionTheTable(t *testing.T) {
	for _, mode := range []string{"emulate zsh", "emulate sh", "emulate -R sh", "emulate csh"} {
		t.Run(mode, func(t *testing.T) {
			set, st := runZsh(t, t.TempDir(), mode+"\nsetopt\n")
			if st != 0 {
				t.Fatalf("setopt status %d", st)
			}
			unset, st := runZsh(t, t.TempDir(), mode+"\nunsetopt\n")
			if st != 0 {
				t.Fatalf("unsetopt status %d", st)
			}
			n := len(listingLines(set)) + len(listingLines(unset))
			if n != 185 {
				t.Errorf("%s: setopt and unsetopt name %d options between them, want 185", mode, n)
			}
			for _, name := range listingLines(set) {
				if strings.Contains("\n"+unset, "\n"+name+"\n") {
					t.Errorf("%s: %s is in both listings", mode, name)
				}
			}
		})
	}
}

// **The state held fixed while the mode moves**, which is the pair that says
// the rule is keyed on the default and not on the state.
//
// `promptpercent` and `promptsubst` are both in emulationStrictReset, so a
// bare `emulate sh` does not touch either: measured on zsh 5.9.2, both read
// `on` and `off` respectively before and after, in every column below. What
// moves is sh's default for them — off and on, the opposite of zsh's — and so
// both the *list they are in* and the *spelling they are written in* invert
// while nothing about the shell's behavior has changed.
//
// A rule keyed on the state would put the same row in the same list with the
// same spelling in both columns, which is what this shell did.
func TestTheListingIsKeyedOnTheModesDefaultRatherThanOnTheState(t *testing.T) {
	for _, c := range []struct {
		mode string
		// state is what `${options[…]}` reads for the pair; it is the same in
		// both rows, which is the point.
		state string
		// setopt and unsetopt are the spellings of the two names in each
		// listing, in table order.
		setopt, unsetopt string
	}{
		{"emulate zsh", "on off", "", "nopromptpercent promptsubst"},
		{"emulate sh", "on off", "promptpercent nopromptsubst", ""},
	} {
		t.Run(c.mode, func(t *testing.T) {
			dir := t.TempDir()
			out, st := runZsh(t, dir, c.mode+"\nzmodload zsh/parameter\n"+
				"print -r -- \"${options[promptpercent]} ${options[promptsubst]}\"\n")
			if st != 0 {
				t.Fatalf("status %d", st)
			}
			if got := strings.TrimRight(out, "\n"); got != c.state {
				t.Fatalf("%s: the pair reads %q, want %q — the case is about a mode change that moves no state",
					c.mode, got, c.state)
			}
			for _, listing := range []struct{ builtin, want string }{
				{"setopt", c.setopt}, {"unsetopt", c.unsetopt},
			} {
				out, st := runZsh(t, dir, c.mode+"\n"+listing.builtin+"\n")
				if st != 0 {
					t.Fatalf("%s status %d", listing.builtin, st)
				}
				got := strings.Join(promptRows(out), " ")
				if got != listing.want {
					t.Errorf("%s then %s names %q of the pair, want %q",
						c.mode, listing.builtin, got, listing.want)
				}
			}
		})
	}
}

// `set -o` is the third surface and it is the same baseline again: one row per
// option, spelled the way the mode's default spells it and marked `on` exactly
// when the state deviates from that default.
//
// It is **not** the same surface as `${options[…]}`, and the two rows below
// are why both are here. Measured on zsh 5.9.2, `${options[promptsubst]}` is
// `off` in both modes — the parameter reports the *state* — while `set -o`
// writes `promptsubst off` under zsh and `nopromptsubst on` under sh.
func TestTheSetOListingMovesWithTheEmulationToo(t *testing.T) {
	for _, c := range []struct{ mode, want string }{
		{"emulate zsh", "nopromptpercent       off\npromptsubst           off"},
		{"emulate sh", "promptpercent         on\nnopromptsubst         on"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.mode+"\nset -o\n")
			if st != 0 {
				t.Fatalf("status %d", st)
			}
			var got []string
			for _, line := range strings.Split(out, "\n") {
				if name, _, ok := strings.Cut(line, " "); ok && isPromptRow(name) {
					got = append(got, line)
				}
			}
			if strings.Join(got, "\n") != c.want {
				t.Errorf("%s then `set -o` writes\n%q\nwant\n%q", c.mode, strings.Join(got, "\n"), c.want)
			}
		})
	}
}

// And the condition does not move with it either. `[[ -o promptsubst ]]` asks
// what the state *is*; a listing asks how far it is from a default. Holding
// the state fixed and moving the mode moves the listing and leaves this alone,
// which is what says the baseline belongs to the listings rather than to the
// namespace.
func TestTheConditionReadsTheStateAndNotTheDeviation(t *testing.T) {
	for _, mode := range []string{"emulate zsh", "emulate sh"} {
		t.Run(mode, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), mode+"\n"+
				"[[ -o promptpercent ]] && print -r -- pc=on || print -r -- pc=off\n"+
				"[[ -o promptsubst ]] && print -r -- ps=on || print -r -- ps=off\n")
			if out != "pc=on\nps=off\n" || st != 0 {
				t.Errorf("%s then the two conditions = %q status %d, want %q at 0",
					mode, out, st, "pc=on\nps=off\n")
			}
		})
	}
}

// listingLines splits a listing into its names.
func listingLines(out string) []string {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// promptRows keeps the rows naming either half of the measured pair.
func promptRows(out string) []string {
	var rows []string
	for _, line := range listingLines(out) {
		if isPromptRow(line) {
			rows = append(rows, line)
		}
	}
	return rows
}

func isPromptRow(name string) bool {
	switch name {
	case "promptpercent", "nopromptpercent", "promptsubst", "nopromptsubst":
		return true
	}
	return false
}
