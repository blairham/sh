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
func (r *Runner) debugPipeline(ctx context.Context, p *syntax.Pipeline) (skip []bool, quiet bool) {
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
