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
// # What is not here
//
// **Descriptions are not shown.** `compdescribe` builds them and hands them
// back, `compadd -d` takes them, and this editor's listing is names only —
// repl's completion seam is answered with replacement words and has nowhere
// to put a description. That is #3041 and it is the visible difference
// between a listing here and zsh's.
//
// **Match groups are not a thing here.** `compgroups`, and `compadd`'s `-J`
// and `-V`, name an ordering and a listing arrangement this editor has not
// got; they are read and ignored rather than refused, which is compctl.go's
// rule and for its reason.

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
	tags      *tagsState
	describe  *describeState
	values    *valuesState
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
