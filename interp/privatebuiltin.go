// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// `private`, which is `local` making a binding the functions this call goes
// on to invoke read straight past.
//
// One shell in the panel has the word and the others do not, which makes it a
// dialect's answer rather than an axis — the same shape `integer` and `float`
// have, and it is registered through the extension seam for the same reason.
// What it is **not** is a second declaration builtin: a private declaration
// is [Runner.declareNames] and [Runner.shadow] here exactly as `local` is, so
// the letters, the three kinds of store, the freeze, the listing and every
// refusal come from the one place. A copy would have drifted the first time
// one of those was measured again — see biInteger, where the same argument is
// made about the same machinery.
//
// The two things the second word decides for itself are both dialect tables:
// which letters it takes, which is [Semantics.PrivateOptions] and is neither
// `local`'s nor `typeset`'s; and what its refusals call it, which follows the
// invoked name through r.inBuiltin the way every other builtin's do.
//
// The third thing is what makes it a different word at all, and it is not
// here: the binding it makes is marked, and a function call steps over it.
// interp/privatescope.go is the whole of that, with the measured table.
//
// ## The module it belongs to, and why the word is not gated on it
//
// In the shell being modeled this word arrives with `zsh/param/private`, and
// the module is **autoloaded by the word**: measured 2026-09-27 on zsh 5.9.2
// under `-f`, `zmodload` lists no such module before `private x=1` and lists
// it after, and `whence -w private` moves from `builtin` to `reserved` across
// that line. So a script that never writes `zmodload` still gets the word,
// with the full visibility rule behind it — the measured table in
// privatescope.go was taken both with the module loaded and without, and the
// two are identical.
//
// This shell does not autoload anything and never will, so the word is
// registered from the start and `zmodload zsh/param/private` loads because
// the one feature the module names is present. That is the same rule every
// other module here loads under, arrived at from the other end.
//
// Two divergences follow from that and are recorded rather than papered over,
// and both are the same fact read from two sides — this shell has one state
// where the reference has two.
//
//   - `whence -w private` is `builtin` here. That is exactly what the
//     reference answers **before** the module is loaded, and not what it
//     answers after, which is `reserved`.
//   - `local -P` works here from the start. In the reference the letter is a
//     bad option until the module is loaded — and unlike the word, `local -P`
//     does **not** autoload it, so a script that writes the letter without the
//     `zmodload` gets a refusal there and a private binding here.
//
// Modeling either would mean modeling autoload, which is a mechanism this
// shell has not got and which nothing else in it would use. A script that
// writes the `zmodload` — which is every script the letter was measured
// through, and the shape the module's own suite file opens with — sees the
// same shell either way.

// PrivateBuiltin is the `private` builtin, for a dialect that has the word to
// register.
//
// Exported for the same reason [IntegerBuiltin] is: a dialect package cannot
// reach an unexported function, and reimplementing the declaration in
// `dialect/zsh` is a second copy of the thing this package exists to hold
// once.
func PrivateBuiltin() Builtin { return biPrivate }

func biPrivate(r *Runner, ctx context.Context, args []string) int {
	status := 0
	r.DeclaringPrivateName(func() { status = biLocal(r, ctx, args) })
	return status
}

// localDeclarationWord is the name `local` and the words built on it report
// their refusals under.
//
// r.inBuiltin is the word the script actually wrote, which is what every
// other builtin's refusals follow — measured, `private -g v` in the shell
// with the word is “f:private: bad option: -g“ and names `private` rather
// than `local`. Defaulted rather than assumed present for biInteger's
// reason: a caller that reaches the builtin without going through the
// dispatcher would otherwise produce a refusal with no command in it.
func (r *Runner) localDeclarationWord() string {
	if r.inBuiltin != "" {
		return r.inBuiltin
	}
	return "local"
}

// localDeclarationOptions is the letter set the running declaration word
// takes.
//
// Asked of the *word* and not of the dialect alone, because the two words
// this builtin serves have different sets in the one shell that has both —
// see [Semantics.PrivateOptions] for the letter-at-a-time measurement. The
// flag rather than the name, because it is the flag that decides what the
// declaration will mean; a word registered on this builtin that did not set
// it would be `local` under another spelling, which is a thing a dialect may
// legitimately want.
func (r *Runner) localDeclarationOptions() string {
	if r.declaringPrivate {
		return r.sem().PrivateOptions
	}
	return r.sem().LocalOptions
}
