// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The autoloadable functions this shell **ships**, driven through the search
// that finds them — the files under share/sh/functions rather than a copy
// written out here, so what is graded is what an installation holds.
//
// Every expectation below is bytes recorded from zsh 5.9.2 on 2026-09-11,
// under "env -i HOME=... zsh -f" with FPATH naming a directory of function
// files, so no startup file is speaking. docs/spec/functions.md holds the
// method, the citations, and the three places this shell answers differently;
// where one of those is in a table here it is marked, with what zsh wrote.
//
// Nothing here reads zsh's own function files or its source, which
// CLEANROOM.md forbids: a probe calls the function and records what came back.

// shippedFunctionDir is where the function files this shell installs live in
// the checkout.
//
// Found from this file's own path rather than from the working directory,
// because "go test ./..." and "go test ./dialect/zsh" start in different
// places and a relative path would be right in one of them. The four names are
// checked rather than assumed: a test that silently graded an empty directory
// would pass every row below by finding no function to disagree with.
func shippedFunctionDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information: cannot find the shipped function files")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "share", "sh", "functions")
	for _, name := range []string{"add-zsh-hook", "colors", "is-at-least", "regexp-replace"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("shipped function %s: %v", name, err)
		}
	}
	return dir
}

// runShipped runs src with the shipped directory as the whole of $fpath.
func runShipped(t *testing.T, src string) (string, int) {
	t.Helper()
	return runZsh(t, t.TempDir(), "fpath=("+shippedFunctionDir(t)+")\n"+src)
}

// Four routes into the search, and they have to be one search. The file a
// call reads and the path a resolution records were two walks of $fpath until
// #1968 folded them, which is how #1704 and #1854 came to be the same defect
// on two roads — a fix on one spelling and a failure on the one a real startup
// file takes.
//
// So every route is driven against the same shipped file: a declaration with
// no letters at all, the "autoload -Uz" every real script writes, a call from
// inside another function, a resolution forced with "+X" before any call, and
// the same through "source". Each has to come back with the function working.
func TestEveryRouteIntoTheFunctionSearchFindsTheShippedFiles(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "sourced.zsh")
	if err := os.WriteFile(sourced, []byte("autoload -Uz is-at-least\nis-at-least 5.0 5.9 && print -r -- \"sourced ok\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, st := runShipped(t, `autoload is-at-least
is-at-least 5.0 5.9 && print -r -- "bare ok"
unfunction is-at-least

autoload -Uz is-at-least
is-at-least 5.0 5.9 && print -r -- "flagged ok"
unfunction is-at-least

caller() { autoload -Uz is-at-least; is-at-least 5.0 5.9 && print -r -- "in a function ok" }
caller
unfunction is-at-least

autoload -Uz +X is-at-least
print -r -- "resolved now: $(whence -w is-at-least)"
is-at-least 5.0 5.9 && print -r -- "resolved ok"
unfunction is-at-least

autoload -rUz is-at-least
is-at-least 5.0 5.9 && print -r -- "fixed path ok"
unfunction is-at-least

source `+sourced)
	want := "bare ok\nflagged ok\nin a function ok\n" +
		"resolved now: is-at-least: function\nresolved ok\n" +
		"fixed path ok\nsourced ok\n"
	if out != want || st != 0 {
		t.Errorf("the routes = %q (status %d), want %q", out, st, want)
	}
}

// The names are found with nothing but the shipped directory on $fpath, which
// is the arrangement an installation has. All four, because a payload that
// ships three of them is a startup file that still fails.
func TestAllFourShippedNamesResolve(t *testing.T) {
	out, st := runShipped(t, `autoload -Uz add-zsh-hook colors is-at-least regexp-replace
for name in add-zsh-hook colors is-at-least regexp-replace; do
	$name --nonsense-argument >/dev/null 2>&1
	if (( $? == 127 )); then print -r -- "$name NOT FOUND"; else print -r -- "$name resolved"; fi
done`)
	want := "add-zsh-hook resolved\ncolors resolved\nis-at-least resolved\nregexp-replace resolved\n"
	if out != want || st != 0 {
		t.Errorf("resolution = %q (status %d), want %q", out, st, want)
	}
}

// is-at-least, against the answers zsh 5.9.2 gives for the same pairs.
//
// The rows that matter are the ones a reading could get wrong: a suffix that
// is letters and digits in one segment is ignored ("2.6-beta3" equals "2.6")
// and a numeric segment of its own is not ("2.6-dev-3" is greater), a missing
// segment counts as zero so "5.0.0" and "5.0" are equal and "2.6-0" and "2.6"
// are too, segments compare as numbers rather than strings ("4.3.10" beats
// "4.3.9"), and an empty second argument takes the default exactly as a
// missing one does.
//
// The panel behind this is wider than the table: 1600 pairs, every zsh release
// spelling from 2.6-17 to 6.0 crossed with itself, agree in full. See
// docs/spec/functions.md.
func TestIsAtLeastAnswersWhatZshAnswers(t *testing.T) {
	out, st := runShipped(t, isAtLeastProbe)
	if out != isAtLeastWant || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", "isAtLeast", out, st, isAtLeastWant)
	}
}

const isAtLeastProbe = `autoload -Uz is-at-least
# Pinned, so the two rows that take the default do not depend on the version
# of the shell running this.
ZSH_VERSION=5.9.2
probe() { is-at-least "$@"; print -r -- "[$1] [$2] $?" }
probe 5.0 5.9.2
probe 5.9.2 5.0
probe 5.9.2 5.9.2
probe 5 5.9.2
probe 5.9.2 5
probe 4.3.10 4.3.9
probe 4.3.9 4.3.10
probe 5.0.0 5.0
probe 5.0 5.0.0
probe 5.0.1 5.0
probe 2.6-beta3 2.6
probe 2.6 2.6-beta3
probe 2.6-dev-3 2.6
probe 2.6 2.6-dev-3
probe 2.6-0 2.6
probe 3.1.1-zefram1 3.1.1
probe 3.1.1 3.1.1-zefram1
probe 5.9.2-dev-1 5.9.2
probe 5.9.2 5.9.2-dev-1
probe 5.9.2-dev-2 5.9.2-dev-1
probe 5.9.2-dev-1 5.9.2-dev-2
probe 5.10 5.9
probe 5.9 5.10
probe 3.1.6-15 5.9.2
probe 6.0 5.9.2
probe abc 5.9.2
probe 5.9.2 abc
probe 5.9.2 ""
probe "" 5.9.2
is-at-least; print -r -- "no args $?"
is-at-least 5.9.2 5.9.2 extra; print -r -- "extra arg $?"
ZSH_VERSION=9.9.9
is-at-least 5.0; print -r -- "default present 5.0 $?"
is-at-least 99.0; print -r -- "default present 99.0 $?"
`

const isAtLeastWant = `[5.0] [5.9.2] 0
[5.9.2] [5.0] 1
[5.9.2] [5.9.2] 0
[5] [5.9.2] 0
[5.9.2] [5] 1
[4.3.10] [4.3.9] 1
[4.3.9] [4.3.10] 0
[5.0.0] [5.0] 0
[5.0] [5.0.0] 0
[5.0.1] [5.0] 1
[2.6-beta3] [2.6] 0
[2.6] [2.6-beta3] 0
[2.6-dev-3] [2.6] 1
[2.6] [2.6-dev-3] 0
[2.6-0] [2.6] 0
[3.1.1-zefram1] [3.1.1] 0
[3.1.1] [3.1.1-zefram1] 0
[5.9.2-dev-1] [5.9.2] 1
[5.9.2] [5.9.2-dev-1] 0
[5.9.2-dev-2] [5.9.2-dev-1] 1
[5.9.2-dev-1] [5.9.2-dev-2] 0
[5.10] [5.9] 1
[5.9] [5.10] 0
[3.1.6-15] [5.9.2] 0
[6.0] [5.9.2] 1
[abc] [5.9.2] 0
[5.9.2] [abc] 1
[5.9.2] [] 0
[] [5.9.2] 0
no args 0
extra arg 0
default present 5.0 0
default present 99.0 1
`

// add-zsh-hook, against the answers zsh 5.9.2 gives for the same probes.
//
// Every line below matched zsh byte for byte on 2026-09-11 **except one**,
// and it is this shell's typeset rather than this function's: the listing
// reads
//
//	typeset -a precmd_functions=( f1 )
//
// where zsh writes "typeset -g -a precmd_functions=( f1 )". Our "typeset -p"
// does not write the -g for a global, which is a gap in the builtin and is
// filed as such; asserting our own spelling here keeps the row exact rather
// than approximate.
//
// The rest is the function, and the sharp corners are: a name already in the
// array is not added twice, -d removes by exact name where -D takes a
// pattern, and removing the last element **unsets** the array rather than
// leaving an empty one.
func TestAddZshHookAnswersWhatZshAnswers(t *testing.T) {
	out, st := runShipped(t, addZshHookProbe)
	if out != addZshHookWant || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", "addZshHook", out, st, addZshHookWant)
	}
}

const addZshHookProbe = `autoload -Uz add-zsh-hook
f1() { : }
f2() { : }
show() { print -r -- "$1 st=$2 set=${+precmd_functions} (${precmd_functions[@]})" }
add-zsh-hook precmd f1; show add-f1 $?
add-zsh-hook precmd f2; show add-f2 $?
add-zsh-hook precmd f1; show add-f1-again $?
add-zsh-hook -d precmd f1; show del-f1 $?
add-zsh-hook -d precmd nosuch; show del-missing $?
add-zsh-hook -d precmd f2; show del-last $?
add-zsh-hook -d precmd f2; show del-unset $?
add-zsh-hook precmd f1; add-zsh-hook precmd f2
add-zsh-hook -d precmd 'f*'; show del-exact-not-pattern $?
add-zsh-hook -D precmd 'f*'; show del-pattern $?
print -r -- "--- listing"
add-zsh-hook precmd f1
add-zsh-hook -L precmd; print -r -- "L-one st=$?"
add-zsh-hook -L nosuchhook; print -r -- "L-unknown st=$?"
add-zsh-hook -L chpwd; print -r -- "L-unset st=$?"
print -r -- "--- refusals"
add-zsh-hook nosuchhook f1; print -r -- "bad-hook st=$?"
add-zsh-hook precmd; print -r -- "one-operand st=$?"
add-zsh-hook; print -r -- "no-operand st=$?"
add-zsh-hook precmd a b; print -r -- "three-operands st=$?"
print -r -- "--- autoloading"
add-zsh-hook -Uz precmd notyet; print -r -- "autoload st=$? whence=$(whence -w notyet)"
functions notyet
print -r -- "--- every hook name"
for h in chpwd precmd preexec periodic zshaddhistory zshexit zsh_directory_name; do
	add-zsh-hook $h f1 || print -r -- "REFUSED $h"
done
print -r -- "all-hooks ok"
`

const addZshHookWant = `add-f1 st=0 set=1 (f1)
add-f2 st=0 set=1 (f1 f2)
add-f1-again st=0 set=1 (f1 f2)
del-f1 st=0 set=1 (f2)
del-missing st=0 set=1 (f2)
del-last st=0 set=0 ()
del-unset st=0 set=0 ()
del-exact-not-pattern st=0 set=1 (f1 f2)
del-pattern st=0 set=0 ()
--- listing
typeset -a precmd_functions=( f1 )
L-one st=0
L-unknown st=0
L-unset st=0
--- refusals
Usage: add-zsh-hook hook function
Valid hooks are:
  chpwd precmd preexec periodic zshaddhistory zshexit zsh_directory_name
bad-hook st=1
Usage: add-zsh-hook hook function
Valid hooks are:
  chpwd precmd preexec periodic zshaddhistory zshexit zsh_directory_name
one-operand st=1
Usage: add-zsh-hook hook function
Valid hooks are:
  chpwd precmd preexec periodic zshaddhistory zshexit zsh_directory_name
no-operand st=1
Usage: add-zsh-hook hook function
Valid hooks are:
  chpwd precmd preexec periodic zshaddhistory zshexit zsh_directory_name
three-operands st=1
--- autoloading
autoload st=0 whence=notyet: function
notyet () {
	# undefined
	builtin autoload -XUz
}
--- every hook name
all-hooks ok
`

// colors, against what zsh 5.9.2 fills — every key and every value of eight
// associative arrays and two scalars, byte for byte.
//
// Written out in full rather than spot-checked on "$fg[red]", because the
// arrays are a data table and a table is wrong one entry at a time: the
// gray/grey pair that maps one way only, the "fg-" and "bg-" spellings, the
// reverse direction from code back to name, and the fourteen attributes are
// each a thing a loop can get right for seven entries and wrong for the
// eighth.
//
// Two rows are not about the values. "typeset -p fg" writes the name and not
// eleven escape sequences, which is the hiding attribute; and "color[zzz]=9"
// leaving "$colour[zzz]" empty is the measurement that the two arrays are
// separate rather than one under two names.
func TestColorsFillsExactlyWhatZshFills(t *testing.T) {
	out, st := runShipped(t, colorsFnProbe)
	if out != colorsFnWant || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", "colorsFn", out, st, colorsFnWant)
	}
}

const colorsFnProbe = `autoload -Uz colors
colors
print -r -- "status=$?"
for a in color colour fg fg_bold fg_no_bold bg bg_bold bg_no_bold; do
  eval 'for k in ${(ko)'"$a"'}; do print -r -- "'"$a"'[$k]=${'"$a"'[$k]//$'"'"'\e'"'"'/<ESC>}"; done'
done
print -r -- "reset_color=${reset_color//$'\e'/<ESC>}"
print -r -- "bold_color=${bold_color//$'\e'/<ESC>}"
typeset -p fg
typeset -p reset_color
color[zzz]=9
print -r -- "colour-is-separate=[${colour[zzz]}]"
colors
print -r -- "second=$? keys=${#fg}"
`

const colorsFnWant = `status=0
color[00]=none
color[01]=bold
color[02]=faint
color[03]=italic
color[04]=underline
color[05]=blink
color[07]=reverse
color[08]=conceal
color[22]=normal
color[23]=no-italic
color[24]=no-underline
color[25]=no-blink
color[27]=no-reverse
color[28]=no-conceal
color[30]=black
color[31]=red
color[32]=green
color[33]=yellow
color[34]=blue
color[35]=magenta
color[36]=cyan
color[37]=white
color[39]=default
color[40]=bg-black
color[41]=bg-red
color[42]=bg-green
color[43]=bg-yellow
color[44]=bg-blue
color[45]=bg-magenta
color[46]=bg-cyan
color[47]=bg-white
color[49]=bg-default
color[bg-black]=40
color[bg-blue]=44
color[bg-cyan]=46
color[bg-default]=49
color[bg-gray]=40
color[bg-green]=42
color[bg-grey]=40
color[bg-magenta]=45
color[bg-red]=41
color[bg-white]=47
color[bg-yellow]=43
color[black]=30
color[blink]=05
color[blue]=34
color[bold]=01
color[conceal]=08
color[cyan]=36
color[default]=39
color[faint]=02
color[fg-black]=30
color[fg-blue]=34
color[fg-cyan]=36
color[fg-default]=39
color[fg-gray]=30
color[fg-green]=32
color[fg-grey]=30
color[fg-magenta]=35
color[fg-red]=31
color[fg-white]=37
color[fg-yellow]=33
color[gray]=30
color[green]=32
color[grey]=30
color[italic]=03
color[magenta]=35
color[no-blink]=25
color[no-conceal]=28
color[no-italic]=23
color[no-reverse]=27
color[no-underline]=24
color[none]=00
color[normal]=22
color[red]=31
color[reverse]=07
color[underline]=04
color[white]=37
color[yellow]=33
colour[00]=none
colour[01]=bold
colour[02]=faint
colour[03]=italic
colour[04]=underline
colour[05]=blink
colour[07]=reverse
colour[08]=conceal
colour[22]=normal
colour[23]=no-italic
colour[24]=no-underline
colour[25]=no-blink
colour[27]=no-reverse
colour[28]=no-conceal
colour[30]=black
colour[31]=red
colour[32]=green
colour[33]=yellow
colour[34]=blue
colour[35]=magenta
colour[36]=cyan
colour[37]=white
colour[39]=default
colour[40]=bg-black
colour[41]=bg-red
colour[42]=bg-green
colour[43]=bg-yellow
colour[44]=bg-blue
colour[45]=bg-magenta
colour[46]=bg-cyan
colour[47]=bg-white
colour[49]=bg-default
colour[bg-black]=40
colour[bg-blue]=44
colour[bg-cyan]=46
colour[bg-default]=49
colour[bg-gray]=40
colour[bg-green]=42
colour[bg-grey]=40
colour[bg-magenta]=45
colour[bg-red]=41
colour[bg-white]=47
colour[bg-yellow]=43
colour[black]=30
colour[blink]=05
colour[blue]=34
colour[bold]=01
colour[conceal]=08
colour[cyan]=36
colour[default]=39
colour[faint]=02
colour[fg-black]=30
colour[fg-blue]=34
colour[fg-cyan]=36
colour[fg-default]=39
colour[fg-gray]=30
colour[fg-green]=32
colour[fg-grey]=30
colour[fg-magenta]=35
colour[fg-red]=31
colour[fg-white]=37
colour[fg-yellow]=33
colour[gray]=30
colour[green]=32
colour[grey]=30
colour[italic]=03
colour[magenta]=35
colour[no-blink]=25
colour[no-conceal]=28
colour[no-italic]=23
colour[no-reverse]=27
colour[no-underline]=24
colour[none]=00
colour[normal]=22
colour[red]=31
colour[reverse]=07
colour[underline]=04
colour[white]=37
colour[yellow]=33
fg[black]=<ESC>[30m
fg[blue]=<ESC>[34m
fg[cyan]=<ESC>[36m
fg[default]=<ESC>[39m
fg[gray]=<ESC>[30m
fg[green]=<ESC>[32m
fg[grey]=<ESC>[30m
fg[magenta]=<ESC>[35m
fg[red]=<ESC>[31m
fg[white]=<ESC>[37m
fg[yellow]=<ESC>[33m
fg_bold[black]=<ESC>[01;30m
fg_bold[blue]=<ESC>[01;34m
fg_bold[cyan]=<ESC>[01;36m
fg_bold[default]=<ESC>[01;39m
fg_bold[gray]=<ESC>[01;30m
fg_bold[green]=<ESC>[01;32m
fg_bold[grey]=<ESC>[01;30m
fg_bold[magenta]=<ESC>[01;35m
fg_bold[red]=<ESC>[01;31m
fg_bold[white]=<ESC>[01;37m
fg_bold[yellow]=<ESC>[01;33m
fg_no_bold[black]=<ESC>[22;30m
fg_no_bold[blue]=<ESC>[22;34m
fg_no_bold[cyan]=<ESC>[22;36m
fg_no_bold[default]=<ESC>[22;39m
fg_no_bold[gray]=<ESC>[22;30m
fg_no_bold[green]=<ESC>[22;32m
fg_no_bold[grey]=<ESC>[22;30m
fg_no_bold[magenta]=<ESC>[22;35m
fg_no_bold[red]=<ESC>[22;31m
fg_no_bold[white]=<ESC>[22;37m
fg_no_bold[yellow]=<ESC>[22;33m
bg[black]=<ESC>[40m
bg[blue]=<ESC>[44m
bg[cyan]=<ESC>[46m
bg[default]=<ESC>[49m
bg[gray]=<ESC>[40m
bg[green]=<ESC>[42m
bg[grey]=<ESC>[40m
bg[magenta]=<ESC>[45m
bg[red]=<ESC>[41m
bg[white]=<ESC>[47m
bg[yellow]=<ESC>[43m
bg_bold[black]=<ESC>[01;40m
bg_bold[blue]=<ESC>[01;44m
bg_bold[cyan]=<ESC>[01;46m
bg_bold[default]=<ESC>[01;49m
bg_bold[gray]=<ESC>[01;40m
bg_bold[green]=<ESC>[01;42m
bg_bold[grey]=<ESC>[01;40m
bg_bold[magenta]=<ESC>[01;45m
bg_bold[red]=<ESC>[01;41m
bg_bold[white]=<ESC>[01;47m
bg_bold[yellow]=<ESC>[01;43m
bg_no_bold[black]=<ESC>[22;40m
bg_no_bold[blue]=<ESC>[22;44m
bg_no_bold[cyan]=<ESC>[22;46m
bg_no_bold[default]=<ESC>[22;49m
bg_no_bold[gray]=<ESC>[22;40m
bg_no_bold[green]=<ESC>[22;42m
bg_no_bold[grey]=<ESC>[22;40m
bg_no_bold[magenta]=<ESC>[22;45m
bg_no_bold[red]=<ESC>[22;41m
bg_no_bold[white]=<ESC>[22;47m
bg_no_bold[yellow]=<ESC>[22;43m
reset_color=<ESC>[00m
bold_color=<ESC>[01m
typeset -A fg
typeset reset_color
colour-is-separate=[]
second=0 keys=11
`

// regexp-replace, against the answers zsh 5.9.2 gives for the same probes.
//
// The empty match is what shapes it: "x*" over "abc" is "YaYbYc" and "b*"
// over "abcabc" is "-a--c-a--c", so a pattern that can match nothing matches
// between every pair of characters and the loop has to step over one. "^"
// anchors what is left of the subject rather than the original string, which
// is why "^a" over "aXaXa" replaces once — zshcontrib(1) warns of exactly
// that.
//
// The last three rows are about the parameters rather than the replacement: a
// caller's own $MATCH, $match and $MBEGIN survive the call, the variable may
// be named indirectly, and a name holding nothing is status 1 with nothing
// written.
func TestRegexpReplaceAnswersWhatZshAnswers(t *testing.T) {
	out, st := runShipped(t, regexpReplaceProbe)
	if out != regexpReplaceWant || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", "regexpReplace", out, st, regexpReplaceWant)
	}
}

const regexpReplaceProbe = `autoload -Uz regexp-replace
t() { local v=$1; regexp-replace v "$2" "$3"; print -r -- "[$1] /$2/$3/ -> [$v] $?" }
t 'hello world' 'o' '0'
t 'hello world' 'l+' 'L'
t 'hello' 'z' 'X'
t 'abc' '(a)(b)' '${match[2]}${match[1]}'
t 'abc' 'b' '[$MATCH]'
t 'a.b.c' '\.' '-'
t 'aaa' 'a*' 'X'
t 'abc' 'x*' 'Y'
t 'abcabc' 'b*' '-'
t 'foo bar' '[[:space:]]+' '_'
t 'abc' 'b' '$(echo CMD)'
t 'abc' 'b' 'x&y'
t 'abc' 'b' ''
t 'aXaXa' '^a' 'Z'
t 'ab' 'b' '$(( 1 + 1 ))'
t 'abcabc' 'b' 'B'
t '' 'a' 'X'
MATCH=kept; match=(kept); MBEGIN=99
v=abcabc; regexp-replace v 'b' 'B'
print -r -- "caller MATCH=[$MATCH] match=[${match[@]}] MBEGIN=[$MBEGIN]"
foo='one two'; name=foo; regexp-replace $name ' ' '-'; print -r -- "indirect foo=[$foo] $?"
unset nv; regexp-replace nv 'a' 'b'; print -r -- "unset var st=$? nv=[$nv]"
`

const regexpReplaceWant = `[hello world] /o/0/ -> [hell0 w0rld] 0
[hello world] /l+/L/ -> [heLo worLd] 0
[hello] /z/X/ -> [hello] 1
[abc] /(a)(b)/${match[2]}${match[1]}/ -> [bac] 0
[abc] /b/[$MATCH]/ -> [a[b]c] 0
[a.b.c] /\./-/ -> [a-b-c] 0
[aaa] /a*/X/ -> [X] 0
[abc] /x*/Y/ -> [YaYbYc] 0
[abcabc] /b*/-/ -> [-a--c-a--c] 0
[foo bar] /[[:space:]]+/_/ -> [foo_bar] 0
[abc] /b/$(echo CMD)/ -> [aCMDc] 0
[abc] /b/x&y/ -> [ax&yc] 0
[abc] /b// -> [ac] 0
[aXaXa] /^a/Z/ -> [ZXaXa] 0
[ab] /b/$(( 1 + 1 ))/ -> [a2] 0
[abcabc] /b/B/ -> [aBcaBc] 0
[] /a/X/ -> [] 1
caller MATCH=[kept] match=[kept] MBEGIN=[99]
indirect foo=[one-two] 0
unset var st=1 nv=[]
`

// A shipped function loaded with -U is not rewritten by the caller's aliases,
// and loaded without -U it is.
//
// Both halves are the row. A real startup file defines aliases before it
// autoloads anything, so if an alias could reach the body of add-zsh-hook the
// shell would fail in a way nobody could see from the file; and a probe that
// only showed the -U case passing could not tell alias suppression from an
// alias that never mattered. Here the marker appears in the second run and
// not the first, so the check would notice if the suppression went.
//
// Nothing in a function file can defend itself against this — the decision is
// the caller's letter and the engine's (#1993) — which is why these files
// carry no defense and this row is where the property is pinned from.
//
// Measured: zsh 5.9.2 handed the same file on FPATH answers identically.
func TestAShippedFunctionLoadedWithUIsNotRewrittenByAnAlias(t *testing.T) {
	out, st := runShipped(t, aliasLoadProbe)
	if out != aliasLoadWant || st != 0 {
		t.Errorf("%s = %q (status %d), want %q", "aliasLoad", out, st, aliasLoadWant)
	}
}

const aliasLoadProbe = `alias emulate='print -r -- ALIAS-REACHED-THE-BODY; :'
autoload -Uz is-at-least
is-at-least 5.0 5.9; print -r -- "with -U st=$?"
unfunction is-at-least
autoload is-at-least
is-at-least 5.0 5.9; print -r -- "without -U st=$?"
`

const aliasLoadWant = `with -U st=0
ALIAS-REACHED-THE-BODY
without -U st=0
`
