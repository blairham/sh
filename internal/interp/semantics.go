// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Semantics is where the shells disagree about what identical syntax *means*.
//
// This is the structure docs/spec/semantics.md argued for, and the argument
// is worth restating because the obvious alternative looks fine. Grammar
// differences are additive — a construct either parses or it does not — and
// syntax.Dialect models those. Semantic differences are conflicts: the same
// text means different things, and no amount of adding or removing features
// produces one from another. They need switches.
//
// Every field is named for the behaviour rather than for the shell that wants
// it, which the spec requires and the measurements insist on: ksh93 accepts
// `&>` or does not depending on which build is installed, twelve years apart
// under the same name, so a field called `Ksh` could not be given a value.
//
// Eighteen axes were measured across four shells and produced eight distinct
// groupings — no ordering of the shells explains the data, which is why this
// is a vector and not a level.
type Semantics struct {
	// SplitParamExpansion field-splits the result of an unquoted parameter
	// expansion. False in zsh, and narrower than "word splitting": zsh still
	// splits an unquoted *command* substitution, so this is two fields and
	// not one.
	SplitParamExpansion bool
	// SplitCommandSubstitution field-splits an unquoted command
	// substitution. True everywhere measured, including zsh.
	SplitCommandSubstitution bool

	// GlobExpansionResults matches the *result* of an expansion against the
	// filesystem. False in zsh, where only a pattern written literally in the
	// source is expanded. The same rule decides whether `[[ abc == $p ]]`
	// treats $p as a pattern, which is one behaviour observed twice rather
	// than two quirks.
	GlobExpansionResults bool
	// GlobNoMatchIsError makes a pattern matching nothing an error instead of
	// passing it through. True only in zsh.
	GlobNoMatchIsError bool

	// AssignmentPrefixPersistsOnSpecialBuiltin keeps `x=1 shift` set
	// afterwards. POSIX requires it; dash and ksh93 comply and bash and zsh
	// do not.
	AssignmentPrefixPersistsOnSpecialBuiltin bool

	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes bool

	// LengthOfSpecialIsCount makes `${#@}` the number of positional
	// parameters. False in dash, which gives the length of the joined
	// string. The first axis measured where dash stands alone, and a silent
	// one: both answers are plausible numbers.
	LengthOfSpecialIsCount bool

	// ArithLeadingZeroIsOctal reads `0100` as sixty-four. False in zsh, where
	// it is one hundred. The quietest divergence measured — nothing warns,
	// both are plausible numbers, and file modes are written this way.
	ArithLeadingZeroIsOctal bool
	// ArithFloat evaluates floating point. True in ksh93 and zsh, where POSIX
	// says integers only.
	ArithFloat bool

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string. True in bash alone; ksh93 and zsh keep it a regex, so
	// quoting a regex is unportable in either direction.
	RegexQuotingMakesLiteral bool

	// LastPipelineElementInCurrentShell runs the last command of a pipeline
	// in this shell, so `echo x | read v` sets v. True in ksh93 and zsh.
	LastPipelineElementInCurrentShell bool

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off
	// the end. True in dash and ksh93.
	ShiftPastEndFatal bool
	// ReadonlyReassignmentFatal ends the script when a readonly variable is
	// assigned. True everywhere but bash, measured with a plain assignment in
	// a script file — adding a redirect makes it a command and reverses the
	// answer, which is the contaminated-probe trap docs/spec/oracle.md
	// records.
	ReadonlyReassignmentFatal bool

	// DollarZeroInFunctionIsFunctionName makes `$0` inside a function the
	// function's name. True only in zsh.
	DollarZeroInFunctionIsFunctionName bool
}

// There is deliberately no CoreSemantics, and the absence is the sharpest
// consequence of the whole specification.
//
// syntax.Core() exists because grammar differences are *additive*: a
// construct either parses or it does not, so "what every shell accepts" is a
// well-defined intersection. Semantic differences are conflicts. There is no
// intersection of "an unquoted expansion is split" and "it is not", and no
// value of a boolean means both. A semantics preset therefore has to name a
// shell, while a grammar preset does not.
//
// The pairing this implies is not an inconsistency: accept the constructs
// every real shell accepts, and behave like the one scripts were written
// against — syntax.Core() with BashSemantics().
//
// Which pairing to use is not this package's decision. This is the substrate:
// it owns the *mechanism*, and a shell built on it owns the policy, the same
// way the gate and the event stream are defined here and the sandbox backends
// and protocols are not. The presets exist so that choice can be spelled in
// one line rather than eighteen.

// PosixSemantics is what the specification requires, which is not what any
// shell does in full — it is the right target for a portability check and the
// wrong one for a runtime.
func PosixSemantics() Semantics {
	return Semantics{
		SplitParamExpansion:                      true,
		SplitCommandSubstitution:                 true,
		GlobExpansionResults:                     true,
		AssignmentPrefixPersistsOnSpecialBuiltin: true,
		LengthOfSpecialIsCount:                   true,
		ArithLeadingZeroIsOctal:                  true,
		ShiftPastEndFatal:                        true,
		ReadonlyReassignmentFatal:                true,
	}
}

// BashSemantics is bash's answers.
func BashSemantics() Semantics {
	s := PosixSemantics()
	s.AssignmentPrefixPersistsOnSpecialBuiltin = false
	s.ReadonlyReassignmentFatal = false
	s.ShiftPastEndFatal = false
	s.RegexQuotingMakesLiteral = true
	return s
}

// ZshSemantics is zsh's, and is the one that shows why this is a vector: it
// differs from bash on seven axes and agrees with dash on one of them.
func ZshSemantics() Semantics {
	s := BashSemantics()
	s.SplitParamExpansion = false
	s.GlobExpansionResults = false
	s.GlobNoMatchIsError = true
	s.EchoInterpretsEscapes = true
	s.ArithLeadingZeroIsOctal = false
	s.ArithFloat = true
	s.RegexQuotingMakesLiteral = false
	s.LastPipelineElementInCurrentShell = true
	s.DollarZeroInFunctionIsFunctionName = true
	return s
}

// KshSemantics is ksh93's.
func KshSemantics() Semantics {
	s := PosixSemantics()
	s.ArithFloat = true
	s.LastPipelineElementInCurrentShell = true
	return s
}

// DashSemantics is dash's.
func DashSemantics() Semantics {
	s := PosixSemantics()
	s.EchoInterpretsEscapes = true
	s.LengthOfSpecialIsCount = false
	return s
}

// sem returns the runner's semantics.
//
// The fallback is bash's, so the zero Runner is usable rather than
// meaningless — the zero Semantics would answer "no" to every axis, which is
// not any shell. It is a default in the Go sense and not a recommendation: a
// shell built on this package is expected to set the field, and the choice is
// its own.
func (r *Runner) sem() Semantics {
	if r.Semantics != nil {
		return *r.Semantics
	}
	return BashSemantics()
}
