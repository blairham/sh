// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"path/filepath"

	"github.com/blairham/sh/interp"
)

// shellPath is where a relative name a script wrote actually is.
//
// **A shell's working directory is not the process's.** `Runner.Dir` is where
// `cd` moves and where every relative name in the language is resolved, and
// the process's own directory is left alone on purpose — a Runner is embedded
// in other programs, and two of them in one program must not fight over one
// `chdir`. Everything inside `interp` resolves against `Runner.Dir` for that
// reason; a registered builtin reaching for `os.Stat("f")` instead asks the
// *process* where `f` is, and after a single `cd` that is a different file or
// no file at all.
//
// It is not a matter of tidiness. `zstat +mtime f` after `cd /tmp` would stat
// whatever `f` is beside the program that embedded this shell, silently, and
// `zf_rm -f -- $tmp` would unlink it.
//
// The name is returned unchanged when it is already absolute, and when nothing
// has set a directory at all — which is the same rule, and the same fallback,
// the interpreter's own glob and path modifiers use.
func shellPath(r *interp.Runner, name string) string {
	if name == "" || filepath.IsAbs(name) || r.Dir == "" {
		return name
	}
	return filepath.Join(r.Dir, name)
}
