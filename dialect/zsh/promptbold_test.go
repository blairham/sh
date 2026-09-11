// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `%b` clears the color, so zsh writes the color back — and so does this.
//
// Measured against zsh 5.9.2 under `TERM=xterm-256color`, one probe per row,
// through `${(%%)v}` with the bytes read off `od -An -tx1`. The rows below
// are those bytes and not a description of them: the whole question here is
// which select-graphic-rendition parameters come out and in what order, so a
// row that checked for a substring would pass on output with the reset in
// the wrong place, with the restore in the wrong place, or with either of
// them written twice.
//
// The fault this pins (#2075): `%b` is bold-off, and on this terminal the
// only sequence that turns boldface off is `\e[0m`, which clears everything
// — the color included. Real zsh writes the reset and then writes the
// foreground that was in effect again; this shell wrote the reset alone, so
// a prompt came back uncolored after its first `%b`. powerlevel10k writes
// `%b%k` between every pair of segments, so every segment after the first
// lost its color.
//
//	${(%%)'%F{031}a%b%k%F{242}x%f'}
//	zsh   \e[38;5;31m a \e[0m\e[38;5;31m \e[49m \e[38;5;242m x \e[39m
//	ours  \e[38;5;31m a \e[0m             \e[49m \e[38;5;242m x \e[39m
//
// The same probe with no color in front of it agreed byte for byte before
// the fix and still does, which is what said the fault was the restore and
// not the reset. It is the first row below.
func TestABoldOffWritesTheColorBackAgain(t *testing.T) {
	for _, tc := range []struct {
		prompt, want, why string
	}{
		// The issue's own pair. Nothing set beforehand, so nothing is
		// written back — this row is the control, and it passed before the
		// fix as well.
		{
			`%b%k%F{242}x%f`,
			"\x1b[0m\x1b[49m\x1b[38;5;242mx\x1b[39m",
			"a reset with nothing in effect restores nothing",
		},
		{
			`%F{031}a%b%k%F{242}x%f`,
			"\x1b[38;5;31ma\x1b[0m\x1b[38;5;31m\x1b[49m\x1b[38;5;242mx\x1b[39m",
			"the foreground is written back after the reset",
		},
		// Bold-*on* restores too, which is the half that reads as a
		// surprise: `%U` and `%S` in the same position write their sequence
		// and nothing more.
		{`%F{red}%Bbold%b still`, "\x1b[31m\x1b[1m\x1b[31mbold\x1b[0m\x1b[31m still", "bold-on restores"},
		{`%B%F{red}bold%b still`, "\x1b[1m\x1b[31mbold\x1b[0m\x1b[31m still", "and the color set after it is not written twice"},
		{`%F{red}%Ua`, "\x1b[31m\x1b[4ma", "underline-on does not restore"},
		{`%F{red}%Sa`, "\x1b[31m\x1b[7ma", "standout-on does not restore"},
		{`%F{red}%Ba%U`, "\x1b[31m\x1b[1m\x1b[31ma\x1b[4m", "and `%U` after a restore still writes alone"},
		// Each `%B` writes one bold and one restore, however many there are.
		{`%F{red}%B%Ba`, "\x1b[31m\x1b[1m\x1b[31m\x1b[1m\x1b[31ma", "a repeated bold-on does not double its own sequence"},
		// The other two attribute-offs restore as well, and their sequences
		// clear nothing but themselves — which is why this is measured
		// rather than derived from "the sequence resets everything".
		{`%F{red}a%u`, "\x1b[31ma\x1b[24m\x1b[31m", "underline-off restores"},
		{`%F{red}a%s`, "\x1b[31ma\x1b[27m\x1b[31m", "standout-off restores"},
		// The color-offs do not restore, and they do clear: a `%b` after
		// `%f` writes the reset alone.
		{`%B%F{red}a%f`, "\x1b[1m\x1b[31ma\x1b[39m", "foreground-off restores nothing"},
		{`%B%F{red}a%k`, "\x1b[1m\x1b[31ma\x1b[49m", "background-off restores nothing"},
		{`%F{red}a%f%b`, "\x1b[31ma\x1b[39m\x1b[0m", "and a cleared foreground is not written back"},
		{`%F{red}%K{#00ff00}a%b`, "\x1b[31m\x1b[48;2;0;255;0ma\x1b[0m\x1b[31m\x1b[48;2;0;255;0m", "both layers come back, foreground first"},
		// The order a restore writes in is fixed rather than the order the
		// text set things in: bold, standout, underline, foreground,
		// background.
		{`%U%S%F{red}%K{blue}a%b`, "\x1b[4m\x1b[7m\x1b[31m\x1b[44ma\x1b[0m\x1b[7m\x1b[4m\x1b[31m\x1b[44m", "standout before underline, whichever was set first"},
		{`%S%U%F{red}%K{blue}a%b`, "\x1b[7m\x1b[4m\x1b[31m\x1b[44ma\x1b[0m\x1b[7m\x1b[4m\x1b[31m\x1b[44m", "and the same the other way round"},
		{`%B%Sa%u`, "\x1b[1m\x1b[7ma\x1b[24m\x1b[1m\x1b[7m", "bold before standout"},
		{`%B%S%U%F{red}%K{blue}a%s`, "\x1b[1m\x1b[7m\x1b[4m\x1b[31m\x1b[44ma\x1b[27m\x1b[1m\x1b[4m\x1b[31m\x1b[44m", "and the whole order at once, minus the one just cleared"},
		// A restore leaves out what the code itself has just turned off.
		{`%B%S%Ua%b`, "\x1b[1m\x1b[7m\x1b[4ma\x1b[0m\x1b[7m\x1b[4m", "the bold `%b` cleared is not written back"},
		{`%B%Ua%u`, "\x1b[1m\x1b[4ma\x1b[24m\x1b[1m", "nor the underline `%u` cleared"},
		{`%Ba%b%bx`, "\x1b[1ma\x1b[0m\x1b[0mx", "a second reset with nothing left writes itself alone"},
		{`%F{red}%Ba%bb%bc`, "\x1b[31m\x1b[1m\x1b[31ma\x1b[0m\x1b[31mb\x1b[0m\x1b[31mc", "and every reset after a color writes it back"},
		// The longest one measured, which is the whole rule in one string.
		{
			`%F{red}%B%U%Sa%s%u%b`,
			"\x1b[31m\x1b[1m\x1b[31m\x1b[4m\x1b[7ma\x1b[27m\x1b[1m\x1b[4m\x1b[31m\x1b[24m\x1b[1m\x1b[31m\x1b[0m\x1b[31m",
			"every attribute on, then off one at a time",
		},
	} {
		src := "v=" + singleQuote(tc.prompt) + `; print -rn -- "${(%%)v}"`
		out, st := runZsh(t, t.TempDir(), src)
		if out != tc.want || st != 0 {
			t.Errorf("%s\n %s\n  drew %q (status %d)\n  want %q", tc.why, tc.prompt, out, st, tc.want)
		}
	}
}

// The table that says which codes do it, asserted as data so that a row
// dropped from dialect/zsh/prompt.go is a failure here rather than a prompt
// that quietly stops restoring.
//
// `%E` is the row that is deliberately absent: clearing to the end of the
// line changes nothing about what the next character looks like, so it has
// no entry at all.
func TestTheVisualTableIsWhatWasMeasured(t *testing.T) {
	got := zsh.PromptStyle().Visual
	want := map[rune]interp.PromptVisual{
		'B': {Attribute: interp.AttributeBold, Restores: true},
		'b': {Attribute: interp.AttributeBold, Off: true, Restores: true},
		'U': {Attribute: interp.AttributeUnderline},
		'u': {Attribute: interp.AttributeUnderline, Off: true, Restores: true},
		'S': {Attribute: interp.AttributeStandout},
		's': {Attribute: interp.AttributeStandout, Off: true, Restores: true},
		'f': {Attribute: interp.AttributeForeground, Off: true},
		'k': {Attribute: interp.AttributeBackground, Off: true},
	}
	if len(got) != len(want) {
		t.Errorf("the visual table has %d rows, want %d: %v", len(got), len(want), got)
	}
	for code, w := range want {
		if g := got[code]; g != w {
			t.Errorf("%%%c is %+v, want %+v", code, g, w)
		}
	}
	if v, ok := got['E']; ok {
		t.Errorf("%%E has a visual entry %+v; clearing the line changes no attribute", v)
	}
	// Every code named here is a sequence the same table draws. A row for a
	// code the walker never reaches would be a measurement with nothing to
	// apply it to.
	for code := range got {
		if _, ok := zsh.PromptStyle().Sequences[code]; !ok {
			t.Errorf("%%%c has a visual entry and no sequence to write", code)
		}
	}
}

// singleQuote wraps text for the shell, which every prompt above needs
// because a `%` is not special in single quotes and a `$` in one of them
// would be.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
