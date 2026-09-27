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
func (r *Runner) SetShortFormBodyIsOneCommandOrNone(on bool) {
	d := r.dialect()
	if d.ShortFormBody == on {
		return
	}
	d.ShortFormBody = on
	r.Dialect = &d
}
