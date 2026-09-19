// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

func runZshSignals(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestTheNameOptionWillNotTakeANumber is #3544's first row.
//
// `-s` takes a name, and this is the one column that holds to that for a word
// of digits. Measured 2026-09-17 against zsh 5.9.2 under a matching `argv[0]`
// from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	kill -s 9 999999    unknown signal: SIG9 + the hint     1
//	kill -s 009 999999  unknown signal: SIG009 + the hint   1
//	kill -9 999999      a real send, reported as missing    1
//	kill -s 0 $$                                            0
//
// bash 5.3.20, ksh93u+, dash and BusyBox ash 1.37.0 all reach a real send on
// the first row, which is why the core takes the number and this dialect does
// not. Signal 0 is asked before the axis and not by it: the probe is not a
// signal, and every column takes it after `-s`.
//
// The neighboring axis is KillSendsASignalNumberItCannotName, which is this
// position with a number *out of range* — `kill -s 99` is refused in ksh93
// and here, where `-99` is sent. This row is the one in range, where the
// three that send part company with this shell.
func TestTheNameOptionWillNotTakeANumber(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`kill -s 9 999999`, "unknown signal: SIG9"},
		{`kill -s 009 999999`, "unknown signal: SIG009"},
	} {
		out, st := runZshSignals(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.Contains(out, "type kill -L for a list of signals") {
			t.Errorf("%s said %q, want the listing hint after it", c.src, out)
		}
		if st != 1 {
			t.Errorf("%s reported %d, want 1", c.src, st)
		}
	}
	// The probe is not a signal, so it never reaches the question.
	if out, st := runZshSignals(t, `kill -s 0 $$`); st != 0 || out != "" {
		t.Errorf("kill -s 0 $$ said %q at %d, want nothing at 0", out, st)
	}
	// And the flag form is unmoved: a number written as `-9` is a number.
	if out, _ := runZshSignals(t, `kill -9 999999`); strings.Contains(out, "unknown signal") {
		t.Errorf("kill -9 said %q, want a send rather than a name refused", out)
	}
}

// TestTheOlderSignalNameIsRead is #3536's axis, read from the other column
// that has it.
//
// Measured 2026-09-17 against zsh 5.9.2: `kill -l IOT` is `6`, `kill -s IOT
// 999999` reaches a real send, and `trap 'x' IOT` is 0 — while `kill -l 6` is
// `ABRT`, so the older spelling is read and never written back. ksh93 reads
// it too and additionally *lists* signal 6 as IOT, which is this shell's
// difference from that one: `trap 'x' ABRT; trap` writes ABRT here and IOT
// there.
//
// bash 5.3.20 refuses the word on every route, which is what makes it an axis
// rather than a name the table always carried.
func TestTheOlderSignalNameIsRead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`kill -l IOT`, "6"},
		{`kill -l 6`, "ABRT"},
	} {
		out, st := runZshSignals(t, c.src)
		if st != 0 || strings.TrimSpace(out) != c.want {
			t.Errorf("%s said %q at %d, want %q at 0", c.src, out, st, c.want)
		}
	}
	if out, st := runZshSignals(t, `trap 'echo x' IOT; echo "st=$?"`); st != 0 ||
		!strings.Contains(out, "st=0") {
		t.Errorf("trap 'echo x' IOT said %q at %d, want st=0", out, st)
	}
	// The bare listing is unmoved: this shell reads the word and writes its
	// own.
	out, _ := runZshSignals(t, `kill -l`)
	if !strings.Contains(out, "ABRT") || strings.Contains(out, "IOT") {
		t.Errorf("kill -l wrote %q, want ABRT in it and no IOT", out)
	}
}
