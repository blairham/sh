// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAnEmptyArgv0NamesNothing: started under an empty argv[0], the shell's
// diagnostics name nothing in front of their location, as the reference's
// do — where an argv[0] nobody gave still names the shell. Measured
// 2026-10-02 under `exec -a ""` (#5373).
func TestAnEmptyArgv0NamesNothing(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"", "-c", `nosuchcmd; echo "[$0]"`})
	if out.String() != "[]\n" {
		t.Errorf("stdout %q, want an empty $0", out.String())
	}
	if want := ": line 1: nosuchcmd: command not found\n"; errs.String() != want {
		t.Errorf("stderr %q, want %q", errs.String(), want)
	}
	errs.Reset()
	driver.MainArgs(sh, []string{"bash", "-c", "nosuchcmd"})
	if strings.HasPrefix(errs.String(), ":") {
		t.Errorf("stderr %q: a named shell names itself", errs.String())
	}
}
