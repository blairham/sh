// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `BASH_COMMAND`: the command the shell is running, written back out.
//
// It is the parameter a DEBUG action reads to find out *which* command it
// fired for, and without it an action can only do something unconditional: a
// breakpoint is `[[ $BASH_COMMAND == … ]]` and there is no other way to write
// one, since `set -x` prints the *expanded* command and this is the
// unexpanded one (#2779).
//
// The core records the command at the places it fires the DEBUG trap and
// nowhere else — measured, see interp.RunningCommand for the probes. What is
// here is the rendering, which is this shell's own deparse and not the source
// text. Measured on bash 5.3.15, 2026-09-14, `env -i PATH=/usr/bin:/bin`,
// with the action reading the parameter *quoted* so that the shell running
// the action does not split the answer:
//
//	written                          reads
//	echo    one                      echo one
//	x=1 echo $x "a  b"               x=1 echo $x "a  b"
//	echo  hi   >   /dev/null         echo hi > /dev/null
//	echo <<<hi                       echo <<< hi
//	[[ 1 ==  1 ]]                    [[ 1 == 1 ]]
//	((1+1))                          ((1+1))
//	(( 1+1 ))                        (( 1+1 ))
//	for  w  in  a   b                for w in a b
//	for w                            for w in "$@"
//	select w                         select w in "$@"
//	case    x    in                  case x in
//	for ((i=0;i<1;i++))              ((i=0)) ((i<1)) ((i++))
//	for ((;;))                       ((1)) ((1))
//
// So the words are re-emitted from the tree with their quoting and their
// expansions as they were written, and the whitespace between them is not the
// script's — which is what [syntax.PrintCommand] already promises, and the
// three lines above where it is quoted verbatim are the nodes that keep their
// own text. Two of the rows are this shell filling something in that the
// script left out: a loop with no `in` reads as though it had written `"$@"`,
// and an arithmetic part the script left empty reads as `1`.
//
// Two departures are recorded rather than reproduced, both of them stray
// whitespace this parser does not keep. A `case` head reads with a trailing
// space after its `in` — `case x in ` — which is a deparser printing a head
// with no patterns after it. And an arithmetic part keeps whatever space
// stood between it and its `;`, so `for (( i = 0 ; ...` reads `((i = 0 ))`;
// [syntax.ForArithClause] holds the part trimmed at both ends, and carrying a
// second untrimmed copy of every part to reproduce a space is not a trade
// worth making.
func registerRunningCommand(r *interp.Runner) {
	r.SetDynamic("BASH_COMMAND", func(rr *interp.Runner) string {
		return runningCommandText(rr.RunningCommand())
	})
	// An assignment goes nowhere, and that is what bash does with one rather
	// than a refusal: measured, `BASH_COMMAND=zzz; echo "[$BASH_COMMAND]"`
	// writes the `echo` back, because the next command records over it before
	// anything reads it. Without a writer here the assignment would land in
	// the stored table, which answers ahead of a producer, and the parameter
	// would stop tracking the shell from the moment a script touched it.
	r.SetDynamicWriter("BASH_COMMAND", func(*interp.Runner, string) {})
	// And it lists with no attribute, like `LINENO` and unlike `RANDOM`:
	// measured, `declare -p BASH_COMMAND` is
	// `declare -- BASH_COMMAND="declare -p BASH_COMMAND"`.
	r.SetDynamicDeclaration("BASH_COMMAND", interp.ProducedDeclaration{})
}

// runningCommandText renders the command this shell is running the way this
// shell prints one back.
func runningCommandText(rc interp.RunningCommand) string {
	if rc.Cmd == nil {
		return ""
	}
	if loop, ok := rc.Cmd.(*syntax.ForArithClause); ok && rc.Part != interp.WholeCommand {
		return arithPartText(loop, rc.Part)
	}
	switch c := rc.Cmd.(type) {
	case *syntax.ForClause:
		return "for " + strings.Join(c.Names, " ") + wordList(c.Items, c.HasItems)
	case *syntax.SelectClause:
		return "select " + c.Name + wordList(c.Items, c.HasItems)
	case *syntax.CaseClause:
		// The trailing space is this shell's, and it is a head with no arms
		// printed after it rather than a separator — see the note above.
		return "case " + syntax.PrintWord(c.Word) + " in "
	}
	return syntax.PrintCommand(rc.Cmd)
}

// wordList is the ` in …` a loop head reads with, including the `"$@"` this
// shell fills in for a head that named no list.
func wordList(items []*syntax.Word, has bool) string {
	if !has {
		return ` in "$@"`
	}
	var b strings.Builder
	b.WriteString(" in")
	for _, w := range items {
		b.WriteString(" ")
		b.WriteString(syntax.PrintWord(w))
	}
	return b.String()
}

// arithPartText is one of an arithmetic loop's three expressions, which reads
// as though the script had written it as an arithmetic command of its own.
func arithPartText(c *syntax.ForArithClause, part interp.CommandPart) string {
	var text string
	switch part {
	case interp.ArithInit:
		text = c.InitText
	case interp.ArithCond:
		text = c.CondText
	case interp.ArithPost:
		text = c.PostText
	}
	if text == "" {
		// A part the script left out reads as the value it stands for: a
		// missing condition is true, and this shell prints the other two the
		// same way rather than printing an empty pair of parentheses.
		text = "1"
	}
	return "((" + text + "))"
}
