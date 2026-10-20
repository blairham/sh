// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestUnsetTakesTheExportOfTheShellsOwnParameter pins that `unset` on a
// parameter the shell owns takes its export letter with it, as it does for
// any name. Measured 2026-10-03 on ksh93u+ under `env -i`. The other side of
// Semantics.UnsetKeepsTheShellsOwnAttributes.
func TestUnsetTakesTheExportOfTheShellsOwnParameter(t *testing.T) {
	const src = "export LINENO=5 SECONDS=5; unset LINENO SECONDS; LINENO=7 SECONDS=7\n" +
		"/usr/bin/env | while read -r kv; do case $kv in LINENO=*|SECONDS=*) echo \"$kv\";; esac; done; echo end\n"
	if got, _ := runKsh(t, t.TempDir(), src); got != "end\n" {
		t.Errorf("got %q, want %q", got, "end\n")
	}
}
