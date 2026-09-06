// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"io"
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
func InteractiveArgs(sh Shell, argv []string) int { return sh.interactive(argv, source{}) }

// interactive is InteractiveArgs with what reading the invocation produced —
// the positional parameters, the set options, and what was said about the
// startup files — which only the front end reading it knows.
//
// The guard here is the backstop and not the working one: repl catches a panic
// per typed line, which is what keeps a session alive, and this catches what
// happens on either side of the loop — the dialect's prelude, the runner being
// built, the EXIT trap at the end. A session that cannot be started is at
// least a session that says why.
func (sh Shell) interactive(argv []string, in source) (status int) {
	sh = sh.withDefaults(argv)
	if sh.guard().Do(func() { status = sh.session(argv, in) }) {
		return panicguard.Status
	}
	return status
}

// session is interactive once the defaults are filled in and a guard is
// around it.
func (sh Shell) session(argv []string, in source) int {
	dg := sh.Diagnostics
	name := sh.Name
	params, opts := in.params, in.opts
	// The three facts the startup files are chosen by, stated here because
	// this route never reaches the place the script routes read them. A
	// prompt is interactive by definition and whether it was reached through
	// `-i` or by finding a terminal, and the other two are argv[0]'s.
	in.interactive = true
	in.login = LoginShell(argv)
	in.posix = PosixNamed(argv)
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
	// And an interactive shell runs the monitor. Unanimous with a terminal —
	// bash, dash, ksh93 and zsh all report `monitor on` and put `m` in `$-`
	// — and a prompt has one by definition on every route but `-i` with
	// nothing to read, which is why the question is still asked rather than
	// assumed. One member of the panel does not need one at all.
	r.SetInteractiveMonitor(sh.hasTerminal())
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
	// Every shell in the panel expands aliases at a prompt whatever it does
	// in a script, so the base is on here — before the startup files, which
	// may define an alias and use it, and before the mode below, which the
	// base must not overwrite.
	r.SetAliasExpansionBase(true)
	if code := sh.startup(r, in); code != 0 {
		return code
	}
	if in.posix {
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

// keyBindings closes the dialect's answer over this run's Runner, or is nil
// where the dialect has no way to rebind a key.
//
// A nil function rather than one returning an empty table, because repl tells
// the two apart to skip the lookup entirely — see bindings.go.
func (sh Shell) keyBindings(r *interp.Runner) func() map[string]repl.Widget {
	if sh.KeyBindings == nil {
		return nil
	}
	return func() map[string]repl.Widget { return sh.KeyBindings(r) }
}

// hasTerminal reports whether any of this shell's three standard streams is a
// terminal, which is what an interactive shell reads to decide whether to run
// the monitor.
//
// Any of the three, and that is measured rather than chosen: a pseudo-terminal
// on standard input alone, on standard output alone, or on standard error
// alone all make bash 5.3.15, dash and zsh 5.9.2 report `monitor on` under
// `-i script.sh`. A *controlling* terminal with all three redirected
// elsewhere does not — all three report it off — so the question is about the
// descriptors this front end was handed and not about the process's terminal,
// which is the question interp could not have asked anyway.
//
// Different from Interactively, which asks only about standard input: that one
// decides whether to draw a prompt, and a prompt is drawn on the stream the
// lines come from.
func (sh Shell) hasTerminal() bool {
	if repl.IsTerminal(sh.Stdin) {
		return true
	}
	for _, w := range []io.Writer{sh.Stdout, sh.Stderr} {
		if f, ok := w.(*os.File); ok && repl.IsTerminal(f) {
			return true
		}
	}
	return false
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
		Style:   sh.PromptStyle,
		Editor:  sh.EditorStyle,
		History: sh.HistoryStyle,
		// What this binary adds to every prompt, which is nothing for a
		// dialect binary and the sandbox marker for cmd/sh under a policy.
		PromptProviders: sh.PromptProviders,
		// And what colors the line while it is typed, which is nothing
		// unless the binary asked for it.
		Highlighter: sh.Highlighter,
		// Bound to *this* runner and read per keystroke, so a `bindkey` typed
		// at the prompt takes effect on the next line rather than the next
		// shell.
		KeyBindings: sh.keyBindings(r),
		Name:        name,
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
