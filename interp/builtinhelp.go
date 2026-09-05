// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// `--help` given to a builtin.
//
// It matters far more than its shape suggests, because on one platform
// fifteen commands in /usr/bin are the same three-word stub around `builtin`,
// so `/usr/bin/alias --help` is a shell builtin call and is the commonest way
// a person or a script pokes at one. Every disagreement the real-script run
// sweep found was this.
//
// It is a *diagnostic* question rather than a semantic one: whether the option
// exists at all differs by shell, and where it exists the answer is text the
// shell prints. So it is answered by Diagnostics.BuiltinHelp, and a dialect
// with no entry for a name refuses `--help` as the option nobody has, which
// is what it already did.

// helpOption is the word, exactly. An abbreviation is not it and neither is
// `--help=x`: measured, both are refused as bad options rather than answered.
const helpOption = "--help"

// builtinHelpAnswer answers `--help` for a builtin, reporting whether there
// was an answer to give.
//
// The answer goes to standard output. That is the whole reason this is not
// routed through the refusal path it sits next to: a script that runs
// `alias --help` and keeps the output gets the text, and one that keeps only
// standard error gets nothing.
func (r *Runner) builtinHelpAnswer(name string) (int, bool) {
	help := r.diag().BuiltinHelp[name]
	if help == "" {
		return 0, false
	}
	r.printf("%s\n", help)
	return orDefault(r.diag().BuiltinHelpStatus, 2), true
}

// callBuiltin runs one, answering `--help` written as its first word first.
//
// The first word rather than anywhere, because that is all this can know:
// `--help` standing where an option stands is the parser's question, and the
// parser is the only thing that knows a letter took the next word as its
// argument. builtinOptionsArg answers it there for every builtin that reads
// its options through it; this covers the rest — `exit`, `shift`, `local`,
// `kill` and the other builtins with no option region at all — and covers the
// shape every caller in the wild writes.
//
// It is also the one place all three dispatch routes meet, which is why the
// fold of a failed write moved in here with it: the outer dispatch, `command`
// and `builtin` all did the same three lines, and a fourth route would have
// been a fourth copy to forget.
func (r *Runner) callBuiltin(ctx context.Context, name string, fn Builtin, args []string) int {
	if len(args) > 0 && args[0] == helpOption {
		if status, ok := r.builtinHelpAnswer(name); ok {
			return status
		}
	}
	r.writeFailed = nil
	return r.builtinWriteStatus(name, fn(r, ctx, args))
}
