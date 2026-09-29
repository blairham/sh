// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// JobSpecCommandWordForm is what a command word beginning with `%` is.
//
// In three of the panel's columns it is not a command name at all: the shell
// reads it as a job specification and runs `fg` on it — or `bg`, if the
// command was written with a trailing `&`. Measured 2026-09-29, script files
// with no job control, `%prep` on a line of its own:
//
//	zsh 5.9.2     zsh:fg:1: no job control in this shell.   1
//	bash 5.3.20   bash: line 1: fg: no job control          1
//	bash 3.2.57   the same                                  1
//	ksh93u+       ksh: line 1: %prep: not found             127
//	dash          dash: 1: %prep: not found                 127
//	ash 1.36.1    ash: line 1: %prep: not found             127
//
// A form rather than a flag, because **the two columns that have it disagree
// about which word they are reading**, and they part on quoting and on
// expansion. Same day, same conditions:
//
//	                          zsh          bash
//	%prep                     fg           fg
//	"%prep"                   not found    fg
//	\%prep                    not found    fg
//	x=%prep; $x               not found    fg
//	command %prep             not found    fg
//
// zsh asks about the word **as the script wrote it** — an unquoted, unescaped
// `%` as the first character of the first word — and bash asks about the name
// the lookup is about to be made with, whatever produced it.
//
// **Nothing in the `%prep` row can tell those two readings apart**, and that
// is the whole reason this is a form. Written plainly, a `%` command word is
// the same word before and after expansion, so every grid built out of
// `%prep`, `%1`, `%%` and the rest agrees in both columns however wide it
// gets — thirteen such rows agree here — and a flag set from any of them
// would have been keyed on the wrong noun and right by luck. The pairs above
// hold the reading fixed and change what produced the word, which is the only
// shape that decides it.
//
// Two rows of bash's reading are measured and not answered here, named rather
// than smoothed: `command %prep` is a job spec there and an ordinary name
// here, because the reading reaches inside a `command` that has already
// stripped itself; and `%prep | cat` is `command not found` there and a job
// spec here, because bash's reading does not reach a pipeline element where
// zsh's does. Both on #4436.
//
// It is not a grammar question: `%prep` parses as an ordinary word in every
// column, and no column refuses the text. What differs is what the shell does
// with the word it has, which is this vector's business.
type JobSpecCommandWordForm int

const (
	// JobSpecCommandWordUnspecified is no answer, and is refused like any
	// other — but only where the question arises, which is a command word
	// starting with `%` and nothing else. See Runner.jobSpecCommandWord.
	JobSpecCommandWordUnspecified JobSpecCommandWordForm = iota
	// JobSpecCommandWordIsNotOne keeps `%prep` an ordinary command name, to
	// be looked up and not found. ksh93, dash and ash.
	JobSpecCommandWordIsNotOne
	// JobSpecCommandWordAsWritten reads the word the script wrote: the first
	// word's first character is an unquoted, unescaped `%`. What follows it
	// may be anything, including expansions — `x=rep; %p$x` is a job spec in
	// zsh — because what is being asked about is how the command word
	// *starts*, not what it comes to. zsh.
	//
	// Pathname expansion still runs first and its failure wins: `%p*` in an
	// empty directory is `no matches found: %p*` there rather than a `fg`,
	// so this decides the dispatch and not whether the word expands. With
	// `nonomatch` set, the same word reaches the resumer.
	//
	// One row of that is not answered here and is on #4436: `%?abc` is a
	// `fg` in the reference where `%?ab*` and `%*` are `no matches found`,
	// so a `?` **immediately after the `%`** is read as the `%?string` job
	// spec rather than as a pattern, and a glob character anywhere else
	// takes the word back to the matcher. We glob it and report no matches.
	JobSpecCommandWordAsWritten
	// JobSpecCommandWordAfterExpansion reads the name the command lookup is
	// about to use, however the script spelled it: a quoted `"%prep"`, an
	// escaped `\%prep`, a parameter holding it, and one behind `command` are
	// all job specs. bash 5.3 and bash 3.2.
	JobSpecCommandWordAfterExpansion
)

func (f JobSpecCommandWordForm) String() string {
	switch f {
	case JobSpecCommandWordIsNotOne:
		return "JobSpecCommandWordIsNotOne"
	case JobSpecCommandWordAsWritten:
		return "JobSpecCommandWordAsWritten"
	case JobSpecCommandWordAfterExpansion:
		return "JobSpecCommandWordAfterExpansion"
	}
	return "JobSpecCommandWordUnspecified"
}

// writtenJobSpecWord reports whether the command word, **as written**, begins
// with an unquoted `%`.
//
// The span's Value carries the source text with escapes unresolved, so a
// `\%prep` keeps its backslash and fails this test without anything having to
// know about escaping — which is what the reference does with it. A quoted
// run is excluded by its Quoting, and a word that begins with a substitution
// is excluded by its Kind.
func writtenJobSpecWord(c *syntax.SimpleCmd) bool {
	if c == nil || len(c.Args) == 0 {
		return false
	}
	w := c.Args[0]
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	return s.Kind == syntax.Literal && s.Quoting == syntax.Unquoted &&
		strings.HasPrefix(s.Value, "%")
}

// jobSpecCommandWord reports whether this command is a job specification
// rather than a command to look up, and is asked **only** when something
// about the command already starts with `%`.
//
// That gating is the point rather than an optimization. An unanswered axis
// refuses by name wherever it is consulted, and a command word is the
// commonest thing a shell has; asking this of every command would leave a
// bare Semantics unable to run anything at all. Asked here, a Semantics with
// no answer runs every script that never writes a `%` command word, and
// refuses exactly the one that does — which is the measurement the axis is
// about.
// **The caller must not read r.unspecified to decide whether this refused.**
// That field carries whatever the command has already refused over — a frozen
// prefix, a bad subscript — so a site that reads it here abandons the command
// for somebody else's refusal. The first draft did exactly that, and a bare
// Semantics refusing the frozen-prefix axis went from "say so and run the
// command anyway" to "say so and run nothing".
//
// Nor does this need to tell its caller that it refused. A refusal sets the
// shared flag, and the command is already abandoned downstream on it: an
// early return here made no difference to any output, measured, and a mutant
// that removed it was equivalent. So there is nothing to return but the
// answer.
func (r *Runner) jobSpecCommandWord(c *syntax.SimpleCmd, argv []string) bool {
	written := writtenJobSpecWord(c)
	expanded := len(argv) > 0 && strings.HasPrefix(argv[0], "%")
	if !written && !expanded {
		return false
	}
	switch r.sem().JobSpecCommandWord {
	case JobSpecCommandWordAsWritten:
		return written
	case JobSpecCommandWordAfterExpansion:
		return expanded
	case JobSpecCommandWordIsNotOne:
		return false
	}
	r.diagf("%s\n", r.unanswered("a command word beginning with % being a job specification"))
	r.status = 2
	r.unspecified = true
	return false
}

// runJobSpecWord hands the command's words to the resumer the reading names.
//
// The whole of argv and not just its first word, because the reference does:
// `%prep arg` and `fg %prep arg` write the same sentence in both columns that
// have the reading, so the extra words reach the builtin and are refused or
// ignored there rather than dropped here.
func (r *Runner) runJobSpecWord(ctx context.Context, argv []string, verb string) int {
	fn, ok := r.lookupBuiltin(verb)
	if !ok {
		// A dialect that answers the axis and has no resumer is a preset
		// mistake rather than a script's, but the shell still has to say
		// something rather than run a command named `%…` after deciding it
		// was not one.
		return 127
	}
	// The resumer is in the location, which is how the reference words it:
	// `zsh:fg:1: no job control in this shell.` where an ordinary refusal on
	// that line is `zsh:1:`. Set here because nothing wrote the command word
	// on the way in — this dispatch never found a builtin named `%…`, it
	// decided the word was not a command name at all.
	outer := r.inBuiltin
	r.inBuiltin = verb
	defer func() { r.inBuiltin = outer }()
	return r.callBuiltin(ctx, verb, fn, argv)
}
