// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// `$terminfo` and `$termcap`, measured against zsh 5.9.2 (2026-09-07) under
// `TERM=xterm-256color` with a scratch HOME and no startup files.
//
// The measurement that decided the shape of this is in repl/terminfo.go: the
// same seven capability names read out of real zsh under seven `$TERM` values.
// What is asserted here is the *parameter* — that it exists, that a key it
// answers gives real zsh's byte-for-byte value, that a key it does not answer
// refuses rather than reading empty, and that a script asking whether a key is
// there is answered instead of stopped.

// The test a plugin manager's entire color table sits behind, run as it is
// written in the file it comes from — #1388.
//
// Both halves have to hold and they are separate things. `${+terminfo}` is 1
// because the parameter is produced and its producer has entries, and
// `-n ${terminfo[colors]}` is satisfied because `colors` is one of the
// capabilities answered. A partial table that failed either would leave the
// table unbuilt and every message the manager printed would come out as raw
// markup.
func TestThePluginManagersColorTestPasses(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `if [[ ( ${+terminfo} -eq 1 && -n ${terminfo[colors]} ) || ( ${+termcap} -eq 1 && -n ${termcap[Co]} ) ]] {
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

// Every capability answered gives real zsh's value, under both name systems.
//
// The values are zsh 5.9.2's own, read out of its `$terminfo` and `$termcap`
// under `TERM=xterm-256color` and written here as the bytes rather than as
// escapes, so that a change to the table is compared against the shell being
// modeled and not against the table's own opinion.
func TestEveryAnsweredCapabilityIsRealZshsValue(t *testing.T) {
	measured := map[string]string{
		"colors": "256",
		"cr":     "\r",
		"cub":    "\x1b[%p1%dD",
		"cud":    "\x1b[%p1%dB",
		"cuf":    "\x1b[%p1%dC",
		"cuu":    "\x1b[%p1%dA",
		"ed":     "\x1b[J",
		"el":     "\x1b[K",
		"home":   "\x1b[H",
		"kcub1":  "\x1bOD",
		"kcud1":  "\x1bOB",
		"kcuf1":  "\x1bOC",
		"kcuu1":  "\x1bOA",
	}
	dir := t.TempDir()
	for _, c := range repl.TerminalCapabilities() {
		want, ok := measured[c.Terminfo]
		if !ok {
			t.Errorf("%s is answered and this test has no measurement of what real zsh says it is", c.Terminfo)
			continue
		}
		out, st := runZsh(t, dir,
			`printf "[%s][%s]" "${terminfo[`+c.Terminfo+`]}" "${termcap[`+c.Termcap+`]}"`)
		if got := "[" + want + "][" + want + "]"; out != got || st != 0 {
			t.Errorf("$terminfo[%s] and $termcap[%s] = %q (status %d), want %q",
				c.Terminfo, c.Termcap, out, st, got)
		}
	}
}

// A capability this shell has no answer for refuses by name, at the expansion
// that asked, with the command not run.
//
// Not empty, and that is the whole of #1388 one layer down: measured, real
// zsh's `$terminfo[colors]` is genuinely absent under `TERM=dumb`, so a
// caller reading an empty string cannot tell a terminal without the
// capability from a shell that never knew it. `cnorm` is the name used here
// because it is one a real prompt reads and one this shell deliberately
// refuses — the xterm family and the screen family spell it differently.
func TestACapabilityThisShellDoesNotAnswerRefusesByName(t *testing.T) {
	for _, c := range []struct{ name, key string }{
		{"terminfo", "cnorm"},
		{"terminfo", "sc"},
		{"termcap", "so"},
	} {
		out, st := runZsh(t, t.TempDir(),
			`print -r -- "before"`+"\n"+`print -r -- "[${`+c.name+`[`+c.key+`]}]"`+"\n"+`print -r -- "after"`)
		want := "before\nzsh:2: " + c.name + "[" + c.key + "]: capability not implemented yet\n"
		if out != want || st == 0 {
			t.Errorf("reading $%s[%s] = %q (status %d), want %q and a non-zero status",
				c.name, c.key, out, st, want)
		}
	}
}

// A script that *asks* is answered rather than stopped, and this is the
// exemption that keeps the refusal from being worse than the gap.
//
// Swept across a real plugin tree, `$+terminfo[…]` is the commonest way these
// keys are touched: a well-written prompt reads `cnorm` only after
// `(( $+terminfo[civis] && $+terminfo[cnorm] ))` has told it there is
// something to read. A guard that stops the shell is not a guard, so the set
// test answers 0 and the four conditional operators supply the script's own
// answer.
func TestAScriptAskingWhetherACapabilityIsThereIsAnswered(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "set=${+terminfo[cnorm]} have=${+terminfo[colors]}"
print -r -- "dash=[${terminfo[cnorm]-none}] colon=[${terminfo[cnorm]:-none}]"
print -r -- "plus=[${terminfo[cnorm]+yes}] present=[${terminfo[colors]+yes}]"
print -r -- "still here"`)
	want := "set=0 have=1\ndash=[none] colon=[none]\nplus=[] present=[yes]\nstill here\n"
	if out != want || st != 0 {
		t.Errorf("asking about a capability = %q (status %d), want %q", out, st, want)
	}
}

// `${terminfo[@]}` is the whole table and never a refusal: no key was named,
// so there is no key to refuse.
//
// The count is the table's size, which is what makes this more than a smoke
// test — a refusal reached from the whole-array path would show up as a
// diagnostic here, and an empty table would show up as 0.
func TestTheWholeTableIsNeitherRefusedNorEmpty(t *testing.T) {
	n := strconv.Itoa(len(repl.TerminalCapabilities()))
	out, st := runZsh(t, t.TempDir(),
		`print -r -- "n=${#terminfo} all=${#terminfo[@]} tc=${#termcap}"`)
	want := "n=" + n + " all=" + n + " tc=" + n + "\n"
	if out != want || st != 0 {
		t.Errorf("the whole table = %q (status %d), want %q", out, st, want)
	}
}

// Both are readonly and both stay out of a listing's values.
//
// Measured: `terminfo[colors]=9` is `read-only variable: terminfo` in zsh
// 5.9.2, `unset terminfo` is the same, and `typeset -p terminfo` writes
// `typeset -Ar terminfo` — the bare name and no values. Readonly alone would
// put the name in the tables a listing walks, and the listing would then write
// out the whole capability table as an assignment somebody could source back.
func TestBothParametersAreReadonlyAndTheirValuesStayOutOfAListing(t *testing.T) {
	for _, name := range []string{"terminfo", "termcap"} {
		// One statement per run, because a readonly assignment is fatal in
		// both shells — measured, `terminfo[colors]=9; echo $?` in zsh 5.9.2
		// prints the complaint and nothing else — so a second line in the
		// same script would never be reached to be asserted on.
		out, st := runZsh(t, t.TempDir(), name+`[colors]=9`)
		want := "zsh:1: read-only variable: " + name + "\n"
		if out != want || st == 0 {
			t.Errorf("writing to $%s = %q (status %d), want %q and a non-zero status", name, out, st, want)
		}
	}
	kept, st := runZsh(t, t.TempDir(), `print -r -- "colors=[${terminfo[colors]}][${termcap[Co]}]"`)
	if want := "colors=[256][256]\n"; kept != want || st != 0 {
		t.Errorf("the tables after a refused write = %q (status %d), want %q", kept, st, want)
	}
	listing, st := runZsh(t, t.TempDir(), `typeset -r`)
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
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/terminfo; print -r -- "ti=$?"
zmodload zsh/termcap; print -r -- "tc=$?"
zmodload -e zsh/terminfo zsh/termcap; print -r -- "both=$?"
echoti smcup 2>&1; print -r -- "echoti=$?"`)
	want := "ti=0\ntc=0\nboth=0\nzsh:4: command not found: echoti\necho" + "ti=127\n"
	if out != want || st != 0 {
		t.Errorf("loading the two modules = %q (status %d), want %q", out, st, want)
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
	out, st := runZsh(t, t.TempDir(), `n=${terminfo[colors]}
print -rn -- "last="; print -Pn -- "%F{$((n - 1))}"; print -r --
print -rn -- "past="; print -Pn -- "%F{$n}"; print -r --`)
	want := "last=\x1b[38;5;255m\npast=\x1b[39m\n"
	if out != want || st != 0 {
		t.Errorf("the counted colors = %q (status %d), want %q", out, st, want)
	}
}
