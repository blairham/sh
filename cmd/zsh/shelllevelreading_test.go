// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An inherited `$SHLVL` is read from its front, in the base a prefix names:
// see interp.ShellLevelReading. Measured 2026-10-02 on zsh 5.9.2, `env -i
// PATH=/usr/bin:/bin SHLVL=… zsh -fc 'print $SHLVL'` (#5145).
func TestAnInheritedShellLevelIsReadFromItsFront(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"1+RANDOM", "2"},
		{"3x", "4"},
		{" 2x", "3"},
		{"2.5", "3"},
		{"0x10", "17"},
		{"08", "1"},
		{"abc", "1"},
		{"-2", "-1"},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "SHLVL=" + c.value}
		driver.MainArgs(sh, []string{"zsh", "-fc", "print $SHLVL"})
		if got := strings.TrimSpace(out.String()); got != c.want {
			t.Errorf("SHLVL=%q: got %q, want %q", c.value, got, c.want)
		}
	}
}
