// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"syscall"
)

// InterruptAtThePrompt answers a ^C typed at an interactive prompt, and
// reports whether the line being typed is kept.
//
// The prompt is the front end's, and so is the keystroke — a line editor
// reads ^C as a byte, and a terminal gathering a line was told to hand it
// over as one — so no signal reaches the shell and nothing here would know
// one was typed. This is the door the front end comes through instead, and it
// is the only place the status of an interrupted prompt is set, so the editor
// and the read without one cannot answer differently.
//
// With no trap the line is given up and the status is 128 plus SIGINT, in
// every shell measured (#5867). Under `trap "" INT` the ^C is nothing at all:
// the line is kept and the status left alone (#5888). A trap with a body is
// the dialect's answer — see Semantics.PromptInterruptTrap.
func (r *Runner) InterruptAtThePrompt(ctx context.Context) (keep bool) {
	name, _ := signalName(syscall.SIGINT)
	s := r.sigs()
	s.mu.Lock()
	body, trapped := s.traps[name]
	s.mu.Unlock()
	interrupted := 128 + int(syscall.SIGINT)
	switch {
	case trapped && body == "":
		return true
	case !trapped:
		r.SetPromptStatus(interrupted)
		return false
	}
	// The handler reads `$?` as it stands at the prompt. A handler shown the
	// status from before the last command — zsh's answer for one that
	// interrupts a command, SignalHandlerSeesEarlierStatus — has no command
	// to look behind here, and zsh shows it the status the prompt has.
	before := r.statusBefore
	r.statusBefore = r.status
	defer func() { r.statusBefore = before }()
	switch r.sem().PromptInterruptTrap {
	case PromptInterruptStatusThenTrap:
		r.SetPromptStatus(interrupted)
		r.runPromptTrapAction(ctx, name, body)
		return false
	case PromptInterruptTrapDecides:
		returned, status, resumed := r.runPromptTrapAction(ctx, name, body)
		if !resumed && r.ctl == controlReturn {
			// A `return` written in the action's own text, which at a prompt
			// has no function to return from: it returns from the action,
			// and its status is the action's. Measured, `trap 'return 1'
			// INT` gives the line up and leaves 1.
			r.ctl = controlNone
			returned, status, resumed = true, r.status, true
		}
		if !resumed {
			return false
		}
		if returned && status != 0 {
			r.SetPromptStatus(status)
			return false
		}
		return true
	default:
		r.runPromptTrapAction(ctx, name, body)
		return false
	}
}

// runPromptTrapAction is runTrapAction with the pipeline record put back
// beside the status, which a handler's own commands would otherwise leave as
// theirs. Measured 2026-10-04 after `false`, `abc` and ^C under a handler
// that prints: zsh 5.9.2 reads pipestatus `1` on the next line whether the
// line was kept or given up, and bash 5.3.20 reads PIPESTATUS `130`, the
// record the prompt's own status wrote before the handler ran.
func (r *Runner) runPromptTrapAction(ctx context.Context, name, body string) (returned bool, status int, resumed bool) {
	kept := r.keepStatus()
	returned, status, resumed = r.runTrapAction(ctx, name, body)
	r.pipeStatus = kept.pipe
	return returned, status, resumed
}
