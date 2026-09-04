// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// ksh93's `read -p` is not bash's prompt: the bare flag names the coprocess
// as the source, and with `|&` outside this grammar there is never one to
// name. Measured (2026-09-04, ksh93 93u+): `read: no query process`, status
// 1, and the variables left exactly as they were — the read failed before
// reaching any input, so nothing is cleared.
func TestReadCoprocessLetterHasNothingToRead(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `v=keep; printf 'x\n' | read -p v; echo "st=$? v=[$v]"`)
	if !strings.Contains(out, "read: no query process") {
		t.Errorf("got %q, want ksh93's own words for the missing coprocess", out)
	}
	if !strings.Contains(out, "st=1 v=[keep]") {
		t.Errorf("got %q, want status 1 and the variable untouched", out)
	}
}
