// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A subscript that evaluates and lands out of range costs the command list
// and not the shell, from a command string as from a file: the `-c` give-up
// is an arithmetic failure's alone. Through a builtin's operand it costs the
// builtin and the line goes on. POSIX mode ends the shell for the statements
// and not for the builtins, since none of them is special.
//
// Measured 2026-10-16 against `/opt/homebrew/bin/bash`, GNU bash 5.3.20, each
// program a `-c` string, a script file, and a script file under `set -o
// posix`, byte for byte (#6835). Every row here ended the `-c` string before.
func TestAnOutOfRangeSubscriptCostsTheCommandList(t *testing.T) {
	for _, c := range []struct {
		probe, script, posix string
	}{
		{
			"a[-5]=x; echo same",
			"bash: line 2: a[-5]: bad array subscript\nnext=1\n",
			"bash: line 3: a[-5]: bad array subscript\n",
		},
		{
			"a[-5]+=x; echo same",
			"bash: line 2: a[-5]: bad array subscript\nnext=1\n",
			"bash: line 3: a[-5]: bad array subscript\n",
		},
		{
			"a=([-1]=x); echo same",
			"bash: line 2: [-1]=x: bad array subscript\nnext=1\n",
			"bash: line 3: [-1]=x: bad array subscript\n",
		},
		{
			"a[1]=(p q); echo same",
			"bash: line 2: a[1]: cannot assign list to array member\nnext=1\n",
			"bash: line 3: a[1]: cannot assign list to array member\n",
		},
		{
			"declare -A m; m=([k]=v x); echo same",
			"bash: line 2: m: x: must use subscript when assigning associative array\nnext=1\n",
			"bash: line 3: m: x: must use subscript when assigning associative array\n",
		},
		{
			`read "a[-5]" <<< x; echo same $?`,
			"bash: line 2: a[-5]: bad array subscript\nsame 1\nnext=0\n",
			"bash: line 3: a[-5]: bad array subscript\nsame 1\nnext=0\n",
		},
		{
			`printf -v "a[-5]" x; echo same $?`,
			"bash: line 2: a[-5]: bad array subscript\nsame 1\nnext=0\n",
			"bash: line 3: a[-5]: bad array subscript\nsame 1\nnext=0\n",
		},
		{
			"declare a[-5]=x; echo same $?",
			"bash: line 2: a[-5]: bad array subscript\nsame 1\nnext=0\n",
			"bash: line 3: a[-5]: bad array subscript\nsame 1\nnext=0\n",
		},
	} {
		src := "a=(1)\n" + c.probe + "\necho next=$?\n"
		for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile} {
			if got, st := posixRouteStatus(t, src, route); got != c.script || st != 0 {
				t.Errorf("%q, route %v:\n got %q at %d\nwant %q at 0", c.probe, route, got, st, c.script)
			}
		}
		if got, _ := posixRouteStatus(t, "set -o posix\n"+src, interp.RouteScriptFile); got != c.posix {
			t.Errorf("%q in POSIX mode:\n got %q\nwant %q", c.probe, got, c.posix)
		}
	}
}

// An array literal element whose subscript expanded to nothing is a bad
// subscript, worded as an out-of-range one is and costing what it costs —
// where the plain `a[$(true)]=x` stores element zero, and blanks are the
// expression zero. A declaration's operand fails at 1, and a table's empty
// key there is quoted as the operand's trace line writes it (#6738).
//
// Measured 2026-10-16 against GNU bash 5.3.20 as one `-c` string, byte for
// byte.
func TestAnEmptySubscriptInAnArrayLiteralIsBad(t *testing.T) {
	for _, c := range []struct{ probe, want string }{
		{"a=([$(true)]=x); echo same", "bash: line 1: []=x: bad array subscript\nst=1\n"},
		{`a=([""]=$(echo v)); echo same`, "bash: line 1: []=v: bad array subscript\nst=1\n"},
		{"e=; a=(p [$e]=x); echo same", "bash: line 1: []=x: bad array subscript\nst=1\n"},
		{"declare -a q=([$(true)]=x); echo same", "bash: line 1: []=x: bad array subscript\nst=1\n"},
		{`declare -A m; declare m=([""]=x); echo same`, "bash: line 1: ['']='x': bad array subscript\nst=1\n"},
		{`declare -A t=([k]=v [""]=$(echo y)); echo same`, "bash: line 1: ['']='y': bad array subscript\nst=1\n"},
		{`declare -A m; m=([$(true)]=x); echo same`, "bash: line 1: [$(true)]=x: bad array subscript\nst=1\n"},
		{"declare -a q=(1 [-5]=x); echo same", "bash: line 1: [-5]=x: bad array subscript\nst=1\n"},
		{`a=([$(echo " ")]=x); echo "${a[0]}"`, "x\nst=0\n"},
		{"a[$(true)]=x; echo \"${a[0]}\"", "x\nst=0\n"},
	} {
		got, _ := posixRouteStatus(t, c.probe+"\necho st=$?\n", interp.RouteCommandString)
		if got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.probe, got, c.want)
		}
	}
}
