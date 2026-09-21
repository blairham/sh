// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The asked half of the report: what a setting resolves to and which layer it
// came from, printed on request.
//
// docs/spec/prompt-theme.md keeps the two halves apart and the distinction is
// the reason both exist. "Is anything wrong" is **volunteered**, at the
// prompt, once each, because nobody thinks to ask it — that is the theme's
// Problems list. "What is my configuration" is **asked**, because a person
// who wants it is sitting in front of the shell typing a word, and printing it
// unasked on every prompt would be a broken shell rather than a report.
//
// Nothing here draws and nothing here renders a prompt. A Report is built from
// the same Settings, Roster and icon table a render would use, so what it says
// is what the next prompt will do rather than a second opinion about it.

// Report is what `prompt show` prints.
type Report struct {
	// Drawing says whether a configuration asked for a theme at all. False is
	// the ordinary state of a shell nobody has configured, and it is the first
	// line because every other line means something different underneath it.
	Drawing bool

	// Layers names the stack, earliest first — the preset, the file, the
	// session. A person reading a value that surprises them is reading this
	// to find out what else could have set it.
	Layers []string

	// Settings is every setting that resolves, with the layer that answered.
	Settings []Setting

	// Elements is each configured element and what draws it, in the order the
	// sides name them.
	Elements []Element

	// Icons is the table asked for and the one serving it, which differ
	// exactly when a name is not carried.
	Icons, IconsServed string

	// Notes is what the theme read and could not honor, carried through so
	// that one command answers both halves for a person who ran it after the
	// prompt had already said something and scrolled it away.
	//
	// Kept beside the settings rather than merged into them: a note is about
	// a setting that could *not* be honored, and a list mixing the two would
	// need a reader to tell them apart by wording.
	Notes []string

	// Partial says the settings list is what the enumerable layers hold and
	// not everything that would resolve.
	//
	// It is always true, and it is a field rather than a constant because the
	// sentence has to be printed: a session's variables are reached one name
	// at a time through a lookup, since no shell offers to enumerate a
	// namespace, so a setting a person typed at the prompt and no file
	// mentions is invisible here. A report that presented itself as complete
	// would be the silent-wrong-answer class applied to a report *about* that
	// class.
	Partial bool
}

// Setting is one resolved setting.
type Setting struct {
	Key   string
	Value string
	Layer string
}

// Element is one element of one side, and what answers it.
type Element struct {
	// Side is "left" or "right", and Line is which line of that side it is on
	// — an element after a `newline` is on the next one.
	Side string
	Line int

	// Name is the element as the configuration spelled it, lower-cased.
	Name string

	// Source is what draws it: a resolver's own name for one from outside the
	// binary, "built-in" for one compiled in, and empty for an element
	// nothing answers.
	Source string
}

// NotYet reports whether nothing draws this element.
func (e Element) NotYet() bool { return e.Source == "" }

// builtInSource is what Describe calls a segment compiled into this binary.
//
// A string rather than a bool, because the interesting cases are the other
// ones: a segment arriving from a session function or a plugin says so by
// name, and the point of printing any of it is that a person whose segment
// stopped drawing can see which of the three is answering.
const builtInSource = "built-in"

// globalKeys are the settings that are not per-segment: the layout, the
// frame, and what picks a preset or a table.
//
// Written down because a session's variables cannot be enumerated, so the
// only way to report a setting nobody wrote in a file is to ask for it by
// name. That makes this list the report's reach rather than the engine's —
// the engine reads what it reads whether or not a name is here — and a key
// added to the engine and not to this list is a setting `prompt show` will
// miss when only the session set it.
func globalKeys() []string {
	return []string{
		"ADD_NEWLINE",
		"CONFIG",
		"CONTINUATION",
		"FIRST_PREFIX", "FIRST_SUFFIX",
		"GAP_CHAR", "GAP_FOREGROUND",
		"ICONS",
		"LAST_PREFIX", "LAST_SUFFIX",
		"LEFT_ELEMENTS",
		"LEFT_END_SYMBOL",
		"LEFT_SEGMENT_SEPARATOR",
		"LEFT_START_SYMBOL",
		"LEFT_SUBSEGMENT_SEPARATOR",
		"MIDDLE_PREFIX", "MIDDLE_SUFFIX",
		"PRESET",
		"RIGHT_ELEMENTS",
		"RIGHT_END_SYMBOL",
		"RIGHT_SEGMENT_SEPARATOR",
		"RIGHT_START_SYMBOL",
		"RIGHT_SUBSEGMENT_SEPARATOR",
		"TRANSIENT",
	}
}

// segmentKeys are the per-segment settings, which are probed as
// <SEGMENT>_<KEY> for every element a configuration names.
//
// The first nine are the ones every segment resolves. The rest belong to one
// segment each, and they are probed against every element rather than against
// the segment that reads them: a list saying which key belongs to which
// segment would be a second table beside the segments themselves, and the
// cost of asking is one map lookup against a configuration nobody set.
func segmentKeys() []string {
	return []string{
		"BACKGROUND", "BOLD", "CONTENT", "FOREGROUND", "ICON",
		"PREFIX", "SUFFIX", "UNDERLINE", "WHITESPACE",
		"ALWAYS", "FORMAT", "MAX_DEPTH", "OK", "PRECISION",
		"SEGMENT_SEPARATOR", "START_SYMBOL", "SYMBOL", "THRESHOLD",
		"TRUNCATION",
	}
}

// Describe builds the report.
//
// The roster is asked how each element resolves without being asked to draw
// one, which is the whole reason Roster.Source exists: rendering to find out
// would run a person's shell functions because they typed `prompt show`.
func Describe(settings *Settings, roster *Roster, icons *IconSet, notes []string) Report {
	report := Report{
		Drawing: settings.Has("LEFT_ELEMENTS") || settings.Has("RIGHT_ELEMENTS"),
		Partial: true,
	}
	for _, layer := range settings.Layers() {
		report.Layers = append(report.Layers, layer.Name())
	}
	report.Elements = describeElements(settings, roster)
	report.Settings = describeSettings(settings, report.Elements)
	if icons != nil {
		report.Icons, report.IconsServed = icons.Name, icons.Served
	}
	report.Notes = slices.Clone(notes)
	return report
}

// describeElements walks each side's elements in order and asks what draws
// them.
func describeElements(settings *Settings, roster *Roster) []Element {
	var out []Element
	for _, side := range []struct {
		name string
		key  string
	}{{"left", "LEFT_ELEMENTS"}, {"right", "RIGHT_ELEMENTS"}} {
		line := 0
		for _, element := range settings.List(side.key) {
			name := strings.ToLower(strings.TrimSpace(element))
			if name == "newline" {
				line++
				continue
			}
			if name == "" {
				continue
			}
			source := ""
			if roster != nil {
				source, _ = roster.Source(name)
			}
			out = append(out, Element{Side: side.name, Line: line, Name: name, Source: source})
		}
	}
	return out
}

// describeSettings collects every setting that resolves, from the enumerable
// layers and from probing the names a session could have set.
func describeSettings(settings *Settings, elements []Element) []Setting {
	asked := map[string]bool{}
	var keys []string
	add := func(key string) {
		k := Key(key)
		if k == "" || asked[k] {
			return
		}
		asked[k] = true
		keys = append(keys, k)
	}
	for _, key := range settings.Keys() {
		add(key)
	}
	for _, key := range globalKeys() {
		add(key)
	}
	for _, element := range elements {
		segment := strings.ToUpper(strings.ReplaceAll(element.Name, "-", "_"))
		for _, key := range segmentKeys() {
			add(segment + "_" + key)
		}
	}
	sort.Strings(keys)

	out := make([]Setting, 0, len(keys))
	for _, key := range keys {
		value, layer, ok := settings.Lookup(key)
		if !ok {
			continue
		}
		out = append(out, Setting{Key: key, Value: value.Text(), Layer: layer})
	}
	return out
}

// Lines renders the report, one line at a time, for a caller that writes them
// to a stream.
//
// Plain text and not a table: a value may hold escape markup and a column
// aligned on its byte length would be aligned on nothing a person can see.
func (r Report) Lines() []string {
	var out []string
	if r.Drawing {
		out = append(out, "drawing: yes")
	} else {
		out = append(out, "drawing: no — no configuration names any elements")
	}
	if len(r.Layers) > 0 {
		out = append(out, "layers, later wins: "+strings.Join(r.Layers, ", "))
	}
	if r.Icons != "" || r.IconsServed != "" {
		line := "icons: " + or(r.Icons, DefaultIconTable)
		if r.IconsServed != "" && r.IconsServed != r.Icons {
			line += " (not carried; served by " + r.IconsServed + ")"
		}
		out = append(out, line)
	}
	if len(r.Settings) > 0 {
		out = append(out, "settings:")
		for _, s := range r.Settings {
			out = append(out, "  "+s.Key+" = "+s.Value+"    ["+s.Layer+"]")
		}
	}
	if len(r.Elements) > 0 {
		out = append(out, "elements:")
		for _, e := range r.Elements {
			source := e.Source
			if source == "" {
				source = "not yet — nothing draws it"
			}
			out = append(out, "  "+e.Side+" line "+strconv.Itoa(e.Line+1)+": "+e.Name+" — "+source)
		}
	}
	if len(r.Notes) > 0 {
		out = append(out, "notes:")
		for _, note := range r.Notes {
			out = append(out, "  "+note)
		}
	}
	if r.Partial {
		out = append(out, "a setting only the session set, and no file or preset mentions,")
		out = append(out, "is not listed: no shell offers to enumerate its variables.")
	}
	return out
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
