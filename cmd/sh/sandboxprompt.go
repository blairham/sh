// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/blairham/sh/repl"

// A prompt that says the session is gated.
//
// A shell running under `-policy` or `-deny` refuses things an ungated shell
// would do, and the refusal is worded as the failure it produces: a stat that
// is denied reads as a path that is not there, and an exec that is denied
// reads as a command that failed. That is deliberate — docs/design/sandboxing.md
// argues a policy must be indistinguishable from the kernel to the *script* —
// and it leaves the person at the keyboard with no way to tell a sandboxed
// session from an ordinary one until something surprising happens.
//
// So the prompt says. It is the same reason every shell in the panel draws
// `#` for root and `$` for everybody else: a session with unusual powers says
// so before you use them, and this is that in the other direction. Prior art
// outside shells does it the same way — an environment that has changed what
// a shell can reach prefixes the prompt with its name in parentheses.
//
// This binary and not driver, and not a dialect. `-policy` is cmd/sh's own
// flag, for the reason set out on it: driver is the shared front end for
// binaries that claim to *be* bash or zsh, and no real shell has a `-policy`,
// so no real shell has anything to say about drawing one either. A dialect
// cannot hold it at all — a provider is a code path and a dialect is a table
// of values.
//
// It is also the reachable consumer of repl's prompt seam, which is why it is
// here rather than in a test: a seam nothing reaches is a seam nothing grades,
// and the wiring from a flag to a drawn prompt crosses three packages.

// sandboxMarker draws the mark that says this session's accesses are decided
// by a policy.
type sandboxMarker struct{}

// sandboxPrompt is the mark itself, with the trailing space that separates it
// from whatever PS1 is. The seam adds no separator of its own, because a
// provider that wanted none would have no way to take one back.
const sandboxPrompt = "(sandboxed) "

// Prompt draws the mark before a new command and nothing before the rest of an
// unfinished one.
//
// Not at the continuation prompt, because the mark is about the session rather
// than about the line: PS2 is the middle of a command that has already been
// marked, and repeating it on every continuation line would say the session
// became sandboxed four times.
func (sandboxMarker) Prompt(info repl.PromptInfo) string {
	if info.Continued {
		return ""
	}
	return sandboxPrompt
}
