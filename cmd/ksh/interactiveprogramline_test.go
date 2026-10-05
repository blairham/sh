// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// Under `-i`, a fatal error in the `-c` string costs only its line in ksh93,
// as in bash (#6073). See
// interp.Semantics.InteractiveProgramErrorWhenInteractive. Measured
// 2026-10-05 on ksh93u+, `env -i` with a scratch HOME: `after 1`.
func TestAnInteractiveCommandStringErrorCostsTheLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", home+"/.sh_history")
	out, errs, _ := prompt(t, "", "ksh", "-i", "-c", "echo ${u?b}; echo same\necho after $?\n")
	if !strings.Contains(out, "after 1\n") || strings.Contains(out, "same") {
		t.Errorf("stdout %q, want `after 1` and no `same` (stderr %q)", out, errs)
	}
}
