// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `)` left over after a complete expression is `unbalanced parenthesis`
// here: measured 2026-10-02 on ksh93u+ 2012-08-01, `foo="3)"; echo $((foo))`
// is `3): unbalanced parenthesis`. See syntax.ErrArithUnmatchedCloseParen
// (#5145).
func TestAStrayCloseParenIsUnbalanced(t *testing.T) {
	var out, errs strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	driver.MainArgs(sh, []string{"ksh", "-c", `foo="3)"; echo $((foo))`})
	if !strings.HasSuffix(errs.String(), ": 3): unbalanced parenthesis\n") || out.Len() != 0 {
		t.Errorf("got %q, %q", out.String(), errs.String())
	}
}
