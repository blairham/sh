// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A quoted `"${a[@]}"` here asks whether the name **exists**, not whether it
// is empty: a name that is not a declared array reads as a scalar, so one
// nothing declared is the single empty field `"$a"` gives, while an array
// declared with no elements is no field at all.
//
// This dialect is alone in the panel on the first half and unanimous on the
// second. Measured against zsh 5.9.2 on 2026-09-07, from a file, with a
// *function* rather than `set --` so the positional-parameter builtin is not
// a confound. The asymmetry runs the opposite way from the one an
// empty-array rule would predict — `a=()` is the row with no field — which is
// why the axis is named for existence (#1379).
func TestAQuotedAtAsksWhetherTheNameExists(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The half this shell is alone on.
		{"never declared", `f() { echo "n=$#"; }; f "${a[@]}"`, "n=1"},
		{"unset after filling", `f() { echo "n=$#"; }; a=(x y); unset a; f "${a[@]}"`, "n=1"},
		// The half nobody diverges on: a declared array with no elements.
		{"empty literal", `f() { echo "n=$#"; }; a=(); f "${a[@]}"`, "n=0"},
		{"typeset -a", `f() { echo "n=$#"; }; typeset -a a; f "${a[@]}"`, "n=0"},
		{"empty association", `f() { echo "n=$#"; }; typeset -A m; f "${m[@]}"`, "n=0"},
		// Two more lists that exist and are empty without any array store
		// behind them, because the guard is about what a name *holds* and
		// not about which table it is in: the positional parameters with
		// none set, and one of this shell's produced arrays that has
		// nothing to produce yet.
		{"no positional parameters", `f() { echo "n=$#"; }; set --; f "${@[@]}"`, "n=0"},
		{"a produced array with nothing in it", `f() { echo "n=$#"; }; f "${dis_patchars[@]}"`, "n=0"},
		// The scalar reading the first half is an instance of, spelled out:
		// a name holding the empty string is one field in every shell, and it
		// is the answer an undeclared name borrows here.
		{"a scalar holding nothing", `f() { echo "n=$#"; }; a=; f "${a[@]}"`, "n=1"},
		// Controls.
		{"one element", `f() { echo "n=$#"; }; a=(x); f "${a[@]}"`, "n=1"},
		{"the count on nothing", `echo "n=${#a[@]}"`, "n=0"},
		{"unquoted on nothing", `f() { echo "n=$#"; }; f ${a[@]}`, "n=0"},
		{"a quoted star on nothing", `f() { echo "n=$#"; }; f "${a[*]}"`, "n=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The directory stack is read through the guarded spelling, and this is the
// test that says why: nothing declares the stack until the first push, so the
// plain `"${DIRSTACK[@]}"` took the scalar reading above and stored an empty
// entry beside the old directory. `dirs` then printed a trailing space for it
// — the whole of the visible damage, and invisible to a count.
//
// Asserted as bytes for that reason. Real zsh cannot reach the state at all:
// its stack parameter is an array from startup.
func TestTheDirectoryStackIsNotGivenAnEmptyEntryByAnUndeclaredName(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshPrelude(t, dir, `cd `+dir+` && pushd / && dirs && popd && echo "p=$?"`)
	if st != 0 {
		t.Fatalf("status = %d, want 0 (out %q)", st, out)
	}
	// One line from `dirs`, and it ends at the last entry: no trailing space,
	// which is what an empty element on the stack would show as.
	line := strings.SplitN(out, "\n", 2)[0]
	if line != strings.TrimRight(line, " ") {
		t.Errorf("dirs = %q, want no trailing space — an empty stack entry", line)
	}
	if strings.Count(line, " ") != 1 {
		t.Errorf("dirs = %q, want exactly two entries on the line", line)
	}
}
