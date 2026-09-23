// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// Where a function definition's own refusals are located.
//
// A definition is not a simple command, so the shell that reports one of its
// refusals has no command line to point at and points at its own counter
// instead. That counter is not the definition, and it is not one thing either:
// measured across 49 shapes it is the innermost of three, and every one of the
// three is reachable. See [Runner.functionDefinitionRefusalLine].

// enterLineConstruct moves the register a function definition's refusals read
// to this construct's own first line, and gives back the call that puts it
// where it was.
//
// `for`, `case` and `select` and no others: `if`, `while`, `until`, `{ }`,
// `( )` and a `;` or `&&` list were each measured **not** to move it, which is
// what makes this a list rather than "every compound command". See
// Runner.functionDefinitionRefusalLine for the rows.
func (r *Runner) enterLineConstruct(at syntax.Pos) func() {
	outer := r.constructLine
	r.constructLine = r.lineOf(at)
	return func() { r.constructLine = outer }
}

// functionDefinitionRefusalLine is the line a refusal raised by a function
// **definition** is reported at, in the dialect that locates one where its own
// counter stands rather than at the definition — see
// Diagnostics.FunctionDefinitionRefusalIsLocatedWhereTheShellWasReading.
//
// Three terms, innermost first, and each was measured on shapes the other two
// cannot explain. bash 5.3.20, 2026-09-23, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, over all three refusals a definition raises —
// a special builtin's name in POSIX mode, a redefinition of a frozen name, and
// a name that is not a name — which answer identically, so this is the
// definition command and not one message's mistake.
//
//  1. **A `for`, `case` or `select` this frame is running**, at its own first
//     line, innermost first:
//
//     for i in 1; do ⏎ break() { :; } ⏎ done      the `for`'s line
//     for i in 1; do ⏎ echo p ⏎ break() … ⏎ done  the `for`'s line still
//     for … do ⏎ if true; then ⏎ break() … ⏎ fi ⏎ done   the `for`'s
//     for … do ⏎ for … do ⏎ break() … ⏎ done ⏎ done      the inner `for`'s
//     case x in ⏎ x) ⏎ break() { :; };; ⏎ esac    the `case`'s line
//     case … ⏎ x) ⏎ for … do ⏎ break() … ⏎ done   the `for`'s, being inner
//     select x in a; do ⏎ break() … ⏎ done        the `select`'s line
//
//     `while` and `until` are the controls and take term 2, at any depth; so
//     does a `( )` inside a `for`, which is why the register does not cross a
//     subshell. A loop that has **finished** takes term 2 again.
//
//  2. **The top-level statement**, at its last line, where no call is in
//     progress:
//
//     break() { :; }                     that line
//     break() { ⏎ : ⏎ }                   the third — the definition's own end
//     break() { :; }; continue() { ⏎ : ⏎ } the third — the *statement's* end,
//     which the definition's own end cannot explain
//     if true; then ⏎ break() { :; } ⏎ fi the `fi`'s line
//     { ⏎ echo p ⏎ break() { :; } ⏎ }      the `}`'s line
//     ( ⏎ break() { :; } ⏎ )              the `)`'s line
//     echo p ⏎ break() { :; }             the definition's line — a statement
//     of its own
//     break() { :; }; echo a \\ ⏎ b \\ ⏎ c     the fourth: the unit the reader
//     took, which the statement's own extent cannot explain
//     ( ⏎ break() { :; } ⏎ ) inside a `for`  the `)`'s line, the parentheses
//     being a unit of their own
//
//  3. **The definition itself**, at its own *first* line, inside a call:
//
//     f() { ⏎ break() { :; } ⏎ }; f            the definition's line
//     f() { ⏎ echo p ⏎ break() { :; } ⏎ }; f   the definition's, not the echo's
//     f() { ⏎ break() { ⏎ : ⏎ } ⏎ }; f          the definition's *first* line,
//     which term 2 would have put at its last
//
// **The rule is this refusal's and not the shell's line numbering.** Every
// other runtime refusal measured in the same shapes already agrees with the
// reference here and is untouched: a reassignment to a readonly name inside a
// `for` on lines 2-4 is reported at 4 by both shells rather than at the loop's
// line, and so is a bad name to `export`. A simple command has a line of its
// own to be pointed at; a definition has not, which is the whole of why this
// exists.
func (r *Runner) functionDefinitionRefusalLine(c *syntax.FuncDecl) int {
	if n := r.constructLine; n != 0 {
		return n
	}
	if !r.insideFunctionCall() {
		if n := r.subshellLine; n != 0 {
			// Inside parentheses the reader's unit is the parentheses: a
			// definition refused in `for … do ⏎ ( ⏎ break() { :; } ⏎ ) ⏎ done`
			// is reported at the `)` and not at the `done`. Measured, and it
			// is the same fact that keeps the loop register from crossing the
			// boundary — see Runner.subshellLine.
			return n
		}
		if n := r.inputUnitLine; n != 0 {
			// The whole unit the reader took, which is not this statement of
			// it: `break() { :; }; continue() { ⏎ : ⏎ }` is reported at the
			// third line. See Runner.inputUnitLine.
			return n
		}
		if n := r.inputLine; n != 0 {
			return n
		}
		// Nothing is running at input level — a trap body, or a caller
		// driving Runner.stmt directly — so there is no statement to name and
		// the definition's own end is the best answer there is, which is what
		// this did everywhere before. The same fallback Runner.giveUpLine
		// takes, for the same reason.
		return r.lineOf(c.End())
	}
	return r.lineOf(c.Pos())
}
