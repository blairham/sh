// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// zshDynamicDirectoryFunctions is who a `~[name]` asks: `zsh_directory_name`
// first, and then each function `$zsh_directory_name_functions` lists, in
// order. Measured on zsh 5.9.2 with both defined and both answering — the
// first one's answer is the one written. See interp/dynamicdir.go for the
// protocol and the rest of the measurement (#5150).
func zshDynamicDirectoryFunctions(r *interp.Runner) []string {
	var out []string
	if r.HasFunction("zsh_directory_name") {
		out = append(out, "zsh_directory_name")
	}
	if more, ok := r.GetArray("zsh_directory_name_functions"); ok {
		for _, fn := range more {
			if r.HasFunction(fn) {
				out = append(out, fn)
			}
		}
	}
	return out
}
