// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
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
func TestFunctionsLeavesOutTheFunctionsThatAreTheShell(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `mine(){ :; }
echo "listed=[${(k)functions}]"
type pushd
echo "declared=[$(declare -f)]"`)
	want := "listed=[mine]\npushd is a shell function from zsh\n" +
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
	want := "listed=[pushd]\nafter=[]\npushd is a shell function from zsh\nagain=[]\n"
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

// Writing to it is refused by name: this shell resolves a command word by
// searching PATH every time, so there is no hash for an entry to change.
// Accepting the assignment and dropping it would leave a caller holding a name
// it believes it has arranged for.
func TestCommandsRefusesAWriteByName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `commands[myc]=/bin/ls 2>&1
echo "then=[${commands[myc]:-ABSENT}]"`)
	want := "zsh:1: commands[myc]: assigning to the command hash is not implemented yet\n" +
		"then=[ABSENT]\n"
	if out != want || st != 0 {
		t.Errorf("writing $commands = %q (status %d), want %q", out, st, want)
	}
}

// A produced association is never replaced by a stored one, whichever route
// the write comes by. Both of these would otherwise put an empty table in
// front of the producer, and every read afterwards would be a plausible answer
// about a shell that had stopped being watched.
func TestAProducedAssociationCannotBeShadowed(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ :; }
typeset -A functions
echo "declared=[${functions[f]:-ABSENT}] n=${#functions}"
functions=(other "echo x") 2>&1
echo "whole=$? [${functions[f]:-ABSENT}] n=${#functions}"`)
	want := "declared=[\t:] n=1\n" +
		"zsh:4: functions: assigning to the whole of a produced association is not implemented yet\n" +
		"whole=0 [\t:] n=1\n"
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

// `zsh/parameter` still refuses, and the count is what says how far off it is:
// five of thirty-three exist and twenty-eight do not. A module must not load
// while a parameter it provides is missing, because an absent one reads empty
// at status 0 — which is the whole reason the rule is not the one builtins get.
func TestTheParameterModuleStillRefusesAndSaysHowFarOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter 2>&1
print -r -- "st=$?"`)
	want := "zsh:1: failed to load module `zsh/parameter': " +
		"28 of its 33 features are not implemented yet\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("zmodload zsh/parameter = %q (status %d), want %q", out, st, want)
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
