// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// Which commands other than the simple ones fire the DEBUG trap.
//
// Every column that has the condition fires it before a simple command, and
// this engine fired it there and nowhere else — with a comment at the firing
// site saying a compound heading fires nothing, which is measured to be wrong
// in all four of the columns that have a DEBUG trap at all.
//
// Measured 2026-09-13 with `trap 'echo D $LINENO' DEBUG` over scripts of one
// construct per line, each construct read off the count of D lines and the
// lines they named:
//
//	construct              bash 5.3 / as sh / 3.2   ksh93               zsh
//	for w in a b           the head on every pass   the head per pass   once
//	for w in               nothing, it never runs   nothing             once
//	select                 the head once            the head per pass   once
//	case                   the head once            once                once
//	[[ … ]]                the head once            once                once
//	(( … ))                the head once            once                once
//	for ((i;c;n))          three parts per round    the written ones    once
//	if / while / until     nothing                  nothing             once
//	{ … } / ( … )          nothing                  nothing             once
//	f() { … } (defining)   nothing                  nothing             once
//	repeat                 no such construct        no such construct   once
//
// So the panel gives three readings rather than a rule with exceptions, and
// which one a dialect takes cannot be derived from anything else it answers.
// dash and BusyBox ash have no DEBUG condition and never reach the question.
//
// Two of ksh93's three departures from the bash columns are modeled here and
// the third is recorded rather than modeled: a `for` or `select` head that
// fires again on a later pass names **wherever the line record has got to** —
// the body's last line — where the bash columns name the head's own line on
// every pass. Reproducing that would mean a line rule that is right for the
// list loops and wrong for the arithmetic one, which names the head's line on
// every pass in ksh93 too.

// DebugTrapHeads is which compound commands fire the DEBUG trap.
type DebugTrapHeads int

const (
	// DebugTrapHeadsWordAndArithmetic fires at the heads whose own work is
	// expansion or arithmetic rather than a command: `case`, `[[`, `((` and
	// `select` once each, the list `for` on every pass, and each of the
	// arithmetic `for`'s three expressions every time one is evaluated,
	// whether the script wrote that expression or not. Nothing else. bash
	// 5.3, that build invoked as `sh`, and bash 3.2, so it is the zero
	// value.
	//
	// `for` firing per pass where `select` fires once is measured and not a
	// slip: a two-reply `select` writes one head and two bodies, and a
	// two-item `for` writes two heads and two bodies.
	DebugTrapHeadsWordAndArithmetic DebugTrapHeads = iota
	// DebugTrapHeadsEveryPassAndWrittenParts fires at the same heads, and
	// parts from the bash reading in two measured ways: **every** loop head
	// that fires at all fires once per pass, the menu loop's included, and
	// an arithmetic `for`'s initializer or step fires nothing when the
	// script did not write one. Its condition fires whether it was written
	// or not, which is what keeps `for ((;;))` writing one head per round
	// there and two in the bash columns. ksh93.
	DebugTrapHeadsEveryPassAndWrittenParts
	// DebugTrapHeadsEveryCompound fires once at the head of every command
	// that is not a simple one, a loop's passes included in that once and a
	// function definition among the heads: zsh.
	DebugTrapHeadsEveryCompound
	// DebugTrapHeadsNone fires at simple commands alone. No column answers
	// it — it is what a dialect with no DEBUG condition would say if it were
	// ever asked, and it is what this engine did before the question was
	// one.
	DebugTrapHeadsNone
)

func (d DebugTrapHeads) String() string {
	switch d {
	case DebugTrapHeadsEveryPassAndWrittenParts:
		return "DebugTrapHeadsEveryPassAndWrittenParts"
	case DebugTrapHeadsEveryCompound:
		return "DebugTrapHeadsEveryCompound"
	case DebugTrapHeadsNone:
		return "DebugTrapHeadsNone"
	}
	return "DebugTrapHeadsWordAndArithmetic"
}

// headsPerPass reports whether a loop head repeats with the loop's passes,
// which is the two readings that count passes at all.
func (d DebugTrapHeads) headsPerPass() bool {
	return d == DebugTrapHeadsWordAndArithmetic || d == DebugTrapHeadsEveryPassAndWrittenParts
}

// debugCompoundHead fires the trap for a command that is not a simple one,
// where this dialect's reading says a head fires at all.
//
// One site rather than one per clause, because the readings divide the node
// kinds between them and a rule spread over a dozen handlers is a rule that
// drifts. The heads that fire *per pass* rather than once are the exception
// and fire from inside their own loops — see debugPass.
func (r *Runner) debugCompoundHead(ctx context.Context, c syntax.Command) {
	switch heads := r.sem().DebugTrapCompoundHeads; heads {
	case DebugTrapHeadsNone:
		return
	case DebugTrapHeadsEveryCompound:
		if _, simple := c.(*syntax.SimpleCmd); simple {
			// A simple command fires from its own site, which is where the
			// measurements put it: before anything about the command is
			// expanded.
			return
		}
	default:
		switch c.(type) {
		case *syntax.CaseClause, *syntax.TestClause, *syntax.ArithCmdClause:
		case *syntax.SelectClause:
			if heads == DebugTrapHeadsEveryPassAndWrittenParts {
				// A menu loop's head is one of the per-pass heads in this
				// reading, so it fires from inside the loop instead — see
				// selectClause.
				return
			}
		default:
			return
		}
	}
	r.recordRunning(c, WholeCommand)
	r.runDebugTrap(ctx)
}

// debugPass fires the trap at a head that fires more than once — the list
// `for`'s head on every pass, the menu loop's in the reading that counts it,
// and each evaluation of an arithmetic `for`'s three expressions — naming the
// head's own line rather than wherever the body has got to.
//
// The head's own position is what the line record is moved to, because a pass
// after the first fires with that record sitting on the body's last command.
// It is put back afterward, so nothing about the body's own reporting moves.
func (r *Runner) debugPass(ctx context.Context, c syntax.Command) {
	r.debugPassOf(ctx, c, WholeCommand)
}

// debugPassOf is debugPass with the part of the command spelled out, for the
// arithmetic loop whose three expressions each fire on their own — see
// CommandPart. A head is the whole of its command for this, which is what the
// caller above passes.
func (r *Runner) debugPassOf(ctx context.Context, c syntax.Command, part CommandPart) {
	if !r.sem().DebugTrapCompoundHeads.headsPerPass() {
		return
	}
	r.recordRunning(c, part)
	line := r.line
	r.line = r.lineOf(c.Pos())
	r.runDebugTrap(ctx)
	r.line = line
}

// debugSelectPass fires a menu loop's head for the one reading that repeats
// it with the replies. The other two fire it once, from the dispatcher.
func (r *Runner) debugSelectPass(ctx context.Context, c *syntax.SelectClause) {
	if r.sem().DebugTrapCompoundHeads != DebugTrapHeadsEveryPassAndWrittenParts {
		return
	}
	r.debugPass(ctx, c)
}

// debugArithPart fires the trap for the arithmetic `for`'s initializer or its
// step, which one reading counts only where the script wrote one.
//
// The condition is not one of these: it fires under both readings whether it
// was written or not, so it calls debugPass directly. Measured — `for ((;;))`
// writes one head per round in the reading that skips the unwritten parts and
// two in the one that does not, and both of those counts are the same for
// every combination of the three parts a script can leave out.
func (r *Runner) debugArithPart(ctx context.Context, c *syntax.ForArithClause, part CommandPart, written bool) {
	if !written && r.sem().DebugTrapCompoundHeads == DebugTrapHeadsEveryPassAndWrittenParts {
		return
	}
	r.debugPassOf(ctx, c, part)
}

// Where a *pipeline* fires the DEBUG trap, which the heads table above says
// nothing about: a pipeline is neither a simple command nor one of the
// compound heads it enumerates, and this engine fired nothing for one at all
// — a traced script skipped every `cmd | cmd` it had (#2797).
//
// Measured 2026-09-14, `env -i PATH=/usr/bin:/bin`, counting the firings
// through a **file**, because an element's own firing writes down the pipe
// and a count taken from the terminal loses it. `trap 'echo x >>d' DEBUG`
// over `true | false`, then `trap - DEBUG`, which fires once itself:
//
//	pipeline                bash 5.3 / as sh / 3.2   ksh93   zsh
//	true | false            2                        2       1
//	true | false | true     3                        3       1
//	true | false | t | f    4                        4       1
//
// So the panel divides two ways on the count, and a third question — *where*
// the firing happens — divides it again. `trap 'echo d' DEBUG; echo a | tr
// a-z A-Z` writes `d d A` in the bash columns, `d D A` in ksh93 and `d A` in
// zsh: the uppercase `D` is an action whose output went **down the pipe**, so
// ksh93 fires inside each element with that element's redirections already in
// place, where the bash columns fire in the shell running the pipeline before
// any element starts. `trap 'n=$((n+1))' DEBUG; true | true` agrees from the
// other side — bash counts 2 in `$n` afterwards and ksh93 counts 1, the one
// being the last element, which that shell runs in the current shell anyway.
//
// A fourth measurement says the bash reading is narrower than "one per
// element": an element that is **not a simple command** fires nothing there,
// even when it is a head the dialect would otherwise fire for. `case x in x)
// echo hi;; esac | cat` writes one firing in the bash columns — the `cat` —
// where the same `case` standing alone writes a head, and `[[ 1 == 1 ]] |
// cat`, `for w in a b; do echo $w; done | cat` and `{ echo A; } | cat` all do
// the same. ksh93 writes the head, because the head fires wherever the
// element runs and ksh93 carries the trap into it.
//
// zsh's single firing is not a head in the sense above either. It stands for
// the pipeline as a *statement*, and the element's own firing is gone rather
// than moved: `echo A | { cat; }` writes one firing for the pipeline and one
// for the `cat` **inside** the group, and `( echo A; echo B ) | sed …` writes
// one for the pipeline and one for each `echo`. So commands nested inside an
// element go on firing normally and only the element's own head is withheld.
// zsh's `ZSH_DEBUG_CMD` at that firing reads the whole pipeline back — `echo
// A | sed "s/^/P:/"` — which is the shape a dialect implementing that
// parameter would have to record; nothing in this tree has one, so the
// firing records nothing. See RunningCommand.
//
// dash and BusyBox ash have no DEBUG condition and never reach the question.

// DebugTrapPipeline is how a pipeline fires the DEBUG trap.
type DebugTrapPipeline int

const (
	// DebugTrapPipelineInEachElement gives a pipeline no rule of its own:
	// each element fires wherever it runs, which is inside the element, with
	// that element's redirections in place. So it is reached only by a
	// dialect that carries the trap into a subshell, and a dialect that does
	// not fires nothing at all. ksh93, and the zero value because it is the
	// absence of a pipeline rule rather than a reading of one.
	DebugTrapPipelineInEachElement DebugTrapPipeline = iota
	// DebugTrapPipelinePerSimpleElement fires once for each element that is
	// a **simple command**, in the shell running the pipeline, before any
	// element starts — so the action's output goes to the shell's own stream
	// and what it assigns survives the pipeline. An element that is not a
	// simple command fires nothing, whatever head the dialect would fire for
	// it elsewhere. bash 5.3, that build invoked as `sh`, and bash 3.2.
	DebugTrapPipelinePerSimpleElement
	// DebugTrapPipelineOnceForThePipeline fires once, for the pipeline as a
	// statement, in the shell running it — and no element fires a head of
	// its own, however many there are and whatever they are. Commands nested
	// *inside* an element are commands in their own right and fire as usual.
	// zsh.
	DebugTrapPipelineOnceForThePipeline
)

func (d DebugTrapPipeline) String() string {
	switch d {
	case DebugTrapPipelinePerSimpleElement:
		return "DebugTrapPipelinePerSimpleElement"
	case DebugTrapPipelineOnceForThePipeline:
		return "DebugTrapPipelineOnceForThePipeline"
	}
	return "DebugTrapPipelineInEachElement"
}

// debugPipeline fires whatever the dialect's reading gives a pipeline of more
// than one element, before any of them starts.
//
// skip is per element and says that element must not run: a DEBUG action
// whose *status* refuses the firing costs that element alone, and the rest of
// the pipeline goes on. Measured under `shopt -s extdebug` on bash 5.3.15 —
// an action refusing `echo A` of `echo A | /bin/sh -c 'echo B'` still writes
// `B`, and one refusing the second element writes nothing while the first
// still runs. The refused element leaves **no** entry in the pipeline's
// status record either: `false | /bin/sh -c 'echo B; exit 7'` with the last
// element refused answers `st=1 ps=1`, one status rather than two.
//
// quiet says the elements fire nothing of their own, which is the other half
// of both readings that fire here: bash's element firing is *moved* into the
// shell and zsh's is withheld, and either way a dialect that carries the trap
// into a subshell would otherwise fire it twice. It is false for the reading
// that fires nothing here.
//
// armed says an `&&`/`||` list has already made the one firing that stands
// for this pipeline, so there is none to make here — and the elements are
// quiet all the same, because what the list's firing stood in for is the
// pipeline's whole statement. See Semantics.DebugTrapSublists.
func (r *Runner) debugPipeline(ctx context.Context, p *syntax.Pipeline, armed bool) (skip []bool, quiet bool) {
	if armed {
		return nil, true
	}
	switch r.sem().DebugTrapPipelines {
	case DebugTrapPipelineOnceForThePipeline:
		// One firing for the whole statement. Nothing is recorded as the
		// running command: the record holds a syntax.Command and a pipeline
		// is not one, and the only parameter in the panel that would read it
		// here is a parameter this tree does not have.
		r.runDebugTrap(ctx)
		if r.debugTrapSkipped() {
			// The refusal is the pipeline's, so it costs every element.
			skip = make([]bool, len(p.Cmds))
			for i := range skip {
				skip[i] = true
			}
		}
		return skip, true
	case DebugTrapPipelinePerSimpleElement:
		// Each element fires **at its own line**, which is the pipeline's
		// firing and not the element's own dispatch: the element has not been
		// reached yet, so the line record still sits on whatever ran before
		// the pipeline. Measured 2026-09-23 against bash 5.3.15 in the pinned
		// image, a `DEBUG` trap printing `$LINENO` over `echo a` on line 4 and
		// `echo b | cat | cat` on line 5: `4 5 5 5` there, `4 4 4 4` here —
		// and with the three elements written on lines 5, 6 and 7, `4 5 6 7`
		// there against `4 4 4 4` here. Put back afterwards, because the
		// pipeline's own statement has not moved (#4155).
		saved := r.line
		defer func() { r.line = saved }()
		for i, c := range p.Cmds {
			if _, simple := c.(*syntax.SimpleCmd); !simple {
				continue
			}
			r.recordRunning(c, WholeCommand)
			r.line = r.commandLine(c)
			r.runDebugTrap(ctx)
			if r.debugTrapSkipped() {
				if skip == nil {
					skip = make([]bool, len(p.Cmds))
				}
				skip[i] = true
			}
			if r.ctl != controlNone {
				// An action that unwound has said more than a skip does,
				// and the elements after it are not fired for either.
				break
			}
		}
		return skip, true
	}
	return nil, false
}

// And where an `&&`/`||` list fires it, which is the layer outside the
// pipeline and the last one a statement has.
//
// Measured 2026-09-25 from script files, one construct per line, with
// `trap 'print "T@$LINENO <$ZSH_DEBUG_CMD>"' DEBUG` under zsh 5.9.2 `-f`,
// `trap 'echo "T@$LINENO <$BASH_COMMAND>"' DEBUG` under bash 5.3.20
// `--noprofile --norc`, and `${.sh.command}` under ksh93 AJM 93u+ 2012-08-01:
//
//	line                       bash 5.3.20   ksh93   zsh 5.9.2
//	echo x && echo y           2 firings     2       1
//	false || echo z            2             2       1
//	echo w && echo v && echo u 3             3       1
//	echo q && { echo r; echo s } 3           3       1 + one per inner command
//	if echo p && echo o; …     3             3       1 head + 1 cond + 1 body
//
// bash and ksh93 were byte-identical over that file, naming the operand each
// firing stood for. zsh writes one firing whose `$ZSH_DEBUG_CMD` is the
// **whole sublist** read back — `print q && {\n\tprint r\n\tprint s\n}` for
// the fourth row — so the firing stands for the list and not for its first
// operand.
//
// The noun is the **sublist** and not the line, which is the pair that holds
// it fixed: `print a; print b` on one line writes **two** firings, and `print
// c &&` with `print d` on the next line writes **one**. A grid that varied
// only the operator would have agreed with either reading.
//
// Two consequences the count alone does not carry, both measured:
//
//   - An operand that is a **compound** fires no head of its own. `print t &&
//     if true; then print u; fi` writes one firing for the list, then the
//     `if`'s condition and body fire as they always do — where the same `if`
//     standing alone writes a head first. So the sublist's firing stands in
//     for the head as well as for the operands, exactly as
//     [DebugTrapPipelineOnceForThePipeline] stands in for an element's.
//   - Commands **inside** an operand are commands in their own right. `print
//     q && { print r && print s }` writes two firings, the list's and the
//     nested list's, and a function called from an operand fires its body's
//     lists at their own offsets.
//
// dash and BusyBox ash never reach the question: both refuse `trap … DEBUG`
// outright — `trap: DEBUG: bad trap` from dash, `trap: line 1: DEBUG: invalid
// signal specification` from BusyBox v1.37.0 in the pinned alpine image, both
// measured 2026-09-25 beside an EXIT trap that fired, so the instrument that
// reported the refusal was one that can fire.
//
// The reading is orthogonal to [Semantics.DebugTrapRunsBeforeTheCommand],
// which is placement rather than count: with `unsetopt DEBUG_BEFORE_CMD` the
// same file writes the same **number** of firings and only moves each one
// behind what it preceded. Measured the same day — `print x && print y` on
// line 3 writes one firing there in both states of the option.

// DebugTrapSublist is how an `&&`/`||` list fires the DEBUG trap.
type DebugTrapSublist int

const (
	// DebugTrapSublistPerOperand gives a list no rule of its own: each
	// operand fires whatever it would have fired standing alone, so a
	// three-operand list of simple commands writes three firings and a list
	// whose operand is a compound writes that compound's head. bash 5.3,
	// that build invoked as `sh`, bash 3.2 and ksh93 — and the zero value,
	// because it is the absence of a list rule rather than a reading of one.
	//
	// It is also what dash and BusyBox ash hold, which is not a measurement
	// of those shells: neither has a DEBUG condition to fire, so neither is
	// ever asked. There is no Unspecified here for the same reason
	// [DebugTrapPipeline] has none — a list either fires once or fires per
	// operand, and there is no third thing for a shell to mean.
	DebugTrapSublistPerOperand DebugTrapSublist = iota
	// DebugTrapSublistOnceForTheList fires once, for the list as a
	// statement, before any operand runs — and no operand fires anything of
	// its own, neither a simple command's firing nor a pipeline's nor a
	// compound's head. Commands nested *inside* an operand are commands in
	// their own right and fire as usual. zsh.
	DebugTrapSublistOnceForTheList
)

func (d DebugTrapSublist) String() string {
	if d == DebugTrapSublistOnceForTheList {
		return "DebugTrapSublistOnceForTheList"
	}
	return "DebugTrapSublistPerOperand"
}

// sublistExpr runs a statement's whole expression, firing the DEBUG trap once
// for it first in the dialect that reads an `&&`/`||` list that way.
//
// The entry point is separate from [Runner.expr] rather than a flag inside
// it, because "the outermost operator of this statement" is the whole of the
// question and the tree already says it: expr recurses on its own operands,
// so a nested list reached through a group, a subshell or a function body
// comes back through here and fires its own. A flag would have had to be
// unset at every door a body is entered by.
//
// A list of one pipeline is not fired for here. Its firing is the pipeline's
// or the command's, which is where it already happens and where the panel
// puts it — see debugPipeline and Runner.simple.
func (r *Runner) sublistExpr(ctx context.Context, e syntax.Expr) error {
	b, chain := e.(*syntax.BinaryExpr)
	if !chain || r.sem().DebugTrapSublists != DebugTrapSublistOnceForTheList {
		return r.expr(ctx, e)
	}
	// The firing stands behind the whole list in the dialect state that
	// fires behind the command rather than ahead of it, so it needs a
	// holding slot of its own — outside every operand's, which is what puts
	// it after everything they flushed. Measured on zsh 5.9.2 with `unsetopt
	// DEBUG_BEFORE_CMD`: `print q && { print r; print s }` writes the two
	// inner firings behind their own commands and the list's last of all.
	// Installed only where a firing is actually made here, so a statement
	// that is not a list pays no closure for it.
	if r.debugTrapRunsBehindTheCommand() {
		defer r.debugAfterScope(ctx)()
	}
	// At the list's own line, which is where the reference puts it: `print c
	// &&` on line 3 with `print d` on line 4 names 3. Taken here because the
	// firing stands ahead of every operand, so nothing has moved the line
	// record onto one yet — the record still sits on whatever ran before the
	// statement. Put back afterwards, because the first operand's dispatch
	// sets it for itself.
	//
	// Nothing is recorded as the running command: the record holds a
	// syntax.Command and a list is not one. What the reference's own
	// `ZSH_DEBUG_CMD` reads back at this firing is the whole list's text,
	// which is the shape a dialect implementing that parameter would have to
	// record; nothing in this tree has one.
	before := len(r.debugHeld)
	line := r.line
	r.line = r.lineOf(b.Pos())
	r.runDebugTrap(ctx)
	r.line = line
	held := -1
	if len(r.debugHeld) > before {
		held = before
	}
	if r.debugTrapStopped() {
		// The refusal is the list's, so it costs every operand — the same
		// reach the pipeline's single firing has.
		return nil
	}
	// Saved rather than cleared on the way out: a list nested inside one of
	// these operands comes back through here and would otherwise hand the
	// enclosing list's remaining operands back their own firings, and take
	// the enclosing list's own line with it.
	saved, savedLine := r.sublistFired, r.sublistLine
	r.sublistFired, r.sublistLine = true, 0
	defer func() { r.sublistFired, r.sublistLine = saved, savedLine }()
	err := r.expr(ctx, e)
	if held >= 0 && held < len(r.debugHeld) && r.sublistLine != 0 {
		// The **last operand's** own line, which is what a held firing
		// names once the list has run — the one place the two readings are
		// not a mirror of each other. Ahead of the list it names the line
		// the list starts on; behind it, it names the line of the last
		// operand that ran, whatever the operands in between did to the
		// line record. Measured on zsh 5.9.2 with `unsetopt
		// DEBUG_BEFORE_CMD`, 2026-09-25, a list written over three lines:
		// `print a &&` on 3, `print b &&` on 4 and `print c` on 5 names 5,
		// and the same list short-circuiting at the `false` on line 3 names
		// 3. A compound operand names its own head's line and not its
		// body's last — `print a && {` on line 3 with a body on 4 names 3,
		// and a group written whole on line 4 names 4.
		r.debugHeld[held].line = r.sublistLine
	}
	return err
}

// armSublistOperand arms the operand about to run to withhold its own DEBUG
// firing, where the list it belongs to has already fired for the whole of it,
// and records that operand's own line for a firing the list is holding.
//
// Read at the arming rather than at the flush, because the line record has
// moved on by then: an operand that is a compound leaves it on the body's
// last command and the firing names the compound's own head.
func (r *Runner) armSublistOperand(e syntax.Expr) {
	if r.sublistFired {
		r.sublistOperand = true
		r.sublistLine = r.lineOf(e.Pos())
	}
}
