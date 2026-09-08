// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"os/user"
)

// LoginName is the name the system has for the user this process runs as, and
// empty when it has none.
//
// **This package never calls it.** The rule SetPromptUser states is unchanged:
// a Runner embedded in another program does not go asking the system who it
// is, it calls back what the caller handed it. What lives here is the
// *question*, written once so that every shell that has an escape for the
// answer asks it the same way — `r.SetPromptUserFunc(interp.LoginName)` in
// dialect/bash and dialect/zsh, and repl through the Runner they told. It is
// the shape interp.IntegerBuiltin already has, and it is here for the reason
// that one is: two dialects needing the same answer is how a second copy gets
// written, and the second copy is the one that keeps the bug.
//
// #1446 is that bug. The login name had one asker — zsh's Apply, in a closure
// of its own — and the drawer had a lookup of its own reading `$USER` and
// `$LOGNAME`, which name nothing under `env -i`, `sudo -i`, a container or a
// daemon-started login shell. bash's `\u` therefore drew *nothing*, and the
// default `\u@\h:\w\$ ` of most distributions rendered `@host:~$` — a prompt
// that looks like a prompt, which is why nothing announced it.
//
// It is not read out of a variable, which is measured rather than assumed.
// bash 5.3.15 and bash 3.2.57 draw the password database's answer for `\u`
// with `USER` and `LOGNAME` injected before the shell starts *and* assigned
// inside it, exactly as zsh 5.9.2 does for `%n`. A prompt that followed the
// variable would name the wrong person under `env USER=someone-else`.
//
// Empty is a real answer and not an omission: a uid with no password database
// entry has no login name. The panel disagrees about what to draw then, which
// is why nothing is invented here — measured in a container at uid 99999,
// bash draws the words `I have no name!` and zsh draws nothing at all. The
// callers get the empty string and each says what it says about it; see
// #1451.
func LoginName() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

// MachineName is the name the system has for this machine, asked the same way
// and for the same reason.
//
// Beside LoginName rather than in a closure of its own so that the two facts a
// prompt wants about where it is being typed are one idiom. That symmetry is
// not decoration: the drawer's host lookup had the fallback to the system that
// its user lookup was missing, so `\h` was right and `\u` was empty, and the
// half that worked is what made the half that did not look like a prompt.
//
// `$HOSTNAME` is not consulted, for the reason `$USER` is not: measured, bash
// draws the system's name for `\h` and `\H` with `HOSTNAME=elsewhere` injected
// and with it assigned inside the shell.
//
// Cheap — 3.9 µs, against 0.83-1.10 ms for LoginName on darwin, which is
// Directory Services — and deferred anyway through SetPromptHostFunc, so the
// pair is one idiom rather than an eager half beside a lazy one.
func MachineName() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
