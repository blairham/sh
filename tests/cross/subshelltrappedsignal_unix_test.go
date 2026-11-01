// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `( … )` or `$( … )` that sends the shell a signal the shell has trapped,
// read through each preset: interp.Semantics.SubshellEndsWhenItSendsTheShellATrappedSignal
// (#7168).
//
// ksh93 runs both in the shell's own process, so the signal lands on the
// subshell: it ends there at 256 + N, every unforked subshell around it with
// it, and the handler runs in the shell afterwards. bash, zsh and dash run
// the subshell in a child and it runs on. Measured 2026-11-01 under `-c`, five
// runs each, ksh93u+ 2012-08-01, bash 5.3.20, zsh 5.9.2 and dash, `trap 'echo
// T' USR1` in front of the rows that do not set their own traps:
//
//	(kill -USR2 $$; echo in1; kill -HUP $$; …)  ksh93 USR2, after 287; bash in1 … HUP USR1 USR2, after 0
//	(kill -USR1 $$; echo in)                    ksh93 T, after 286; bash, zsh, dash in, T, after 0
//	{ kill -USR1 $$; echo in; }                 T, in, after 0 in all four
//	x=$(kill -USR1 $$; echo in)                 ksh93 T, after 286, x empty
//	( (kill -USR1 $$; echo in1); echo in2 )     ksh93 T, after 286
//	( x=$(kill -USR1 $$; echo in); echo in2 )   ksh93 T, after 286
//
// The rows after them are recorded and not modeled — docs/spec/semantics.md,
// "And a signal the shell has trapped" — and what is pinned is this shell's
// own answer:
//
//	(exit 5); (kill …), handler `echo T $?`      ksh93 T 5, after 286
//	(trap 'echo S' USR1; kill -USR1 $$; …)       ksh93 Trace/BPT trap, after 261
//	(kill -USR1 $$; echo in) | /bin/cat          ksh93 T, in, after 0
func TestASubshellEndsOnATrappedSignalToTheShellInOneDialect(t *testing.T) {
	const trap = `trap 'echo T' USR1; `
	killed := strconv.Itoa(256 + int(syscall.SIGUSR1))
	issue := `for s in HUP USR1 USR2; do trap "echo $s" $s; done; ` +
		`(kill -USR2 $$; echo in1; kill -HUP $$; echo in2; kill -USR1 $$; echo in3); echo "after $?"`
	simple := trap + `(kill -USR1 $$; echo in); echo "after $?"`
	for _, c := range []struct{ dialect, src, want string }{
		{"ksh", issue, "USR2 after " + strconv.Itoa(256+int(syscall.SIGUSR2))},
		{"bash", issue, "in1 in2 in3 HUP USR1 USR2 after 0"},
		{"ksh", simple, "T after " + killed},
		{"bash", simple, "in T after 0"},
		{"zsh", simple, "in T after 0"},
		{"dash", simple, "in T after 0"},
		// The pid held fixed, the subshell taken away.
		{"ksh", trap + `{ kill -USR1 $$; echo in; }; echo "after $?"`, "T in after 0"},
		{"ksh", trap + `x=$(kill -USR1 $$; echo in); echo "after $? x=$x"`, "T after " + killed + " x="},
		// Every unforked subshell around it ends too.
		{"ksh", trap + `( (kill -USR1 $$; echo in1); echo in2 ); echo "after $?"`, "T after " + killed},
		{"ksh", trap + `( x=$(kill -USR1 $$; echo in); echo in2 ); echo "after $?"`, "T after " + killed},
		// Recorded, not modeled: this shell's answers.
		{
			"ksh", `trap 'echo T $?' USR1; (exit 5); (kill -USR1 $$; echo in); echo "after $?"`,
			"T " + killed + " after " + killed,
		},
		{"ksh", trap + `(trap 'echo S' USR1; kill -USR1 $$; echo in); echo "after $?"`, "T after " + killed},
		{"ksh", trap + `(kill -USR1 $$; echo in) | /bin/cat; echo "after $?"`, "in T after 0"},
	} {
		out, st, err := presets[c.dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatalf("%s: %s: %v", c.dialect, c.src, err)
		}
		if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
			t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", c.dialect, c.src, got, st, c.want)
		}
	}
}

// And the subshell's own EXIT trap, which a subshell ended where its signal
// landed does not run, nor any unforked subshell around it (#7153): the
// process that would run one is the shell. Measured 2026-11-01 under `-c`,
// each shell in a process group of its own, ksh93u+ 2012-08-01, bash 5.3.20,
// zsh 5.9.2 and dash, `trap 'echo bye' EXIT` in front of each row:
//
//	(trap 'echo sub' EXIT; kill -TERM $$; echo s2)  ksh93 bye; bash s2, sub, bye; zsh, dash s2, sub
//	x=$(trap 'echo sub >&2' EXIT; kill -TERM $$; …) ksh93 bye
//	: | (trap 'echo sub' EXIT; kill -TERM $$; …)    ksh93 bye
//	( (trap … EXIT; kill -TERM $$); echo s2 )       ksh93 bye
//	(trap 'echo sub' EXIT; kill -SEGV $$; echo s2)  ksh93 nothing
//	the first row with USR1 trapped                 ksh93 T, after 286, bye
//
// each killed by the signal but the last. Here ksh wrote `sub` before `bye`.
func TestASubshellEndedWhereItsSignalLandedRunsNoExitTrap(t *testing.T) {
	const bye = `trap 'echo bye' EXIT; `
	term := strconv.Itoa(128 + int(syscall.SIGTERM))
	sub := `(trap 'echo sub' EXIT; kill -TERM $$; echo s2); echo after`
	for _, c := range []struct{ dialect, src, want, status string }{
		{"ksh", bye + sub, "bye", term},
		{"bash", bye + sub, "s2 sub bye", term},
		{"zsh", bye + sub, "s2 sub", term},
		{"dash", bye + sub, "s2 sub", term},
		{"ksh", bye + `x=$(trap 'echo sub >&2' EXIT; kill -TERM $$; echo s2 >&2); echo after`, "bye", term},
		{"ksh", bye + `: | (trap 'echo sub' EXIT; kill -TERM $$; echo s2); echo after`, "bye", term},
		{
			"ksh", bye + `(trap 'echo outer' EXIT; (trap 'echo in' EXIT; kill -TERM $$; echo s3); echo s2); echo after`,
			"bye", term,
		},
		{
			"ksh", bye + `(trap 'echo sub' EXIT; kill -SEGV $$; echo s2); echo after`,
			"", strconv.Itoa(128 + int(syscall.SIGSEGV)),
		},
		{
			"ksh", `trap 'echo T' USR1; ` + bye + `(trap 'echo sub' EXIT; kill -USR1 $$; echo s2); echo "after $?"`,
			"T after " + strconv.Itoa(256+int(syscall.SIGUSR1)) + " bye", "0",
		},
	} {
		out, st, err := presets[c.dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatalf("%s: %s: %v", c.dialect, c.src, err)
		}
		if got := strings.Join(strings.Fields(out), " "); got != c.want || strconv.Itoa(st) != c.status {
			t.Errorf("%s: %s\n got %q at %d\nwant %q at %s", c.dialect, c.src, got, st, c.want, c.status)
		}
	}
}
