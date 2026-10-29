// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestATrappedSignalGivesUpAWaitingReadAtItsNumber: the handler runs, every
// name is emptied, the line written afterwards is not read by it — it is the
// next read's, whole — and the status is 256 plus the signal. Measured
// 2026-10-03 on ksh93u+, and the next read 2026-10-29 (#7024). See
// interp.Semantics.ReadAbandonedReportsTheSignal.
func TestATrappedSignalGivesUpAWaitingReadAtItsNumber(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{
		Env: []string{"PATH=/usr/bin:/bin"}, Dir: t.TempDir(),
	}, `mkfifo p; exec 3<>p; trap "echo T" INT; l=old; (sleep 0.3; kill -INT $$; sleep 0.5; echo late >p) & read -r l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "T\nst=258 l=[]\nnext [late]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
