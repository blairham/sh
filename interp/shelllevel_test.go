// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// levelRunner is a shell told one policy and handed one `SHLVL`.
func levelRunner(t *testing.T, policy ShellLevelPolicy, env []string, out *strings.Builder) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.ShellLevel = policy
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Env: env, Stdout: out, Stderr: out,
	})
}

// How an inherited value becomes a depth.
//
// The reading is bash's, taken for every dialect that counts — the entire
// value, blanks allowed on either end, an optional sign and decimal digits and
// nothing else, and anything else is no depth at all. See ShellLevelPolicy for
// the three other readings on the panel and why none of them is modeled: each
// is a C library call's residue rather than a rule, and they part company only
// over values no shell's own startup writes.
//
// The rows that are *not* about rubbish are the ones that matter, and they are
// unanimous across bash, bash 3.2, zsh, ksh93 and BusyBox ash: an absent, empty
// or plainly non-numeric value starts the count at 1, and a number adds one.
func TestHowAnInheritedShellLevelIsRead(t *testing.T) {
	for _, c := range []struct {
		inherited, want string
	}{
		{"", "1"},
		{"0", "1"},
		{"3", "4"},
		{"007", "8"},
		{"+4", "5"},
		{" 7 ", "8"},
		{"abc", "1"},
		{"2x", "1"},
		{"0x10", "1"},
		{"1000000000000000000000", "1"},
	} {
		t.Run("["+c.inherited+"]", func(t *testing.T) {
			out := &strings.Builder{}
			r := levelRunner(t, ShellLevelCounted, []string{"SHLVL=" + c.inherited}, out)
			runCd(t, r, `echo "[$SHLVL]"`)
			if got := strings.TrimSpace(out.String()); got != "["+c.want+"]" {
				t.Errorf("SHLVL read back as %s, want [%s]", got, c.want)
			}
		})
	}
	// And the name being absent from the environment is not the same input as
	// the empty value, though both start the count at 1: one of them is a
	// shell nobody started from a shell.
	out := &strings.Builder{}
	r := levelRunner(t, ShellLevelCounted, nil, out)
	runCd(t, r, `echo "[$SHLVL]"`)
	if got := strings.TrimSpace(out.String()); got != "[1]" {
		t.Errorf("with nothing inherited, SHLVL is %s, want [1]", got)
	}
}

// A shell with no answer counts nothing, and does not touch what it was handed.
//
// The zero vector belongs to a library embedder and to a test, and neither
// should find a parameter invented for them — the same rule
// Semantics.LoginStartupFiles states for the file a login shell reads.
func TestAnUnansweredShellLevelInventsNothing(t *testing.T) {
	for _, policy := range []ShellLevelPolicy{ShellLevelUnspecified, ShellLevelNotCounted} {
		t.Run(policy.String(), func(t *testing.T) {
			out := &strings.Builder{}
			r := levelRunner(t, policy, []string{"SHLVL=3"}, out)
			runCd(t, r, `echo "[${SHLVL-NONE}]"`)
			if got := strings.TrimSpace(out.String()); got != "[3]" {
				t.Errorf("SHLVL read back as %s, want the inherited [3] untouched", got)
			}
			out.Reset()
			r = levelRunner(t, policy, nil, out)
			runCd(t, r, `echo "[${SHLVL-NONE}]"`)
			if got := strings.TrimSpace(out.String()); got != "[NONE]" {
				t.Errorf("SHLVL read back as %s, want [NONE]", got)
			}
		})
	}
}

// The count is the process's, so it is settled once however many chunks run.
//
// A Runner runs more than one: a dialect's prelude and then the script, and a
// line at a time for a person typing. A count that were re-decided per chunk
// would climb a level for every command entered at a prompt, which is the
// shape the OLDPWD fixup beside it had to be written around for the same
// reason — see settleInheritedOldpwd.
func TestTheShellLevelIsSettledOncePerSession(t *testing.T) {
	out := &strings.Builder{}
	r := levelRunner(t, ShellLevelCounted, []string{"SHLVL=2"}, out)
	for range 4 {
		runCd(t, r, `echo "[$SHLVL]"`)
	}
	if got := strings.TrimSpace(out.String()); got != "[3]\n[3]\n[3]\n[3]" {
		t.Errorf("four chunks read\n%s\nwant [3] four times", got)
	}
}

// A value the script wrote is the script's, and the environment does not
// reach past it.
func TestAnAssignedShellLevelIsNotOverwritten(t *testing.T) {
	out := &strings.Builder{}
	sem := PosixSemantics()
	sem.ShellLevel = ShellLevelCounted
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Env: []string{"SHLVL=2"}, Vars: map[string]string{"SHLVL": "40"},
		Stdout: out, Stderr: out,
	})
	runCd(t, r, `echo "[$SHLVL]"`)
	if got := strings.TrimSpace(out.String()); got != "[40]" {
		t.Errorf("SHLVL read back as %s, want the caller's [40]", got)
	}
}

// The ceiling, and that it is one policy's alone.
//
// Measured 2026-09-16: bash 5.3.20 takes an inherited 998 to 999 quietly and
// an inherited 999 to `warning: shell level (1000) too high, resetting to 1`,
// at status 0 and on standard error; zsh 5.9.2, ksh93u+ and BusyBox ash write
// 1000 and say nothing. The refusal names the level the shell *would* have
// had, not the one it settles for.
func TestTheCeilingRefusesADepthAndSaysSo(t *testing.T) {
	for _, c := range []struct {
		inherited, capped, uncapped, warned string
	}{
		{inherited: "998", capped: "999", uncapped: "999"},
		{inherited: "999", capped: "1", uncapped: "1000", warned: "1000"},
		{inherited: "1000", capped: "1", uncapped: "1001", warned: "1001"},
		{inherited: "-1", capped: "0", uncapped: "0"},
		{inherited: "-5", capped: "0", uncapped: "-4"},
	} {
		t.Run("["+c.inherited+"]", func(t *testing.T) {
			for _, policy := range []ShellLevelPolicy{ShellLevelCounted, ShellLevelCountedToACeiling} {
				out := &strings.Builder{}
				r := levelRunner(t, policy, []string{"SHLVL=" + c.inherited}, out)
				runCd(t, r, `echo "[$SHLVL]"`)
				want := "[" + c.uncapped + "]"
				if policy == ShellLevelCountedToACeiling {
					want = "[" + c.capped + "]"
					if c.warned != "" {
						want = "sh: warning: shell level (" + c.warned +
							") too high, resetting to 1\n" + want
					}
				}
				if got := strings.TrimSpace(out.String()); got != want {
					t.Errorf("%v wrote %q, want %q", policy, got, want)
				}
			}
		})
	}
}
