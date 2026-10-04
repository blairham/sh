// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestATrappedSignalGivesUpAWaitingRead: the handler runs, the read returns 1
// with nothing assigned, and the line written afterwards is not read.
// Measured 2026-10-03 in the pinned image. See
// interp.Semantics.ReadIsAbandonedByATrappedSignal.
func TestATrappedSignalGivesUpAWaitingRead(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Env: []string{"PATH=/usr/bin:/bin"}, Dir: t.TempDir(),
	}, `mkfifo p; exec 3<>p; trap "echo T" INT; (sleep 0.3; kill -INT $$; sleep 0.5; echo late >p) & read -r l <&3; echo "st=$? l=[$l]"; wait`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "T\nst=1 l=[]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
