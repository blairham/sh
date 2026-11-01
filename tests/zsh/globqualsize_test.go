// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The `L` qualifier compares a file's size, in bytes or in the unit a letter
// names, rounded up to whole units; and with nothing to compare it is
// `number expected`, the sentence `a`, `m` and `c` give (#6613). Measured
// 2026-10-15 on zsh 5.9.2 with `-fc`; the rows are on interp's
// parseGlobSize.
func TestTheSizeQualifier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []int{0, 1, 512, 513, 1024, 1025} {
		name := filepath.Join(dir, "s"+strconv.Itoa(n))
		if err := os.WriteFile(name, make([]byte, n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ q, want string }{
		{"L1024", "s1024\n"},
		{"L+1024", "s1025\n"},
		{"L-513", "s0 s1 s512\n"},
		{"Lk1", "s1 s1024 s512 s513\n"},
		{"LK1", "s1 s1024 s512 s513\n"},
		{"Lk0", "s0\n"},
		{"Lk+1", "s1025\n"},
		{"Lp1", "s1 s512\n"},
		{"Lp2", "s1024 s513\n"},
		{"Lm1", "s1 s1024 s1025 s512 s513\n"},
		{"^Lk1", "s0 s1025\n"},
		{"L", "zsh:1: number expected\n"},
		{"Lk", "zsh:1: number expected\n"},
		{"Lx", "zsh:1: number expected\n"},
		{"L1k", "zsh:1: unknown file attribute: k\n"},
	} {
		src := "print -r -- s*(" + c.q + ")"
		if out, _ := runZsh(t, dir, src); out != c.want {
			t.Errorf("%s: %q, want %q", src, out, c.want)
		}
	}
}

// An expression in a qualifier's subscript that will not read is worded as
// the same expression is in an array's subscript (#6613).
func TestAQualifierSubscriptIsWordedAsAnExpression(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ src, want string }{
		{"print *([1,x+])", "zsh:1: bad math expression: operand expected at end of string\n"},
		{"print *([x+])", "zsh:1: bad math expression: operand expected at end of string\n"},
		{"print *([1,2+*])", "zsh:1: bad math expression: operand expected at `*'\n"},
		{"print *([1,])", "zsh:1: bad math expression: empty string\n"},
		{"print *([1,1/0])", "zsh:1: division by zero\n"},
	} {
		if out, _ := runZsh(t, dir, c.src); out != c.want {
			t.Errorf("%s: %q, want %q", c.src, out, c.want)
		}
	}
}
