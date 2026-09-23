// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// commandLine is the line a command reports itself at, which is where it
// begins in five of the seven columns and where its **first word ends** in
// two.
//
// The split is a shell's line counter rather than a choice about locations:
// bash and dash name the line the reader had reached when the command's first
// word was complete, so a first word that spans lines moves the command down
// with it, while zsh and ksh93 name the line the command started on. Measured
// 2026-09-23, a script file with `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin
// from /dev/null, `echo one` on line one:
//
//	command                                    bash  dash  zsh  ksh93
//	`v=$(⏎:⏎) > /nonexistent/f` from line 2       4     4    2      2
//	`v="a⏎b" > /nonexistent/f` from line 2        3     3    2      2
//	`v=a\⏎b > /nonexistent/f` from line 2         3     3    2      2
//	`echo x "$(⏎:⏎)" > /nonexistent/f`            2     2    2      2
//	`nosuchcmd "$(⏎:⏎)"` from line 2              2     2    2      2
//
// The last two rows are the controls and are what says this is the **first**
// word and not the command: with the substitution in a later word, every
// column names the line the command's name is on, however far below the
// command runs on. The third row is what says it is not about substitutions —
// a backslash-newline inside the first word moves the line the same way.
//
// `v=$( … )` is the shape that matters and the reason this is here at all: the
// same number anchors a substitution body's own numbering, which is
// Runner.substRunBase.
//
// Not applied inside a substitution body, which is measured and is not an
// exception this package invented: in `v=$(⏎w=$(⏎:⏎) > /nonexistent/f⏎)` bash
// names the body's own first line rather than the line the inner `)` is on.
// The counter that runs a re-parsed body does not advance over a **nested**
// substitution's newlines, so the first word of a command in there ends where
// it began. A multi-line quoted word in the same position does move the line,
// which is why this is the flag and not a rule about bodies — see
// Runner.substRunBase for what is and is not held.
func (r *Runner) commandLine(c syntax.Command) int {
	if !r.diag().CommandIsLocatedWhereItsFirstWordEnds || r.inSubstBody {
		return r.lineOf(c.Pos())
	}
	x, simple := c.(*syntax.SimpleCmd)
	if !simple {
		return r.lineOf(c.Pos())
	}
	end, ok := firstWordEnd(x)
	if !ok {
		return r.lineOf(c.Pos())
	}
	return r.lineOf(end)
}

// firstWordEnd is where the first of a simple command's words ends, whichever
// class it belongs to.
//
// By position rather than by field, because the classes interleave: `a=1 cmd`
// and `cmd a=1` are both an assignment and an argument, and a precommand
// stands in front of either. Taking the assignment list first would name the
// wrong word for the second of those.
func firstWordEnd(c *syntax.SimpleCmd) (syntax.Pos, bool) {
	var at, end syntax.Pos
	found := false
	consider := func(pos, stop syntax.Pos) {
		if !found || pos.Offset < at.Offset {
			at, end, found = pos, stop, true
		}
	}
	for _, w := range c.Precommands {
		consider(w.Pos(), w.End())
	}
	for _, a := range c.Assigns {
		consider(a.Pos(), a.End())
	}
	for _, w := range c.Args {
		consider(w.Pos(), w.End())
	}
	return end, found
}
