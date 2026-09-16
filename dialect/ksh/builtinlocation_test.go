// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// TestKshNamesThePlaceTwoWays records the measurement, against ksh93 93u+m on
// macOS. A builtin's own complaint brackets the line and everything else uses
// the word form, in the one script:
//
//	k.sh[1]: cd: /nope: [No such file or directory]
//	k.sh: line 1: nosuchcmd: not found
//
// Measured across cd, unset, kill, shift, export, let and trap on one side,
// and a missing command, a readonly reassignment, a division by zero and an
// unset parameter under `set -u` on the other.
func TestKshNamesThePlaceTwoWays(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{"cd", "cd /no/such/dir-xyz", "ksh[2]: "},
		{"shift", "shift 99", "ksh[2]: "},
		{"unset", "unset -f 1x", "ksh[2]: "},
		{"not found", "nosuchcmd-xyz", "ksh: line 2: "},
		{"divide by zero", "echo $((1/0))", "ksh: line 2: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// On line 2, because line 1 of a command string names no
			// line at all — in either of the two styles.
			out, _ := runKsh(t, dir, "true\n"+tc.src)
			if !strings.HasPrefix(out, tc.want) {
				t.Errorf("output = %q, want it to open with %q", out, tc.want)
			}
		})
	}
}

// TestKshLeavesTheLineOutOnTheFirstLineOfACommandString is the same
// invocation dependence the word form already had, and it applies to the
// bracketed form too: `ksh -c 'cd /nope'` names no line, `ksh -c $'true\ncd
// /nope'` says `ksh[2]:`, and a script file says `k.sh[1]:` on line 1.
func TestKshLeavesTheLineOutOnTheFirstLineOfACommandString(t *testing.T) {
	dir := t.TempDir()

	first, _ := runKsh(t, dir, "cd /no/such/dir-xyz")
	if !strings.HasPrefix(first, "ksh: cd: ") {
		t.Errorf("line 1 of a command string = %q, want no line named", first)
	}

	later, _ := runKsh(t, dir, "true\ncd /no/such/dir-xyz")
	if !strings.HasPrefix(later, "ksh[2]: ") {
		t.Errorf("line 2 of a command string = %q, want the line named", later)
	}
}

// TestOnlyKshNamesThePlaceTwoWays keeps the field from spreading: it is one
// dialect's habit, and the other three name the place one way.
func TestOnlyKshNamesThePlaceTwoWays(t *testing.T) {
	if ksh.Diagnostics().BuiltinLocation == 0 {
		t.Error("ksh93 should name a builtin's place its own way")
	}
	if ksh.Diagnostics().ScriptBuiltinLocation == 0 {
		t.Error("ksh93 should name it its own way in a script too")
	}
}

// TestTestKeepsTheBuiltinLocation is the same two styles asked of the one
// builtin that used to write both.
//
// `test` has three complaints in this dialect and they were not located
// alike: a word standing where an operator belonged took the *parser's*
// `<file>: line N: ` and every other one took `<file>[N]: `. Measured
// 2026-09-15 on AT&T ksh93u+ 2012-08-01, in a script called `t.sh`:
//
//	t.sh[1]: test: b: unknown operator      test a b c
//	t.sh[1]: [: b: unknown operator         [ a b c ]
//	t.sh[1]: [: -Q: unknown operator        [ -Q x ]
//
// All three bracketed, so the disagreement was ours alone (#3035). The cause
// was a rule measured on the dialect that writes a builtin's name *into* the
// location — there `test a b c` is `zsh:1: condition expected: b`, the same
// as `[[ a b c ]]`, which is not a builtin — applied to a dialect that has a
// second location style instead.
func TestTestKeepsTheBuiltinLocation(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"a word where a binary operator belongs", "test a b c"},
		{"the same through the bracket", "[ a b c ]"},
		{"a word after a file test", "test -n x y"},
		{"a word where a unary operator belongs", "[ -Q x ]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// On line 2, because line 1 of a command string names no line
			// at all and the two styles are indistinguishable there.
			out, _ := runKsh(t, dir, "true\n"+tc.src)
			if !strings.HasPrefix(out, "ksh[2]: ") {
				t.Errorf("output = %q, want it to open with %q", out, "ksh[2]: ")
			}
		})
	}
}
