// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"

	"github.com/blairham/sh/syntax"
)

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
//
// **And the operand sees what the prefix came to.** Working through the
// prefix first is not only an order of side effects: the two columns that do
// it have each value under its name by the time the operand expands. Measured
// 2026-10-29 from `-c` and from a script file under `env -i
// PATH=/usr/bin:/bin`, against ksh93u+ 2012-08-01, zsh 5.9.2, GNU bash 5.3.20,
// bash 3.2.57 and dash 0.5.12:
//
//	                                             ksh93u+, zsh   both bash, dash
//	v=old; v=new export y=$v                     y=new          y=old
//	unset v; v=new export y=$v                   y=new          y=
//
//	                                             ksh93u+, zsh   both bash
//	v=old; a=1 v=new b=2 typeset y=$v$a$b        y=new12        y=old
//	v=old; v=new typeset y=$v z=$v               y=new z=new    y=old z=old
//	v=old; v=new typeset -a a=($v)               a=new          a=old
//	v=old; v=new typeset y=$(echo "$v")          y=new          y=old
//	f() { v=new typeset y=$v; …; }; v=old; f     y=new          y=old
//
// So no column expands the prefix first and keeps it out of the operand's
// sight, and the question is the one this axis already asks rather than a
// second axis beside it: the columns that answer No have not expanded the
// prefix when the operand expands, so there is nothing for it to see. What
// the operand sees is held the way interp/prefixsees.go holds an entry for
// the entries behind it, and put back before the route stores the prefix for
// real, so the route's own scope and persistence are unchanged: zsh still
// gives `v` back as `old` afterwards and ksh93 still keeps `new`. An operand
// over the prefix's own name stands over it — `v=new typeset v=x y=$v` is
// `y=x` in ksh93u+, which stores its operands in turn, and `y=new` in zsh
// 5.9.2, which does not. A command substitution sees the held value too, and
// a child sees it where the name was already exported: `export v=old; v=new
// typeset y=$(/usr/bin/printenv v)` is `y=new` in zsh 5.9.2 (#6999).

// prefixBeforeADeclarationsOperand works through this command's assignment
// prefix ahead of the operand, in the two columns that do it that way.
//
// Asked only where a prefix is written and the command has an operand for it
// to be about, which is what keeps `x=1 cmd` and a bare `export s=1` from
// asking anything at all.
//
// Each value is held under its name in held, for the operands to see, and is
// not applied: the store still runs where it always ran, once the caller has
// put held back, and reads the values back through Runner.prefixExpansion, so
// nothing here expands a value twice. Called again for each operand, and an
// entry already expanded is neither expanded nor held a second time.
//
// argv is the command's words so far, which name the utility.
//
// Reports whether the command may carry on. It may not where an axis went
// unanswered, which r.unspecified says, or where a frozen entry was refused
// and the refusal gave the command up, which has set its own status.
func (r *Runner) prefixBeforeADeclarationsOperand(assigns []*syntax.Assign, argv []string, held *heldPrefix) bool {
	if !aPrefixIsWritten(assigns) {
		return true
	}
	if !r.ask(r.sem().PrefixExpandedBeforeADeclarationsOperand,
		"a command's assignment prefix against a declaration utility's operand") {
		// Either the column that reaches the operand first, or no column at
		// all: r.unspecified separates them and is what the caller stops on.
		return !r.unspecified
	}
	// Where the prefix is a local of the running function, it is held in the
	// local, so what the operands see is the body's own cell. See
	// interp/prefixdeclarationlocal.go.
	local, answered := r.prefixLocalDeclaration(assigns, argv)
	if !answered {
		return false
	}
	// **A frozen entry is refused where this walk reaches it**, so nothing
	// behind it, and no operand, is expanded. The early check's own walk and
	// its own refusal, asked here rather than behind the operands: it stops at
	// the frozen entry, writes the lines of the entries in front of it, and
	// costs the command what the early check costs it. Measured 2026-11-02
	// from `-c` and a script file under `env -i PATH=/usr/bin:/bin` against
	// ksh93u+ 2012-08-01, `readonly r=0` first:
	//
	//	r=3 export y=$(echo s >&2)               the refusal; no `s`
	//	v=1 r=3 typeset -a a=($(echo s >&2))     the same
	//	r=$(echo R >&2) export y=$(echo s >&2)   `R`, then the refusal
	//	r=3 v=$(echo V >&2) typeset y=$(…)       the refusal alone
	//	set -x; v=1 r=3 typeset y=1              `+ v=1`, then the refusal
	//
	// This expanded, stored and traced every operand first and refused the
	// entry behind them all (#7094). Where the early check does not refuse
	// in front of this command it says so and nothing has moved: zsh checks
	// the name with the command, and a prefix about to become the running
	// function's locals is a different cell (see above). The entries in front
	// of the refused one are left the way the early check leaves them, which
	// is #7004's question and not this one's.
	if local == "" && !r.prefixCheckedFirst && r.refusePrefixesEarly(assigns, argv) {
		return false
	}
	// Held with the export attribute the declaration's route gives each
	// entry, so a child the operand starts is handed what the route will:
	// `export v=old; v=new typeset y=$(printenv v)` is `y=` in ksh93u+,
	// which takes the attribute off at `typeset`, and `v=old; v=new export
	// y=$(printenv v)` is `y=new` there (#7003). See
	// interp/prefixheldexport.go.
	if !held.route.set {
		held.route = r.heldExportRouteOf(argv)
	}
	// The same walk the trace makes, and deliberately the same one: it skips
	// a frozen name and a refused subscripted word, so a prefix this shell
	// never evaluates is not evaluated here either, and it records each value
	// where the store and the trace both read it back from. The holding form,
	// because what it holds must outlive this call: the operand has not
	// expanded yet.
	r.expandPrefixTraceValuesHolding(assigns, held, local, r.prefixSubscriptsReadHere(assigns, argv))
	r.writeThePrefixAheadOfTheOperand(assigns, argv)
	return true
}

// writeThePrefixAheadOfTheOperand writes the prefix's lines where the walk
// above has just expanded it, ahead of the operand, in the column that writes
// one line per entry in front of the command.
//
// That column traces an operand as it stores it (see
// interp/declarationinturn.go), so the prefix's lines, written by the route
// that stores the prefix, used to land behind the operands'. ksh93 writes them
// where it works through the prefix. Measured 2026-10-30 against ksh93u+
// 2012-08-01 (`/bin/ksh`), script files under `env -i PATH=/usr/bin:/bin`
// (#7001):
//
//	set -x; v=new typeset y=1                  `+ v=new`, `+ y=1`, `+ typeset y`
//	set -x; v=new w=2 export y=1 z=2           `+ v=new`, `+ w=2`, `+ y=1`, `+ z=2`, …
//	set -x; v=1 typeset y=$(echo s >&2)        `+ v=1`, then the substitution's lines
//	set -x; v=1 typeset -a a=($(echo s >&2))   the same, for a literal operand
//	readonly r=1; set -x; v=1 r=2 typeset -a a=(1)
//	                                           `+ v=1`, then the refusal
//
// So the lines are the walk's and not the operand's: the substitution in a
// literal operand, which no walk stores in turn, is written behind them too.
//
// Each entry is marked as written, so the route behind does not write it a
// second time — the same list the frozen-name walk keeps, and read the same
// way. The writing stops at a frozen entry, which is refused and never
// written, and at an entry with no parent, whose refusal the route behind
// writes in place of every line after it.
func (r *Runner) writeThePrefixAheadOfTheOperand(assigns []*syntax.Assign, argv []string) {
	if !r.tracesEachPrefixEntryOnItsOwnLine() || !r.tracesItsPrefix(assigns, argv) ||
		r.tracePrefixFollowsTheCommand(assigns, argv) {
		return
	}
	d := r.diag()
	for _, a := range assigns {
		if a.Operand || slices.Contains(r.prefixTracedEarly, a) {
			continue
		}
		if r.attrHas(a.Name, attrReadonly) ||
			d.NoParent != "" && r.dottedNameHasNoParent(a.Name) {
			return
		}
		if _, ok := r.prefixTraceValue(a); !ok {
			continue
		}
		for _, w := range r.prefixTraceWords([]*syntax.Assign{a}, d) {
			r.awaitTraceTurn()
			r.traceLine(w, d)
			r.releaseTraceTurn()
		}
		r.prefixTracedEarly = append(r.prefixTracedEarly, a)
	}
}
