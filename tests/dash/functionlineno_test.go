// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`$LINENO` in a function counts from the function's own line as 1** —
// Semantics.LinenoCountsTheFunctionsLineAsOne. Measured 2026-10-03 on dash
// 0.5.12 with this layout: `in=2 e=-1 out=4 top=9`. The `eval` reads its own
// text's line 1, less the function's 3, plus one.
func TestLinenoInAFunctionCountsFromItsLine(t *testing.T) {
	src := "\n\nf(){\necho in=$LINENO\neval 'echo e=$LINENO'\necho out=$LINENO\n}\nf\necho top=$LINENO\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "in=2\ne=-1\nout=4\ntop=9\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
