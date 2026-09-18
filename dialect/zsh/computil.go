// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `zsh/computil`: the eight builtins the completion system zsh *ships* is
// written in.
//
// # What this is for
//
// #2776 landed the completion widget's half — `compadd`, `compset`,
// `$compstate`, `$words` — so a completion somebody writes by hand runs. The
// tree of functions zsh ships still did not, because it does not call
// `compadd` directly: `_arguments`, `_describe`, `_tags` and `_values` are
// shell functions whose working parts are C builtins in this module, and
// `_main_complete` → `_complete` → `_normal` → `_dispatch` reached
// `_arguments` and stopped at `comparguments: command not found` (#3039).
//
// # How the protocol here was arrived at, and why no source was read
//
// `CLEANROOM.md` puts zsh's distribution on the red list, and the eight
// builtins are documented in `zshmodules(1)` only in summary — a paragraph
// each, naming what the builtin is *for* and none of its arguments. So the
// protocol was measured rather than read, and the instrument is worth
// describing because it is reusable:
//
// **A function shadows a builtin in zsh.** So an rc file that defines
// `comptags() { print -r -- "$@" >>$log; builtin comptags "$@"; ... }` for
// each of the eight, and binds a key to a `zle -C` widget calling
// `_main_complete`, logs every call the shipped system makes, with its status
// and — through `${(P)name}` — the parameters it set. Driven through a
// pseudo-terminal against `/bin/zsh` 5.9.2 on 2026-09-15, with a two-row
// prompt, over eighteen shipped completions, that is the whole contract:
// which verbs are used, with what arguments, and what each one answers.
//
// Dynamic scoping is what makes the shadow honest: a builtin called from the
// wrapper's frame still assigns to the caller's locals, so the shipped
// function sees exactly what it would have seen.
//
// # What the eighteen traces say the surface is
//
// The verb set the shipped system actually uses is narrow, and this is the
// whole of it:
//
//	comparguments   -i -D -O -M -W -s -a
//	comptags        -i -T -N -R -A
//	comptry         (tags…) and -m (patterns…)
//	compdescribe    -i -I -g
//	compvalues      -i -d -D -s -S -V
//	compfiles       -p -P -i -r
//	compquote       (names…)
//	compgroups      (names…)
//
// Anything outside it is refused rather than guessed at, because a builtin
// that answered a verb it does not implement would be the silent success
// `zmodload` exists to avoid — see zmodload.go.
//
// # What is not here, and why each one is not
//
// The eight all answer, and the shipped system runs on them: `git che<TAB>`
// offers the same eight sub-commands `/bin/zsh` offers on the same rc, and
// `uname -a<TAB>` completes the rest of the stack the same way.
//
// **Two of the three things that used to be smaller than zsh's here were one
// missing seam**, and it is worth writing down which, because both read as
// work inside these files and neither was. `compdescribe` built descriptions
// and handed them back, `compadd -d` took them, and the listing was names
// only; `compgroups`, `compdescribe`'s per-arrangement splitting and
// `compadd`'s `-J` and `-V` all named an arrangement that was thrown away.
// Neither was a builtin that answered wrongly: the *editor* was answered with
// replacement words and had nowhere to put a row or a block. It has both now
// — see repl.Candidate and repl.Group — so those builtins say what they
// measured all along, and #3041 and #3232 are what the widening cost.
//
// **`compfiles` does no globbing optimisation — work, not a boundary.** It is
// the one of the eight that is entirely an optimisation, and the conservative
// answers it gives are correct rather than approximate; the cost is that
// `_files` globs unoptimised. See compfiles.go.

// compArity is the argument-count gate zsh applies to the completion
// builtins *before* the builtin body runs, and it is not an implementation
// detail: it is the first thing a caller outside a completion sees.
//
// zsh's dispatcher checks each builtin's declared minimum and maximum against
// the words it was given and refuses there, so `comparguments` with no
// arguments is `not enough arguments` and never reaches the code that would
// have said `can only be called from completion function`. Every one of the
// ten had that order the other way round here, which meant eight of them
// worded the commonest refusal a script can provoke differently from zsh —
// found by `make coverage`, which reported them as surface no case in the
// tree ever asked about (#2293, under #2291).
//
// Measured on zsh 5.9.2, 2026-09-16, each builtin called outside a completion
// with 0…20 arguments and the refusal classified:
//
//	compadd        0 and up: can only be called
//	comparguments  0: not enough · 1 and up: can only be called
//	compdescribe   0,1,2: not enough · 3 and up: can only be called
//	compfiles      0: not enough · 1 and up: can only be called
//	compgroups     0: not enough · 1 and up: can only be called
//	compquote      0: not enough · 1 and up: can only be called
//	compset        0: not enough · 1,2,3: can only be called · 4 and up: too many
//	comptags       0: not enough · 1 and up: can only be called
//	comptry        0 and up: can only be called
//	compvalues     0: not enough · 1 and up: can only be called
//
// The count is of *words* and not of operands in nine of the ten:
// `compdescribe -i a` is two and refuses, `compdescribe a b c` is three and
// does not, and `compset -p 1 x x` is four and is too many. So an option
// letter is nothing special, which is what a gate ahead of any option parsing
// means. `compquote` is the exception and parses its `-p` off first — see
// compquote.go, where that measurement is. `compadd` and `comptry` declare no
// minimum and are the two with no call to this at all.
//
// max < 0 is "no maximum", which is every one of them but `compset`.
func compArity(r *interp.Runner, args []string, minArgs, maxArgs int) bool {
	if len(args) < minArgs {
		r.Diagnosef("not enough arguments\n")
		return false
	}
	if maxArgs >= 0 && len(args) > maxArgs {
		r.Diagnosef("too many arguments\n")
		return false
	}
	return true
}

// registerComputil puts the eight in the table.
//
// Named together because `zmodload zsh/computil` is answered by asking the
// shell whether it has each of them — the module's gate opens by itself when
// the builtins exist, and it would open on a subset if these were registered
// one file at a time.
func registerComputil(r *interp.Runner) {
	r.Register("comparguments", compargumentsBuiltin)
	r.Register("compdescribe", compdescribeBuiltin)
	r.Register("compfiles", compfilesBuiltin)
	r.Register("compgroups", compgroupsBuiltin)
	r.Register("compquote", compquoteBuiltin)
	r.Register("comptags", comptagsBuiltin)
	r.Register("comptry", comptryBuiltin)
	r.Register("compvalues", compvaluesBuiltin)
}

// computilState is what the eight builtins keep for the length of one
// completion, hung off completionState for the reason the rest of it is: the
// dynamic extent of one call is exactly how long it is good for, and a
// `comparguments` reached anywhere else finds no state and refuses.
type computilState struct {
	arguments *argumentsState
	// tags is one `comptags -i` loop per function nesting level — see
	// comptags.go, where the measurement that says it has to be is.
	tags map[int]*tagsState
	// tagsLatest is the loop `comptry` adds a set to: the one most recently
	// installed, whatever level it went to.
	tagsLatest *tagsState
	describe   *describeState
	values     *valuesState
}

// computilFrom is the state of the completion being performed now, made on
// first use.
//
// The refusal outside a completion is the same one `compadd` gives, measured
// on zsh 5.9.2: `comparguments: can only be called from completion function`
// at status 1.
func computilFrom(r *interp.Runner, ctx context.Context) (*completionState, *computilState, bool) {
	cs, completing := completionFrom(ctx)
	if !completing {
		r.Diagnosef("can only be called from completion function\n")
		return nil, nil, false
	}
	if cs.computil == nil {
		cs.computil = &computilState{}
	}
	return cs, cs.computil, true
}
