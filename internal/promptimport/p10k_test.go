// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"strings"
	"testing"
)

// The powerlevel10k converter.
//
// What arrives here is a parameter namespace rather than a file, so nothing
// in this file is about evaluating a shell program — that half is
// repl/themeimport.go's and is exercised against a real configuration there.
// What is here is the conversion: which names are carried, which are
// translated, and — the half that matters most — which are **named** rather
// than written into our file where nothing would read them.

// drawsTheFirstSet is the roster a converter is graded against in these
// tests: the elements this engine's first segment set draws.
func drawsTheFirstSet(element string) bool {
	switch element {
	case "dir", "vcs", "status", "command_execution_time",
		"background_jobs", "context", "time", "prompt_char":
		return true
	}
	return false
}

func p10kFrom(t *testing.T, scalars map[string]string, arrays map[string][]string) Result {
	t.Helper()
	if scalars == nil {
		scalars = map[string]string{}
	}
	if arrays == nil {
		arrays = map[string][]string{}
	}
	return Powerlevel10k(Params{Scalars: scalars, Arrays: arrays}, drawsTheFirstSet)
}

func setting(t *testing.T, r Result, key string) (string, bool) {
	t.Helper()
	v, ok := r.Settings.Lookup(key)
	return v.Text(), ok
}

func notedAbout(r Result, name string) string {
	for _, note := range r.NotCarried {
		if strings.HasPrefix(note, name+":") {
			return note
		}
	}
	return ""
}

// The elements lists are carried, and filtered to what this shell can draw.
//
// A configuration naming forty elements this engine has no segment for is a
// prompt that reports forty problems on its first line — and the person did
// not write those names here, so they cannot read them as their own typo.
// Each is named at import instead, which is where they can do something
// about it.
func TestTheElementsAreCarriedAndFilteredToWhatDraws(t *testing.T) {
	r := p10kFrom(t, nil, map[string][]string{
		"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS":  {"dir", "asdf", "newline", "prompt_char"},
		"POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS": {"status", "kubecontext"},
	})
	if got, _ := setting(t, r, "LEFT_ELEMENTS"); got != "dir newline prompt_char" {
		t.Errorf("LEFT_ELEMENTS = %q", got)
	}
	if got, _ := setting(t, r, "RIGHT_ELEMENTS"); got != "status" {
		t.Errorf("RIGHT_ELEMENTS = %q", got)
	}
	for _, missing := range []string{"asdf", "kubecontext"} {
		if notedAbout(r, missing) == "" {
			t.Errorf("%s was dropped without being named: %v", missing, r.NotCarried)
		}
	}
	// `newline` survives the filter, because it is the layout and not a
	// segment: filtering it out would collapse a two-row prompt into one.
	if strings.Contains(notedAbout(r, "newline"), "newline") {
		t.Error("newline was treated as a segment nothing draws")
	}
}

// A name whose spelling differs is renamed, and its value survives.
func TestANameWhoseSpellingDiffersIsRenamed(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_SHORTEN_DIR_LENGTH":                     "3",
		"POWERLEVEL9K_SHORTEN_DELIMITER":                      "…",
		"POWERLEVEL9K_MULTILINE_FIRST_PROMPT_PREFIX":          "%242F╭─",
		"POWERLEVEL9K_LEFT_PROMPT_FIRST_SEGMENT_START_SYMBOL": "",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"dir"}})
	for key, want := range map[string]string{
		"DIR_MAX_DEPTH":     "3",
		"DIR_TRUNCATION":    "…",
		"FIRST_PREFIX":      "%242F╭─",
		"LEFT_START_SYMBOL": "",
	} {
		got, ok := setting(t, r, key)
		if !ok {
			t.Errorf("%s was not carried; notes: %v", key, r.NotCarried)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// A per-segment spelling of a side symbol is renamed too, so the chain can
// resolve it.
func TestAPerSegmentSymbolIsRenamedOntoTheChain(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_PROMPT_CHAR_LEFT_PROMPT_LAST_SEGMENT_END_SYMBOL": "",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"prompt_char"}})
	if _, ok := setting(t, r, "PROMPT_CHAR_LEFT_END_SYMBOL"); !ok {
		t.Errorf("the per-segment end symbol was not renamed; notes: %v", r.NotCarried)
	}
}

// The clock's format is unwrapped out of the date expansion it is written
// in, because what this engine reads is the format itself.
func TestTheClockFormatIsUnwrapped(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_TIME_FORMAT": "%D{%H:%M:%S}",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"time"}})
	if got, _ := setting(t, r, "TIME_FORMAT"); got != "%H:%M:%S" {
		t.Errorf("TIME_FORMAT = %q, want the format without the wrapper", got)
	}
}

// A content expansion that is shell code is named, and not carried.
//
// The case the spec says decides "most" from "all". This engine's templates
// substitute rather than expand, so carrying it would draw the source text
// of a program where a person expects its output — the silent wrong answer,
// on the most visible line on the screen.
func TestAContentExpansionThatIsShellCodeIsNamed(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_VCS_CONTENT_EXPANSION": "${$((my_git_formatter(1)))+${my_git_format}}",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"vcs"}})
	if got, ok := setting(t, r, "VCS_CONTENT"); ok {
		t.Errorf("shell code was carried as a content template: %q", got)
	}
	note := notedAbout(r, "POWERLEVEL9K_VCS_CONTENT_EXPANSION")
	if !strings.Contains(note, "shell code") || !strings.Contains(note, "shell function") {
		t.Errorf("the note does not say what happened or what to do: %q", note)
	}
}

// And a literal one is carried, because it is text and nothing else.
func TestALiteralContentExpansionIsCarried(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_PROMPT_CHAR_OK_VIINS_CONTENT_EXPANSION": "❯",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"prompt_char"}})
	if got, _ := setting(t, r, "PROMPT_CHAR_OK_CONTENT"); got != "❯" {
		t.Errorf("PROMPT_CHAR_OK_CONTENT = %q, want the character", got)
	}
}

// The insert-mode value is the one carried, and the other three modes are
// named rather than applied to every mode.
func TestOnlyTheInsertModeValueIsCarried(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_PROMPT_CHAR_OK_VIINS_FOREGROUND": "76",
		"POWERLEVEL9K_PROMPT_CHAR_OK_VICMD_FOREGROUND": "1",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"prompt_char"}})
	if got, _ := setting(t, r, "PROMPT_CHAR_OK_FOREGROUND"); got != "76" {
		t.Errorf("PROMPT_CHAR_OK_FOREGROUND = %q, want the insert-mode value", got)
	}
	if note := notedAbout(r, "POWERLEVEL9K_PROMPT_CHAR_OK_VICMD_FOREGROUND"); note == "" {
		t.Errorf("the command-mode value was dropped without being named: %v", r.NotCarried)
	}
}

// An icon mode maps onto a table this binary carries, and one it does not is
// named rather than guessed at.
func TestTheIconModeMapsOntoACarriedTable(t *testing.T) {
	r := p10kFrom(t, map[string]string{"POWERLEVEL9K_MODE": "nerdfont-v3"}, nil)
	if got, _ := setting(t, r, "ICONS"); got != "nerdfont" {
		t.Errorf("ICONS = %q, want nerdfont", got)
	}
	r = p10kFrom(t, map[string]string{"POWERLEVEL9K_MODE": "awesome-mapped-fontconfig"}, nil)
	if got, _ := setting(t, r, "ICONS"); got != "nerdfont" {
		t.Errorf("ICONS = %q for a patched-font mode", got)
	}
	r = p10kFrom(t, map[string]string{"POWERLEVEL9K_MODE": "something-else"}, nil)
	if _, ok := setting(t, r, "ICONS"); ok {
		t.Error("an icon mode with no table here was carried anyway")
	}
	if notedAbout(r, "POWERLEVEL9K_MODE") == "" {
		t.Errorf("an unknown icon mode was dropped without being named: %v", r.NotCarried)
	}
}

// A key this engine reads for one segment is not carried for another.
//
// The mutation this catches is the one that was there: a pooled table of
// per-segment keys carried `COMMAND_EXECUTION_TIME_FORMAT` because `time`
// reads a `FORMAT`, writing into our file a setting nothing here would ever
// read.
func TestAKeyOneSegmentReadsIsNotCarriedForAnother(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_COMMAND_EXECUTION_TIME_FORMAT": "d h m s",
	}, map[string][]string{
		"POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS": {"command_execution_time", "time"},
	})
	if got, ok := setting(t, r, "COMMAND_EXECUTION_TIME_FORMAT"); ok {
		t.Errorf("a setting nothing reads was carried: %q", got)
	}
	if notedAbout(r, "POWERLEVEL9K_COMMAND_EXECUTION_TIME_FORMAT") == "" {
		t.Errorf("it was dropped without being named: %v", r.NotCarried)
	}
}

// A configuration that applied nothing says so, because an empty import and
// a file with nothing in it look identical afterwards.
func TestAConfigurationThatAppliedNothingSaysSo(t *testing.T) {
	r := p10kFrom(t, map[string]string{"POWERLEVEL9K_DIR_FOREGROUND": "31"}, nil)
	if note := notedAbout(r, "POWERLEVEL9K_LEFT_PROMPT_ELEMENTS"); !strings.Contains(note, "version gate") {
		t.Errorf("an empty conversion did not name the usual reason: %v", r.NotCarried)
	}
}

// What is written is this engine's own file format, and a value whose
// whitespace matters is quoted — which is the one reason the format has
// quoting at all.
func TestWhatIsWrittenIsOurOwnFormat(t *testing.T) {
	r := p10kFrom(t, map[string]string{
		"POWERLEVEL9K_LEFT_SUBSEGMENT_SEPARATOR": " ",
		"POWERLEVEL9K_DIR_FOREGROUND":            "31",
	}, map[string][]string{"POWERLEVEL9K_LEFT_PROMPT_ELEMENTS": {"dir"}})
	text := Write("somewhere/.p10k.zsh", r)
	if !strings.Contains(text, `LEFT_SUBSEGMENT_SEPARATOR = " "`) {
		t.Errorf("a value whose whitespace matters was written unquoted:\n%s", text)
	}
	if !strings.Contains(text, "DIR_FOREGROUND = 31\n") {
		t.Errorf("an ordinary value was written oddly:\n%s", text)
	}
	if !strings.Contains(text, "# Imported from somewhere/.p10k.zsh") {
		t.Errorf("the file does not say where it came from:\n%s", text)
	}
}
