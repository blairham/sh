// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"errors"
	"os"
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

// What a shell hands over when it replaces its own process, which is a second
// question over the same parameter and splits the panel the other way.
//
// Two columns take this shell back out of the count before the execve, so the
// program that stands in its place reads the number this shell held rather
// than one more; two leave the environment alone. Neither answer follows from
// the counting policy — one column with a ceiling and one without are on each
// side of it — which is why it is a field beside ShellLevel rather than a
// value of it (#3118).
//
// The environment handed to the replacement is what the case reads, because
// that is the whole of what a script can see: the far side is another process
// and it is that entry, plus its own reading rules, that settles the number.
func replacedEnv(t *testing.T, sem Semantics, env []string, src string) []string {
	t.Helper()
	var handed []string
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Env: env, Stdout: out, Stderr: out,
		ReplaceProcess: func(_ string, _, env []string, _ []*os.File) error {
			handed = env
			return errors.New("measured rather than replaced")
		},
	})
	runCd(t, r, src)
	return handed
}

func handedLevel(t *testing.T, handed []string) (string, bool) {
	t.Helper()
	for _, entry := range handed {
		if name, value, found := strings.Cut(entry, "="); found && name == ShellLevelName {
			return value, true
		}
	}
	return "", false
}

func TestWhatAReplacedShellIsHandedForItsDepth(t *testing.T) {
	for _, c := range []struct {
		name      string
		policy    ShellLevelPolicy
		exec      ShellLevelExec
		inherited string
		want      string
	}{
		// The count is kept: the replacement is one deeper, exactly as a
		// command this shell merely started would be.
		{"counted", ShellLevelCounted, ShellLevelExecCounted, "", "1"},
		{"counted, from a depth", ShellLevelCounted, ShellLevelExecCounted, "5", "6"},
		// The count is not: this shell takes itself out, and the far side
		// puts it back.
		{"not counted", ShellLevelCounted, ShellLevelExecNotCounted, "", "0"},
		{"not counted, from a depth", ShellLevelCounted, ShellLevelExecNotCounted, "5", "5"},
		// An unanswered field keeps the count, which is the reading that
		// changes nothing for a vector that has not chosen.
		{"unanswered", ShellLevelCounted, ShellLevelExecUnspecified, "", "1"},
		// The floor belongs to the counting policy and applies on this side
		// too: a shell holding 0 hands over 0 where there is a floor and -1
		// where there is none, and that is the one input the two
		// not-counting columns disagree on.
		{"a floor under the subtraction", ShellLevelCountedToACeiling, ShellLevelExecNotCounted, "-1", "0"},
		{"and none where there is none", ShellLevelCounted, ShellLevelExecNotCounted, "-1", "-1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.ShellLevel, sem.ShellLevelExec = c.policy, c.exec
			handed := replacedEnv(t, sem, []string{"SHLVL=" + c.inherited}, `exec /bin/echo x`)
			got, found := handedLevel(t, handed)
			if !found {
				t.Fatalf("no %s in the environment handed over: %q", ShellLevelName, handed)
			}
			if got != c.want {
				t.Errorf("handed %s=%s, want %s", ShellLevelName, got, c.want)
			}
		})
	}
}

// A shell with no depth of its own hands over nothing of its own, whichever
// way the second field is set: there is no count to take this shell out of,
// and an entry a script made and exported is the script's.
func TestAShellThatCountsNoDepthRewritesNothing(t *testing.T) {
	for _, exec := range []ShellLevelExec{ShellLevelExecCounted, ShellLevelExecNotCounted} {
		t.Run(exec.String(), func(t *testing.T) {
			sem := PosixSemantics()
			sem.ShellLevel, sem.ShellLevelExec = ShellLevelNotCounted, exec
			handed := replacedEnv(t, sem, nil, `export SHLVL=7; exec /bin/echo x`)
			if got, found := handedLevel(t, handed); !found || got != "7" {
				t.Errorf("handed %s=%q found=%v, want 7", ShellLevelName, got, found)
			}
		})
	}
}

// And `exec` in a subshell is not a replacement at all, here or on the panel,
// so the count crosses untouched however the field is set.
//
// It cannot reach the hook — a subshell is a cloned Runner and an execve in one
// would take the parent shell with it — so this is the guard that says the
// rewrite is on the replacement road rather than on `exec`.
func TestExecInASubshellHandsOverNoLoweredDepth(t *testing.T) {
	sem := PosixSemantics()
	sem.ShellLevel, sem.ShellLevelExec = ShellLevelCounted, ShellLevelExecNotCounted
	handed := replacedEnv(t, sem, []string{"SHLVL=5"}, `( exec /bin/echo x ); echo after`)
	if handed != nil {
		t.Errorf("a subshell reached the replacement hook: %q", handed)
	}

	// And the child it runs instead is handed the depth this shell holds,
	// which is the half the hook cannot see: the rewrite is on the
	// replacement road and not on the builtin, so the ordinary command keeps
	// the count.
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Env: []string{"SHLVL=5"}, Stdout: out, Stderr: out,
	})
	runCd(t, r, `( exec /usr/bin/env ); echo after`)
	if !strings.Contains(out.String(), ShellLevelName+"=6\n") {
		t.Errorf("the child of a subshell should be handed the depth this shell holds, got %q", out.String())
	}
}
