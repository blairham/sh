// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package pgo holds the programs the shipped binaries' build profile is
// trained on, and nothing else.
//
// Every binary under cmd/ is built with profile-guided optimization: its
// directory carries a default.pgo, which `go build` reads without being told
// to, so `make build`, the release and a plain `go build ./cmd/zsh` all get it.
// Each of those files is a link to the one profile in this directory, so there
// is one profile and one place it is made. `make pgo` makes it, by profiling
// BenchmarkTrain over Programs.
//
// What the compiler does with the profile is inline and lay out by what is
// hot rather than by its static budget alone. It never changes what the
// program does, so the profile can be stale without being wrong: a function
// it does not mention is built as it would have been without one. An old
// profile is a slower binary, not a broken one, and regenerating it is a
// chore for when the interpreter's shape has moved, not a step of every
// change.
//
// The programs are chosen to look like what the shell spends its life doing,
// rather than any one benchmark, so that the compiler is not taught to win one
// row at the cost of the rest (#6496):
//
//   - the CI performance gate's workload and the construct table, which is
//     the simple-command path every script runs;
//   - functions with locals, string operations, `case`, patterns and arrays,
//     which is most of what an rc file and its plugins do;
//   - zsh's own readers — parameter flags, `[[ ]]` patterns, `emulate -L`,
//     tables — which a real zsh start spends its time in;
//   - a long script of definitions, which is the parser;
//   - an empty `-c`, which is each dialect's setup.
//
// Each program prints its answer, and TestEveryTrainingProgramAnswers holds
// it to that in every dialect it names: a program a dialect refused would
// train the build on the refusal.
package pgo

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/startupcost"
)

// Program is one training program.
type Program struct {
	// Name is what a failure is reported under.
	Name string
	// Text is the `-c` string.
	Text string
	// Answer is everything the program must print.
	Answer string
	// Dialects are the shells that run it: those whose language it is in.
	Dialects []string
	// Weight is how many times a training pass runs it, which is what sets
	// its share of the profile. The construct table's loops are long and a
	// start is short, so each is weighted toward the mix in the package
	// comment rather than left at whatever its text happens to cost.
	Weight int
}

var (
	everyDialect = []string{"bash", "zsh", "ksh", "dash", "ash"}
	arrayShells  = []string{"bash", "zsh", "ksh"}
)

// Programs are the training set.
func Programs() []Program {
	var ps []Program
	for _, c := range startupcost.Cases() {
		if c.Gating {
			ps = append(ps, Program{Name: "gate-" + c.Name, Text: c.Program, Answer: c.Answer, Dialects: everyDialect, Weight: 4})
		}
	}
	ps = append(ps, []Program{
		{
			Name:     "start",
			Text:     ":",
			Dialects: everyDialect,
			Weight:   50,
		},
		{
			Name:     "posix-functions",
			Text:     posixFunctions,
			Answer:   "200 x199 abc\n",
			Dialects: everyDialect,
			Weight:   4,
		},
		{
			Name:     "functions",
			Text:     functions,
			Answer:   "300 84 10 1 7\n",
			Dialects: arrayShells,
			Weight:   4,
		},
		{
			Name:     "zsh-readers",
			Text:     zshReaders,
			Answer:   "200 a,b,c,200 3 2 C\n",
			Dialects: []string{"zsh"},
			Weight:   10,
		},
		{
			Name:     "definitions",
			Text:     definitions(),
			Answer:   "defined 150\n",
			Dialects: everyDialect,
			Weight:   2,
		},
	}...)
	for _, c := range startupcost.Constructs() {
		ps = append(ps, Program{
			Name:     "construct-" + c.Name,
			Text:     c.Program,
			Answer:   c.Answer + "\n",
			Dialects: []string{"bash", "zsh"},
			Weight:   1,
		})
	}
	return ps
}

// posixFunctions is what a POSIX script does a lot of: call functions, take
// its arguments apart, test strings and build a result a piece at a time.
const posixFunctions = `pick() {
	case $1 in
	*1) r=one ;;
	*[02468]) r=even ;;
	*) r=odd ;;
	esac
}
i=0 s= last=
while [ "$i" -lt 200 ]; do
	pick "x$i"
	if [ "$r" = one ]; then s=$s.; fi
	last=x$i
	set -- a b c
	t=$1$2$3
	i=$((i + 1))
done
printf '%s %s %s\n' "$i" "$last" "$t"
`

// functions is an rc file's middle: locals, expansions, `case`, `[[ ]]` and
// arrays, in the language bash, zsh and ksh share.
const functions = `f() {
	typeset a=$1 b=$2 c
	c=$a$b
	case $c in
	*5*) r=1 ;;
	*) r=0 ;;
	esac
}
n=0 m=0 i=0
while (( i < 300 )); do
	f "a$i" "b$i"
	(( n++ ))
	[[ $r == 1 || $i == *0 ]] && (( m++ ))
	(( i++ ))
done
arr=()
for w in one two three four five six seven eight nine ten; do
	arr+=("$w")
done
k=0
for w in "${arr[@]}"; do
	x=${w//e/E} y=${w%e*} z=${#w}
	[[ $x == *E* ]] && (( k++ ))
done
echo $n $m ${#arr[@]} ${#y} $k
`

// zshReaders is what a zsh start spends its time in that the shared language
// does not reach: parameter flags, extended patterns, `emulate -L`, local
// arrays and tables.
const zshReaders = `emulate -L zsh
setopt extendedglob
typeset -A h
h=(a 1 b 2 c 3)
g() {
	emulate -L zsh
	local -a parts
	parts=(${(s:/:)1})
	REPLY=${(j:,:)parts}
}
n=0 hits=0
for i in {1..200}; do
	g a/b/c/$i
	[[ $i == (#b)(1)(*) ]] && (( hits++ ))
	x=${(U)REPLY[1]}
	(( n++ ))
done
print -r -- $n $REPLY ${#h} ${h[b]} ${(U)${(k)h[(r)3]}}
`

// definitions is a long script of definitions, so that the parser is as hot
// in the profile as it is in a real start: the program text is read once a
// run and each function in it is defined once.
func definitions() string {
	var b strings.Builder
	const n = 150
	for i := range n {
		k := strconv.Itoa(i)
		b.WriteString("fn_" + k + "() {\n\tif [ \"$1\" = x ]; then\n\t\techo \"" + k + "\" | while read -r line; do\n\t\t\tcase $line in\n\t\t\t" + k + ") : ;;\n\t\t\t*) return 1 ;;\n\t\t\tesac\n\t\tdone\n\telse\n\t\tv=${2:-default} w=$(( " + k + " * 2 ))\n\t\tfor e in a b c; do\n\t\t\t[ -n \"$e\" ] && v=$v$e\n\t\tdone\n\tfi\n}\n")
	}
	b.WriteString("echo defined " + strconv.Itoa(n) + "\n")
	return b.String()
}
