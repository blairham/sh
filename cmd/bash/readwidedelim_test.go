// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `read -d £` stops at the first byte of the `£` here, under a UTF-8 locale
// too, and the second byte begins the next record. Measured 2026-10-02 on
// bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=en_US.UTF-8`. See
// interp.Semantics.ReadDelimiterIsTheLocalesCharacter (#5153).
func TestReadTakesTheFirstByteOfAWideDelimiter(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LC_ALL=en_US.UTF-8"}
	driver.MainArgs(sh, []string{"bash", "-c", `printf 'first£second£' | { read -d £ one; read -d £ two; echo "$one"; echo "$two"; }`})
	if got, want := out.String(), "first\n\xa3second\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
