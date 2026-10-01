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
// A fresh slice every time, so a copy of the Runner that shares the backing
// array — a pipeline element, a substitution — never has its view written by
// the shell it was copied from.
func (r *Runner) openRuntime(key string) func() {
	saved := r.openRun
	r.openRun = append(slices.Clip(saved), key)
	return func() { r.openRun = saved }
}

// replaceOpenRuntime puts a clause in place of the construct it belongs to —
// `then` for `if` — which is how the field draws one.
func (r *Runner) replaceOpenRuntime(key string) {
	if n := len(r.openRun); n > 0 {
		out := slices.Clone(r.openRun)
		out[n-1] = key
		r.openRun = out
	}
}

// openRuntimeFresh is a function call: the body is drawn as inside nothing,
// whatever the call was made from, and the braces of the body itself are not
// a group of the script's. Answers the restore.
func (r *Runner) openRuntimeFresh(body syntax.Command) func() {
	saved, savedBody := r.openRun, r.openRunBody
	r.openRun, r.openRunBody = nil, body
	return func() { r.openRun, r.openRunBody = saved, savedBody }
}

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
