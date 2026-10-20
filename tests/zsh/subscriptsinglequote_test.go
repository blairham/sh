// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Single quotes in a key are characters of the key and stop nothing: what is
// between them is substituted, and escaped, as the rest of the subscript is.
// Measured on zsh 5.9.2 (`-f`, a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`), 2026-10-03 (#5268). bash 5.3.20 and ksh93u+ store `a$b` for the
// first row — their subscript is a quoting context — which is
// Semantics.SubscriptIsAQuotingContext's other answer and not this one's.
func TestASingleQuotedKeyIsSubstituted(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `b=X; typeset -A h
h['a$b']=1; h['1$(echo Q)']=1; h['2\$b']=1; h['3${b}c']=1; h['4$((1+1))']=1
h['5"$b"']=1; h['6`+"`echo T`"+`']=1; h['7a\"b']=1; h['8a\\b']=1; h['9 a  b ']=1; h['$b']=1
for k in "${(@ko)h}"; print -r -- "[$k]"
print -r -- ${h['a$b']-unset} ${h[\'aX\']-unset}
a=(aX 'a$b' "'aX'" x)
print -r -- ${a[(i)'a$b']} ${a[(r)'a$b']}`)
	want := "['1Q']\n['2$b']\n['3Xc']\n['42']\n['5\"X\"']\n['6T']\n['7a\\\"b']\n['8a\\b']\n['9 a  b ']\n['X']\n['aX']\n" +
		"1 unset\n3 'aX'\n"
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}
