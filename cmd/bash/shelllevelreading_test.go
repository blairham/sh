// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An inherited `$SHLVL` is read whole or not at all here: measured
// 2026-10-02 on bash 5.3.20, `SHLVL=3x` and `SHLVL=1+RANDOM` are counted from
// nothing and `SHLVL=08` from 8. See interp.ShellLevelReading (#5145).
func TestAnInheritedShellLevelIsReadWhole(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"1+RANDOM", "1"}, {"3x", "1"}, {"08", "9"}, {" 4", "5"},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "SHLVL=" + c.value}
		driver.MainArgs(sh, []string{"bash", "-c", "echo $SHLVL"})
		if got := strings.TrimSpace(out.String()); got != c.want {
			t.Errorf("SHLVL=%q: got %q, want %q", c.value, got, c.want)
		}
	}
}
