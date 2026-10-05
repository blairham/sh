// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Which spellings a loop's body may be written in is a question about the
// **grammar**, so it lives on syntax.Dialect and not on Semantics — and one
// dialect spells half of it as an option a running script switches, which is
// why a runner has to be able to move it.
//
// That is the same combination [Runner.SetDoubledQuoteInSingleQuotes],
// [Runner.SetCasePatternListReadAsOneWord] and [Runner.SetBarePatternGroups]
// have, and the same consequences follow: the answer that decides is the one
// in force when the text is parsed, the dialect is copied and replaced rather
// than written through, and the front end's run loop reads the rest of the
// program with whatever it finds.
//
// # Half of the family and not the whole of it
//
// The option reaches the body written as **one command** and the body
// **omitted altogether**, and leaves the brace-spelled body, the
// parenthesized item list and the redundant `fi` exactly where they were.
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, run `-f` over a script file under `set -n`,
// 2026-09-27, with the option moved on the line before:
//
//	                          on       off
//	while (( 0 )) :           parses   refused
//	while (( 0 )) { :; }      parses   parses
//	for i (a b) echo $i       parses   refused
//	for i (a b) { echo $i; }  parses   parses
//	while true                parses   refused
//
// So a runner that wrote the whole family from this one name would refuse
// four spellings the shell takes with the option off, which is what
// [syntax.Dialect.ShortFormBody] exists to keep apart.
//
// # And the `function` keyword's body, which is the same question elsewhere
//
// The same name decides whether the `function` keyword's body may be written
// as one command or left out, and there the answer is sharper than "one
// command": with the option off, **only a brace group** will do. Measured in
// the same run, the option moved on the line after an `emulate`:
//
//	                                    on       off
//	function a; { echo B; }             parses   parses
//	function a { echo B; }              parses   parses
//	function a                          parses   refused
//	function a; echo B                  parses   refused
//	function a; ( echo B )              parses   refused
//	function a; if true; then :; fi     parses   refused
//	f() echo hi                         parses   parses
//
// The last row is the control and it is the one that says this is the
// keyword's question: the parenthesized spelling keeps its one-command body
// in every state, so a runner that wrote both from this name would refuse a
// definition the shell takes. The first row is the other control: the
// separator survives, which is why
// [syntax.Dialect.FunctionKeywordSeparatorBeforeBody] is a field of its own.

// ShortFormBodyIsOneCommandOrNone reports whether a body standing where
// `do … done` would may be written as a single command, or left out
// altogether.
//
// It says nothing about the brace-spelled body, which the dialect answers on
// its own and no option here moves.
func (r *Runner) ShortFormBodyIsOneCommandOrNone() bool {
	return r.lang().ShortFormBody
}

// SetShortFormBodyIsOneCommandOrNone moves it, for a dialect whose option
// namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
// SetShortRepeatBody moves syntax.Dialect.ShortRepeatBody, for the dialect
// whose `shortrepeat` names it.
func (r *Runner) SetShortRepeatBody(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().ShortRepeatBody == on {
		return
	}
	d := r.dialect()
	d.ShortRepeatBody = on
	r.Dialect = &d
}

// ShortRepeatBody reports it.
func (r *Runner) ShortRepeatBody() bool { return r.lang().ShortRepeatBody }

func (r *Runner) SetShortFormBodyIsOneCommandOrNone(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().ShortFormBody == on {
		return
	}
	d := r.dialect()
	d.ShortFormBody = on
	r.Dialect = &d
}

// FunctionKeywordBodyIsOneCommandOrNone reports whether the `function`
// keyword's body may be written as a single command, or left out altogether.
//
// It is two fields of syntax.Dialect rather than one because the shell states
// the narrow answer twice over: with it off the body is refused unless it is a
// brace group, and a declaration with no body at all is refused as well. They
// are written through one setter because they are one measured answer — see
// the table above.
func (r *Runner) FunctionKeywordBodyIsOneCommandOrNone() bool {
	// One axis per lang call — see TestNothingReadsAnAdjustedAxisOffLang.
	return r.lang().FunctionKeywordBodyIsOptional &&
		!r.lang().FunctionKeywordBodyMustBeBraceGroup
}

// SetFunctionKeywordBodyIsOneCommandOrNone moves it, for a dialect whose
// option namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
func (r *Runner) SetFunctionKeywordBodyIsOneCommandOrNone(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().FunctionKeywordBodyIsOptional == on && r.lang().FunctionKeywordBodyMustBeBraceGroup == !on {
		return
	}
	d := r.dialect()
	d.FunctionKeywordBodyIsOptional = on
	d.FunctionKeywordBodyMustBeBraceGroup = !on
	r.Dialect = &d
}

// LoopBodyEndsInEnd reports whether a loop's body that is neither `do … done`
// nor a brace group is a list closed by `end`. See
// syntax.Dialect.LoopBodyEndsInEnd.
func (r *Runner) LoopBodyEndsInEnd() bool { return r.lang().LoopBodyEndsInEnd }

// SetLoopBodyEndsInEnd moves it, for a dialect whose option namespace has a
// name for the reading — zsh's `cshjunkieloops`. Copied and replaced for the
// reason SetShortFormBodyIsOneCommandOrNone is.
func (r *Runner) SetLoopBodyEndsInEnd(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().LoopBodyEndsInEnd == on {
		return
	}
	d := r.dialect()
	d.LoopBodyEndsInEnd = on
	r.Dialect = &d
}

// QuotedNewlineIsUnmatched reports the grammar flag of that name, which one
// dialect's `cshjunkiequotes` turns on and off at run time. See
// syntax.Dialect.QuotedNewlineIsUnmatched.
func (r *Runner) QuotedNewlineIsUnmatched() bool { return r.lang().QuotedNewlineIsUnmatched }

// SetQuotedNewlineIsUnmatched sets that flag for what this runner reads from
// here on, the way SetLoopBodyEndsInEnd does for its own.
func (r *Runner) SetQuotedNewlineIsUnmatched(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().QuotedNewlineIsUnmatched == on {
		return
	}
	d := r.dialect()
	d.QuotedNewlineIsUnmatched = on
	r.Dialect = &d
}
