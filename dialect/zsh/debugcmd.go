// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `ZSH_DEBUG_CMD`: the command the shell is about to run, written back out.
//
// It is the parameter a DEBUG action reads to find out *which* command it
// fired for. Without it an action can only do something unconditional, since
// `set -x` prints the *expanded* command and this is the unexpanded one.
//
// The core already records the command at the places it fires the trap — see
// interp.RunningCommand — so what is here is only the name and the
// rendering.
//
// **The rendering is this shell's own function layout, not a one-line
// deparse.** That is measured rather than assumed, and it is the thing that
// makes this parameter different from bash's `BASH_COMMAND`: a compound
// command reads back over several lines, indented with tabs, exactly as
// `functions` would say it. Measured 2026-09-28 against
// /opt/homebrew/bin/zsh — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m`: *not a Go executable* — with the action reading the parameter through
// `print -r --` so nothing is re-split:
//
//	written                    reads
//	echo    one                echo one
//	x=1 echo $x "a  b"         x=1 echo $x "a  b"
//	echo  hi   >   /dev/null   echo hi > /dev/null
//	[[ 1 ==  1 ]]              [[ 1 == 1 ]]
//	((1+1))                    ((1+1))
//	(( 1+1 ))                  (( 1+1 ))
//	f() { :; }                 f () {⏎→:⏎}
//	{ : ; }                    {⏎→:⏎}
//	( : )                      (⏎→:⏎)
//	for  w  in  a   b; do :; done   for w in a b⏎do⏎→:⏎done
//	case    x    in x) :;; esac     case x in⏎→(x) : ;;⏎esac
//
// So the words are re-emitted from the tree with their quoting as written and
// the whitespace between them normalized, and a body is laid out the way
// FunctionLayout says — which is why this calls that rather than carrying a
// second description of the same shell's habits.
//
// It fires for the command that **removes** the trap as well, which is what
// says the parameter is written before each command rather than after the one
// that has just run: `trap - DEBUG` is the last value an action ever reads.
//
// **It is an ordinary parameter that a firing writes, not a producer that
// answers every read**, and two rows say so rather than one. Before any DEBUG
// trap has fired it is **empty** even though commands have run — `print -r --
// "[$ZSH_DEBUG_CMD]"` on the first line of a script writes `[]`, not itself —
// and an assignment to it **sticks**: `ZSH_DEBUG_CMD=zzz` then reading it back
// writes `zzz`. A dynamic producer, which is how bash's `BASH_COMMAND` is
// modeled here, answers on every read and can do neither. That is why this
// hangs off the firing rather than off the name.
func registerDebugCommand(r *interp.Runner) {
	r.SetBeforeDebugTrap(func(rr *interp.Runner) {
		rc := rr.RunningCommand()
		if rc.Cmd == nil {
			return
		}
		rr.SetVar("ZSH_DEBUG_CMD", syntax.PrintWith(rc.Cmd, debugCommandLayout()))
	})
}

// debugCommandLayout is FunctionLayout with the brace wrapping taken off.
//
// The same arrangement and not a second description of it — one field
// cleared, because the two readers differ in exactly one thing. A function
// *listing* is a body and this shell always spells a body with braces, so
// `functions f` writes `f () {⏎→:⏎}` even for a body declared without them.
// The command a DEBUG trap fired for is not a body: `echo one` reads back as
// `echo one`, and wrapping it would make every simple command in the
// parameter read as a brace group that the script never wrote.
func debugCommandLayout() syntax.Layout {
	l := FunctionLayout()
	l.BodyIsAlwaysBraced = false
	return l
}
