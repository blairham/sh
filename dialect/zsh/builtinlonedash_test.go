// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A lone `-` handed to a **builtin** is the end of its options here, where
// the other columns pass it on as an operand. See
// interp.Semantics.LoneDashIsAnOption — this is not the word standing where
// the *command name* goes, which is the other question the same shell
// answers yes (#3236).
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME.
//
// `echo` had its own option reader rather than the shared one and so had
// never asked the axis at all, while `print` in the same shell ate the word —
// which is what made the gap visible (#5026).
func TestEchoReadsALoneDashAsTheEndOfItsOptions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo - hi`, "hi\n"},
		// The discriminating row: a reading where the word is merely
		// *dropped* would print `hi` with no newline, having read the `-n`.
		{`echo - -n hi`, "-n hi\n"},
		{`echo -n - hi`, "hi"},
		{`echo - - hi`, "- hi\n"},
		{`echo -E - hi`, "hi\n"},
		{`echo -en - hi`, "hi"},
		{`echo -nE - hi`, "hi"},
		{`echo -`, "\n"},
		{`echo - -e 'a\tb'`, "-e a\tb\n"},
		{`echo -e - 'a\tb'`, "a\tb\n"},
		// The controls. `print` in the same shell already ate it, `--` is an
		// ordinary operand to `echo`, `printf` does not take the axis at
		// all, and a dash behind an operand is past the options.
		{`print - hi`, "hi\n"},
		{`print - -n hi`, "-n hi\n"},
		{`echo -- hi`, "-- hi\n"},
		{`printf - hi`, "-"},
		{`echo x - y`, "x - y\n"},
		{`echo -n x - y`, "x - y"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src+"\n")
		if err != nil {
			t.Fatal(err)
		}
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestALoneDashEndsABuiltinsOptionsRatherThanBeingSkipped: the shared option
// reader ate the dash and **carried on**, so a dash-word behind it was still
// read as the option it spells.
//
// The two readings agree wherever nothing but operands follows the dash,
// which is every row the axis was first measured from — `export -` lists and
// `unset - v` unsets under either one. An option word behind the dash is what
// tells them apart, and it moves every builtin that has one.
func TestALoneDashEndsABuiltinsOptionsRatherThanBeingSkipped(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"v=1\nunset - -v v\n", "unset:2: -v: invalid parameter name"},
		{"read - -r x\n", "not an identifier: -r"},
		{"hash - -r\n", "hash:1: no such command: -r"},
		{"unalias - -m 'x*'\n", "unalias:1: no such hash table element: -m"},
		// A dash and a `--` are one answer to `alias`, which is the same
		// fact said the other way: the dash **ended** the options.
		{"alias a=b\nalias - -L\nprint st=$?\n", "st=1"},
		{"alias a=b\nalias -- -L\nprint st=$?\n", "st=1"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
	}
	// The controls, one line along: each of those letters with no dash in
	// front of it is still read as the option it spells.
	for _, tc := range []struct{ src, want string }{
		{"v=1\nunset -v v\nprint \"v=[$v]\"\n", "v=[]"},
		{"alias a=b\nalias -L\n", "alias a=b"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("control %s = %q, want %q in it", tc.src, out, tc.want)
		}
	}
}
