// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `help` is this shell's only self-documenting builtin, and it used to be no
// builtin at all: `help true` was `command not found` at 127 (#2623).
//
// Every row here is measured on bash 5.3.15, 2026-09-13, and the ones marked
// are the discriminators — a rule that looks like the rule beside it until
// exactly one case parts them.
func TestHelpAnswersTheSynopsis(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a builtin", `help -s true`, "true: true\n", 0},
		{
			"and without the letter, since there is no description to leave out",
			`help true`, "true: true\n", 0,
		},
		{"a keyword", `help -s while`, "while: while COMMANDS; do COMMANDS-2; done\n", 0},
		{
			"several operands, in the order asked",
			`help -s cd pwd`, "cd: cd [-L|[-P [-e]]] [-@] [dir]\npwd: pwd [-LP]\n", 0,
		},
		{"the operand after --", `help -s -- cd`, "cd: cd [-L|[-P [-e]]] [-@] [dir]\n", 0},
		// The exact-match rule. `time` and `times` are both topics, and a
		// prefix match alone would answer with both.
		{"an exact name wins over the prefix", `help -s time`, "time: time [-p] pipeline\n", 0},
		// The prefix rule. Without it this answers nothing at all.
		{"a name that is only a prefix", `help -s ec`, "echo: echo [-neE] [arg ...]\n", 0},
		{
			"and one that matches two", `help -s sh`,
			"shift: shift [n]\nshopt: shopt [-pqsu] [-o] [optname ...]\n", 0,
		},
		// The pattern rule, and the case that says it is not the prefix rule
		// with a `*` appended: that reading matches `getopts` and bash does
		// not.
		{
			"a pattern is anchored at both ends", `help -s '*pt'`,
			"Shell commands matching keyword `*pt'\n\n" +
				"compopt: compopt [-o|+o option] [-DEI] [name ...]\n" +
				"shopt: shopt [-pqsu] [-o] [optname ...]\n", 0,
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: %s = %q status %d, want %q status %d",
				tc.name, tc.src, out, st, tc.want, tc.status)
		}
	}
}

// The refusals, and the statuses that go with them.
func TestHelpRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, src, said string
		status          int
	}{
		{
			"a topic nobody has", `help nosuchthing`,
			"help: no help topics match `nosuchthing'.  Try `help help' or " +
				"`man -k nosuchthing' or `info nosuchthing'.", 1,
		},
		{
			"a bad option, with the usage line after it", `help -q true`,
			"help: -q: invalid option\nhelp: usage: help [-dms] [pattern ...]", 2,
		},
		// The two letters that ask for the description. Refused by name
		// rather than answered with the synopsis: this shell does not carry
		// another project's documentation prose, and a letter taken and
		// answered with something else hands a script a success it did not
		// earn.
		{"the summary letter", `help -d true`, "help: -d: not implemented", 2},
		{"the man-page letter", `help -m true`, "help: -m: not implemented", 2},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if !strings.Contains(out, tc.said) || st != tc.status {
			t.Errorf("%s: %s = %q status %d, want %q status %d",
				tc.name, tc.src, out, st, tc.said, tc.status)
		}
	}
	// A pattern that matches nothing still writes its header first, at 1.
	// Measured, and it is what says the header is written before the match
	// rather than after it.
	out, st := runBash(t, t.TempDir(), `help -s 'z*'`)
	if st != 1 || !strings.HasPrefix(out, "Shell commands matching keyword `z*'\n\n") {
		t.Errorf(`help -s 'z*' = %q status %d, want the header and then the complaint`, out, st)
	}
}

// `help -s ”` writes every topic, one per line and sorted.
//
// This used to assert that a **bare** `help` wrote the same lines, on the
// reasoning that the real shell's listing opens with a version line this shell
// must not claim and so the whole listing was ours to shape. Re-measured
// 2026-09-18, the two are different answers in the real shell as well —
// `help -s ”` is 77 lines of `name: synopsis` and a bare `help` is the
// 47-line two-column listing — so the divergence was in the *layout*, which is
// measurable, rather than in the header, which is the part that cannot be
// reproduced (#3055). The bare listing's own rows are asserted in
// helplisting_test.go; this is the one-per-line form it is not.
func TestBareHelpListsEveryTopic(t *testing.T) {
	bare, st := runBash(t, t.TempDir(), `help -s ''`)
	if st != 0 {
		t.Fatalf("help exited %d", st)
	}
	listing, _ := runBash(t, t.TempDir(), `help`)
	if bare == listing {
		t.Errorf("an empty operand and no operand wrote the same thing; they are two answers")
	}
	lines := strings.Split(strings.TrimRight(bare, "\n"), "\n")
	if len(lines) < 60 {
		t.Errorf("%d topics listed, want the whole table", len(lines))
	}
	// Sorted by *topic name* and not by the rendered line, which are two
	// different orders: `for` comes before `for ((` where `for:` comes after
	// `for ((:`, the colon sorting above the space. bash writes them in name
	// order, so that is what is checked.
	for i := 1; i < len(lines); i++ {
		prev, _, _ := strings.Cut(lines[i-1], ": ")
		this, _, _ := strings.Cut(lines[i], ": ")
		if prev > this {
			t.Errorf("the listing is not in name order: %q before %q", prev, this)
			break
		}
	}
	// A builtin this dialect really has, a keyword, and one of the six that
	// answer `--help` with nothing and are topics all the same.
	for _, want := range []string{"cd: ", "for: ", "true: true", "[: [ arg... ]"} {
		if !strings.Contains(bare, want) {
			t.Errorf("the listing has no %q", want)
		}
	}
}

// The topic list has **77 names**, and the one that made it 76 is the reason
// this is a test rather than a table.
//
// `variables` is the one topic whose line is a sentence about what the topic
// covers rather than a synopsis of a construct, so the real shell's wording is
// CLEANROOM.md's red list and this shell writes its own. It was left out
// entirely for that reason, and leaving it out was the more expensive of the
// two choices: the bare listing is laid out **column-major**, so the topic
// count decides how many rows there are and which pair of names each row
// holds. One short of 77 moves every row of a 39-row listing, and a listing
// differing in a single cell was instead differing in all of them — 76 lines
// of one suite file where 2 was available (#2298).
//
// So the count is asserted, not just the membership: a topic quietly added or
// dropped is a listing that stops lining up, and the cell is the smaller half
// of what goes wrong.
func TestEveryTopicIsListedIncludingTheOneWrittenInOurOwnWords(t *testing.T) {
	bare, st := runBash(t, t.TempDir(), `help -s ''`)
	if st != 0 {
		t.Fatalf("help exited %d", st)
	}
	lines := strings.Split(strings.TrimRight(bare, "\n"), "\n")
	// Measured 2026-09-21 on bash 5.3.20 with `help -s ''` in `env -i`.
	if want := 77; len(lines) != want {
		t.Errorf("%d topics, want %d — the listing's geometry is the count", len(lines), want)
	}
	if !strings.Contains(bare, "variables: variables - ") {
		t.Error("no `variables` topic, which is the name whose absence moves every row of the listing")
	}
	// And it answers, at 0, the way every other topic does. The wording is
	// ours; that it is a topic at all is the real shell's.
	out, st := runBash(t, t.TempDir(), `help variables`)
	if st != 0 {
		t.Errorf("help variables exited %d, want 0", st)
	}
	if !strings.HasPrefix(out, "variables: variables - ") {
		t.Errorf("help variables wrote %q", out)
	}
}
