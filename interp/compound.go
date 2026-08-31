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
// and modelling it as an error would make every caller check for something
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
		err := sub.runList(ctx, c.List)
		r.status = sub.status
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
		// Zero iterations exits 0, which is why the status is set before the
		// loop rather than left as whatever the condition produced.
		r.status = 0
		for {
			if err := r.condList(ctx, c.Cond); err != nil {
				return err
			}
			done := r.status == 0
			if c.Until {
				done = !done
			}
			if !done {
				r.status = 0
				return nil
			}
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
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
		if c.HasItems {
			for _, w := range c.Items {
				items = append(items, r.expandWord(w)...)
			}
		} else {
			items = r.Params
		}

		r.status = 0
		for _, it := range items {
			r.setVar(c.Name, it)
			if err := r.runList(ctx, c.Body); err != nil {
				return err
			}
			if stop := r.loopControl(); stop {
				return nil
			}
		}
		return nil
	})
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
	case controlReturn:
		return true
	}
	return false
}

func (r *Runner) caseClause(ctx context.Context, c *syntax.CaseClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		subject := strings.Join(r.expandWord(c.Word), " ")
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
			switch item.Term {
			case syntax.TokSemiAmp:
				// Fall through to the next body without testing its pattern.
				if i+1 < len(c.Items) {
					if err := r.runList(ctx, c.Items[i+1].Body); err != nil {
						return err
					}
				}
			case syntax.TokDSemiAmp:
				// Keep testing the *later* patterns, which is the narrower
				// thing `;;&` means and the reason it is a separate operator
				// rather than a spelling of `;&`.
				for _, later := range c.Items[i+1:] {
					if !r.caseItemMatches(later, subject) {
						continue
					}
					if err := r.runList(ctx, later.Body); err != nil {
						return err
					}
					if later.Term != syntax.TokDSemiAmp {
						break
					}
				}
			}
			return nil
		}
		return nil
	})
}

func (r *Runner) caseItemMatches(item *syntax.CaseItem, subject string) bool {
	for _, p := range item.Patterns {
		// A pattern is a word: unquoted it is a pattern, quoted a literal,
		// and only the spans still know which.
		if r.matchPatternR(r.patternOf(p), subject) {
			return true
		}
	}
	return false
}

func (r *Runner) funcDecl(c *syntax.FuncDecl) error {
	if r.funcs == nil {
		r.funcs = map[string]*syntax.FuncDecl{}
	}
	r.funcs[c.Name] = c
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
	saved, savedIn := r.Params, r.inFunc
	r.Params, r.inFunc = args, fn.Name
	r.depth++
	// A scope the function's locals unwind into.
	sc := &scope{saved: map[string]string{}, existed: map[string]bool{}}
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
	r.Params, r.inFunc = saved, savedIn
	if r.ctl == controlReturn {
		r.ctl = controlNone
	}
	return err
}
