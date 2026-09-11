// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/terminfofixture"
)

// `$terminfo` and `$termcap`, measured against zsh 5.9.2 under
// `TERM=xterm-256color` with a scratch HOME and no startup files.
//
// The description these run against is written by the test rather than taken
// from the machine, and that is not a convenience. /usr/share/terminfo is not
// the same on a laptop and a runner and is not guaranteed to exist, and
// `$TERM` is environment like any other — a test that let the developer's own
// through would pass here and answer differently somewhere else. So every
// case below points `$TERMINFO` at a directory it wrote, with a `$TERM` it
// chose, and the *values* in that directory are real ones: `cuu1` is `\e[A`
// because `od -An -tx1` on real zsh's answer says `1b 5b 41`.

// The indices these fixtures are written at.
//
// A compiled description is three arrays and no names — the position is the
// name — so a fixture has to say where each capability goes. They are the
// measured slots, and repl's own tests pin them; here they are just the
// arithmetic that makes a description with three capabilities in it.
const (
	boolAutoMargin   = 1   // am
	numberColors     = 13  // colors
	numberLines      = 2   // lines
	stringCursorUp   = 19  // cuu1
	stringCursorBack = 14  // cub1
	stringEraseLine  = 6   // el
	stringTopBottom  = 369 // smgtb
)

// fixtureTerm is the terminal every case here runs under.
const fixtureTerm = "zshfixture"

// runZshTerminfo runs src against a written description, with nothing of the
// machine's environment reaching it.
func runZshTerminfo(t *testing.T, src string) (string, int) {
	t.Helper()
	strs := map[int]string{
		stringCursorUp:   "\x1b[A",
		stringCursorBack: "\b",
		stringEraseLine:  "\x1b[K",
		// A string capability whose termcap code is claimed by a boolean as
		// well, so that the two views cannot be checked by counting alone.
		stringTopBottom: "\x1bMM",
	}
	nums := make([]int, numberColors+1)
	for i := range nums {
		nums[i] = terminfofixture.Absent
	}
	nums[numberColors], nums[numberLines] = 256, 24
	bools := make([]byte, boolAutoMargin+1)
	bools[boolAutoMargin] = 1
	dir := terminfofixture.Database(t, terminfofixture.Description{
		Name: fixtureTerm, Bools: bools, Nums: nums,
		StrCount: stringTopBottom + 1, Strs: strs,
		Ext: &terminfofixture.Extended{
			Values: []string{"\x1b[2 q"}, Names: []string{"Se"},
		},
	})
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(),
		Vars: map[string]string{
			"PATH": t.TempDir(), "TERM": fixtureTerm, "TERMINFO": dir,
			// Emptied rather than left out: an unset variable would fall
			// through to whatever the process was started with, and the
			// personal database is one of the two places a description is
			// looked for.
			"HOME": t.TempDir(), "TERMINFO_DIRS": "",
		},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The capability #2076 is about: a theme asks whether the terminal can move
// the cursor up, and is told yes, with the terminal's own bytes.
//
// Both halves are the bug. powerlevel10k's `_p9k_init_prompt` guards its
// scroll-and-redraw block on `(( $+terminfo[cuu1] ))`, so a refused `cuu1`
// did not produce an error — it produced a prompt built for a terminal that
// cannot scroll, missing the newline and `\e[A` that begin zsh's own
// rendering, with nothing said. The bytes are asserted as bytes because a
// plausible-but-wrong value would be worse than the refusal: the theme's
// have-I-got-it test would pass and the prompt would draw in the wrong place.
func TestTheCursorUpCapabilityIsThereWithItsOwnBytes(t *testing.T) {
	out, st := runZshTerminfo(t, `print -r -- "set=${+terminfo[cuu1]}"
print -rn -- "${terminfo[cuu1]}" | od -An -tx1 2>/dev/null || print -r -- "${(V)terminfo[cuu1]}"
print -r -- "guard=$(( $+terminfo[cuu1] ))"`)
	if !strings.HasPrefix(out, "set=1\n") || st != 0 {
		t.Errorf("asking about cuu1 = %q (status %d), want it set", out, st)
	}
	if !strings.Contains(out, "guard=1") {
		t.Errorf("the guard a theme writes = %q, want 1", out)
	}
}

// The value is the description's bytes and not a rendering of them.
func TestACapabilityReadsAsTheBytesTheDescriptionHolds(t *testing.T) {
	out, st := runZshTerminfo(t,
		`for k in cuu1 el cub1; do printf "%s=[%s]\n" $k "${(V)terminfo[$k]}"; done`)
	want := "cuu1=[^[[A]\nel=[^[[K]\ncub1=[^H]\n"
	if out != want || st != 0 {
		t.Errorf("the capabilities = %q (status %d), want %q", out, st, want)
	}
}

// The test a plugin manager's entire color table sits behind, run as it is
// written in the file it comes from — #1388.
//
// Both halves have to hold and they are separate things. `${+terminfo}` is 1
// because the parameter is produced, and `-n ${terminfo[colors]}` is
// satisfied because the description carries a color count. A partial table
// that failed either would leave the table unbuilt and every message the
// manager printed would come out as raw markup.
func TestThePluginManagersColorTestPasses(t *testing.T) {
	out, st := runZshTerminfo(t, `if [[ ( ${+terminfo} -eq 1 && -n ${terminfo[colors]} ) || ( ${+termcap} -eq 1 && -n ${termcap[Co]} ) ]] {
  print -r -- built
} else {
  print -r -- "unbuilt +ti=${+terminfo} colors=[${terminfo[colors]}] +tc=${+termcap} Co=[${termcap[Co]}]"
}
if [[ ( ${+terminfo} -eq 1 && ${terminfo[colors]} -ge 256 ) || ( ${+termcap} -eq 1 && ${termcap[Co]} -ge 256 ) ]] {
  print -r -- "256-color branch"
}`)
	want := "built\n256-color branch\n"
	if out != want || st != 0 {
		t.Errorf("the color test = %q (status %d), want %q", out, st, want)
	}
}

// A capability the terminal does not have is absent, and reading it is not an
// error.
//
// This is what #2076 changed and it is the whole of the fix's shape: a key
// with no answer used to stop the shell with `capability not implemented
// yet`, which made `$+terminfo[…]` 0 for capabilities the terminal has. Now
// 0 means the terminal has not got it, which is what real zsh reports and
// what the four conditional operators are written against.
func TestACapabilityTheTerminalLacksIsAbsentRatherThanRefused(t *testing.T) {
	out, st := runZshTerminfo(t, `print -r -- "set=${+terminfo[cnorm]} have=${+terminfo[cuu1]}"
print -r -- "dash=[${terminfo[cnorm]-none}] colon=[${terminfo[cnorm]:-none}]"
print -r -- "plus=[${terminfo[cnorm]+yes}] present=[${terminfo[cuu1]+yes}]"
print -r -- "nosuch=${+terminfo[nosuchcapability]}"
print -r -- "still here"`)
	want := "set=0 have=1\ndash=[none] colon=[none]\nplus=[] present=[yes]\nnosuch=0\nstill here\n"
	if out != want || st != 0 {
		t.Errorf("asking about a capability = %q (status %d), want %q", out, st, want)
	}
}

// Every boolean name answers, whether the description stores it or not.
//
// Measured: zsh's `$terminfo` under `TERM=dumb` is 50 keys and 44 of them are
// booleans, though `dumb`'s description stores far fewer. A shell answering
// only the stored ones would report `$+terminfo[bce]` as 0 on most terminals,
// which is #2076's failure with a different key.
func TestEveryBooleanNameAnswersYesOrNo(t *testing.T) {
	out, st := runZshTerminfo(t,
		`print -r -- "am=${terminfo[am]} bce=${terminfo[bce]} hc=${terminfo[hc]} set=${+terminfo[bce]}"`)
	want := "am=yes bce=no hc=no set=1\n"
	if out != want || st != 0 {
		t.Errorf("the booleans = %q (status %d), want %q", out, st, want)
	}
}

// The extended section answers under the names it carries.
//
// A prompt restoring the cursor shape reads `$terminfo[Se]`, which is not in
// the standard capability set at all: it is a name the description carries
// itself. A reader that stopped at the standard arrays would answer 0 to that
// test and the prompt would leave the cursor as the last widget set it.
func TestAnExtendedCapabilityAnswersUnderItsOwnName(t *testing.T) {
	out, st := runZshTerminfo(t, `print -r -- "Se=${+terminfo[Se]} [${(V)terminfo[Se]}]"`)
	want := "Se=1 [^[[2 q]\n"
	if out != want || st != 0 {
		t.Errorf("the extended capability = %q (status %d), want %q", out, st, want)
	}
}

// A `$TERM` with no description leaves the parameter there and the table
// empty.
//
// Measured against zsh 5.9.2: `${+terminfo}` is 1 under every `$TERM`
// including one the database has never heard of, and `${#terminfo}` is 0 for
// that one. The parameter always exists and the table is what varies, which
// is what a script testing `$+terminfo[cuu1]` is written against — and it is
// why nothing substitutes a plausible value for a capability it cannot find.
func TestATermWithNoDescriptionLeavesTheParameterEmpty(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(),
		Vars: map[string]string{
			"PATH": t.TempDir(), "TERM": "nosuchterminal",
			"TERMINFO": t.TempDir(), "HOME": t.TempDir(), "TERMINFO_DIRS": "",
		},
	}, `print -r -- "+ti=${+terminfo} n=${#terminfo} cuu1=${+terminfo[cuu1]} v=[${terminfo[cuu1]}]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "+ti=1 n=0 cuu1=0 v=[]\n"
	if out != want || st != 0 {
		t.Errorf("an unknown terminal = %q (status %d), want %q", out, st, want)
	}
}

// `$termcap` is the same reading under termcap's two-letter codes.
func TestTermcapIsTheSameValuesUnderTheOtherNames(t *testing.T) {
	out, st := runZshTerminfo(t,
		`print -r -- "up=[${(V)termcap[up]}] ce=[${(V)termcap[ce]}] Co=[${termcap[Co]}] am=[${termcap[am]}]"`)
	want := "up=[^[[A] ce=[^[[K] Co=[256] am=[yes]\n"
	if out != want || st != 0 {
		t.Errorf("the termcap view = %q (status %d), want %q", out, st, want)
	}
}

// Where two capabilities claim one termcap code, `$termcap` answers with the
// one zsh's own search reaches first.
//
// Three codes are claimed twice by terminfo(5)'s own table, and `MT` is the
// case a real description can produce: the boolean `OTMT` and the string
// `smgtb`. Measured against zsh 5.9.2 with a description carrying `smgtb`,
// `${termcap[MT]}` is `no` — the boolean — while `${terminfo[smgtb]}` is the
// sequence. Booleans come before strings in the search, so the view is built
// in that order and the first writer keeps the key.
func TestATermcapCodeClaimedTwiceAnswersWithTheBoolean(t *testing.T) {
	out, st := runZshTerminfo(t,
		`print -r -- "MT=[${(V)termcap[MT]}] smgtb=[${(V)terminfo[smgtb]}] OTMT=[${terminfo[OTMT]}]"`)
	want := "MT=[no] smgtb=[^[MM] OTMT=[no]\n"
	if out != want || st != 0 {
		t.Errorf("the doubly-claimed code = %q (status %d), want %q", out, st, want)
	}
}

// `${terminfo[@]}` is the whole table, and the two views hold the same number
// of things.
//
// The count is what makes this more than a smoke test: an empty table would
// show up as 0, and a view that lost keys to a collision between two
// capabilities claiming one termcap code would show up as a smaller second
// number.
func TestTheWholeTableIsNeitherEmptyNorLopsided(t *testing.T) {
	out, st := runZshTerminfo(t, `print -r -- "n=${#terminfo} all=${#terminfo[@]} tc=${#termcap}"`)
	// Forty-four booleans, two numbers, four strings and one extended
	// string; `$termcap` is the same set minus the extended one, which has no
	// two-letter code to be found under, and minus `smgtb`, whose code `MT`
	// the boolean `OTMT` already claimed.
	want := "n=51 all=51 tc=49\n"
	if out != want || st != 0 {
		t.Errorf("the whole table = %q (status %d), want %q", out, st, want)
	}
}

// Both are readonly and both stay out of a listing's values.
//
// Measured: `terminfo[colors]=9` is `read-only variable: terminfo` in zsh
// 5.9.2, `unset terminfo` is the same, and `typeset -p terminfo` writes
// `typeset -Ar terminfo` — the bare name and no values. Readonly alone would
// put the name in the tables a listing walks, and the listing would then
// write out the whole capability table as an assignment somebody could source
// back.
func TestBothParametersAreReadonlyAndTheirValuesStayOutOfAListing(t *testing.T) {
	for _, name := range []string{"terminfo", "termcap"} {
		// One statement per run, because a readonly assignment is fatal in
		// both shells — measured, `terminfo[colors]=9; echo $?` in zsh 5.9.2
		// prints the complaint and nothing else — so a second line in the
		// same script would never be reached to be asserted on.
		out, st := runZshTerminfo(t, name+`[colors]=9`)
		want := "zsh:1: read-only variable: " + name + "\n"
		if out != want || st == 0 {
			t.Errorf("writing to $%s = %q (status %d), want %q and a non-zero status", name, out, st, want)
		}
	}
	kept, st := runZshTerminfo(t, `print -r -- "colors=[${terminfo[colors]}][${termcap[Co]}]"`)
	if want := "colors=[256][256]\n"; kept != want || st != 0 {
		t.Errorf("the tables after a refused write = %q (status %d), want %q", kept, st, want)
	}
	listing, st := runZshTerminfo(t, `typeset -r`)
	if st != 0 {
		t.Fatalf("typeset -r exited %d: %q", st, listing)
	}
	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(line, "terminfo=") || strings.HasPrefix(line, "termcap=") ||
			strings.Contains(line, "\x1b") {
			t.Errorf("a readonly listing carries a capability table's values: %q", line)
		}
	}
}

// The modules load now, because their parameters are here — and their
// builtins are still missing and still refuse where they are called.
//
// That pair is the module rule in zmodload.go rather than an inconsistency: a
// missing builtin refuses by name on the line that ran it, so it never holds a
// module shut, and a script told `zsh/terminfo` loaded finds out about
// `echoti` where it calls `echoti`.
func TestTheModulesLoadAndTheirBuiltinsStillRefuse(t *testing.T) {
	out, st := runZshTerminfo(t, `zmodload zsh/terminfo; print -r -- "ti=$?"
zmodload zsh/termcap; print -r -- "tc=$?"
zmodload -e zsh/terminfo zsh/termcap; print -r -- "both=$?"
echoti smcup 2>&1; print -r -- "echoti=$?"`)
	want := "ti=0\ntc=0\nboth=0\nzsh:4: command not found: echoti\necho" + "ti=127\n"
	if out != want || st != 0 {
		t.Errorf("loading the two modules = %q (status %d), want %q", out, st, want)
	}
}

// The table follows `$TERM` rather than being read once.
//
// A produced association is produced on every read, and the reading behind
// this one is cached — so the cache's key has to be the environment it was
// read from. A view that stopped tracking would be the failure
// SetDynamicAssocWriter exists to prevent one layer up: nothing about it says
// it stopped, and a script that exported a different `$TERM` would go on
// being told about the old terminal.
func TestTheTableFollowsTerm(t *testing.T) {
	other := "zshfixtureplain"
	dir := terminfofixture.Database(t,
		terminfofixture.Description{
			Name: fixtureTerm, StrCount: stringCursorUp + 1,
			Strs: map[int]string{stringCursorUp: "\x1b[A"},
		},
		terminfofixture.Description{
			Name: other, StrCount: stringCursorUp + 1,
			Strs: map[int]string{stringCursorUp: "\x1bM"},
		})
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(),
		Vars: map[string]string{
			"PATH": t.TempDir(), "TERM": fixtureTerm, "TERMINFO": dir,
			"HOME": t.TempDir(), "TERMINFO_DIRS": "",
		},
	}, `print -r -- "first=[${(V)terminfo[cuu1]}]"
TERM=`+other+`
print -r -- "second=[${(V)terminfo[cuu1]}]"
TERM=nosuchterminal
print -r -- "third=${+terminfo[cuu1]} n=${#terminfo}"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "first=[^[[A]\nsecond=[^[M]\nthird=0 n=0\n"
	if out != want || st != 0 {
		t.Errorf("changing $TERM = %q (status %d), want %q", out, st, want)
	}
}

// The count `$terminfo[colors]` reports is the count this shell paints.
//
// The discriminating pair: the highest index it claims draws a color, and the
// first index past the claim draws the default. A capability reporting a
// number the renderer disagrees with is the failure worth catching — a theme
// takes the 256-color branch and comes out colorless — and it is invisible to
// any test that only compares the number against itself.
func TestTheColorCountIsTheCountThisShellPaints(t *testing.T) {
	out, st := runZshTerminfo(t, `n=${terminfo[colors]}
print -rn -- "last="; print -Pn -- "%F{$((n - 1))}"; print -r --
print -rn -- "past="; print -Pn -- "%F{$n}"; print -r --`)
	want := "last=\x1b[38;5;255m\npast=\x1b[39m\n"
	if out != want || st != 0 {
		t.Errorf("the counted colors = %q (status %d), want %q", out, st, want)
	}
}
