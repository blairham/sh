// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A name may hold the locale's letters and digits past ASCII here, as in
// zsh. Measured 2026-10-02 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 ksh -c 'hähä=3; ä1=4; echo $hähä $ä1 ${hähä}; typeset
// ñ=2; echo $ñ'` writes `3 4 3` and `2`. See
// interp.Semantics.NamesTakeTheLocalesLetters (#5153).
func TestANameHoldsTheLocalesLetters(t *testing.T) {
	var out, errs strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LC_ALL=en_US.UTF-8"}
	driver.MainArgs(sh, []string{"ksh", "-c", "hähä=3; ä1=4; echo $hähä $ä1 ${hähä}; typeset ñ=2; echo $ñ"})
	if got, want := out.String(), "3 4 3\n2\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
