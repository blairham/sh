// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `ignorebraces` is acted on rather than remembered (#1856).
//
// This shell spells the option the other way round: it names the state that
// *stops* the expansion where the substrate names the expansion, so the name
// is the substrate's `braceexpand` inverted. It was a recorded name until
// now, which is the worst shape a state report can take — the request
// succeeded, `setopt` listed it, `[[ -o ignorebraces ]]` agreed, and the
// braces went on expanding. Remembering was the bug.
//
// Measured on zsh 5.9.2, 2026-09-11: `setopt ignorebraces; echo {a,b}` writes
// `{a,b}`, `unsetopt ignorebraces` puts the expansion back, `echo {1..3}` is
// the literal while it is on, and `unsetopt braceexpand` — the borrowed
// spelling — is the same request under the other name.
func TestIgnoreBracesReallyStopsTheExpansion(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"the option", "setopt ignorebraces; echo {a,b}\n", "{a,b}\n"},
		{"and taking it back", "setopt ignorebraces; unsetopt ignorebraces; echo {a,b}\n", "a b\n"},
		{"a range too", "setopt ignorebraces; echo {1..3}\n", "{1..3}\n"},
		// The borrowed spelling, which is the same state read the other way
		// round: `unsetopt braceexpand` and `setopt ignorebraces` are one
		// request, so a shell with two stores would disagree here.
		{"the borrowed spelling", "unsetopt braceexpand; echo {a,b}\n", "{a,b}\n"},
		{"and its condition", "setopt ignorebraces; [[ -o braceexpand ]]; echo \"st=$?\"\n", "st=1\n"},
		{"read back by its own name", "unsetopt braceexpand; [[ -o ignorebraces ]]; echo \"st=$?\"\n", "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
