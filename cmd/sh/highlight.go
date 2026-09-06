// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/repl"
)

// -highlight: the reachable consumer of repl's highlighting seam.
//
// Off by default and a flag rather than a setting, for two reasons that point
// the same way. Measured, none of bash, zsh, dash or ksh93 colors a line as it
// is typed — coloring by default would make this binary the odd one out at
// every prompt — and there is no terminal capability database here to ask
// whether the terminal on the other end can do color at all. A person who
// turns it on has answered both questions.
//
// It is this binary's and not driver's, for the reason `-policy` and `-plugin`
// are: driver is the shared front end for binaries that claim to *be* bash or
// zsh, and `./bash -highlight` accepting a flag bash rejects would be a lie
// about what those binaries are. cmd/sh claims to be nothing, so it is where a
// substrate seam gets a way in — and a seam nothing reaches is a seam nothing
// grades.
//
// What it colors is `repl.UnclosedQuote`: the run of the line swallowed by a
// quotation that has not been closed. That case and not a general grammar
// coloring, because it is the one where the shell knows something the person
// does not — see the type's own note.

// unclosedQuoteStyle is what an unclosed quotation is drawn in: red.
//
// The direct color rather than one read from a variable or a terminfo entry.
// SGR 31 is the terminal's own arithmetic, the same numbers repl's prompt
// colors use, and it is what every terminal that does color at all understands
// — there is nothing here that a capability database would answer differently.
const unclosedQuoteStyle = "\x1b[31m"

// withHighlighting fills in the shell's highlighter from what the invocation
// asked for, and leaves it nil when it asked for nothing.
//
// A function of its own so a test can look at what the flag produced. Wiring
// dropped on the floor inside run() is invisible: the line is colored only at
// a terminal, so a highlighter that never arrives looks exactly like a person
// who did not pass the flag — which is the failure installSeams was pulled out
// of main() for, one seam over.
func withHighlighting(sh driver.Shell, own ownFlags) driver.Shell {
	if own.highlight {
		sh.Highlighter = repl.UnclosedQuote{Style: unclosedQuoteStyle}
	}
	return sh
}
