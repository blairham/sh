// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// TestADashWordOfDigitsIsOptionLetters is the column #3139 did not touch, and
// the one whose answer the other three had been given.
//
// There is no signal number out of range here, because `-99` is not a number
// at all: a dash-word is option letters, so it is the letter `9` with junk
// behind it and the complaint names `-9`. At 2, which is this shell's status
// for a bad option and nobody else's. Measured 2026-09-16 against dash 0.5.12
// on macOS arm64:
//
//	kill -99 999999    kill: Illegal option -9    status 2
//	kill -65 999999    kill: Illegal option -6    status 2   (Linux)
//	kill -s 99 999999  kill: invalid signal number or name: 99    status 2
//
// The second row is what proves it is the first character and not the value:
// a shell reading the whole word would name `-65`. It is written as 65 rather
// than as 32 because **32 is a signal on Linux and is not one on macOS**, and
// dash sends the ones its kernel has: measured 2026-09-17 in the panel's
// Alpine image, `kill -32 999999` and `kill -34 999999` are both `kill: No
// such process` at 1 there, where the same words on macOS are `Illegal option
// -3`. The row below has both answers rather than one, which is what #3168
// and #3287 are about — the range is the kernel's and not the table's.
//
// A pid that cannot be there rather than `$$`, because on a kernel with the
// signal the send is real and the shell under test is this process.
func TestADashWordOfDigitsIsOptionLetters(t *testing.T) {
	if got := dash.Semantics().KillSendsASignalNumberItCannotName; got != interp.No {
		t.Errorf("KillSendsASignalNumberItCannotName is %v, want No", got)
	}
	// The first number past this kernel's last signal, which is the same
	// question `-99` asks on a machine whose signals stop at 31.
	past, pastLetter := `kill -65 999999`, "Illegal option -6"
	if runtime.GOOS == "darwin" {
		past, pastLetter = `kill -32 999999`, "Illegal option -3"
	}
	for _, c := range []struct{ src, want string }{
		{`kill -99 999999`, "Illegal option -9"},
		{past, pastLetter},
		{`kill -s 99 999999`, "invalid signal number or name: 99"},
	} {
		out, _ := answersRun(t, c.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s said %q, want status 2", c.src, out)
		}
	}
}
