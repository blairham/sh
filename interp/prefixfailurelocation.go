// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// Who speaks for a value in an assignment prefix that will not expand.
//
// One column in the panel has two location shapes and chooses between them by
// which command is speaking, and a failed prefix value is a place it chooses.
// Measured 2026-09-26 over a script file under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME, against `/bin/ksh` `Version AJM 93u+ 2012-08-01` —
// `go version -m` says *not a Go executable* — with `a=$((1/0)) <command>` on
// line 2 of the file, beside what the same line does to `a` when the value is
// an ordinary `9`:
//
//	command             the prefix persists   the location
//	echo RAN            no                    S[2]:
//	true                no                    S[2]:
//	read v </dev/null   no                    S[2]:
//	print R             no                    S[2]:
//	command echo R      no                    S[2]:
//	whence echo         no                    S[2]:
//	let 1               no                    S[2]:
//	test -n x           no                    S[2]:
//	ulimit -n           no                    S[2]:
//	:                   yes                   S: line 2:
//	eval :              yes                   S: line 2:
//	typeset q=1         yes                   S: line 2:
//	shift 0             yes                   S: line 2:
//	export e=1          yes                   S: line 2:
//	alias zz=1          yes                   S: line 2:
//	a function          yes                   S: line 2:
//	/bin/echo R         no                    S: line 2:
//	nosuchcmd_zz        no                    S: line 2:
//
// Eighteen rows and one rule: the builtin speaks exactly where the prefix is
// **the builtin's environment** rather than a store this shell keeps. The two
// halves are both needed and each has a row that says so — the last two do not
// persist and are not builtins, and the function persists and is not one
// either.
//
// That the two columns line up is the reading rather than a coincidence. A
// prefix that persists is an ordinary assignment to this shell, and this shell
// locates its own assignments the shell's way: `a=$((1/0))` on its own line is
// `S: line 2:` in the same build, which is the control that keeps this off
// "an arithmetic failure past line 1" — a rule that shape would also have
// moved `echo $((1/0))`, which does not move (#4683).
//
// Line 1 hides all of it, because that column writes neither the bracket nor
// the word `line` for the first line of a file. So the rows above are on line
// 2 deliberately, and `a=$((1/0)) echo RAN` alone is `S:` in both shapes.

// prefixValueSpeaker is who a failed prefix value is located as, while one is
// being expanded.
type prefixValueSpeaker uint8

const (
	// prefixValueSpeakerNone is no prefix value being expanded, which is
	// every diagnostic but the ones this file is about.
	prefixValueSpeakerNone prefixValueSpeaker = iota
	// prefixValueSpeakerBuiltin is the builtin the prefix stands in front
	// of, which is the environment the prefix makes.
	prefixValueSpeakerBuiltin
	// prefixValueSpeakerShell is this shell, which is what a prefix it keeps
	// is an assignment to.
	prefixValueSpeakerShell
)

// prefixFailureSpeaker decides, once per command, who a value in its prefix
// that will not expand is located as.
//
// Answered here rather than at the expansion because the *command* is what
// decides it, and the four routes a prefixed command takes reach the expansion
// from four different places. None in the dialects that do not ask — every
// column but one writes a single location shape, and one of those names the
// builtin in it, so making a builtin speak there would write `S:echo:2:` where
// that shell writes `S:2:`.
func (r *Runner) prefixFailureSpeaker(argv []string) prefixValueSpeaker {
	if !r.diag().PrefixFailureIsTheBuiltins || len(argv) == 0 {
		return prefixValueSpeakerNone
	}
	if _, ok := r.lookupBuiltin(argv[0]); !ok {
		// A function, an external, or a word that names nothing: not a
		// builtin, so there is no builtin to speak.
		return prefixValueSpeakerShell
	}
	kind := r.prefixCommandOf(argv)
	if r.prefixPersistsAtThisBuiltin(argv[0], kind) {
		// A store this shell keeps, which is an ordinary assignment and is
		// located as one.
		return prefixValueSpeakerShell
	}
	return prefixValueSpeakerBuiltin
}

// speakForAPrefixValue arms this command's answer for the length of one
// prefix value's expansion, and hands back what puts it away.
//
// Scoped to the expansion rather than to the command, because a builtin that
// runs commands of its own must not lend them a speaker — and because the
// only diagnostic this is about is one the expansion itself raises.
func (r *Runner) speakForAPrefixValue() func() {
	if r.prefixSpeaker == prefixValueSpeakerNone {
		return func() {}
	}
	outer := r.prefixValueSpeaker
	r.prefixValueSpeaker = r.prefixSpeaker
	return func() { r.prefixValueSpeaker = outer }
}

// prefixValueIsTheBuiltins answers builtinIsSpeaking while a prefix value is
// expanding, and reports whether it answered at all.
func (r *Runner) prefixValueIsTheBuiltins() (bool, bool) {
	switch r.prefixValueSpeaker {
	case prefixValueSpeakerBuiltin:
		return true, true
	case prefixValueSpeakerShell:
		return false, true
	}
	return false, false
}

// aPrefixValueCanFail reports whether any of these assignments has a
// right-hand side that could raise a diagnostic at all, which is what decides
// whether the question above is worth asking for this command.
func aPrefixValueCanFail(assigns []*syntax.Assign) bool {
	for _, a := range assigns {
		if !a.Operand && wordCanWrite(a.Value) {
			return true
		}
	}
	return false
}
