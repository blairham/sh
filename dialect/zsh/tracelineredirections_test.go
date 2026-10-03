// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Where a command's trace line stands against its redirections** (#5547).
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheRedirections.
func TestATraceLineFollowsTheRedirections(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; true >/nope/f`, "zsh:1: no such file or directory: /nope/f\n"},
		{`set -x; z=$(echo s >&2) true >/nope/f`, "zsh:1: no such file or directory: /nope/f\n"},
		{`set -x; f() { :; }; f >/nope/f`, "zsh:1: no such file or directory: /nope/f\n"},
		{`set -x; true 2>/dev/null; echo end`, "+zsh:1> true\n+zsh:1> echo end\nend\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
