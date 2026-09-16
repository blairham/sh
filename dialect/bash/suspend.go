// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"

	"github.com/blairham/sh/interp"
)

// The `suspend` builtin: this shell stops until something continues it.
//
// Measured 2026-09-15 against bash 5.3.15, `env -i` with a scratch HOME and no
// startup files. Every probe that can *stop* was run in a process group of its
// own and killed from outside, which is the warning #2557 carries and which
// cost two runs while it was being written: a stopped process does not answer
// SIGTERM, so a harness waiting on one waits forever.
//
//	bash -c 'suspend; echo st=$?'       cannot suspend: no job control   st=1
//	bash -l -c 'suspend; echo st=$?'    cannot suspend: no job control   st=1
//	bash -c 'suspend -f'                stops
//	bash -c 'suspend -q; echo st=$?'    -q: invalid option + usage       st=2
//	bash -c 'suspend x; echo st=$?'     too many arguments, and it ends
//
// # Two halves, and only one of them is a script's
//
// The half a script sees is a **refusal**, and it is the common one: `bash -c`
// runs no monitor, so the builtin a script most often meets is the one that
// says so and reports 1. That half is entirely this shell's own state — the
// monitor and whether this is a login shell — and needs nothing from the
// process.
//
// The other half really stops, and stopping is what a library may not do. The
// rule AGENTS.md states for `exec` and for a fatal signal applies here word
// for word: a Runner is embedded in other programs, and a line of script that
// could stop the *host* until somebody found it and sent it SIGCONT is a
// library that can be told to hang its caller. So the system call is
// [interp.Runner.StopThisProcess] — nil in a library, filled in by driver —
// and this file decides only *when* it is asked.
//
// #2557 said this "belongs with the job control work, and the refusal comes
// with it", on the reasoning that a builtin doing only the reachable half
// would answer `cannot suspend: no job control` at an interactive prompt where
// `$-` says `m` — a wrong sentence rather than a missing feature. The hook is
// what removes that objection rather than working around it: where the monitor
// is running and the front end owns the process, this really stops, and the
// refusal is left to say only what is true.
//
// # What decides the refusal
//
// Three conditions, and `-f` overrides the first two and not the third.
//
//   - **No monitor.** `set -m` off is `cannot suspend: no job control`, which
//     is the state every `bash -c` starts in.
//   - **A login shell.** bash refuses to suspend one, and answers it with the
//     same sentence: `bash -l -c 'suspend'` is `cannot suspend: no job
//     control` and not a login-specific wording, because a login shell started
//     that way has no monitor either. The condition is recorded as bash's own
//     — `help suspend` states it — and the wording is the measured one.
//   - **No hook**, which is a library Runner rather than a shell — and this
//     one is asked *after* the other two, so a `bash -c 'suspend'` embedded in
//     some other program still answers what the real shell answers. Only
//     `suspend -f` gets this far, and what it hears is the substrate's own
//     sentence rather than bash's: `-f` forces past what a *shell* declines,
//     and a Runner that is not the process is not the shell declining. The
//     wording is the one `umask` uses when it was given no mask to change —
//     this shell in its own voice, stating what it was not given, because
//     there is no real shell's sentence for a state no real shell can be in.
func registerSuspend(r *interp.Runner) { r.Register("suspend", suspendBuiltin) }

// suspendUsage is the line a refused option prints after the complaint, in
// bash's own words.
const suspendUsage = "suspend: usage: suspend [-f]"

func suspendBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	force, ended := false, false
	for _, arg := range args {
		switch {
		case !ended && arg == "--":
			ended = true
		case !ended && arg == "-f":
			force = true
		case !ended && len(arg) > 1 && arg[0] == '-':
			r.Diagnosef("suspend: -%c: invalid option\n", arg[1])
			// Bare rather than located, which is bash's shape for a usage
			// line and the reason it is written to the stream directly:
			// `bash: line 1: suspend: -q: invalid option` is followed by
			// `suspend: usage: suspend [-f]` with no location at all. See
			// Diagnostics.BuiltinUsageUnprefixed, which says the same thing
			// for the builtins the substrate owns.
			_, _ = fmt.Fprintf(r.Err(), "%s\n", suspendUsage)
			return 2
		default:
			// An operand, and this builtin has none. Re-measured on bash
			// 5.3.15, 2026-09-16, each in a process group of its own with
			// stdin from /dev/null: `suspend x`, `suspend a b` and `suspend
			// -- x` are each `suspend: too many arguments` at **2**, the
			// status bash gives a usage error and the same one the bad
			// option above returns. The 1 recorded here before was wrong,
			// and it is the reason this line is asked in a test rather than
			// left to a comment — see TestSuspendOperandStatus.
			//
			// **The order is 5.x's and not bash's.** bash 3.2 asks about job
			// control first and answers `suspend x` with `cannot suspend: no
			// job control` at 1, never reaching the count. This column is
			// graded against 5.3, so the count comes first here.
			//
			// bash also **abandons the rest of the command list** there —
			// `suspend x || echo caught` writes nothing, though a following
			// *line* still runs and the shell leaves with 0. That half is
			// recorded and not modeled: a dialect builtin has no way to
			// abandon a list from here, and inventing one for a corner no
			// script writes would be a wider seam than the fact deserves.
			// The wording and the status are this shell's; what follows the
			// line is not.
			r.Diagnosef("suspend: too many arguments\n")
			return 2
		}
	}
	if !force && (!r.MonitorOn() || r.LoginShell) {
		r.Diagnosef("suspend: cannot suspend: no job control\n")
		return 1
	}
	if r.StopThisProcess == nil {
		r.Diagnosef("suspend: this shell was not given a way to stop this process\n")
		return 1
	}
	if err := r.StopThisProcess(); err != nil {
		r.Diagnosef("suspend: %v\n", err)
		return 1
	}
	// Reached only after a SIGCONT, which is why the success is reported here
	// rather than beside the call: `suspend` is 0 once the shell is running
	// again and says nothing at the moment it stops.
	return 0
}
