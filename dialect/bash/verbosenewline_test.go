// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// The `-v` echo ends a line the input did not end here (#3130), which is this
// column alone: ksh93u+, dash and zsh all leave the echo of a `-c` string's
// last line and the first byte the script writes on the same line.
//
// Measured 2026-09-18 through `od -c` on bash 5.3.20.
func TestVerboseEchoEndsTheInputsLastLine(t *testing.T) {
	// One stream for both, because what is asserted is where the echo falls
	// against the output it runs into.
	var both bytes.Buffer
	sh := bashShell(&both, &both)
	if code := driver.MainArgs(sh, []string{"bash", "-v", "-c", "echo one\necho two"}); code != 0 {
		t.Fatalf("status %d (%q)", code, both.String())
	}
	const want = "echo one\none\necho two\ntwo\n"
	if got := both.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
