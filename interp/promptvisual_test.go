// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// A sequence that turns an attribute off takes the color with it, so the
// walker writes the color back.
//
// The mechanism, named as flags and never as a shell — the measured bytes of
// the one dialect that has such a language are in dialect/zsh. What is
// asserted here is the *rule* [PromptVisual] states: a code marked Restores
// is followed by every other setting still in effect, in the order the
// [PromptAttribute] constants are in, and a code not marked Restores is
// followed by nothing.
//
// It exists because the substrate had no visual state at all. Sequences were
// a flat table of strings written through as they stood, so `\e[0m` — the
// only way this terminal has of clearing boldface, and one that clears the
// color with it — ended a prompt's color for good (#2075).
//
// The table below is a stand-in dialect rather than a real one: two setting
// codes and two clearing ones, one of each marked Restores, so every arm of
// the rule has a row and none of them is a shell's.
func visualStyle() PromptStyle {
	return PromptStyle{
		Escape: '%',
		Sequences: map[rune]string{
			'H': "<H>", 'h': "<h>",
			'L': "<L>", 'l': "<l>",
			'N': "<N>", 'n': "<n>",
			'z': "<z>",
		},
		Visual: map[rune]PromptVisual{
			// The pair that disturbs everything: the clear is the whole
			// terminal's, and the set is marked too, which is the arm a rule
			// derived from "off codes restore" would miss.
			'H': {Attribute: AttributeBold, Restores: true},
			'h': {Attribute: AttributeBold, Off: true, Restores: true},
			// The pair that does not. The clear is marked as a clear, so the
			// walker stops writing the setting back, but writes nothing
			// after it.
			'L': {Attribute: AttributeUnderline},
			'l': {Attribute: AttributeUnderline, Off: true},
			// A third setting, so the order the restore writes in is pinned
			// between two attributes and not only between an attribute and a
			// color. It sits *before* the underline in the constants and
			// after it in every row below, which is what makes the ordering
			// assertions fail if the constants are reordered.
			'N': {Attribute: AttributeStandout},
			'n': {Attribute: AttributeStandout, Off: true},
			// And a sequence with no entry at all, which is every code in
			// every other dialect: bytes and nothing more.
		},
		Colors: map[rune]PromptColor{'F': Foreground, 'K': Background},
	}
}

func expandVisual(t *testing.T, text string) string {
	t.Helper()
	got, refused, ok := ExpandPromptStyle(visualStyle(), text, func(PromptField, string, bool) (string, bool) {
		return "", false
	}, nil)
	if !ok {
		t.Fatalf("%q was refused at %q", text, refused)
	}
	return got
}

func TestARestoringSequenceWritesBackWhatIsStillInEffect(t *testing.T) {
	for _, tc := range []struct {
		text, want string
		why        string
	}{
		// Nothing in effect, so there is nothing to write back — and this is
		// the row that says the restore is the restore and not a second copy
		// of the sequence.
		{"%h", "<h>", "a clear with no state before it"},
		{"%H", "<H>", "a set with no state before it"},
		// The bug itself: a color, then the clear that takes it away.
		{"%F{red}a%hb", "\x1b[31ma<h>\x1b[31mb", "a color survives a restoring clear"},
		// The set restores too, and does not write its own attribute twice.
		{"%F{red}%Ha", "\x1b[31m<H>\x1b[31ma", "a restoring set writes the color after itself"},
		{"%F{red}%H%Ha", "\x1b[31m<H>\x1b[31m<H>\x1b[31ma", "each restoring set writes one of itself"},
		// Both layers, in the constants' order: foreground before background.
		{"%K{blue}%F{red}%h", "\x1b[44m\x1b[31m<h>\x1b[31m\x1b[44m", "both colors, foreground first"},
		// And the attributes come before the colors, in the constants' order,
		// whichever order they were set in.
		{"%L%F{red}%h", "<L>\x1b[31m<h><L>\x1b[31m", "the attribute is written before the color"},
		{"%F{red}%L%h", "\x1b[31m<L><h><L>\x1b[31m", "and the order is the constants', not the text's"},
		// Two attributes, so the order between them is pinned as well:
		// standout comes before underline whichever was set first.
		{"%L%N%h", "<L><N><h><N><L>", "standout before underline"},
		{"%N%L%h", "<N><L><h><N><L>", "and the same the other way round"},
		{"%N%L%F{red}%K{blue}%h", "<N><L>\x1b[31m\x1b[44m<h><N><L>\x1b[31m\x1b[44m", "the whole order in one row"},
		// A clear that is not marked Restores writes its sequence alone —
		// which is the half a rule reading "an off code restores" gets wrong.
		{"%F{red}a%lb", "\x1b[31ma<l>b", "a plain clear restores nothing"},
		{"%F{red}%La", "\x1b[31m<L>a", "a plain set restores nothing"},
		// It still *clears*, so the next restore does not write it back.
		{"%L%F{red}%l%h", "<L>\x1b[31m<l><h>\x1b[31m", "a cleared attribute is not restored later"},
		// A code with no Visual entry is bytes and nothing else, in both
		// directions: it neither restores nor becomes state to restore.
		{"%F{red}%z%h", "\x1b[31m<z><h>\x1b[31m", "an unlisted sequence changes no state"},
		// A color code never restores, however much is in effect.
		{"%H%F{red}a", "<H>\x1b[31ma", "setting a color restores nothing"},
		// The clear's own attribute is gone, so a restore after it skips it.
		{"%H%L%h", "<H><L><h><L>", "the cleared attribute is left out of its own restore"},
		// Every setting at once, cleared by the one that clears everything.
		{"%H%L%F{red}%K{blue}%l%h", "<H><L>\x1b[31m\x1b[44m<l><h>\x1b[31m\x1b[44m", "the whole state, minus what was cleared"},
		{"%H%N%L%F{red}%K{blue}%n%h", "<H><N><L>\x1b[31m\x1b[44m<n><h><L>\x1b[31m\x1b[44m", "and with the standout cleared instead"},
	} {
		if got := expandVisual(t, tc.text); got != tc.want {
			t.Errorf("%s: %q drew %q, want %q", tc.why, tc.text, got, tc.want)
		}
	}
}

// The state belongs to one walk. Two expansions of the same text draw the
// same bytes, so nothing a prompt set leaks into the next one drawn.
func TestVisualStateStartsEmptyAtEveryWalk(t *testing.T) {
	const text = "%F{red}a%h"
	first := expandVisual(t, text)
	if second := expandVisual(t, text); second != first {
		t.Errorf("the second walk drew %q where the first drew %q", second, first)
	}
	// And a walk that sets nothing restores nothing, after one that did.
	if got, want := expandVisual(t, "%h"), "<h>"; got != want {
		t.Errorf("a later walk drew %q, want %q — state carried over", got, want)
	}
}

// A dialect that says nothing about its sequences gets what it always got:
// the bytes, written through. This is the shape of every table but one, and
// asserting it is what keeps the restore from becoming a rule over the table
// rather than a row of it.
func TestASequenceWithNoVisualEntryIsWrittenAlone(t *testing.T) {
	st := visualStyle()
	st.Visual = nil
	got, _, ok := ExpandPromptStyle(st, "%F{red}a%h%L", nil, nil)
	if want := "\x1b[31ma<h><L>"; !ok || got != want {
		t.Errorf("with no Visual table, %%F{red}a%%h%%L drew %q (ok %v), want %q", got, ok, want)
	}
}
