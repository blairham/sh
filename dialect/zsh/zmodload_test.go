// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zmodload`, measured against zsh 5.9.2 (2026-09-06). Whole rendered lines
// with their locations, because the location is half of what these messages
// say: `bad option` carries the builtin's name and a load failure does not.

// What a fresh shell has loaded, and the two shapes it says it in.
func TestZmodloadListsWhatAFreshShellHasLoaded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload
print -r -- "bare=$?"
zmodload -L
print -r -- "commands=$?"`)
	want := "zsh/main\nbare=0\nzmodload zsh/main\ncommands=0\n"
	if out != want || st != 0 {
		t.Errorf("zmodload = %q (status %d), want %q", out, st, want)
	}
}

// `-e` asks rather than loads, and says nothing either way.
func TestZmodloadDashEAsksWhetherAModuleIsLoaded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -e zsh/main
print -r -- "main=$?"
zmodload -e zsh/zutil
print -r -- "zutil=$?"
zmodload -e zsh/main zsh/zutil
print -r -- "both=$?"`)
	want := "main=0\nzutil=1\nboth=1\n"
	if out != want || st != 0 {
		t.Errorf("zmodload -e = %q (status %d), want %q", out, st, want)
	}
}

// **A missing builtin does not hold a module shut; a missing parameter does.**
// The two halves of that rule, side by side, because each on its own is
// indistinguishable from a shell that simply had the rule the other way round.
//
// `zsh/zutil` is four builtins and no parameters. This shell has three of them
// — `zstyle`, `zparseopts` and `zformat` — and not `zregexparse`, and it
// loads: a builtin nobody has written is `command not found` on the line that
// calls it, which is loud, names itself, and is where a person would have to
// look anyway. Holding the module shut over it stops a plugin manager's first
// line for a builtin that file never calls, and `zregexparse` is the one the
// manual describes in a single sentence — "this implements some internals of
// the _regex_arguments function" — so it is not a thing that can be learned by
// running it.
//
// `zsh/terminfo` is one builtin and one parameter, and this shell has neither.
// It refuses, and it names **`terminfo` alone**: the parameter, not `echoti`
// beside it. A missing parameter reads `0` at status 0 and reaches a caller as
// data rather than as a diagnostic, which is the failure this builtin was
// written to prevent.
func TestAMissingBuiltinDoesNotHoldAModuleShutAndAMissingParameterDoes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil 2>&1
print -r -- "zutil=$?"
zregexparse a b c 2>&1
print -r -- "call=$?"
zmodload zsh/terminfo 2>&1
print -r -- "terminfo=$?"`)
	want := "zutil=0\n" +
		"zsh:3: command not found: zregexparse\ncall=127\n" +
		"zsh:5: failed to load module `zsh/terminfo': terminfo is not implemented yet\n" +
		"terminfo=1\n"
	if out != want || st != 0 {
		t.Errorf("the two halves of the rule = %q (status %d), want %q", out, st, want)
	}
}

// A module this shell has only *part* of counts instead of naming: twenty-eight
// missing parameters on one line is not something anyone can read, and the two
// numbers together are what say how far off it is. Five of `zsh/parameter`'s
// thirty-three are here (#1060) and the module still refuses, which is the
// rule for a parameter: an absent one reads empty at status 0 and reaches a
// caller as data rather than as a diagnostic.
func TestZmodloadCountsTheFeaturesOfAModuleItHasMostOfMissing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter 2>&1
print -r -- "st=$?"`)
	want := "zsh:1: failed to load module `zsh/parameter': " +
		"28 of its 33 features are not implemented yet\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("zmodload zsh/parameter = %q (status %d), want %q", out, st, want)
	}
}

// A feature that is neither a builtin nor a parameter has no registry to ask,
// so it holds its module shut and is *named*. `zsh/complete` is the module
// that shows it: two builtins and four conditions, and it is the **four
// conditions** that are named — `compadd` and `compset` are missing too and
// are the loud kind. A shell that counted an unaskable feature as present
// would load this module and look more capable than it is, which is the silent
// success in miniature: `[[ -prefix x ]]` has no call site to complain at.
func TestAFeatureThatCannotBeAskedAboutHoldsItsModuleShut(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/complete 2>&1
print -r -- "st=$?"`)
	want := "zsh:1: failed to load module `zsh/complete': " +
		"after, between, prefix and suffix are not implemented yet\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("zmodload zsh/complete = %q (status %d), want %q", out, st, want)
	}
}

// A name that is in no table at all, and the proof that a load failure does
// **not** carry the builtin's name in its location where every other message
// this builtin writes does. That is measured in zsh: the module loader speaks
// for the first and the builtin for the second.
func TestALoadFailureIsNotTheBuiltinSpeaking(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/nosuchmodule 2>&1
print -r -- "load=$?"
zmodload -q zsh/main 2>&1
print -r -- "opt=$?"`)
	want := "zsh:1: failed to load module `zsh/nosuchmodule': not implemented yet\nload=1\n" +
		"zsh:zmodload:3: bad option: -q\nopt=1\n"
	if out != want || st != 0 {
		t.Errorf("the two locations = %q (status %d), want %q", out, st, want)
	}
}

// `-s` is what a script can act on with no wording at all: the complaint is
// silenced and the status is still 1. A shell that answered 0 here sends a
// plugin manager on to call a builtin the module was supposed to bring.
func TestZmodloadDashSIsSilentAndStillFails(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -s zsh/parameter 2>&1
print -r -- "st=$?"`)
	want := "st=1\n"
	if out != want || st != 0 {
		t.Errorf("zmodload -s = %q (status %d), want %q", out, st, want)
	}
}

// Every module named is attempted, and one that will not load does not stop
// the ones after it — which is measured in zsh, where a middle module that
// fails leaves the one after it loaded. Two failures are what shows it here:
// a shell that returned at the first would write one line, and a module that
// was already loaded cannot show it at all, because becoming loaded twice
// looks the same as not being reached.
func TestZmodloadAttemptsEveryModuleNamed(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/nosuchmodule zsh/terminfo zsh/main 2>&1
print -r -- "st=$?"
zmodload -e zsh/main
print -r -- "main-still=$?"`)
	want := "zsh:1: failed to load module `zsh/nosuchmodule': not implemented yet\n" +
		"zsh:1: failed to load module `zsh/terminfo': " +
		"terminfo is not implemented yet\n" +
		"st=1\nmain-still=0\n"
	if out != want || st != 0 {
		t.Errorf("three modules = %q (status %d), want %q", out, st, want)
	}
}

// `-u` says nothing about whether a name is a module — only whether it is
// loaded here and now. `zsh/mathfunc` is a module zsh ships and gets the same
// `no such module` as a name nobody has ever used; `zsh/main` is loaded, so
// it goes, and the emptied listing survives being written.
func TestZmodloadDashUMeansNotLoaded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -u zsh/mathfunc 2>&1
print -r -- "real=$?"
zmodload -u zsh/nosuchmodule 2>&1
print -r -- "unreal=$?"
zmodload -u zsh/main
print -r -- "main=$?"
zmodload
print -r -- "listing=$?"`)
	want := "zsh:zmodload:1: no such module zsh/mathfunc\nreal=1\n" +
		"zsh:zmodload:3: no such module zsh/nosuchmodule\nunreal=1\n" +
		"main=0\nlisting=0\n"
	if out != want || st != 0 {
		t.Errorf("zmodload -u = %q (status %d), want %q", out, st, want)
	}
}

// The feature letters. `-l` needs `-F`, `-F` needs a module, and a module
// that is loaded but names no features has its own sentence — all three
// measured, and the third is the one `zsh/main` reaches.
func TestZmodloadFeatureLetters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -l zsh/main 2>&1
print -r -- "l=$?"
zmodload -F 2>&1
print -r -- "F=$?"
zmodload -lF zsh/main 2>&1
print -r -- "lF-main=$?"
zmodload -lF zsh/zutil 2>&1
print -r -- "lF-absent=$?"`)
	want := "zsh:zmodload:1: -l is only allowed with -F\nl=1\n" +
		"zsh:zmodload:3: -F requires a module name\nF=1\n" +
		"zsh:zmodload:5: module `zsh/main' does not support features\nlF-main=1\n" +
		"zsh:zmodload:7: module `zsh/zutil' is not yet loaded\nlF-absent=1\n"
	if out != want || st != 0 {
		t.Errorf("the feature letters = %q (status %d), want %q", out, st, want)
	}
}

// The letters zsh has and this shell has not are named as missing; the ones
// zsh does not have at all are `bad option`. A script can tell the two apart,
// which is the whole point of keeping the first wording.
func TestZmodloadTellsAMissingLetterFromAnUnknownOne(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -a zsh/x mybuiltin 2>&1
print -r -- "a=$?"
zmodload -d zsh/main 2>&1
print -r -- "d=$?"
zmodload -X zsh/main 2>&1
print -r -- "X=$?"`)
	want := "zsh:zmodload:1: -a is not implemented yet\na=1\n" +
		"zsh:zmodload:3: -d is not implemented yet\nd=1\n" +
		"zsh:zmodload:5: bad option: -X\nX=1\n"
	if out != want || st != 0 {
		t.Errorf("the refused letters = %q (status %d), want %q", out, st, want)
	}
}

// The line a plugin manager actually writes, in the shape it writes it: the
// refusal is legible at the line that asked, and the `||` branch runs. It was
// `command not found: zmodload`, which is not something that script can act
// on at all.
//
// `zsh/parameter` rather than `zsh/zutil`, because `zsh/zutil` loads now — and
// that is the other half of the same story: the plugin manager's line 231 goes
// through and its line 232 is the one that still stops it.
func TestZmodloadRefusalReachesTheScriptsOwnBranch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload zsh/parameter 2>/dev/null || { print -r -- "aborting"; }
print -r -- "st=$?"`)
	want := "aborting\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("the `||` branch = %q (status %d), want %q", out, st, want)
	}
}

// A subshell gets its own set, which is what keeping the store in the
// runner's own table is for.
func TestZmodloadUnloadingInASubshellLeavesTheParentAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `(zmodload -u zsh/main; zmodload; print -r -- "in=$?")
zmodload
print -r -- "out=$?"`)
	want := "in=0\nzsh/main\nout=0\n"
	if out != want || st != 0 {
		t.Errorf("a subshell's modules = %q (status %d), want %q", out, st, want)
	}
}
