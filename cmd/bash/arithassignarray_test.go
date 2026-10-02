// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An arithmetic assignment to an array name writes its first element here
// and declares nothing: measured 2026-10-02 on bash 5.3.20, `a=(5 6);
// ((a=3)); declare -p a` is `declare -a a=([0]="3" [1]="6")` (#5145).
func TestAnArithAssignmentToAnArrayWritesItsFirstElement(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"bash", "-c", `a=(5 6); ((a=3)); declare -p a`})
	if got, want := out.String(), "declare -a a=([0]=\"3\" [1]=\"6\")\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
