// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `comptags` and `comptry`: the tag loop `_tags`, `_requested` and
// `_next_label` are written in, and the pair most of the completion tree goes
// through.
//
// # The loop, measured
//
// Traced against zsh 5.9.2 on 2026-09-15 — see computil.go for the
// instrument. `make ` offers two tags and the shipped `_tags` drives them
// like this, with the answers:
//
//	comptags -i :complete:make: argument-rest options    0
//	comptry -m '(|*-)argument-* (|*-)option[-+]* values' 0
//	comptry -m options                                   0
//	comptry argument-rest options                        0
//	comptags -T                                          0
//	comptags -N                                          0
//	comptags -R argument-rest                            0
//	comptags -R options                                  1
//	comptags -N                                          0
//	comptags -R argument-rest                            1
//	comptags -R options                                  0
//	comptags -N                                          1
//
// which says the whole of it:
//
//   - `-i` opens a loop over a context and the tags that could be offered in
//     it. It needs at least one tag: `comptags -i :x:` is `not enough
//     arguments` at status 1.
//   - each `comptry` adds **one set** — the tags it names, or the tags
//     matching the patterns of its `-m`, in the order the `-i` list has them.
//     **A tag is used at most once across every set**, which is what makes
//     the third `comptry` above add nothing and leaves the loop with two
//     sets rather than three. An empty set is not added.
//   - `-T` is whether any set was built. Measured from both sides: straight
//     after `-i` it is 1, and after one `comptry` that matched it is 0.
//   - `-N` steps to the next set, 1 when there is none left.
//   - `-R tag` is whether that tag is in the set `-N` last stepped to.
//   - `-A tag curtag spec` is `-R` plus the label iteration `_next_label`
//     runs: the first call in a set answers 0 with both parameters set to the
//     tag, and the second answers 1 with the spec emptied.
//
// # One state and not a stack
//
// A nested `-i` — `_description` opening `:complete:uname:options` inside the
// loop over `:complete:uname:` — replaces the state rather than stacking on
// it. That is measured rather than chosen: after the nested loop ends there
// is exactly one `-N`, answering 1, and both the nested loop and the outer
// one stop on it. A stack would have needed two.
//
// # What is not here
//
// **The `tag-order` style is not read.** The two `comptry -m` calls the
// shipped `_tags` always makes carry the patterns it wants tried first, and
// those are honored; a `zstyle ':completion:*' tag-order …` a person sets is
// applied by `_tags` itself before it gets here, so nothing in this file has
// to know the style exists.

// tagsState is one `comptags -i` loop: the tags on offer, the sets built out
// of them, and where `-N` has got to.
type tagsState struct {
	context string
	offered []string
	sets    [][]string
	used    map[string]bool
	at      int
	// labeled is which (set, tag) pairs `-A` has already answered for, so
	// that `_next_label`'s loop runs once per tag and then stops.
	labeled map[string]bool
}

func comptagsBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if !compArity(r, args, 1, -1) {
		return 1
	}
	_, st, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	verb, level := tagsVerb(args[0], tagsLevel(r))
	if verb == "-i" {
		if len(args) < 3 {
			r.Diagnosef("not enough arguments\n")
			return 1
		}
		if st.tags == nil {
			st.tags = map[int]*tagsState{}
		}
		t := &tagsState{
			context: args[1], offered: args[2:],
			used: map[string]bool{}, at: -1, labeled: map[string]bool{},
		}
		st.tags[level], st.tagsLatest = t, t
		return 0
	}
	t := st.tags[level]
	if t == nil {
		r.Diagnosef("no tags registered\n")
		return 1
	}
	switch verb {
	case "-T":
		return boolStatus(len(t.sets) > 0)
	case "-N":
		t.at++
		return boolStatus(t.at < len(t.sets))
	case "-R":
		if len(args) < 2 {
			r.Diagnosef("not enough arguments\n")
			return 1
		}
		return boolStatus(t.requested(args[1]))
	case "-A":
		return t.nextLabel(r, args[1:])
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}

// tagsLevel is the function nesting level the call is being made at — zsh's
// own `$#funcstack`, read from where a builtin stands, which is inside the
// function that called it.
func tagsLevel(r *interp.Runner) int { return len(funcstackNames(r)) }

// tagsVerb splits the trailing `-` off a verb: `comptags -i-`, `-T-`, `-N-`
// mean "the level before this one", which is what `_tags --` is for. The
// shipped `_tags` writes the flag straight into the verb — `comptags
// "-i$prev"` — so it arrives as one word.
func tagsVerb(word string, level int) (string, int) {
	if len(word) > 2 && strings.HasSuffix(word, "-") {
		return word[:len(word)-1], level - 1
	}
	return word, level
}

// requested is whether a tag is in the set `-N` last stepped to.
func (t *tagsState) requested(tag string) bool {
	if t.at < 0 || t.at >= len(t.sets) {
		return false
	}
	for _, have := range t.sets[t.at] {
		if have == tag {
			return true
		}
	}
	return false
}

// nextLabel is `-A`: `_next_label`'s one step. With no `tag:label` in play
// there is exactly one label per tag, so the first call in a set answers and
// the second ends the loop.
func (t *tagsState) nextLabel(r *interp.Runner, names []string) int {
	if len(names) < 3 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	tag := names[0]
	key := itoa(t.at) + ":" + tag
	if !t.requested(tag) || t.labeled[key] {
		r.SetVar(names[2], "")
		return 1
	}
	t.labeled[key] = true
	r.SetVar(names[1], tag)
	r.SetVar(names[2], tag)
	return 0
}

// comptry adds one set to the loop: the tags it names that are on offer and
// not spent, or — with `-m` — the ones matching its patterns.
//
// A `comptry` with nothing left to add is still 0. Measured, and it has to
// be: the shipped `_tags` writes three of them in a row and reads none of
// their statuses, so a refusal would be a diagnostic nobody asked for.
func comptryBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	_, st, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	// The loop most recently installed, not the one at this level.
	// `comptry` is written beside the `comptags -i` that opened the loop and
	// acts on it even where that `-i` named the level before this one:
	// measured on zsh 5.9.2, 2026-09-16, a helper doing `comptags -i- ctx p
	// q; comptry p q` leaves `comptags -R p` answering 0 in its *caller*.
	t := st.tagsLatest
	if t == nil {
		r.Diagnosef("no tags registered\n")
		return 1
	}
	var set []string
	if len(args) > 0 && args[0] == "-m" {
		set = t.matching(r, args[1:])
	} else {
		set = t.named(args)
	}
	if len(set) == 0 {
		return 0
	}
	for _, tag := range set {
		t.used[tag] = true
	}
	t.sets = append(t.sets, set)
	return 0
}

// named is the tags of this `comptry` that are on offer and not already used.
func (t *tagsState) named(args []string) []string {
	var set []string
	for _, tag := range t.offered {
		if t.used[tag] {
			continue
		}
		for _, want := range args {
			if want == tag {
				set = append(set, tag)
				break
			}
		}
	}
	return set
}

// matching is the same for `-m`, whose arguments are whitespace-separated
// patterns rather than names. The order is the `-i` list's, so that a set is
// always written in the order the caller offered its tags.
func (t *tagsState) matching(r *interp.Runner, args []string) []string {
	var patterns []string
	for _, arg := range args {
		patterns = append(patterns, splitCompletionWords(arg)...)
	}
	var set []string
	for _, tag := range t.offered {
		if t.used[tag] {
			continue
		}
		for _, pattern := range patterns {
			if r.MatchPattern(pattern, tag) {
				set = append(set, tag)
				break
			}
		}
	}
	return set
}
