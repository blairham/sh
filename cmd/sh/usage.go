// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
)

// axisRemedy is what this binary would have a person do about an axis no
// dialect answered.
//
// It names `-dialect` and the shells it takes, which is why it lives here and
// not where the refusal is written: nothing under interp may name a shell, and
// driver has no flags of its own to point at. Both carry the sentence instead —
// see interp.Runner.AxisRemedy.
//
// `core` is deliberately absent from the list. It is a real value for the flag
// and the default, and it is also the answer that produced the refusal, so
// offering it back would be offering to change nothing.
const axisRemedy = "pass -dialect with one of posix, bash, zsh, ksh or dash"

// usage writes what this binary is and how it is invoked.
//
// It goes to standard output and the invocation ends 0, because help was what
// was asked for: a usage message on stderr with a failing status is for a
// person who got the line wrong, and this one did not.
//
// name is argv[0] rather than "sh", so the message names the binary as it was
// actually invoked — the same rule the front end uses for every diagnostic.
func usage(w io.Writer, name string) {
	if name == "" {
		name = "sh"
	}
	_, _ = fmt.Fprintf(w, usageText, name)
}

// usageText is the message, with one verb for argv[0].
//
// It lists the dialects before the flags on purpose. The dialect is the choice
// this binary exists to make visible and the one whose absence a person meets
// first: the core refuses every axis the shells disagree about, by design, and
// somebody who has just been told so needs the name of the flag more than they
// need the tracing options.
const usageText = `%[1]s — the substrate's driver: one parser and interpreter, wearing a chosen dialect.

Usage:
	%[1]s                            a prompt, if standard input is a terminal
	%[1]s -c COMMAND [$0 [ARG ...]]  run a command string
	%[1]s SCRIPT [ARG ...]           run a script, with $1 onward set
	%[1]s < SCRIPT                   run a script on standard input

Dialects (-dialect NAME, default core):
	core    what every shell agrees on; anything they disagree about is
	        refused rather than guessed — a portability check, not a runtime
	posix   the standard: no arrays, no [[ ]], no substrings
	bash    the disputed axes answered the way that shell answers them
	zsh
	ksh
	dash

Flags this binary has and no shell does. They come first on the line: the
first word that is not one of them belongs to the shell.

	-h, -help, --help      print this and exit
	-dialect NAME          which shell to be where the shells disagree
	-tokens SOURCE         dump the token stream
	-parse SOURCE          dump the syntax tree
	-policy FILE           run under a declarative policy
	-deny RULE             refuse one path, or one kind of action, repeatable
	-audit FILE            record every gated action as JSON, one per line
	-trace-events          print every gated action to standard error
	-blocks-list[=N]       the recent blocks: what ran, and how it went
	-blocks-show ID        one block in full, its record and its output
	-acp                   serve the Agent Client Protocol on standard input
	-acp-connect CMD ...   drive an ACP agent, under the same policy
	-acp-allow             answer the agent's permission requests with yes
	-acp-auth METHOD       sign in with one of the methods the agent offers

Everything else — -c, -i, -s, a lone -, set options like -e — is read by the
shared front end, exactly as each dialect binary reads it.
`
