// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ksh answers the substrate's questions the way ksh93 does.
package ksh

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ksh93 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	d.ParamIndirection = true
	// The measured ksh93 is 93u+ 2012, which has no `&>`. Later ksh93u+m
	// does, twelve years apart under the same name — which is the divergence
	// syntax.Dialect's own comment names as the reason its fields are called
	// after constructs rather than after shells. A dialect built for the
	// newer build sets this back to true; the panel measures the older one.
	d.AmpersandRedirect = false
	return d
}

// Semantics is what ksh93 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithInvalidOctalDigitIsError = interp.No
	s.IndirectionYieldsName = interp.Yes
	s.BraceExpansion = interp.Yes
	s.BracketCaretNegates = interp.Yes
	s.ArithFloat = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	s.ExitArgument = interp.ExitArgLenient
	s.UnsetPositionalIsAllowed = interp.Yes
	s.TraceShowsItsOwnDisabling = interp.No
	s.TraceAssignmentsSeparately = interp.Yes
	// An unparseable `eval` is reported and survived here, unlike dash, but a
	// file `.` cannot open still ends the script — so the two halves of the
	// POSIX "a special builtin's failure is fatal" rule are answered
	// differently, which is why they are two axes.
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotPassesArguments = interp.Yes
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	return s
}

// Diagnostics is how ksh93 reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location:          interp.LocationNone,
		TraceQuoting:      interp.QuoteDollar,
		ScriptLocation:    interp.LocationLineWord,
		ReadonlyVariable:  "%s: is read only",
		ShiftTooMany:      "shift: %d: bad number",
		ArithError:        "%[1]s: %[2]s",
		DivisionByZero:    "divide by zero",
		SyntaxErrorStatus: 3,
		// The status is never reached — a file `.` cannot open ends the script
		// here — but the wording is, and it names the operand and the reason in
		// brackets rather than after a colon.
		DotCannotOpen:      ".: %[1]s: cannot open [%[2]s]",
		DotNoOperand:       ".: Usage: . [ options ] name [arg ...]",
		DotNoOperandStatus: 2,
		// The reason goes in brackets, as it does for `.`.
		ExecFailed: "exec: %[1]s: cannot execute [%[2]s]",
		// Not "cannot execute": ksh93 distinguishes a missing command from one
		// that will not run, and only the second gets the brackets.
		ExecNotFound: "exec: %[1]s: not found",
	}
}

// Apply adjusts the substrate to ksh93: one removal and one addition.
//
// Which builtins a shell provides is neither grammar nor a conflict of
// meaning, so it is not a Dialect flag or a Semantics axis. It is what the
// extension seam is for, and a dialect uses it exactly as anything else built
// on the substrate would — the difference being that this one also takes
// something away.
//
// `local` goes because ksh93 is the only shell in the panel without it: it
// spells the same idea `typeset` and reports `local` as a command that was not
// found, so a script finding it here would be relying on something the real
// shell does not have.
//
// `source` arrives because ksh93 has it as a synonym for `.` and dash does
// not, which makes the name a dialect's answer. It is the same function under
// a second name rather than a second implementation.
func Apply(r *interp.Runner) {
	r.Unregister("local")
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
}
