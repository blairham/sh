// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// An empty `ulimit` operand is refused here (#3064), which is a 5.x answer:
// bash 3.2.57 reads it as a limit of nought and lowers the limit to 0, as zsh,
// ksh93 and dash do. The preset takes the newer answer, in the shape
// SymbolicMaskTakesAPermissionCopy already has.
//
// Measured 2026-09-18 from a script file under `LC_ALL=C` against a starting
// limit of 1048576, which is what says the other columns do not merely say
// nothing — they set the limit to zero.
func TestAnEmptyUlimitOperandIsRefused(t *testing.T) {
	out, st := runMaskAndLimits(t, "ulimit -n \"\"\necho \"st=$?\"")
	if st != 0 || !strings.Contains(out, "ulimit: : invalid number") ||
		!strings.Contains(out, "st=1") {
		t.Errorf("out %q status %d, want the refusal at 1", out, st)
	}
}

// runMaskAndLimits runs a snippet on a runner holding a mask and a table of
// limits of its own, since a [interp.Runner] with neither hook refuses every
// `umask` and every `ulimit` before an operand is read. Both are variables
// rather than the process's own, so nothing here touches state the package
// shares.
func runMaskAndLimits(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "bash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf,
	})
	held := 0o022
	r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
	limits := map[interp.Resource][2]int64{}
	r.GetRlimit = func(res interp.Resource) (int64, int64, error) {
		if p, ok := limits[res]; ok {
			return p[0], p[1], nil
		}
		return 1048576, 1048576, nil
	}
	r.SetRlimit = func(res interp.Resource, soft, hard int64) error {
		limits[res] = [2]int64{soft, hard}
		return nil
	}
	status, err := r.Run(context.Background(), preset.Parse(t, src))
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String(), status
}
