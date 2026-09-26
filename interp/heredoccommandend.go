// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// Where a here-document body's failure is located when the redirection is
// written on a compound command spelled with a **reserved word**.
//
// One column puts it at the line the whole command **ended** on rather than
// at the line the command began on — which is a line the reader had already
// gone past by the time the body ran, and is what a shell whose counter is
// simply "where the input has got to" reports for a construct that does not
// set it back. Measured 2026-09-26 with `-c` against `/opt/homebrew/bin/bash`
// 5.3.20 (`go version -m` says *not a Go executable*) and `cmd/bash` built
// under its own name. Each program has the command on line 1 with `$(( 1/0 ))`
// as its here-document body on line 2, the delimiter on line 3, and
// `echo done` under it:
//
//	command                        bash   before
//	{ :; } <<END                      3        1
//	while read x; do :; done <<END    3        1
//	until :; do :; done <<END         3        1
//	if :; then :; fi <<END            3        1
//	for i in 1; do :; done <<END      3        1
//	case x in x) :;; esac <<END       3        1
//	select i in a; do break; done     3        1
//	( : ) <<END                       1        1
//	[[ x = x ]] <<END                 1        1
//	(( 1 )) <<END                     1        1
//	: <<END                           1        1
//	cat <<END                         1        1
//	f <<END                           1        1
//
// The last six are the controls, and they are why this is keyed on the
// *spelling* rather than on "a compound command": a `( … )`, a `[[ … ]]` and
// an arithmetic command are all compound and all report the line they began
// on, exactly as a simple command, a builtin and a function call do.
//
// **The number is the end of the whole command and not the delimiter's
// line**, which four shapes separate. With the group on line 1 and the
// pipeline continued below its here-document, bash names the line the
// *pipeline* ends on; with two here-documents it names the second
// delimiter's; and inside an `if`, it names the `fi`:
//
//	program                                            bash
//	{ :; } <<END |⏎ <body> ⏎ END ⏎ cat                    4
//	{ :; } <<A <<B ⏎ a1 ⏎ A ⏎ <body> ⏎ B                  5
//	if :; then ⏎ { :; } <<END ⏎ <body> ⏎ END ⏎ fi         5
//	the same with two `echo` lines before the `fi`        7
//	for i in 1 ⏎ do : ⏎ done <<END ⏎ <body> ⏎ END         5
//
// So it is the greater of where the command's own text ends and where its
// last here-document closed, which is exactly "where the reader had got to".
//
// Read by Runner.applyRedirs, which is where the line a redirection's failure
// carries is already chosen. See
// Diagnostics.HeredocBodyOnAReservedWordCompoundIsLocatedWhereTheCommandEnds
// (#4690, #4712).

// reservedWordCompoundEndLine is that line: how far the reader had got when
// the command holding these redirections was complete.
//
// [Runner.inputUnitLine] is the first half — the line the whole logical unit
// ends on, which the front end's loop already records for the give-up rule —
// and it does not count a here-document's body, because a redirection's own
// extent stops at the delimiter word. The bodies are the second half, and
// they are taken from the list in hand rather than from a walk of the tree:
// the list *is* this command's redirections.
//
// Nought where neither half has a line, which is the answer that leaves the
// line where the caller had already put it.
func (r *Runner) reservedWordCompoundEndLine(rs []*syntax.Redirect) int {
	if r.inFunc != "" {
		// **A function body is left exactly alone**, and that is a limit of
		// this rule rather than a case of it. A body is not read where it
		// runs, so the reader's position in force inside one is the
		// *caller's* — taking it named the line of the call, which is
		// nobody's answer. The column this is measured on names the line
		// the function's **body** begins on there instead, which is a rule
		// of its own and is not measured into this one: with the group on
		// the function's second line and its delimiter three below that,
		// bash 5.3.20 names 1 for a definition on line 1, 3 for the same
		// definition on line 3, 2 for `f()` on line 1 with its `{` on line
		// 2, and 3 for a group two commands into a body opened on line 3 —
		// never the delimiter's line and never the call's.
		//
		// Nought, which is the answer that leaves the line where the caller
		// had already put it: the command's own, which is what this shell
		// reported before this rule existed and still reports in there.
		return 0
	}
	line := r.inputUnitLine
	for _, rd := range rs {
		if rd.Heredoc == nil {
			continue
		}
		// The delimiter's own line, which is one below where the body's
		// word ends: the word runs past the delimiter line's newline, so
		// its end is the line after it.
		if n := r.lineOf(rd.Heredoc.End()) - 1; n > line {
			line = n
		}
	}
	return line
}

// heredocBodyOnAReservedWordCompound reports whether the redirections being
// applied belong to such a command, and whether this dialect moves the line
// for one.
//
// The spelling is carried rather than derived, because the runner's
// redirection code is handed a list and an owner and never the command: see
// [Runner.bracketedCompoundRedirs], which the three bracketed spellings set
// and [Runner.applyRedirs] consumes.
func (r *Runner) heredocBodyOnAReservedWordCompound(compound, bracketed bool, owner redirOwner) bool {
	return compound && !bracketed && owner != redirOwnerASubshell &&
		r.diag().HeredocBodyOnAReservedWordCompoundIsLocatedWhereTheCommandEnds
}

// withRedirsOfABracketedCommand is [Runner.withRedirs] for a compound command
// spelled with brackets rather than with a reserved word: `[[ … ]]`,
// `(( … ))` and the anonymous function's `() { … }`. A `( … )` is the fourth
// and says so already, through its owner.
//
// The flag is **consumed** by applyRedirs rather than restored here, so that
// a command written inside one of these bodies does not inherit it: an
// anonymous function has a body, and the group in it is spelled with a
// reserved word.
func (r *Runner) withRedirsOfABracketedCommand(ctx context.Context, rs []*syntax.Redirect, body func() error) error {
	r.bracketedCompoundRedirs = true
	return r.withRedirs(ctx, rs, body)
}
