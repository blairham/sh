// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A handler written as a `TRAP<signal>` function that returns a status other
// than zero, with a `return` written in it, interrupts the shell: nothing
// after the command it arrived during runs. The `TRAPZERR` half of that
// convention is Runner.forcedByATrapFunction; this is the signal half, which
// has a shape of its own (#5359).
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin`, each row a
// `-c` string and the same text as a script file, `T` standing for
// `TRAPUSR1() { return 3 }`:
//
//	                                          -c                 file
//	T; kill -USR1 $$; print no                0                  1
//	T; f() { kill -USR1 $$; print f }; f      0                  1
//	T; for/while around the kill              0                  1
//	trap 'print X' EXIT; T; kill -USR1 $$     0, no X            X, 1
//	T; ( kill -USR1 $$; print sub )           sub, 3, no X       sub, X, 3
//	T; /bin/kill -USR1 $$                     3                  3
//	T; x=$(kill -USR1 $$; print sub)          1                  1
//	T; eval "kill -USR1 \$\$; print e"        1                  1
//	T; { kill -USR1 $$; print in } always { print always $? }
//	                                          always 0, 1        always 0, 1
//	T; . ./f   (f: kill -USR1 $$; print in)   after 126, 0       after 126, 0
//	TRAPUSR1() { print t; (exit 3) }          t, and the program carries on
//	trap 'return 3' USR1; f() { … }; f        the action's `return` returns
//	                                          from f, and nothing is
//	                                          interrupted
//
// So the status left behind is the interrupted builtin's own where the shell
// signaled itself — `kill` returns 0 after the handler — the handler's where
// the arrival came from a child or a program, and 1 from a substitution. An
// `eval` fails and passes the interruption on, an always half runs and fails,
// and a `.` gives up its file and returns 126 to a script that carries on. At
// the end, a script file runs its EXIT trap and never exits 0; a command
// string runs none.

// trapOrigin is where a self-raised arrival came from.
type trapOrigin uint8

const (
	// originElsewhere is a child, a program, or anything not recorded.
	originElsewhere trapOrigin = iota
	// originBuiltin is `kill` in the shell itself.
	originBuiltin
	// originSubstitution is `kill` in a command substitution.
	originSubstitution
)

// selfSignalOrigin is where a signal this runner is raising on the shell
// comes from.
func (r *Runner) selfSignalOrigin() trapOrigin {
	switch {
	case !r.inSubshell:
		return originBuiltin
	case r.inCommandSubst:
		return originSubstitution
	}
	return originElsewhere
}

// interruptByATrapFunction is the shell interrupted by a TRAP function's
// non-zero return: it unwinds as `exit` does, with the status the arrival's
// origin leaves.
func (r *Runner) interruptByATrapFunction(status int, origin trapOrigin) {
	switch origin {
	case originBuiltin:
		// The builtin's own status stands: it returned after the handler.
	case originSubstitution:
		r.status = 1
	default:
		r.status = status
	}
	r.trapInterrupt = true
	r.stopTheShell()
}

// interruptedAtTheEnd settles a trap function's interruption as the shell
// ends: a script file never exits 0 from one, and a command string runs no
// EXIT trap. It reports whether the EXIT trap is to be skipped.
func (r *Runner) interruptedAtTheEnd() (skipExitTrap bool) {
	if !r.trapInterrupt {
		return false
	}
	if r.Route == RouteCommandString {
		return true
	}
	if r.status == 0 {
		r.status = 1
	}
	return false
}
