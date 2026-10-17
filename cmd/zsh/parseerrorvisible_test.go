// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// zshErrsInLocale runs args through the front end in a scratch directory with
// the environment naming a locale, and answers what it wrote on standard
// error.
func zshErrsInLocale(t *testing.T, locale string, files map[string]string, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "LC_ALL=" + locale}
	driver.MainArgs(sh, append([]string{"zsh"}, args...))
	return errs.String()
}

// The text a parse failure quotes is written visibly on every route, and what
// a byte past ASCII is depends on the locale: under UTF-8 a character is
// itself and a byte that begins none is `\M-` and its low half; under C only
// 0x80 to 0x9f take `\M-`. The near text of an unmatched construct and a
// refused token alike. Measured 2026-10-16 on zsh 5.9.2, `-f`, by a file,
// standard input, `-c`, `eval`, `.`, a trap's text, `-n` and a startup file
// (#6786).
func TestAParseFailureQuotesTheScriptVisiblyInItsLocale(t *testing.T) {
	for _, c := range []struct{ locale, script, want string }{
		{"en_US.UTF-8", "v=$(echo \xe1a", "s.zsh:2: parse error near `v=$(echo \\M-aa'\n"},
		{"en_US.UTF-8", "v=$(echo \xe2\x82\xaca", "s.zsh:2: parse error near `v=$(echo \xe2\x82\xaca'\n"},
		{"en_US.UTF-8", "{ echo; } \xe1a", "s.zsh:1: parse error near `\\M-aa'\n"},
		{"en_US.UTF-8", "{ echo; } \x01a", "s.zsh:1: parse error near `^Aa'\n"},
		{"en_US.UTF-8", "{ echo; } \xe2\x82a", "s.zsh:1: parse error near `\\M-b\\M-^Ba'\n"},
		{"C", "v=$(echo \xe1a", "s.zsh:2: parse error near `v=$(echo \xe1a'\n"},
		{"C", "{ echo; } \xe2\x82\xaca", "s.zsh:1: parse error near `\xe2\\M-^B\xaca'\n"},
	} {
		got := zshErrsInLocale(t, c.locale, map[string]string{"s.zsh": c.script + "\n"}, "-f", "s.zsh")
		if got != c.want {
			t.Errorf("%s %q:\n got %q\nwant %q", c.locale, c.script, got, c.want)
		}
	}
	// And `eval`'s text, which is written by the runner rather than the
	// front end.
	got := zshErrsInLocale(t, "en_US.UTF-8", map[string]string{"b": "{ echo; } \xe1a"}, "-fc", `eval "$(<b)"`)
	if want := "(eval):1: parse error near `\\M-aa'\n"; got != want {
		t.Errorf("eval: got %q, want %q", got, want)
	}
}
