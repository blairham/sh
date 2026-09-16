// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// `set -k`, POSIX's *keyword* option: with it on, **every** `name=value` word
// of a simple command is a prefix assignment and not only the ones written in
// front of the command name.
//
// Two of the panel have it, one spells a different option with the same
// letter, and two have neither. Measured 2026-09-16, C locale, from a script
// file, with `f() { echo "1=[$1] KV=[${KV-unset}] n=$#"; }` and `f KV=2 a`:
//
//	bash 5.3.20, bash-as-sh, bash 3.2.57   1=[a] KV=[2] n=1
//	ksh93u+ 2012-08-01                     1=[a] KV=[2] n=1
//	zsh 5.9.2                              1=[KV=2] KV=[unset] n=2
//	dash 0.5.12                            set: Illegal option -k, and the file ends
//	BusyBox ash 1.37.0                     set: illegal option -k, and the file ends
//
// **zsh's `-k` is `interactivecomments`**, a different option entirely, so the
// letter is not the option and this axis is not about the letter. zsh reaches
// `set -k` through its own letter table and never arrives here; zsh's `set -o
// keyword` is `no such option`, which is its own answer too. dash and BusyBox
// ash have neither spelling, and their refusal is the right answer rather than
// a gap — which is what makes this an axis and not a plain core gap.
//
// # What the two shells that have it agree about
//
// Everything below was measured on both, from a script file, with a neutral
// argument word — `r` is a preset alias for `hist -s` in ksh93 and an argument
// in the command-word position is alias-expanded once an assignment stands in
// front of it, which is how a probe using `r` comes to report two arguments
// and an argument nobody wrote:
//
//   - several of them on one command, each taken: `f A=1 x B=2` is `$1=x`,
//     `$#=1`, and both names assigned
//   - a `--` does not stop it: `f -- A=3` leaves `$1=--` and assigns `A`
//   - the value may hold anything a written assignment's may — `Q=$V`,
//     `Q="$V"`, `Q=x$V`, `Q=$(echo cs)`, `Q=$((1+1))` and `Q=~` are all
//     promoted, and the value is never split and never a pattern
//   - the **name** must be written, unquoted and a name: `'Q=q'` stays a
//     positional in both, and so does `1Q=b`
//   - it reaches builtins, functions and external commands alike, and an
//     external is handed the name in its environment — which is what POSIX's
//     "placed in the environment for a command" says it is for
//   - `set +k` and `set +o keyword` turn it off, and `$-` carries `k` while
//     it is on
//
// Whether the assignment outlives the command is *not* this axis: bash drops
// it and ksh93 keeps it, exactly as each does for an assignment written in
// front of a function, which is AssignmentPrefixPersistsAfterAFunction. A promoted
// assignment is an ordinary prefix assignment from the moment it is promoted,
// and every axis that already answers for one answers for it.
//
// # Recorded rather than modeled
//
// **ksh93 decides at parse time.** `set -k` on its own line reaches the lines
// after it, and `set -k; f Q=1 zz` on *one* line reaches nothing — measured
// both ways round, from a file and under `-c`. bash decides when the command
// runs and both spellings work there. This engine answers as bash does, which
// agrees with ksh93 for the way the option is written in practice and in the
// issue's own repro, and differs from it on a one-liner. Modeling it would
// mean the option state a command was *parsed* under, which is a fact this
// runner does not keep and a front end reading a person's input does not have.
//
// **A subscripted name is not promoted here.** `f arr[0]=v zz` is `$1=zz` in
// both references — the word is taken — but bash then refuses the name,
// ``arr[0]': not a valid identifier``, and assigns nothing, while ksh93
// assigns the element silently. Two answers to one word, neither of them the
// obvious one, and a promotion that had to carry a subscript's own expansions
// to be right in the second. Ours leaves the word a positional, which is a
// third answer and the only one that loses nothing.

// keywordPromotable reports whether an argument word is one `set -k` turns
// into a prefix assignment.
//
// The written word decides, never the expanded one: measured on bash 5.3.20
// and ksh93u+ alike, a word that *expands* to `A=9` stays a positional and a
// quoted `'A=10'` stays one too, while `A=$V` — written as an assignment with
// an expansion in its value — is promoted in both. So this asks the syntax
// tree, and it asks it with assignNameSplit, which is the same test a
// declaration utility's operand already goes through and already refuses a
// quoted name, a name from an expansion and a name that is not one.
//
// A subscript is the one shape assignNameSplit takes that this does not: see
// the note above for the two answers the references give it.
func keywordPromotable(w *syntax.Word) bool {
	span, off, ok := assignNameSplit(w)
	if !ok || span != 0 {
		return false
	}
	for i := 0; i < off; i++ {
		if w.Spans[0].Value[i] == '[' {
			return false
		}
	}
	return true
}

// keywordAssign is the prefix assignment an argument word becomes.
//
// The word is cut at the `=` keywordPromotable found and the halves become an
// [syntax.Assign] exactly as the parser would have built one had the word been
// written in front of the command: the name is the literal text before the
// `=`, and the value is the rest of that span followed by every span after it.
// Nothing about the value is re-read — it is the same spans, so it expands
// once, as an assignment's, and a promoted `A=$(date)` runs its substitution
// no more often than a written one does.
//
// A nil Value for a word with nothing after the `=`, which is what the parser
// produces for a written `A=` and what keeps `f A= x` and `A= f x` one answer.
func keywordAssign(w *syntax.Word) *syntax.Assign {
	_, off, ok := assignNameSplit(w)
	if !ok {
		return nil
	}
	head := w.Spans[0]
	rest := head.Value[off+1:]
	a := &syntax.Assign{Name: head.Value[:off]}
	if rest == "" && len(w.Spans) == 1 {
		return a
	}
	value := *w
	value.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: rest, Quoting: head.Quoting, Pos: head.Pos,
	}}, w.Spans[1:]...)
	a.Value = &value
	return a
}
