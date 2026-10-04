// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

func kshC(t *testing.T, src string, env ...string) string {
	t.Helper()
	for _, kv := range env {
		k, v, _ := bytes.Cut([]byte(kv), []byte("="))
		t.Setenv(string(k), string(v))
	}
	var out, errs bytes.Buffer
	driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", src})
	return out.String() + errs.String()
}

// A letter range keeps to one case. Measured 2026-10-03 on ksh93u+.
func TestALetterRangeKeepsToOneCase(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{`{Z..a}`, "[{Z..a}]"},
		{`{a..Z}`, "[{a..Z}]"},
		{`{A..z..10}`, "[{A..z..10}]"},
		{`x{Z..a}y`, "[x{Z..a}y]"},
		{`{A..C}`, "[A][B][C]"},
		{`{c..a}`, "[c][b][a]"},
	} {
		if got := kshC(t, `printf "[%s]" `+c.body); got != c.want {
			t.Errorf("%s: %q, want %q", c.body, got, c.want)
		}
	}
}

// A quoted dash in a bracket is still the range operator, where an escaped
// one is a member. Measured 2026-10-03 on ksh93u+.
func TestAQuotedDashInABracketIsARange(t *testing.T) {
	loop := func(pat string) string {
		return `v=-; for s in a - z b; do case $s in ` + pat + `) printf "[%s]" "$s";; esac; done`
	}
	for _, c := range []struct{ pat, want string }{
		{`[a"-"z]`, "[a][z][b]"},
		{`["a-z"]`, "[a][z][b]"},
		{`[a'-'z]`, "[a][z][b]"},
		{`[a"$v"z]`, "[a][z][b]"},
		{`[a\-z]`, "[a][-][z]"},
		{`[a-z]`, "[a][z][b]"},
	} {
		if got := kshC(t, loop(c.pat)); got != c.want {
			t.Errorf("%s: %q, want %q", c.pat, got, c.want)
		}
	}
}

// A character outside ASCII is alpha and in no other class. Measured
// 2026-10-03 on ksh93u+ under LC_ALL=C.UTF-8.
func TestAWideCharacterIsInAlphaAlone(t *testing.T) {
	src := `for v in é É 日 Ⅷ ٣ ½ ·; do printf "%s:" "$v"; for c in alpha alnum upper lower digit space punct print graph; do case $v in [[:$c:]]) printf "%s," "$c";; esac; done; printf " "; done`
	const want = "é:alpha, É:alpha, 日:alpha, Ⅷ:alpha, ٣:alpha, ½: ·: "
	if got := kshC(t, src, "LC_ALL=C.UTF-8"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
