// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

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
// `local` was forgotten in front of.

// scopeWarningKind is the word the sentence uses for what the name holds.
type scopeWarningKind string

const (
	scopeWarningScalar scopeWarningKind = "scalar"
	scopeWarningArray  scopeWarningKind = "array"
	// scopeWarningNumeric is the word for a name arithmetic declares as it
	// creates it. Measured 2026-10-02 on zsh 5.9.2, `f() { setopt
	// warncreateglobal; (( g=1.5 )); let h=2 }; f` names both `numeric
	// parameter` (#5155).
	scopeWarningNumeric scopeWarningKind = "numeric"
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
	if r.localInTheInnermostScope(name) {
		return
	}
	if kind == scopeWarningScalar {
		// A number arithmetic is creating, or a name already holding one:
		// measured 2026-10-02 on zsh 5.9.2, `integer g=5; f() { (( g=8 )) }`
		// under the nested lint is `numeric parameter g set in enclosing
		// scope` (#5155).
		if a := r.numericAttributeOf(name); r.creatingANumber || a.isInteger || a.isFloat {
			kind = scopeWarningNumeric
		}
	}
	d := r.diag()
	defer r.scopeWarningSpeaksForTheShell()()
	if r.nameIsSet(name) {
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
