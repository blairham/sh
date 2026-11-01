// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `unset` takes the hide word off any binding a script made — a hidden local
// of one of the shell's own names included — and leaves it on the shell's
// own parameter. Measured 2026-10-16 on zsh 5.9.2, `-f`, under `env -i
// PATH=/usr/bin:/bin` (#6741).
func TestUnsetDropsTheHideWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`f(){ local -h v; unset v; v=x; echo ${(t)v}; }; f`, "scalar-local\n"},
		{`f(){ local -h -a v; unset v; v=x; echo ${(t)v}; }; f`, "scalar-local\n"},
		{`f(){ local -h -a v; unset v; : ${v:=x}; echo ${(t)v}; }; f`, "scalar-local\n"},
		{`f(){ local -h v; g; echo ${(t)v}; }; g(){ unset v; v=x; }; f`, "scalar-local\n"},
		{`f(){ local -h HOME=3; unset HOME; HOME=4; echo ${(t)HOME}; }; f`, "scalar-local\n"},
		{`typeset -h v; unset v; v=x; echo ${(t)v}`, "scalar\n"},
		// The controls: the shell's own parameter keeps the word, and an
		// array store changes the kind, which drops it anyway.
		{`typeset -h WORDCHARS; unset WORDCHARS; WORDCHARS=x; echo ${(t)WORDCHARS}`, "scalar-hide-special\n"},
		{`f(){ local -h v; unset v; v=(x); echo ${(t)v}; }; f`, "array-local\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}

// `unset` of a binding the tie no longer reaches — a local of a script's tie
// half, or a hidden local of one of the shell's own — leaves the outer pair
// alone. Measured 2026-10-16 on zsh 5.9.2 (#6822).
func TestUnsetOfADetachedTieHalfLeavesTheOtherHalf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`typeset -T FOO foo=(a b); f(){ local FOO=x; unset FOO; echo "[$foo]"; }; f; echo $foo`, "[a b]\na b\n"},
		{`typeset -T FOO foo=(a b); f(){ local -h FOO=x; unset FOO; echo "[$foo]"; }; f; echo $foo`, "[a b]\na b\n"},
		{`PATH=/usr/bin:/bin; f(){ local -h PATH=/x; unset PATH; echo $path; }; f`, "/usr/bin /bin\n"},
		{`PATH=/usr/bin:/bin; f(){ local -h path; unset path; path=(/q); echo $PATH; }; f`, "/usr/bin:/bin\n"},
		{`PATH=/usr/bin:/bin; f(){ local -h PATH=/q; unset PATH; PATH=/y; echo ${(t)PATH} $path; }; f`, "scalar-local /usr/bin /bin\n"},
		// The control: the tie's own binding takes both halves.
		{`typeset -T FOO foo=(a b); unset FOO; echo "[$foo]"`, "[]\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
