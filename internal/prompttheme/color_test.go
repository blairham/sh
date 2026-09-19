// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestTheThreeSpellingsResolve(t *testing.T) {
	for _, c := range []struct {
		spec       string
		foreground string
		background string
	}{
		{"4", "38;5;4", "48;5;4"},
		{"31", "38;5;31", "48;5;31"},
		{"255", "38;5;255", "48;5;255"},
		{"#1e66f5", "38;2;30;102;245", "48;2;30;102;245"},
		{"#abc", "38;2;170;187;204", "48;2;170;187;204"},
		{"blue", "38;5;4", "48;5;4"},
		{"bright-blue", "38;5;12", "48;5;12"},
		{"BLUE", "38;5;4", "48;5;4"},
		{"  blue  ", "38;5;4", "48;5;4"},
	} {
		color, ok := prompttheme.ParseColor(c.spec)
		if !ok {
			t.Errorf("ParseColor(%q) did not resolve", c.spec)
			continue
		}
		if got := color.Foreground(); got != c.foreground {
			t.Errorf("ParseColor(%q).Foreground = %q, want %q", c.spec, got, c.foreground)
		}
		if got := color.Background(); got != c.background {
			t.Errorf("ParseColor(%q).Background = %q, want %q", c.spec, got, c.background)
		}
	}
}

func TestALowIndexIsStillTheLongSpelling(t *testing.T) {
	// 38;5;N even for 0-7: it is the same color on every terminal that
	// supports either, and one code path is one fewer thing to get wrong.
	color, ok := prompttheme.ParseColor("1")
	if !ok || color.Foreground() != "38;5;1" {
		t.Errorf("ParseColor(1).Foreground = %q", color.Foreground())
	}
}

func TestAnUnrecognizedSpellingIsUnsetAndNotAWrongColor(t *testing.T) {
	for _, spec := range []string{"", "  ", "puce", "brblue", "bright-puce", "256", "-1", "#12", "#12345", "#gggggg"} {
		color, ok := prompttheme.ParseColor(spec)
		if ok || color.Set() {
			t.Errorf("ParseColor(%q) resolved to %q", spec, color.Foreground())
		}
		if got := color.Foreground(); got != "" {
			t.Errorf("an unset color emitted %q", got)
		}
	}
}

func TestAStyleWritesItsWholeAppearance(t *testing.T) {
	// The whole appearance at every change rather than a delta, because the
	// layout concatenates independently rendered pieces.
	fg, _ := prompttheme.ParseColor("31")
	bg, _ := prompttheme.ParseColor("#000000")
	style := prompttheme.Style{Fg: fg, Bg: bg, Bold: true, Underline: true}

	if got, want := style.SGR(), "\x1b[0;1;4;38;5;31;48;2;0;0;0m"; got != want {
		t.Errorf("SGR = %q, want %q", got, want)
	}
	if (prompttheme.Style{}).SGR() != prompttheme.Reset {
		t.Errorf("an empty style did not enter the terminal's own default")
	}
	if !(prompttheme.Style{}).Empty() {
		t.Error("the zero style reported something to draw")
	}
	if style.Empty() {
		t.Error("a style with four settings reported nothing to draw")
	}
}

func TestASegmentStyleInheritsEachSettingSeparately(t *testing.T) {
	blue, _ := prompttheme.ParseColor("blue")
	def := prompttheme.Style{Bg: blue, Bold: true}

	settings := prompttheme.NewSettings(assignments(t, "preset", "DIR_FOREGROUND", "red"))
	style := settings.SegmentStyle("dir", "", def)

	if got, want := style.Fg.Foreground(), "38;5;1"; got != want {
		t.Errorf("foreground = %q, want %q", got, want)
	}
	if got, want := style.Bg.Background(), "48;5;4"; got != want {
		t.Errorf("a background the configuration never mentioned became %q, want %q", got, want)
	}
	if !style.Bold {
		t.Error("a bold default was lost to a configuration that said nothing about it")
	}
}

func TestAnUnrecognizedSegmentColorLeavesTheTerminalAlone(t *testing.T) {
	blue, _ := prompttheme.ParseColor("blue")
	settings := prompttheme.NewSettings(assignments(t, "preset", "DIR_FOREGROUND", "puce"))
	style := settings.SegmentStyle("dir", "", prompttheme.Style{Fg: blue})

	if style.Fg.Set() {
		t.Errorf("an unreadable color fell back to the default rather than to nothing: %q", style.Fg.Foreground())
	}
}
