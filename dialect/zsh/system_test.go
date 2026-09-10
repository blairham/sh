// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
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

// TestProcSubstPidRefusesRatherThanReadingAsNone is the one key that is
// refused, and the reason is sharper than incompleteness.
//
// A process substitution here runs on a goroutine of the shell's own process,
// so there is no process to report. The value that means "none yet" is 0, and
// a real plugin's build script writes `kill -- -$sysparams[procsubstpid]` —
// where `kill -- -0` is a signal to the shell's own process group. So the
// plausible answer is the dangerous one, and the key says so at the expansion
// that asked.
func TestProcSubstPidRefusesRatherThanReadingAsNone(t *testing.T) {
	got := sysParam(t, `print -r -- "[$sysparams[procsubstpid]]"`)
	want := "zsh:1: sysparams[procsubstpid]: " +
		"no process substitution runs in a process of its own here\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestTheSetTestOnSysParamsIsAnswered: the guard in front of the read is
// answered rather than refused, which is [interp.Runner.SetAbsentElements]'s
// exemption and the reason a well-written script is not stopped.
func TestTheSetTestOnSysParamsIsAnswered(t *testing.T) {
	got := sysParam(t, `print -r -- "${+sysparams[pid]}${+sysparams[procsubstpid]}`+
		` [${sysparams[procsubstpid]-none}]"`)
	if want := "10 [none]\n"; got != want {
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

// TestTheSystemModuleLoadsWhileThreeOfItsBuiltinsAreMissing is zmodload.go's
// rule stated as a case rather than left in a comment.
//
// The module names nine features and this shell has six of them: three
// builtins were added with #1737 and three are still to come. It loads,
// because what it has not got are builtins and a builtin refuses by name on
// the line that calls it — so a script is told where it depended on one, and
// holding the module shut would stop a file for features it may never call.
func TestTheSystemModuleLoadsWhileThreeOfItsBuiltinsAreMissing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zmodload zsh/system && zmodload -lF zsh/system\n")
	want := "+b:syserror\n+b:sysopen\n+b:sysread\n+b:sysseek\n+b:syswrite\n" +
		"+b:zsystem\n+f:systell\n+p:errnos\n+p:sysparams\n"
	if out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q and 0", out, st, want)
	}
	out, _, errs := runZshSplit(t, t.TempDir(), "zmodload zsh/system\nzsystem\n")
	if got, want := out+errs, "zsh:2: command not found: zsystem\n"; got != want {
		t.Errorf("calling a missing builtin = %q, want %q", got, want)
	}
}
