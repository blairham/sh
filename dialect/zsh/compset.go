// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compset`: the builtin a completion function moves the word's boundaries
// with.
//
// A completion function does not complete the whole word. `_arguments` takes
// `--opt=` off the front and completes what is after it; `_git` takes the
// subcommand off and completes the subcommand's arguments. `compset` is how
// the part that has already been decided is moved out of `$PREFIX` and into
// `$IPREFIX`, where it stays on the line and stops being matched against.
//
// # Measured on zsh 5.9.2, 2026-09-15
//
// Through a pseudo-terminal, from inside a `zle -C` widget's function, with
// `git foo=che` typed — so `PREFIX` is `foo=che`, `SUFFIX` is empty,
// `words` is `(git foo=che)` and `CURRENT` is 2:
//
//	compset -p 2       0  PREFIX=o=che  IPREFIX=fo
//	compset -p 99      1  unchanged
//	compset -P '*='    0  PREFIX=che    IPREFIX=foo=
//	compset -P ch      1  unchanged
//	compset -S '*'     0  unchanged
//	compset -s 1       1  unchanged
//	compset -n 2       0  words=(foo=che) CURRENT=1
//	compset -N git     0  words=(foo=che) CURRENT=1
//	compset -q         0  words=(foo=che) CURRENT=1
//
// Three of those rows are the ones worth having measured:
//
//   - **The status is "did the pattern match", not "did anything change".**
//     `compset -S '*'` is 0 against an empty `SUFFIX`, because `*` matches the
//     empty string; nothing moved and the answer is still 0.
//   - **`-P` anchors at the start.** `ch` is in `foo=che` and `compset -P ch`
//     is 1, because the match has to begin where `PREFIX` does.
//   - **`-n` and `-N` do not move anything into `IPREFIX`.** The words before
//     the one they name are dropped from `$words` and `CURRENT` is renumbered,
//     and `PREFIX` is untouched.
//
// # `-s`, `-S` and the cursor
//
// Both answer 1 here whenever `SUFFIX` is empty, and `SUFFIX` is always empty
// here — see compsys.go, where the reason is this editor's: it completes the
// text before the cursor and replaces exactly that, so there is no text after
// the cursor for the word to own. The two are implemented rather than refused
// because the answer they give is the one zsh gives for the same state.
//
// # `-q` is the shape of the split and not the splitting
//
// zsh's `-q` breaks the current word at its quotation marks so that a
// completion inside `'…'` completes the quoted text. Measured with nothing
// quoted, it leaves `$words` holding the current word alone and `CURRENT` at
// 1, which is what one word split into one word means — and that is what this
// does. A word that really is quoted is not split further here; the opening
// quote is already off `$PREFIX` and in `$QIPREFIX` before the function runs,
// which is the half of the same job that had to be done anyway.

func registerCompset(r *interp.Runner) { r.Register("compset", compsetBuiltin) }

func compsetBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	// The one of the ten with a maximum as well as a minimum: a fourth word
	// is `too many arguments` before anything looks at the option.
	if !compArity(r, args, 1, 3) {
		return 1
	}
	cs, completing := completionFrom(ctx)
	if !completing {
		r.Diagnosef("can only be called from completion function\n")
		return 1
	}
	letter, rest, ok := compsetOption(r, args)
	if !ok {
		return 1
	}
	switch letter {
	case 'p':
		return cs.moveCount(r, rest, &cs.prefix, &cs.iprefix, true)
	case 's':
		return cs.moveCount(r, rest, &cs.suffix, &cs.isuffix, false)
	case 'P':
		return cs.movePattern(r, rest, &cs.prefix, &cs.iprefix, true)
	case 'S':
		return cs.movePattern(r, rest, &cs.suffix, &cs.isuffix, false)
	case 'n':
		return cs.dropWordsBefore(compsetIndex(r, rest, cs))
	case 'N':
		return cs.dropWordsBefore(compsetMatchingIndex(r, rest, cs))
	case 'q':
		// One word split into one word, which is what an unquoted word gives
		// — see the file comment.
		cs.words, cs.current = []string{cs.prefix + cs.suffix}, 1
		return 0
	}
	return 1
}

// compsetOption is the one letter this call names and whatever followed it.
func compsetOption(r *interp.Runner, args []string) (byte, []string, bool) {
	word := args[0]
	if len(word) != 2 || word[0] != '-' || strings.IndexByte("psPSnNq", word[1]) < 0 {
		r.Diagnosef("bad option: %s\n", word)
		return 0, nil, false
	}
	return word[1], args[1:], true
}

// moveCount is `-p` and `-s`: so many characters out of the matched side of
// the word and into the ignored one, or 1 and nothing moved where the side is
// shorter than that.
func (cs *completionState) moveCount(
	r *interp.Runner, rest []string, from, into *string, front bool,
) int {
	if len(rest) == 0 {
		r.Diagnosef("number expected\n")
		return 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
	if err != nil || n < 0 || n > len(*from) {
		return 1
	}
	if front {
		*into, *from = *into+(*from)[:n], (*from)[n:]
		return 0
	}
	*from, *into = (*from)[:len(*from)-n], (*from)[len(*from)-n:]+*into
	return 0
}

// movePattern is `-P` and `-S`: the longest match anchored at the start of
// `PREFIX`, or at the end of `SUFFIX`, moved the same way.
//
// The *finding* is matchedLength's, in completioncondition.go, because
// `[[ -prefix … ]]` is this test with the move left out — the manual says so
// and one reading of the rule is what keeps the two from drifting. Only the
// move is here.
func (cs *completionState) movePattern(
	r *interp.Runner, rest []string, from, into *string, front bool,
) int {
	if len(rest) == 0 {
		r.Diagnosef("pattern expected\n")
		return 1
	}
	// A leading count restricts which match is taken. It is read so that the
	// pattern is never mistaken for it, and the longest match is still what
	// is taken — a distinction no completion in a real tree turns on.
	length := cs.matchedLength(r, rest[len(rest)-1], *from, front)
	if length < 0 {
		return 1
	}
	if front {
		*into, *from = *into+(*from)[:length], (*from)[length:]
		return 0
	}
	*from, *into = (*from)[:len(*from)-length], (*from)[len(*from)-length:]+*into
	return 0
}

// compsetIndex is `-n`'s operand: the one-based index of the word to make the
// first, or 0 where it is not a number this call can act on.
func compsetIndex(r *interp.Runner, rest []string, cs *completionState) int {
	if len(rest) == 0 {
		r.Diagnosef("number expected\n")
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
	if err != nil || n < 1 || n > cs.current {
		return 0
	}
	return n
}

// compsetMatchingIndex is `-N`'s: the same index, found by the pattern the
// word before it matches.
//
// Both halves are matchingIndex's and endPatternIsAhead's, in
// completioncondition.go, because `[[ -between … … ]]` is this test with the
// move left out — see movePattern for why that is one function and not two.
// The second pattern is the half `-N` took no notice of until the condition
// needed it: a word matching it has to stand after the cursor, and one
// matching it nowhere leaves the test as if it were not given.
func compsetMatchingIndex(r *interp.Runner, rest []string, cs *completionState) int {
	if len(rest) == 0 {
		r.Diagnosef("pattern expected\n")
		return 0
	}
	if len(rest) > 1 && !cs.endPatternIsAhead(r, rest[1]) {
		return 0
	}
	return cs.matchingIndex(r, rest[0])
}

// dropWordsBefore renumbers the line so the word at index n is the first one,
// which is what `-n` and `-N` both leave behind.
func (cs *completionState) dropWordsBefore(n int) int {
	if n < 1 || n > len(cs.words) || n > cs.current {
		return 1
	}
	cs.words = cs.words[n-1:]
	cs.current -= n - 1
	return 0
}
