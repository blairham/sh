// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// TestThisPresetHasAThirdWordingForASignalSpec is half of #3167.
//
// This shell reads a dash-word three ways and had two here. Measured
// 2026-09-17 on zsh 5.9.2, macOS arm64, under a matching `argv[0]`:
//
//	kill -9 <gone>     kill <pid> failed: no such process    1   — all digits, sent
//	kill -9x <pid>     invalid signal number: -9x            1   — starts with a digit
//	kill -0x <pid>     invalid signal number: -0x            1
//	kill -1x <pid>     invalid signal number: -1x            1
//	kill -n 9x <pid>   invalid signal number: 9x             1
//	kill -x9 <pid>     unknown signal: SIGX9 + the hint      1   — starts with a letter
//	kill -NOPE <pid>   unknown signal: SIGNOPE + the hint    1
//	kill -s 9x <pid>   unknown signal: SIG9X + the hint      1   — `-s` takes a name
//
// Ours had only the third, so a word that was never a name came back with a
// SIG- prefixed spelling of itself. Three things in the middle rows are
// measured rather than assumed: the **dash** is printed for the flag form and
// not after `-n`, so it belongs to the form; the listing **hint** does not
// follow this wording where it follows the other two; and the discriminator
// is the *shape* of the word rather than the failure, which `-x9` is what
// says.
func TestThisPresetHasAThirdWordingForASignalSpec(t *testing.T) {
	if got := zsh.Diagnostics().KillInvalidSignalNumber; got == "" {
		t.Error("KillInvalidSignalNumber is empty, want this shell's third wording")
	}
	for _, c := range []struct{ src, want string }{
		{`kill -9x 999999`, "invalid signal number: -9x"},
		{`kill -0x 999999`, "invalid signal number: -0x"},
		{`kill -n 9x 999999`, "invalid signal number: 9x"},
		{`kill -x9 999999`, "unknown signal: SIGX9"},
		{`kill -NOPE 999999`, "unknown signal: SIGNOPE"},
		{`kill -s 9x 999999`, "unknown signal: SIG9X"},
	} {
		out, _ := answersRun(t, c.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", c.src, out)
		}
		// The hint follows an unknown *signal* and not an invalid *number*.
		hinted := strings.Contains(out, "kill -L")
		if want := !strings.Contains(c.want, "invalid signal number"); hinted != want {
			t.Errorf("%s said %q: hint %v, want %v", c.src, out, hinted, want)
		}
	}
	// And `-n` is an option here, which is what puts a word after it in the
	// number position at all.
	if got := zsh.Semantics().KillReadsTheNumberOption; got != interp.Yes {
		t.Errorf("KillReadsTheNumberOption is %v, want Yes", got)
	}
}
