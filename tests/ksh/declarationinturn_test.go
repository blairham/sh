// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A declaration utility's operands are stored one at a time here, each before
// the next is expanded, and the utility then runs over the bare names. Measured
// 2026-10-27 on ksh93u+ 2012-08-01 from a script file, against bash 5.3.20,
// zsh 5.9.2 and dash 0.5.12, which hand the utility every operand expanded
// against the state the command started in. See
// interp.Semantics.DeclarationOperandsAreStoredInTurn (#6770).
func TestDeclarationOperandsAreStoredInTurn(t *testing.T) {
	if got := ksh.Semantics().DeclarationOperandsAreStoredInTurn; got != interp.Yes {
		t.Fatalf("DeclarationOperandsAreStoredInTurn = %v, want Yes", got)
	}
	for _, c := range []struct{ name, src, want string }{
		{"a later operand sees an earlier one", "x=0\ntypeset x=1 y=$x\necho $y", "1\n"},
		{"export and readonly alike", "export a=1 b=$a; readonly c=1 d=$c; echo $b $d", "1 1\n"},
		{"the same name twice", "typeset x=1 x=$x$x; echo $x", "11\n"},
		// The letters land after the stores, so `y` reads the text and the
		// utility then evaluates both.
		{"ahead of the letters", "typeset -i x=1+1 y=$x; echo $y", "2\n"},
		{"a table's element", "typeset -A a; typeset a[k]=1 b=${a[k]}; echo $b", "1\n"},
		{"an element appended to", "typeset -A m; typeset m[k]=1 m[k]+=2 w=${m[k]}; echo $w", "12\n"},
		// An array literal operand in front of a scalar one, and behind it.
		{"a literal before a scalar", "typeset a=(1 2) x=${a[1]}; echo $x", "2\n"},
		{"a scalar before a literal", "typeset y=7 b=($y); typeset -p b", "typeset -a b=(7)\n"},
		{"an element literal before a scalar", "typeset f[1]=(p q) g=${f[1]}; echo $g", "p\n"},
		{"a keyed literal before a scalar", "typeset -A a=([k]=v) x=${a[k]}; echo $x", "v\n"},
		// Still local inside a keyword function, which is the utility's to
		// decide: the walk holds the value and the utility makes the store.
		{"local in a function", "function f { typeset x=1 y=$x; echo in=$x,$y; }; x=g; y=h; f; echo out=$x,$y", "in=1,1\nout=g,h\n"},
		{"over a frozen name the function shadows", "readonly x=1; function f { typeset x=2 y=$x; echo in=$x,$y; }; f; echo out=$x", "in=2,2\nout=1\n"},
		// A subscript is read once: `i` steps once for each operand.
		{"an evaluated subscript steps once", "i=1; typeset q[i++]=x f[i++]=(p); echo $i; typeset -p q f", "3\ntypeset -a q=([1]=x)\ntypeset -a f=([2]=(p) )\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// Each operand's trace line is written as it is stored, with a warning the
// store makes in front of its own line, and the utility's line comes last over
// the bare names. The subscript is written as it resolved and quoted as a
// word, the rule a bare element assignment follows here (#6582); a literal
// operand's is written as it resolved and unquoted (#6876). Measured
// 2026-10-15 and 2026-10-27 on ksh93u+ 2012-08-01 from a script file with
// `typeset -A a` and `set -x` first (#6770, #6683, #6876).
func TestDeclarationOperandsAreTracedAsTheyAreStored(t *testing.T) {
	const warn = "ksh: line 2: warning: adding empty subscript\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"scalars, elements and a substitution",
			`typeset x=1 a[""]=2 y=$(echo s >&2) a[k]=3`,
			"+ x=1\n" + warn + "+ a['']=2\n+ echo s\n+ 1>& 2\ns\n+ y=''\n+ a[k]=3\n+ typeset x a y a\n",
		},
		{"a keyed literal operand", `typeset -A a=([""]=1)`, warn + "+ a['']=1\n+ typeset -A a\n"},
		{"two empty keys in one literal", `typeset -A a=([""]=1 [""]=2)`, warn + "+ a['']=1\n" + warn + "+ a['']=2\n+ typeset -A a\n"},
		{"an element operand", `typeset a[""]=1`, warn + "+ a['']=1\n+ typeset a\n"},
		{"the same key twice", `typeset a[""]=1 a[""]=2`, warn + "+ a['']=1\n" + warn + "+ a['']=2\n+ typeset a a\n"},
		{"an evaluated subscript", `typeset q[1+1]=x`, "+ q[2]=x\n+ typeset q\n"},
		{"a key with a blank", `k='x y'; typeset a[$k]=v`, "+ k='x y'\n+ a['x y']=v\n+ typeset a\n"},
		{"a literal before a scalar", `typeset b=(1 2) x=${b[1]}`, "+ b=( 1 2 )\n+ x=2\n+ typeset b x\n"},
		{"a scalar before a literal", `typeset y=7 b=($y)`, "+ y=7\n+ b=( 7 )\n+ typeset y b\n"},
		{"a scalar after a literal", `typeset f[1]=(p q) f[2]=r`, "+ f[1]=( p q )\n+ f[2]=r\n+ typeset f f\n"},
		{"an element literal", `typeset f[1]=(p q)`, "+ f[1]=( p q )\n+ typeset f\n"},
		{"an element literal's evaluated subscript", `typeset f[1+1]=(p)`, "+ f[2]=( p )\n+ typeset f\n"},
		{"an element literal's key, unquoted", `k='x y'; typeset a[$k]=(p)`, "+ k='x y'\n+ a[x y]=( p )\n+ typeset a\n"},
		{"a chained element literal", `typeset g[1][2]=(p q)`, "+ g[1][2]=( p q )\n+ typeset g\n"},
		{"a substitution in the subscript, once", `typeset f[$(echo S >&2; echo 2)]=(p)`, "+ echo S\n+ 1>& 2\nS\n+ echo 2\n+ f[2]=( p )\n+ typeset f\n"},
		{"the lines before a value that will not expand", "typeset x=1 y=${q?boom} z=2\necho after", "+ x=1\nksh: line 2: q: boom\n"},
		{"the lines before an arithmetic failure", "typeset x=1 y=$((1/0)) z=2\necho after", "+ x=1\nksh: line 2: 1/0: divide by zero\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), "typeset -A a; set -x\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// A frozen name is refused where its operand is stored, after the operands
// before it have been stored and traced, and the utility never runs. Measured
// 2026-10-27 on ksh93u+ 2012-08-01: an EXIT trap reads the operand before the
// refusal.
func TestARefusedDeclarationOperandKeepsTheOnesBeforeIt(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "readonly r=0; trap 'echo x=$x y=$y' EXIT; set -x\ntypeset x=1 r=2 y=3")
	want := "+ x=1\nksh: line 2: r: is read only\n+ echo x=1 y=\nx=1 y=\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
	// The same for a literal operand, whose refusal ends the walk the same
	// way.
	out, _ = runKsh(t, t.TempDir(), "readonly r=0; set -x\ntypeset x=1 r=(1 2) y=3\necho after")
	if want := "+ x=1\nksh: line 2: r: is read only\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// An indexed operand whose subscript will not evaluate is refused in the
	// assignment's own words, and the walk ends there.
	out, _ = runKsh(t, t.TempDir(), `set -x
typeset a["p q"]=1 a[""]=3
echo after`)
	if want := "ksh: line 2: p q: arithmetic syntax error\n"; !strings.HasSuffix(out, want) || strings.Contains(out, "after") || strings.Contains(out, "+ a[") {
		t.Errorf("got %q, want it to end with %q and nothing traced", out, want)
	}
}
