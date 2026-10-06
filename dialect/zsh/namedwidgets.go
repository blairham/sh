// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// NamedWidgets is every widget a name reaches, for execute-named-cmd: the
// editor's own under both their spellings — `up-case-word` and
// `.up-case-word` — and every one `zle -N` or `zle -C` defined, which wins
// over an editor's name it redefines, as it does on a key (#6241).
//
// The dialect's answer to driver.Shell.NamedWidgets.
func NamedWidgets(r *interp.Runner) map[string]repl.Binding {
	out := map[string]repl.Binding{}
	for name, w := range bindkeyWidgets {
		if name == undefinedKey {
			continue
		}
		out[name] = repl.Binding{Widget: w}
		out["."+name] = repl.Binding{Widget: w}
	}
	for name, def := range readWidgets(r) {
		if def.completer != "" {
			out[name] = completionBinding(name, def.completer)
			continue
		}
		out[name] = repl.Binding{Function: name}
	}
	return out
}
