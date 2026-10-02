// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `%q` with no operand left quotes the empty string here: measured
// 2026-10-02 on bash 5.3.20, `printf '[%q]\n'` writes the brackets with a
// pair of single quotes between them. See
// interp.Semantics.PrintfQuoteOfNoArgumentIsEmpty (#5153).
func TestPrintfQuoteOfNoArgumentQuotesTheEmptyString(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"bash", "-c", `printf '[%q]\n'`})
	if got, want := out.String(), "['']\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
