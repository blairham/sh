// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"strings"

	"github.com/blairham/sh/internal/prompttheme"
)

// The powerlevel10k converter.
//
// Its configuration is a zsh program, and the spec's decision is to
// **evaluate it rather than parse it**. That is not a refinement: two shapes
// in a real generated configuration defeat a text reader outright, and this
// machine's own file has both.
//
//   - The **version gate the file opens with**, a range inside a group,
//     which decides whether any of the configuration is applied at all.
//   - **Names built by brace expansion.** One line spelling
//     `PROMPT_CHAR_{OK,ERROR}_VIINS_CONTENT_EXPANSION` is *two* settings. A
//     text reader has to reimplement brace expansion to see them; an
//     evaluator gets them for free.
//
// A parser would have to become a zsh interpreter to be correct. We have one,
// and it is in the process — so what arrives here is a parameter namespace
// and not a file, and the evaluation happens where there is a Runner. See
// repl/themeimport.go.
//
// # Why most of it is a pass-through and that is not a coincidence
//
// This engine's namespace was designed on the same shape — a flat keyed
// store, `<SEGMENT>_<STATE>_<KEY>`, colors as xterm-256 indices — because
// that shape is what a rich prompt's configuration surface has to be, not
// because anybody was copying. The consequence is that once the prefix is
// off, most names already say what they mean here: `DIR_FOREGROUND`,
// `VCS_FOREGROUND`, `STATUS_OK_FOREGROUND`, `LEFT_SEGMENT_SEPARATOR`.
//
// So the converter is three tables and a rule:
//
//  1. rename the names whose spelling differs;
//  2. translate the values whose *language* differs;
//  3. carry a name this engine reads, and **name** one it does not.
//
// The third is the whole honesty of it. A configuration written for another
// program says far more than this engine can honor, and writing those
// settings into our file anyway would produce the *set and ignored* state the
// spec calls the invisible one.

// p10kPrefix is the parameter prefix a powerlevel10k configuration uses.
const p10kPrefix = "POWERLEVEL9K_"

// p10kRenames maps a name, once the prefix is off, to this engine's spelling.
//
// Only the ones that differ. A name not in here keeps its spelling and is
// then checked against what this engine reads, which is what makes adding a
// setting to the engine also add it to the importer.
var p10kRenames = map[string]string{
	"LEFT_PROMPT_ELEMENTS":  "LEFT_ELEMENTS",
	"RIGHT_PROMPT_ELEMENTS": "RIGHT_ELEMENTS",

	"LEFT_PROMPT_FIRST_SEGMENT_START_SYMBOL":  "LEFT_START_SYMBOL",
	"RIGHT_PROMPT_FIRST_SEGMENT_START_SYMBOL": "RIGHT_START_SYMBOL",
	"LEFT_PROMPT_LAST_SEGMENT_END_SYMBOL":     "LEFT_END_SYMBOL",
	"RIGHT_PROMPT_LAST_SEGMENT_END_SYMBOL":    "RIGHT_END_SYMBOL",

	"MULTILINE_FIRST_PROMPT_PREFIX":   "FIRST_PREFIX",
	"MULTILINE_NEWLINE_PROMPT_PREFIX": "MIDDLE_PREFIX",
	"MULTILINE_LAST_PROMPT_PREFIX":    "LAST_PREFIX",
	"MULTILINE_FIRST_PROMPT_SUFFIX":   "FIRST_SUFFIX",
	"MULTILINE_NEWLINE_PROMPT_SUFFIX": "MIDDLE_SUFFIX",
	"MULTILINE_LAST_PROMPT_SUFFIX":    "LAST_SUFFIX",

	"MULTILINE_FIRST_PROMPT_GAP_CHAR":       "GAP_CHAR",
	"MULTILINE_FIRST_PROMPT_GAP_FOREGROUND": "GAP_FOREGROUND",

	"PROMPT_ADD_NEWLINE": "ADD_NEWLINE",
	"SHORTEN_DIR_LENGTH": "DIR_MAX_DEPTH",
	"SHORTEN_DELIMITER":  "DIR_TRUNCATION",
	"MODE":               "ICONS",
}

// Powerlevel10k converts a harvested parameter namespace.
//
// The elements lists are read first because everything else is graded against
// them: a per-segment setting for an element no side names is a setting this
// engine would never read, and carrying it would be writing into our file a
// line that does nothing.
func Powerlevel10k(p Params, draws Draws) Result {
	out := Result{Settings: prompttheme.NewStore("import:powerlevel10k")}

	elements := p10kElements(p, draws, &out)
	if len(elements) == 0 {
		// Nothing was applied. Almost always the version gate the file opens
		// with, which is the shape the spec names — and it is worth saying
		// plainly, because an empty configuration and a configuration that
		// declined to apply itself look identical afterwards.
		out.note(p10kPrefix+"LEFT_PROMPT_ELEMENTS",
			"no elements at all, so nothing in the file was applied — "+
				"the version gate it opens with is the usual reason, and it reads $ZSH_VERSION")
	}

	for _, name := range p.Names() {
		key, ok := strings.CutPrefix(name, p10kPrefix)
		if !ok {
			continue
		}
		if key == "LEFT_PROMPT_ELEMENTS" || key == "RIGHT_PROMPT_ELEMENTS" {
			continue // carried above, because everything else is graded on them
		}
		p10kSetting(p, key, name, draws, &out)
	}
	return out
}

// p10kElements carries the two elements lists.
//
// A list in the source may be an array or a scalar, and both are read: the
// namespace holds either and a configuration written in a shell with no
// arrays would say it the second way. `newline` passes through unchanged
// because both spell the line break that way.
func p10kElements(p Params, draws Draws, out *Result) []string {
	var all []string
	for _, side := range []struct{ from, to string }{
		{"LEFT_PROMPT_ELEMENTS", "LEFT_ELEMENTS"},
		{"RIGHT_PROMPT_ELEMENTS", "RIGHT_ELEMENTS"},
	} {
		name := p10kPrefix + side.from
		values, ok := p.Arrays[name]
		if !ok {
			text, found := p.Scalar(name)
			if !found {
				continue
			}
			values = strings.Fields(text)
		}
		// Only the elements something here draws. An imported configuration
		// naming forty this shell has no segment for is a prompt that
		// reports forty problems on its first line — and the person did not
		// write those names here, so they cannot read them as their own
		// typo. Each is named at import instead, which is where they can do
		// something about it: the resolution order puts a shell function
		// first, so putting one back is writing the function and putting the
		// name back in the list.
		var kept []string
		for _, element := range values {
			if element == "newline" || draws.draws(element) {
				kept = append(kept, element)
				continue
			}
			out.note(element, "no segment here draws that element, so it was left out of "+side.to)
		}
		out.carryList(side.to, kept...)
		all = append(all, kept...)
	}
	return all
}

// p10kSetting converts one setting, or names it.
func p10kSetting(p Params, key, name string, draws Draws, out *Result) {
	if renamed, ok := p10kRenames[key]; ok {
		key = renamed
	}
	key, ok := p10kNarrowed(key, name, out)
	if !ok {
		return
	}

	text, isScalar := p.Scalar(name)
	if !isScalar {
		// A list in a place this engine has no list for. Named rather than
		// joined into a scalar, because joining is exactly the kind of
		// nearly-right the spec refuses.
		out.note(name, "holds a list and this engine has no list setting of that name")
		return
	}

	value, ok := p10kValue(key, text, name, out)
	if !ok {
		return
	}
	if !honored(key, draws) {
		out.note(name, "there is no "+key+" in this engine's vocabulary")
		return
	}
	out.carry(key, value)
}

// p10kSuffixes are the side symbols, which a configuration may spell
// globally or per segment — `PROMPT_CHAR_LEFT_PROMPT_LAST_SEGMENT_END_SYMBOL`
// is the same setting as `LEFT_PROMPT_LAST_SEGMENT_END_SYMBOL` with a segment
// in front of it.
//
// A suffix table rather than more rows in p10kRenames, because the per-segment
// forms are one row each times every segment, and this engine resolves them
// through the same three-step chain the global one is the last step of.
var p10kSuffixes = [][2]string{
	{"_LEFT_PROMPT_FIRST_SEGMENT_START_SYMBOL", "_LEFT_START_SYMBOL"},
	{"_RIGHT_PROMPT_FIRST_SEGMENT_START_SYMBOL", "_RIGHT_START_SYMBOL"},
	{"_LEFT_PROMPT_LAST_SEGMENT_END_SYMBOL", "_LEFT_END_SYMBOL"},
	{"_RIGHT_PROMPT_LAST_SEGMENT_END_SYMBOL", "_RIGHT_END_SYMBOL"},
	{"_VISUAL_IDENTIFIER_EXPANSION", "_ICON"},
}

// p10kNarrowed handles the names whose *shape* differs rather than their
// spelling, and names the ones that cannot survive the narrowing.
func p10kNarrowed(key, name string, out *Result) (string, bool) {
	// The per-segment spellings, including the visual identifier — which in
	// this engine is an icon, since an icon here names a table entry or is a
	// glyph a configuration chose, the same thing one setting along.
	for _, pair := range p10kSuffixes {
		if trimmed, ok := strings.CutSuffix(key, pair[0]); ok && trimmed != "" {
			return trimmed + pair[1], true
		}
	}
	// A content expansion is a content template, and this is where most of
	// what cannot be carried lives — see p10kValue, which refuses one that
	// is shell code.
	if trimmed, ok := strings.CutSuffix(key, "_CONTENT_EXPANSION"); ok {
		key = trimmed + "_CONTENT"
	}
	// The vi-mode states. This engine has no editing mode in its lookup
	// chain, so the insert-mode value is the one a prompt draws and the
	// other three are named rather than silently applied to every mode.
	for _, mode := range []string{"_VIINS", "_VICMD", "_VIVIS", "_VIOWR"} {
		i := strings.Index(key, mode+"_")
		if i < 0 {
			continue
		}
		if mode != "_VIINS" {
			out.note(name, "this engine has no "+strings.ToLower(mode[1:])+
				" editing mode, so only the insert-mode value is carried")
			return "", false
		}
		key = key[:i] + key[i+len(mode):]
	}
	return key, true
}

// p10kValue translates a value whose *language* differs, and refuses one
// whose language is shell.
func p10kValue(key, text, name string, out *Result) (string, bool) {
	switch {
	case key == "ICONS":
		return p10kIconTable(text, name, out)
	case key == "TIME_FORMAT":
		// The clock's format is wrapped in zsh's own date expansion. What
		// this engine reads is the format itself, through the interpreter's
		// strftime — one reader of a format language, which is why that
		// function is exported at all.
		if inner, ok := strings.CutPrefix(text, "%D{"); ok {
			if inner, closed := strings.CutSuffix(inner, "}"); closed {
				return inner, true
			}
		}
		return text, true
	case strings.HasSuffix(key, "_CONTENT") || strings.HasSuffix(key, "_ICON"):
		// The case the spec says decides "most" from "all". A value holding
		// a parameter expansion, a command substitution or arithmetic is
		// shell code, and this engine's templates substitute rather than
		// expand — so carrying it would draw the source text of a program
		// where a person expects its output. Named instead.
		if strings.ContainsAny(text, "$`") {
			out.note(name, "its value is shell code (`"+summary(text)+"`), and a content "+
				"template here substitutes rather than expands — write it as a shell "+
				"function and name the function as an element")
			return "", false
		}
		return text, true
	}
	return text, true
}

// p10kIconTable maps an icon-mode name onto the tables this binary carries.
//
// The nerd-font modes all mean "a patched font is installed", which is the
// only question this engine's table asks. A mode naming something else is
// named rather than guessed at: a glyph this tree is not sure of is a box on
// somebody's screen.
func p10kIconTable(text, name string, out *Result) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(mode, "nerdfont"), strings.HasPrefix(mode, "awesome"),
		strings.HasPrefix(mode, "flat"), strings.HasPrefix(mode, "powerline"):
		return prompttheme.DefaultIconTable, true
	case mode == "ascii", mode == "compatible":
		return "ascii", true
	}
	out.note(name, "there is no icon table here for "+text+
		"; the default is serving and `prompt show` says so")
	return "", false
}

// summary shortens a value for a report line, so a 200-character expansion
// does not become the whole report.
func summary(text string) string {
	const most = 48
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= most {
		return text
	}
	return text[:most] + "…"
}
