// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "github.com/blairham/sh/interp"

// newTestRunner is a Runner with just enough in it for the prompt questions.
//
// With no history unless the caller asks for one. A session numbers its
// prompts from where the history file left off, so a test that said nothing
// would read this machine's own history — a different answer on every machine,
// and the user's file besides. An empty HISTFILE is how a session says to keep
// none, which is exactly what a test wants.
func newTestRunner(vars map[string]string) *interp.Runner {
	sem := interp.PosixSemantics()
	if vars == nil {
		vars = map[string]string{}
	}
	if _, ok := vars["HISTFILE"]; !ok {
		vars["HISTFILE"] = ""
	}
	// A nil Env is genuinely empty — interp never falls back to the process
	// environment — so the externals these tests reach (sleep, cat) get a
	// PATH handed in, from the two directories every supported platform
	// keeps the basics in.
	r := &interp.Runner{Semantics: &sem, Vars: vars, Env: []string{"PATH=/usr/bin:/bin"}}
	// Told who is typing and where, which is what a shell binary does at
	// startup — dialect/bash and dialect/zsh both call SetPromptUserFunc with
	// interp.LoginName. The drawer does not read these names out of the
	// environment and must not: `%n` and `\u` are measured to ignore `USER`,
	// `LOGNAME` and `HOSTNAME` in every column of the panel, so a test that
	// set only the variable would be asserting a behavior no shell has.
	//
	// Taken from the map the caller already passes so that the tests written
	// against those keys keep saying what they said. What they now say is
	// "this shell was told", and TestTheDrawnUserIsWhatTheShellWasTold is the
	// row that holds the two apart.
	if v, ok := vars["USER"]; ok {
		r.SetPromptUser(v)
	}
	if v, ok := vars["HOSTNAME"]; ok {
		r.SetPromptHost(v)
	}
	return r
}
