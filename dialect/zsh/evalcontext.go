// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// `$ZSH_EVAL_CONTEXT` and `$zsh_eval_context`: what the shell is currently
// inside, outermost first, as one `:`-joined string and as an array.
//
// A plugin manager reads it to decide whether it is being sourced or run, so
// an absent parameter did not leave a field blank — it took the wrong arm in
// silence. Both halves answered `${+NAME}` of 0 here (#4908).
//
// # The value is a stack, and the probe is what pushes onto it
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so the two columns
// are two programs — script files run `-f` under `env -i PATH=/usr/bin:/bin`
// with a scratch `HOME`:
//
//	where the read happens            $ZSH_EVAL_CONTEXT
//	top level of a script file        toplevel
//	standard input                    toplevel
//	zsh -c                            cmdarg
//	inside a function                 toplevel:shfunc
//	inside an anonymous function      toplevel:shfunc
//	inside a sourced file             toplevel:file
//	inside eval                       toplevel:eval
//	inside a trap action              toplevel:trap
//	inside $( … ) and ` … `           toplevel:cmdsubst
//	inside <( … )                     toplevel:outsubst
//	inside >( … )                     toplevel:insubst
//	inside =( … )                     toplevel:equalsubst
//	. from -c                         cmdarg:file
//	eval inside eval                  …:eval:eval
//	a function inside a sourced file  toplevel:file:shfunc
//
// # Loading a function is on the stack too, once
//
// Measured 2026-10-04 the same way, with each function in a file on `$fpath`
// and `autoload -Uz` (or `-Uk`) ahead of the calls, reading
// `$zsh_eval_context` under `zsh -c`:
//
//	first call of a -z function           cmdarg shfunc loadautofunc
//	second call of it                     cmdarg shfunc
//	a function the body calls, first call cmdarg shfunc loadautofunc shfunc
//	a file holding only the definition    cmdarg shfunc loadautofunc
//	a -k file, while it runs              cmdarg shfunc evalautofunc
//	the definition it left, first call    cmdarg shfunc loadautofunc
//
// So the word is pushed over the call's own `shfunc` for the run of the body
// the load produced, and only on the call that loaded it. It is what a
// function file reads to know the load is what is running it: zsh's own
// contributed functions end in `[[ $zsh_eval_context = *loadautofunc ]]` and
// call the function they just defined only when that holds, so without the
// word their first call defined everything and did nothing. That was #5880 —
// `bracketed-paste-magic` bound as the paste widget let the first paste of a
// session through as typed keys, newline and all.
//
// **The apparatus is in the answer**, which is why the table is written with
// its own shape beside each row. A sweep that wraps every cell in a command
// substitution — the obvious way to keep going past a name that might refuse
// — records `toplevel:cmdsubst`, a value this parameter never has at rest,
// and the first table filed against #4866 recorded exactly that. Every row
// above was read the way it says it was read, and the rows that go through a
// helper function carry that helper's `shfunc` in the measurement rather
// than having it removed by hand.
//
// # What is *not* on the stack is as much of the answer
//
// Measured in the same run: a subshell `( … )`, a brace group, a `for` body,
// an `always` block and `$(( … ))` all read exactly what the line outside
// them reads. So none of the five pushes a word, and a rule that took "a new
// shell" or "a new scope" as the trigger would have been wrong about four of
// them — a subshell is a new shell and says nothing.
//
// # Both halves are frozen, and they really are tied
//
//	${(t)ZSH_EVAL_CONTEXT}   scalar-readonly-tied-special
//	${(t)zsh_eval_context}   array-readonly-tied-special
//	ZSH_EVAL_CONTEXT=x       read-only variable: ZSH_EVAL_CONTEXT
//	unset ZSH_EVAL_CONTEXT   read-only variable: ZSH_EVAL_CONTEXT
//
// Unlike the `WATCH`/`watch` pair in #4907 this one *does* say `tied`, with
// `:` joining the scalar — so the tie is registered here even though nothing
// can ever write through it, because the word a listing prints is what the
// tie decides. It is registered before the producers are, while neither name
// holds anything, so the seeding a tie does has nothing to seed and nothing
// to store over the producer.
func registerTheEvalContext(r *interp.Runner) {
	// The tie first, and the seam that does not seed: `Tie` lays a value
	// down, and a stored one is exactly what shadows a producer — see
	// interp.Runner.TieProduced, where the half-right answer that came of
	// using the other door is written out.
	r.TieProduced(evalContextScalar, evalContextArray, ":")
	r.SetDynamicArray(evalContextArray, evalContextWords)
	r.SetDynamic(evalContextScalar, func(r *interp.Runner) string {
		return strings.Join(evalContextWords(r), ":")
	})
	// Readonly, which is both halves of what the reference refuses — an
	// assignment and an `unset` draw the same sentence there — and is also
	// what stops an assignment landing in a stored parameter that then
	// shadows the producer, which is the reason `funcstack` beside it needs
	// the same call.
	r.MarkReadonly(evalContextScalar)
	r.MarkReadonly(evalContextArray)
	// And neither writes a `typeset -p` row, which is the seam `$status`,
	// `$ARGC` and `$funcstack` are already on: measured 2026-09-27,
	// `typeset -p ZSH_EVAL_CONTEXT` and `typeset -p zsh_eval_context` both
	// write nothing at all and leave 0. The bare `typeset -T` listing is
	// the other side of that and does show them, with the live value —
	// `ZSH_EVAL_CONTEXT=cmdarg` and `zsh_eval_context=( cmdarg )`.
	r.SetDynamicDeclaration(evalContextScalar, interp.ProducedDeclaration{Silent: true})
	r.SetDynamicDeclaration(evalContextArray, interp.ProducedDeclaration{Array: true, Silent: true})
}

const (
	evalContextScalar = "ZSH_EVAL_CONTEXT"
	evalContextArray  = "zsh_eval_context"
)

// evalContextWords is the stack in this shell's words.
//
// The core keeps the stack and this names it, which is why the mapping is a
// table here and not a set of strings in interp: every word below is this
// shell's own spelling of a shape that has nothing to do with any shell's
// vocabulary.
func evalContextWords(r *interp.Runner) []string {
	stack := r.EvalContextStack()
	out := make([]string, 0, len(stack))
	for _, c := range stack {
		if w := evalContextWord[c]; w != "" {
			out = append(out, w)
		}
	}
	return out
}

// evalContextWord is the vocabulary, one word per shape.
//
// Every one of them is measured in the table at the top of this file. They
// are not guesses off the construct's name: `<( … )` is `outsubst` and
// `>( … )` is `insubst`, which is the *opposite* pairing to the one the
// spellings suggest — the word names the direction the body's data travels
// and not the redirection the word is written with.
var evalContextWord = map[interp.EvalContext]string{
	interp.EvalContextScript:                   "toplevel",
	interp.EvalContextCommandString:            "cmdarg",
	interp.EvalContextSourcedFile:              "file",
	interp.EvalContextFunctionBody:             "shfunc",
	interp.EvalContextEval:                     "eval",
	interp.EvalContextTrap:                     "trap",
	interp.EvalContextCommandSubstitution:      "cmdsubst",
	interp.EvalContextProcessSubstitutionRead:  "outsubst",
	interp.EvalContextProcessSubstitutionWrite: "insubst",
	interp.EvalContextTempFileSubstitution:     "equalsubst",
	interp.EvalContextAutoloadedBody:           "loadautofunc",
	interp.EvalContextAutoloadedFile:           "evalautofunc",
}
