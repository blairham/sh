// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// The `suspend` builtin: this shell stops until something continues it.
//
// The same name bash has and a different builtin, which is why it is written
// here rather than shared. Measured 2026-09-15 against zsh 5.9.2, `env -i`
// with a scratch HOME and no startup files, every stopping probe in a process
// group of its own and killed from outside:
//
//	zsh -f -c 'suspend'                    stops
//	zsh -f -l -c 'suspend; echo st=$?'     can't suspend login shell   st=1
//	zsh -f -l -c 'suspend -f'              stops
//	zsh -f -l -c 'suspend -q; echo st=$?'  bad option: -q              st=1
//	zsh -f -l -c 'suspend x; echo st=$?'   too many arguments          st=1
//
// # Where it parts from bash
//
// **Job control decides nothing here.** `zsh -f -c 'suspend'` stops, and that
// shell runs no monitor — where the same line in bash is `cannot suspend: no
// job control` at 1. So the refusal in this shell has one condition and in
// that one it has two, which is why the two builtins share no code: a shell
// that asked bash's question would refuse the common case, and a shell that
// asked this one would stop where bash declines.
//
// **One status for every refusal.** A bad option is 1 here and 2 there, and
// this shell writes no usage line after it.
//
// **The login-shell sentence is its own**, where bash answers a login shell
// with the job-control sentence it gives every other refusal — measured:
// `bash -l -c 'suspend'` is `cannot suspend: no job control`.
//
// # The stop is not this package's to make
//
// [interp.Runner.StopThisProcess] is nil in a library and filled in by driver,
// for the reason AGENTS.md gives for `exec` and for a fatal signal: a Runner
// is embedded in other programs, and a line of script that could stop the
// *host* until somebody sent it SIGCONT is a library that can be told to hang
// its caller. This file decides only when it is asked.
//
// A Runner that was given no hook says so in the substrate's own words rather
// than in this shell's, because no real zsh can be in that state and borrowing
// a sentence from one that can would name the wrong reason.
func registerSuspend(r *interp.Runner) { r.Register("suspend", suspendBuiltin) }

func suspendBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	force, ended := false, false
	for _, arg := range args {
		switch {
		case !ended && arg == "--":
			ended = true
		case !ended && arg == "-f":
			force = true
		case !ended && len(arg) > 1 && arg[0] == '-':
			// One line and status 1, with no usage after it — this shell's
			// shape for a bad option and the same wording `sched` and
			// `bindkey` write.
			r.Diagnosef("bad option: -%c\n", arg[1])
			return 1
		default:
			r.Diagnosef("too many arguments\n")
			return 1
		}
	}
	if !force && r.LoginShell {
		r.Diagnosef("can't suspend login shell\n")
		return 1
	}
	if r.StopThisProcess == nil {
		r.Diagnosef("this shell was not given a way to stop this process\n")
		return 1
	}
	if err := r.StopThisProcess(); err != nil {
		r.Diagnosef("%v\n", err)
		return 1
	}
	// Reached only after a SIGCONT: `suspend` reports 0 once the shell is
	// running again and says nothing at the moment it stops.
	return 0
}
