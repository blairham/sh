// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// The two opt-in lints about *where* an assignment inside a function landed.
//
// Neither changes what a script computes: the assignment behaves the same way
// in both states of each switch, and what is added is a sentence on standard
// error that somebody asked for. They are here rather than in a dialect for
// the reason every switch in sessionswitches.go is — the *question* is about
// this shell's scoping and not about any shell's option namespace — and the
// two sentences are Diagnostics values, so a dialect that has no wording says
// nothing however the switches stand.
//
// # What was measured
//
// Every row is zsh 5.9.2 (`/opt/homebrew/bin/zsh`, `-f`), 2026-09-26, over a
// script file with both options on, reading standard error alone:
//
//	f() { gv=1 }; f                         gv created globally in function f
//	f() { gv=1 }; f; l() { gv=9 }; l        gv set in enclosing scope in l
//	q() { gq=1; gq=2 }; q                   both, in that order, from one body
//	g() { typeset -g tg=1 }; g              nothing
//	h() { local lv=2 }; h                   nothing
//	m() { export ev=1 }; m                  nothing
//	n() { unset gv }; n                     nothing
//	o() { for lo in 1; do :; done }; o      lo created globally in function o
//	p() { read pv <<< hi }; p               pv created globally in function p
//	w() { : ${wv:=1} }; w                   wv created globally in function w
//	v() { vv+=x }; v                        vv created globally in function v
//	x() { printf -v pv2 hi }; x             pv2 created globally in function x
//	i() { arr=(a b) }; i                    arr created globally, **array**
//	r() { local -a la; la=(x) }; r          nothing
//	top-level `tv=1`                        nothing, with no function to name
//
// Four things come out of that and each of them is a rule here:
//
//   - **The two are complementary rather than alternatives.** The name being
//     created is the first sentence and the name already existing outside this
//     call is the second, which is why `q` above draws one of each from two
//     assignments to one name.
//   - **A declaration is exempt**, all five spellings of it. That is not a
//     list of builtin names here: a declaration's store carries
//     assignedByDeclaration, and its array operand is marked by the one route
//     that knows — see Runner.writingADeclarationsOperand.
//   - **`local` in *this* call is what silences it**, and a `local` in a
//     calling function is not: that is the second sentence's whole subject.
//   - **There has to be a function *this shell* is inside.** A `{ … }` block
//     is not a scope — measured, `b() { { bv=1 } }` still creates a global —
//     the top level draws nothing at all, and neither does a subshell: `t() {
//     (tv=1) }` is silent while `t() { (f() { tv=1 }; f) }` names `f`. That
//     pair is why the test is Runner.insideFunctionCall rather than a name
//     being set, since the enclosing function's name is still there.
//
// # What is measured and deliberately not modeled
//
// An **element** write that creates the name: measured, `a() { arr2[1]=x }`
// draws `scalar parameter arr2 created globally`, with the word `scalar` for
// a name the same statement makes an array of. An element write to a name
// that already exists outside the call draws nothing at all, in either
// switch — `typeset -A ex` at the top level and `ex[k]=v` inside a function
// is silent. Both rows are left alone rather than guessed at: the first needs
// a word this shell would have to choose against its own reading of the
// statement, and neither is what the lint is for, which is an assignment a
// `local` was forgotten in front of. That is the plain route only: an
// element *arithmetic* writes is an assignment to the array, see
// scopeWarningNumeric.
//
// An arithmetic element write to an **associative** array is measured and
// not modeled either: the reference names the *key* rather than the table —
// `typeset -A h; f() { (( h[zz]=3 )) }` draws `numeric parameter zz set in
// enclosing scope` with no `zz` set anywhere, and a `local k` does not
// silence `(( h[k]=3 ))` — which is not a sentence about any parameter this
// shell could name.

// scopeWarningKind is the word the sentence uses for what the name holds.
type scopeWarningKind string

const (
	scopeWarningScalar scopeWarningKind = "scalar"
	scopeWarningArray  scopeWarningKind = "array"
	// scopeWarningNumeric is the word for a store **arithmetic** makes, and
	// the word follows that route rather than what the name holds. Measured
	// 2026-10-15 on zsh 5.9.2 under `-f`, both lints on, each row the body of
	// a function after the setup shown:
	//
	//	x=5; x=8 / x+=1 / read / for x in       scalar … set in enclosing scope
	//	integer g=5; g=8 / g+=1 / read / for    scalar …, the same
	//	float g=5; g=8; MAILCHECK=6             scalar …, the same
	//	x=5; (( x=8 )) / let / $(( )) / x=abc   numeric … set in enclosing scope
	//	integer g=5; (( g=8 )); float alike     numeric …, the same
	//	x=5; (( x++ )) / ++x / x-- / x+=1       nothing
	//	  / x*=2 / let x++ / $(( x++ ))
	//	integer g=5; (( g++ )) / g+=1; float    nothing
	//	(( y=1 )) / (( y++ )) / (( y+=1 ))      numeric … created globally
	//	a=(1 2); (( a[1]=3 )) / let / a[3]      numeric parameter a set …
	//	a=(1 2); (( a[1]++ )) / a[1]+=1         nothing
	//	(( n[1]=3 )) / (( n[1]++ ))             numeric parameter n created …
	//	a=(1 2); a[1]=3                         nothing (the plain route)
	//
	// So the two pairs that hold the type fixed and move the route — `x=8`
	// against `(( x=8 ))` on a scalar, and on an integer — both move the
	// word, and the pairs that hold the route fixed and move the type move
	// nothing. A read-modify-write is the other half of the route: it never
	// reports a name that is already set, and still reports one it creates,
	// which is why `for (( y=0; y<1; y++ ))` is one sentence and not two
	// (#6461). See arithStoreRoute.
	scopeWarningNumeric scopeWarningKind = "numeric"
)

// arithStoreRoute is which arithmetic store is being made, the one thing the
// scope lints read about how an assignment was written. See
// scopeWarningNumeric.
type arithStoreRoute uint8

const (
	// arithNotStoring is every store that is not arithmetic's own.
	arithNotStoring arithStoreRoute = iota
	// arithAssigns is a plain `=` — `(( x=8 ))`, `let x=8`, `$(( x=8 ))`.
	arithAssigns
	// arithModifies is a read-modify-write — `++`, `--`, `+=` and the other
	// compound operators.
	arithModifies
)

// warnAboutTheScope writes whichever of the two sentences this assignment has
// earned, and nothing where neither switch is on.
//
// Called *before* the store, because both questions are about the state the
// assignment found: a check behind it would see the name the write had just
// made and report every creation as a set.
func (r *Runner) warnAboutTheScope(name string, kind scopeWarningKind) {
	if !r.warnsGlobalCreatedInAFunction && !r.warnsNestedSetHere() {
		return
	}
	if r.writingADeclarationsOperand || r.inFunc == "" {
		return
	}
	// A name the running function made local answers almost every store a
	// function makes, and it is one lookup in one scope. Every question below
	// it is an early return with nothing to write, so which of them is asked
	// first changes no answer — and asked first, this one spares the rest:
	// scopeLintPassesOverTheShellsOwn alone is three producer tables and two
	// attribute reads, and it was 1.4% of an F-Sy-H highlight call (#6423).
	if r.localInTheInnermostScope(name) {
		return
	}
	if r.engineOwns(name) {
		// The engine's own state, written on the script's behalf by
		// something the script did — `print -P %B` setting the terminal up,
		// `fc -p` pushing a history, `rand48()` advancing its seed. No
		// script made the assignment, so neither sentence is about one.
		// Measured 2026-10-12 on zsh 5.9.2 under warn_create_global and
		// warn_nested_var: each of those inside a function is silent, while
		// `x=1` beside them still draws its sentence (#6422).
		return
	}
	if r.scopeLintPassesOverTheShellsOwn(name) {
		return
	}
	if r.mirroring {
		// The other half of a tie, written because the script wrote this
		// one: one assignment, and one sentence for it. Measured 2026-10-15
		// on zsh 5.9.2, `typeset -T FOO foo; f() { foo=(c) }` under
		// warn_nested_var names `array parameter foo` alone, where this
		// shell also wrote the scalar `FOO` the mirror set.
		return
	}
	if r.storingATakenBackPrefix {
		// A prefix the command's end takes back is not an assignment that
		// landed anywhere. See Runner.storePrefixQuietlyIfTakenBack.
		return
	}
	if !r.insideFunctionCall() {
		// A call *this* shell made, which is not the same as a call the
		// process is inside: a subshell inherits its caller's frames as
		// state and is not running in them. Measured — `t() { (tv=1) }` is
		// silent where `t() { (f() { tv=1 }; f) }` names `f` — so a copy of
		// the frame count would have reported the enclosing function for an
		// assignment it never saw. See Runner.funcFloor.
		return
	}
	if !r.locationIsInsideAFunctionBody() {
		// The line being run is the caller's rather than this body's — a
		// function that sources a file is still the innermost function while
		// the file runs, and what the file does is not this call's doing.
		// The same rule a diagnostic's location turns on.
		return
	}
	if kind == scopeWarningScalar && r.arithStoring != arithNotStoring {
		// The route and not the name's type: an integer written by `g=8` is
		// `scalar`, a string written by `(( x=8 ))` is `numeric`. See
		// scopeWarningNumeric.
		kind = scopeWarningNumeric
	}
	d := r.diag()
	defer r.scopeWarningSpeaksForTheShell()()
	if r.nameIsSet(name) {
		if r.arithStoring == arithModifies {
			// `(( x++ ))` on a name that is already there. See
			// scopeWarningNumeric.
			return
		}
		if r.warnsNestedSetHere() && d.EnclosingScopeSetInAFunction != "" {
			r.diagf(d.EnclosingScopeSetInAFunction+"\n", kind, name, r.inFunc)
		}
		return
	}
	if r.warnsGlobalCreatedInAFunction && d.GlobalCreatedInAFunction != "" {
		r.diagf(d.GlobalCreatedInAFunction+"\n", kind, name, r.inFunc)
	}
}

// scopeWarningSpeaksForTheShell takes the speaking builtin out of the
// location for the length of one sentence, and puts it back.
//
// The lint is a remark the *shell* makes about a scope, not a complaint the
// builtin that happened to make the write is raising — measured, `p() { read
// pv <<< hi }` and `x() { printf -v pv2 hi }` are located `p:` and `x:` in
// the reference where every ordinary refusal either of them raises carries
// the builtin's name. It is the same rule a trace prefix follows, and for the
// same reason: no builtin is speaking there either.
func (r *Runner) scopeWarningSpeaksForTheShell() func() {
	speaker, inBuiltin := r.speaker, r.inBuiltin
	r.speaker, r.inBuiltin = "", ""
	return func() { r.speaker, r.inBuiltin = speaker, inBuiltin }
}

// scopeWarningsAreOff is the cheap test the two call sites make before doing
// any work at all, so a shell nobody has asked for the lint pays a bool.
func (r *Runner) scopeWarningsAreOff() bool {
	return !r.warnsGlobalCreatedInAFunction && !r.warnsEnclosingScopeSet && len(r.warnNestedFuncs) == 0
}

// warnsNestedSetHere reports whether a set in an enclosing scope is warned
// about in the body running now: the option, or a `functions -W` mark on the
// innermost function. The mark is the body's alone — measured 2026-10-02 on
// zsh 5.9.2, a function the marked one defines and calls warns about
// nothing, and E01options' WARN_NESTED_VAR row has `fn2` silent under the
// attribute where the option names it (#5155).
func (r *Runner) warnsNestedSetHere() bool {
	return r.warnsEnclosingScopeSet || r.warnNestedFuncs[r.inFunc]
}

// storePrefixQuietlyIfTakenBack is Runner.prefixStore with the two scope
// lints held off where the command's end takes the prefix back.
//
// Measured 2026-10-02 on zsh 5.9.2 under `-f`, with `warncreateglobal` on
// and with `functions -W` on the function: `f(){ g=2 print hi }`, `g=3 true`,
// `g=4 command true`, `g=6 eval :` and `g=2 f2` in front of a function each
// write nothing, where this shell wrote the created-globally or the
// set-in-enclosing-scope sentence for every one (#5485). A prefix that
// *persists* is an assignment that stays, and is warned about like one: under
// `posixbuiltins`, `f(){ g=2 :; }` draws `g created globally in function f`.
func (r *Runner) storePrefixQuietlyIfTakenBack(
	ctx context.Context,
	a *syntax.Assign,
	landsOn string,
	stores, fresh, takenBack bool,
) {
	if takenBack {
		outer := r.storingATakenBackPrefix
		r.storingATakenBackPrefix = true
		defer func() { r.storingATakenBackPrefix = outer }()
	}
	r.prefixStore(ctx, a, landsOn, stores, fresh)
}

// scopeLintPassesOverTheShellsOwn reports whether an assignment is to one of
// the shell's own parameters, which neither lint reports.
//
// The lint is about a `local` a script forgot, and a script does not declare
// `$HOME` or `$PS1` before writing them. Measured 2026-10-15 on zsh 5.9.2
// under `env -i PATH=/usr/bin:/bin TERM=xterm`, `-f`, each assignment inside
// a function with both lints on:
//
//	HOME HISTSIZE PS1 TERM IFS RANDOM SECONDS COLUMNS SHLVL USERNAME
//	NULLCMD WORDCHARS histchars SAVEHIST PROMPT prompt OPTIND
//	PATH path fpath psvar                         nothing
//	LANG LC_ALL LC_CTYPE TERMINFO, all unset      nothing
//	LOGNAME MAILCHECK TTY KEYTIMEOUT ZSH_NAME
//	ZSH_SCRIPT EDITOR TMPDIR HISTFILE REPLY       the sentence
//
// The key is the word `special` — [ParameterAttributes.Provided] — and the
// pairs that hold the kind fixed say so: `HOME` (scalar-export-special) is
// silent and `LOGNAME` (scalar-export) is not, `HISTSIZE` (integer-special)
// beside `MAILCHECK` (integer), `PS1` (scalar-special) beside `TTY`
// (scalar). It is the *name's* place among the shell's own and not its
// present value: the unset `LANG` family is silent, and so is `HOME` or
// `RANDOM` after a top-level `unset` — those are the two sources beside the
// word, the marks that outlive a missing value.
//
// A `local -h` in a calling function is the control in the other direction:
// it makes the binding an ordinary parameter, and `g` writing `HOME` under
// it draws `set in enclosing scope` in the reference, where a plain `local
// HOME` in the same place is silent (#6425). It is asked separately because
// the marks above are per name and outlive any binding: under `local -h
// LANG` in the caller, `LANG=x` draws the sentence in the reference too.
func (r *Runner) scopeLintPassesOverTheShellsOwn(name string) bool {
	if r.notShellOwn[name] || r.shadowIsHidden(name) {
		return false
	}
	return r.parameterIsProvided(name) || r.shellOwnWhileSet[name] || r.removedShellOwn[name]
}
