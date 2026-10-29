// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The setup every row below runs behind, on its own first line.
const elementPrefixSetup = `typeset -A a; b=(x y z); i=1; f() { echo "f y=$y"; }`

// An entry behind a subscripted prefix entry reads the element that entry
// wrote. Measured 2026-10-29 on ksh93u+ 2012-08-01 (`/bin/ksh`) under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file alike. This shell held a
// scalar entry for the entries behind it and nothing for a subscripted one, so
// they read the element the shell held: `b[1]=Q y=${b[1]} f` showed the body
// `y=y` (#6985). See interp/prefixsubscriptinturn.go.
//
// The controls are rows this shell answered already: a regular builtin's prefix
// is stored as it is walked, the element stays after a function, and a frozen
// array and a subscript that will not evaluate are refused before anything is
// seen.
func TestAPrefixElementIsSeenByTheEntriesBehindIt(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a key, to a function", `a[k]=1 y=${a[k]} f`, "f y=1\n", 0},
		{"an index, to a function", `b[1]=Q y=${b[1]} f`, "f y=Q\n", 0},
		{"a key, to a program", `a[k]=2 y=${a[k]} /usr/bin/printenv y`, "2\n", 0},
		{"an index, to a program", `b[1]=Q y=${b[1]} /usr/bin/printenv y`, "Q\n", 0},
		{"a new array", `n[1]=Q y="${n[@]}" f`, "f y=Q\n", 0},
		{"every element", `b[1]=Q y="${b[@]}" f`, "f y=x Q z\n", 0},
		{"the count", `b[5]=Q y=${#b[@]} f`, "f y=4\n", 0},
		{"an append", `b[1]+=Q y=${b[1]} f`, "f y=yQ\n", 0},
		{"a keyed append", `a[k]=1; a[k]+=Q y=${a[k]} f`, "f y=1Q\n", 0},
		{"a string the write promotes", `s=str; s[1]=Q y="${s[*]}" f`, "f y=str Q\n", 0},
		{"a negative index", `b[-1]=Q y="${b[*]}" f`, "f y=x y Q\n", 0},
		{"two elements", `b[1]=Q b[2]=R y="${b[*]}" f`, "f y=x Q R\n", 0},
		{"a special builtin", `b[1]=Q y=${b[1]} :; echo "y=$y b=${b[*]}"`, "y=Q b=x Q z\n", 0},
		{"eval", `b[1]=Q y=${b[1]} eval 'echo "y=$y"'`, "y=Q\n", 0},
		{"a function a value calls", `g() { echo "${b[*]}"; }; b[1]=Q y=$(g) f`, "f y=x Q z\n", 0},
		{"in a function-form body", `function g { b[1]=Q y=${b[1]} f; }; g`, "f y=Q\n", 0},
		{"the subscript steps once", `b[i++]=Q y=$i:${b[1]} f; echo "i=$i"`, "f y=2:Q\ni=2\n", 0},
		{
			"and a program's in its child",
			`b[i++]=Q y=$i:${b[1]} /usr/bin/printenv y; echo "i=$i"`,
			"2:Q\ni=1\n", 0,
		},
		{
			"the subscript's words, the value, the entry behind",
			`b[$(echo s >&2; echo 1)]=$(echo t >&2; echo v) y=$(echo u >&2)${b[1]} f`,
			"s\nt\nu\nf y=v\n", 0,
		},
		// Controls.
		{
			"a regular builtin", "b[1]=: IFS=${b[1]} read u v <<EOT\np:q\nEOT\necho \"u=$u v=$v b=${b[*]}\"",
			"u=p v=q b=x y z\n", 0,
		},
		{"the element stays after a function", `b[1]=Q f; echo "${b[*]}"`, "f y=\nx Q z\n", 0},
		{"a frozen array", `readonly b; b[1]=Q y=${b[1]} f; echo after $?`, "ksh: line 2: b: is read only\n", 1},
		{"a subscript that will not evaluate", `b[1/0]=v y=1 f; echo after $?`, "ksh: line 2: 1/0: divide by zero\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != c.status || out != c.want {
				t.Errorf("got %q (status %d), want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// A declaration's operand reads the element its prefix wrote, as it reads a
// scalar prefix's value (#6999): ksh93 works through the prefix first. Measured
// the same day and the same way. This shell read the element the command found,
// and in a `function`-form body, where the prefix is a fresh local of the body
// (#7002), nothing at all (#7012). The last row is the control: an ordinary
// command's argument is expanded before its prefix.
func TestADeclarationsOperandSeesItsPrefixsElement(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `c=(p q r); c[1]=B typeset y=${c[1]}; echo "y=$y ${c[*]}"`, "y=B p B r\n"},
		{"export", `c=(p q r); c[1]=B export y=${c[1]}; echo "y=$y ${c[*]}"`, "y=B p B r\n"},
		{"a key", `typeset -A m; m[k]=B typeset y=${m[k]}; echo "y=$y ${m[k]}"`, "y=B B\n"},
		{"every operand", `c=(p q r); c[1]=B typeset y=${c[1]} z=${#c[@]}; echo "y=$y z=$z"`, "y=B z=3\n"},
		{
			"in a function-form body",
			`function g { c=(p q r); c[1]=B typeset y=${c[1]}; echo "y=$y ${c[*]}"; }; g`,
			"y=B B\n",
		},
		{
			"a key in a function-form body",
			`function g { typeset -A m; m[k]=1; m[k]=B typeset y=${m[k]}; echo "y=$y ${m[k]}"; }; g`,
			"y=B B\n",
		},
		{"the subscript steps once", `b[i++]=Q typeset y=$i:${b[1]}; echo "y=$y i=$i"`, "y=2:Q i=2\n"},
		{"the subscript's words before the value", `a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) typeset z=1`, "s\nt\n"},
		{
			"and both ahead of the operand",
			`a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) y=$(echo u >&2)${a[k]} typeset z=$(echo o >&2)${a[k]}` +
				`; echo "y=$y z=$z"`,
			"s\nt\nu\no\ny=v z=v\n",
		},
		// Control.
		{"an ordinary argument", `c=(p q r); c[1]=B echo "${c[1]}"`, "q\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// The same rows under `set -x`, whose walk is the one that expands the values:
// the element is held there too, so the trace does not change the answer.
// Measured the same day and the same way.
func TestATracedPrefixElementIsSeenByTheEntriesBehindIt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a function", `set -x; b[1]=Q y=${b[1]} f`, "+ b[1]=Q\n+ y=Q\n+ f\n+ echo 'f y=Q'\nf y=Q\n"},
		{"a key", `set -x; a[k]=1 y=${a[k]} f`, "+ a[k]=1\n+ y=1\n+ f\n+ echo 'f y=1'\nf y=1\n"},
		{
			"the subscript steps once", `set -x; b[i++]=Q y=$i:${b[1]} f`,
			"+ b[1]=Q\n+ y=2:Q\n+ f\n+ echo 'f y=2:Q'\nf y=2:Q\n",
		},
		{"a program", `set -x; b[1]=Q y=${b[1]} /usr/bin/printenv y`, "+ /usr/bin/printenv y\n+ b[1]=Q\n+ y=Q\nQ\n"},
		{"a regular builtin", `set -x; b[1]=Q y=${b[1]} echo hi`, "+ echo hi\n+ b[1]=Q\n+ y=Q\nhi\n"},
		{
			"the empty key", `set -x; a[""]=1 y=${a[""]} f`,
			"ksh: line 2: warning: adding empty subscript\n+ a['']=1\n+ y=1\n+ f\n+ echo 'f y=1'\nf y=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), elementPrefixSetup+"\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
