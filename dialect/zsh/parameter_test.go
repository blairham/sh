// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `zsh/parameter` module's five, measured against zsh 5.9.2 (2026-09-06)
// with a scratch HOME and no startup files.
//
// **Every assertion that matters here is a read, a mutation and a read
// again**, in one shell, and that shape is the whole point. A parameter filled
// in once and never updated passes any test that defines a function and then
// looks at `$functions` — the snapshot was taken before the look — so a test
// written that way proves nothing about the thing this module is. Three reads
// around two mutations cannot be satisfied by a snapshot at any instant.
//
// The values are bracketed and the whole output is compared, because these are
// listings: a body carries tabs and newlines, a command carries a path, and a
// check on the visible text alone would pass for a view that emitted the wrong
// bytes between them.

// $functions is a view of the function table, and the reads are placed so that
// no single snapshot could answer all three.
func TestFunctionsIsAViewOfTheFunctionTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `echo "a=[${functions[g]:-ABSENT}] n=${#functions}"
g(){ :; }
echo "b=[${functions[g]:-ABSENT}] n=${#functions}"
unset -f g
echo "c=[${functions[g]:-ABSENT}] n=${#functions}"`)
	want := "a=[ABSENT] n=0\nb=[\t:] n=1\nc=[ABSENT] n=0\n"
	if out != want || st != 0 {
		t.Errorf("$functions across two mutations = %q (status %d), want %q", out, st, want)
	}
}

// The body is the lines a listing puts between the braces — tab-indented, no
// header, no trailing newline — and a nested one is indented twice. Asserted
// as bytes, because every character that decides this is whitespace.
func TestFunctionsHoldsTheBodyAListingWouldPrint(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f(){ echo one; if [[ 1 = 1 ]]; then echo two; fi; }`+"\n"+
			`printf "[%s]" "$functions[f]"`)
	want := "[\techo one\n\tif [[ 1 = 1 ]]\n\tthen\n\t\techo two\n\tfi]"
	if out != want || st != 0 {
		t.Errorf("$functions[f] = %q (status %d), want %q", out, st, want)
	}
}

// Assigning to an element *defines* a function, and unsetting one undefines
// it. Both write through to the function table rather than into a stored
// association — which the read afterwards is what proves: a write that landed
// in a stored table would shadow the view, and `${#functions}` would then
// stop moving.
func TestFunctionsIsWrittenThroughToTheFunctionTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `functions[h]="echo made"
h
echo "n=${#functions}"
unset "functions[h]"
h 2>&1
echo "after=$? n=${#functions}"`)
	want := "made\nn=1\nzsh:5: command not found: h\nafter=127 n=0\n"
	if out != want || st != 0 {
		t.Errorf("writing $functions = %q (status %d), want %q", out, st, want)
	}
}

// A body that will not parse defines nothing and says so, rather than storing
// text whose call fails several hundred lines away.
func TestFunctionsRefusesABodyItCannotRead(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `functions[h]="if" 2>&1
echo "n=${#functions}"`)
	want := "zsh:1: h: not a function body this shell can read\nn=0\n"
	if out != want || st != 0 {
		t.Errorf("a bad body = %q (status %d), want %q", out, st, want)
	}
}

// The names are the *listed* set: the script's own, with this dialect's
// prelude left out. `pushd` is a prelude function here — callable, and this
// shell's own — so it must be absent from `$functions` and present to `type`
// at the same time. That is the fourth caller of one rule rather than a fifth
// notion of whose a function is (#1035, #1081, #1082): the predicate is
// speaksForTheShell, reached through ListedFuncNames, and it is the one
// `declare -f` and the `set` listing already ask.
//
// What `type` says about it is a builtin's sentence since #1117, which is
// what real zsh 5.9.2 says: `type pushd` there is `pushd is a shell
// builtin`, and `${(k)functions}` names none of the three.
func TestFunctionsLeavesOutTheFunctionsThatAreTheShell(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `mine(){ :; }
echo "listed=[${(k)functions}]"
type pushd
echo "declared=[$(declare -f)]"`)
	want := "listed=[mine]\npushd is a shell builtin\n" +
		"declared=[mine () {\n\t:\n}]\n"
	if out != want || st != 0 {
		t.Errorf("$functions with a prelude = %q (status %d), want %q", out, st, want)
	}
}

// And a script that redefines a prelude function takes the name back, in both
// listings at once — because both ask the same predicate, which compares the
// *declaration*. A parallel record of prelude-ness set at definition time
// would have to be cleared on every route a redefinition can arrive by, and
// this is the route a fifth one would have missed.
func TestRedefiningAPreludeFunctionPutsItInTheListing(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `echo "before=[${(k)functions}]"
pushd(){ echo mine; }
echo "after=[${(k)functions}] body=[$functions[pushd]]"`)
	want := "before=[]\nafter=[pushd] body=[\techo mine]\n"
	if out != want || st != 0 {
		t.Errorf("redefining pushd = %q (status %d), want %q", out, st, want)
	}
}

// Removing an element is the same operation `unset -f` is, and the one name
// where that is observable is a *redefined* prelude function: taking the
// script's version away gives the name back to this dialect's own rather than
// leaving it undefined. Two spellings of one thing must not leave the shell in
// two states (#1082).
func TestUnsettingAFunctionsElementIsUnsetDashF(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `pushd(){ echo mine; }
echo "listed=[${(k)functions}]"
unset "functions[pushd]"
echo "after=[${(k)functions}]"
type pushd
unset "functions[pushd]"
echo "again=[${(k)functions}]"`)
	want := "listed=[pushd]\nafter=[]\npushd is a shell builtin\nagain=[]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting a redefined prelude function = %q (status %d), want %q", out, st, want)
	}
}

// $options is a view of the option namespace `setopt`, `unsetopt` and
// `[[ -o ]]` already share — the same 197 keys real zsh carries, and the same
// live state.
func TestOptionsIsAViewOfTheOptionNamespace(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `echo "n=${#options}"
echo "a=$options[extendedglob]"
setopt extendedglob
echo "b=$options[extendedglob]"
unsetopt extendedglob
echo "c=$options[extendedglob]"`)
	want := "n=197\na=off\nb=on\nc=off\n"
	if out != want || st != 0 {
		t.Errorf("$options across two mutations = %q (status %d), want %q", out, st, want)
	}
}

// The twelve compat spellings are keys of their own, negated where the two
// names mean opposite states — which is what makes 197 rather than the 185
// the listings print.
func TestOptionsCarriesTheCompatSpellingsAsKeys(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`echo "[$options[log]] [$options[histnofunctions]]"
setopt histnofunctions
echo "[$options[log]] [$options[histnofunctions]]"`)
	want := "[on] [off]\n[off] [on]\n"
	if out != want || st != 0 {
		t.Errorf("a compat spelling = %q (status %d), want %q", out, st, want)
	}
}

// Assigning is `setopt` written as an assignment, with zsh's two measured
// complaints — both of which leave the *command's* status at 0.
func TestOptionsIsWrittenThroughToTheOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `options[extendedglob]=on
[[ -o extendedglob ]] && echo on-now
options[nosuchopt]=on 2>&1
echo "unknown=$?"
options[extendedglob]=maybe 2>&1
echo "bad=$? still=[$options[extendedglob]]"`)
	want := "on-now\nzsh:3: no such option: nosuchopt\nunknown=0\n" +
		"zsh:5: invalid value: maybe\nbad=0 still=[on]\n"
	if out != want || st != 0 {
		t.Errorf("writing $options = %q (status %d), want %q", out, st, want)
	}
}

// $builtins is a view of the builtin table, and a builtin switched off is not
// a key here — measured, the count drops and the key goes.
func TestBuiltinsIsAViewAndDropsOneThatIsSwitchedOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `echo "a=[$builtins[cd]]"
disable cd
echo "b=[${builtins[cd]:-ABSENT}]"
enable cd
echo "c=[$builtins[cd]]"`)
	want := "a=[defined]\nb=[ABSENT]\nc=[defined]\n"
	if out != want || st != 0 {
		t.Errorf("$builtins across two mutations = %q (status %d), want %q", out, st, want)
	}
}

// And it is readonly, which is zsh's own answer — a produced association with
// no writer would otherwise take the assignment into a stored table and shadow
// itself from then on.
func TestBuiltinsIsReadonly(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `builtins[x]=y 2>&1; echo "st=$?"`)
	want := "zsh:1: read-only variable: builtins\n"
	if out != want || st != 1 {
		t.Errorf("writing $builtins = %q (status %d), want %q at 1", out, st, want)
	}
}

// $aliases is a view of the alias table, written through in both directions.
func TestAliasesIsAViewOfTheAliasTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `echo "a=[${aliases[q]:-ABSENT}] n=${#aliases}"
alias q=ls
echo "b=[$aliases[q]] n=${#aliases}"
unalias q
echo "c=[${aliases[q]:-ABSENT}] n=${#aliases}"
aliases[zz]="echo z"
echo "d=[$(alias zz)] n=${#aliases}"
unset "aliases[zz]"
echo "e=[${aliases[zz]:-ABSENT}] n=${#aliases}"`)
	want := "a=[ABSENT] n=0\nb=[ls] n=1\nc=[ABSENT] n=0\n" +
		"d=[zz='echo z'] n=1\ne=[ABSENT] n=0\n"
	if out != want || st != 0 {
		t.Errorf("$aliases across four mutations = %q (status %d), want %q", out, st, want)
	}
}

// $commands is a view of the PATH search, which is what makes it move when
// PATH does — the property a hash table filled in once would not have.
func TestCommandsIsAViewOfThePathSearch(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `echo "absent=[${commands[definitelynotacommand]:-ABSENT}]"
echo "a=[${commands[ls]:-ABSENT}]"
PATH=/nonexistent
echo "b=[${commands[ls]:-ABSENT}]"
PATH=/bin
echo "c=[${commands[ls]:-ABSENT}]"`)
	want := "absent=[ABSENT]\na=[ABSENT]\nb=[ABSENT]\nc=[/bin/ls]\n"
	if out != want || st != 0 {
		t.Errorf("$commands across two PATH changes = %q (status %d), want %q", out, st, want)
	}
}

// Writing to it **hashes a command**, which is what makes this a view in both
// directions rather than a read that happens to agree with one.
//
// Three things in one run, and the third is the one a refusal could not have
// given: the entry is what the parameter reads back, it is what `hash` lists,
// and it is what the *name itself* runs. Measured on zsh 5.9.2 with
// `PATH=/usr/bin:/bin` — `commands[zz]=/bin/echo; zz hi` prints `hi`.
func TestWritingCommandsHashesTheCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "realcmd"),
		[]byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"),
		[]byte("#!/bin/sh\necho other\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `commands[myc]=`+filepath.Join(dir, "other")+`
echo "then=[${commands[myc]:-ABSENT}]"
hash
myc`)
	want := "then=[" + filepath.Join(dir, "other") + "]\n" +
		"myc=" + filepath.Join(dir, "other") + "\n" +
		"other\n"
	if out != want || st != 0 {
		t.Errorf("writing $commands = %q (status %d), want %q", out, st, want)
	}
}

// And an entry a script wrote wins over what the search would have found, in
// every one of the three readings — which is the row that says the table is on
// *top* of the PATH scan and not behind it. Measured: `commands[ls]=/bin/echo;
// ls WOW` prints `WOW` there.
func TestAWrittenCommandsEntryWinsOverThePathSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "realcmd"),
		[]byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"),
		[]byte("#!/bin/sh\necho other\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `echo "before=[${commands[realcmd]}]"
commands[realcmd]=`+filepath.Join(dir, "other")+`
echo "after=[${commands[realcmd]}]"
realcmd`)
	want := "before=[" + filepath.Join(dir, "realcmd") + "]\n" +
		"after=[" + filepath.Join(dir, "other") + "]\n" +
		"other\n"
	if out != want || st != 0 {
		t.Errorf("$commands over a real PATH entry = %q (status %d), want %q", out, st, want)
	}
}

// The table is read **raw** rather than through the lookup: an entry that does
// not run is still what the parameter reports. Measured —
// `commands[qq]=/no/such/thing` leaves `${commands[qq]}` as the path it was
// given and `${+commands[qq]}` as 1, with `qq` itself `command not found`.
//
// It is the discriminating row for the read, because a lookup-backed read
// would answer empty for the very entry the line before it wrote.
func TestCommandsReportsAnEntryThatWillNotRun(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `commands[qq]=/no/such/thing
echo "v=[${commands[qq]}] plus=${+commands[qq]}"`)
	want := "v=[/no/such/thing] plus=1\n"
	if out != want || st != 0 {
		t.Errorf("a stale $commands entry = %q (status %d), want %q", out, st, want)
	}
}

// The **whole-table** reading carries the hash too, which is a separate
// producer from the one-key lookup above and the half a test of
// `${commands[c]}` alone cannot reach.
//
// Two rows, because the hash sits on top of the PATH scan in both of them and
// they fail differently: a name PATH never had is a key the enumeration gains,
// and a name PATH does have is a key whose *value* the hash replaces without
// changing the count. Measured on zsh 5.9.2 with `PATH=/usr/bin:/bin` —
// `commands[zz]=/bin/echo` puts `zz` in `${(k)commands}` beside everything the
// scan found, and `commands[ls]=/bin/echo` leaves the count alone and makes
// `${(v)commands}` say `/bin/echo` for it.
//
// Both readings have to agree about this: the contract
// `interp.Runner.SetDynamicAssocElement` states allows a view to read more
// than it lists and never less, so a table that listed the scan alone while
// the lookup answered the hash would be the one shape it forbids — a
// `${(k)commands}` that names fewer commands than `${+commands[c]}` admits to.
func TestTheWholeCommandsTableCarriesTheHash(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "realcmd"),
		[]byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `echo "n=${#commands} k=[${(ok)commands}]"
commands[zzz]=/zzz/zzz
echo "n=${#commands} k=[${(ok)commands}]"
commands[realcmd]=/realcmd/other
echo "n=${#commands} v=[${(o)commands}]"`)
	// The two paths are named so that sorting them by value and listing them
	// in key order give the same line, which keeps this row about the hash
	// rather than about what `(o)` orders.
	want := "n=1 k=[realcmd]\n" +
		"n=2 k=[realcmd zzz]\n" +
		"n=2 v=[/realcmd/other /zzz/zzz]\n"
	if out != want || st != 0 {
		t.Errorf("the whole $commands table = %q (status %d), want %q", out, st, want)
	}
}

// A produced association is never replaced by a stored one, whichever route
// the write comes by. Either of these would otherwise put an empty table in
// front of the producer, and every read afterwards would be a plausible answer
// about a shell that had stopped being watched.
//
// The whole-table assignment used to be refused here for exactly that fear,
// and the refusal was the wrong half to keep: measured on zsh 5.9.2, the same
// five lines answer `whole=0 [\t:] n=2` and `other` then runs. So the write
// goes through the producer's own writer, one key at a time, and `f` survives
// it — which is the no-shadowing property this test is about, now shown by a
// table that grew rather than by one that refused to.
func TestAProducedAssociationCannotBeShadowed(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ :; }
typeset -A functions
echo "declared=[${functions[f]:-ABSENT}] n=${#functions}"
functions=(other "echo x") 2>&1
echo "whole=$? [${functions[f]:-ABSENT}] n=${#functions}"
other`)
	want := "declared=[\t:] n=1\n" +
		"whole=0 [\t:] n=2\n" +
		"x\n"
	if out != want || st != 0 {
		t.Errorf("shadowing attempts = %q (status %d), want %q", out, st, want)
	}
}

// A subshell reads its own state through the *same producers*, which is what
// makes a clone's view its own rather than the parent's: the producer is
// handed whichever runner is asking.
//
// What it shares with the parent is the function table itself, and that is a
// substrate gap this change did not introduce and does not fix: measured,
// `(g(){ :; }); type g` finds `g` here and `g not found` in real zsh, and the
// same is true of an alias. So the assertion below is about the view being
// read through — the count moves inside — and not about isolation, which is
// filed separately.
func TestAProducedAssociationIsReadThroughInsideASubshell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ :; }
(echo "before=${#functions}"; g(){ :; }; echo "after=${#functions}")`)
	want := "before=1\nafter=2\n"
	if out != want || st != 0 {
		t.Errorf("a subshell's view = %q (status %d), want %q", out, st, want)
	}
}

// **An absent parameter refuses by name on every route that reads one**, and
// the routes are the point: a test of `$jobstates` alone passes against a
// shell that loses `${jobstates[x]}`, which is the spelling a plugin manager
// actually writes — `${functions[name]}` is 57 of zinit's uses.
//
// Each row asserts two things and needs both. That the parameter is *named*,
// because a status alone passes against the exact bug this exists to prevent:
// a shell that expanded to nothing and set 1 would satisfy a status check and
// still be handing a caller an empty value. And that nothing was written on
// standard output, because "reads as empty" is precisely what an unnamed
// absence looks like from the caller's side.
func TestAnAbsentParameterRefusesByNameOnEveryReadRoute(t *testing.T) {
	for _, tc := range []struct{ name, snippet string }{
		{"a bare name", `print -r -- "[$jobstates]"`},
		{"a subscript", `print -r -- "[${jobstates[running]}]"`},
		{"the whole array", `print -r -- "[${jobstates[@]}]"`},
		{"a length", `print -r -- "[${#jobstates}]"`},
		{"a flag group", `print -r -- "[${(k)jobstates}]"`},
		{"an unquoted word", `print -r -- ${jobstates[x]}`},
		{"a condition", `[[ -n $jobstates ]]`},
		// A pattern rather than a value, which is where an empty read is
		// least visible of all: an unrefused one matches nothing, takes
		// the `*` branch, and looks exactly like a script whose input did
		// not match.
		{"a case pattern", `case x in $jobstates) print -r -- matched;; *) print -r -- fell-through;; esac`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.snippet+"\nprint -r -- UNREACHED")
			if !strings.Contains(out, "jobstates: parameter not implemented yet") {
				t.Errorf("%s = %q (status %d), want the parameter named", tc.snippet, out, st)
			}
			if strings.Contains(out, "[") || strings.Contains(out, "UNREACHED") {
				t.Errorf("%s = %q, want no value handed to the caller", tc.snippet, out)
			}
			if st == 0 {
				t.Errorf("%s = status 0, want a failure", tc.snippet)
			}
		})
	}
}

// **A here-document refuses by name**, and it does so for an ordinary unset
// name under `set -u` in exactly the same way.
//
// It is the route that proves the refusal is asked in *two* places and not
// one. Every spelling in the table above is answered by the list path, which
// reaches its own test; a here-document body is expanded span by span down
// the scalar path alone, and a mutant that deleted the scalar site survived
// every other test in this file and failed only this one.
//
// Asserted as a *parity* rather than as a behavior worth having, which is the
// only honest way to write it down: this is the substrate's here-document
// route and not this parameter's, and a refusal that behaved differently here
// would be a second answer to a question already answered. The diagnostic
// still names the parameter, which is the part this change is responsible for.
//
// The parity used to be the *wrong* one and said so: the body was expanded in
// this shell as the redirection was arranged, so both spellings reported and
// then reached the command anyway — run with `cat` on PATH, both printed `[]`.
// That was the substrate's here-document route, and #1157 fixed it. Both
// spellings now stop at the diagnostic, because a body fed to a program is
// expanded in that program's process and a failure there costs the command.
func TestAHereDocumentNamesAnAbsentParameterTheWayItNamesAnUnsetOne(t *testing.T) {
	absent, ast := runZsh(t, t.TempDir(), "cat <<E\n[$jobstates]\nE\n")
	unset, ust := runZsh(t, t.TempDir(), "set -u\ncat <<E\n[$nosuchvar]\nE\n")
	if !strings.Contains(absent, "jobstates: parameter not implemented yet") {
		t.Errorf("a here-document reading an absent parameter = %q (status %d), want it named", absent, ast)
	}
	if !strings.Contains(unset, "nosuchvar: parameter not set") {
		t.Errorf("a here-document reading an unset name under set -u = %q (status %d), want it named", unset, ust)
	}
	// And the parity itself: in neither is the command the here-document was
	// for reached, because the body could not be expanded and the process it
	// was being expanded for is not this shell. `cat` is not on this test's
	// PATH, so the shell's own complaint about it is what would say it had
	// been tried — and neither spelling produces one.
	if strings.Contains(absent, "command not found: cat") ||
		strings.Contains(unset, "command not found: cat") {
		t.Errorf("the command was reached: absent %q, unset %q", absent, unset)
	}
}

// Arithmetic is deliberately left alone, and that is measured rather than
// overlooked: `$(( jobstates + 1 ))` in a real zsh with the module loaded is
// `1`, because an association read as a number is 0 there. A refusal here
// would be this shell inventing a diagnostic the shell it models does not
// write, over a spelling that means nothing in either.
func TestArithmeticReadsAnAbsentParameterAsZeroLikeTheRealThing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "n=$(( jobstates + 1 ))"`)
	if want := "n=1\n"; out != want || st != 0 {
		t.Errorf("arithmetic on an absent parameter = %q (status %d), want %q", out, st, want)
	}
}

// The refusal is about reading something absent and not about owning a
// spelling: a script that gave the name a value of its own gets it back, the
// way it would in a shell where the module was never loaded. And the four
// conditional operators are a script saying what to do when there is no value,
// which is the same exemption `set -u` makes — being told is what they are
// for, and `${p+x}` answering "no" is an answer where `${p}` answering empty
// is not.
func TestAnAbsentParameterYieldsToTheScriptsOwnAnswer(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "default=[${jobstates-d}]"
print -r -- "alternate=[${jobstates+set}]"
dirstack=(a b)
print -r -- "own=[$dirstack] [${dirstack[1]}] [${#dirstack}]"`)
	want := "default=[d]\nalternate=[]\nown=[a b] [a] [2]\n"
	if out != want || st != 0 {
		t.Errorf("a script's own answers = %q (status %d), want %q", out, st, want)
	}
}

// `dirstack` rather than `jobstates` for the owning half, because owning is
// exactly what the rest of them no longer allow: fifteen of the sixteen
// absent names are frozen against a write here as they are in zsh, and
// `dirstack` is the one the shell being modeled lets a script assign — it is
// the directory stack, and setting one is what the assignment is for. So the
// exemption is still reachable and this is where it is reached.
func TestAnAbsentParameterRefusesAWriteAsAReadOnlyName(t *testing.T) {
	for _, src := range []string{
		`jobstates=(a b c)`,
		`jobstates[1]=q`,
		`typeset -g jobstates=(a)`,
	} {
		out, st := runZsh(t, t.TempDir(), src+"\necho unreached")
		if st == 0 || !strings.Contains(out, "read-only variable: jobstates") ||
			strings.Contains(out, "unreached") {
			t.Errorf("%s = %q (status %d), want the read-only refusal and the script ended", src, out, st)
		}
	}
}

// And `unset` of one is *not* refused, which is measured rather than tidy:
// the shell being modeled answers `unset jobstates` with a silent 0 on the
// line after refusing `jobstates=(a b c)` as read-only. Two answers from one
// attribute, so the exemption is written down where the difference is.
func TestAnAbsentParameterMayStillBeUnset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "unset jobstates\necho \"st=$?\"")
	if out != "st=0\n" || st != 0 {
		t.Errorf("unsetting an absent parameter = %q (status %d), want a silent 0", out, st)
	}
}

// **The seven that are empty read empty and say nothing**, and that is an
// answer rather than a stub: each reports on something this shell cannot do,
// so "none" is true. A real zsh with none of them defined says the same.
func TestTheEmptyParametersReadEmptyAndSayNothing(t *testing.T) {
	for _, name := range emptyModuleParams() {
		t.Run(name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(),
				`print -r -- "n=${#`+name+`} one=[${`+name+`[x]}]"`)
			want := "n=0 one=[]\n"
			if out != want || st != 0 {
				t.Errorf("$%s = %q (status %d), want %q", name, out, st, want)
			}
		})
	}
}

// **And they stay honest.** Each of the ten is empty *because* something else
// refuses, and this runs that something else and requires it to still refuse.
//
// Not a restatement of the implementation. It is the alarm: the day `alias -g`
// works there are global aliases, `$galiases` is silently wrong, and nothing
// in the parameter itself would have noticed — an empty association is exactly
// as plausible then as it is now. This fails instead, at the name of the
// parameter that has to move with it (#1137).
func TestTheEmptyParametersStayHonest(t *testing.T) {
	for _, tc := range []struct{ param, waitsFor string }{
		{"dis_aliases", "disable -a nosuch"},
		{"dis_functions", "disable -f nosuch"},
		{"dis_functions_source", "disable -f nosuch"},
		{"dis_galiases", "disable -a nosuch"},
		{"dis_patchars", "disable -p nosuch"},
		{"dis_reswords", "disable -r nosuch"},
		{"dis_saliases", "disable -s nosuch"},
		// `nameddirs` was the eighth row and has left this list: `hash -d`
		// works now, so the table it reports on exists and the parameter is
		// a view over it rather than an honest emptiness. That is the alarm
		// firing and being answered rather than silenced — see
		// TestTheNamedDirectoryParameterIsAViewOfTheTable (#2191).
	} {
		t.Run(tc.param, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.waitsFor+` 2>&1
print -r -- "st=$?"`)
			if strings.Contains(out, "st=0") {
				t.Errorf("`%s` succeeded (%q, status %d) — so $%s is no longer honestly empty and needs implementing",
					tc.waitsFor, out, st, tc.param)
			}
			if !strings.Contains(out, "not implemented yet") && !strings.Contains(out, "bad option") {
				t.Errorf("`%s` = %q, want a refusal that names the letter", tc.waitsFor, out)
			}
		})
	}
}

// A write to one of the still-empty tables is refused by the name of the
// letter that is missing, which is a to-do rather than a wall — and it is also what keeps the
// view a view. A produced association with no writer takes the assignment into
// a stored table, and a stored table is what a read finds first, so one
// `dis_aliases[x]=ls` would freeze the parameter at that instant with
// nothing said at either end. The three zsh marks readonly refuse in zsh's own words
// instead, which does the same job.
func TestWritingToAnEmptyParameterIsRefusedByName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `dis_aliases[x]=ls 2>&1
print -r -- "after=${#dis_aliases}"
dis_functions[f]=x 2>&1
print -r -- "after=${#dis_functions}"
dis_reswords=(x) 2>&1
print -r -- "unreached"`)
	want := "zsh:1: dis_aliases[x]: disable -a is not implemented yet\nafter=0\n" +
		"zsh:3: dis_functions[f]: disable -f is not implemented yet\nafter=0\n" +
		"zsh:5: read-only variable: dis_reswords\n"
	if out != want || st != 1 {
		t.Errorf("writes to the still-empty tables = %q (status %d), want %q", out, st, want)
	}
}

// **`$aliases`, `$galiases` and `$saliases` are three sets, not one table
// read three ways.** Measured on zsh 5.9.2: `alias -g G=x; alias r=y; alias
// -s t=z` leaves `${(k)aliases}` naming `r` alone.
//
// A test of its own rather than a line in the listing tests, because these
// are the *parameter* surface and nothing else grades it: the builtin's
// listings could be perfectly split while `$aliases` handed a script every
// kind at once, which is what #2081's first pass did (the alarm above caught
// the letters, not this).
func TestTheThreeAliasParametersAreThreeSets(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unalias -a
alias -g G=x; alias r=y; alias -s t=z
print -r -- "aliases=${(k)aliases}"
print -r -- "galiases=${(k)galiases}"
print -r -- "saliases=${(k)saliases}"`)
	want := "aliases=r\ngaliases=G\nsaliases=t\n"
	if out != want || st != 0 {
		t.Errorf("the three parameters = %q (status %d), want %q", out, st, want)
	}
}

// **And each writes the kind it reads.** `galiases[G]=…` is `alias -g G=…`
// and `saliases[t]=…` is `alias -s t=…`, which is what makes them the
// parameter form of the builtin rather than three views of one map.
func TestWritingTheAliasParametersDefinesThatKind(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unalias -a
galiases[G]=x
saliases[t]=z
print -r -- "--plain--"; alias
print -r -- "--g--"; alias -g
print -r -- "--s--"; alias -s
unset "galiases[G]"
print -r -- "after=${#galiases}"`)
	want := "--plain--\nG=x\n--g--\nG=x\n--s--\nt=z\nafter=0\n"
	if out != want || st != 0 {
		t.Errorf("writing through the parameters = %q (status %d), want %q", out, st, want)
	}
}

// emptyModuleParams is the seven of `zsh/parameter` that are empty here and
// right to be. Spelled out rather than read from the package, because a test
// that asked the implementation which parameters it thought were empty would
// agree with it whatever it said.
//
// It was ten. `galiases` and `saliases` left the list when `alias -g` and
// `alias -s` arrived (#2081), which is exactly what the alarm below was for:
// they are produced tables now and TestTheGlobalAndSuffixTablesAreLive is
// what grades them. The prose said eight for as long as the list held seven,
// which is the count going stale in the one place nothing reads it —
// TestTheThreeRostersAccountForTheModuleExactlyOnce counts the entries and
// never the sentence.
func emptyModuleParams() []string {
	return []string{
		"dis_aliases", "dis_functions", "dis_functions_source", "dis_galiases",
		"dis_patchars", "dis_reswords", "dis_saliases",
	}
}

// None of the five is a `set` listing's business, and that is not cosmetic:
// a listing is a capture surface an agent harness sources back, and five
// generated tables in it would hand the next shell every builtin, every
// option and every command on PATH as assignments somebody had written.
// zsh hides them for the same reason; `builtins` appears as a bare name
// because it is readonly, and carries no value.
func TestTheProducedParametersStayOutOfASetListing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ :; }; alias a=b; set`)
	for _, name := range []string{"functions=", "options=", "commands=", "aliases=", "builtins="} {
		if strings.Contains(out, name) {
			t.Errorf("`set` wrote %s into the listing: %q", name, out)
		}
	}
	if !strings.Contains(out, "\nbuiltins\n") {
		t.Errorf("`set` = %q, want the bare name `builtins` in it", out)
	}
	if st != 0 {
		t.Errorf("`set` status %d, want 0", st)
	}
}

// **Each of the ten refuses by name**, one name at a time. (The sentence said
// fourteen while the roster held eleven, and now holds ten; the roster is
// what this loops over, which is the point of reading it from the list.)
//
// The eight read routes are graded against `jobstates` alone above, which is
// right for the routes — they are a property of the expansion and not of the
// name. This is the other axis and it was missing: nothing named the roster,
// so deleting an entry from it left every test in this package green and left
// that parameter reading `0` at status 0, which is the exact failure the whole
// of #1146 and #1152 exists to prevent. Present on the absent side, in the
// mechanism written to close it (#1137).
//
// It is also where the roster shrinks. The day `$modules` is a live view, this
// fails at the name until it is moved out of absentModuleParams and into
// implementedModuleParams, which is a great deal better than nobody noticing
// that a refusal is still registered over something that now works.
func TestEveryAbsentParameterRefusesByName(t *testing.T) {
	for _, name := range absentModuleParams() {
		t.Run(name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `print -r -- "n=${#`+name+`}"`)
			want := "zsh:1: " + name + ": parameter not implemented yet\n"
			if out != want || st != 1 {
				t.Errorf("$%s = %q (status %d), want %q", name, out, st, want)
			}
		})
	}
}

// **And the three rosters account for the module, exactly once each.**
//
// `zsh/parameter` names thirty-three parameters and the gate opens only when
// every one of them is implemented or refuses by name. That is held by
// zmodloadHolds already. What was not held is the *classification*: a name
// could be in two rosters, or in none, and the shell would still load the
// module — one of the two answers would simply win and nothing would say
// which. A parameter promoted from absent to implemented and left in both
// lists is the shape that costs the most, because the refusal is registered
// after the view and would take it back.
//
// All four lists are spelled out rather than read from the package, for the
// reason emptyModuleParams gives: a census that asked the implementation what
// it thought would agree with it whatever it said. Their being separate lists
// is what makes this an assertion.
func TestTheThreeRostersAccountForTheModuleExactlyOnce(t *testing.T) {
	seen := map[string]string{}
	for roster, names := range map[string][]string{
		"implemented": implementedModuleParams(),
		"empty":       emptyModuleParams(),
		"absent":      absentModuleParams(),
	} {
		for _, name := range names {
			if other, dup := seen[name]; dup {
				t.Errorf("$%s is in both the %s and the %s roster; the later registration would take the earlier one back", name, other, roster)
				continue
			}
			seen[name] = roster
		}
	}
	for _, name := range moduleParams() {
		if _, ok := seen[name]; !ok {
			t.Errorf("$%s is named by zsh/parameter and is in no roster, so nothing says what a read of it does", name)
		}
		delete(seen, name)
	}
	for name, roster := range seen {
		t.Errorf("$%s is in the %s roster and is not named by zsh/parameter", name, roster)
	}
	if got, want := len(moduleParams()), 33; got != want {
		t.Errorf("zsh/parameter names %d parameters here, want %d — measured against zsh 5.9.2 with `zmodload -lF zsh/parameter`", got, want)
	}
}

// `unset "options[name]"` turns the option off. Not a guess and not a
// refusal — this was written as one, saying an unset "has nothing to mean",
// and #1527's measurements say otherwise.
//
// Three probes rather than one, because a status and a view agree between the
// three answers that were on the table. "The option moved" and "the key was
// dropped and the view reports off for a missing key" both read `off`, so the
// first probe is a *behavior*: `noclobber` on, then unset, then a `>` that
// would have been refused. And "moved off" and "put back to its default" both
// read `off` for an option that starts off, so the second probe uses `equals`,
// which is **on** in a fresh shell — it goes off, so it is off and not the
// default.
func TestUnsettingAnOptionsElementTurnsTheOptionOff(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt noclobber
: > f
echo one > f 2>&1
echo "refused=$?"
unset "options[noclobber]"
echo "st=$? view=[$options[clobber]]"
echo two > f
read line < f
echo "wrote=[$line]"
echo "equals-before=[$options[equals]]"
unset "options[equals]"
echo "equals-after=[$options[equals]]"`)
	want := "zsh:3: file exists: f\nrefused=1\nst=0 view=[on]\nwrote=[two]\n" +
		"equals-before=[on]\nequals-after=[off]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting an $options element = %q (status %d), want %q", out, st, want)
	}
}

// A name nobody has is the one thing the unset does not share with the
// assignment: silent here, `no such option` there. Measured both ways in the
// same shell, so a single rule for the two spellings fails this test.
func TestUnsettingAnUnknownOptionsElementIsSilent(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unset "options[nosuchopt]" 2>&1
echo "unset=$?"
options[nosuchopt]=on 2>&1
echo "assign=$?"`)
	want := "unset=0\nzsh:3: no such option: nosuchopt\nassign=0\n"
	if out != want || st != 0 {
		t.Errorf("an unknown $options key = %q (status %d), want %q", out, st, want)
	}
}

// A compat spelling is folded and negated through the unset as it is through
// the assignment — `unset "options[hashall]"` is `hashcmds` off — which is
// what shows the lookup was not written a second time here.
func TestUnsettingACompatSpellingGoesThroughTheCanonicalOption(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt hashall
echo "a=[$options[hashall]] [$options[hashcmds]]"
unset "options[HASH_ALL]"
echo "b=[$options[hashall]] [$options[hashcmds]]"`)
	want := "a=[on] [on]\nb=[off] [off]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting a compat spelling = %q (status %d), want %q", out, st, want)
	}
}

// An element of one of the still-empty tables is *silent* on an unset where
// the assignment refuses by name. The assignment's caller believes it has
// arranged a disabled alias and would be told nothing at either end; an unset's caller
// asked for absence and has it, and reads the same empty string here that zsh
// reads. Both halves in one shell, because the point is that they differ.
func TestUnsettingAnEmptyParametersElementIsSilent(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unset "dis_aliases[f]" 2>&1
echo "unset=$? n=${#dis_aliases} v=[${dis_aliases[f]}]"
unset "nameddirs[x]" 2>&1
echo "nameddirs=$?"
dis_aliases[f]=ls 2>&1
echo "assign=$?"`)
	want := "unset=0 n=0 v=[]\nnameddirs=0\n" +
		"zsh:5: dis_aliases[f]: disable -a is not implemented yet\nassign=0\n"
	if out != want || st != 0 {
		t.Errorf("unsetting an empty table's element = %q (status %d), want %q", out, st, want)
	}
}

// `$commands` is the table where the silence above would be wrong in the
// other direction, and this is where the four answers part: an `unset` of one
// element **forgets a hashed command**. Measured 2026-09-12 — `echo
// ${#commands}; unset "commands[ls]"; hash` leaves `ls` out of the listing in
// zsh 5.9.2, where bash's `unset "BASH_CMDS[q]"` leaves its entry alone.
//
// The row that makes it a removal rather than a word taken and dropped is the
// one in the middle: the name the write put in the table is gone from `hash`
// afterwards, and the read falls back to what the search finds. A name PATH
// still resolves comes back on the next read, which is what the shell being
// modeled does too — there because every touch of the parameter refills the
// table, here because the search is the other half of the view.
func TestUnsettingACommandsElementForgetsTheHashedCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "toolx"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `commands[zzonly]=/bin/zzonly
echo "hashed=[${commands[zzonly]}]"
unset "commands[zzonly]"
echo "st=$? gone=[${commands[zzonly]:-ABSENT}]"
echo "table=[$(hash)]"
unset "commands[toolx]"
echo "onpath=[${commands[toolx]:+found}]"`)
	want := "hashed=[/bin/zzonly]\n" +
		"st=0 gone=[ABSENT]\ntable=[]\n" +
		"onpath=[found]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting a $commands element = %q (status %d), want %q", out, st, want)
	}
}

// An element of a parameter this shell has not got refuses by name, in the
// sentence a *read* of it gives — one table, one sentence.
//
// The two keys are the pair that used to split: `parameters[PATH]` complained
// `invalid number:` and then the whole of PATH, because the brackets of a name
// holding nothing are read as arithmetic and PATH's value is not a number; and
// `parameters[nope]` evaluated to 0 and was silent at status 0, which is the
// answer this mechanism exists to stop.
func TestUnsettingAnAbsentParametersElementRefusesByName(t *testing.T) {
	// `jobstates` rather than `parameters`, which this used to ask about:
	// that one is a live table now (#1599), so it is no longer an example of
	// the thing being tested. The mechanism is unchanged and so is the
	// wording — only the parameter standing in for "absent" moved.
	out, st := runZsh(t, t.TempDir(), `unset "jobstates[PATH]" 2>&1
echo "known=$?"
unset "jobstates[nope]" 2>&1
echo "unknown=$?"
unset "jobtexts[1]" 2>&1
echo "other=$?"`)
	want := "zsh:unset:1: jobstates: parameter not implemented yet\nknown=1\n" +
		"zsh:unset:3: jobstates: parameter not implemented yet\nunknown=1\n" +
		"zsh:unset:5: jobtexts: parameter not implemented yet\nother=1\n"
	if out != want || st != 0 {
		t.Errorf("unsetting an absent parameter's element = %q (status %d), want %q", out, st, want)
	}
}

// And a name the script gave a value of its own is the script's, on the unset
// as on the read: the refusal is about reading something absent, not about
// owning a spelling.
//
// `dirstack` rather than `jobstates`, which this used to use, and rather than
// `parameters` before that. Each move is the same correction one step
// further: a script cannot own a name the shell being modeled freezes, and
// since #1604 that is fifteen of the sixteen absent names here too. What is
// left is the one absent name zsh lets a script assign, which is where the
// exemption is still reachable.
//
// The element `unset` is this shell's answer and not zsh's, and there is
// nothing to compare it against: `unset "dirstack[2]"` takes zsh 5.9.2 down
// with SIGSEGV. absentparam.go records that these names have no coherent
// panel answer for an element write, which is why the rule here is the
// mechanism's own rather than a copy.
func TestUnsettingAnAbsentNameTheScriptOwnsIsTheScriptsArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `dirstack=(a b c)
unset "dirstack[2]" 2>&1
echo "st=$? v=[${dirstack[*]}]"`)
	want := "st=0 v=[a  c]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting an owned name's element = %q (status %d), want %q", out, st, want)
	}
}

// The line #1527 was filed about, kept as it is written in the wild: the whole
// body of one plugin manager's `.zi-set-m-func` is `noglob unset functions[m]`
// and it means to undefine `m`.
//
// The issue reported this as removing the function where zsh refuses, and the
// refusal is real but is not about `unset` on this table — see the note in
// parameter.go on the autoload stub. With the parameter materialized, which is
// the state of every shell that has read `$functions` at all, zsh removes the
// function at status 0 and so does this. The unquoted control is the row that
// makes the pair readable rather than a decoration: without it a fixed glob
// and a fixed `unset` are the same measurement. It never reaches `unset` at
// all — the pathname match finds nothing and ends the script, which is why the
// line after it is marked unreached and the status is 1.
func TestUnsettingFunctionsThroughNoglobUndefinesTheFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `m(){ echo hi; }
noglob unset functions[m]
echo "st=$? left=${#functions}"
m 2>&1
echo "call=$?"
n(){ echo hi; }
unset functions[n] 2>&1
echo "unreached kept=${#functions}"`)
	want := "st=0 left=0\nzsh:4: command not found: m\ncall=127\n" +
		"zsh:7: no matches found: functions[n]\n"
	if out != want || st != 1 {
		t.Errorf("the line in the wild = %q (status %d), want %q at 1", out, st, want)
	}
}

// The other subscript forms on `$functions` are a no-op at status 0, which is
// zsh's own answer with the parameter materialized — a key nobody has, an
// index, a range and a second subscript alike. Held here because the fix above
// could have been written as "any subscript removes something".
func TestOtherSubscriptFormsOnFunctionsRemoveNothing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `m(){ echo hi; }
for s in nope 1 1,2 '(r)m'; do
  unset "functions[$s]" 2>&1
  echo "$s=$? n=${#functions}"
done`)
	want := "nope=0 n=1\n1=0 n=1\n1,2=0 n=1\n(r)m=0 n=1\n"
	if out != want || st != 0 {
		t.Errorf("other subscripts on $functions = %q (status %d), want %q", out, st, want)
	}
}

// moduleParams is the thirty-three parameters `zsh/parameter` provides,
// measured 2026-09-07 as `zmodload -lF zsh/parameter` in zsh 5.9.2 and sorted.
func moduleParams() []string {
	return []string{
		"aliases", "builtins", "commands", "dirstack", "dis_aliases",
		"dis_builtins", "dis_functions", "dis_functions_source",
		"dis_galiases", "dis_patchars", "dis_reswords", "dis_saliases",
		"funcfiletrace", "funcsourcetrace", "funcstack", "functions",
		"functions_source", "functrace", "galiases", "history", "historywords",
		"jobdirs", "jobstates", "jobtexts", "modules", "nameddirs", "options",
		"parameters", "patchars", "reswords", "saliases", "userdirs",
		"usergroups",
	}
}

// implementedModuleParams is the sixteen this shell has: the five a real
// plugin manager reads and the only five it reads (#1060), `funcstack`, which
// the completion system touches on its fourth line (#1598), `galiases` and
// `saliases` for the two alias namespaces `alias -g` and `alias -s` brought
// (#2081), `parameters` for `${(t)name}` (#1599), and `reswords`, which a
// highlighter reads before it can tell a reserved word from a command
// (#2517).
//
// Fifteen of them are live views. `dirstack` is the sixteenth and is not one
// (#4592): real zsh lets a script assign to it and the assignment *is* the
// new directory stack, so this shell keeps it as an ordinary array that the
// prelude's `pushd`, `popd` and `dirs` maintain under that name — there is no
// private store for a view to read. That is also why `zmodloadHasFeature`
// needs a third arm for it: the runner cannot tell an array the shell
// maintains from an array a script wrote, so the dialect says which name it
// is, through Semantics.PushedDirectoriesParameter.
func implementedModuleParams() []string {
	return []string{
		"aliases", "builtins", "commands", "dirstack", "funcfiletrace",
		"funcsourcetrace", "funcstack", "functions", "functrace",
		"galiases", "history", "nameddirs", "options", "parameters",
		"reswords", "saliases",
	}
}

// absentModuleParams is the ten this shell has not got, each registered
// with [interp.Runner.SetAbsentParameter] so that reading one is refused at
// the expansion that asked (#1152).
//
// Every one of them is non-empty, or can be, in a shell that has it — so
// reading empty would be a claim and not an answer, which is what separates
// these from the seven above. Some are answerable from a table this shell
// already keeps and the rest need a seam that does not exist; #1137 is the
// survey.
//
// It was sixteen. `reswords` left when the reserved-word table was exposed
// (#2517), which is the shrink the comment on TestEveryAbsentParameterRefusesByName
// describes happening for real: the fact was already in the shell and the
// parameter was what was missing. `history` left the same way and for the
// same reason (#4408) — `fc` had kept the list all along — and it is the one
// that says what staying here costs, since zsh-autosuggestions reads it on
// every keystroke and the refusal was written over the line being typed.
// `functrace` left third (#4447): `$funcstack` already walked the stack it
// reports on, and a handler that reads it to say where it was entered from
// was stopped rather than left with a blank field. `funcfiletrace` and
// `funcsourcetrace` left with it (#4470, #4469) — the same walk asked for a
// different field, sharing its frame selection so the three can never report
// different lengths. `dirstack` left sixth (#4592), and it is the one that
// could never have been answered with an empty array: `pushd` and `dirs`
// already kept a stack, so an empty one here would have been a claim and not
// an answer. It is also the only one that stopped being a *view* on the way
// out — see implementedModuleParams.
func absentModuleParams() []string {
	return []string{
		"dis_builtins", "functions_source", "historywords",
		"jobdirs", "jobstates", "jobtexts", "modules",
		"patchars", "userdirs", "usergroups",
	}
}

// `${functions[f]}` and the whole table have to say the same thing about
// every name, because since #1806 they are two different routes: a key goes
// to zshFunctionValue and the table to zshFunctionsView, and the first exists
// only because running the second to answer one key was ten seconds of a real
// startup.
//
// Four names, each a shape where the two could drift: one defined here, one
// still waiting to be autoloaded — whose value is neither its listing nor its
// body — one that is the shell's own and so not in the listing at all, and
// one that is nothing. `pushd` is the shell's own — a function that exists
// and is deliberately not a key — so the prelude is loaded here; without it
// that row would be a second spelling of `nosuch` and would prove nothing.
// The comparison is against the table's own answer,
// fetched through `(k)`, so a change to either route that did not change the
// other fails this.
func TestTheKeyAndTheTableAgreeAboutEveryFunction(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `f(){ echo hi; }
autoload -Uz a1
for n in f a1 pushd nosuch; do
  key="${functions[$n]-UNSET}"
  table=UNSET
  for k in "${(@k)functions}"; do
    if [[ $k = $n ]]; then table="${functions[$k]}"; fi
  done
  if [[ $key = $table ]]; then echo "$n agree [$key]"; else echo "$n DIFFER key=[$key] table=[$table]"; fi
done`)
	want := "f agree [\techo hi]\n" +
		"a1 agree [builtin autoload -XU]\n" +
		"pushd agree [UNSET]\n" +
		"nosuch agree [UNSET]\n"
	if out != want || st != 0 {
		t.Errorf("key against table = %q (status %d), want %q", out, st, want)
	}
}

// The same agreement, for the three views that got a keyed reading after
// `$functions` did. Each is a shape where the key and the table are computed
// by different code — an index into the option tables, a PATH search, a
// parameter's attributes — so each can drift on its own.
//
// The names are chosen to cover what the keyed route has to get right rather
// than what is convenient: a compat spelling of an option, a command that is
// not on PATH and one whose name has a slash in it (which the table can have
// no key for however well it resolves), and a parameter of every container
// kind beside one the shell refuses by name.
func TestTheKeyAndTheTableAgreeAboutOptionsCommandsAndParameters(t *testing.T) {
	// The command PATH resolves has to be one this test made: the session
	// runs with no PATH and no external commands, which is what keeps it a
	// test about the shell.
	dir := t.TempDir()
	bin := filepath.Join(dir, "zzbin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bin, "zzcmd"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, st := runZshPrelude(t, dir, `typeset -A myassoc; myassoc[k]=v
myarr=(a b); myscalar=x; integer myint=3
PATH=`+bin+`
for p in options:extendedglob options:braceexpand options:nosuchopt \
         commands:zzcmd commands:zznosuchcommand commands:/bin/sh \
         parameters:myscalar parameters:myarr parameters:myassoc parameters:myint \
         parameters:options parameters:reswords parameters:zznosuchparam; do
  name=${p%%:*}; key=${p#*:}
  eval "keyval=\${${name}[\$key]-UNSET}"
  table=UNSET
  eval "for k in \"\${(@k)${name}}\"; do [[ \$k = \$key ]] && table=\${${name}[\$k]}; done"
  # The path a command resolves to is this test's temporary directory, so
  # the row shows the file's name; the comparison above is on the whole of
  # both answers.
  show=$keyval
  [[ $name = commands && $keyval != UNSET ]] && show=${keyval:t}
  if [[ $keyval = $table ]]; then print -r -- "$p agree [$show]"
  else print -r -- "$p DIFFER key=[$keyval] table=[$table]"; fi
done`)
	want := "options:extendedglob agree [off]\n" +
		"options:braceexpand agree [on]\n" +
		"options:nosuchopt agree [UNSET]\n" +
		"commands:zzcmd agree [zzcmd]\n" +
		"commands:zznosuchcommand agree [UNSET]\n" +
		"commands:/bin/sh agree [UNSET]\n" +
		"parameters:myscalar agree [scalar]\n" +
		"parameters:myarr agree [array]\n" +
		"parameters:myassoc agree [association]\n" +
		"parameters:myint agree [integer]\n" +
		"parameters:options agree [association-special]\n" +
		// `reswords` was chosen here as a parameter the shell refused by
		// name, and it is a produced readonly array since #2517 — which is
		// what the row now reads, and which still covers the case it was
		// picked for: `zznosuchparam` below is the refusing name.
		"parameters:reswords agree [array-readonly-hide-hideval-special]\n" +
		"parameters:zznosuchparam agree [UNSET]\n"
	if out != want || st != 0 {
		t.Errorf("key against table = %q (status %d), want %q", out, st, want)
	}
}

// `$nameddirs` is a view of the table `hash -d` writes, which is where it
// went when it stopped being honestly empty.
//
// Measured on zsh 5.9.2, 2026-09-14. It is a *view* rather than a stored
// table for the reason every other one in this file is: a stored table is
// what a read finds first, so a name given one has stopped answering for
// anything from that moment.
func TestTheNamedDirectoryParameterIsAViewOfTheTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"empty until something writes one", `print -r -- "n=${#nameddirs}"`, "n=0\n"},
		{
			"the builtin fills it",
			`hash -d a=/tmp; print -r -- "n=${#nameddirs} one=[$nameddirs[a]]"`,
			"n=1 one=[/tmp]\n",
		},
		{"and the parameter writes it", `nameddirs[x]=/tmp; print -r -- ~x`, "/tmp\n"},
		{"which the builtin then lists", `nameddirs[x]=/tmp; hash -d`, "x=/tmp\n"},
		{"the keys are the names", `hash -d b=/b a=/a; print -r -- ${(k)nameddirs}`, "a b\n"},
		// An element unset *removes* the entry, measured on zsh 5.9.2 and on
		// zsh 5.9 alike. This row used to want `n=1` on the reading that zsh
		// refuses the subscript and changes nothing — and the `2>/dev/null`
		// it kept is the tell, because a probe that throws the diagnostic
		// away cannot see that there was never one to throw. zsh is silent
		// here at status 0 and the entry is gone; the shell was accepting the
		// unset and leaving `~a` pointing at `/tmp`, which is #3092's silent
		// wrong answer one name over.
		{
			"an element unset removes it",
			`hash -d a=/tmp; unset "nameddirs[a]"; print -r -- "n=${#nameddirs}"`,
			"n=0\n",
		},
		{
			"and the builtin no longer lists it",
			`hash -d a=/tmp b=/b; unset "nameddirs[a]"; hash -d`,
			"b=/b\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A *whole-table* assignment to a produced association is the write #3092 was
// filed about. It used to be refused by name and the status stayed 0, so a
// script that rewired a command read back the PATH search and was told
// nothing had gone wrong.
//
// Measured on zsh 5.9.2, 2026-09-16, and every row uses **two different
// keys**: one written the ordinary way, then a literal naming another, then
// the first asked for again. A literal naming the key the table already holds
// would give the same output whether the table was emptied first or merged
// into, so that probe cannot tell the two answers apart — and which of them
// each name gives is the only thing this test is about.
func TestAWholeTableAssignmentReachesTheProducer(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The four that merge. The first key survives.
		{
			"commands keeps what was there",
			`commands[cA]=/bin/echo
commands=(cB /bin/echo)
print -r -- "st=$? A=[$commands[cA]] B=[$commands[cB]]"`,
			"st=0 A=[/bin/echo] B=[/bin/echo]\n",
		},
		{
			// And the write is a real binding rather than a table entry: the
			// name is not on PATH, so a shell that stored the pair and looked
			// the word up afresh would answer `command not found`.
			"and the command it bound runs",
			`commands=(cB /bin/echo)
cB ran`,
			"ran\n",
		},
		{
			"functions keeps what was there",
			`fA() { print A }
functions=(fB "print B")
print -r -- "st=$?"
fA
fB`,
			"st=0\nA\nB\n",
		},
		{
			"and the keyed spelling of the literal reaches it too",
			`functions=([fC]="print C")
fC`,
			"C\n",
		},
		{
			"options keeps what was there",
			`setopt extendedglob
options=(nomatch on)
print -r -- "st=$? eg=[$options[extendedglob]] nm=[$options[nomatch]]"`,
			"st=0 eg=[on] nm=[on]\n",
		},
		// The four that empty the table first. The first key is gone.
		{
			"aliases empties first",
			`alias aA=x
aliases=(aB y)
print -r -- "st=$? A=[$aliases[aA]] B=[$aliases[aB]]"`,
			"st=0 A=[] B=[y]\n",
		},
		{
			"galiases empties first",
			`alias -g gA=x
galiases=(gB y)
print -r -- "st=$? A=[$galiases[gA]] B=[$galiases[gB]]"`,
			"st=0 A=[] B=[y]\n",
		},
		{
			"saliases empties first",
			`alias -s sA=x
saliases=(sB y)
print -r -- "st=$? A=[$saliases[sA]] B=[$saliases[sB]]"`,
			"st=0 A=[] B=[y]\n",
		},
		{
			"nameddirs empties first",
			`hash -d dA=/tmp
nameddirs=(dB /usr)
print -r -- "st=$? A=[$nameddirs[dA]] B=[$nameddirs[dB]]"`,
			"st=0 A=[] B=[/usr]\n",
		},
		// Emptying one kind of alias is not emptying the others: the three
		// parameters are three namespaces, and a clear that reached the
		// shared table would take the regular alias with it.
		{
			"and only its own namespace",
			`alias rA=x
alias -g gA=y
galiases=(gB z)
print -r -- "r=[$aliases[rA]] g=[$galiases[gA]] new=[$galiases[gB]]"`,
			"r=[x] g=[] new=[z]\n",
		},
		// The shape that reads most like "clear this" clears nothing. zsh's
		// parameter is handed no table at all by an empty literal and its set
		// function returns before touching anything — so this row is measured
		// rather than reasoned, and it is the one row that would have been
		// wrong had the emptying been written as an obvious rule.
		{
			"an empty literal empties nothing",
			`alias aA=x
aliases=()
print -r -- "st=$? A=[$aliases[aA]]"`,
			"st=0 A=[x]\n",
		},
		{
			"nor one that is empty only after expansion",
			`alias aA=x
e=()
aliases=($e)
print -r -- "st=$? A=[$aliases[aA]]"`,
			"st=0 A=[x]\n",
		},
		// A readonly produced table still refuses, which is the other of
		// zsh's two answers and the one that must not be lost to this: the
		// write is stopped with a sentence and a status, not accepted.
		{
			"a readonly produced table still refuses",
			`builtins=(b c) 2>&1
print -r -- "st=$?"`,
			"zsh:2: read-only variable: builtins\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), "zmodload zsh/parameter\n"+tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
