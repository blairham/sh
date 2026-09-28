// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A name the *shell* owns, told rather than derived.
//
// [ParameterAttributes.Provided] is the fact a dialect renders as `special`,
// and until this there were two sources for it and both were shapes rather
// than statements: a name with a producer behind it, and a name registered to
// refuse. Those cover every parameter whose *value* is made up on a read, and
// they cover none of the ones the shell stores an ordinary value into and
// still calls its own — which is most of them. `$UID`, `$EUID`, `$GID`,
// `$EGID` and `$IFS` arrive through [Runner.SetSpecial], `$PATH` and its
// seven sibling pairs through [Runner.Tie], and every one of them sat in
// `Vars` looking exactly like a name a script had written (#4488).
//
// The mark is deliberately not something `SetSpecial` does on its own.
// Measured 2026-09-26 on zsh 5.9.2 under `-f`, `${(t)HOST}` is a bare
// `scalar` there and here — and `$HOST` is stored through that same hook, so
// a hook that marked what it stored would have taught this shell a fact the
// reference contradicts. It is one name per line, at the site that knows.

// MarkShellOwnParameter records that a name is the shell's own rather than a
// script's, which is the fact a dialect writes as `special`.
//
// For a parameter with an ordinary stored value. A *produced* one needs
// nothing: a producer is already the statement, and [Runner.DynamicParameter]
// answers for it.
func (r *Runner) MarkShellOwnParameter(name string) {
	if r.shellOwn == nil {
		r.shellOwn = map[string]bool{}
	}
	r.shellOwn[name] = true
}

// MarkParameterNotTheShellsOwn says a **produced** name is not the shell's
// own after all, which is the one case [ParameterAttributes.Provided] gets
// wrong by deriving the fact from a shape.
//
// The derivation is stated at the top of this file: a producer *is* the
// statement, for every parameter whose value is made up on a read. Three
// names break it, and they break it because the production is this engine's
// decision rather than the modeled shell's — measured 2026-09-27 on zsh
// 5.9.2 under `-f` from a script file:
//
//	${(t)TTY}         scalar          the terminal's path
//	${(t)ZSH_SCRIPT}  scalar          the script's path as written
//	${(t)LOGNAME}     scalar-export   the session's login name
//	${(t)USERNAME}    scalar-special  the process's login name  ← the control
//
// All four are ordinary stored scalars in the reference and only one of them
// is the shell's own. Here the first three have to be produced for reasons
// the reference does not share — the terminal and the script path are not
// known when a dialect registers its parameters, and the login name costs a
// millisecond nothing should pay until it is read — so the shape says
// `special` about three names the shell being modeled does not.
//
// The fourth row is why this is a statement rather than a rule about
// produced names: `USERNAME` is produced for the same reason `LOGNAME` is
// and *is* special, so nothing about the production decides it.
//
// It does not change what the parameter does, only what it says about
// itself — the same split [Runner.SetDynamicDeclaration] draws for a
// listing's letters.
func (r *Runner) MarkParameterNotTheShellsOwn(name string) {
	if r.notShellOwn == nil {
		r.notShellOwn = map[string]bool{}
	}
	r.notShellOwn[name] = true
}

// shellOwnParameter is the read, and it is a name `unset` has not taken away.
//
// A parameter a script removes stops being the shell's: the name is gone
// until something brings it back, and a listing or a type query that still
// called it special would be describing a record rather than a parameter.
func (r *Runner) shellOwnParameter(name string) bool {
	return !r.removed[name] && r.shellOwn[name]
}
