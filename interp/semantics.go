// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Answer is one axis's value, and it has three states rather than two.
//
// Unspecified is the point of the type. The vector contains *only* the places
// the shells disagree — everything they agree about never became an axis and
// is simply the implementation — so leaving one unset is not an oversight, it
// is saying "no shell has been chosen here". A script that depends on such an
// axis is refused, naming it, rather than silently getting one shell's answer.
type Answer uint8

const (
	// Unspecified refuses the behaviour rather than guessing at it.
	Unspecified Answer = iota
	Yes
	No
)

func (a Answer) String() string {
	switch a {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "unspecified"
}

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
	SplitParamExpansion Answer
	// SplitCommandSubstitution field-splits an unquoted command
	// substitution. True everywhere measured, including zsh.
	SplitCommandSubstitution Answer

	// GlobExpansionResults matches the *result* of an expansion against the
	// filesystem. False in zsh, where only a pattern written literally in the
	// source is expanded. The same rule decides whether `[[ abc == $p ]]`
	// treats $p as a pattern, which is one behaviour observed twice rather
	// than two quirks.
	GlobExpansionResults Answer
	// GlobNoMatchIsError makes a pattern matching nothing an error instead of
	// passing it through. True only in zsh.
	GlobNoMatchIsError Answer

	// AssignmentPrefixPersistsOnSpecialBuiltin keeps `x=1 shift` set
	// afterwards. POSIX requires it; dash and ksh93 comply and bash and zsh
	// do not.
	AssignmentPrefixPersistsOnSpecialBuiltin Answer

	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes Answer

	// LengthOfSpecialIsCount makes `${#@}` the number of positional
	// parameters. False in dash, which gives the length of the joined
	// string. The first axis measured where dash stands alone, and a silent
	// one: both answers are plausible numbers.
	LengthOfSpecialIsCount Answer

	// ArithLeadingZeroIsOctal reads `0100` as sixty-four. False in zsh, where
	// it is one hundred. The quietest divergence measured — nothing warns,
	// both are plausible numbers, and file modes are written this way.
	ArithLeadingZeroIsOctal Answer
	// ArithFloat evaluates floating point. True in ksh93 and zsh, where POSIX
	// says integers only.
	ArithFloat Answer

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string. True in bash alone; ksh93 and zsh keep it a regex, so
	// quoting a regex is unportable in either direction.
	RegexQuotingMakesLiteral Answer

	// LastPipelineElementInCurrentShell runs the last command of a pipeline
	// in this shell, so `echo x | read v` sets v. True in ksh93 and zsh.
	LastPipelineElementInCurrentShell Answer

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off
	// the end. True in dash and ksh93.
	ShiftPastEndFatal Answer
	// ReadonlyReassignmentFatal ends the script when a readonly variable is
	// assigned. True everywhere but bash, measured with a plain assignment in
	// a script file — adding a redirect makes it a command and reverses the
	// answer, which is the contaminated-probe trap docs/spec/oracle.md
	// records.
	ReadonlyReassignmentFatal Answer

	// ArrayBaseIsZero indexes arrays from 0. True in bash and ksh93, false in
	// zsh, which counts from 1. dash has no arrays at all, which is why the
	// axis is absent rather than false there.
	ArrayBaseIsZero Answer

	// DollarZeroInFunctionIsFunctionName makes `$0` inside a function the
	// function's name. True only in zsh.
	DollarZeroInFunctionIsFunctionName Answer
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
		SplitParamExpansion:                      Yes,
		SplitCommandSubstitution:                 Yes,
		GlobExpansionResults:                     Yes,
		GlobNoMatchIsError:                       No,
		AssignmentPrefixPersistsOnSpecialBuiltin: Yes,
		EchoInterpretsEscapes:                    No,
		LengthOfSpecialIsCount:                   Yes,
		ArithLeadingZeroIsOctal:                  Yes,
		ArithFloat:                               No,
		RegexQuotingMakesLiteral:                 No,
		LastPipelineElementInCurrentShell:        No,
		ShiftPastEndFatal:                        Yes,
		ReadonlyReassignmentFatal:                Yes,
		ArrayBaseIsZero:                          Yes,
		DollarZeroInFunctionIsFunctionName:       No,
	}
}

// CoreSemantics fixes the axes every shell in the core panel agrees on and
// leaves the rest unspecified.
//
// It is the counterpart of syntax.Core(), built the same way: that refuses
// constructs not every shell has, and this refuses *behaviours* not every
// shell shares. A script that runs under it depends on nothing the panel
// disagrees about, which makes it a portability check rather than a runtime —
// the same role docs/spec/core.md gave strict POSIX.
//
// Only two of the fourteen axes survive, and that is not a defect of the
// panel. The axes exist because they diverge; everything shells agree about
// never became one.
func CoreSemantics() Semantics {
	return Semantics{
		SplitCommandSubstitution: Yes,
		LengthOfSpecialIsCount:   Yes,
	}
}

// BashSemantics is bash's answers.
func BashSemantics() Semantics {
	s := PosixSemantics()
	s.AssignmentPrefixPersistsOnSpecialBuiltin = No
	s.ReadonlyReassignmentFatal = No
	s.ShiftPastEndFatal = No
	s.RegexQuotingMakesLiteral = Yes
	return s
}

// ZshSemantics is zsh's, and is the one that shows why this is a vector: it
// differs from bash on seven axes and agrees with dash on one of them.
func ZshSemantics() Semantics {
	s := BashSemantics()
	s.SplitParamExpansion = No
	s.GlobExpansionResults = No
	s.GlobNoMatchIsError = Yes
	s.EchoInterpretsEscapes = Yes
	s.ArithLeadingZeroIsOctal = No
	s.ArithFloat = Yes
	s.RegexQuotingMakesLiteral = No
	s.LastPipelineElementInCurrentShell = Yes
	s.DollarZeroInFunctionIsFunctionName = Yes
	s.ArrayBaseIsZero = No
	return s
}

// KshSemantics is ksh93's.
func KshSemantics() Semantics {
	s := PosixSemantics()
	s.ArithFloat = Yes
	s.LastPipelineElementInCurrentShell = Yes
	return s
}

// DashSemantics is dash's.
func DashSemantics() Semantics {
	s := PosixSemantics()
	s.EchoInterpretsEscapes = Yes
	s.LengthOfSpecialIsCount = No
	return s
}

// sem returns the runner's semantics, defaulting to the core.
//
// Defaulting to a *shell* would be the substrate answering a question that is
// not its to answer. Defaulting to the core answers it honestly: what every
// shell agrees on is done, and anything else is refused until something above
// chooses. A shell built on this package sets the field; that is its job.
func (r *Runner) sem() Semantics {
	if r.Semantics != nil {
		return *r.Semantics
	}
	return CoreSemantics()
}

// ask reads one axis.
//
// An unspecified axis is refused rather than guessed, and the refusal names
// it, because "this script depends on something the shells disagree about"
// is a useful thing to be told and a silent wrong answer is not.
//
// Callers consult an axis only when the input actually depends on it — `echo
// hi` does not ask about escapes and `echo 'a\tb'` does — which is what keeps
// the core usable rather than refusing everything.
func (r *Runner) ask(a Answer, axis string) bool {
	switch a {
	case Yes:
		return true
	case No:
		return false
	}
	r.errf("sh: %s: the shells disagree here and no dialect was chosen\n", axis)
	r.status = 2
	r.unspecified = true
	return false
}
