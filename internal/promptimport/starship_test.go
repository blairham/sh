// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"strings"
	"testing"
)

// The starship converter, and the TOML subset under it.
//
// The fixture below is the *shape* of a real configuration — a multi-line
// `format` with a `$fill`, per-module tables, styles, symbols — written so
// that every branch of the converter is reached by something. It is not the
// fidelity claim: that is the cell-grid comparison, and "it looks the same"
// stays an opinion until something can fail.

const starshipFixture = `
# A prompt in two rows, with the right-hand modules after the fill.
add_newline = false

format = """
$directory$git_branch$fill$status$cmd_duration$golang
$character"""

[fill]
symbol = " "

[directory]
style = "fg:31"
truncation_length = 1
truncation_symbol = ""
format = "[$path]($style)"

[git_branch]
symbol = ""
style = "bold fg:76"

[character]
success_symbol = "[❯](fg:76)"
error_symbol = "[❯](fg:196)"

[cmd_duration]
min_time = 3000
style = "fg:101"

[status]
disabled = false
style = "fg:160"
symbol = "✘ "

[golang]
symbol = " "

[time]
disabled = true
style = "fg:66"
`

func starshipFrom(t *testing.T, text string) Result {
	t.Helper()
	r, err := Starship(text, drawsTheFirstSet)
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	return r
}

// The module order, the line break and the fill all come out of one string,
// and each maps onto something here.
func TestTheFormatBecomesTwoSidesAndTwoLines(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if got, _ := setting(t, r, "LEFT_ELEMENTS"); got != "dir vcs newline prompt_char" {
		t.Errorf("LEFT_ELEMENTS = %q", got)
	}
	// The right side is the modules after the fill, on the first line only —
	// so it is one line where the left is two, which is what puts the right
	// prompt on the banner row rather than on the row being typed.
	if got, _ := setting(t, r, "RIGHT_ELEMENTS"); got != "status command_execution_time" {
		t.Errorf("RIGHT_ELEMENTS = %q", got)
	}
	if got, _ := setting(t, r, "ADD_NEWLINE"); got != "false" {
		t.Errorf("ADD_NEWLINE = %q", got)
	}
}

// A module with no counterpart here is named rather than carried.
func TestAModuleWithNoSegmentHereIsNamed(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if strings.Contains(mustSetting(t, r, "RIGHT_ELEMENTS"), "golang") {
		t.Error("a module nothing draws was carried into the elements")
	}
	if notedAbout(r, "golang") == "" {
		t.Errorf("it was dropped without being named: %v", r.NotCarried)
	}
	if notedAbout(r, "[golang]") == "" {
		t.Errorf("its table was dropped without being named: %v", r.NotCarried)
	}
}

// A style string becomes this engine's appearance settings, word by word.
func TestAStyleBecomesColorsAndAttributes(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if got := mustSetting(t, r, "DIR_FOREGROUND"); got != "31" {
		t.Errorf("DIR_FOREGROUND = %q", got)
	}
	if got := mustSetting(t, r, "VCS_FOREGROUND"); got != "76" {
		t.Errorf("VCS_FOREGROUND = %q", got)
	}
	if got := mustSetting(t, r, "VCS_BOLD"); got != "true" {
		t.Errorf("VCS_BOLD = %q, want the attribute word to have been read", got)
	}
}

// The prompt character's symbols are the one markup shape worth translating,
// and both halves of each are carried.
func TestThePromptCharacterSymbolsAreTranslated(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	for key, want := range map[string]string{
		"PROMPT_CHAR_OK_CONTENT":       "❯",
		"PROMPT_CHAR_OK_FOREGROUND":    "76",
		"PROMPT_CHAR_ERROR_CONTENT":    "❯",
		"PROMPT_CHAR_ERROR_FOREGROUND": "196",
	} {
		if got := mustSetting(t, r, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// A duration threshold is milliseconds there and seconds here, and the
// conversion is exact rather than rounded to taste.
func TestTheDurationThresholdIsConvertedExactly(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if got := mustSetting(t, r, "COMMAND_EXECUTION_TIME_THRESHOLD"); got != "3" {
		t.Errorf("threshold = %q, want 3 seconds", got)
	}
	// And one that is not whole seconds is named rather than rounded: a
	// threshold of 2500ms is not two seconds and is not three.
	r = starshipFrom(t, "format = \"$cmd_duration\"\n[cmd_duration]\nmin_time = 2500\n")
	if _, ok := setting(t, r, "COMMAND_EXECUTION_TIME_THRESHOLD"); ok {
		t.Error("a threshold that is not whole seconds was rounded and carried")
	}
	if notedAbout(r, "[cmd_duration] min_time") == "" {
		t.Errorf("it was dropped without being named: %v", r.NotCarried)
	}
}

// A module's own format is named rather than half-interpreted.
//
// It is starship's markup and this engine's content template is a different
// one; translating between them is where a converter stops being a converter.
func TestAModuleFormatIsNamedAndNotTranslated(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if _, ok := setting(t, r, "DIR_CONTENT"); ok {
		t.Error("a module format was carried as a content template")
	}
	note := notedAbout(r, "[directory] format")
	if !strings.Contains(note, "markup") {
		t.Errorf("the note does not say why: %q", note)
	}
}

// A module that is turned off is not carried, and says so.
func TestADisabledModuleIsNotCarried(t *testing.T) {
	r := starshipFrom(t, starshipFixture)
	if _, ok := setting(t, r, "TIME_FOREGROUND"); ok {
		t.Error("a disabled module's settings were carried")
	}
	if notedAbout(r, "[time] disabled") == "" {
		t.Errorf("it was dropped without being named: %v", r.NotCarried)
	}
}

// A color this engine cannot spell is named rather than written, because an
// unrecognized color resolves to *unset* here and emits nothing.
func TestAColorThisEngineCannotSpellIsNamed(t *testing.T) {
	r := starshipFrom(t, "format = \"$directory\"\n[directory]\nstyle = \"fg:puce\"\n")
	if _, ok := setting(t, r, "DIR_FOREGROUND"); ok {
		t.Error("a color this engine does not know was carried")
	}
	if notedAbout(r, "DIR_FOREGROUND") == "" {
		t.Errorf("it was dropped without being named: %v", r.NotCarried)
	}
}

// A file this reader cannot read is a refusal and not a partial import.
//
// A reader that quietly skipped a construct it did not know would drop a
// setting and produce a configuration missing something, which is the
// silent-wrong-answer class the whole import is written against.
func TestAFileThisReaderCannotReadIsRefused(t *testing.T) {
	if _, err := Starship("[[modules]]\nname = \"x\"\n", drawsTheFirstSet); err == nil {
		t.Error("an array of tables was read rather than refused")
	}
	if _, err := Starship("format = \"unterminated\n", drawsTheFirstSet); err == nil {
		t.Error("a string that never closes was read rather than refused")
	}
}

func mustSetting(t *testing.T, r Result, key string) string {
	t.Helper()
	v, ok := r.Settings.Lookup(key)
	if !ok {
		t.Fatalf("%s was not carried; notes: %v", key, r.NotCarried)
	}
	return v.Text()
}
