// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The five prompt spellings, and the row every listing writes for each.
//
// They were present and **invisible** (#4911), which is a different fault
// from the absent parameters #4866 is a ledger of: `${+PROMPT}` was 1 and
// `$PROMPT3` read `?# `, while `typeset -p PROMPT3` answered
// `no such variable` at 1 and a bare `typeset` wrote no line for any of
// them. A produced name is in none of the tables a listing walks, so the
// values were right and the rows were missing.
//
// # Which default belongs to whom, because it decides what a test may assert
//
// `PS3` is the **dialect's**, from startupvalues.go, so it is `?# ` in a
// Runner this package builds. `PS1`, `PS2` and `PS4` are the **front end's**
// — driver/promptdefaults.go fills them — so they are absent here and hold
// the reference's values only in the shipped binary. Measured 2026-09-27 on
// `cmd/zsh` under `-f` from a script file, where `typeset -p PROMPT4` is
// `typeset PROMPT4='+%N:%i> '` exactly as zsh 5.9.2 writes it, and on the
// package's own Runner, where the same command writes that row with an empty
// value because nothing has put a `PS4` there.
//
// So the row's *value* is asserted through `PROMPT3`, which this package
// owns, and through a store this test writes itself — never through a
// default the driver supplies, which would make this a test of the front end
// passing or failing for a reason in another package.
func TestTheFivePromptSpellingsWriteANamedListingRow(t *testing.T) {
	for _, row := range []struct{ name, want string }{
		{"PROMPT", "typeset PROMPT=''\n"},
		{"PROMPT2", "typeset PROMPT2=''\n"},
		// The one with a value of this package's own, and the row that
		// separates "a row is written" from "a row is written with what is
		// in it": a change producing bare rows passes every line but this.
		{"PROMPT3", "typeset PROMPT3='?# '\n"},
		{"PROMPT4", "typeset PROMPT4=''\n"},
		{"prompt", "typeset prompt=''\n"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `typeset -p `+row.name)
			if out != row.want || st != 0 {
				t.Errorf("typeset -p %s = %q (status %d), want %q at 0", row.name, out, st, row.want)
			}
		})
	}
}

// And the two whole-shell listings, which are a separate question from the
// named `-p` — the panel answers them with different rules in the same shell,
// so one cannot be read off the other.
//
// `PS3` is in both assertions as the control, in the same run: it is an
// ordinary stored scalar with a value and has always listed, so a change that
// broke listings altogether fails the control rather than passing this test
// against two empty haystacks.
func TestTheFivePromptSpellingsAreInTheWholeShellListings(t *testing.T) {
	for _, form := range []string{"typeset -p", "set"} {
		t.Run(form, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), form)
			if st != 0 {
				t.Fatalf("%s: status %d", form, st)
			}
			prefix := ""
			if form == "typeset -p" {
				prefix = "typeset "
			}
			for _, want := range []string{
				prefix + "PS3='?# '",
				prefix + "PROMPT=''",
				prefix + "PROMPT2=''",
				prefix + "PROMPT3='?# '",
				prefix + "PROMPT4=''",
				prefix + "prompt=''",
			} {
				if !strings.Contains(out, want+"\n") {
					t.Errorf("%s wrote no %q line", form, want)
				}
			}
		})
	}
}

// What the rows must **not** bring with them.
//
// Five names arriving in two listings at once is exactly the shape that takes
// a third answer along, so each of the three is asserted rather than assumed:
//
//   - the type word does not move — `${(t)PROMPT}` was `scalar-special`
//     before this and stays that way, from markTheShellsOwnParameters and not
//     from the declaration;
//   - the declaration claims no attribute, so no row carries a letter and
//     `readonly -p` gains nothing, these names being unfrozen;
//   - and the silence the two frozen produced names have is untouched, which
//     is the control that separates "these five now list" from "every
//     produced name now lists".
func TestTheFivePromptSpellingsKeepTheirTypeWordAndClaimNoAttribute(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		for n in PROMPT PROMPT2 PROMPT3 PROMPT4 prompt; do
			print -r -- "$n ${(tP)n}"
		done
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"PROMPT scalar-special",
		"PROMPT2 scalar-special",
		"PROMPT3 scalar-special",
		"PROMPT4 scalar-special",
		"prompt scalar-special",
	)

	out, _ = runZsh(t, t.TempDir(), `readonly -p`)
	for _, name := range []string{"PROMPT", "PROMPT2", "PROMPT3", "PROMPT4", "prompt"} {
		if strings.Contains(out, name+"=") {
			t.Errorf("readonly -p wrote a row for %s; output %q", name, out)
		}
	}

	// The control: `$ARGC` and `$LINENO` are produced too and their named
	// `-p` writes nothing at status 0. A change that gave every producer a
	// row would pass everything above and fail here.
	out, st = runZsh(t, t.TempDir(), `typeset -p ARGC LINENO; print -r -- "rc=$?"`)
	if out != "rc=0\n" || st != 0 {
		t.Errorf("typeset -p ARGC LINENO = %q (status %d), want no row at 0", out, st)
	}
}

// And the pair is still one parameter, which is what the rows are rows of.
//
// A row is only worth having if the name behind it is still the `PS` store
// under another spelling, so both directions are in the same run — and a
// listing must report what a *write through the other name* left, not a value
// captured when the declaration was registered. `PS4` is the store used here
// rather than a default, which is also how the value half of `PROMPT4`'s row
// is asserted without reading the front end's defaults.
func TestAPromptSpellingsRowFollowsTheStoreItIsASpellingOf(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PS1=QQ; typeset -p PROMPT prompt`)
	if st != 0 || out != "typeset PROMPT=QQ\ntypeset prompt=QQ\n" {
		t.Errorf("PS1=QQ then typeset -p = %q at %d, want both spellings reading QQ", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `PS4='+%N:%i> '; typeset -p PROMPT4`)
	if st != 0 || out != "typeset PROMPT4='+%N:%i> '\n" {
		t.Errorf("PS4 set then typeset -p PROMPT4 = %q at %d, want the store's value", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `PROMPT=PP; typeset -p PS1`)
	if st != 0 || out != "typeset PS1=PP\n" {
		t.Errorf("PROMPT=PP then typeset -p PS1 = %q at %d, want PP", out, st)
	}
}
