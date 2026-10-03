// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Whether an assignment's trace line waits for its store** (#5546).
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheStore.
func TestATracedStoreFollowsTheAssignment(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2 c=3`, "+ a=1\nksh: r: is read only\n"},
		{`x=0; function x.set { print -u2 SET; }; set -x; x=1`, "SET\n+ x=1\n"},
		{`set -x; a=1 b=${x?boom} true; echo after`, "+ true\n+ a=1\nksh: x: boom\n+ echo after\nafter\n"},
		{`set -x; a=1 b=2 true`, "+ true\n+ a=1\n+ b=2\n"},
		{`readonly r; set -x; a=1 r=2 c=3 true; echo $?`, "+ true\n+ a=1\n+ r=2\n+ c=3\n+ echo 0\n0\n"},
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
