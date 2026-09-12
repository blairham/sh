// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$sysparams`, `$errnos` and `systell` — the three of `zsh/system` that are
// not builtins.
//
// Measured on zsh 5.9.2 (Homebrew, aarch64) with a scratch HOME and no startup
// files. The six builtins the module also names — `sysopen`, `sysread`,
// `syswrite`, `sysseek`, `syserror`, `zsystem` — are not implemented and do
// not hold the module shut, which is zmodload.go's rule and is asserted below
// rather than left to be inferred.

// sysParam runs src and returns everything it wrote.
func sysParam(t *testing.T, src string) string {
	t.Helper()
	out, _, errs := runZshSplit(t, t.TempDir(), src)
	return out + errs
}

// TestSysParamsReportsTheProcessThisShellIsIn is the key a real startup reads:
// counted across the installed plugin tree, twelve of the fourteen uses of
// this parameter are `pid`.
//
// Asserted against the *test process's* own numbers, which is what makes it a
// measurement rather than a shape check — a shell that answered any plausible
// number, or the same number twice, fails.
func TestSysParamsReportsTheProcessThisShellIsIn(t *testing.T) {
	got := sysParam(t, `print -r -- "[$sysparams[pid]] [$sysparams[ppid]]"`)
	want := "[" + strconv.Itoa(os.Getpid()) + "] [" + strconv.Itoa(os.Getppid()) + "]\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestProcSubstPidIsEmptyRatherThanZero is the one deliberate deviation in
// this file, and the two assertions below are the two halves of why.
//
// A process substitution here runs on a goroutine of the shell's own
// process, so there is no process to report and none ever will be. Real
// zsh answers 0, which means "none yet" there and is the answer that must
// not be copied: a plugin writes `kill -- -$sysparams[procsubstpid]`, and
// `kill -- -0` signals the shell's own process group.
//
// The key is therefore present — so the read is not fatal to the line that
// asked, which is what a refusal was — and empty, so the guard callers put
// in front of the dangerous line skips it. The second half is the one that
// matters in the wild and is asserted as a whole line below, because a
// test on the value alone passes for a shell that ends the script.
func TestProcSubstPidIsEmptyRatherThanZero(t *testing.T) {
	got := sysParam(t, `print -r -- "[$sysparams[procsubstpid]]"`)
	if want := "[]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// And the shape a caller actually writes: the guard reads it as nothing to
// signal, and the line after it still runs. Against the refusal this
// replaces, the `print` never happened; against an answer of 0, the `kill`
// branch is taken and it is the shell's own process group.
func TestTheGuardInFrontOfTheKillSkipsAnEmptyProcSubstPid(t *testing.T) {
	got := sysParam(t, `pid=$sysparams[procsubstpid]
[[ -n $pid ]] && print -r -- "WOULD KILL -$pid"
print -r -- "carried on"`)
	if want := "carried on\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestTheSetTestOnSysParamsIsAnswered: the key is one the table has, so the
// set test is 1 and `-` does not reach its default. Both differ from the
// refusal this replaced, where the key was absent to every route but the
// guard.
func TestTheSetTestOnSysParamsIsAnswered(t *testing.T) {
	got := sysParam(t, `print -r -- "${+sysparams[pid]}${+sysparams[procsubstpid]}`+
		` [${sysparams[procsubstpid]-none}]"`)
	if want := "11 []\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestErrnosIsIndexedByTheNumberTheSystemGives, which is what the parameter is
// for: turning a number a system call gave back into a name.
//
// The rows are the POSIX errnos, which hold their numbers on both platforms
// this project ships for — the ones that do not are exactly the ones the
// per-platform tables exist to keep apart, and a case naming a
// platform-specific number would fail on the other one for the right reason
// and be useless anyway.
func TestErrnosIsIndexedByTheNumberTheSystemGives(t *testing.T) {
	got := sysParam(t, `print -r -- "[$errnos[1]] [$errnos[2]] [$errnos[9]] [$errnos[13]]"`)
	if want := "[EPERM] [ENOENT] [EBADF] [EACCES]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestSystellIsTheDescriptorsPositionAndMovesWithIt.
//
// A math function and not a builtin — the module's listing writes `+f:systell`
// — so it is called from arithmetic. Reading from the descriptor moves it,
// which is the half that separates a real answer from a constant nought.
func TestSystellIsTheDescriptorsPositionAndMovesWithIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bytes")
	if err := os.WriteFile(path, []byte("abcd\nefgh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, errs := runZshSplit(t, dir,
		"exec 3< "+path+"\n"+
			`print -r -- "[$(( systell(3) ))]"`+"\n"+
			"read -u3 x\n"+
			`print -r -- "[$x] [$(( systell(3) ))]"`+"\n"+
			"read -u3 x\n"+
			`print -r -- "[$x] [$(( systell(3) ))]"`+"\n")
	// Five and ten: the line and its newline each time, which is the same
	// pair zsh gives for the same file. A shell that read the whole file into
	// a buffer would answer ten to the first question, and one that never
	// asked the kernel would answer nought to both.
	if got, want := out+errs, "[0]\n[abcd] [5]\n[efgh] [10]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestSystellReportsRatherThanRefuses: a descriptor nothing is open at is -1
// and not a complaint, measured. Every case with no position to report gives
// the same -1 — a closed number, and a stream that has no position at all.
func TestSystellReportsRatherThanRefuses(t *testing.T) {
	got := sysParam(t, "v=$(( systell(99) ))\n"+`print -r -- "[$v]"`)
	if want := "[-1]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestBothSystemParametersAreReadonlyAndDifferInKind: `typeset -Ar sysparams`
// for the association and `typeset -ar errnos` for the array, measured — which
// is what says one is a table of names and the other a list indexed by number.
func TestBothSystemParametersAreReadonlyAndDifferInKind(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a sysparams write", "sysparams[pid]=1", "zsh:1: read-only variable: sysparams\n"},
		{"an errnos write", "errnos[1]=x", "zsh:1: read-only variable: errnos\n"},
		{"the sysparams listing", "typeset -p sysparams", "typeset -Ar sysparams\n"},
		{"the errnos listing", "typeset -p errnos", "typeset -ar errnos\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := sysParam(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// TestTheSystemModuleHasAllNineOfItsFeatures is what #1749 was for: the
// module names nine features and every one of them is here.
//
// It was six of nine and loading anyway, which zmodload.go's rule allows — a
// missing builtin refuses by name at the word that runs it, so holding the
// module shut would stop a file for features it may never call. That rule is
// still right and there is nothing left here for it to forgive, which is the
// state a module is supposed to end in rather than the one it passes through.
//
// The second half is the part a listing cannot check: `zsystem` runs. A
// feature table saying `+b:zsystem` over a builtin nobody registered is
// exactly the accepting-and-inert failure this module has produced twice
// (#1737, #1668), so the case calls it and reads what came back.
func TestTheSystemModuleHasAllNineOfItsFeatures(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zmodload zsh/system && zmodload -lF zsh/system\n")
	want := "+b:syserror\n+b:sysopen\n+b:sysread\n+b:sysseek\n+b:syswrite\n" +
		"+b:zsystem\n+f:systell\n+p:errnos\n+p:sysparams\n"
	if out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q and 0", out, st, want)
	}
	out, _, errs := runZshSplit(t, t.TempDir(), "zmodload zsh/system\nzsystem supports flock\nprint -r -- \"flock=$?\"\n")
	if got, want := out+errs, "flock=0\n"; got != want {
		t.Errorf("the module's own builtin = %q, want %q", got, want)
	}
}

// TestSysParamsPidIsEmptyInsideABodyARealShellWouldHaveForked is #2046: the
// key that tells a body which process it is, in a shell where a body has not
// got one.
//
// Real zsh answers a different number in each of these, because each is a
// fork. This shell runs all five on goroutines of one process, so the honest
// answer is that four of them have no process to name — see subshellPid for
// why the plausible answer is the one that kills the shell.
//
// **With no placeholder program supplied**, which is what runZshSplit builds
// and what a Runner embedded in another program is. A shell binary gives a
// process substitution's body a process group of its own, and that row answers
// the group's id instead — see systemgroup_unix_test.go, which measures the
// same five contexts with one. The other four are unchanged either way.
func TestSysParamsPidIsEmptyInsideABodyARealShellWouldHaveForked(t *testing.T) {
	self := strconv.Itoa(os.Getpid())
	for _, c := range []struct{ name, src, want string }{
		{"the shell itself", `print -r -- "[$sysparams[pid]]"`, "[" + self + "]"},
		{"a subshell", `( print -r -- "[$sysparams[pid]]" )`, "[]"},
		{"a command substitution", `print -r -- "[$(print -rn -- $sysparams[pid])]"`, "[]"},
		{"a process substitution", "read -r line < <(print -r -- \"[$sysparams[pid]]\")\nprint -r -- $line", "[]"},
		{"a background job", `{ print -r -- "[$sysparams[pid]]" } &` + "\nwait", "[]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := sysParam(t, c.src); got != c.want+"\n" {
				t.Errorf("output = %q, want %q", got, c.want+"\n")
			}
		})
	}
}

// And `$$` is unchanged everywhere, which is the half that makes the answer
// above a *difference* rather than a shell that has lost track of itself. Real
// zsh reports the parent's number in all five too — that is the whole reason
// `$sysparams[pid]` exists next to it.
func TestTheShellsOwnPidIsTheSameInsideThoseBodies(t *testing.T) {
	self := strconv.Itoa(os.Getpid())
	got := sysParam(t, `print -rn -- "[$$]"
( print -rn -- "[$$]" )
print -rn -- "[$(print -rn -- $$)]"
read -r line < <(print -r -- "[$$]")
print -rn -- $line
{ print -rn -- "[$$]" } &
wait
print`)
	want := strings.Repeat("["+self+"]", 5) + "\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestATeardownInsideAProcessSubstitutionDoesNotSignalTheShellsGroup is the
// shape that was measured in the wild, asserted at the one place it can be
// asserted without the test aiming a SIGTERM at the process running it: the
// gate every signal leaving this shell passes through.
//
// The prompt theme on this machine writes, inside a `<(…)` body, a watchdog
// whose last act is `kill -- -$pgid` with `pgid` taken from `$sysparams[pid]`.
// Before #2046 that reached the gate as the shell's own process group,
// negated. It must now reach the gate as nothing at all.
func TestATeardownInsideAProcessSubstitutionDoesNotSignalTheShellsGroup(t *testing.T) {
	var mu sync.Mutex
	var aimed []int
	gate := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		if a.Kind == interp.ActionSignal {
			mu.Lock()
			aimed = append(aimed, a.PID)
			mu.Unlock()
			// Denied, so that a shell without the fix reports EPERM rather
			// than signaling the process group this test is running in.
			return interp.Deny
		}
		return interp.Allow
	})

	f, err := syntax.Parse(`read -r line < <(
  pgid=$sysparams[pid]
  print -r -- "[$pgid]"
  kill -- -$pgid
)
print -r -- $line`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Dialect: presetDialect(), Gate: gate,
	}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "[]\n" {
		t.Errorf("the body read %q as its own process, want %q", got, "[]\n")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(aimed) != 0 {
		t.Errorf("signals left the shell aimed at %v, want none; -%d is this process's own group",
			aimed, os.Getpid())
	}
}

// TestTheColonDefaultReachesTheCallersNumber is the limit of the empty
// answer, pinned because the test above does not reach it.
//
// `${x-d}` and `${x:-d}` ask different questions, and the key is *set* — so
// `-` does not reach its default and `:-` does. The tests above assert the
// `-` form, which is the one that makes the deviation look self-evidently
// safe; `:-` is the form the program in the wild actually writes:
//
//	typeset -gi GITSTATUS_DAEMON_PID_$name="${sysparams[procsubstpid]:--1}"
//	                                   — gitstatus.plugin.zsh:640
//
// That program survives because it *also* guards with `[[ $daemon_pid ==
// <1-> ]]` before `kill -- -$daemon_pid`. One without that second guard
// would ask to signal every process it can reach, since `kill -- -1` means
// that in every POSIX shell.
//
// The hazard is the caller's rather than this deviation's — real zsh's `0`
// is worse, and reaches `kill -- -0` through a guard that passes. But the
// row is pinned so that nobody reads the `-` test as covering both, and so
// that a future change to the empty answer has to look at this line. See
// docs/spec/semantics.md, *What a shell with no child subshells reports for
// a pid* (#2125).
func TestTheColonDefaultReachesTheCallersNumber(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The key is set, so `-` keeps the empty value.
			name: "a plain dash keeps the empty value",
			src:  `print -r -- "[${sysparams[procsubstpid]-none}]"`,
			want: "[]\n",
		},
		{
			// The value is empty, so `:-` takes the default.
			name: "a colon dash takes the caller's default",
			src:  `print -r -- "[${sysparams[procsubstpid]:-none}]"`,
			want: "[none]\n",
		},
		{
			name: "the number the program in the wild chose",
			src:  `print -r -- "[${sysparams[procsubstpid]:--1}]"`,
			want: "[-1]\n",
		},
		{
			// And the second guard, which is what makes that program safe.
			name: "the range guard the caller pairs it with rejects it",
			src: `pid=${sysparams[procsubstpid]:--1}
[[ $pid == <1-> ]] && print -r -- "WOULD KILL -$pid"
print -r -- "carried on"`,
			want: "carried on\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sysParam(t, tc.src); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}
