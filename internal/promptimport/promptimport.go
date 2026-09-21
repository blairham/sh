// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package promptimport converts another prompt program's configuration into
// this one's.
//
// docs/spec/prompt-theme.md's *Importing a configuration from another
// prompt*, and the sentence that shapes the whole package is there:
//
//	This is a converter, not a compatibility surface.
//
// Three consequences, and each is a property of the code rather than a
// promise about it. Import runs **once** and writes our settings into our
// configuration file. Nothing on the render path ever reads another
// project's file, name or format — nothing in `repl` or `internal/prompttheme`
// imports this package, and this package is reached only from the `prompt
// import` command. And the result is an ordinary configuration afterwards:
// editable, shareable, and indistinguishable from one written by hand.
//
// # What it may read, and why that is clear of CLEANROOM.md
//
// A **user's configuration file** is data. #1323 says so in as many words —
// "reading a user's config file is also entirely clear of CLEANROOM.md — it
// is data, not an implementation" — and the parameter names in it are the
// documented configuration surface of a program, which the green list covers
// as "public interface shape". Nothing here was learned from anybody's
// source, and nothing here reproduces anybody's implementation: a converter
// that reads a name and writes a different name is the opposite of a port.
//
// # The rule that keeps it honest
//
// **A setting that cannot be carried is named, never dropped and never
// half-interpreted into something that looks nearly right.** That is the
// silent-wrong-answer rule applied to a conversion, and it is the difference
// between a prompt that is missing a segment and a prompt that is confidently
// showing something else. Every converter here answers with what it carried
// *and* with a line per setting it did not.
package promptimport

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/prompttheme"
)

// Params is another program's configuration once it has been read, whatever
// reading it meant.
//
// Two maps rather than a file, because the two converters read their sources
// in completely different ways — one *evaluates* a shell program and harvests
// its parameter namespace, the other parses a text format — and what they have
// in common is what comes out. It is also what keeps this package free of an
// interpreter: the evaluation happens where there is a Runner, and what
// crosses into here is data.
type Params struct {
	// Scalars are the settings that hold one value.
	Scalars map[string]string

	// Arrays are the settings that hold a list. A name is in one map or the
	// other and never both, which is the same rule prompttheme.Value holds.
	Arrays map[string][]string
}

// Scalar reads one, or empty and false.
func (p Params) Scalar(name string) (string, bool) {
	v, ok := p.Scalars[name]
	return v, ok
}

// Names is every setting the source holds, sorted, so a report walks them in
// an order that does not change between runs.
func (p Params) Names() []string {
	var names []string
	for name := range p.Scalars {
		names = append(names, name)
	}
	for name := range p.Arrays {
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// Result is what a conversion produced.
type Result struct {
	// Settings is the configuration to write, in our own vocabulary.
	Settings *prompttheme.Store

	// Carried is how many of the source's settings were honored, and
	// NotCarried names every one that was not, with the reason.
	//
	// The count and the list are both here because neither is the report on
	// its own. A number says how much of a configuration arrived; the list
	// says what a person has to go and look at. Presenting only the first is
	// how "it looks the same" becomes an opinion again.
	Carried    int
	NotCarried []string
}

// note records a setting that was read and not honored.
func (r *Result) note(name, why string) {
	r.NotCarried = append(r.NotCarried, name+": "+why)
}

// carry stores a setting and counts it.
func (r *Result) carry(key, value string) {
	r.Settings.SetText(key, value)
	r.Carried++
}

// carryList stores a list setting and counts it.
func (r *Result) carryList(key string, values ...string) {
	r.Settings.SetList(key, values...)
	r.Carried++
}

// segmentKeys are the settings **every** segment resolves through the
// three-step chain.
//
// A converter checks against this before carrying `<SEGMENT>_<KEY>`, because
// writing a key nothing reads is the *set and ignored* state the spec calls
// the invisible one — the person configured something, the tool accepted it,
// and the prompt drew something else.
var segmentKeys = map[string]bool{
	"FOREGROUND": true, "BACKGROUND": true, "BOLD": true, "UNDERLINE": true,
	"WHITESPACE": true, "PREFIX": true, "SUFFIX": true, "CONTENT": true,
	"ICON": true,
	// The separators and the side symbols, which also resolve through the
	// chain so that one segment may carry its own without a preset having to
	// give every other segment one.
	"SEGMENT_SEPARATOR": true, "SUBSEGMENT_SEPARATOR": true,
	"START_SYMBOL": true, "END_SYMBOL": true,
}

// ownKeys are the settings **one** segment each reads, which is the spec's
// own table of what the first set reads beyond the common keys.
//
// Kept per segment rather than pooled with the universal ones, and the
// difference is not tidiness: pooled, a converter carried
// `COMMAND_EXECUTION_TIME_FORMAT` because `time` reads a `FORMAT` — a
// setting written into our file that nothing here would ever read, which is
// exactly the state this whole package is written to avoid.
var ownKeys = map[string]map[string]bool{
	"dir":                    {"MAX_DEPTH": true, "TRUNCATION": true},
	"status":                 {"OK": true},
	"command_execution_time": {"THRESHOLD": true, "PRECISION": true},
	"background_jobs":        {"ALWAYS": true},
	"context":                {"ALWAYS": true},
	"time":                   {"FORMAT": true},
	"prompt_char":            {"SYMBOL": true},
}

// globalKeys are the settings that are not per-segment.
var globalKeys = map[string]bool{
	"LEFT_ELEMENTS": true, "RIGHT_ELEMENTS": true,
	"LEFT_START_SYMBOL": true, "RIGHT_START_SYMBOL": true,
	"LEFT_END_SYMBOL": true, "RIGHT_END_SYMBOL": true,
	"LEFT_SEGMENT_SEPARATOR": true, "RIGHT_SEGMENT_SEPARATOR": true,
	"LEFT_SUBSEGMENT_SEPARATOR": true, "RIGHT_SUBSEGMENT_SEPARATOR": true,
	"FIRST_PREFIX": true, "FIRST_SUFFIX": true,
	"MIDDLE_PREFIX": true, "MIDDLE_SUFFIX": true,
	"LAST_PREFIX": true, "LAST_SUFFIX": true,
	"GAP_CHAR": true, "GAP_FOREGROUND": true,
	"ADD_NEWLINE": true, "CONTINUATION": true, "TRANSIENT": true,
	"ICONS": true, "PRESET": true,
	"FOREGROUND": true, "BACKGROUND": true, "BOLD": true, "UNDERLINE": true,
	"WHITESPACE": true, "PREFIX": true, "SUFFIX": true, "CONTENT": true,
	"ICON": true,
}

// Draws says whether this shell has a segment for an element name.
//
// Supplied by the caller rather than computed here, and that is the same
// call every other part of the engine makes: the roster is a session's, so
// the answer includes a shell function the person has already defined and a
// plugin they are already running. A converter with a list of its own would
// grade a configuration against a shell nobody is running.
//
// A nil predicate means nothing is known to draw, which is the honest answer
// for a caller that did not say — it carries the elements and names the
// settings, rather than pretending to know.
type Draws func(element string) bool

func (d Draws) draws(element string) bool { return d != nil && d(strings.ToLower(element)) }

// honored reports whether this engine reads a key.
//
// The element has to be one something **draws**, not merely one the source's
// list named: a `<SEGMENT>_<KEY>` for a segment this shell has no way to
// render is a line in our file that does nothing, and writing it would be
// the third state — set and ignored — arriving by the back door.
//
// The split is found rather than looked up, longest element name first, so
// `PROMPT_CHAR_OK_FOREGROUND` is tried as `prompt_char_ok` before
// `prompt_char`. An element name may hold an underscore and a state sits
// between the element and the key, so there is no other way to tell the
// three apart than to ask what this shell draws.
func honored(key string, draws Draws) bool {
	if globalKeys[key] {
		return true
	}
	for i := len(key) - 1; i > 0; i-- {
		if key[i] != '_' {
			continue
		}
		element := strings.ToLower(key[:i])
		if !draws.draws(element) {
			continue
		}
		rest := key[i+1:]
		if segmentKeys[rest] || ownKeys[element][rest] {
			return true
		}
		// A state in the middle: <SEGMENT>_<STATE>_<KEY>. The state is
		// whatever the segment calls itself in that condition, so it is not
		// checked against a list — only the key at the end is, because that
		// is the part the engine reads.
		if j := strings.LastIndex(rest, "_"); j > 0 {
			if last := rest[j+1:]; segmentKeys[last] || ownKeys[element][last] {
				return true
			}
		}
	}
	return false
}

// Write renders a converted configuration in this engine's own file format.
//
// The same format the configuration file is read in, because the point of a
// converter is that what comes out is an ordinary configuration: a person can
// open it, change a color and share it, and nothing about it says where it
// came from except the comment at the top saying where it came from.
//
// Values are written quoted **when the whitespace matters**, which is the one
// reason the format has quoting at all: a separator of a single space and a
// suffix of two are real settings, and unquoted the line that holds one is
// indistinguishable from a line that empties it.
func Write(source string, result Result) string {
	var b strings.Builder
	b.WriteString("# Imported from ")
	b.WriteString(source)
	b.WriteString("\n#\n")
	b.WriteString("# This is an ordinary configuration now: edit it, share it, point\n")
	b.WriteString("# SH_PROMPT_CONFIG at it. Nothing reads the file it came from again.\n")
	if len(result.NotCarried) > 0 {
		b.WriteString("#\n# What was read and not honored is listed by the import itself,\n")
		b.WriteString("# and there were ")
		b.WriteString(strconv.Itoa(len(result.NotCarried)))
		b.WriteString(" of them.\n")
	}
	b.WriteString("\n")
	for _, key := range result.Settings.Keys() {
		value, _ := result.Settings.Lookup(key)
		b.WriteString(key)
		b.WriteString(" = ")
		b.WriteString(quoteIfNeeded(value.Text()))
		b.WriteString("\n")
	}
	return b.String()
}

// quoteIfNeeded wraps a value whose leading or trailing whitespace is part of
// it, and leaves everything else as written.
func quoteIfNeeded(value string) string {
	if value == strings.TrimSpace(value) && !strings.Contains(value, "#") {
		return value
	}
	return `"` + value + `"`
}
