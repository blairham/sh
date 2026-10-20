// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestASubstitutionRunsTheTreeReadWithItsLine: the body was read before the
// `alias` beside it ran, so it does not see the alias; `eval` reads its text
// when it runs and does; a later line sees it. Measured 2026-10-03
// on dash 0.5.12, as in the pinned ash image. See interp.Semantics.SubstitutionRunsTheBodyReadWithItsLine.
func TestASubstitutionRunsTheTreeReadWithItsLine(t *testing.T) {
	out, _, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{
		Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}, "alias t=echo; eval \"t E\"; v=$(t S); echo \"v=$v\"\nw=$(t L); echo \"w=$w\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "E\n") || !strings.Contains(out, "t: not found") ||
		!strings.Contains(out, "v=\n") || !strings.HasSuffix(out, "w=L\n") {
		t.Errorf("got %q, want E, t: not found, v= and then w=L", out)
	}
}
