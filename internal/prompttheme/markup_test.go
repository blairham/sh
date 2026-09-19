// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestTheMarkupVocabularyPaints(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"%F{red}x%f", "\x1b[0;38;5;1mx\x1b[0m"},
		{"%K{4}x%k", "\x1b[0;48;5;4mx\x1b[0m"},
		{"%Bx%b", "\x1b[0;1mx\x1b[0m"},
		{"%Ux%u", "\x1b[0;4mx\x1b[0m"},
		{"%F{#1e66f5}╭─", "\x1b[0;38;2;30;102;245m╭─\x1b[0m"},
	} {
		if got := prompttheme.Expand(c.value); got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestAValueThatPaintsNothingEmitsNothing(t *testing.T) {
	// The layout tells a segment that rendered from one that declined by
	// looking at what came back, so a value with no markup must come back
	// as itself and not as itself wrapped in a reset.
	for _, value := range []string{"", "plain", "100%%", "~/src"} {
		got := prompttheme.Expand(value)
		if strings.Contains(got, "\x1b") {
			t.Errorf("Expand(%q) = %q, which instructs the terminal", value, got)
		}
	}
	if got := prompttheme.Expand("100%%"); got != "100%" {
		t.Errorf("a doubled percent read as %q", got)
	}
}

func TestAnythingOutsideTheVocabularyPassesThrough(t *testing.T) {
	for _, value := range []string{"%Z", "%", "%F", "%K{unterminated", "$NAME", "${a-b}", "$(date)", "$((1+1))", "50% done"} {
		if got := prompttheme.Expand(value); got != value {
			t.Errorf("Expand(%q) = %q, want it untouched", value, got)
		}
	}
}

func TestAnUnrecognizedColorInMarkupPaintsNothing(t *testing.T) {
	// %F{puce} is in the vocabulary and its color is not, so the foreground
	// goes back to the terminal's own rather than to a guess.
	if got, want := prompttheme.Expand("%F{puce}x"), "\x1b[0mx\x1b[0m"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestAReferenceReachesTheCallersLookup(t *testing.T) {
	e := prompttheme.Expander{Lookup: func(name string) string {
		if name == "CONTENT" {
			return "main"
		}
		return ""
	}}

	if got, want := e.Expand("${CONTENT}"), "main"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
	if got, want := e.Expand("[${CONTENT}][${ICON}]"), "[main][]"; got != want {
		t.Errorf("an unknown reference did not expand to nothing: %q", got)
	}
	if got, want := prompttheme.Expand("${CONTENT}"), ""; got != want {
		t.Errorf("a reference with no lookup at all became %q", got)
	}
}

func TestTheAppearanceIsWrittenInFullAtEveryChange(t *testing.T) {
	// Not a delta: the layout concatenates pieces rendered independently, so
	// every escape has to say the whole appearance it means.
	got := prompttheme.Expand("%F{red}%Bx%K{4}y")
	want := "\x1b[0;38;5;1m" + "\x1b[0;1;38;5;1m" + "x" + "\x1b[0;1;38;5;1;48;5;4m" + "y" + "\x1b[0m"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestMarkWrapsEveryRunOfEscapes(t *testing.T) {
	e := prompttheme.Expander{Mark: func(escapes string) string { return "\x01" + escapes + "\x02" }}
	got := e.Expand("%F{red}x")
	want := "\x01\x1b[0;38;5;1m\x02x\x01\x1b[0m\x02"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}

	// Everything the editor would measure is exactly the text that occupies
	// cells, which is the whole point of marking.
	var visible strings.Builder
	marked := false
	for _, r := range got {
		switch r {
		case '\x01':
			marked = true
		case '\x02':
			marked = false
		default:
			if !marked {
				visible.WriteRune(r)
			}
		}
	}
	if visible.String() != "x" {
		t.Errorf("the unmarked text was %q, want x", visible.String())
	}
}
