// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `print -z` pushes onto the editor buffer stack and `read -z` takes off it.
//
// `read -z` was `-z is not implemented yet` at 2 and `print -z` was a no-op
// whose operands went nowhere — right about the *writing* and wrong about the
// stack, which is a script-visible thing with or without an editor. The pair
// round-trips in a plain `zsh -f` script, which is what makes the letter
// gradable at all (#4966).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin TERM=dumb`
// and a scratch `HOME`, one shell per row.
//
// **The issue this closes measured only the empty stack**, where the answer is
// status 1 whatever the implementation does. A grid of those rows alone would
// have passed against a `read -z` that did nothing at all, which is why the
// producer is in almost every row below.
func TestTheEditorBufferStack(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The round trip, and the row the empty-stack measurement could
			// not reach.
			"a pushed entry is what the next read takes",
			`print -z "buffered line"
			 read -z l
			 print -r -- "st=$? l=[$l]"`,
			"st=0 l=[buffered line]\n",
		},
		{
			// Last in, first out, which is what says this is a stack rather
			// than a slot.
			"the newest entry comes off first",
			`print -z one
			 print -z two
			 read -z a; print -r -- "1=[$a] st=$?"
			 read -z a; print -r -- "2=[$a] st=$?"
			 read -z a; print -r -- "3=[$a] st=$?"`,
			"1=[two] st=0\n2=[one] st=0\n3=[] st=1\n",
		},
		{
			// An empty stack is a read that did not happen: the name is
			// cleared and **present**, at 1, which is a plain `read`'s
			// answer at end of input.
			"an empty stack clears the name and reports 1",
			`l=keep
			 read -z l
			 print -r -- "st=$? l=[$l] p=${+l}"`,
			"st=1 l=[] p=1\n",
		},
		{
			// And the stream is not touched, which is the whole point of the
			// letter: the `x` is still there for the next reader.
			"the shell's input is left alone",
			`printf 'x\ny\n' | { read -z l; read m; print -r -- "st=$? l=[$l] m=[$m]"; }`,
			"st=0 l=[] m=[x]\n",
		},
		{
			// The entry is split the way an ordinary record is: IFS, the
			// last name taking the remainder.
			"the entry is split like any record",
			`print -z "a b c"
			 read -z l m
			 print -r -- "st=$? l=[$l] m=[$m]"`,
			"st=0 l=[a] m=[b c]\n",
		},
		{
			"and IFS decides where",
			`IFS=:
			 print -z "a:b:c"
			 read -z l m
			 print -r -- "l=[$l] m=[$m]"`,
			"l=[a] m=[b:c]\n",
		},
		{
			// A name with no field of its own still gets the empty string,
			// and is present.
			"a name past the fields is cleared and present",
			`print -z "a b"
			 read -z l m n
			 print -r -- "st=$? n=[$n] p=${+l}${+m}${+n}"`,
			"st=0 n=[] p=111\n",
		},
		{
			// The escapes are the reading `-r` decides, exactly as on a
			// record off the stream. `print -rz` is what puts a literal
			// backslash on the stack to be read back.
			"the escapes are processed without -r",
			`print -rz 'a\tb'
			 read -z l
			 print -r -- "l=[$l]"`,
			"l=[atb]\n",
		},
		{
			"and kept with it",
			`print -rz 'a\tb'
			 read -rz l
			 print -r -- "l=[$l]"`,
			`l=[a\tb]` + "\n",
		},
		{
			// An array target fills from the entry and still answers 1,
			// which is measured and is not the empty-stack answer in
			// disguise — the row below has two entries and answers 1 twice.
			"an array target fills and reports 1",
			`print -z "a b c"
			 read -z -A arr
			 print -r -- "st=$? n=${#arr} arr=[${(j:|:)arr}]"`,
			"st=1 n=3 arr=[a|b|c]\n",
		},
		{
			"and reports 1 with entries still on the stack",
			`print -z "one two"
			 print -z "three four"
			 read -z -A arr; print -r -- "st=$? arr=[${(j:|:)arr}]"
			 read -z -A br;  print -r -- "st=$? br=[${(j:|:)br}]"`,
			"st=1 arr=[three|four]\nst=1 br=[one|two]\n",
		},
		{
			// The operands become **one** entry, joined with a space, and
			// the letters that change the writing do not change the join.
			"the operands are one entry joined with a space",
			`print -z one two
			 read -z l
			 print -r -- "l=[$l]"`,
			"l=[one two]\n",
		},
		{
			"which -l and -N do not change",
			`print -lz one two
			 read -z l; print -r -- "l=[$l]"
			 print -Nz a b
			 read -z m; print -r -- "m=[$m]"`,
			"l=[one two]\nm=[a b]\n",
		},
		{
			// A push that joins to nothing pushes **nothing**, which is the
			// row that separates it from pushing an empty entry: an empty
			// entry would have read back at 0.
			"a push with nothing in it pushes no entry",
			`print -z
			 print -z ""
			 read -z l
			 print -r -- "st=$? l=[$l]"`,
			"st=1 l=[]\n",
		},
		{
			// A subshell's stack is its own, which is what keeping it in an
			// array under a private name buys — the same property
			// `zmodload`'s and `zstyle`'s stores have.
			"a subshell's pushes do not reach the parent",
			`( print -z inner; read -z l; print -r -- "inner=[$l]" )
			 read -z o
			 print -r -- "outer st=$? o=[$o]"`,
			"inner=[inner]\nouter st=1 o=[]\n",
		},
		{
			// And nothing is written, which is the half of `print -z` that
			// was already right.
			"the push writes nothing",
			`print -z "x"
			 print -r -- "after"`,
			"after\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
