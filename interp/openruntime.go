// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"

	"github.com/blairham/sh/syntax"
)

// What a prompt's open-state field draws while the shell is *running*.
//
// The field is the parser's at a continuation prompt — `for> ` while a loop
// is still being typed — and the same field drawn while a command runs says
// which constructs that command is inside. Measured 2026-10-01 on zsh 5.9.2
// (`-f`, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`) with
// `PS4='[%_]'` and `setopt xtrace`:
//
//	if false; then :; elif true; then true; else false; fi
//	                                [if]false [elif]true [elif-then]true
//	if false; then :; else true; fi [if]false [else]true
//	while true; do if true; then true; fi; break; done
//	                                [while]true [while if]true [while then]true [while]break
//	false || true                   []false [cmdor]true
//	true | true                     []true [pipe]true
//	( true )                        []true — a subshell draws nothing
//	x=$(true)                       []x=[cmdsubst]true, then []x=''
//	true && { true || true; }       []true [cmdand cursh]true
//	repeat 1 true                   [repeat]true
//	f() { if true; then true; fi }; f
//	                                []f [if]true [then]true — a call starts afresh
//	case y in y) if …               [case]case y (y) [case if]true [case then]true
//	for ((i=0;i<1;i++)); do true; done
//	                                []i=0 [for]i<1 [for]true [for]i++
//
// and `print -P '[%_]'` inside a `then` writes `[then]`, so it is the field
// and not the trace that knows. The words are the dialect's — the same
// PromptStyle.OpenWords the continuation prompt draws from, keyed the same
// way — and a key with no word is not drawn.

// openRuntime pushes one construct onto what a command is running inside,
// and answers the pop.
//
// Pushed in place and truncated on the way out, the way substLevelsOut is —
// see substLevelUndo. Every `if`, loop, `&&`, `||` and pipeline element
// opens one, and a fresh slice and a closure for each was two allocations a
// construct (#6423). The stack is only ever pushed and popped in this order,
// so the slots past its length belong to nobody; a copy of the Runner takes
// an array of its own (see ownTables), and a pipeline element or a
// substitution that is itself a construct starts from openRunWith's.
func (r *Runner) openRuntime(key string) openRuntimeUndo {
	undo := openRuntimeUndo{r: r, depth: len(r.openRun)}
	r.openRun = append(r.openRun, key)
	return undo
}

// openRuntimeUndo is openRuntime's pop.
type openRuntimeUndo struct {
	r     *Runner
	depth int
}

func (u openRuntimeUndo) undo() {
	if s := u.r.openRun; u.depth <= len(s) {
		u.r.openRun = s[:u.depth]
	}
}

// replaceOpenRuntime puts a clause in place of the construct it belongs to —
// `then` for `if` — which is how the field draws one. In place, because the
// slot is the construct's own: see openRuntime.
func (r *Runner) replaceOpenRuntime(key string) {
	if n := len(r.openRun); n > 0 {
		r.openRun[n-1] = key
	}
}

// openRuntimeFresh is a function call: the body is drawn as inside nothing,
// whatever the call was made from, and the braces of the body itself are not
// a group of the script's. Answers the restore.
//
// The body's view starts past the caller's slots rather than at nil, so what
// the body pushes goes on the same array instead of a new one per call; the
// caller's slots are below the view, where the body cannot reach them.
func (r *Runner) openRuntimeFresh(body syntax.Command) openRuntimeFreshUndo {
	undo := openRuntimeFreshUndo{r: r, saved: r.openRun, savedBody: r.openRunBody}
	n := len(r.openRun)
	r.openRun, r.openRunBody = r.openRun[n:n], body
	return undo
}

// openRuntimeFreshUndo is openRuntimeFresh's restore.
type openRuntimeFreshUndo struct {
	r         *Runner
	saved     []string
	savedBody syntax.Command
}

func (u openRuntimeFreshUndo) undo() { u.r.openRun, u.r.openRunBody = u.saved, u.savedBody }

// openRuntimeText is the field's text while a command runs.
func (r *Runner) openRuntimeText(words map[string]OpenWord) string {
	var out []string
	for _, key := range r.openRun {
		if w, ok := words[key]; ok && w.Text != "" {
			out = append(out, w.Text)
		}
	}
	return strings.Join(out, " ")
}

// openRunWith is the view a copy of this Runner starts from when the copy
// itself is the construct — a pipeline element after the first, a command
// substitution's body.
func (r *Runner) openRunWith(key string) []string {
	return append(slices.Clip(r.openRun), key)
}
