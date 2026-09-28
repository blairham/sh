// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A `${` that ran out of input **once its operand had begun** ends at the end
// of the input here and the command runs, where bash, zsh and dash all refuse
// the script.
//
// Measured 2026-09-28 from `-c` under `env -i PATH=/usr/bin:/bin` with a
// scratch `HOME`, against `/bin/ksh` `Version AJM 93u+ 2012-08-01`,
// `/opt/homebrew/bin/bash` 5.3.20, `/opt/homebrew/bin/zsh` 5.9.2 and
// `/bin/dash` 0.5.12 — `go version -m` says *not a Go executable* for each.
//
// Every probe carries `; echo AFTER`, and whether `AFTER` arrives is half the
// answer: the operand swallows the rest of the line, so a row that runs prints
// its value and **not** `AFTER`.
//
// The rows here are the **unquoted** spelling on purpose. The quoted one needs
// the driver's reading of an unterminated double quote as well, which this
// harness does not model — `echo "abc` is `abc; echo AFTER` through the
// shipped binary and a parse refusal through a bare Runner — so the quoted
// grid is asserted where the lexer can be driven on its own, in
// syntax.TestAnUnterminatedExpansionOperandEndsAtTheEndOfTheInput.
func TestAnUnterminatedExpansionOperandRuns(t *testing.T) {
	// A refusal, whose wording is this library's rather than the shipped
	// diagnostic's: what the rows below assert is that the expansion is
	// refused, not how. Through the binary both of them are `syntax error at
	// line 1` at status 3, which is the reference's status too.
	const refused = "parse:"
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// Once an operand has begun, whatever the operator is. None of these
		// holds a `{`, which is what says the reading is not the bare brace
		// #4936 gave a way to open a level.
		{`s=abc; echo ${s#x`, "abc\n", 0},
		{`s=abc; echo ${s%x`, "abc\n", 0},
		{`s=abc; echo ${s:-x`, "abc\n", 0},
		{`s=abc; echo ${s/x/y`, "abc\n", 0},
		// And through a bare `{` that opened a level nothing closes, which is
		// the shape #4973 reports.
		{`s=abc; echo ${s#{}`, "abc\n", 0},

		// **Before** an operand has begun it is still refused, and that pair
		// is what fixes the rule: it is not "input that ran out inside an
		// expansion is a value", it is that an operand consumes to the end of
		// the input so no character can be unexpected. The reference refuses
		// both of these too, at status 3.
		{`s=abc; echo ${s`, refused, -1},
		{`s=abc; echo ${#s`, refused, -1},

		// The control: a well-formed expansion is untouched, and `AFTER`
		// arrives because nothing swallowed it.
		{`s=abc; echo ${s}`, "abc\nAFTER\n", 0},
	} {
		out, st := kshOut(t, tc.src+"; echo AFTER")
		if tc.want == refused {
			if st != -1 || !strings.HasPrefix(out, refused) {
				t.Errorf("%s\n got %q at %d\nwant a refusal", tc.src, out, st)
			}
			continue
		}
		if out != tc.want || st != tc.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// And it is one dialect's. A flag test beside the behavior, because the four
// other columns refuse every row above and a change that turned the reading on
// for them would be a change to shells nobody measured that way.
func TestAnUnterminatedExpansionOperandIsKshsAlone(t *testing.T) {
	if !ksh.Dialect().UnterminatedExpansionOperandIsAValue {
		t.Error("ksh: UnterminatedExpansionOperandIsAValue is off, want it on")
	}
	for _, tc := range []struct {
		name string
		d    syntax.Dialect
	}{
		{"bash", bash.Dialect()},
		{"zsh", zsh.Dialect()},
		{"dash", dash.Dialect()},
		{"ash", ash.Dialect()},
	} {
		if tc.d.UnterminatedExpansionOperandIsAValue {
			t.Errorf("%s: UnterminatedExpansionOperandIsAValue is on, want it off", tc.name)
		}
	}
}
