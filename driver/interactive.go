// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"os"

	"github.com/blairham/sh/internal/panicguard"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// Interactive reads and runs commands from a terminal until the input ends.
//
// The same Runner the script path builds, so an interactive shell and a script
// are the same shell: the hooks, the dialect's builtins and its prelude are
// all wired the way they are for a file. What differs is where the lines come
// from and that an unfinished construct asks for more rather than failing.
func Interactive(sh Shell) int { return InteractiveArgs(sh, os.Args) }

// InteractiveArgs is Interactive with the argument vector given rather than
// taken from the process, which is what makes it testable without a
// subprocess — the same split as Main and MainArgs, and for the same reason.
func InteractiveArgs(sh Shell, argv []string) int { return sh.interactive(argv, nil, nil) }

// interactive is InteractiveArgs with the positional parameters and the set
// options the invocation supplied, which only the front end reading it knows.
//
// The guard here is the backstop and not the working one: repl catches a panic
// per typed line, which is what keeps a session alive, and this catches what
// happens on either side of the loop — the dialect's prelude, the runner being
// built, the EXIT trap at the end. A session that cannot be started is at
// least a session that says why.
func (sh Shell) interactive(argv, params []string, opts []optionSpec) (status int) {
	sh = sh.withDefaults(argv)
	if sh.guard().Do(func() { status = sh.session(argv, params, opts) }) {
		return panicguard.Status
	}
	return status
}

// session is interactive once the defaults are filled in and a guard is
// around it.
func (sh Shell) session(argv, params []string, opts []optionSpec) int {
	dg := sh.Diagnostics
	name := sh.Name
	// A prompt reads its program from standard input, which is what it is
	// however it was reached: `sh`, `sh -s` and `sh -i` at a terminal all
	// show `s` in `$-` across the panel.
	r := sh.newRunner(name, params, dg, interp.RouteStandardInput)
	// A prompt has someone to tell about its jobs, and a script does not:
	// no shell in the panel announces a background job to `sh -c`, and a
	// Runner embedded in another program has nobody to announce one to.
	r.JobControl = true
	// And a prompt is interactive by definition, whether or not `-i` said
	// so: measured, all four shells put `i` in `$-` at a terminal with
	// nothing to run. Set beside JobControl and not folded into it — the two
	// answer different questions, and ksh93 is alone in turning the monitor
	// on for `-i script.sh`.
	r.Interactive = true
	if sh.Prelude != "" {
		if code := sh.source(r, name); code != 0 {
			return code
		}
	}
	// The invocation's options come before the startup files, which is
	// where the panel has them: what `-x` traces includes what the rc file
	// does. And a refused one ends the shell before it prompts — `sh -Q`
	// with a terminal is an error, not a session.
	if code, ok := sh.applyOptions(r, opts); !ok {
		return code
	}
	// And the environment's own option list, in the same place and for the
	// same measured reasons as on the script routes: after the argument
	// vector, before the files. A prompt reads it too — an inherited `xtrace`
	// traces the lines a person types.
	r.ApplyInheritedShellOptions()
	// The prelude is the dialect's own; these are the user's, and come after
	// it so a person's settings win over the shell's defaults.
	if code := sh.startup(r, LoginShell(argv)); code != 0 {
		return code
	}
	if PosixNamed(argv) {
		// The other fact argv[0] carries, asked here for the reason
		// LoginShell is asked here: the prompt route never reaches the place
		// the script routes read it. Last for the same measured reason — the
		// name wins over the invocation's own options, and the startup files
		// run before the mode is on.
		r.SetPosixMode(true)
	}
	s := sh.frontEnd(r, name, dg)
	ctx := context.Background()
	status, err := s.Run(ctx)
	if err != nil {
		sh.errf("%s", dg.Report(name, 1, err.Error()+"\n"))
		return usageStatus
	}
	// The EXIT trap fires when the session ends, the same as at the end of a
	// script — `trap 'echo bye' EXIT` typed at the prompt has to mean
	// something.
	if code := r.Finish(ctx); code != 0 && status == 0 {
		status = code
	}
	return status
}

// Interactively reports whether this shell should offer a prompt: nothing to
// run was named, and the input is a terminal.
//
// Both halves matter. `sh < script.sh` has no argument either and must not
// prompt, and `echo x | sh` must read the pipe rather than wait for a
// keystroke that will never come.
//
// "A terminal" is an ioctl the kernel answers, not the mode bits. It was the
// mode bits, and `sh < /dev/null` — the canonical cron, systemd, CI and
// harness invocation, whose whole purpose is that the shell waits for nobody —
// took the prompt route, asked the null device for raw mode, was told ENOTTY
// and exited 2 with `operation not supported by device`. The null device is a
// character device; so are `/dev/zero` and `/dev/random`. Measured, no shell
// in the panel prompts on any of them, and none prompts on a regular file, a
// pipe or a closed descriptor either — `docs/spec/invocation.md` has the
// grid. Only `-i`, which is `forcePrompt` and never reaches here, overrides
// it.
//
// It is not the same question as whether the shell is *interactive*, and the
// two were once one field. `sh -i script.sh` runs the script and does not
// prompt afterwards — three of the four panel shells exit there — but it is
// interactive while it does, which `$-` reports and which decides whether
// aliases expand. That fact is `source.interactive`; this is only whether to
// draw a prompt.
func Interactively(sh Shell, hasWork bool) bool {
	if hasWork {
		return false
	}
	if sh.Stdin == nil {
		sh.Stdin = os.Stdin
	}
	// repl owns the termios calls, so the ioctl lives there and there is one
	// of it. The decision is still the front end's: this function is what
	// says a terminal plus no operands means a person.
	return repl.IsTerminal(sh.Stdin)
}

// frontEnd is the interactive shell this dialect asks for, as a value.
//
// Assembled here rather than inline so that what a dialect says reaches the
// prompt is something a test can look at. Two mutations of this wiring
// survived everything when it was inline: the editor exists only with a
// terminal, so no test could reach the fields being carried across, and a
// dialect's answer dropped on the floor here looks exactly like a dialect
// that did not answer.
func (sh Shell) frontEnd(r *interp.Runner, name string, dg interp.Diagnostics) repl.Shell {
	return repl.Shell{
		Runner:  r,
		Dialect: sh.Dialect,
		In:      sh.Stdin,
		Out:     sh.Stdout,
		Err:     sh.Stderr,
		// A parse failure is worded by the dialect here exactly as it is for
		// a script, minus the line echo: the line is still on the screen
		// above the complaint, having just been typed.
		Report: func(err error) string {
			return dg.ParseDiagnostic(name, "", err, "")
		},
		Style:  sh.PromptStyle,
		Editor: sh.EditorStyle,
		Name:   name,
		// The same policy and observer the Runner is given, because a
		// session gated for what a script does and ungated for what the
		// prompt does has a hole shaped exactly like `HISTFILE=/somewhere`.
		Gate:   sh.Gate,
		Events: sh.Events,
		// And the run's identity, so what the prompt records about a command
		// and what the event stream records about the same command name the
		// same session.
		Session: sh.Session,
		// Whether a panic caught on a typed line prints its stack. Decided
		// here because it is read from the process's environment, which is
		// this package's to read and not repl's — see panic.go.
		PanicTrace: panicTrace(),
	}
}
