// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ContinuesPastAFatalError is one shell's switch for "a fatal error gives up
// the statement rather than the shell".
//
// It is a session switch rather than a Semantics axis because there is no
// disagreement to settle: one shell in the panel has the option and the other
// five have nothing to answer with. A script that never turns it on cannot
// tell whether it exists.
//
// **The route decides whether it applies**, which is measured rather than
// assumed and is the same split giveUpForABadSubscript already carries.
// Measured 2026-09-29 on zsh 5.9.2, the option set *inside* the program so
// that its state is identical on both sides and only the route moves:
//
//	setopt continueonerror; readonly foo; foo=bar set output; print after
//	    from a file or standard input   `after`, status 0
//	    from `-c`                       nothing after, status 1
//
// The option reports itself **on** under `-c` — `[[ -o continueonerror ]]`
// is true there — so this is not the option failing to take. It is a fact
// about the invocation, and it is `Runner.Route` that carries it.
func (r *Runner) SetContinuesPastAFatalError(on bool) { r.continuePastFatal = on }

// ContinuesPastAFatalError reports that switch, for the option that sets it.
func (r *Runner) ContinuesPastAFatalError() bool { return r.continuePastFatal }

// rescuesAFatalError reports whether a fatal error raised now gives up the
// statement instead of the shell.
//
// Two conditions, and the second is the measured one: `-c` is not rescued
// however the option is set.
func (r *Runner) rescuesAFatalError() bool {
	return r.continuePastFatal && r.Route != RouteCommandString
}
