// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A handler spelled as a `TRAP…` function is scoped the way the trap it
// stands for is: an EXIT handler always, and a signal's or a
// pseudo-condition's under `localtraps`. What comes back at the return is the
// function as well as the trap, because the two spellings are one slot.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5147).
func TestATrapFunctionIsScopedWithItsTrap(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a nested TRAPEXIT goes back to the caller's",
			"fn1() { TRAPEXIT() { print EXIT1 }; fn2() { TRAPEXIT() { print EXIT2 } }; fn2; functions TRAPEXIT }; fn1; functions TRAPEXIT; print st=$?",
			"EXIT2\nTRAPEXIT () {\n\tprint EXIT1\n}\nEXIT1\nst=1\n",
		},
		{
			"an EXIT trap set by a call takes the caller's function away only for the call",
			"f() { TRAPEXIT() { print E1 }; g() { trap 'print T2' EXIT }; g; print mid }; f",
			"T2\nmid\nE1\n",
		},
		{
			"a TRAPEXIT a call defines leaves the caller's action trap",
			"f() { trap 'print T1' EXIT; g() { TRAPEXIT() { print E2 } }; g; trap }; f",
			"E2\ntrap -- 'print T1' EXIT\nT1\n",
		},
		{
			"a TRAP function goes with the trap at the return",
			"fn1() { setopt localtraps; TRAPINT() { print INT1; }; }; fn1; trap; functions TRAPINT; print st=$?",
			"st=1\n",
		},
		{
			"a TRAP function gives the caller's action back",
			"trap 'print O' INT; fn1() { setopt localtraps; TRAPINT() { print INT1; }; trap; }; fn1; trap; functions TRAPINT; print st=$?",
			"TRAPINT () {\n\tprint INT1\n}\ntrap -- 'print O' INT\nst=1\n",
		},
		{
			"an action gives the caller's TRAP function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; trap 'print T' INT; trap; }; fn1; trap",
			"trap -- 'print T' INT\nTRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a redefinition gives the caller's body back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; TRAPINT() { print N; }; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"unfunction gives the caller's function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; unfunction TRAPINT; trap; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a one-word reset gives the caller's function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; trap INT; trap; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a reset gives the caller's function back",
			"TRAPINT() { print O; }; fn1() { setopt localtraps; trap - INT; trap; }; fn1; trap",
			"TRAPINT () {\n\tprint O\n}\n",
		},
		{
			"a pseudo-condition's function comes back too",
			"TRAPZERR() { print Z; }; fn1() { setopt localtraps; TRAPZERR() { print N; }; false; }; fn1; false",
			"N\nZ\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// Each row waits on a handshake rather than on a timing: the body's external
// command creates `ready` and then waits for `go`, and the shell sends the
// signal only once `ready` exists and creates `go` only after sending it. So
// the signal always arrives while the external command is running, however
// loaded the machine is. A 0.2 s sleep standing in for "the body is ready"
// and a 0.6 s one for "it is still running" lost that race under load
// (#5473). The external command writes `ready` itself, so a handler cannot
// run between a line that announces readiness and the command after it.
// Re-measured that way 2026-10-02 on zsh 5.9.2 (`-f -c`): every row writes
// what it wrote before.
//
// A signal aimed at a background job whose body is this shell's reaches the
// body's own traps, as it reaches a forked shell's, and not the programs the
// body has started: the `sleep` goes on to the end and the handler runs
// after it. See interp/bodyinbox.go.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5147).
func TestASignalToABackgroundBodyReachesItsTraps(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a trapped signal runs the body's handler after its command",
			"{ trap 'print T' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done'; print after $?; } & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM $!; : > go; wait",
			"T\nafter 0\n",
		},
		{
			"on the body's last command",
			"{ trap 'print T' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done'; } & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM $!; : > go; wait; print st=$?",
			"T\nst=0\n",
		},
		{
			"where the parentheses are the job",
			"( trap 'print T' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done'; print after ) & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM $!; : > go; wait",
			"T\nafter\n",
		},
		{
			"named by a job spec, in a function",
			"f() { trap 'print T; return 1' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done'; print no }; f & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM %1; : > go; wait",
			"T\n",
		},
		{
			"an ignored signal reaches nothing",
			"{ trap '' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done'; print after $?; } & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM $!; : > go; wait",
			"after 0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// A subshell inside the body is a fork of its own, which a signal aimed at
// the job does not reach: its trap is not the job's. Measured 2026-10-02 on
// zsh 5.9.2, `{ ( trap 'print S' TERM; sleep 0.6 ); print after $? } & sleep
// 0.2; kill -TERM $!; wait; print end` writes only `end` — the job's body
// dies of the signal untrapped, which is a difference of this shell's that
// is not asserted here; that S is not written is.
func TestASignalToABackgroundBodyMissesASubshellInIt(t *testing.T) {
	const src = "{ ( trap 'print S' TERM; /bin/sh -c ': > ready; until [ -e go ]; do sleep 0.01; done' ); print after $?; } & until [[ -e ready ]]; do /bin/sleep 0.01; done; kill -TERM $!; : > go; wait; print end"
	out, _ := runZshOnPath(t, t.TempDir(), src)
	if strings.Contains(out, "S") || !strings.HasSuffix(out, "end\n") {
		t.Errorf("%s\n got %q, want no S and a last line of end", src, out)
	}
}

// A function defined through text rather than written out — an autoloaded
// stub, `functions[name]=…` — is the handler its name says, as a written-out
// one is, and the handler the EXIT trap is firing is not hidden from itself
// the way a call the script makes of it is. Measured 2026-10-02 on zsh
// 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc` with a file `TRAPEXIT` holding
// `print R` and a file `TRAPUSR1` holding `print U` on `$fpath` (#5147).
func TestATrapFunctionDefinedFromText(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an autoloaded TRAPEXIT fires once", "fpath=(. $fpath); autoload TRAPEXIT; print a; exit", "a\nR\n"},
		{"and once after another call", "fpath=(. $fpath); autoload TRAPEXIT; fn() { print F }; fn; print a", "F\na\nR\n"},
		{"an autoloaded signal handler", "fpath=(. $fpath); autoload TRAPUSR1; kill -USR1 $$; print a; functions TRAPUSR1", "U\na\nTRAPUSR1 () {\n\tprint U\n}\n"},
		{"a definition through the table", "functions[TRAPUSR1]='print hi'; kill -USR1 $$; print a", "hi\na\n"},
		{"the firing handler can see itself", "TRAPEXIT() { functions TRAPEXIT >/dev/null; print f=$? }; TRAPEXIT; print b", "f=1\nb\nf=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range map[string]string{"TRAPEXIT": "print R\n", "TRAPUSR1": "print U\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, _ := runZshOnPath(t, dir, c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
