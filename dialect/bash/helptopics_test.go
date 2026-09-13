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

// A bare `help` writes every topic, one per line and sorted, which is the
// same set and the same lines its own `help -s ”` writes.
//
// The real shell's own bare listing opens with its version and four lines
// about itself and then packs the synopses into two truncated columns. The
// version line is a claim this shell must not make, so what is left is the
// same information one topic per line. That is a deliberate divergence and
// the one place `help` is not this shell answering as bash answers.
func TestBareHelpListsEveryTopic(t *testing.T) {
	bare, st := runBash(t, t.TempDir(), `help`)
	if st != 0 {
		t.Fatalf("help exited %d", st)
	}
	empty, _ := runBash(t, t.TempDir(), `help -s ''`)
	if bare != empty {
		t.Errorf("a bare help and `help -s \"\"` differ")
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
