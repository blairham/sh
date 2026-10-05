// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// At a prompt ksh93 numbers each typed input from its own first line, and
// locates `eval`'s text by its own line as `ksh -c` does (#6074). See
// interp.Semantics.PromptNumbersEachInputFromOne and
// interp.Diagnostics.EvalTextAtAPromptKeepsItsLocation.
//
// Measured 2026-10-05 on ksh93u+ (/bin/ksh -i) through a pseudo-terminal and
// on a pipe alike, f holding `(exit 3)` and `echo ${unset?boom}`; every
// expected line is that shell's. This shell counted the session's lines —
// `L=2`, `ksh[6]: h[5]: …`, `ksh[7]: .: …` — and wrote `eval:` with no line.
func TestAKshPromptNumbersEachInputFromOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	// An interactive ksh writes its history, so the file is the test's own.
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Chdir(home)
	if err := os.WriteFile(filepath.Join(home, "f"), []byte("(exit 3)\necho ${unset?boom}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	typed := "echo L=$LINENO\necho L=$LINENO\n" +
		"function h {\ntrue\n. ./f; }\nh\n" +
		"g(){ . ./f; }; g\n" +
		"eval \"true\necho \\${unset?boom}\"\n" +
		"function e2 { eval \"echo \\${unset?boom}\"; }; e2\n" +
		"eval \"echo )\"\n"
	out, errs, _ := prompt(t, typed, "ksh", "-i")
	if out != "L=1\nL=1\n" {
		t.Errorf("stdout %q, want L=1 twice", out)
	}
	for _, want := range []string{
		"ksh: h[3]: .: line 2: unset: boom\n",
		"$ ksh: .: line 2: unset: boom\n",
		"ksh: eval: line 2: unset: boom\n",
		"ksh: e2[1]: eval: line 1: unset: boom\n",
		// The parse half stays the prompt's.
		"ksh: eval: syntax error: `)' unexpected\n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr %q, want %q in it", errs, want)
		}
	}
}
