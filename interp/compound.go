// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// control is how break, continue and return leave a construct without
// unwinding the whole interpreter. They are not errors: a `break` that reaches
// the top is a misuse, but a `break` inside a loop is ordinary control flow,
// and modeling it as an error would make every caller check for something
// that is not a failure.
type control uint8

const (
	controlNone control = iota
	controlBreak
	controlContinue
	controlReturn
	// controlExit is a fatal error: the shell abandons the script. Nothing
	// consumes it — not loopControl, not the function-call site — so it
	// unwinds past every construct to Run, which is exactly what "fatal"
	// means and what break, continue and return each deliberately are not.
	controlExit
	// controlAbandon gives up the statement being run and goes on to the
	// next one. It unwinds like controlExit — past loops, functions, groups
	// and subshells alike — and is consumed at the top-level statement loop
	// rather than at Run, which is the whole of the difference.
	//
	// Measured, and it is a third thing rather than a shade of the other
	// two. bash refuses an assignment to a readonly name, reports it, and
	// abandons what it was running:
	//
	//	readonly r=1
	//	for i in 1 2; do r=2; echo one; done     the loop stops, `one` never prints
	//	echo two                                 and this runs
	//
	// The same in a function body, an `if`, a group and a subshell: every
	// shape gives up at the top-level statement boundary and the shell
	// carries on at the next one. Neither fatal nor survivable, which is
	// why neither of the existing two could express it.
	controlAbandon
)

// condList runs a list whose status is being *tested* rather than required to
// succeed, so `set -e` does not fire inside it.
//
// The suppression is a counter on the runner, so it reaches whatever the
// condition calls: a function invoked from an `if` has it suppressed all the
// way down. That is measured and unanimous, and it is the part of `set -e`
// most implementations get wrong.
func (r *Runner) condList(ctx context.Context, list []*syntax.Stmt) error {
	r.tested++
	defer func() { r.tested-- }()
	return r.runList(ctx, list)
}

// runList executes a list of statements, stopping early if one of them
// transferred control.
func (r *Runner) runList(ctx context.Context, list []*syntax.Stmt) error {
	for _, st := range list {
		if err := r.stmt(ctx, st); err != nil {
			return err
		}
		if r.ctl != controlNone {
			return nil
		}
	}
	// The end of a list is a command boundary too, and it is the last one a
	// subshell has: a handler runs *between* commands, so an element whose
	// final write broke its own pipe would set a handler for exactly that and
	// never reach one. See runSelfRaisedTraps for what the panel does.
	//
	// Here rather than where the subshell's body is started, which is the
	// difference between running the handler inside the element's
	// redirections and running it after they have been taken down. Measured
	// with the element's standard error sent to a file: dash, bash 5.3, ksh93
	// and zsh all put the handler's output in that file.
	r.runSelfRaisedTraps(ctx)
	return nil
}

func (r *Runner) group(ctx context.Context, c *syntax.Group) error {
	// A brace group runs in *this* shell, so its assignments escape. That is
	// the whole difference between it and a subshell.
	return r.withRedirs(ctx, c.Redirs, func() error { return r.runList(ctx, c.List) })
}

func (r *Runner) subshell(ctx context.Context, c *syntax.Subshell) error {
	// A subshell gets a copy of the state, so nothing it does escapes. This
	// is a copy rather than a forked process, which is honest for everything
	// the corpus asks and would not be for a background job or a trap; those
	// are not here yet.
	return r.withRedirs(ctx, c.Redirs, func() error {
		sub := r.clone()
		sub.inheritJobs(jobBoundaryCompound)
		err := sub.runList(ctx, c.List)
		// The status and what produced it travel together: a subshell whose
		// last command a signal killed is a signal death out here too, and
		// a pipeline substituting the status has to know that.
		r.status, r.diedOfSig = sub.status, sub.diedOfSig
		return err
	})
}

func (r *Runner) ifClause(ctx context.Context, c *syntax.IfClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// The condition is a *list* judged by its last command, which is why
		// this runs the whole thing and then looks at the status.
		if err := r.condList(ctx, c.Cond); err != nil {
			return err
		}
		if r.status == 0 {
			return r.runList(ctx, c.Then)
		}
		for _, e := range c.Elifs {
			if err := r.condList(ctx, e.Cond); err != nil {
				return err
			}
			if r.status == 0 {
				return r.runList(ctx, e.Then)
			}
		}
		if c.HasElse {
			return r.runList(ctx, c.Else)
		}
		// No branch ran, so the `if` itself succeeded.
		r.status = 0
		return nil
	})
}

func (r *Runner) loop(ctx context.Context, c *syntax.LoopClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// What the loop will report, kept rather than read off the runner at
		// the end: the condition runs once more after the final iteration
		// and overwrites the live status with its own.
		//
		// Zero means a loop whose body never ran, which exits 0 whatever
		// preceded it, and after that it is the body's last status. Both
		// halves are one rule and neither can be dropped: `while [ $i -lt
		// 1 ]; do i=1; true; done` is 0 even though the condition that
		// ended it was *false*, so it is the body being reported and not
		// the condition, and `false; while false; do :; done` is 0 rather
		// than 1. Unanimous across the panel, and POSIX says the same.
		//
		// It is a local and not `r.status = 0` up here, because the live
		// status belongs to the last command that ran until this loop has
		// something of its own to say: the condition can *see* it —
		// `false; while [ $? -eq 0 ]; do …` does not run in any shell in
		// the panel, and resetting first made it run in ours.
		defer r.enteringLoop()()
		body := 0
		for {
			if err := r.condList(ctx, c.Cond); err != nil {
				return err
			}
			if r.ctl != controlNone {
				// The condition gave up rather than answering — an
				// interrupt at the prompt, a refused assignment — so there
				// is no answer to read and the status it left is the loop's.
				// Reading it as a condition instead put the loop's own
				// bookkeeping over the top of it, so `while :; do …; done`
				// interrupted reported 0 where the same interrupt in a `for`
				// reported 130.
				return nil
			}
			done := r.status == 0
			if c.Until {
				done = !done
			}
			if !done {
				r.status = body
				return nil
			}
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			body = r.status
			if stop := r.loopControl(); stop {
				return nil
			}
		}
	})
}

func (r *Runner) forClause(ctx context.Context, c *syntax.ForClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// An absent word list iterates the positional parameters; an empty
		// one iterates nothing. HasItems is what tells them apart, and a nil
		// slice could not.
		var items []string
		r.beginHeading()
		if c.HasItems {
			for _, w := range c.Items {
				items = append(items, r.expandWord(w)...)
			}
		} else {
			items = r.Params
		}
		if r.failedHeading() {
			// The word list is what the loop iterates, so a failure in it
			// costs the loop rather than one pass of it: the body must not
			// run over a word the shell has just said it could not read.
			// It ran three times for `for i in a "$((1/0))" b` (#1215).
			return nil
		}

		// The same two questions the conditional loops answer, and the same
		// local for the same reason: what the loop reports is 0 until its
		// body has run and the body's last status afterwards, while `$?`
		// inside the body is still the last command's until the body sets
		// one. `false; for i in a b; do echo $?; done` prints 1 and then 0
		// in every shell in the panel — the first iteration sees what
		// preceded the loop — and a reset written up here printed 0 twice.
		defer r.enteringLoop()()
		body := 0
		// The name count is the stride: a loop with two names takes two words
		// on every pass, which is zsh's `for key value ( a 1 b 2 )`. One name
		// is every other shell and every other loop, and the arithmetic is
		// the same for it.
		//
		// Guarded rather than assumed, because a Runner can be handed a tree
		// nobody parsed and a stride of zero is an endless loop rather than a
		// wrong answer.
		stride := len(c.Names)
		if stride == 0 {
			return nil
		}
		for i := 0; i < len(items); i += stride {
			// A final pass with fewer words than names leaves the names it
			// did not reach **empty rather than unset** — measured 2026-09-06
			// in zsh 5.9.2, `for a b ( 1 2 3 ) { … }` reads `[3][]` on its
			// second pass and `${b-U}` is `[]` there, not `U`. The body still
			// runs for that pass; an empty list runs it no times at all.
			for j, name := range c.Names {
				it := ""
				if i+j < len(items) {
					it = items[i+j]
				}
				r.setVar(name, it)
			}
			// After the assignments, because zsh traces the assignments
			// themselves — one line per name, measured `a=1` then `b=2` then
			// the body — and before the body, because bash's header is the
			// line that introduces the iteration.
			r.traceForNames(c.Header, c.Names, items, i)
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			body = r.status
			if stop := r.loopControl(); stop {
				return nil
			}
		}
		r.status = body
		return nil
	})
}

// forArithClause runs `for ((init; cond; post))`.
//
// It iterates on a condition rather than over a list, which is why it is a
// separate clause: the list form knows how many times it will run before it
// starts, and this one does not.
//
// An omitted condition is *true*, not false. `for ((;;))` is the endless loop
// every shell writes it as, and treating a missing expression as zero would
// have made it run no times at all — the quietest possible way to get this
// wrong.
//
// A redirection on it covers the whole loop, as it does on every other
// compound command: `for ((…)); do echo $i; done > f` puts every iteration in
// the file, unanimously among the four shells that have the construct. It
// wanted saying twice — the node had no place to keep one, so the parser left
// the operator where it stood and the *next* statement redirected nothing into
// the file, which created it empty and sent the loop's output to the terminal.
func (r *Runner) forArithClause(ctx context.Context, c *syntax.ForArithClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		r.status = 0
		// Each part is resolved from its text, because a part containing an
		// expansion has no tree until it runs — and the condition and the step
		// are resolved *again* every time round, since what they expand to may
		// have changed since the last one.
		//
		// Which is why the heading is cleared here and each part checks: a
		// part whose *expansion* failed leaves text the arithmetic then
		// cannot parse either, so `for (( i=$((1/0)); … ))` reported the
		// division and then a second complaint about the `i=` that was left
		// (#1215).
		r.beginHeading()
		if _, ok := r.forArithPart(c.Init, c.InitText); !ok {
			return nil
		}
		defer r.enteringLoop()()
		for {
			if c.Cond != nil || c.CondText != "" {
				v, ok := r.forArithPart(c.Cond, c.CondText)
				if !ok {
					return nil
				}
				if v == 0 {
					return nil
				}
			}
			r.traceForIteration(c.Header, "", "")
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			if stop := r.loopControl(); stop {
				return nil
			}
			if _, ok := r.forArithPart(c.Post, c.PostText); !ok {
				return nil
			}
		}
	})
}

// forArithPart evaluates one of the three parts of `for (( ; ; ))`.
//
// An absent part needs no special case. Its value is only ever read for the
// condition, and the caller asks about that only when there is one — which is
// what makes `for ((;;))` endless rather than a loop that never runs.
func (r *Runner) forArithPart(tree syntax.ArithExpr, text string) (int, bool) {
	resolved, perr := r.arithTree(tree, text)
	if r.failedHeading() {
		// The expansion inside the part failed. Its diagnostic is written and
		// what is left of the text is not an expression, so parsing on would
		// complain a second time about a residue the script never wrote.
		return 0, false
	}
	if perr != nil {
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		r.status = 1
		return 0, false
	}
	v, err := r.evalArith(resolved)
	if err != nil {
		r.diagf("%v\n", err)
		r.status = 1
		return 0, false
	}
	return v, true
}

// loopControl consumes a break or continue aimed at this loop, reporting
// whether the loop should stop.
func (r *Runner) loopControl() bool {
	switch r.ctl {
	case controlBreak:
		r.ctl = controlNone
		if r.ctlDepth > 1 {
			// An outer loop is the target, so the break carries on outwards.
			r.ctlDepth--
			r.ctl = controlBreak
			return true
		}
		return true
	case controlContinue:
		r.ctl = controlNone
		if r.ctlDepth > 1 {
			r.ctlDepth--
			r.ctl = controlContinue
			return true
		}
		return false
	case controlReturn, controlExit, controlAbandon:
		// `exit` ends the loop as surely as `return` does, and neither is
		// cleared here: the caller above has to see it too. Abandoning a
		// statement ends it for the same reason and is cleared in the same
		// one place, which is the top-level statement loop.
		//
		// Naming controlAbandon here is a fast exit rather than the thing
		// that stops the loop: without it the rounds still run and do
		// nothing, because stmt refuses once control flow is set. Measured
		// by mutation — the behavior is identical either way, and the
		// difference is whether a `for i in 1 2 3` spins twice for nothing.
		//
		// Leaving `exit` out was not a missing case so much as an invisible
		// one. The loop carried on, the next round found the shell refusing
		// to run anything, and `while` then fell out of its condition and set
		// the status to 0 — so `exit 3` from inside one exited 0. A `for`
		// over a finite list happened to come out right, which is why this
		// survived: the shape most scripts use hid it.
		//
		// For `exit` it is *not* only a fast exit, and which loop that is
		// true of was measured rather than assumed (#894). Dropping
		// controlExit from this case and taking a goroutine dump: `while`
		// and `until` both still finish, because loop refuses another round
		// the moment control flow is set whatever this returns, and a `for`
		// over a list runs out of list. `for ((;;))` does neither — it has no
		// guard of its own and no end of its own — so this line is the only
		// thing that stops it, and without it the shell spins in
		// forArithPart evaluating the step for ever. That is why
		// TestExitLeavesALoop runs each of its cases under a bound: the
		// arithmetic one cannot fail, it can only hang.
		return true
	}
	return false
}

func (r *Runner) caseClause(ctx context.Context, c *syntax.CaseClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		// The subject expands but is neither field-split nor globbed, even
		// unquoted — the same exemption `[[ ]]` operands and a scalar
		// assignment value have, unanimous across the panel: a subject
		// holding only a tab still reaches [[:blank:]], `g='*'; case $g`
		// matches a literal star and never the directory listing, and a
		// value with a space in it stays one subject. The ordinary word
		// pipeline split the tab to zero fields, so the arm never fired —
		// silently, status 0.
		r.beginHeading()
		subject := strings.Join(r.expandWordNoSplit(c.Word), "")
		if r.failedHeading() {
			// Before any arm is tested, because a subject that failed is
			// empty and empty *matches*: the `""` arm fired and the shell
			// chose a branch from a value it could not compute (#1215).
			return nil
		}
		// A case matching nothing exits 0.
		r.status = 0
		r.unspecified = false

		for i, item := range c.Items {
			matched := r.caseItemMatches(item, subject)
			if r.ctl != controlNone {
				// A pattern the dialect rejects outright. Testing the later
				// items would report it again, once per item.
				return nil
			}
			if r.unspecified {
				// A pattern asked an axis no dialect answered. Falling
				// through to the next item would run a body chosen by a
				// guess, and reporting the refusal while running anyway is
				// the failure this whole structure exists to avoid.
				r.status = 2
				return nil
			}
			if !matched {
				continue
			}
			if err := r.runList(ctx, item.Body); err != nil {
				return err
			}
			// The matched body has run; from here the terminators drive,
			// and every arm reached is honored in turn. `;;` stops. `;&`
			// runs the next body without testing its pattern. `;;&` keeps
			// testing the *later* patterns, which is the narrower thing it
			// means and the reason it is a separate operator rather than a
			// spelling of `;&`. Honoring only the matched arm's terminator
			// ran one extra body and stopped, so a three-link `;&` chain
			// dropped its third body and a `;&` into a `;;&` never went
			// back to matching.
			at := i
			for {
				switch c.Items[at].Term {
				case syntax.TokSemiAmp:
					at++
					if at == len(c.Items) {
						return nil
					}
				case syntax.TokDSemiAmp:
					at++
					for at < len(c.Items) && !r.caseItemMatches(c.Items[at], subject) {
						at++
					}
					if at == len(c.Items) {
						return nil
					}
				default:
					return nil
				}
				if err := r.runList(ctx, c.Items[at].Body); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *Runner) caseItemMatches(item *syntax.CaseItem, subject string) bool {
	for _, p := range item.Patterns {
		// A pattern is a word: unquoted it is a pattern, quoted a literal,
		// and only the spans still know which.
		if r.matchPatternR(r.patternOf(p), subject, false) {
			return true
		}
	}
	return false
}

// isPlainFuncName reports a name POSIX would call one — the shape every
// dialect defines without a word.
func isPlainFuncName(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return s != ""
}

func (r *Runner) funcDecl(c *syntax.FuncDecl) error {
	// A name the grammar admitted may still be one this dialect refuses to
	// define: ksh93 parses `f-g()` and stops the script at the definition,
	// with a different sentence for a dot — a discipline function is its own
	// concept there and `a.b` does not name one.
	if !isPlainFuncName(c.Name) &&
		r.ask(r.sem().PunctuatedFunctionNameIsRefused, "a function name carrying punctuation being refused") {
		wording, fallback := r.diag().FunctionNameInvalid, "%[1]s: invalid function name"
		if strings.ContainsRune(c.Name, '.') && r.diag().FunctionNameDiscipline != "" {
			wording, fallback = r.diag().FunctionNameDiscipline, "%[1]s: invalid discipline function"
		}
		r.fatal("%s\n", Wording(wording, fallback, c.Name))
		return nil
	}
	if r.unspecified {
		r.status = 2
		return nil
	}
	if r.funcs == nil {
		r.funcs = map[string]*syntax.FuncDecl{}
	}
	r.funcs[c.Name] = c
	// And whether the dialect defined it or the script did, which decides
	// whose voice its diagnostics carry — see prelude.go.
	r.preludeDefined(c.Name, c)
	// Where it was defined, which is the file its frame reports — a function
	// declared in a sourced library and called from the script names the
	// library, not the script.
	if r.funcFiles == nil {
		r.funcFiles = map[string]string{}
	}
	r.funcFiles[c.Name] = r.currentFile()
	r.status = 0
	return nil
}

// callFunc runs a function body with the arguments as its positional
// parameters.
//
// The parameters are saved and restored rather than copied into a new runner,
// because a function shares the shell's variables — the scoping is dynamic,
// and `local` is what carves out an exception. `local` is not here yet.
func (r *Runner) callFunc(ctx context.Context, fn *syntax.FuncDecl, args []string) error {
	if r.depth >= maxDepth {
		r.diagf("%s: too deeply nested\n", fn.Name)
		r.status = 1
		return nil
	}
	saved, savedIn, savedLine := r.Params, r.inFunc, r.funcLine
	r.Params, r.inFunc = args, fn.Name
	// A function the dialect's prelude defined is the shell speaking rather
	// than the script, so what it reports is named after it and located where
	// it was called. The outermost such call owns both: a prelude helper it
	// calls in turn adds nothing, because the script named the outer one.
	savedSpeaker, savedSpeakerLine := r.speaker, r.speakerLine
	if r.speaker == "" && r.speaksForTheShell(fn) {
		r.speaker, r.speakerLine = fn.Name, r.line
	}
	defer func() { r.speaker, r.speakerLine = savedSpeaker, savedSpeakerLine }()
	// Where the function was written, so a dialect that numbers a message
	// from the function rather than from the file can subtract it.
	r.funcLine = fn.Pos().Line
	r.pushFrame(Frame{File: r.funcFiles[fn.Name], Name: fn.Name})
	defer r.popFrame()
	// This call's own serial, because the RETURN trap fires for the one
	// function whose body set it and for nobody else — not a caller, and
	// not a sibling entered after it returned.
	frameSerial := r.currentFrameSerial()
	r.depth++
	// A scope the function's locals unwind into.
	sc := &scope{saved: map[string]string{}, existed: map[string]bool{}, keyword: fn.Keyword}
	r.scopes = append(r.scopes, sc)
	// What the EXIT trap was on the way in, so zsh can tell whether this
	// function set one of its own.
	outerTrap, outerDepth := r.exitTrap, r.trapDepth

	err := r.command(ctx, fn.Body)

	r.depth--
	// Put back what `local` displaced, in whatever order it was declared:
	// the values are keyed by name, so order does not matter.
	for name, old := range sc.saved {
		if sc.existed[name] {
			r.Vars[name] = old
		} else {
			delete(r.Vars, name)
		}
		// And the name stops being one a *declaration* gave its value to,
		// which is measured rather than tidiness: the shell that reads a
		// declaration as setting the name leaves the caller's an empty
		// export once a function has declared a local of it, where before
		// the declaration it was told to no child at all. Putting the
		// record back instead kept it silent, which no shell does.
		delete(r.declaredEmpty, name)
	}
	for name, old := range sc.savedArrays {
		if sc.arrayExisted[name] {
			r.Arrays[name] = old
		} else {
			delete(r.Arrays, name)
		}
	}
	for name, old := range sc.savedAssoc {
		if sc.assocExisted[name] {
			r.AssocArrays[name] = old
		} else {
			delete(r.AssocArrays, name)
		}
	}
	// And the frozen attribute, where a declaration shadowed a readonly: the
	// outer name is frozen again, so a function cannot thaw one for good.
	for name := range sc.savedReadonly {
		if r.readonly == nil {
			r.readonly = map[string]bool{}
		}
		r.readonly[name] = true
	}
	// And the export attribute, where the dialect took it off for the local:
	// the outer name goes back to whatever the shell had recorded about it,
	// including having recorded nothing.
	for name, spoken := range sc.exportedSpoken {
		if spoken {
			r.exported[name] = sc.savedExported[name]
		} else {
			delete(r.exported, name)
		}
	}
	// And whether `unset` had hidden the name, which a hiding `local` set
	// for the function's duration: put back what was true at the shadow.
	for name, was := range sc.removedBefore {
		if was {
			r.removed[name] = true
		} else {
			delete(r.removed, name)
		}
	}
	// And whatever a dialect asked to have run when this call unwinds, in
	// reverse order of registration, before the scope is dropped: the last
	// thing registered is the innermost, the same order a defer stack has.
	for i := len(sc.onReturn) - 1; i >= 0; i-- {
		sc.onReturn[i]()
	}
	r.scopes = r.scopes[:len(r.scopes)-1]
	// zsh runs an EXIT trap set *inside* a function when the function
	// returns, and then forgets it; the other three keep it for the end of
	// the script. Only a trap this call installed counts, which is what the
	// depth records — an inherited one is the caller's business.
	if r.exitTrap != nil && r.exitTrap != outerTrap && r.trapDepth == r.depth+1 &&
		r.ask(r.sem().ExitTrapIsFunctionLocal, "an EXIT trap set in a function firing when it returns") {
		body := *r.exitTrap
		r.exitTrap, r.trapDepth = outerTrap, outerDepth
		ctl := r.ctl
		r.ctl = controlNone
		r.runTrapBody(ctx, body)
		if r.ctl == controlNone {
			r.ctl = ctl
		}
	}
	r.Params, r.inFunc, r.funcLine = saved, savedIn, savedLine
	// The RETURN trap, if this call's own body set one. After the locals
	// and parameters are back — the action runs in the caller — and before
	// controlReturn is cleared, so an explicit `return` still fires it.
	r.runReturnTrap(ctx, frameSerial)
	if r.ctl == controlReturn {
		r.ctl = controlNone
	}
	return err
}
