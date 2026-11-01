// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The setup every row below runs behind, on its own first line.
const elementPrefixSetup = `typeset -A a; b=(x y z); i=1; f() { echo "f y=$y"; }`

// An entry behind a subscripted prefix entry reads the element that entry
// wrote. Measured 2026-10-29 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`,
// from `-c` and from a script file alike. A function's and a builtin's prefix
// are stored as they are walked here, so those routes already agreed; the
// program's child did not, and showed `y` the element the shell held (#6985).
// See interp/prefixsubscriptinturn.go.
//
// A string the subscript names a character of is held as the string the
// splice leaves. The controls are the function's rows, a splice among them,
// the substitutions' order and the element staying after a function.
func TestAPrefixElementIsSeenByTheEntriesBehindIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a key, to a program", `a[k]=2 y=${a[k]} /usr/bin/printenv y`, "2\n"},
		{"an index, to a program", `b[1]=Q y=${b[1]} /usr/bin/printenv y`, "Q\n"},
		{"every element, to a program", `b[1]=Q y="${b[*]}" /usr/bin/printenv y`, "Q y z\n"},
		{"a new array, to a program", `n[2]=Q y=${#n[@]} /usr/bin/printenv y`, "2\n"},
		{"an append, to a program", `b[1]+=Q y=${b[1]} /usr/bin/printenv y`, "xQ\n"},
		{
			"the subscript steps once in the child",
			`b[i++]=Q y=$i:${b[1]} /usr/bin/printenv y; echo "i=$i b=${b[*]}"`,
			"2:Q\ni=1 b=x y z\n",
		},
		{"a string spliced, to a program", `s=str; s[1]=Q y=$s /usr/bin/printenv y`, "Qtr\n"},
		{"an append spliced, to a program", `s=str; s[2]+=Q y=$s /usr/bin/printenv y`, "stQr\n"},
		// Controls.
		{"a key, to a function", `a[k]=1 y=${a[k]} f`, "f y=1\n"},
		{"an index, to a function", `b[1]=Q y=${b[1]} f`, "f y=Q\n"},
		{"a string spliced", `s=str; s[1]=Q y=$s f`, "f y=Qtr\n"},
		{
			"the value, the subscript's words, the entry behind",
			`b[$(echo s >&2; echo 1)]=$(echo t >&2; echo v) y=$(echo u >&2)${b[1]} f`,
			"t\ns\nu\nf y=v\n",
		},
		{"the element stays after a function", `b[1]=Q f; echo "${b[*]}"`, "f y=\nQ y z\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// A declaration's operand reads the element its prefix wrote, as it reads a
// scalar prefix's value (#6999): zsh works through the prefix first. Measured
// the same day and the same way, where this shell read the element the command
// found (#7012), or the string before the splice. Each shell sees the element
// its own subscript placed, so `B` is the first element here. The ordinary
// argument is the control.
func TestADeclarationsOperandSeesItsPrefixsElement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `c=(p q r); c[1]=B typeset y=${c[1]}; echo "y=$y ${c[*]}"`, "y=B B q r\n"},
		{"export", `c=(p q r); c[1]=B export y=${c[1]}; echo "y=$y ${c[*]}"`, "y=B B q r\n"},
		{"a key", `typeset -A m; m[k]=B typeset y=${m[k]}; echo "y=$y ${m[k]}"`, "y=B B\n"},
		{"every operand", `c=(p q r); c[1]=B typeset y=${c[1]} z=${#c[@]}; echo "y=$y z=$z"`, "y=B z=3\n"},
		{
			"in a function-form body",
			`function g { c=(p q r); c[1]=B typeset y=${c[1]}; echo "y=$y ${c[*]}"; }; g`,
			"y=B B q r\n",
		},
		{"the subscript steps once", `b[i++]=Q typeset y=$i:${b[1]}; echo "y=$y i=$i"`, "y=2:Q i=2\n"},
		{"a string spliced", `s=str; s[1]=Q typeset z=$s; echo "z=$z s=$s"`, "z=Qtr s=Qtr\n"},
		{"counting back in a string", `s=str; s[-1]=Q typeset z=$s; echo "z=$z"`, "z=stQ\n"},
		{
			"the value before the subscript's words",
			`a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) y=$(echo u >&2)${a[k]} typeset z=$(echo o >&2)${a[k]}` +
				`; echo "y=$y z=$z"`,
			"t\ns\nu\no\ny= z=v\n",
		},
		// Control.
		{"an ordinary argument", `c=(p q r); c[1]=B echo "${c[1]}"`, "p\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// Under `set -x` the trace's walk expands the values ahead of the store, so the
// element is held there too and the trace does not change the answer: this
// shell showed the body `y=x`, and handed `read` the separator `x`. Measured
// the same day and the same way.
func TestATracedPrefixElementIsSeenByTheEntriesBehindIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a function", `set -x; b[1]=Q y=${b[1]} f`, "+zsh:2> b[1]=Q y=Q +zsh:2> f\n+f:0> echo 'f y=Q'\nf y=Q\n"},
		{"a key", `set -x; a[k]=1 y=${a[k]} f`, "+zsh:2> a[k]=1 y=1 +zsh:2> f\n+f:0> echo 'f y=1'\nf y=1\n"},
		{
			"the subscript steps once", `set -x; b[i++]=Q y=$i:${b[1]} f; echo "i=$i"`,
			"+zsh:2> b[i++]=Q y=2:Q +zsh:2> f\n+f:0> echo 'f y=2:Q'\nf y=2:Q\n+zsh:2> echo 'i=2'\ni=2\n",
		},
		{
			"a special builtin", `set -x; b[1]=Q y=${b[1]} eval 'echo "y=$y"'`,
			"+zsh:2> b[1]=Q y=Q +zsh:2> eval 'echo \"y=$y\"'\n+(eval):1> echo 'y=Q'\ny=Q\n",
		},
		{"typeset", `set -x; b[1]=Q typeset y=${b[1]}`, "+zsh:2> b[1]=Q +zsh:2> typeset y=Q\n"},
		{
			"a string spliced", `s=str; set -x; s[1]=Q y=$s f`,
			"+zsh:2> s[1]=Q y=Qtr +zsh:2> f\n+f:0> echo 'f y=Qtr'\nf y=Qtr\n",
		},
		{
			"a regular builtin", "set -x; b[1]=: IFS=${b[1]} read u v <<EOT\np:q\nEOT\necho \"u=$u v=$v\"",
			"+zsh:2> b[1]=: IFS=: +zsh:2> read u v\n+zsh:5> echo 'u=p v=q'\nu=p v=q\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
