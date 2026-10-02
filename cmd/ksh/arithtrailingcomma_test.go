// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A sequence may end at its comma here: measured 2026-10-02 on ksh93u+
// 2012-08-01, `echo $(( 3, ))` writes 3. See
// syntax.Dialect.ArithCommaMayEndTheExpression (#5145).
func TestASequenceMayEndAtItsComma(t *testing.T) {
	var out, errs strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	driver.MainArgs(sh, []string{"ksh", "-c", `echo $(( 3, ))`})
	if got := out.String(); got != "3\n" || errs.Len() != 0 {
		t.Errorf("got %q, %q, want 3", got, errs.String())
	}
}
