// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"os"
	"strings"
	"sync"
)

// A preset is a set of assignments, so it is data.
//
// The layout pass spans "no backgrounds, a space separator" to "a background
// per segment and powerline arrows", which is the whole range prompts live
// in, so a preset costs no code. That is the structural claim the engine
// rests on, and these two files are it being cashed: they differ only in
// their settings and the same loop draws both.
//
// **They ship as files and are loaded from files.** A look somebody publishes
// is a path you point a variable at; the ones carried here are seeded from
// exactly the same format, so there is no arrangement a shipped preset can
// express and a downloaded one cannot. That is the half of the no-rebuild
// constraint a preset decides.
//
// Named for what they look like and never for another project. A preset named
// after somebody else's theme is a claim about fidelity, and the only honest
// way to make one is the cell-grid comparison the spec asks for and nothing
// here has yet.

// leanPreset is two rows, no backgrounds, and a space between segments.
//
// It is what most people actually want from a rich prompt: the directory and
// the branch where the eye already is, everything else on the right where it
// can be ignored, and the line typed on a row of its own so a long command
// never fights the prompt for width.
const leanPreset = `
LEFT_ELEMENTS  = dir vcs newline prompt_char
RIGHT_ELEMENTS = status command_execution_time background_jobs

DIR_FOREGROUND = 4
DIR_NOT_WRITABLE_FOREGROUND = 1
VCS_FOREGROUND = 2
VCS_DETACHED_FOREGROUND = 3
STATUS_ERROR_FOREGROUND = 1
STATUS_OK_FOREGROUND = 2
COMMAND_EXECUTION_TIME_FOREGROUND = 3
BACKGROUND_JOBS_FOREGROUND = 6
PROMPT_CHAR_OK_FOREGROUND = 2
PROMPT_CHAR_ERROR_FOREGROUND = 1
PROMPT_CHAR_SYMBOL = "❯"

LEFT_SEGMENT_SEPARATOR = " "
RIGHT_SEGMENT_SEPARATOR = " "
`

// framePreset is the same elements with a background each and an arrow
// between them, which is the other look prompts come in.
//
// The point of carrying both is that the difference between them is entirely
// in this file. If a third look ever needs code, the layout pass has stopped
// being generic and that is the bug rather than the look.
const framePreset = `
LEFT_ELEMENTS  = dir vcs newline prompt_char
RIGHT_ELEMENTS = status command_execution_time background_jobs

WHITESPACE = " "
FOREGROUND = 0
DIR_BACKGROUND = 4
VCS_BACKGROUND = 2
VCS_DETACHED_BACKGROUND = 3
STATUS_ERROR_BACKGROUND = 1
STATUS_ERROR_FOREGROUND = 15
STATUS_OK_BACKGROUND = 2
COMMAND_EXECUTION_TIME_BACKGROUND = 3
BACKGROUND_JOBS_BACKGROUND = 6

PROMPT_CHAR_WHITESPACE = ""
PROMPT_CHAR_OK_FOREGROUND = 2
PROMPT_CHAR_ERROR_FOREGROUND = 1
PROMPT_CHAR_SYMBOL = "❯"

LEFT_SEGMENT_SEPARATOR = ""
LEFT_END_SYMBOL = ""
RIGHT_SEGMENT_SEPARATOR = ""
RIGHT_START_SYMBOL = ""
`

// Presets names the looks this binary carries.
func Presets() []string { return []string{"lean", "frame"} }

// shippedPresets parses the carried looks once, through the configuration
// file's own reader — so a look shipped here and a look downloaded from
// somewhere cannot differ in what they are able to say.
var shippedPresets = sync.OnceValue(func() map[string]*Store {
	texts := map[string]string{"lean": leanPreset, "frame": framePreset}
	parsed := make(map[string]*Store, len(texts))
	for name, text := range texts {
		store, _ := ParseFile("preset:"+name, strings.NewReader(text))
		for _, key := range store.Keys() {
			v, _ := store.Lookup(key)
			store.SetText(key, unescape(v.Text()))
		}
		parsed[name] = store
	}
	return parsed
})

// LoadPreset answers the layer SH_PROMPT_PRESET asks for: a look this binary
// carries, or a file somewhere.
//
// A name that is neither is **named rather than ignored**. A person who
// misspelled a preset asked for a look and got the plain prompt, and the
// difference between "there is no such preset" and "presets do not work" is
// the whole of what a report is for.
func LoadPreset(name string) (Layer, string) {
	asked := strings.TrimSpace(name)
	if asked == "" {
		return nil, ""
	}
	if store, ok := shippedPresets()[strings.ToLower(asked)]; ok {
		return store, ""
	}
	if _, err := os.Stat(asked); err != nil {
		return nil, "no preset named " + asked + " (carried: " +
			strings.Join(Presets(), ", ") + "), and no file there either"
	}
	// A path, read the way the configuration file is read and re-read when it
	// changes: a preset somebody is writing is a file they are editing.
	file := OpenFile(asked)
	file.Refresh()
	return file, ""
}
