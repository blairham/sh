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

// MarkShellOwnParameterWhileSet records that a name is the shell's own
// **while it holds a value**, and an ordinary absent name otherwise.
//
// MarkShellOwnParameter is a name the shell *has*: marking one makes it exist,
// so `${(t)HOME}` answers whatever a script did to it. A second set of names
// is the shell's own and still absent until something sets it. Measured
// 2026-10-03 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f`:
//
//	                  unset          after `NAME=x`    after `export NAME=y`
//	${(t)TERM}        (nothing)      scalar-special    scalar-export-special
//	${(t)LANG}        (nothing)      scalar-special    scalar-export-special
//	${(t)HISTFILE}    (nothing)      scalar            scalar-export  ← control
//
// and `TERMINFO`, `TERMINFO_DIRS`, `LC_ALL`, `LC_CTYPE`, `LC_COLLATE`,
// `LC_MESSAGES`, `LC_NUMERIC`, `LC_TIME`, `RPROMPT`, `RPS1`, `RPROMPT2` and
// `RPS2` answer as `TERM` does (#5575). The mark is the fact the word is read
// off, and it is also what an `unset` asks before deciding the attributes
// stay: `export TERM=x; unset TERM; TERM=y` hands a child `TERM=y` there.
func (r *Runner) MarkShellOwnParameterWhileSet(name string) {
	if r.shellOwnWhileSet == nil {
		r.shellOwnWhileSet = map[string]bool{}
	}
	r.shellOwnWhileSet[name] = true
}

// ShellOwnParameter reports whether this name is one the shell itself
// maintains, which is the mark above read back.
//
// For a dialect asking whether it *has* a name at all — a module's feature
// gate is the one caller, and the question it is really asking is "would a
// script find this parameter here", to which a name the shell owns is yes
// whether or not anything has referred to it yet. See
// dialect/zsh/zmodload.go, where a deferred `$WATCH` read as missing and shut
// the gate on the module that owns it.
func (r *Runner) ShellOwnParameter(name string) bool { return r.shellOwn[name] }

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
