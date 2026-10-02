// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An element store the arithmetic refuses is reported here and the
// expression goes on with the value it was storing. Measured 2026-10-02 on
// bash 5.3.20, `bash -c`. See interp.Semantics.ArithStoreRefusalIsAnError
// (#5145).
func TestAnArithStoreRefusalLetsTheExpressionGoOn(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	src := `a=(1); (( a[-10] = 1 )); echo st=$?; (( a[-10] = 0 )); echo st=$?; echo $(( a[-10] = 5 )); (( b = a[-10] = 3 )); echo b=$b`
	driver.MainArgs(sh, []string{"bash", "-c", src})
	if got, want := out.String(), "st=0\nst=1\n5\nb=3\n"; got != want || strings.Count(errs.String(), "a[-10]: bad array subscript") != 4 {
		t.Errorf("got %q, %q, want %q and four complaints", got, errs.String(), want)
	}
}
