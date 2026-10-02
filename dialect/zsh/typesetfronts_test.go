// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTypesetFrontsOfB02 holds the B02typeset fronts this change closes
// (#5142). Every row measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`).
func TestTypesetFrontsOfB02(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The width words, after `local` and ahead of the case.
		{`typeset -L10 x=a; typeset +m x`, "left justified 10 x\n"},
		{`typeset -R5 y=b; typeset +m y`, "right justified 5 y\n"},
		{`typeset -Z3 z=1; typeset +m z`, "zero filled 3 z\n"},
		{`typeset -L 10 -F 3 f; typeset +m f`, "float left justified 10 f\n"},
		{`typeset -LZ4 q=1; typeset +m q`, "left justified 4 zero filled 4 q\n"},
		{`typeset -i16 -R6 h=255; typeset +m h`, "integer 16 right justified 6 h\n"},
		{`typeset -rL5 x=a; typeset +m x`, "left justified 5 readonly x\n"},
		{`f() { local -uL5 x=a; typeset +m x }; f`, "local left justified 5 uppercase x\n"},
		{`f() { local -u x=a; typeset +m x }; f`, "local uppercase x\n"},
		{`typeset -L x; typeset +m x`, "left justified x\n"},
		// The zero fill keeps a value's leading blanks.
		{
			`typeset -Z 6 z; for v in "  4" " 4" " 4 " " -4" "a4"; do z=$v; print -r -- "[$z]"; done`,
			"[  0004]\n[ 00004]\n[ 0004 ]\n[    -4]\n[    a4]\n",
		},
		// And fills an integer's or a float's zeros after its sign and base.
		{
			`integer -Z 10 n; for v in 42 -42 " 43" " -43"; do n=$v; print -r -- "[$n]"; done`,
			"[0000000042]\n[-000000042]\n[0000000043]\n[-000000043]\n",
		},
		{`typeset -i16 -Z 10 h=255; print -r -- "[$h]"; h=-255; print -r -- "[$h]"`, "[16#00000FF]\n[-16#0000FF]\n"},
		{`setopt cbases; integer -Z 10 -i 16 h=42; print -r -- "[$h]"; h=-42; print -r -- "[$h]"`, "[0x0000002A]\n[-0x000002A]\n"},
		{`typeset -F 3 -Z 10 f; for v in 3.14159 -3.14159; do f=$v; print -r -- "[$f]"; done`, "[000003.142]\n[-00003.142]\n"},
		{`integer -Z 4 n=-12345; print -r -- "[$n]"`, "[2345]\n"},
		{`integer -L 6 n=-4; print -r -- "[$n]"; typeset -p n`, "[-4    ]\ntypeset -iL6 n=-4\n"},
		{`integer -R 6 n=-4; print -r -- "[$n]"`, "[    -4]\n"},
		// `(P)` reads the name from what the parameter holds.
		{`ABC=UP; abc=low; typeset -u n=abc; print ${(P)n}`, "low\n"},
		{`abc=HIT; typeset -R5 n=abc; print -r -- "[${(P)n}]"`, "[HIT]\n"},
		{`abc=HIT; typeset -l n=ABC; ABC=UPV; print ${(P)n}`, "UPV\n"},
		// Under POSIX_BUILTINS a valueless declaration leaves nothing, and
		// the readonly attribute stays on.
		{
			`setopt posixbuiltins; readonly x; print ${+x}; typeset -r y; print ${+y}; export z; print ${+z}; f() { local -r q; print ${+q} }; f`,
			"0\n0\n0\n0\n",
		},
		{`setopt posixbuiltins; [[ -o typesettounset ]] && echo on || echo off`, "off\n"},
		{`setopt posixbuiltins; readonly r1; readonly -p`, "readonly r1\n"},
		{`setopt posixbuiltins; v=1; readonly v; typeset +r v; echo $?`, "zsh:1: read-only variable: v\n"},
		{`v=1; readonly v; typeset +r v; echo $?; v=2; echo $v`, "0\n2\n"},
		// A frozen produced table is silent to `-p`.
		{`zmodload zsh/parameter; readonly foo=bar; readonly -p`, "typeset -r foo=bar\n"},
		{`zmodload zsh/parameter; typeset -p builtins funcstack reswords; echo $?`, "0\n"},
		{`zmodload zsh/parameter; x=$(typeset -p functions); print -r -- ${x[1,19]}`, "typeset -A function\n"},
		{`zmodload zsh/system; typeset -p sysparams`, "typeset -Ar sysparams\n"},
		// A local in front of a table still waiting for its module is the
		// local's to lose.
		{`f() { typeset -h +g keymaps; unset keymaps }; f; readonly -p`, ""},
		{`f() { typeset -h +g -m "*"; unset -m "*" }; f; readonly foo=1; readonly -p`, "typeset -r foo=1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
