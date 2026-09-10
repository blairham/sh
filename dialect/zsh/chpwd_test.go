// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `chpwd`, the hook this shell runs once the working directory has moved, and
// the one hook of the family whose site is a builtin rather than the prompt
// loop. #1775.
//
// Measured 2026-09-10, `/opt/homebrew/bin/zsh` 5.9.2, with `-c` — the hook
// needs no terminal, which is itself the first measurement and the reason it
// could not have lived in the prompt loop:
//
//	$ zsh -c 'chpwd() { print "moved to $PWD"; }; cd /tmp'
//	moved to /tmp
//
// The panel's other five have no such hook. A `chpwd` function defined in bash
// 5.3.15, in that binary under an argv[0] of `sh`, in bash 3.2.57, in dash and
// in ksh93 ran on none of their `cd`s and not one of them said anything — see
// dialect/bash's TestThisShellHasNoDirectoryChangeHook.
//
// Every case here prints a marker no other part of the snippet can print. A
// hook that printed `$PWD` after a `cd` into a directory the test also names
// could not tell "the hook ran" from "the line said where we are", and the
// cases below that assert a hook did *not* run would pass against a `cd` that
// fired it every time.

// chpwdTree is somewhere to go: a scratch directory with two subdirectories.
//
// t.TempDir rather than a written-down path, and everything asserted by
// basename, because `/tmp` is a symlink to `/private/tmp` on one of the two
// platforms this has to pass on and a `$PWD` compared against a literal would
// be a test that only holds on the machine it was written on.
func chpwdTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The function, then the array, in the order the array holds them.
//
//	$ zsh -c 'chpwd() { print NAMED }; a() { print A }; b() { print B }
//	         chpwd_functions=(a b); cd /tmp'
//	NAMED
//	A
//	B
func TestChpwdRunsItsFunctionAndThenItsArray(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print CHPWDMARK-NAMED }
a() { print CHPWDMARK-A }
b() { print CHPWDMARK-B }
chpwd_functions=(a b)
cd one
print "st=$?"`)
	const want = "CHPWDMARK-NAMED\nCHPWDMARK-A\nCHPWDMARK-B\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("chpwd = %q (status %d), want %q at 0", out, st, want)
	}
}

// The array on its own is enough, which is the shape `add-zsh-hook` leaves
// behind: it defines no function called `chpwd`, it appends to the array. A
// site that read only the named function would find a correctly registered
// hook and run nothing — #1281 one level down.
func TestChpwdRunsAnArrayWithNoNamedFunctionBehindIt(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `on_cd() { print CHPWDMARK-LIST }
chpwd_functions=(on_cd)
cd one
print "st=$?"`)
	const want = "CHPWDMARK-LIST\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("chpwd = %q (status %d), want %q at 0", out, st, want)
	}
}

// A `cd` that failed runs nothing, and the marker's *absence* is the whole
// assertion: this is the case a hook fired unconditionally would still pass
// if the check were only about where the shell ended up.
//
//	$ zsh -c 'chpwd() { print MARKER-RAN }; cd /nope-nope; echo "st=$?"'
//	zsh:cd:1: no such file or directory: /nope-nope
//	st=1
func TestChpwdDoesNotRunWhenTheCdFailed(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print CHPWDMARK-NAMED }
a() { print CHPWDMARK-A }
chpwd_functions=(a)
cd nosuchdir_zz
print "st=$?"`)
	if strings.Contains(out, "CHPWDMARK") {
		t.Errorf("a failed cd printed %q, want no hook in it", out)
	}
	if !strings.Contains(out, "st=1") || st != 0 {
		t.Errorf("a failed cd printed %q (status %d), want st=1", out, st)
	}
}

// It fires on the *move* and not on the change: `cd` to where the shell
// already is runs it, with `$PWD` and `$OLDPWD` the same directory.
//
//	$ zsh -c 'cd /usr; chpwd() { print "SAME[$PWD][$OLDPWD]" }; cd /usr'
//	SAME[/usr][/usr]
func TestChpwdRunsForACdToTheDirectoryTheShellIsAlreadyIn(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `cd one
chpwd() { print "CHPWDMARK[${PWD##*/}][${OLDPWD##*/}]" }
cd .
print "st=$?"`)
	const want = "CHPWDMARK[one][one]\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("chpwd = %q (status %d), want %q at 0", out, st, want)
	}
}

// `cd -q` is hook suppression and nothing else — the whole of what that letter
// means, and the letter a plugin manager wraps every move in. #1558.
//
// Both halves in one case: a `-q` that suppressed by *not moving* would pass
// a silence check on its own, which is exactly the misreading #1558 was.
func TestCdQuietRunsNoChpwdAndStillMoves(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print CHPWDMARK-NAMED }
a() { print CHPWDMARK-A }
chpwd_functions=(a)
cd -q one
print "st=$? at=${PWD##*/}"`)
	const want = "st=0 at=one\n"
	if out != want || st != 0 {
		t.Errorf("cd -q = %q (status %d), want %q at 0", out, st, want)
	}
}

// The same line without the letter runs it — the pairing that stops the case
// above passing because the hook never fires at all.
func TestTheSameCdWithoutTheQuietLetterRunsChpwd(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print CHPWDMARK-NAMED }
cd one
print "st=$? at=${PWD##*/}"`)
	const want = "CHPWDMARK-NAMED\nst=0 at=one\n"
	if out != want || st != 0 {
		t.Errorf("cd = %q (status %d), want %q at 0", out, st, want)
	}
}

// Where the shell now is and where it was, both already set — and no
// arguments at all.
//
//	$ zsh -c 'chpwd() { print "n=$# 1=[$1] 0=[$0]" }; cd /tmp'
//	n=0 1=[] 0=[chpwd]
func TestChpwdIsToldWhereAndNothingElse(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `cd one
chpwd() { print "CHPWDMARK n=$# 1=[$1] pwd=${PWD##*/} old=${OLDPWD##*/}" }
cd ../two`)
	const want = "CHPWDMARK n=0 1=[] pwd=two old=one\n"
	if out != want || st != 0 {
		t.Errorf("chpwd = %q (status %d), want %q at 0", out, st, want)
	}
}

// A `cd` inside a function fires it at the `cd`, which is the case a
// prompt-loop hook could not have reached at all: the shell is back where the
// caller is by the time any prompt is drawn, and a `zsh -c` script draws none.
func TestChpwdRunsForACdInsideAFunction(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print "CHPWDMARK[${PWD##*/}]" }
f() { cd one; }
f
print "at=${PWD##*/}"`)
	const want = "CHPWDMARK[one]\nat=one\n"
	if out != want || st != 0 {
		t.Errorf("chpwd = %q (status %d), want %q at 0", out, st, want)
	}
}

// Assigning to `PWD` is not a move and runs nothing.
//
//	$ zsh -c 'chpwd() { print "H[$PWD]" }; PWD=/tmp; echo "pwd=$PWD"'
//	pwd=/tmp
func TestAssigningToPwdRunsNoChpwd(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print CHPWDMARK-NAMED }
PWD=/nowhere
print "pwd=$PWD"`)
	const want = "pwd=/nowhere\n"
	if out != want || st != 0 {
		t.Errorf("an assignment printed %q (status %d), want %q at 0", out, st, want)
	}
}

// Every member is told the status of the command before the `cd`, none of
// them can change what the next command reads, and one that fails stops
// nothing.
//
//	$ zsh -c 'chpwd() { print "named st=$?"; return 3 }
//	         a() { print "a st=$?"; return 9 }; b() { print "b st=$?" }
//	         chpwd_functions=(a b); (exit 5); cd /tmp; print "after=$?"'
//	named st=5
//	a st=5
//	b st=5
//	after=0
func TestTheChpwdChainIsToldTheStatusAndCannotChangeIt(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `chpwd() { print "CHPWDMARK-NAMED=$?"; return 3 }
a() { print "CHPWDMARK-A=$?"; return 9 }
b() { print "CHPWDMARK-B=$?" }
chpwd_functions=(a b)
(exit 5)
cd one
print "after=$?"`)
	const want = "CHPWDMARK-NAMED=5\nCHPWDMARK-A=5\nCHPWDMARK-B=5\nafter=0\n"
	if out != want || st != 0 {
		t.Errorf("the chain printed %q (status %d), want %q at 0", out, st, want)
	}
}

// A name in the array with nothing behind it, or with a builtin or a file on
// `PATH` behind it, is passed over in silence and the rest of the array still
// runs.
//
//	$ zsh -c 'a(){print A}; chpwd_functions=(a nosuchfn_zz b2)
//	         b2(){print B2}; cd /tmp; echo "st=$?"'
//	A
//	B2
//	st=0
func TestAChpwdArrayEntryThatIsNotAFunctionIsPassedOverInSilence(t *testing.T) {
	out, st := runZsh(t, chpwdTree(t), `a() { print CHPWDMARK-A }
b() { print CHPWDMARK-B }
chpwd_functions=(a nosuchfn_zz print b)
cd one
print "st=$?"`)
	const want = "CHPWDMARK-A\nCHPWDMARK-B\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("the chain printed %q (status %d), want %q at 0", out, st, want)
	}
}

// `pushd` and `popd` move through `cd` here and in that shell both, so both
// run it — and `${DIRSTACK}` is asserted beside the marker so that a `pushd`
// which had stopped pushing could not pass as one that fired the hook.
//
//	$ zsh -c 'chpwd() { print "H[$PWD][$OLDPWD]" }; pushd /tmp; print --; popd'
//	H[/tmp][/]
//	--
//	H[/][/tmp]
func TestPushdAndPopdRunChpwd(t *testing.T) {
	out, st := runZshPrelude(t, chpwdTree(t), `chpwd() { print "CHPWDMARK[${PWD##*/}]" }
pushd one
print "pushed=${#DIRSTACK[@]}"
popd
print "popped=${#DIRSTACK[@]} at=${PWD##*/}"`)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 || lines[0] != "CHPWDMARK[one]" || lines[1] != "pushed=1" ||
		lines[2] == "" || !strings.HasPrefix(lines[2], "CHPWDMARK[") ||
		!strings.HasPrefix(lines[3], "popped=0 at=") {
		t.Errorf("pushd/popd printed %q (status %d), want a hook on each move", out, st)
	}
}
