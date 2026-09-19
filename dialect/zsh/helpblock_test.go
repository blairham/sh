// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"
	"testing"
)

// The `--help` block is **generated from the three option tables**, and these
// are the guards that keep it so. A committed block would pass a test that
// only looked at the text it shipped with; what these ask is whether the block
// and the tables are the same data, which a committed one could not answer on
// the day a name was added.
//
// See helpBlock for the measurement that settled generate-against-paste — the
// reference shell's own help misstates four of its own rows.

// section returns the rows under one heading of the block, without their
// indentation, so a test can count them and read them.
func section(t *testing.T, heading string) []string {
	t.Helper()
	block := helpBlock()
	_, rest, found := strings.Cut(block, "\n"+heading+":\n")
	if !found {
		t.Fatalf("the block has no %q section:\n%s", heading, block)
	}
	body, _, _ := strings.Cut(rest, "\n\n")
	var rows []string
	for _, line := range strings.Split(body, "\n") {
		if row := strings.TrimSpace(line); row != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

// Every name in the table is a row, and there are no rows that are not names.
// The count is the half that fails when a name is added: a block that did not
// move would be short by exactly one.
func TestTheHelpBlockListsEveryOptionNameAndNoOthers(t *testing.T) {
	rows := section(t, "Named options")
	if len(rows) != len(zshOptions) {
		t.Errorf("%d named rows, want %d — the block and the table have parted",
			len(rows), len(zshOptions))
	}
	listed := map[string]bool{}
	for _, row := range rows {
		listed[strings.TrimPrefix(row, "--")] = true
	}
	for i := range zshOptions {
		if !listed[zshOptions[i].base] {
			t.Errorf("%q is in the table and not in the block", zshOptions[i].base)
		}
	}
}

// The aliases, each with the name it resolves to and the direction it
// resolves in. The direction is the half a reader acts on, and it is the half
// the reference shell's own block gets wrong.
func TestTheHelpBlockNamesWhatEachAliasResolvesTo(t *testing.T) {
	rows := section(t, "Option aliases")
	if len(rows) != len(zshOptionAliases) {
		t.Errorf("%d alias rows, want %d", len(rows), len(zshOptionAliases))
	}
	for alias, to := range zshOptionAliases {
		want := "--" + to.base
		if to.inv {
			want = "--no-" + to.base
		}
		found := false
		for _, row := range rows {
			if strings.HasPrefix(row, "--"+alias+" ") {
				found = true
				if !strings.HasSuffix(row, "equivalent to "+want) {
					t.Errorf("%q says %q, want it to end `equivalent to %s'", alias, row, want)
				}
			}
		}
		if !found {
			t.Errorf("%q is in the alias table and not in the block", alias)
		}
	}
}

// And the letters. The one that is taken and moves nothing is a row too —
// a letter missing from the list reads as a letter the shell refuses, and
// that one is accepted.
func TestTheHelpBlockListsEveryOptionLetter(t *testing.T) {
	rows := section(t, "Option letters")
	if len(rows) != len(setLetterOptions) {
		t.Errorf("%d letter rows, want %d", len(rows), len(setLetterOptions))
	}
	for letter, name := range setLetterOptions {
		found := false
		for _, row := range rows {
			if !strings.HasPrefix(row, "-"+string(letter)+" ") && row != "-"+string(letter) {
				continue
			}
			found = true
			switch {
			case name == "" && !strings.Contains(row, "moves nothing"):
				t.Errorf("-%c says %q, want it to say the letter moves nothing", letter, row)
			case name != "" && !strings.HasSuffix(row, "equivalent to --"+name):
				t.Errorf("-%c says %q, want `equivalent to --%s'", letter, row, name)
			}
		}
		if !found {
			t.Errorf("-%c is in the letter table and not in the block", letter)
		}
	}
}

// The block reads the shell's own name rather than baking one in, which is
// what the trailer's one verb is for. A `%` anywhere else in it would be read
// as a verb too, so the format is checked rather than trusted.
func TestTheHelpBlockCarriesExactlyOneVerb(t *testing.T) {
	block := helpBlock()
	if n := strings.Count(block, "%"); n != 1 {
		t.Errorf("%d `%%' in the block, want exactly the one that names the shell", n)
	}
	if !strings.HasPrefix(block, "Usage: %[1]s ") {
		t.Errorf("the block opens %q, want the usage line to name the shell",
			strings.SplitN(block, "\n", 2)[0])
	}
}
