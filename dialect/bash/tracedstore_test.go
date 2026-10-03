// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Whether an assignment's trace line waits for its store** (#5546).
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheStore.
func TestATracedStoreFollowsTheAssignment(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2 c=3`, "+ a=1\n+ r=2\nbash: line 1: r: readonly variable\n"},
		{`set -x; a=1 b=${x?boom} true; echo after`, "+ a=1\nbash: line 1: x: boom\n"},
		{`set -x; a=1 b=2 true`, "+ a=1\n+ b=2\n+ true\n"},
		{`readonly r; set -x; a=1 r=2 true; echo $?`, "+ a=1\nbash: line 1: r: readonly variable\n+ true\n+ echo 0\n0\n"},
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
