// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A subscripted assignment prefix is read in turn — the subscript's words,
// the value, the subscript's arithmetic — and its trace line follows, with the
// subscript as it resolved and quoted as a word, and with the warning a store
// under a table's empty key makes in front of it. Measured 2026-10-28 on
// ksh93u+ 2012-08-01 (`/bin/ksh`) from script files with `typeset -A a;
// b=(x y z); i=1` and `set -x` first, where this shell wrote every line from
// the source text ahead of the stores and the warning after the command
// (#6771). See interp/prefixsubscriptinturn.go.
func TestAPrefixSubscriptIsTracedAsItResolved(t *testing.T) {
	const warn = "ksh: line 2: warning: adding empty subscript\n"
	for _, c := range []struct{ name, src, want string }{
		// The four rows of the issue.
		{"a special builtin", `a[""]=1 :`, warn + "+ a['']=1\n+ :\n"},
		{
			"three entries", `a["p q"]=1 a[""]=2 a[$((1+1))]=3 :`,
			"+ a['p q']=1\n" + warn + "+ a['']=2\n+ a[2]=3\n+ :\n",
		},
		{
			"a program, whose lines follow its own", `a["p q"]=1 a[""]=2 /usr/bin/true`,
			"+ /usr/bin/true\n+ a['p q']=1\n" + warn + "+ a['']=2\n",
		},
		{"a function", `a[""]=1 f`, warn + "+ a['']=1\n+ f\n+ :\n"},
		// The subscript as it resolved, read once.
		{"an expression", `b[1+1]=Q :`, "+ b[2]=Q\n+ :\n"},
		{"a negative position", `b[-1]=Q :`, "+ b[2]=Q\n+ :\n"},
		{"a key with a blank", `k='x y'; a[$k]=1 :`, "+ k='x y'\n+ a['x y']=1\n+ :\n"},
		{"a name that is not a table yet", `c[k]=1 :`, "+ c[0]=1\n+ :\n"},
		{
			"the arithmetic after the value", `b[i++]=$((i*10)) :; echo $i ${b[@]}`,
			"+ b[1]=10\n+ :\n+ echo 2 x 10 z\n2 x 10 z\n",
		},
		{"a regular builtin", `b[i++]=Q echo hi; echo $i`, "+ echo hi\n+ b[1]=Q\nhi\n+ echo 2\n2\n"},
		// A program's prefix is read in its child, so nothing it steps is
		// this shell's.
		{"a program", `b[i++]=Q /usr/bin/true; echo $i`, "+ /usr/bin/true\n+ b[1]=Q\n+ echo 1\n1\n"},
		{"a program's value", `y=$((i++)) /usr/bin/true; echo $i`, "+ /usr/bin/true\n+ y=1\n+ echo 1\n1\n"},
		// The subscript's words, then the value, then the line.
		{
			"substitutions on both sides", `a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) :`,
			"+ echo s\n+ 1>& 2\ns\n+ echo k\n+ echo t\n+ 1>& 2\nt\n+ echo v\n+ a[k]=v\n+ :\n",
		},
		{
			"between scalar entries", `x=1 a[""]=2 y=$(echo s >&2) a[k]=3 :`,
			"+ x=1\n" + warn + "+ a['']=2\n+ echo s\n+ 1>& 2\ns\n+ y=''\n+ a[k]=3\n+ :\n",
		},
		// The store's own warning is owed to the one written ahead of the
		// line, so there is one; and a program's store is its child's, so
		// the table keeps what it held.
		{"one warning, and the store made", `a[""]=1 :; typeset -p a`, warn + "+ a['']=1\n+ :\n+ typeset -p a\ntypeset -A a=(['']=1)\n"},
		{"a program's store is not this shell's", `a[""]=1 /usr/bin/true; typeset -p a`, "+ /usr/bin/true\n" + warn + "+ a['']=1\n+ typeset -p a\ntypeset -A a=()\n"},
		// A subscript that will not evaluate writes no line, and costs what a
		// value that will not expand costs.
		{"a failed subscript ends the script", "b[1/0]=v :\necho after", "ksh: line 2: 1/0: divide by zero\n"},
		{"and a builtin's command alone", "b[1/0]=v echo hi; echo after $?", "+ echo hi\nksh[2]: 1/0: divide by zero\n+ echo after 1\nafter 1\n"},
		{"and a program's command alone", "b[1/0]=v /usr/bin/true; echo after $?", "+ /usr/bin/true\nksh: line 2: 1/0: divide by zero\n+ echo after 1\nafter 1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), "typeset -A a; b=(x y z); i=1; f() { :; }; set -x\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The same reading without `set -x`, which is what keeps the trace from
// changing the answer: a persisting prefix is read ahead of the entries behind
// it and ahead of the command's redirections, a builtin's by the store in the
// same order, and a program's in its child. Measured 2026-10-28 on ksh93u+
// 2012-08-01 from a script file; the program rows are zsh 5.9.2's answer as
// well.
func TestAPrefixSubscriptIsReadInTurnUntraced(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an entry behind sees the step", `b[i++]=Q y=$i f; echo $i`, "y=2\n2\n"},
		{"ahead of a failed redirection", "b[i++]=1 f >/nope/x; echo $i", "ksh: line 2: /nope/x: cannot create [No such file or directory]\n2\n"},
		{"once", `b[i++]=Q :; echo $i ${b[@]}`, "2 x Q z\n"},
		// A regular builtin's prefix is read by the store, in the same order
		// and located as a value of it is.
		{"a builtin's words before its value", `a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) echo hi`, "s\nt\nhi\n"},
		{"a builtin's failed subscript", `b[1/0]=v echo hi; echo after $?`, "ksh[2]: 1/0: divide by zero\nafter 1\n"},
		{"a program's child", `b[i++]=v /usr/bin/true; echo $i`, "1\n"},
		{"a program's failed subscript", `b[1/0]=v /usr/bin/true; echo after $?`, "ksh: line 2: 1/0: divide by zero\nafter 1\n"},
		{"a program's key", `a[$((i++))]=v /usr/bin/true; echo $i`, "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "typeset -A a; b=(x y z); i=1; f() { echo \"y=$y\"; }\n" + c.src
			out, _ := runKsh(t, t.TempDir(), src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
			// And traced, the same answer under the lines. A complaint
			// written once the trace prompt is out — a redirection's, here —
			// is kept without the prompt.
			traced, _ := runKsh(t, t.TempDir(), strings.Replace(src, "\n", "; set -x\n", 1))
			var kept strings.Builder
			for line := range strings.Lines(traced) {
				if rest, prompted := strings.CutPrefix(line, "+ "); prompted {
					if !strings.HasPrefix(rest, "ksh: ") {
						continue
					}
					line = rest
				}
				if !strings.Contains(line, "warning:") {
					kept.WriteString(line)
				}
			}
			if kept.String() != c.want {
				t.Errorf("traced, got %q under the lines, want %q", kept.String(), c.want)
			}
		})
	}
}
