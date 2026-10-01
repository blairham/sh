// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// This shell's default IFS holds a NUL, and a NUL in it is a separator that is
// not whitespace: two in a row delimit an empty field, the way `:` would
// (#5263). It never split here, because a value's NUL was read as the mark
// that stands for a value's backslash. Measured 2026-09-30 on zsh 5.9.2
// (`-f`, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`), each row
// the bytes that shell wrote.
func TestTheDefaultIFSSplitsAtANUL(t *testing.T) {
	const show = "show() { for x in \"$@\"; do printf '<%s>' \"$x\"; done; }\n"
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		// `read`, the route the issue was found on.
		{"read into two names", `printf 'a\0b c\0d\n' | { read -r a b; show "$a" "$b"; }`, "<a><b c\x00d>"},
		{"read keeps the trailing separator in the remainder", `printf 'a\0\0b\0\n' | { read -r x y z; show "$x" "$y" "$z"; }`, "<a><><b\x00>"},
		{"read into one name takes the line", `printf 'a\0b\n' | { read -r v; show "$v"; }`, "<a\x00b>"},
		// The split flag in a flag group, quoted and not.
		{"(@)= quoted", `v=$(printf 'a\0b'); show "${(@)=v}"`, "<a><b>"},
		{"(@)= unquoted", `v=$(printf 'a\0b'); show ${(@)=v}`, "<a><b>"},
		{"(@)= quoted, two in a row", `n=$(printf 'a\0\0b\0'); show "${(@)=n}"`, "<a><><b><>"},
		// The split flag on its own was right and must stay so.
		{"= unquoted", `v=$(printf 'a\0b'); show ${=v}`, "<a><b>"},
		{"= unquoted, two in a row", `n=$(printf 'a\0\0b\0'); show ${=n}`, "<a><><b><>"},
		// A value's backslash is still not a NUL, and is still data unless
		// IFS holds a backslash — which is where reading every NUL as the
		// mark split at the NUL and not at the backslash.
		{"a backslash value", `w='a\b c'; show "${(@)=w}"`, `<a\b><c>`},
		{"IFS a backslash, a backslash value", `IFS='\'; w='a\b c'; show "${(@)=w}"`, "<a><b c>"},
		{"IFS a backslash, a NUL value", `IFS='\'; v=$(printf 'a\0b'); show "${(@)=v}"`, "<a\x00b>"},
		{"IFS a backslash, read", `IFS='\'; printf 'a\0b c\0d\n' | { read -r a b; show "$a" "$b"; }`, "<a\x00b c\x00d><>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, show+c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
