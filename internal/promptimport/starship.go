// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/prompttheme"
)

// The starship converter, which is a second converter and not the same one.
//
// Its configuration is TOML with a different model — a top-level `format`
// string naming modules, a `$fill` that pushes the rest of a line to the
// right, and per-module tables rather than a flat namespace — so it shares
// the fidelity harness and the output format with the powerlevel10k
// converter and shares no code with the reading half.
//
// It is second because powerlevel10k is what is in use. It is worth doing
// because the two together are most of the themed prompts in the world, and
// because a person who has hand-translated one into the other has given the
// fidelity harness a fixture nothing else could: **the same prompt from two
// sources**, which is a stronger test than either alone.
//
// # What it carries and what it names
//
// The parts of starship's model this engine has: the module order, per-module
// colors, the symbol, and the handful of settings that are the same question
// in both — a duration threshold, a clock format, a directory truncation.
//
// The part it does not is the one to be clear about. A starship module
// `format` is **its own markup language** — `' [$symbol$version]($style)'` —
// and this engine's content template is a different one. Translating between
// them is where a converter stops being a converter and starts being a
// reimplementation of somebody's renderer, which is exactly what
// docs/design/plugins.md's bridge argument says not to take on. So a module
// format is **named** rather than half-interpreted into something that looks
// nearly right. The two exceptions are the prompt character's symbols, whose
// shape is a single `[text](style)` and is translated exactly or named.
//
// Nothing here forks a process either, which is the other half of what cannot
// be identical: starship runs `node --version` to fill a version module and
// no segment here forks. Those modules are named at import.

// starshipElements maps a starship module onto the element this engine
// draws.
//
// Only where the two are the same thing. A module with no counterpart is
// **named at import** rather than carried, because an element nothing draws
// is a line in the configuration that does nothing and a complaint at every
// prompt — and the person did not write it here, so they cannot read it as
// their own typo.
var starshipElements = map[string]string{
	"directory":    "dir",
	"git_branch":   "vcs",
	"character":    "prompt_char",
	"status":       "status",
	"cmd_duration": "command_execution_time",
	"jobs":         "background_jobs",
	"time":         "time",
	"username":     "context",
}

// starshipKeys maps a module's own settings onto this engine's per-segment
// keys, where the two ask the same question.
var starshipKeys = map[string]string{
	"symbol":            "ICON",
	"truncation_length": "MAX_DEPTH",
	"truncation_symbol": "TRUNCATION",
	"time_format":       "FORMAT",
}

// Starship converts a starship configuration.
func Starship(text string, draws Draws) (Result, error) {
	doc, err := parseTOML(text)
	if err != nil {
		return Result{}, err
	}
	out := Result{Settings: prompttheme.NewStore("import:starship")}

	if v, ok := doc.get("", "add_newline"); ok {
		out.carry("ADD_NEWLINE", v.Text)
	}
	elements := starshipFormat(doc, &out)

	for _, table := range doc.names() {
		if table == "" {
			starshipRoot(doc, &out)
			continue
		}
		starshipModule(doc, table, elements, draws, &out)
	}
	return out, nil
}

// starshipRoot names the top-level keys that are not the two carried above.
func starshipRoot(doc *tomlDoc, out *Result) {
	for _, key := range doc.keys("") {
		switch key {
		case "add_newline", "format":
			continue
		case "right_format":
			out.note(key, "this engine reads the right side out of `format`'s `$fill`, "+
				"so a separate right format would be two answers to one question")
		default:
			out.note(key, "there is no setting of that kind here")
		}
	}
}

// starshipFormat reads the module order out of `format`.
//
// Three things are in that one string and each maps onto something here. A
// newline splits the prompt into lines, which is this engine's `newline`
// element. A `$fill` splits a line into its two sides, which is this
// engine's left and right elements for that line. And the modules in order
// are the elements.
func starshipFormat(doc *tomlDoc, out *Result) map[string]bool {
	format, ok := doc.get("", "format")
	if !ok {
		out.note("format", "the configuration names no module order, so nothing is drawn")
		return nil
	}
	drawn := map[string]bool{}
	var left, right []string
	for i, line := range strings.Split(format.Text, "\n") {
		if i > 0 {
			left = append(left, "newline")
			right = append(right, "newline")
		}
		before, after, split := strings.Cut(line, "$fill")
		for _, module := range starshipModules(before, out) {
			left = append(left, module)
			drawn[module] = true
		}
		if !split {
			continue
		}
		for _, module := range starshipModules(after, out) {
			right = append(right, module)
			drawn[module] = true
		}
	}
	if len(left) > 0 {
		out.carryList("LEFT_ELEMENTS", trimNewlines(left)...)
	}
	if len(right) > 0 {
		out.carryList("RIGHT_ELEMENTS", trimNewlines(right)...)
	}
	return drawn
}

// starshipModules reads `$one$two` into the elements this engine draws, and
// names the modules it does not.
func starshipModules(text string, out *Result) []string {
	var elements []string
	for _, piece := range strings.Split(text, "$") {
		name := strings.TrimSpace(piece)
		// A module reference is a bare name; anything else on the line is
		// literal text starship draws between modules, which this engine has
		// no place for and which is almost always a space.
		if name == "" || !plainModuleName(name) {
			continue
		}
		element, ok := starshipElements[name]
		if !ok {
			out.note(name, "no segment here draws that module")
			continue
		}
		elements = append(elements, element)
	}
	return elements
}

// starshipModule converts one module's table.
func starshipModule(doc *tomlDoc, table string, drawn map[string]bool, draws Draws, out *Result) {
	element, known := starshipElements[table]
	if !known {
		out.note("["+table+"]", "no segment here draws that module")
		return
	}
	segment := strings.ToUpper(element)
	if v, ok := doc.get(table, "disabled"); ok && v.Text == "true" {
		out.note("["+table+"] disabled", "the module is off, so the element was not carried")
		return
	}
	if !drawn[element] {
		out.note("["+table+"]", "the module is configured but `format` never names it")
	}

	for _, key := range doc.keys(table) {
		value, _ := doc.get(table, key)
		starshipSetting(table, key, value, segment, draws, out)
	}
}

// starshipSetting converts one key of one module.
func starshipSetting(table, key string, value tomlValue, segment string, draws Draws, out *Result) {
	switch key {
	case "disabled":
		return
	case "style", "style_user":
		starshipStyle(value.Text, segment, "", out)
		return
	case "style_root":
		starshipStyle(value.Text, segment, "ROOT", out)
		return
	case "format":
		out.note("["+table+"] format",
			"a module format is starship's own markup (`"+summary(value.Text)+"`) and this "+
				"engine's content template is a different one; translating between them is "+
				"where a converter would start guessing")
		return
	case "min_time":
		// Milliseconds there, seconds here, and the conversion is exact
		// rather than rounded to taste: a threshold of 3000 is three
		// seconds and a threshold of 2500 is not two.
		ms, err := strconv.Atoi(value.Text)
		if err != nil || ms%1000 != 0 {
			out.note("["+table+"] min_time",
				"this engine's threshold is whole seconds and "+value.Text+"ms is not")
			return
		}
		out.carry(segment+"_THRESHOLD", strconv.Itoa(ms/1000))
		return
	case "success_symbol", "error_symbol":
		state := "OK"
		if key == "error_symbol" {
			state = "ERROR"
		}
		starshipSymbol(table, key, value.Text, segment, state, out)
		return
	}
	mapped, ok := starshipKeys[key]
	if !ok {
		out.note("["+table+"] "+key, "there is no setting of that kind here")
		return
	}
	if !honored(segment+"_"+mapped, draws) {
		out.note("["+table+"] "+key, "there is no "+segment+"_"+mapped+" in this engine's vocabulary")
		return
	}
	out.carry(segment+"_"+mapped, value.Text)
}

// starshipSymbol translates the one markup shape worth translating: a whole
// value that is `[text](style)`.
//
// The prompt character is the most visible glyph on the screen and its
// configuration is always this shape, so carrying it is the difference
// between an imported prompt that looks right and one that is missing the
// character a person types at. Anything else is named: a partial translation
// of a markup language is the nearly-right this converter refuses.
func starshipSymbol(table, key, text, segment, state string, out *Result) {
	inner, rest, ok := strings.Cut(strings.TrimSpace(text), "](")
	if !ok || !strings.HasPrefix(inner, "[") {
		out.carry(segment+"_"+state+"_CONTENT", text)
		return
	}
	style, closed := strings.CutSuffix(rest, ")")
	if !closed {
		out.note("["+table+"] "+key, "its value is starship markup this converter does not read")
		return
	}
	out.carry(segment+"_"+state+"_CONTENT", strings.TrimPrefix(inner, "["))
	starshipStyle(style, segment, state, out)
}

// starshipStyle reads a style string into this engine's appearance settings.
//
// The vocabulary is small and documented: `fg:<color>`, `bg:<color>`, a bare
// color meaning a foreground, and the attribute words. A word outside it is
// named rather than dropped — `italic` and `dimmed` are real starship
// styles and this engine has neither, so a prompt that quietly lost one
// would be the silent half.
func starshipStyle(text, segment, state string, out *Result) {
	key := func(name string) string {
		if state == "" {
			return segment + "_" + name
		}
		return segment + "_" + state + "_" + name
	}
	for _, word := range strings.Fields(text) {
		switch {
		case word == "":
		case word == "bold":
			out.carry(key("BOLD"), "true")
		case word == "underline":
			out.carry(key("UNDERLINE"), "true")
		case strings.HasPrefix(word, "fg:"):
			starshipColor(strings.TrimPrefix(word, "fg:"), key("FOREGROUND"), out)
		case strings.HasPrefix(word, "bg:"):
			starshipColor(strings.TrimPrefix(word, "bg:"), key("BACKGROUND"), out)
		default:
			starshipColor(word, key("FOREGROUND"), out)
		}
	}
}

// starshipColor carries a color this engine can spell, and names one it
// cannot.
//
// The three spellings are the same three — an index, a hex triple, a name —
// which is not a coincidence: they are what a person writing a configuration
// reaches for. An unrecognized one resolves to *unset* here, which emits
// nothing, so carrying it would be carrying a color that silently is not
// one.
func starshipColor(spec, key string, out *Result) {
	if _, ok := prompttheme.ParseColor(spec); !ok {
		out.note(key, spec+" is not a color this engine spells "+
			"(an xterm-256 index, a #rrggbb triple, or a base name)")
		return
	}
	out.carry(key, spec)
}

// trimNewlines drops leading and trailing line breaks from a side, so a line
// with nothing on one half does not become an empty row.
func trimNewlines(elements []string) []string {
	for len(elements) > 0 && elements[0] == "newline" {
		elements = elements[1:]
	}
	for len(elements) > 0 && elements[len(elements)-1] == "newline" {
		elements = elements[:len(elements)-1]
	}
	return elements
}

// plainModuleName reports whether a word read out of `format` is a module
// reference rather than literal text.
func plainModuleName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}
