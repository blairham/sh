// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`&` returns at once for a pipeline whose last element runs no program**,
// in the dialect that forks that element too (#5530). Measured 2026-10-02 on
// bash 5.3.20: `jobs` right after lists the job Running.
func TestABackgroundedPipelineWithABuiltinLastReturnsAtOnce(t *testing.T) {
	src := `/bin/sleep 3 | true & jobs; kill %1`
	start := time.Now()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[1]+  Running                    /bin/sleep 3 | true &\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %v: `&` waited for the pipeline", took)
	}
}
