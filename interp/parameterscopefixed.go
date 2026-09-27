// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A name whose binding the *shell* holds, which a declaration may not move
// into a scope of its own.
//
// The shell in the panel with a second declaration word — see
// privatescope.go, and [PrivateBuiltin] for the word — refuses that word over
// most of the parameters it owns, in the same sentence it refuses a
// redeclaration with. It is not a refusal `local` or `typeset` shares: those
// two shadow `PATH`, `HOME` and `path` happily in that shell and in this one.
// What `private` asks for is a binding a deeper frame reads *past*, and a
// parameter the shell reaches through a slot of its own has nowhere for such
// a binding to stand.
//
// ## The set is measured, one name at a time
//
// It is not derivable from anything a script can see, which is why this is a
// list rather than a rule. Swept 2026-09-27 against `/opt/homebrew/bin/zsh`,
// `zsh 5.9.2 (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go
// executable* for it and `github.com/blairham/sh/cmd/zsh` for ours, so these
// are two programs — one name per run, `-f` from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, every row behind
// `zmodload zsh/param/private`, and the whole of each row being
//
//	f() { private NAME; print "st=$?" }
//	f
//
// 177 candidates: every name `typeset +` lists in a fresh `-f` shell, plus
// every parameter the manual's *Parameters Used By The Shell* section names
// that such a shell leaves unset. **80 refuse and 97 are taken**, and the
// instrument produced both answers in the same sweep, which is what says it
// could have produced either for any row.
//
// ## Three readings the sweep rules out
//
// **"Every parameter the shell provides."** `PWD`, `OLDPWD`, `HOST`,
// `OSTYPE`, `MACHTYPE`, `CPUTYPE`, `VENDOR`, `LOGNAME`, `TTY`, `ZSH_NAME`,
// `ZSH_VERSION`, `ZSH_PATCHLEVEL`, `ZSH_ARGZERO`, `MAILCHECK`, `KEYTIMEOUT`,
// `LISTMAX`, `TIMEFMT` and `TMPPREFIX` are all the shell's and all taken.
//
// **"Every parameter with a producer behind it."** `RANDOM`, `SECONDS` and
// `LINENO` refuse, and so do `HOME`, `IFS` and `path`, which hold ordinary
// stored values.
//
// **"Every tie."** Both halves of all eight built-in ties refuse — but a
// script's *own* tie does not: `typeset -T TT tt` in front, and `private tt`
// and `private TT` inside a function are both status 0. So the refusal is
// about which names the shell hardwired and not about the mechanism.
//
// What comes closest from the outside is `${(t)}`: over the 123 names a fresh
// shell has set, *refused* and *`special` without `hide`* agree on 121, the
// two exceptions being `watch` (`array-special`) and `WATCH`
// (`scalar-special`), which are taken. And that near-rule breaks outright on
// the wider sweep, because `TERM`, `LANG`, the six `LC_*`, `RPROMPT`,
// `RPS1`, `RPROMPT2`, `RPS2`, `POSTEDIT`, `TERMINFO`, `TERMINFO_DIRS` and
// `ZLE_RPROMPT_INDENT` are all unset in that shell — `${(t)}` is empty for
// every one of them — and every one of them refuses. The slot is there
// whether or not a value is. Which is also the answer to `unset`: `unset
// path; private path` and `unset HOME; private HOME` both still refuse, so
// this table is deliberately *not* the one [Runner.shellOwnParameter] reads,
// where a name a script removes stops being the shell's.
//
// ## Where it does not apply
//
// **The top level takes every one of them.** `private path` outside a
// function prints `path=( /usr/bin /bin )` at status 0 — it is a listing
// there, the same as `typeset path` — and `private HOME=/x` outside a
// function assigns. Measured the same day, and in a subshell and a sourced
// file at the top level too, both of which are taken. A declaration that
// takes no shadow is not moving a binding anywhere, so there is nothing for
// this to refuse; see [Runner.shadow], whose `len(r.scopes) == 0` is the same
// line. An **anonymous** function refuses, in its own location:
// `(anon):private: can't change scope of existing param: path`.

// MarkParameterScopeFixed records that a name's binding is the shell's own
// and may not be moved into a function's scope.
//
// One name per call, and written out by the dialect that measured them. The
// same argument [Runner.MarkShellOwnParameter] makes applies twice over here:
// there is no hook this could ride on that would get the set right, because
// the set is not the names a hook stores — `$HOST` is stored through
// [Runner.SetSpecial] beside `$IFS` and is taken where `$IFS` refuses, and
// `TERM` has no value at all in the shell that refuses it.
func (r *Runner) MarkParameterScopeFixed(name string) {
	if r.scopeFixed == nil {
		r.scopeFixed = map[string]bool{}
	}
	r.scopeFixed[name] = true
}

// parameterScopeFixed is the read.
func (r *Runner) parameterScopeFixed(name string) bool {
	return r.scopeFixed[name]
}
