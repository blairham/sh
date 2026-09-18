// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A permission copy beside permission letters is OR-ed in here like any other
// permission, where bash throws the letters away (#3074), and an empty
// `ulimit` operand is a limit of nought rather than a refusal (#3064).
//
// Measured 2026-09-18 from a script file under `LC_ALL=C`.
func TestUmaskCopyContributesAndAnEmptyLimitIsNought(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Setting with `-S` prints nothing here, so each row asks for the
		// mask back afterwards. From `umask 222` the owner is allowed
		// `r-x`, so a copy adds those and the `w` beside it stays — `g=rwx`
		// either way round, where bash answers `g=rx` for the spelling with
		// the copy last.
		{"umask 222\numask -S g=uw\numask -S", "u=rx,g=rwx,o=rx\n"},
		{"umask 222\numask -S g=wu\numask -S", "u=rx,g=rwx,o=rx\n"},
		// The portable spelling, unchanged.
		{"umask 222\numask -S g=u\numask -S", "u=rx,g=rx,o=rx\n"},
	} {
		out, st := runMaskAndLimits(t, tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
	out, st := runMaskAndLimits(t, "ulimit -n \"\"\necho \"st=$?\"\nulimit -n")
	if st != 0 || !strings.Contains(out, "st=0") || !strings.HasSuffix(out, "0\n") {
		t.Errorf("out %q status %d, want a silent 0 and a limit of 0 afterwards", out, st)
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
		Name: "dash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
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
