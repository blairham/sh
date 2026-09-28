// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The bound on how deep a function may call, and the name a script moves it
// with.
//
// This shell had a bound already — the substrate's own ceiling, which no
// script can reach for or move — so what was missing was not the refusal but
// everything around it: the parameter, the bound *being* that parameter, the
// sentence that names it, and what the refusal costs. #4905 is the argument
// for doing the four together, and it is worth repeating here because the
// first of them alone is the trap: `FUNCNEST=5` would then be stored,
// described with the right letters, and ignored, which is a wrong answer
// where an absent parameter is merely a missing one.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go executable*
// for it, so the reference is a different program from ours — script files
// run `-f` under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`:
//
//	${+FUNCNEST}         1                      here, before: 0
//	${(t)FUNCNEST}       integer-special        here, before: nothing
//	typeset -p FUNCNEST  typeset -i10 FUNCNEST=500
//	env FUNCNEST=7       export -i10 FUNCNEST=7 integer-export-special
//
// so the parameter is the shell's own, base ten written down, and a value the
// environment hands in wins — the same row `SAVEHIST` is on in
// startupvalues.go, which is where the value lives.
//
// And what the two shells say to a function that calls itself forever, one
// run, `-f` from a script file:
//
//	reference   f: maximum nested function level reached; increase FUNCNEST?
//	ours before f: f: too deeply nested
//
// The sentence is Diagnostics.FunctionNestingLimit in zsh.go. The second
// `f:` in this shell's old answer is the substrate's ceiling speaking under
// the shell's location prefix, and that ceiling is still here behind the
// parameter: it is what a negative `FUNCNEST` falls through to, where the
// reference segmentation faults.
func boundFunctionNesting(r *interp.Runner) {
	r.SetFunctionNesting(interp.FunctionNesting{
		Parameter: "FUNCNEST",
		// A stored zero is a bound of zero here and is "nothing said" in
		// bash — see interp.FunctionNesting.ZeroIsABound for the table the
		// two columns were measured in together. The integer attribute is
		// what folds `FUNCNEST=` and `FUNCNEST=abc` into this row rather
		// than into the unreadable one.
		ZeroIsABound: true,
		// And the refusal is fatal rather than a give-up of the line: the
		// line after the call never runs, where bash's does.
		RefusalEndsTheScript: true,
		// And the sentence has no name in it, because the name is in the
		// location: `f:4:` at the top level and `f:` inside a body. See the
		// field, where the three rows are measured beside bash's — which
		// says the same two things the other way round.
		RefusalNamesTheCalleeInTheLocation: true,
	})
}
