// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The two parameters this shell runs a command that is only redirections
// through, and the two option names that take that command off them.
//
// `NULLCMD` is the command; `READNULLCMD` replaces it where the one
// redirection is a plain `<file`, which is what makes `<f` at a prompt page
// the file rather than copy it. Their defaults — `cat` and `more` — are
// assignments in the prelude rather than values here, because a script reads
// them (`print -r -- $NULLCMD` answers `cat`), reassigns them and unsets
// them like any other scalar. Measured 2026-09-10 on zsh 5.9.2: `typeset -p`
// reports both as plain, unexported scalars.
const (
	nullCommandParameter     = "NULLCMD"
	readNullCommandParameter = "READNULLCMD"

	// csh's reading of the same line: a redirection with no command is an
	// error rather than a command. Reached by pointing both fields at a name
	// that is always empty, which is exactly what the core already does with
	// an emptied `NULLCMD` — measured, `NULLCMD=; >f` and `setopt cshnullcmd;
	// >f` are the same sentence at the same status, and neither creates the
	// file.
	cshNullCommandParameter = ".zsh.nullcmd.csh"
	// sh's reading: the command is `:`, so the redirections happen and
	// nothing else does. Measured — `setopt shnullcmd; >f` creates `f`,
	// prints nothing and succeeds, and `setopt shnullcmd; <f` prints nothing
	// even with `READNULLCMD` pointed at a marker, which is what says this
	// covers both parameters rather than only the writing one.
	shNullCommandParameter = ".zsh.nullcmd.sh"
)

// registerNullCommandParameters gives the two private names above their
// values, which never change and which no script can reach — the same
// dot-prefixed namespace `zstyle` and the recorded options already use.
//
// Dynamic rather than assigned, so that a subshell, a `local` and an `unset`
// have nothing to do to them: the option decides which name the core reads
// and the name decides what it finds.
func registerNullCommandParameters(r *interp.Runner) {
	r.SetDynamic(cshNullCommandParameter, func(*interp.Runner) string { return "" })
	r.SetDynamic(shNullCommandParameter, func(*interp.Runner) string { return ":" })
}

// nullCommandOption is `cshnullcmd` or `shnullcmd`: a name whose own bit
// lives in the recorded store and whose effect is recomputed from both bits
// together.
//
// Both, and not one each, because they are not independent: `cshnullcmd` wins
// while it is on — measured in both orders — so turning it off has to hand
// the shell back to `shnullcmd` if that one is still on, which a setter that
// looked only at its own name could not do. The same arrangement
// `checkrunningjobs` uses and for the same reason.
func nullCommandOption(base string) zshOption {
	o := storeBacked(base, false)
	store := o.set
	o.set = func(r *interp.Runner, on bool) int {
		code := store(r, on)
		applyNullCommandOptions(r)
		return code
	}
	return o
}

// applyNullCommandOptions points the two semantics fields at whichever pair of
// parameters the option states name.
func applyNullCommandOptions(r *interp.Runner) {
	null, read := nullCommandParameter, readNullCommandParameter
	switch {
	case recordedDeviates(r, "cshnullcmd"):
		null, read = cshNullCommandParameter, cshNullCommandParameter
	case recordedDeviates(r, "shnullcmd"):
		null, read = shNullCommandParameter, shNullCommandParameter
	}
	swapAxes(r, func(s *interp.Semantics) {
		s.NullCommandVariable, s.ReadNullCommandVariable = null, read
	})
}
