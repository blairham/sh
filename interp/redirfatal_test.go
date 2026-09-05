// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runRedirFatal runs src with the axis answered both ways and nothing else
// moved, so what changes between the two runs is the axis and not the setup.
func runRedirFatal(t *testing.T, src string, fatal, statusIsOne Answer) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := permissive()
		s.RedirectErrorOnSpecialBuiltinFatal = fatal
		s.FatalErrorStatusIsOne = statusIsOne
		r.Semantics = &s
	})
}

// A redirection that cannot be made, written on a special builtin, ends a
// non-interactive shell where the axis says so — and where it does not, the
// complaint is all that happens and the next command runs.
//
// Asserted on what came *after* rather than on the status alone: a shell that
// stopped and a shell that carried on can report the same number, and the
// first version of this could not tell them apart.
func TestAFailedRedirectionOnASpecialBuiltinEndsTheScriptWhereTheAxisSaysSo(t *testing.T) {
	const src = "exec 3>/nope/x\necho after\n"

	out, st := runRedirFatal(t, src, Yes, Yes)
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have stopped at the redirection", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status this vector gives", st)
	}

	out, st = runRedirFatal(t, src, No, Yes)
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have carried on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want the status of the command that carried on", st)
	}
}

// The status of the shell that stops is the vector's fatal status and not one
// of this axis's own, which is why the axis is a single Answer. Two of the
// panel stop at 1 and one at 2, and that split is FatalErrorStatusIsOne's
// everywhere else too.
func TestTheStoppedShellTakesTheVectorsFatalStatus(t *testing.T) {
	if _, st := runRedirFatal(t, "exec 3>/nope/x\necho after\n", Yes, Yes); st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if _, st := runRedirFatal(t, "exec 3>/nope/x\necho after\n", Yes, No); st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// The reach is every special builtin and not `exec` alone. Measured across
// the panel on four of them, and they answer together: where one stops they
// all stop.
func TestEverySpecialBuiltinCarriesTheRule(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"exec", "exec 3>/nope/x"},
		{"colon", ": 3>/nope/x"},
		{"eval", "eval : 3>/nope/x"},
		{"readonly", "readonly zz=1 3>/nope/x"},
		{"dot", ". /dev/null 3>/nope/x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := runRedirFatal(t, c.src+"\necho after\n", Yes, Yes); strings.Contains(out, "after") {
				t.Errorf("got %q, want the script to have stopped", out)
			}
			if out, _ := runRedirFatal(t, c.src+"\necho after\n", No, Yes); !strings.Contains(out, "after") {
				t.Errorf("got %q, want the script to have carried on", out)
			}
		})
	}
}

// And the boundary is real. A regular builtin, an external command and a
// *compound* command's own redirection all carry on with the axis at Yes,
// which is what every shell in the panel does — so a rule written as "a
// failed redirection is fatal" would be wrong in three places at once.
func TestOnlyASpecialBuiltinsRedirectionEndsTheScript(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"regular builtin", "true 3>/nope/x"},
		{"external command", "/bin/echo x 3>/nope/x"},
		{"compound command", "{ echo x; } 3>/nope/x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runRedirFatal(t, c.src+"\necho after\n", Yes, Yes)
			if !strings.Contains(out, "after") {
				t.Errorf("got %q, want the script to have carried on", out)
			}
		})
	}
}

// Inside a subshell it ends the subshell and the parent runs on, which is
// what "ends the shell" has to mean in a runner whose subshells are clones
// rather than processes.
func TestInASubshellItEndsTheSubshellAlone(t *testing.T) {
	out, st := runRedirFatal(t, "( exec 3>/nope/x; echo inner )\necho after\n", Yes, Yes)
	if strings.Contains(out, "inner") {
		t.Errorf("got %q, want the subshell to have stopped", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the parent to have carried on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want the parent's own", st)
	}
}

// Inside a function it ends the shell, because a function is not a boundary.
func TestInAFunctionItEndsTheShell(t *testing.T) {
	out, _ := runRedirFatal(t, "f() { exec 3>/nope/x; echo inner; }\nf\necho after\n", Yes, Yes)
	if strings.Contains(out, "inner") || strings.Contains(out, "after") {
		t.Errorf("got %q, want nothing after the redirection to have run", out)
	}
}

// With no answer the shell refuses rather than guessing, which is what every
// axis does and is the reason a Runner that has chosen no dialect is not
// quietly given one shell's reading of this.
func TestWithNoAnswerTheAxisIsRefusedByName(t *testing.T) {
	out, _ := runRedirFatal(t, "exec 3>/nope/x\necho after\n", Unspecified, Yes)
	if !strings.Contains(out, "a failed redirection on a special builtin") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
}

// posix mode is where the answer lives for the two shells that have one, and
// it is the reason this is an axis rather than a property of a binary: the
// panel's two disagreeing columns are the same build of the same shell with
// this option on and off.
//
// Turning it off puts back the answer that was there rather than asserting
// the opposite of the standard's, which is the difference that matters for a
// dialect POSIX already agrees with.
func TestPosixModeMovesAnAxis(t *testing.T) {
	withPosix := func(fatal Answer) func(*Runner) {
		return func(r *Runner) {
			s := permissive()
			s.RedirectErrorOnSpecialBuiltinFatal = fatal
			r.Semantics = &s
			r.AddSetOptions("posix")
		}
	}

	out, _ := run(t, "set -o posix\nexec 3>/nope/x\necho after\n", withPosix(No))
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want posix mode to have made the failure fatal", out)
	}
	if strings.Contains(out, "not implemented") {
		t.Errorf("got %q, want the request granted rather than refused", out)
	}

	out, _ = run(t, "set -o posix\nset +o posix\nexec 3>/nope/x\necho after\n", withPosix(No))
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want leaving posix mode to put the answer back", out)
	}

	// And a shell whose own answer is already the standard's keeps it when
	// posix mode is turned off, rather than being handed the opposite.
	out, _ = run(t, "set -o posix\nset +o posix\nexec 3>/nope/x\necho after\n", withPosix(Yes))
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the answer the vector started with", out)
	}

	// `set +o posix` with the mode never turned on moves nothing at all,
	// which is what has always made it a grant rather than a promise.
	out, _ = run(t, "set +o posix\nexec 3>/nope/x\necho after\n", withPosix(Yes))
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want a request for the state we are in to move nothing", out)
	}
}

// The mode is this runner's own. A subshell that enters it leaves the parent
// where it was, which is the copy-on-write the vector is swapped with.
func TestPosixModeEnteredInASubshellStaysThere(t *testing.T) {
	out, _ := run(t, "( set -o posix; exec 3>/nope/x; echo inner )\nexec 4>/nope/y\necho after\n", func(r *Runner) {
		s := permissive()
		s.RedirectErrorOnSpecialBuiltinFatal = No
		r.Semantics = &s
		r.AddSetOptions("posix")
	})
	if strings.Contains(out, "inner") {
		t.Errorf("got %q, want the subshell's mode to have applied there", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the parent left outside the mode", out)
	}
}
