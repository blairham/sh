// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"sort"
	"strconv"

	"github.com/blairham/sh/interp"
)

// `$history`: the list `fc` keeps, presented as an association from a history
// event number to the command recorded under it.
//
// The eleventh view of the `zsh/parameter` module and the one a real prompt
// asks for on **every keystroke**. zsh-autosuggestions' default strategy is a
// single line of it:
//
//	typeset -g suggestion="${history[(r)$pattern]}"
//
// so while the name refused, a person typing at this shell's prompt got
// `_zsh_autosuggest_strategy_history:22: history: parameter not implemented
// yet` per character, written over the line they were typing, and never a
// suggestion (#4408).
//
// Measured 2026-09-24 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) under
// `zsh -f`, with five entries put in by `fc -R`:
//
//	${#history}          5
//	${(k)history}        5 4 3 2 1
//	${(v)history}        ls gamma · echo three · ls beta · echo two · ls alpha
//	${history[5]}        ls gamma            an event number is the key
//	${history[999]}      ``                  and a number the list has not got
//	${history[(r)ls*]}   ls gamma            the newest match
//	${history[(R)ls*]}   ls gamma ls beta ls alpha
//	${(t)history}        association-readonly-hide-hideval-special
//	typeset -m history   history=( [5]='ls gamma' … [1]='ls alpha' )
//
// **Newest first is the whole of what makes it useful**, and it is not a
// presentation choice: `(r)` takes the *first* value the pattern matches in
// scan order, so the order is what decides whether a suggestion is the command
// somebody ran a minute ago or one from last year. This engine's tables read
// in sorted key order, which is wrong here twice over — it is the oldest
// first, and as strings it is not even history order, since `"10" < "9"`. See
// [interp.Runner.SetDynamicAssocKeyOrder], which exists for this one name.
//
// No `zmodload zsh/parameter` is needed, the same as the ten views beside it:
// the probe above ran without one and answered.
//
// # The table leaves out the current event, and where that entry is differs
//
// zsh's `$history` omits the event numbered `$HISTCMD`, which is always the
// **newest entry of zsh's own list** — so there the rule is one sentence.
// Measured in the script above, `fc -R` of a six-line file leaves `fc -l`
// listing six and `${#history}` saying five.
//
// It is one sentence there because zsh's list carries the line that has not
// run yet. While a command runs, that line is the command; **while somebody
// is typing, it is the line in the editor**, which zsh keeps in the list as
// the in-progress event. This shell's list carries the first and not the
// second: a reader records an accepted line before running it, and nothing
// puts a half-typed line in the list at all.
//
// So the entry to leave out is the newest one, except inside a widget, where
// there is nothing to leave out. Measured 2026-09-24 against zsh 5.9.2 under
// `zsh -f -i` on a pseudo-terminal — the only way to ask — with three
// commands run and a bound widget asked at the prompt with nothing typed:
//
//	WIDGET count=3 HISTCMD=4 newest=echo BBB keys=3 2 1
//
// which is every command the session has run and not one fewer. A view that
// dropped the newest entry there would drop the command the person just ran,
// which is the one a suggestion is most often made of — and that is the
// reading this parameter exists for. See editorRunning, which is the same
// predicate `zle other-widget` is refused outside.
//
// `$historywords` is deliberately still absent. It is the same list cut into
// words, nothing on this machine reads it, and an empty or wrong answer at
// status 0 is worse than the refusal it gives today.
func registerHistoryParameter(r *interp.Runner) {
	r.SetDynamicAssoc("history", zshHistoryView)
	// And the same table read one key at a time, which is what
	// `${history[$((HISTCMD-1))]}` and every `$history[$n]` is. See
	// zshHistoryValue.
	r.SetDynamicAssocElement("history", zshHistoryValue)
	r.SetDynamicAssocKeyOrder("history", zshHistoryNewestFirst)
	r.MarkReadonly("history")
	// `association-readonly-hide-hideval-special`, which is the word every
	// module parameter answers — see hideModuleParameter, where the pair is
	// measured.
	hideModuleParameter(r, "history")
}

// zshHistoryView is the whole table: one key per entry the list still holds,
// numbered from [interp.Runner.HistoryFirst].
//
// The numbers are the list's own and not positions in it. zsh's entries keep
// the number they were given when a `HISTSIZE` trim drops them off the front
// — see fcFirst — so the oldest entry of a trimmed list is numbered by
// everything that has left, and `${history[1]}` on such a list is nothing
// rather than the oldest surviving command.
func zshHistoryView(r *interp.Runner) interp.AssocArray {
	entries := zshHistoryEvents(r)
	first := r.HistoryFirst()
	out := make(interp.AssocArray, len(entries))
	for i, entry := range entries {
		out[strconv.Itoa(first+i)] = interp.Scalar(entry)
	}
	return out
}

// zshHistoryValue is `${history[n]}`: the one event, without building a key
// for every other entry to reach it.
//
// The same answer zshHistoryView gives under the same key, which is the
// contract SetDynamicAssocElement states — a shorter route to that value and
// not a second opinion about it. A key that is not a number the list holds is
// not a key: no value, and `${history[x]+SET}` is unset, exactly as the whole
// table reads.
func zshHistoryValue(r *interp.Runner, key string) (string, bool) {
	n, err := strconv.Atoi(key)
	if err != nil {
		return "", false
	}
	at := n - r.HistoryFirst()
	entries := zshHistoryEvents(r)
	if at < 0 || at >= len(entries) {
		return "", false
	}
	return entries[at], true
}

// zshHistoryEvents is the list this table views: every entry but the one the
// shell is on, since that is the event `$HISTCMD` names and zsh's table
// leaves it out.
//
// Inside a widget there is no such entry to leave out — the line being typed
// is in the editor and not in this list — so the whole list is what zsh's
// table holds at that moment. See the note above the registration for the two
// measurements this is between.
func zshHistoryEvents(r *interp.Runner) []string {
	entries := r.HistoryEntries()
	if editorRunning(r) || len(entries) == 0 {
		return entries
	}
	return entries[:len(entries)-1]
}

// zshHistoryNewestFirst is the order every reading of the table takes: the
// highest event number first, compared as a **number**.
//
// A key that is not a number sorts after every key that is, so the order is
// total whatever ends up in the table. Nothing puts a non-numeric key there
// today — zshHistoryView writes only `strconv.Itoa` — and an order that
// depended on that would be one more thing to remember about a view somebody
// later widens.
func zshHistoryNewestFirst(keys []string) []string {
	out := append([]string(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		a, aNumber := strconv.Atoi(out[i])
		b, bNumber := strconv.Atoi(out[j])
		switch {
		case aNumber == nil && bNumber == nil:
			return a > b
		case aNumber == nil:
			return true
		case bNumber == nil:
			return false
		default:
			return out[i] < out[j]
		}
	})
	return out
}
