// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheVisibleFlagReachesPastASCII pins `(V)` over a byte that begins no
// character: `\M-` and the visible form of its low seven bits, every such
// byte under a locale that counts characters and only 0x80 to 0x9f under one
// that does not. Measured 2026-10-02 on zsh 5.9.2 (#5315).
func TestTheVisibleFlagReachesPastASCII(t *testing.T) {
	for _, tc := range []struct{ locale, src, want string }{
		{"en_US.UTF-8", `x=$'\x9b'; print -r -- ${(V)x}`, `\M-^[` + "\n"},
		{"en_US.UTF-8", `x=$'\x89'; print -r -- ${(V)x}`, `\M-\t` + "\n"},
		{"en_US.UTF-8", `x=$'\xe1'; print -r -- ${(V)x}`, `\M-a` + "\n"},
		{"en_US.UTF-8", `x=$'\xe2\x82'; print -r -- ${(V)x}`, `\M-b\M-^B` + "\n"},
		{"en_US.UTF-8", `x=é; print -r -- ${(V)x}`, "é\n"},
		{"C", `x=$'\x9b'; print -r -- ${(V)x}`, `\M-^[` + "\n"},
		{"C", `x=$'\xe1'; print -r -- ${(V)x}`, "\xe1\n"},
	} {
		got, _ := runZshUTF8Locale(t, tc.locale, tc.src)
		if got != tc.want {
			t.Errorf("%s %s\n got %q\nwant %q", tc.locale, tc.src, got, tc.want)
		}
	}
}

// runZshUTF8Locale is runZshUTF8 under a locale of the caller's choosing.
func runZshUTF8Locale(t *testing.T, locale, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	dir := t.TempDir()
	r := &interp.Runner{
		Stdout: &o, Stderr: &o, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Route: interp.RouteScriptFile,
		Vars:    map[string]string{"PATH": dir, "LC_ALL": locale},
		Dialect: presetDialect(),
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), st
}
