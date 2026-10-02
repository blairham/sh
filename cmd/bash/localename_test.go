// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A name is ASCII only here whatever the locale: `hähä=3` is a command and
// `$hähä` is `$h` followed by text. Measured 2026-10-02 on bash 5.3.20,
// `env -i PATH=/usr/bin:/bin LC_ALL=en_US.UTF-8 bash -c 'hähä=3; echo
// $hähä'` writes `hähä=3: command not found` and `ähä`. See
// interp.Semantics.NamesTakeTheLocalesLetters (#5153).
func TestANameIsASCIIInEveryLocale(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LC_ALL=en_US.UTF-8"}
	driver.MainArgs(sh, []string{"bash", "-c", "hähä=3; echo $hähä"})
	if got, want := out.String(), "ähä\n"; got != want || !strings.Contains(errs.String(), "hähä=3: command not found") {
		t.Errorf("got %q, %q, want %q and the command refused", got, errs.String(), want)
	}
}
