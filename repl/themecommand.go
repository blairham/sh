// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"

	"github.com/blairham/sh/internal/prompttheme"
	"github.com/blairham/sh/interp"
)

// What the prelude's `prompt` function runs.
//
// The name a person types is a **function**, defined in a prelude the front
// end sources, and this is the Go behind it. interp/promptengine.go carries
// the decision and the reason — a `prompt` builtin would be a name no real
// shell has, visible to `type`, `command -v` and `compgen -A builtin` at
// once, where a prelude name is a divergence of a kind this shell already
// has and already measures.
//
// It is on the theme rather than on the session because the theme is the
// thing that knows its configuration: the layers, what each resolved to and
// which one answered. A session holds a theme; it does not hold a
// configuration.

// Engine is the theme as an interp.Builtin, for a front end to install with
// Runner.SetPromptEngine.
//
// Subcommands are matched here and not in the prelude's `case`, so that a
// front end shipping the function does not have to keep two lists in step.
// What the function does decide is the usage line and the status of a word
// it does not know, because those are located and named the dialect's way
// and only a prelude function can be.
func (t *Theme) Engine(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		return t.engineUsage(r)
	}
	switch args[0] {
	case "show":
		if len(args) > 1 {
			r.DiagnoseAsf(promptName, "%s: show takes no operands\n", promptName)
			return 2
		}
		for _, line := range t.Show().Lines() {
			// The stream is the shell's, so a write that fails has failed for
			// the same reason every other command's output would have. Nothing
			// useful can be said about it on the stream that just refused a
			// line, which is why the error goes nowhere.
			_, _ = fmt.Fprintln(r.Out(), line)
		}
		return 0
	case "import":
		return t.importConfig(r, ctx, args[1:])
	default:
		r.DiagnoseAsf(promptName, "%s: %s: no such subcommand\n", promptName, args[0])
		return t.engineUsage(r)
	}
}

// promptName is what the function is called, and what its complaints are
// named after. One spelling, because a diagnostic naming a word the person
// did not type is a diagnostic they cannot search for.
const promptName = "prompt"

// engineUsage writes what the command takes, and is a failure: a bare
// `prompt` asked nothing, and answering 0 to it would make a typo in a
// startup file look like a success.
//
// Located and named the dialect's way rather than written straight to the
// stream, which is the whole reason the name is a prelude function: what a
// person sees is their own line and the word they typed, worded the way the
// shell they are running words a builtin's complaint.
func (t *Theme) engineUsage(r *interp.Runner) int {
	r.DiagnoseAsf(promptName, "usage: %s show\n       %s\n", promptName, importUsage)
	return 2
}

// Show is what `prompt show` prints, as a value, so that the report can be
// tested without a stream and without a session.
//
// It resolves the configuration the way a render does — the same layer stack,
// re-reading a file whose mtime has moved — because a report that answered
// from a stale read would be describing a prompt other than the next one.
func (t *Theme) Show() prompttheme.Report {
	settings := t.resolve()
	return prompttheme.Describe(settings, t.roster, t.icons(settings), t.Problems())
}
