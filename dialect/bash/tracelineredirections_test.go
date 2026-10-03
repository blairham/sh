// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Where a command's trace line stands against its redirections** (#5547).
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheRedirections.
func TestATraceLineFollowsTheRedirections(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; true >/nope/f`, "+ true\nbash: line 1: /nope/f: No such file or directory\n"},
		{`set -x; z=$(echo s >&2) true >/nope/f`, "++ echo s\ns\n+ z=\n+ true\nbash: line 1: /nope/f: No such file or directory\n"},
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
