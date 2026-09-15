// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// TestAMissingConditionNamesTheTokenItStoppedOn — the same five sites as
// bash's, worded this dialect's way: one line, naming the token the reading
// stopped on, and no sentence about the construct at all.
//
// Measured 2026-09-15 on zsh 5.9.2 over `-c` (#2909). The file and
// standard-input routes are *not* this — that shell reads the closer as an
// ordinary word and blames the newline after it, which is #2964 and a change
// to what the parser consumes rather than to how a refusal is worded.
func TestAMissingConditionNamesTheTokenItStoppedOn(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"[[ ]]", "zsh:1: parse error near `]]'\n"},
		{"[[ ! ]]", "zsh:1: parse error near `]]'\n"},
		{"[[ a && ]]", "zsh:1: parse error near `]]'\n"},
		{"[[ a || ]]", "zsh:1: parse error near `]]'\n"},
		{"[[ ( ]]", "zsh:1: parse error near `]]'\n"},
		{"[[ ( ) ]]", "zsh:1: parse error near `)'\n"},
		{"[[ ; ]]", "zsh:1: parse error near `;'\n"},
		{"[[ ( ( ) ) ]]", "zsh:1: parse error near `)'\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, zsh.Dialect())
			if err == nil {
				t.Fatalf("%s: parsed, want a refusal", tc.src)
			}
			got := zsh.Diagnostics().ParseDiagnostic("zsh", "-c", err, tc.src)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
