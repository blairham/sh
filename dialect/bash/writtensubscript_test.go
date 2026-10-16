// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What this shell writes back for a word it quotes as written — a traced
// subscript, a traced array literal, a refusal's name and a function listing —
// when the word holds a substitution (#6679).
//
// The rule, measured 2026-10-15 on bash 5.3.20 with -c and from a file: the
// text kept for a word is the text it was parsed from, except that the body of
// every `$( … )` and `<( … )` in it is read again and laid out the way the
// function listing lays one out — in double quotes, inside a `${ … }` and
// inside arithmetic alike. A backquoted body is kept as written, and so is
// everything around a substitution, the blanks of a subscript included.
//
// Every `want` below is that shell's own output, byte for byte, with PATH
// pointing nowhere.
func TestAWordWrittenBackLaysOutItsSubstitutions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a traced subscript respaces the redirection",
			src:  "set -x; a[$(true 2>/dev/null)]=x",
			want: "+ a[$(true 2> /dev/null)]=x\n++ true\n",
		},
		{
			name: "inside double quotes",
			src:  `set -x; a["$(true;true)"]=x`,
			want: "+ a[\"$(true; true)\"]=x\n++ true\n++ true\n",
		},
		{
			name: "inside arithmetic",
			src:  "set -x; a[$((1+$(true;echo 0)))]=x",
			want: "+ a[$((1+$(true; echo 0)))]=x\n++ true\n++ echo 0\n",
		},
		{
			name: "a traced array literal's subscript",
			src:  "set -x; a=([$(true;echo 1)]=x)",
			want: "+ a=([$(true; echo 1)]=x)\n++ true\n++ echo 1\n",
		},
		{
			// The control: a backquoted body is not laid out.
			name: "a backquoted body is kept",
			src:  "set -x; a[`true;true`]=x",
			want: "+ a[`true;true`]=x\n++ true\n++ true\n",
		},
		{
			// The brackets delimit a subscript, so its blanks are not
			// escaped on the way out.
			name: "a subscript's blanks",
			src:  "set -x; a[1 + 1]=x",
			want: "+ a[1 + 1]=x\n",
		},
		{
			name: "the name a refused prefix quotes back",
			src:  "a[$(true;true)]=x true",
			want: "bash: line 1: `a[$(true; true)]': not a valid identifier\n",
		},
		{
			name: "the listing, in arithmetic, a process substitution and a subscript",
			src: "f(){ echo $((1+$(a;b))) $[1+$(a;b)]; (( x=$(a;b) )); for ((i=$(a;b);;)); do :; done; " +
				"cat <(a;b) <( (a) ); a[1<2]=1; a[1 + 1]=2; }; declare -f f",
			want: "f () \n{ \n    echo $((1+$(a; b))) $[1+$(a; b)];\n    (( x=$(a; b) ));\n" +
				"    for ((i=$(a; b); 1; 1))\n    do\n        :;\n    done;\n" +
				"    cat <(a; b) <( ( a ));\n    a[1<2]=1;\n    a[1 + 1]=2\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
