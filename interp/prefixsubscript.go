// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A subscripted assignment written as a command **prefix** — the `a[1]=v` in
// `a[1]=v cmd` — is a different question from the same word written as a
// statement, and the panel gives it four answers where this shell gave a
// fifth: it dropped the subscript and made a temporary *scalar* `a`, which is
// a name none of the five would have (#3433).
//
// Where the readings are, and what decides each of them:
//
//   - the **grammar** decides whether the word is an assignment at all. dash
//     and BusyBox ash never make one of it — `a[1]=v f` there is
//     `a[1]=v: not found`, the word in command position — so nothing in this
//     file is reached in those dialects.
//   - Semantics.SubscriptedAssignmentPrefix decides between bash's refusal
//     and ksh93's and zsh's element write.
//   - Semantics.SubscriptedPrefixIsTakenBack decides, where the element is
//     written, whether it is given back at the end of the command the way the
//     scalar beside it is. ksh93 yes, zsh no.
//
// One row is unanimous and so is here rather than in the vector: a prefix in
// front of a command a **child** runs is never seen. An array reaches no
// child's environment, and `arr=(x y z); arr[1]=P /usr/bin/true` leaves `x`
// in both shells that take the word at all. The value is still expanded —
// `arr[1]=$(echo side >&2; echo v) /usr/bin/true` writes `side` in zsh and
// ksh93 — so the route expands it and does nothing with it, rather than
// skipping the word before its right-hand side has run.

// prefixIsSubscripted reports whether an assignment prefix entry names an
// element rather than a name.
//
// [syntax.Assign.Index] alone: it holds the *last* subscript, so a chained
// `a[1][2]=v` answers here through the same field a single one does, and an
// operand — a declaration utility's own `a=(…)`, which is not a prefix —
// never reaches any of this.
func prefixIsSubscripted(a *syntax.Assign) bool {
	return !a.Operand && a.Index != nil
}

// aPrefixIsSubscripted reports whether any entry of this prefix is, which is
// what keeps the axis off the common path: a command with a prefix at all is
// a minority of a script's lines, and one with a subscript in it a minority
// of those.
func aPrefixIsSubscripted(assigns []*syntax.Assign) bool {
	for _, a := range assigns {
		if prefixIsSubscripted(a) {
			return true
		}
	}
	return false
}

// subscriptedPrefixSpelling is the name a refusal quotes back: the name and
// every subscript exactly as the source wrote them.
//
// Unexpanded, which is measured and not a shortcut — bash 5.3.20 quotes
// `i=1; a[$i]=v cmd` back as `a[$i]` — and it is also what keeps a refusal from
// evaluating a subscript the refusing shell never evaluates: the complaint
// about `a[$((1/0))]=v cmd` there is this sentence and not a division.
//
// [syntax.Assign.IndexText] is the spelling and [syntax.PrintWord] the
// fallback, for a tree built by hand rather than by the parser.
func subscriptedPrefixSpelling(a *syntax.Assign) string {
	var b strings.Builder
	b.WriteString(a.Name)
	for _, lead := range a.Leading {
		b.WriteByte('[')
		b.WriteString(subscriptSubject(lead.Text, syntax.PrintWord(lead.Index)))
		b.WriteByte(']')
	}
	b.WriteByte('[')
	b.WriteString(subscriptSubject(a.IndexText, syntax.PrintWord(a.Index)))
	b.WriteByte(']')
	return b.String()
}

// refuseSubscriptedPrefixes reports every subscripted entry of this command's
// prefix, in the dialect that refuses them.
//
// Called before the command's values are expanded and before its redirections
// are opened, because that is where bash puts it: `a[1]=v f >/nope/x` writes
// the identifier complaint and *then* the file's, `a[1]=$(echo side) f` never
// runs the substitution, and under `set -x` the complaint stands ahead of the
// trace — which writes the command with the refused word left out of it.
//
// It reports and does not stop the command: the command runs, the status is
// the command's own, and a prefix of several refused words gets a line each.
// The entries the routes then skip are decided by subscriptedPrefixDropped,
// which asks the vector again rather than being handed a list — a prefix is
// one, two or three assignments, and a list is the shape of state a route
// forgets to clear.
func (r *Runner) refuseSubscriptedPrefixes(assigns []*syntax.Assign) {
	if !r.subscriptedPrefixRefusalAnswered(assigns) {
		return
	}
	for _, a := range assigns {
		if !prefixIsSubscripted(a) {
			continue
		}
		r.refuseOneSubscriptedPrefix(a)
	}
}

// subscriptedPrefixRefusalAnswered settles the dialect's reading of a
// subscripted prefix and reports whether there is anything left to refuse.
//
// Split out of the loop so that the **ordered** walk can share it: the reading
// is one question about the command and the refusal is one line per word, and
// a second copy of the first is how the two walks would come to refuse in
// different shells. See interp/prefixredirorder.go.
func (r *Runner) subscriptedPrefixRefusalAnswered(assigns []*syntax.Assign) bool {
	if !aPrefixIsSubscripted(assigns) {
		return false
	}
	switch r.sem().SubscriptedAssignmentPrefix {
	case SubscriptedPrefixStoresTheElement:
		return false
	case SubscriptedPrefixIsRefused:
		return true
	}
	r.diagf("%s\n", r.unanswered("a subscripted assignment written as a command prefix"))
	r.status, r.unspecified = 2, true
	return false
}

// refuseOneSubscriptedPrefix is the complaint for one such word.
func (r *Runner) refuseOneSubscriptedPrefix(a *syntax.Assign) {
	r.diagf("%s\n", Wording(r.diag().SubscriptedPrefixIsNotAName,
		"`%[1]s': not a valid identifier", subscriptedPrefixSpelling(a)))
}

// subscriptedPrefixDropped reports whether a prefix entry is one this dialect
// does not assign at all: the refused reading, and the unanswered one, which
// refuseSubscriptedPrefixes has already reported for the whole command.
func (r *Runner) subscriptedPrefixDropped(a *syntax.Assign) bool {
	return prefixIsSubscripted(a) &&
		r.sem().SubscriptedAssignmentPrefix != SubscriptedPrefixStoresTheElement
}

// subscriptedPrefixReachesAChild reports whether the command this prefix
// stands in front of is one a child process runs, so that an element store
// would be the child's and is therefore not made at all.
//
// Two ways to be one, and the second is why this is not simply the resolved
// kind. `command` is transparent in ksh93 — `arr[1]=A1 command :` keeps `A1`
// there, exactly as the bare `:` does — and in zsh `command` asks for an
// external program alone, so `command :` is `command not found: :` and the
// element is not kept. That is Semantics.CommandReachesABuiltin, read here
// rather than measured again.
func (r *Runner) subscriptedPrefixReachesAChild(p prefixCommand) bool {
	if p.kind == prefixBeforeExternal {
		return true
	}
	if !p.throughCommand {
		return false
	}
	return !r.ask(r.sem().CommandReachesABuiltin,
		"`command` in front of a builtin running that builtin")
}

// subscriptedPrefixTakenBack reports whether this entry's store is given back
// when the command is over, the way the scalar prefix beside it is.
//
// True for every entry that is not subscripted, so the axis is asked only
// where the two shells that store an element disagree — see
// Semantics.SubscriptedPrefixIsTakenBack.
func (r *Runner) subscriptedPrefixTakenBack(a *syntax.Assign) bool {
	if !prefixIsSubscripted(a) {
		return true
	}
	return r.ask(r.sem().SubscriptedPrefixIsTakenBack,
		"a subscripted assignment prefix given back the way the scalar beside it is")
}
