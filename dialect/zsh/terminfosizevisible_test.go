// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"runtime"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestTheVisibleFlagReachesPastASCII pins `(V)` over a byte that begins no
// character: `\M-` and the visible form of its low seven bits, every such
// byte under a locale that counts characters and, under one that does not,
// every byte the C library does not class as printing. Measured 2026-10-02 on zsh 5.9.2 (#5315).
func TestTheVisibleFlagReachesPastASCII(t *testing.T) {
	for _, tc := range []struct{ locale, src, want string }{
		{"en_US.UTF-8", `x=$'\x9b'; print -r -- ${(V)x}`, `\M-^[` + "\n"},
		{"en_US.UTF-8", `x=$'\x89'; print -r -- ${(V)x}`, `\M-\t` + "\n"},
		{"en_US.UTF-8", `x=$'\xe1'; print -r -- ${(V)x}`, `\M-a` + "\n"},
		{"en_US.UTF-8", `x=$'\xe2\x82'; print -r -- ${(V)x}`, `\M-b\M-^B` + "\n"},
		{"en_US.UTF-8", `x=é; print -r -- ${(V)x}`, "é\n"},
		{"C", `x=$'\x9b'; print -r -- ${(V)x}`, `\M-^[` + "\n"},
		{"C", `x=$'\xe1'; print -r -- ${(V)x}`, cLocaleHighByte("\xe1", `\M-a`)},
		// Latin-1's soft hyphen, which macOS's C locale classes as no
		// printing character and glibc's classes as none of the high half
		// either: `\M--` on both.
		{"C", `x=$'\xad'; print -r -- ${(V)x}`, `\M--` + "\n"},
		{"C", `x=$'\xff'; print -r -- ${(V)x}`, cLocaleHighByte("\xff", `\M-^?`)},
	} {
		got, _ := runZshUTF8Locale(t, tc.locale, tc.src)
		if got != tc.want {
			t.Errorf("%s %s\n got %q\nwant %q", tc.locale, tc.src, got, tc.want)
		}
	}
}

// cLocaleHighByte is what `(V)` writes under the C locale for a byte from
// 0xa0 up other than 0xad, which is the C library's answer: macOS classes it
// as printing and writes it as itself, and glibc does not and writes it
// `\M-` and its low half. Measured 2026-10-02 on zsh 5.9.2 on this Mac and
// inside `ghcr.io/blairham/sh/zsh@sha256:aab8255c…`, every byte from 0x80 to
// 0xff.
func cLocaleHighByte(raw, meta string) string {
	if runtime.GOOS == "darwin" {
		return raw + "\n"
	}
	return meta + "\n"
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
