// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `read -t` takes an expression here, as `ulimit`'s operand does (#3209), and
// a duplication's descriptor number stops at 64 (#3210). Both measured
// 2026-09-18 against ksh93u+ 2012-08-01 from a script file under `env -i`.
func TestReadTimeoutIsAnExpressionAndDescriptorsStopAtSixtyFour(t *testing.T) {
	// An unset name is nought, so the read times out at once and says
	// nothing — where every bash writes `invalid timeout specification`.
	out, st := runKsh(t, t.TempDir(), "read -t abc x </dev/null\necho \"t=$?\"")
	if st != 0 || out != "t=1\n" {
		t.Errorf("out %q status %d, want a silent 1", out, st)
	}
	// And a word the expression grammar cannot read is still refused.
	out, st = runKsh(t, t.TempDir(), "read -t 3abc x </dev/null\necho \"t=$?\"")
	if st != 0 || !strings.Contains(out, "3abc") || !strings.Contains(out, "t=1") {
		t.Errorf("out %q status %d, want the word named and 1", out, st)
	}
	for _, tc := range []struct{ src, want string }{
		// Below the ceiling the ordinary sentence, with the errno in
		// brackets this shell puts there.
		{"echo x >&63", "63: cannot open [Bad file descriptor]"},
		{"exec 6>&7", "7: cannot open [Bad file descriptor]"},
		// At it and past it, a sentence about the number with no errno at
		// all — the one refusal about a descriptor this shell writes
		// without one.
		{"echo x >&64", "64: bad file unit number"},
		{"echo x >&99", "99: bad file unit number"},
		{"cat <&99", "99: bad file unit number"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q, want %q in it", tc.src, out, tc.want)
		}
		if strings.Contains(tc.want, "bad file unit number") && strings.Contains(out, "bad file unit number [") {
			t.Errorf("%s: out %q, want no errno in brackets after it", tc.src, out)
		}
	}
}

// The `-v` echo writes a line as it was read, so the last line of a `-c`
// string with no newline is echoed without one and the first byte the script
// writes lands beside it (#3130). Measured 2026-09-18 through `od -c`.
func TestVerboseEchoKeepsTheInputsLastLineOpen(t *testing.T) {
	// One stream for both, because what is asserted is where the echo falls
	// against the output it runs into.
	var both bytes.Buffer
	sh := kshShell(&both, &both)
	if code := driver.MainArgs(sh, []string{"ksh", "-v", "-c", "echo one\necho two"}); code != 0 {
		t.Fatalf("status %d (%q)", code, both.String())
	}
	const want = "echo one\none\necho twotwo\n"
	if got := both.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
