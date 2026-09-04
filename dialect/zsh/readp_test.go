// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// zsh's `read -p` is not bash's prompt: the bare flag names the coprocess as
// the source, and with zsh's `coproc` outside this grammar there is never
// one to name. Measured (2026-09-04, zsh 5.9): `-p: no coprocess` under the
// builtin-naming location, status 1, and the variables left exactly as they
// were — the read failed before reaching any input, so nothing is cleared.
func TestReadCoprocessLetterHasNothingToRead(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `v=keep; printf 'x\n' | read -p v; echo "st=$? v=[$v]"`)
	if !strings.Contains(out, "-p: no coprocess") {
		t.Errorf("got %q, want zsh's own words for the missing coprocess", out)
	}
	if !strings.Contains(out, "st=1 v=[keep]") {
		t.Errorf("got %q, want status 1 and the variable untouched", out)
	}
}

// A -d or -t whose argument never arrives is zsh's own sentence with the
// letter after it, and status 1 — the same number as any other option
// complaint here. Measured 2026-09-04.
func TestOptionArgumentExpectedIsZshsSentence(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `read -d </dev/null; echo "st=$?"`)
	if !strings.Contains(out, "argument expected: -d") {
		t.Errorf("got %q, want zsh's missing-argument sentence", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want status 1", out)
	}
}
