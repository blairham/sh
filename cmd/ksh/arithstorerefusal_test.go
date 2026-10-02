// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An element store the arithmetic refuses fails the expression here: `let`
// reports it under its own name and leaves 1, and `(( ))` ends the input as
// it does for `1/0`. Measured 2026-10-02 on ksh93u+ 2012-08-01, `ksh -c`.
// See interp.Semantics.ArithStoreRefusalIsAnError (#5145).
func TestAnArithStoreRefusalIsAnError(t *testing.T) {
	var out, errs strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	driver.MainArgs(sh, []string{"ksh", "-c", `a=(1); let "a[-10] = 1"; echo st=$?; (( a[-10] = 1 )); echo no`})
	if got, want := out.String(), "st=1\n"; got != want || !strings.Contains(errs.String(), "let: a: subscript out of range") {
		t.Errorf("got %q, %q, want %q after let's complaint and nothing after the (( ))", got, errs.String(), want)
	}
}
