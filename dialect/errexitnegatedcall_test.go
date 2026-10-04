// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set -e` judges a negated call in zsh** —
// Semantics.ErrexitJudgesANegatedCall. Measured 2026-10-03 on zsh 5.9.2 and
// bash 5.3.20: `f(){ false; echo inner; }; ! f` writes `inner` in both, and
// zsh then ends where bash goes on; `! { false; }` and an `if ! f` go on in
// both, so it is the call and not the negation.
func TestErrexitJudgesANegatedCallOnlyInZsh(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"zsh", "set -e; f() { false; echo inner; }; if ! f; then :; fi; ! { false; }; ! f; echo reached\n", "inner\ninner\n"},
		{"bash", "set -e; f() { false; echo inner; }; if ! f; then :; fi; ! { false; }; ! f; echo reached\n", "inner\ninner\nreached\n"},
		{"zsh", "set -e; f() { false; }; ! f; echo reached\n", "reached\n"},
	} {
		out, _, err := presets[c.name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s %q: got %q, want %q", c.name, c.src, out, c.want)
		}
	}
}
