// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// When a command's assignment prefix is worked through against when a
// **declaration utility's operand** is expanded.
//
// A third position in the sequence interp/prefixredirorder.go already halves:
// that file is the prefix against the command's redirections, and this is the
// prefix against the word the utility reads as an assignment. The two are not
// the same question and they do not answer alike — zsh opens the redirections
// before it touches the prefix and still works through the prefix before the
// operand, which no single ordering of the three can express.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, stdin on /dev/null. Both substitutions write to standard
// error, so the order of the two lines is the order they were expanded in:
//
//	PRE=$(echo PRE >&2) export s=$(echo OP >&2)
//
//	bash 5.3.20        	`OP` then `PRE`
//	bash 3.2.57        	`OP` then `PRE`
//	ksh93u+ 2012-08-01 	`PRE` then `OP`
//	zsh 5.9.2          	`PRE` then `OP`
//	dash 0.5.12        	`OP` then `PRE`
//	BusyBox ash 1.37.0 	`OP` then `PRE`
//
// So four columns reach the operand first and two reach the prefix first.
//
// **The control, because the obvious reading is wrong.** This is not "a prefix
// is worked through before the words" in general: an ordinary command's
// argument is expanded before its prefix in every column, including the two
// above. Measured the same day and the same way:
//
//	PRE=$(echo PRE >&2) : s=$(echo OP >&2)	`OP` then `PRE`, all six
//
// What moves is the operand of a declaration utility — the word the utility
// reads as an assignment rather than as an argument — which is why the
// question is asked where the operand is recognized and nowhere else.
//
// **It is not gated on the utility being special either**, which is the other
// obvious reading and is what separates this from
// PrefixExpandedBeforeTheRedirections's third answer: `export` is a special
// builtin and `typeset` is not, and ksh93 writes `PRE` first for both. The
// array-literal spelling of the operand answers identically —
// `PRE=$(echo PRE >&2) typeset a=($(echo OP >&2))` is `OP` then `PRE` in bash
// and `PRE` then `OP` in ksh93 and zsh — so the axis is about the operand and
// not about which body the parentheses hold.
//
// And two prefix entries stay in written order on either side of the split:
// `P1=$(…) P2=$(…) export s=$(…)` is `OP P1 P2` in bash and dash and
// `P1 P2 OP` in ksh93 and zsh (#3814).

// prefixBeforeADeclarationsOperand works through this command's assignment
// prefix ahead of the operand, in the two columns that do it that way.
//
// Asked only where a prefix is written and the command has an operand for it
// to be about, which is what keeps `x=1 cmd` and a bare `export s=1` from
// asking anything at all.
//
// The values are put aside rather than applied: the store still runs where it
// always ran, and reads them back through Runner.prefixExpansion, so nothing
// here expands a value twice. What moves is when the substitution in it runs.
//
// Reports whether the command may carry on — an unanswered axis is refused
// like any other.
func (r *Runner) prefixBeforeADeclarationsOperand(assigns []*syntax.Assign) bool {
	if !aPrefixIsWritten(assigns) {
		return true
	}
	if !r.ask(r.sem().PrefixExpandedBeforeADeclarationsOperand,
		"a command's assignment prefix against a declaration utility's operand") {
		// Either the column that reaches the operand first, or no column at
		// all: r.unspecified separates them and is what the caller stops on.
		return !r.unspecified
	}
	// The same walk the trace makes, and deliberately the same one: it skips
	// a frozen name and a refused subscripted word, so a prefix this shell
	// never evaluates is not evaluated here either, and it records each value
	// where the store and the trace both read it back from.
	r.expandPrefixTraceValues(assigns)
	return true
}
