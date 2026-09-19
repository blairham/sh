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

// TestAnAppendOperandMovesWithTheOtherOperands: an appending operand is an
// assignment in this grammar, so both fields carry it exactly as they carry a
// plain one.
//
// It did not until #3772. The word was not read as an assignment, so it split
// into fields and the position both lines are keyed on was never recorded —
// the split-before answer left `+ export x+=a b` on one line where the column
// that has it writes `+ x+='a b'` and then `+ export x+`. So this is a trace
// row that was fixed by a change to the *expansion*, which is why it stands
// beside the reading's own suite rather than inside it.
//
// `x+=$v` and not `x+=1`, because the value is what says the word arrived
// whole: a value with no blank in it is one field under either reading.
//
// How the operand is *rendered* where it stands is a third field and not this
// one — see TraceAssignmentOperand — so the rows below carry whatever that
// field's own default writes and vary only in where the operand went.
func TestAnAppendOperandMovesWithTheOtherOperands(t *testing.T) {
	const src = "v=\"a b\"\nset -x\nexport x+=$v\n"
	for _, tc := range []struct {
		name string
		diag Diagnostics
		want string
	}{
		{
			"on the command line",
			Diagnostics{},
			"+ export 'x+=a b'\n",
		},
		{
			"split in front of the command",
			Diagnostics{TraceDeclarationOperand: TraceOperandSplitBefore},
			"+ x+='a b'\n+ export x+\n",
		},
		{
			"written again behind the command",
			Diagnostics{TraceRepeatsAScalarOperandAfter: []string{"export"}},
			"+ export 'x+=a b'\n+ x+='a b'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.diag
			d.TraceQuoting = QuoteShell
			sem := permissive()
			sem.DeclarationTakesAnAppendOperand = Yes
			wantTrace(t, traceOf(t, src, sem, d), tc.want)
		})
	}
}
