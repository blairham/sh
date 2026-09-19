// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Where `set -x` writes a declaration utility's `name=value` operand —
// Diagnostics.TraceDeclarationOperand and TraceRepeatsAScalarOperandAfter, and
// #3567, where the operand was written on the command line in every column
// because nothing else was modeled.
//
// Named for the fields and not for the shells: which value each preset picks
// is asserted in dialect/xtracedeclaration_test.go against the panel's own
// bytes. What is asserted here is that the answers are different answers, that
// each can be reached, and that neither reaches a word it should not.
//
// The probe carries three rows and every one of them discriminates:
//
//   - `typeset x=1` is the plain operand, and moves under both fields.
//   - `"y=a b"` **on the same command** is the word that only looks like one
//     once it has expanded. It must not move, and it has to stand beside a
//     word that does: a command holding nothing but the look-alike answers
//     the same either way, because the whole command is then one the fields
//     have nothing to say about.
//   - `export ev=1 ew=2` is two operands on one command, which is what says
//     the lines are per operand and in order rather than a repetition of the
//     command.
const declarationOperandProbe = "set -x\ntypeset x=1 \"y=a b\"\nexport ev=1 ew=2\n"

func declarationOperandTrace(t *testing.T, d Diagnostics) string {
	t.Helper()
	d.TraceQuoting = QuoteShell
	return traceOf(t, declarationOperandProbe, permissive(), d)
}

// TestADeclarationOperandStandsOnTheCommandLine is the zero value, which is
// every column but one: the operand is written where the script wrote it.
func TestADeclarationOperandStandsOnTheCommandLine(t *testing.T) {
	wantTrace(t, declarationOperandTrace(t, Diagnostics{}),
		"+ typeset x=1 'y=a b'\n+ export ev=1 ew=2\n")
}

// TestADeclarationOperandIsSplitInFrontOfTheCommand takes each operand off the
// command line and writes it as an assignment ahead of it, leaving the name.
//
// The middle row is the discriminator: an operand that was not written as one
// stays where it is, quoted as the word it is, and the command line still
// carries it.
func TestADeclarationOperandIsSplitInFrontOfTheCommand(t *testing.T) {
	wantTrace(t, declarationOperandTrace(t, Diagnostics{
		TraceDeclarationOperand: TraceOperandSplitBefore,
	}), "+ x=1\n+ typeset x 'y=a b'\n+ ev=1\n+ ew=2\n+ export ev ew\n")
}

// TestADeclarationOperandIsWrittenAgainBehindTheCommand is the other field: the
// command line is unchanged and each operand is written again behind it, for
// the utilities named and for no others.
//
// `typeset` is not in the list here and `export` is, over the same run, which
// is what says the field is a list of command words rather than a property of
// declarations.
func TestADeclarationOperandIsWrittenAgainBehindTheCommand(t *testing.T) {
	wantTrace(t, declarationOperandTrace(t, Diagnostics{
		TraceRepeatsAScalarOperandAfter: []string{"export"},
	}), "+ typeset x=1 'y=a b'\n+ export ev=1 ew=2\n+ ev=1\n+ ew=2\n")
}

// TestTheRepeatFollowsTheUtilityAndNotTheDeclarationReading: a word in front
// that only says where to look for the utility does not stop the repeat, and a
// command word that names nothing in the list does not start one.
func TestTheRepeatFollowsTheUtilityAndNotTheDeclarationReading(t *testing.T) {
	d := Diagnostics{TraceRepeatsAScalarOperandAfter: []string{"export"}}
	sem := permissive()
	sem.CommandPrefixKeepsADeclaration = Yes
	got := traceOf(t, "set -x\ncommand export ev=1\ntypeset tv=2\n", sem, d)
	wantTrace(t, got, "+ command export ev=1\n+ ev=1\n+ typeset tv=2\n")
}

// TestTheOperandLinesAreEmptyWhereNoOperandWasWritten: a declaration utility
// with only names after it writes one line in every answer, so neither field
// can add a line to a command that has no operand.
func TestTheOperandLinesAreEmptyWhereNoOperandWasWritten(t *testing.T) {
	const src = "set -x\nexport ev\n"
	for _, d := range []Diagnostics{
		{TraceDeclarationOperand: TraceOperandSplitBefore},
		{TraceRepeatsAScalarOperandAfter: []string{"export"}},
	} {
		wantTrace(t, traceOf(t, src, permissive(), d), "+ export ev\n")
	}
}
