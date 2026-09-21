// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestAnIconTableThatIsNotCarriedIsServedByTheDefaultAndSaysSo(t *testing.T) {
	t.Parallel()
	// Silently substituting a different glyph set is how a prompt ends up
	// full of boxes with no explanation, so the substitution is named. The
	// table still draws: a person who named one wanted icons.
	set := prompttheme.LoadIcons("a-font-nobody-here-has")
	if set.Carried() {
		t.Fatal("a table this binary does not carry reported itself as carried")
	}
	if set.Served != prompttheme.DefaultIconTable {
		t.Errorf("served by %q, want the default", set.Served)
	}
	if glyph, ok := set.Glyph("DIR"); !ok || glyph == "" {
		t.Errorf("the substitute table drew no directory icon: %q %v", glyph, ok)
	}
}

func TestEveryCarriedTableIsCarriedAndTheDefaultIsOneOfThem(t *testing.T) {
	t.Parallel()
	carried := false
	for _, name := range prompttheme.IconTables() {
		if set := prompttheme.LoadIcons(name); !set.Carried() {
			t.Errorf("%s is listed as carried and is served by %s", name, set.Served)
		}
		if name == prompttheme.DefaultIconTable {
			carried = true
		}
	}
	if !carried {
		t.Errorf("the default table %q is not one of the carried ones", prompttheme.DefaultIconTable)
	}
}

func TestTheAsciiTableNeedsNoFontAndTheNoneTableDrawsNothing(t *testing.T) {
	t.Parallel()
	// A table whose glyphs might not exist cannot be the honest answer to "I
	// do not have that font", so every entry in the ascii table is a
	// character a terminal has had for decades.
	ascii := prompttheme.LoadIcons("ascii")
	for _, key := range []string{"DIR", "VCS_BRANCH", "STATUS_ERROR", "TIME"} {
		glyph, ok := ascii.Glyph(key)
		if !ok {
			t.Errorf("the ascii table has no %s", key)
			continue
		}
		for _, r := range glyph {
			if r > 0x7e {
				t.Errorf("the ascii table's %s is %q, which is not ascii", key, glyph)
				break
			}
		}
	}
	none := prompttheme.LoadIcons("none")
	if glyph, ok := none.Glyph("DIR"); ok {
		t.Errorf("the none table answered %q", glyph)
	}
}

func TestTheShippedTablesAreSeededFromTheConfigurationFormat(t *testing.T) {
	t.Parallel()
	// A shipped table that were a Go map would be an arrangement a downloaded
	// one could not express, and the spec's claim is that the two are the same
	// format. This is that claim: a table written the way a person would write
	// one is read by the same reader and answers the same way.
	store, err := prompttheme.ParseFile("icons:test", strings.NewReader("DIR = \"[d] \"\n# a comment\n"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	v, ok := store.Lookup("DIR")
	if !ok || v.Text() != "[d] " {
		t.Errorf("a hand-written table read as %q %v", v.Text(), ok)
	}
	// And the codepoint spelling the shipped tables use is read, so a glyph
	// outside the keyboard can be written down as what it is.
	if glyph, _ := prompttheme.LoadIcons("nerdfont").Glyph("DIR"); strings.Contains(glyph, `\u`) {
		t.Errorf("a shipped table kept its escape unread: %q", glyph)
	}
}

func TestAConfigurationMayNameASegmentsGlyphItself(t *testing.T) {
	t.Parallel()
	// The ICON setting resolves through the same three-step chain as every
	// other per-segment setting, so the bare key is the global override the
	// table has to be able to default to — and setting it to empty is how one
	// icon is suppressed without turning the table off.
	engine := newEngine(t, assignments(t, "preset", "LEFT_ELEMENTS", "one", "ONE_ICON", "<> "))
	engine.Icons = func(string) (string, bool) { return "TABLE", true }
	if got := plain(engine.Render(&prompttheme.Context{}).Text); !strings.HasPrefix(got, "<> ") {
		t.Errorf("a named icon drew %q", got)
	}

	global := newEngine(t, assignments(t, "preset", "LEFT_ELEMENTS", "one", "ICON", "* "))
	global.Icons = func(string) (string, bool) { return "TABLE", true }
	if got := plain(global.Render(&prompttheme.Context{}).Text); !strings.HasPrefix(got, "* ") {
		t.Errorf("a global icon override drew %q", got)
	}

	off := newEngine(t, assignments(t, "preset", "LEFT_ELEMENTS", "one", "ONE_ICON", ""))
	off.Icons = func(string) (string, bool) { return "TABLE", true }
	if got := plain(off.Render(&prompttheme.Context{}).Text); got != "ONE " {
		t.Errorf("a suppressed icon drew %q", got)
	}
}
