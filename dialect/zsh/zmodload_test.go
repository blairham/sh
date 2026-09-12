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
// `zsh/complete` is two builtins and four **conditions** — `-after`,
// `-between`, `-prefix` and `-suffix`. It refuses, and it names the four
// conditions and neither of the two builtins beside them. A condition is the
// one kind of feature with no registry to ask and no call site to refuse at:
// `[[ -after x ]]` in a shell without it is not `command not found`, it is a
// test that quietly answers something, so a caller cannot be told where it
// depended on one. That is the failure this builtin was written to prevent,
// and it is the last kind of feature still holding a module shut.
//
// The other three kinds have all left this list, and each left the same way —
// the gate opening by itself as the shell caught up, with nothing in
// zmodload.go changed for it. A builtin never held. A parameter stopped
// holding in #1146 once it could refuse by name, which the `zsh/parameter`
// test below is the other side of. This case named `zsh/terminfo` until
// #1388, when the two capability parameters arrived; it named `zsh/system`
// until #1618, when `systell`, `$errnos` and `$sysparams` did.
func TestAMissingBuiltinDoesNotHoldAModuleShutAndAMissingConditionDoes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil 2>&1
print -r -- "zutil=$?"
zregexparse a b c 2>&1
print -r -- "call=$?"
zmodload zsh/complete 2>&1
print -r -- "complete=$?"`)
	want := "zutil=0\n" +
		"zsh:3: command not found: zregexparse\ncall=127\n" +
		"zsh:5: failed to load module `zsh/complete': " +
		"after, between, prefix and suffix are not implemented yet\n" +
		"complete=1\n"
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
	out, st := runZsh(t, t.TempDir(), `zmodload -s zsh/complete 2>&1
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
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/nosuchmodule zsh/complete zsh/main 2>&1
print -r -- "st=$?"
zmodload -e zsh/main
print -r -- "main-still=$?"`)
	want := "zsh:1: failed to load module `zsh/nosuchmodule': not implemented yet\n" +
		"zsh:1: failed to load module `zsh/complete': " +
		"after, between, prefix and suffix are not implemented yet\n" +
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
// `zsh/zpty` rather than `zsh/zutil` or `zsh/parameter`, because both of those
// load now — the plugin manager's lines 231 and 232 go through — and no longer
// `zsh/terminfo`, which loads since #1388, nor `zsh/system`, which loads since
// #1618. `zmodload zsh/zpty zsh/system 2>/dev/null` is a line that file really
// writes, in this shape, by a script that expects the module to be missing on
// plenty of machines; half of it still is.
//
// A module this shell has *no part of* rather than one short of a feature,
// which is the other half of what a script sees. Both are one status and one
// silenced line to the caller, and the branch it takes is the same.
func TestZmodloadRefusalReachesTheScriptsOwnBranch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload zsh/zpty 2>/dev/null || { print -r -- "aborting"; }
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
// having in a shell that cannot load a module at all — and since #1634 it
// moves in *both* directions, which is the whole of what that issue settled.
//
// `zsh/zutil` is four builtins and this shell has three of them. A plain load
// is 0, because a builtin nobody has written is loud at its own call site and
// so never holds a module shut. Naming one of the three under `-F` is 0 for
// the same reason and a different one: it is there. Naming `zregexparse` — the
// one that is not — is **1**, where the plain load of the same module a line
// earlier was 0.
//
// That is not the plain rule being contradicted. `zmodload zsh/zutil` is the
// *shell* inferring that a script wanting the module wants all of it, and a
// plugin manager writing `|| return 1` there calls three of the four and never
// the fourth. `zmodload -F zsh/zutil b:zregexparse` is the script saying which
// one it wants, and the only honest answer to that question is whether it can
// have it — measured against zsh 5.9.2, a `-F` at status 0 is followed by a
// builtin that runs, which is why `zmodload -F zsh/stat b:zstat || return` is
// written as a guard by a prompt theme and a plugin manager alike.
//
// The last line is the control that keeps the rest honest: a condition is
// still refused by its own name and not by all four of its module's, so the
// selection is being read rather than waved through.
func TestZmodloadDashFNarrowingMovesTheVerdict(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zutil 2>&1
print -r -- "whole=$?"
zmodload -F zsh/zutil b:zstyle 2>&1
print -r -- "named-what-we-have=$?"
zmodload -F zsh/zutil b:zregexparse 2>&1
print -r -- "named-what-we-have-not=$?"
zmodload -F zsh/complete c:prefix 2>&1
print -r -- "held=$?"`)
	want := "whole=0\n" +
		"named-what-we-have=0\n" +
		"zsh:5: failed to load module `zsh/zutil': zregexparse is not implemented yet\n" +
		"named-what-we-have-not=1\n" +
		"zsh:7: failed to load module `zsh/complete': prefix is not implemented yet\nheld=1\n"
	if out != want || st != 0 {
		t.Errorf("narrowing = %q (status %d), want %q", out, st, want)
	}
}

// The four lines a real startup writes about these three modules, and the four
// answers they now get (#1634).
//
// Three are a prompt theme's, written one after another with no `||` guard at
// all, so each of them was a line of `failed to load module` on every start of
// this shell. The fourth is a plugin manager's, guarded — and a guard is the
// case the rule above is *for*: answering 0 there without `zf_rm` would make
// the `|| return` not fire and leave the failure to turn up later, in a
// function whose caller has gone.
func TestTheThreeModulesAStartupNarrowsAreLoadedByTheFeaturesItNames(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload -F zsh/stat b:zstat 2>&1
print -r -- "stat=$?"
zmodload -F zsh/net/socket b:zsocket 2>&1
print -r -- "socket=$?"
zmodload -F zsh/files b:zf_mv b:zf_rm 2>&1
print -r -- "files=$?"
zmodload -F zsh/files b:zf_rm 2>&1 || print -r -- "guard returned"
print -r -- "guard=$?"`)
	want := "stat=0\nsocket=0\nfiles=0\nguard=0\n"
	if out != want || st != 0 {
		t.Errorf("the startup lines = %q (status %d), want %q", out, st, want)
	}
}

// And the half of each module that would shadow a command: produced on
// request, and only on request.
//
// `stat`, `rm`, `mv` and the six beside them are real features of these
// modules, and this shell's builtins are registered before any script runs —
// so a `stat` in the table used to be a `stat` every script got, and every
// `stat -f %z` on the machine would have stopped reaching the command. They
// were left out for that reason and `zmodload -F zsh/stat b:stat` refused by
// name.
//
// It is a third state that makes them affordable, not a compromise between
// the two the refusal was choosing from. Registered **withdrawn**, the name
// is out of the lookup until a `zmodload` asks for it — which is precisely
// the state zsh is in, and this asserts both halves of it (#1670).
//
// Every line below is byte-identical to zsh 5.9.2, measured 2026-09-12. The
// two that discriminate hardest are the last two: `rm: command` proves that
// naming `mv` and `ln` produced *those* and did not open the module, and
// `zf_mv: none` proves the narrowed load withdrew the eighteen it did not
// name — the `zf_` spellings included, which are otherwise always present
// here.
func TestTheNamesThatWouldShadowACommandArriveOnlyWhenAsked(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `whence -w stat rm mv
zmodload -F zsh/stat b:stat 2>&1
print -r -- "stat=$?"
whence -w stat zstat
zmodload -F zsh/files b:mv b:ln 2>&1
print -r -- "two=$?"
whence -w mv ln rm zf_mv`)
	// `none` where a shell on a real machine writes `command`: this runner
	// has no PATH, so a name that is not a builtin finds nothing rather than
	// /bin/rm. The two are the same answer to the question being asked —
	// **not this shell's** — and either one tells `builtin` apart.
	want := "stat: none\nrm: none\nmv: none\n" +
		"stat=0\nstat: builtin\nzstat: none\n" +
		"two=0\nmv: builtin\nln: builtin\nrm: none\nzf_mv: none\n"
	if out != want || st != 1 {
		t.Errorf("the shadowing names = %q (status %d), want %q at 1", out, st, want)
	}
}

// A plain load switches all eighteen on, and an unload puts the nine plain
// spellings back out of the table.
//
// The unload row is the one that had to be measured rather than reasoned:
// three modules lose their builtins when unloaded and `zsh/zutil` keeps
// every one of them, so the rule this shell states is the narrow one — an
// unload undoes the load. See zmodloadRelease.
func TestAPlainLoadSwitchesTheShadowingNamesOnAndAnUnloadPutsThemBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/files
print -r -- "load=$?"
whence -w rm zf_rm
zmodload -u zsh/files
print -r -- "unload=$?"
whence -w rm
zmodload zsh/zutil
zmodload -u zsh/zutil
whence -w zstyle`)
	want := "load=0\nrm: builtin\nzf_rm: builtin\n" +
		"unload=0\nrm: none\nzstyle: builtin\n"
	if out != want || st != 0 {
		t.Errorf("load and unload = %q (status %d), want %q at 0", out, st, want)
	}
}

// A whole load of the same three modules is still 0, and that is the plain
// rule standing where it always did rather than an inconsistency.
//
// `zmodload zsh/files` names eighteen builtins and this shell has nine of
// them. Nothing in that line says which nine the script wanted, so nothing is
// gained by refusing: a script that goes on to call `rm` gets the `rm` on its
// `$PATH`, which is the command it would have got in any other shell in the
// world, and one that calls `zf_rm` gets this shell's. The refusal is reserved
// for the line that *asked*.
func TestAWholeLoadOfTheThreeModulesIsStillTheInferredQuestion(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/stat zsh/net/socket zsh/files 2>&1
print -r -- "st=$?"
zmodload -lF zsh/stat`)
	want := "st=0\n+b:stat\n+b:zstat\n"
	if out != want || st != 0 {
		t.Errorf("a whole load = %q (status %d), want %q", out, st, want)
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
