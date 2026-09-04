// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"os"

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
func InteractiveArgs(sh Shell, argv []string) int { return sh.interactive(argv, nil) }

// interactive is InteractiveArgs with the positional parameters the invocation
// supplied, which only the front end reading it knows.
func (sh Shell) interactive(argv, params []string) int {
	sh = sh.withDefaults(argv)
	dg := sh.Diagnostics
	name := sh.Name
	// A prompt is not a command string, whatever else it is.
	r := sh.newRunner(name, params, dg, false)
	// A prompt has someone to tell about its jobs, and a script does not:
	// no shell in the panel announces a background job to `sh -c`, and a
	// Runner embedded in another program has nobody to announce one to.
	r.JobControl = true
	if sh.Prelude != "" {
		if code := sh.source(r, name); code != 0 {
			return code
		}
	}
	// The prelude is the dialect's own; these are the user's, and come after
	// it so a person's settings win over the shell's defaults.
	if code := sh.startup(r, LoginShell(argv)); code != 0 {
		return code
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
func Interactively(sh Shell, hasWork bool) bool {
	if hasWork {
		return false
	}
	if sh.Stdin == nil {
		sh.Stdin = os.Stdin
	}
	info, err := sh.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
	}
}
