// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// stackSpecs is one spec set holding every argument form a stack letter can
// meet, an optional option argument, and a first and a rest argument.
const stackSpecs = `'-a[all]' '-f+[file]:fname:(f1)' '--opt=[o]:oo:(o1)' ` +
	`'-x-[dir]:xx:(x1)' '-n[n]:nn:(n1)' '-o=[oo]:ooo:(o1)' '-e=-[ee]:eee:(e1)' ` +
	`'-T[t]:t1:(a):t2:(b)' '-q[q]::qq:(q1)' '1:first:(a b)' '*:rest:(r1)'`

// TestCompargumentsReadsTheLineAsZshDoes is #6189: a stack whose letter
// takes its argument in the same word, a letter whose argument is the next
// word, `-O` and `-s` inside an option's own argument word, an optional
// option argument, and the bare name of an `=` option under `-s`.
//
// Every row is all five read-backs asked of one line — `-D`'s tags with
// `[IPREFIX|PREFIX]` after it, `-W`'s `$line` and `$opt_args` (keys sorted),
// `-O`'s status and `next`, and `-s` — and every expected string is the one
// zsh 5.9.2 printed for the same widget body on 2026-10-05, driven through a
// pseudo-terminal. The names `-O` and `-s` fill are set to `MARK` first, so
// "assigned nothing" shows.
func TestCompargumentsReadsTheLineAsZshDoes(t *testing.T) {
	for _, c := range []struct{ switches, line, want string }{
		{"-s", "cmd -afz", `D=0 s=(option-f-1) [-af|z] line=('') opt=(-a= -f=afz) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -afz ", `D=0 s=(argument-1) [|] line=('') opt=(-a= -f=afz) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -axq", `D=0 s=(option-x-1) [-ax|q] line=('') opt=(-a= -x=axq) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -aoz", `D=0 s=(option-o-1) [-ao|z] line=('') opt=(-a= -o=aoz) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -ao=z", `D=0 s=(option-o-1) [-ao=|z] line=('') opt=(-a= -o=ao=z) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -aez", `D=0 s=(option-e-1) [-ae|z] line=('') opt=(-a= -e=aez) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -af=z", `D=0 s=(option-f-1) [-af|=z] line=('') opt=(-a= -f=af=z) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -afaa", `D=0 s=(option-f-1) [-af|aa] line=('') opt=(-a= -f=afaa) O=0 n=(-n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -anz", `D=0 s=(argument-1) [|-anz] line=(-anz) opt=() O=0 n=(-a:all -n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -an x", `D=0 s=(option-n-1) [|x] line=('') opt=(-a= -n=x) O=2 n=(MARK) s=0 v=`},
		{"-s", "cmd -na ", `D=0 s=(option-n-1) [|] line=('') opt=(-a= -n=) O=2 n=(MARK) s=0 v=`},
		{"-s", "cmd -fx", `D=0 s=(option-f-1) [-f|x] line=('') opt=(-f=x) O=0 n=(-a:all -n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -af", `D=0 s=(option-f-1) [-af|] line=('') opt=(-a= -f=) O=0 n=(-n:n -T:t -q:q) s=0 v=`},
		{"-s", "cmd -n ", `D=0 s=(option-n-1) [|] line=('') opt=(-n=) O=2 n=(MARK) s=0 v=`},
		{"-s", "cmd -T a ", `D=0 s=(option-T-2) [|] line=('') opt=(-T=a:) O=1 n=(MARK) s=1 v=MARK`},
		{"-s", "cmd --opt ", `D=0 s=(option--opt-1) [|] line=('') opt=(--opt=) O=1 n=(MARK) s=1 v=MARK`},
		{"-s", "cmd -o", `D=0 s=(option-o-1) [-o|] line=('') opt=(-o=) O=0 n=(-a:all -n:n -T:t -q:q) s=0 v=`},
		{"-s", "cmd --opt", `D=0 s=(argument-1) [|--opt] line=(--opt) opt=() O=0 n=(-a:all -n:n -T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -q ", `D=0 s=(option-q-1 argument-1) [|] line=('') opt=(-q=) O=0 n=(-a:all -n:n -T:t) s=0 v=`},
		{"-s", "cmd -q x", `D=0 s=(option-q-1 argument-1) [|x] line=('') opt=(-q=x) O=0 n=(-a:all -n:n -T:t) s=0 v=`},
		{"-s", "cmd -q a b ", `D=0 s=(argument-rest) [|] line=(b\ ) opt=(-q=a) O=0 n=(-a:all -n:n -T:t) s=1 v=MARK`},
		{"-s", "cmd -q -z ", `D=0 s=(argument-1) [|] line=('') opt=(-q=-z) O=0 n=(-a:all -n:n -T:t) s=1 v=MARK`},
		{"-s", "cmd -n -a ", `D=0 s=(argument-1) [|] line=('') opt=(-a= -n=-a) O=0 n=(-T:t -q:q) s=1 v=MARK`},
		{"-s", "cmd -aq ", `D=0 s=(option-q-1 argument-1) [|] line=('') opt=(-a= -q=) O=0 n=(-n:n -T:t) s=0 v=`},
		{"", "cmd -n ", `D=0 s=(option-n-1) [|] line=('') opt=(-n=) O=1 n=(MARK) s=1 v=MARK`},
		{"", "cmd -o", `D=0 s=(argument-1) [|-o] line=(-o) opt=() O=0 n=(-a:all -n:n -T:t -q:q) s=1 v=MARK`},
		{"", "cmd --opt", `D=0 s=(argument-1) [|--opt] line=(--opt) opt=() O=0 n=(-a:all -n:n -T:t -q:q) s=1 v=MARK`},
		{"", "cmd -q x", `D=0 s=(option-q-1 argument-1) [|x] line=('') opt=(-q=x) O=0 n=(-a:all -n:n -T:t) s=1 v=MARK`},
	} {
		t.Run(c.switches+" "+c.line, func(t *testing.T) {
			got := reported(t, `local -a d a s n dd o e l k; local -A oa; local v=MARK; n=(MARK)
				comparguments -i '' `+c.switches+` : `+stackSpecs+`
				comparguments -D d a s; local D="D=$? s=(${s[*]}) [$IPREFIX|$PREFIX]"
				comparguments -W l oa 0; for x in ${(ko)oa}; do k+=("$x=${oa[$x]}"); done
				local W="line=(${(qj: :)l}) opt=(${(j: :)k})"
				comparguments -O n dd o e; local O="O=$? n=(${n[*]})"
				comparguments -s v; say "$D $W $O s=$? v=$v"`, c.line)
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
