// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `local OPTIND=1` is how a reusable option parser is written in this shell,
// and the scan position it shadows has a second half — how far into a
// clustered word the letters have been read — that is not a parameter. This
// shell hands both halves back when the call returns; bash 3.2 hands back only
// the number, which is the answer this engine had.
//
// See interp.Semantics.GetoptsLocalOptindRestoresTheCursor.

func TestGetoptsLocalOptindRestoresTheCursorAnswer(t *testing.T) {
	if got, want := bash.Semantics().GetoptsLocalOptindRestoresTheCursor, interp.Yes; got != want {
		t.Errorf("GetoptsLocalOptindRestoresTheCursor = %v, want %v", got, want)
	}
}

// TestAGetoptsLoopCallingItselfTerminates runs the reproduction unbounded, and
// the deadline is the assertion: what failed here was termination rather than
// an answer, so a case with a counter in it would pass against the very engine
// that could not finish.
//
// 228 MB of output in 240 s and still going, where the shell this claims to be
// finishes the same script instantly (#2226). The loop reads `-pqr`, calls
// itself on seeing `q`, and the inner call's `local OPTIND` used to leave the
// outer scan pointed at the start of `-pqr` — so it read `p` and `q` again,
// called itself again, and never reached `r`.
func TestAGetoptsLoopCallingItselfTerminates(t *testing.T) {
	const src = `descend() {
	local OPTIND=1
	local flag
	while getopts "pqr" flag; do
		printf 'saw %s\n' "$flag"
		if [ "$flag" = q ]; then descend -p; fi
	done
}
descend -pqr`
	const want = "saw p\nsaw q\nsaw p\nsaw r\n"

	f := preset.Parse(t, src)
	var out strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &out, Stderr: &out, Dir: t.TempDir()})

	// Long enough that a loaded machine cannot fail it and short enough that
	// a regression is reported rather than waited on. The runner honors the
	// context, so this ends the run instead of naming a hang it cannot stop
	// — see interp/cancel.go.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := r.Run(ctx, f); err != nil {
		t.Fatalf("the loop did not finish: %v\nit had written %d bytes", err, out.Len())
	}
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
