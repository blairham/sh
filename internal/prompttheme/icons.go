// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"strings"
	"sync"
)

// Icons live in a table, not in the segments.
//
// A segment that hardcoded its glyph would make SH_PROMPT_ICONS=ascii a lie,
// and would leave a global icon override with nothing to default to. So a
// segment names a key and never draws a glyph, and every glyph in the engine
// is in one of the tables below.
//
// The tables are written in the configuration file's own format and parsed by
// its own reader. That is not tidiness: the spec's rule is that a glyph set
// for a font this tree has never seen is a file you point a variable at, and
// a shipped table that were a Go map would be an arrangement a downloaded one
// could not express. Seeding the compiled tables from the same format is what
// makes that claim checkable rather than aspirational.

// IconTables names the tables this binary carries.
//
// A table named by a configuration and not carried is served by the default
// and said so, rather than silently substituted: a prompt full of boxes with
// no explanation is the same silent-wrong-answer class as a prompt showing
// the wrong branch, one decoration down.
func IconTables() []string { return []string{"nerdfont", "ascii", "none"} }

// DefaultIconTable is what an unset SH_PROMPT_ICONS asks for, and what serves
// a table that was asked for and is not carried.
const DefaultIconTable = "nerdfont"

// asciiIcons is the table for a terminal with no icon font, and the fallback
// spelling for anything the nerdfont table has no confident glyph for.
//
// Every entry is a character that has been in terminals for decades. A table
// whose glyphs might not exist is a table that cannot be the honest answer to
// "I do not have that font".
const asciiIcons = `
DIR             = ""
DIR_HOME        = "~"
VCS             = "on "
VCS_BRANCH      = ""
VCS_DETACHED    = "@"
VCS_TAG         = "#"
STATUS_OK       = ""
STATUS_ERROR    = ""
TIME            = ""
CONTEXT         = ""
BACKGROUND_JOBS = ""
COMMAND_EXECUTION_TIME = ""
`

// nerdfontIcons is the default, and it carries a glyph only where the
// codepoint is one of the long-standing ones a Nerd Font patch places at a
// fixed position. Where it is not, the entry is the ascii spelling rather
// than a guess: a glyph this tree is not sure of is a box on somebody's
// screen, and a box says nothing about what the segment is.
const nerdfontIcons = `
DIR             = " "
DIR_HOME        = " "
VCS             = " "
VCS_BRANCH      = " "
VCS_DETACHED    = " "
VCS_TAG         = " "
STATUS_OK       = " "
STATUS_ERROR    = " "
TIME            = " "
CONTEXT         = ""
BACKGROUND_JOBS = " "
COMMAND_EXECUTION_TIME = " "
`

// IconSet resolves a segment's icon key to a glyph.
type IconSet struct {
	// Name is the table that was asked for, and Served is the one answering.
	// They differ when a configuration named a table this binary does not
	// carry, and the difference is what a report says out loud.
	Name   string
	Served string

	store *Store
}

// Carried reports whether the table that was asked for is the one answering.
func (s *IconSet) Carried() bool { return s.Name == s.Served }

// Glyph answers an icon key, or declines. A key the table does not hold draws
// nothing, which is what an icon set that deliberately omits one should get.
func (s *IconSet) Glyph(key string) (string, bool) {
	if s == nil || s.store == nil {
		return "", false
	}
	v, ok := s.store.Lookup(key)
	if !ok {
		return "", false
	}
	return v.Text(), true
}

// shippedIcons parses the compiled tables once. Through the configuration
// file's own reader, so that a table shipped here and a table loaded from a
// path cannot diverge in what they can say.
var shippedIcons = sync.OnceValue(func() map[string]*Store {
	tables := map[string]string{
		"nerdfont": nerdfontIcons,
		"ascii":    asciiIcons,
		"none":     "",
	}
	parsed := make(map[string]*Store, len(tables))
	for name, text := range tables {
		store, _ := ParseFile("icons:"+name, strings.NewReader(text))
		for _, key := range store.Keys() {
			v, _ := store.Lookup(key)
			store.SetText(key, unescape(v.Text()))
		}
		parsed[name] = store
	}
	return parsed
})

// LoadIcons returns the table a configuration asked for.
//
// An unrecognized name is served by the default and says so through Carried,
// rather than resolving to no icons at all: a person who misspells a table
// name wanted icons, and drawing none of them is a quieter wrong answer than
// drawing the default ones and naming the mistake.
func LoadIcons(name string) *IconSet {
	asked := strings.ToLower(strings.TrimSpace(name))
	if asked == "" {
		asked = DefaultIconTable
	}
	tables := shippedIcons()
	if store, ok := tables[asked]; ok {
		return &IconSet{Name: asked, Served: asked, store: store}
	}
	return &IconSet{Name: asked, Served: DefaultIconTable, store: tables[DefaultIconTable]}
}

// unescape reads the one escape a table needs: \uXXXX, so a glyph outside the
// keyboard can be written down in a file as the codepoint it is.
//
// Deliberately only that, and deliberately only here. A configuration value
// is drawn as it stands everywhere else in this namespace — a directory
// holding a backslash is a directory and not an escape — and widening this to
// the general settings reader would make that untrue.
func unescape(text string) string {
	if !strings.Contains(text, `\u`) {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+5 >= len(text) || text[i+1] != 'u' {
			b.WriteByte(text[i])
			continue
		}
		r, ok := hexRune(text[i+2 : i+6])
		if !ok {
			b.WriteByte(text[i])
			continue
		}
		b.WriteRune(r)
		i += 5
	}
	return b.String()
}

func hexRune(digits string) (rune, bool) {
	var r rune
	for i := range len(digits) {
		d := digits[i]
		switch {
		case d >= '0' && d <= '9':
			r = r<<4 | rune(d-'0')
		case d >= 'a' && d <= 'f':
			r = r<<4 | rune(d-'a'+10)
		case d >= 'A' && d <= 'F':
			r = r<<4 | rune(d-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}
