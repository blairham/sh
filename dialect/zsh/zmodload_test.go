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

// **A feature holds a module shut when nothing would tell a script it was
// missing.** The two halves of that rule, side by side, because each on its
// own is indistinguishable from a shell that simply had the rule the other way
// round.
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
// `zsh/system` is six builtins, a function and two parameters. It refuses,
// and it names **`systell`, `errnos` and `sysparams`**: the function and the
// two parameters, and not one of the six builtins beside them. Those two
// parameters are absent in the sense that has no call site — nothing is
// registered for either, so `${sysparams[pid]}` would read empty at status 0
// and reach a caller as data rather than as a diagnostic. That is the failure
// this builtin was written to prevent, and the one case still left in the rule
// after #1146: a parameter that *does* refuse by name no longer holds
// anything, which the `zsh/parameter` test below is the other side of.
//
// It was `zsh/terminfo` here until #1388, which is the gate opening by itself
// exactly as the file above says it would: the two capability parameters
// arrived, nothing in zmodload.go changed, and both modules started loading.
func TestAMissingBuiltinDoesNotHoldAModuleShutAndAMissingParameterDoes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil 2>&1
print -r -- "zutil=$?"
zregexparse a b c 2>&1
print -r -- "call=$?"
zmodload zsh/system 2>&1
print -r -- "system=$?"`)
	want := "zutil=0\n" +
		"zsh:3: command not found: zregexparse\ncall=127\n" +
		"zsh:5: failed to load module `zsh/system': " +
		"systell, errnos and sysparams are not implemented yet\n" +
		"system=1\n"
	if out != want || st != 0 {
		t.Errorf("the two halves of the rule = %q (status %d), want %q", out, st, want)
	}
}

// **`zsh/parameter` loads, and the twenty-eight it has not got are still not
// there.** This is the whole of #1146 in one shell, and the two halves have to
// be read together or either one alone is a shell that lies.
//
// Five of the thirty-three are implemented (#1060). Ten more are empty and
// right to be. The other eighteen refuse by name at the expansion that reads
// one — which is what lets the module load at all, and it is why the second
// line here is not `n=0`. A shell that loaded the module *and* answered `0`
// for `$jobstates` would be exactly the silent success the module rule was
// written to prevent; a shell that refuses the module over a parameter no
// script in the file touches stops a plugin manager at its second line.
//
// The count is checked with `$functions` rather than assumed, because "the
// module loads" is worth nothing if the five that made it worth loading
// stopped answering.
func TestZmodloadLoadsAModuleWhoseAbsencesRefuseByName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter 2>&1
print -r -- "st=$?"
g(){ :; }
print -r -- "functions=${#functions}"
print -r -- "galiases=${#galiases}"
print -r -- "jobstates=${#jobstates}"
print -r -- "after=$?"`)
	want := "st=0\nfunctions=1\ngaliases=0\n" +
		"zsh:6: jobstates: parameter not implemented yet\n"
	if out != want || st != 1 {
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
	out, st := runZsh(t, t.TempDir(), `zmodload -s zsh/system 2>&1
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
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/nosuchmodule zsh/system zsh/main 2>&1
print -r -- "st=$?"
zmodload -e zsh/main
print -r -- "main-still=$?"`)
	want := "zsh:1: failed to load module `zsh/nosuchmodule': not implemented yet\n" +
		"zsh:1: failed to load module `zsh/system': " +
		"systell, errnos and sysparams are not implemented yet\n" +
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
// `zsh/system` rather than `zsh/zutil` or `zsh/parameter`, because both of
// those load now — the plugin manager's lines 231 and 232 go through — and no
// longer `zsh/terminfo`, which loads since #1388. `zmodload zsh/zpty
// zsh/system 2>/dev/null` is a line that file really writes, in this shape,
// by a script that expects the module to be missing on plenty of machines.
func TestZmodloadRefusalReachesTheScriptsOwnBranch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload zsh/system 2>/dev/null || { print -r -- "aborting"; }
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

// `-F` with features named selects which of them the module exposes, and the
// selection is what `-lF` then reports. Measured byte for byte against zsh
// 5.9.2 (2026-09-09): a module `-F` loads starts with everything *off* and
// gains only what is named, so the three features nobody asked for are `-`.
//
// The control is the plain load in the same snippet: a shell that accepted
// `-F` and did nothing would write `+` against all four here and pass a test
// that only asked for status 0.
func TestZmodloadDashFSelectsWhichFeaturesAModuleExposes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/zutil b:zstyle
print -r -- "select=$?"
zmodload -lF zsh/zutil
zmodload -e zsh/zutil
print -r -- "loaded=$?"
zmodload zsh/zutil
zmodload -lF zsh/zutil`)
	want := "select=0\n" +
		"-b:zformat\n-b:zparseopts\n-b:zregexparse\n+b:zstyle\n" +
		"loaded=0\n" +
		"+b:zformat\n+b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("selecting a feature = %q (status %d), want %q", out, st, want)
	}
}

// The starting point depends on whether the module is loaded already, and
// that is the whole difference between a selection and a delta. Named
// features on a module that is not loaded are the only ones that end up on;
// on one that is, the features nobody named stay exactly as they were.
func TestZmodloadDashFIsADeltaOnAModuleAlreadyLoaded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil
zmodload -F zsh/zutil -b:zparseopts
print -r -- "narrow=$?"
zmodload -lF zsh/zutil`)
	want := "narrow=0\n+b:zformat\n-b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("a delta on a loaded module = %q (status %d), want %q", out, st, want)
	}
}

// A bare feature name means `+`, which is the spelling every real caller
// uses, and the last operand about a feature wins.
func TestZmodloadDashFReadsTheSignsOnItsOperands(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/zutil b:zstyle +b:zformat -b:zstyle
print -r -- "st=$?"
zmodload -lF zsh/zutil`)
	want := "st=0\n+b:zformat\n-b:zparseopts\n-b:zregexparse\n-b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("the operand signs = %q (status %d), want %q", out, st, want)
	}
}

// **An operand naming no feature of the module leaves the module unloaded**,
// even when a good operand came first. Measured, and it is what tells a
// refusal from a half-applied command: a shell that applied as it went would
// answer `loaded=0` on the second question.
func TestZmodloadDashFAppliesNothingWhenAnOperandNamesNoFeature(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/zutil +b:zstyle +b:nosuch 2>&1
print -r -- "st=$?"
zmodload -e zsh/zutil
print -r -- "loaded=$?"`)
	want := "zsh:1: module `zsh/zutil' has no such feature: `b:nosuch'\nst=1\nloaded=1\n"
	if out != want || st != 0 {
		t.Errorf("an unknown feature = %q (status %d), want %q", out, st, want)
	}
}

// **Narrowing moves the verdict**, which is the reason the letter is worth
// having in a shell that cannot load a module at all. `zsh/complete` is held
// shut by its four conditions — the kind with no registry to ask and no call
// site to refuse at — and a caller naming one of its builtins instead is
// asking a question this shell can answer yes to.
//
// The third line is the control that keeps the first two honest: naming one
// of the conditions is still refused, and refused by that condition's name
// alone rather than by all four, so the selection is being read rather than
// waved through.
func TestZmodloadDashFNarrowingMovesTheVerdict(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/complete 2>&1
print -r -- "whole=$?"
zmodload -F zsh/complete b:compadd 2>&1
print -r -- "narrowed=$?"
zmodload -F zsh/complete c:prefix 2>&1
print -r -- "held=$?"`)
	want := "zsh:1: failed to load module `zsh/complete': " +
		"after, between, prefix and suffix are not implemented yet\nwhole=1\n" +
		"narrowed=0\n" +
		"zsh:5: failed to load module `zsh/complete': prefix is not implemented yet\nheld=1\n"
	if out != want || st != 0 {
		t.Errorf("narrowing = %q (status %d), want %q", out, st, want)
	}
}

// A module with no features has its own sentence on this path too, and it is
// the shell speaking rather than the builtin — the other way round from the
// identical sentence the `-lF` listing gives, which is measured and is why
// the two paths keep their own locations.
func TestZmodloadDashFOnAModuleWithNoFeatures(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/main +b:x 2>&1
print -r -- "select=$?"
zmodload -lF zsh/main 2>&1
print -r -- "list=$?"`)
	want := "zsh:1: module `zsh/main' does not support features\nselect=1\n" +
		"zsh:zmodload:3: module `zsh/main' does not support features\nlist=1\n"
	if out != want || st != 0 {
		t.Errorf("a module with no features = %q (status %d), want %q", out, st, want)
	}
}

// `-u` and `-F` are refused together, in zsh's own wording — the sentence is
// about the combination rather than about either letter, and the four other
// letters it names reach an unimplemented-letter refusal here first.
func TestZmodloadDashUCannotBeCombinedWithDashF(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -uF zsh/zutil +b:zstyle 2>&1
print -r -- "st=$?"`)
	want := "zsh:zmodload:1: -b, -c, -f, -p and -u cannot be combined with -F\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("-uF = %q (status %d), want %q", out, st, want)
	}
}

// The operands after the module are a filter on the listing, compared to the
// feature names *as written*: a bare name narrows it to one line and a signed
// one narrows it to none. Measured, and it looks like a slip until the two
// rules are separated — the sign is stripped to decide whether the operand
// names a feature at all, and not stripped again to decide what it matches.
func TestZmodloadListingFiltersOnTheOperandAsWritten(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil
zmodload -lF zsh/zutil b:zstyle
print -r -- "bare=$?"
zmodload -lF zsh/zutil +b:zstyle
print -r -- "signed=$?"
zmodload -lF zsh/zutil b:nosuch 2>&1
print -r -- "unknown=$?"`)
	want := "+b:zstyle\nbare=0\n" +
		"signed=0\n" +
		"zsh:zmodload:6: module `zsh/zutil' has no such feature: `b:nosuch'\nunknown=1\n"
	if out != want || st != 0 {
		t.Errorf("the listing filter = %q (status %d), want %q", out, st, want)
	}
}

// `-LF` writes the selection as the command that would reproduce it, and with
// no module names it does that for every loaded module that has features.
// `-lF` with no module is refused instead, which is measured and is the one
// asymmetry between the two listing letters.
func TestZmodloadDashLFWritesTheSelectionAsACommand(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/zutil b:zformat b:zstyle
zmodload -LF zsh/zutil
print -r -- "one=$?"
zmodload -LF
print -r -- "all=$?"
zmodload -lF 2>&1
print -r -- "list=$?"`)
	want := "zmodload -F zsh/zutil b:zformat b:zstyle\none=0\n" +
		"zmodload -F zsh/zutil b:zformat b:zstyle\nall=0\n" +
		"zsh:zmodload:6: -F requires a module name\nlist=1\n"
	if out != want || st != 0 {
		t.Errorf("-LF = %q (status %d), want %q", out, st, want)
	}
}

// **A module that is not loaded has nothing on**, which is the rule the whole
// selection turns on and the one an unload leans on rather than cleaning up
// after itself. So a narrowed load *after* an unload starts from silence and
// not from what the last `-F` left, and a plain one after it starts wide.
//
// Both halves in one snippet because either alone is satisfied by the wrong
// rule: the plain load is widened by zmodloadWiden whatever the unload did,
// so only the narrowed reload can say that the earlier selection is gone.
func TestAModuleThatIsNotLoadedHasNothingOn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/zutil b:zstyle
zmodload -u zsh/zutil
zmodload -F zsh/zutil b:zformat
zmodload -lF zsh/zutil
zmodload zsh/zutil
zmodload -lF zsh/zutil`)
	want := "+b:zformat\n-b:zparseopts\n-b:zregexparse\n-b:zstyle\n" +
		"+b:zformat\n+b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("a reload after an unload = %q (status %d), want %q", out, st, want)
	}
}

// The narrowing is the subshell's, the same as the loaded set it sits beside
// — which is what keeping it in the runner's own tables is for.
func TestZmodloadNarrowingInASubshellLeavesTheParentAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil
(zmodload -F zsh/zutil -b:zstyle; zmodload -lF zsh/zutil)
zmodload -lF zsh/zutil`)
	want := "+b:zformat\n+b:zparseopts\n+b:zregexparse\n-b:zstyle\n" +
		"+b:zformat\n+b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("a subshell's narrowing = %q (status %d), want %q", out, st, want)
	}
}

// The line the issue was filed on, in the shape a real plugin writes it:
// `_fzf_completion` asks `zsh/parameter` for the one parameter it reads.
func TestZmodloadDashFReachesTheLineAPluginWrites(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload -F zsh/parameter p:functions 2>/dev/null || print -r -- "no functions"
print -r -- "st=$?"`)
	want := "st=0\n"
	if out != want || st != 0 {
		t.Errorf("the plugin's line = %q (status %d), want %q", out, st, want)
	}
}
