// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `exec 10>f` names descriptor ten here, as it does in bash — the grammar
// flag was left false because a doc comment listed four shells out of seven
// and this one was in neither list (#3238).
//
// Measured 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file in a scratch directory:
//
//	exec 10>f10        status 0
//	echo hi >&10       status 0, and `f10` is three bytes
//	exec 100>f100      status 0 — the width is not two
//	10                 10: not found, status 127
//
// The last row is the control: the digits are a descriptor only in front of a
// redirection operator, which is the same rule bash has.
func TestMultiDigitDescriptorIsARedirection(t *testing.T) {
	dir := t.TempDir()
	out, status, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
	}, "exec 10>f10\necho hi >&10\nexec 10>&-\nexec 100>f100\nexec 100>&-\necho done\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if status != 0 || !strings.Contains(out, "done") {
		t.Fatalf("got %q at %d, want the redirections taken", out, status)
	}
	body, err := os.ReadFile(filepath.Join(dir, "f10"))
	if err != nil {
		t.Fatalf("f10: %v", err)
	}
	if string(body) != "hi\n" {
		t.Errorf("f10 holds %q, want `hi\\n`: the write on ten has to reach "+
			"the file the two-digit number opened", string(body))
	}
	// Three digits as well, so the flag is a width rule and not a two-digit
	// special case.
	if _, err := os.Stat(filepath.Join(dir, "f100")); err != nil {
		t.Errorf("f100: %v — `exec 100>f100` is 0 in the reference", err)
	}
}

// The control, which is the half that says the number is read as a descriptor
// only where a redirection operator follows it. A bare `10` is still a command
// nobody can find, in the reference and here.
func TestABareNumberIsStillACommand(t *testing.T) {
	out, status := run(t, "10\n")
	if status != 127 || !strings.Contains(out, "10") {
		t.Errorf("got %q at %d, want `10: not found` at 127", out, status)
	}
}
