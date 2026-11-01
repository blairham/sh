// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Every row in this file is a script run from -c in a directory of its own on
// 2026-10-31 against bash 5.3.20, ksh93u+ 2012-08-01 and BusyBox ash 1.37.0 in
// the pinned alpine image, and the wanted output is what that shell printed.
// ash printed what bash printed on every row, so its rows are bash's.

// countedReadRow is one of those scripts, with what bash and ash printed and
// what ksh93 printed.
type countedReadRow struct{ name, src, bash, ksh string }

func (tc countedReadRow) run(t *testing.T) {
	for _, d := range []string{"bash", "ash", "ksh"} {
		want := tc.bash
		if d == "ksh" {
			want = tc.ksh
		}
		t.Run(d+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			out, st, err := presets[d].Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, tc.src)
			if err != nil || st != 0 || out != want {
				t.Errorf("got %q (status %d, err %v), want %q", out, st, err, want)
			}
		})
	}
}

// TestACountedReadOfAPipeReadsWhatTheShellsRead: a `read -n` of a pipe or a
// fifo at a number `exec` put there answers 1 in ksh93, with the name empty
// and the line left in the pipe, where bash and BusyBox ash read the count
// (#7179). The read's own redirection of the same pipe reads it in all three.
// See interp.Semantics.ReadCountOfAPipeExecOpenedFindsItsEnd.
func TestACountedReadOfAPipeReadsWhatTheShellsRead(t *testing.T) {
	for _, tc := range []countedReadRow{
		{
			"exec's copy of a pipe",
			`printf 'abcdef\nxyz\n' | { exec 3<&0; read -r -n 4 -u3 l; echo "st=$? l=[$l]"; read -r -u3 m; echo "st=$? m=[$m]"; }`,
			"st=0 l=[abcd]\nst=0 m=[ef]\n",
			"st=1 l=[]\nst=0 m=[]\n",
		},
		{
			"the line stays in the pipe",
			`printf 'abcdef\nxyz\n' | { exec 3<&0; read -r -n 4 -u3 l; read -r m <&3; echo "st=$? m=[$m]"; }`,
			"st=0 m=[ef]\n",
			"st=0 m=[abcdef]\n",
		},
		{
			"a fifo exec opened",
			`mkfifo p; { printf 'abcdef\nxyz\n' >p; } & exec 3<p; read -r -n 4 -u3 l; echo "st=$? l=[$l]"; read -r -u3 m; echo "st=$? m=[$m]"; wait`,
			"st=0 l=[abcd]\nst=0 m=[ef]\n",
			"st=1 l=[]\nst=0 m=[]\n",
		},
		{
			"the read's own redirection",
			`printf 'abcdef\n' | { read -r -n 4 -u3 l 3<&0; echo "st=$? l=[$l]"; }`,
			"st=0 l=[abcd]\n",
			"st=0 l=[abcd]\n",
		},
	} {
		tc.run(t)
	}
}

// TestACountedReadAtTheEndSticksWhereTheShellsDo: a `read -n` that finds the
// end before a character leaves the number stuck in ksh93, so every later
// read of it answers 0 with its names empty, a line written since or not;
// bash and BusyBox ash read on (#7179). Opening the number again reads in all
// three. See interp.Semantics.ReadCountAtTheEndSticksTheDescriptor.
func TestACountedReadAtTheEndSticksWhereTheShellsDo(t *testing.T) {
	for _, tc := range []countedReadRow{
		{
			"standard input",
			`: | { read -r -n 4 l; echo "st=$?"; read -r m; echo "st=$? m=[$m]"; }`,
			"st=1\nst=1 m=[]\n",
			"st=1\nst=0 m=[]\n",
		},
		{
			"a line written since",
			`: >f; exec 3<f; read -r -n 4 -u3 l; echo "st=$?"; echo abc >>f; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=1\nst=0 m=[abc]\n",
			"st=1\nst=0 m=[]\n",
		},
		{
			"two names",
			`: >f; exec 3<f; read -r -n 4 -u3 l; echo 'a b' >>f; x=X y=Y; read -r -u3 x y; echo "st=$? [$x][$y]"`,
			"st=0 [a][b]\n",
			"st=0 [][]\n",
		},
		{
			"the number opened again",
			`: >f; exec 3<f; read -r -n 4 -u3 l; echo abc >>f; exec 3<f; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[abc]\n",
			"st=0 m=[abc]\n",
		},
	} {
		tc.run(t)
	}
}
