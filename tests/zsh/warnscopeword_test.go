// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestScopeLintWordFollowsTheArithmeticRoute pins that the scope lints' word
// and their silence are keyed on how the assignment was written, not on the
// type of the name it lands on.
//
// Measured 2026-10-15 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `env -i
// PATH=/usr/bin:/bin`), each script's whole output (#6461). The rows come in
// pairs that hold one key fixed and move the other: a scalar and an integer
// each written plainly and by arithmetic, an assignment and an increment on
// one name, and an element written plainly and by arithmetic — so a rule keyed
// on the type passes half of each pair and fails the other.
func TestScopeLintWordFollowsTheArithmeticRoute(t *testing.T) {
	t.Parallel()
	const on = "setopt localoptions warn_nested_var warn_create_global"
	set := func(kind, name string) string {
		return "f: " + kind + " parameter " + name + " set in enclosing scope in function f\n"
	}
	made := func(kind, name string) string {
		return "f: " + kind + " parameter " + name + " created globally in function f\n"
	}
	for _, tc := range []struct{ name, src, want string }{
		{"scalar, plain", "x=5; f() { " + on + "; x=8; }; f", set("scalar", "x")},
		{"scalar, arithmetic", "x=5; f() { " + on + "; (( x=8 )); }; f", set("numeric", "x")},
		{"scalar, let", "x=5; f() { " + on + "; let x=8; }; f", set("numeric", "x")},
		{"integer, plain", "integer g=5; f() { " + on + "; g=8; }; f", set("scalar", "g")},
		{"integer, append", "integer g=5; f() { " + on + "; g+=1; }; f", set("scalar", "g")},
		{"integer, for", "integer g=5; f() { " + on + "; for g in 1; do :; done; }; f", set("scalar", "g")},
		{"integer, arithmetic", "integer g=5; f() { " + on + "; (( g=8 )); }; f", set("numeric", "g")},
		{"float, plain", "float g=5; f() { " + on + "; g=8; }; f", set("scalar", "g")},
		{"integer not special", "f() { " + on + "; MAILCHECK=6; }; f", set("scalar", "MAILCHECK")},
		{"scalar, increment", "x=5; f() { " + on + "; (( x++ )); (( --x )); (( x+=1 )); let 'x*=2'; }; f", ""},
		{"integer, increment", "integer g=5; f() { " + on + "; (( g++ )); }; f", ""},
		{
			"arithmetic for", "x=5; f() { " + on + "; for (( x=0; x<1; x++ )); do :; done; }; f",
			set("numeric", "x"),
		},
		{
			"increment creates", "f() { " + on + "; (( y++ )); (( y++ )); }; f",
			made("numeric", "y"),
		},
		{"element, plain", "a=(1 2); f() { " + on + "; a[1]=3; }; f", ""},
		{
			"element, arithmetic", "a=(1 2); f() { " + on + "; (( a[1]=3, a[2]=4 )); }; f",
			set("numeric", "a") + set("numeric", "a"),
		},
		{"element, increment", "a=(1 2); f() { " + on + "; (( a[1]++ )); (( a[2]+=1 )); }; f", ""},
		{"element, local", "a=(1 2); f() { " + on + "; local a; (( a[1]=3 )); }; f", ""},
		{"element creates", "f() { " + on + "; (( n[1]=3 )); }; f", made("numeric", "n")},
		{"element increment creates", "f() { " + on + "; (( n[1]++ )); }; f", made("numeric", "n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := runZshOnPath(t, t.TempDir(), tc.src); got != tc.want {
				t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
