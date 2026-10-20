// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Whether an assignment's trace line waits for its store** (#5546).
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheStore.
func TestATracedStoreFollowsTheAssignment(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2 c=3`, "+zsh:1> a=1 r=2 zsh:1: read-only variable: r\n\n"},
		{`set -x; a=1 b=2 true`, "+zsh:1> a=1 b=2 +zsh:1> true\n"},
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
