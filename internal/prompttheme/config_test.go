// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestKeyDropsThePrefixAndTheCase(t *testing.T) {
	for _, spelling := range []string{"SH_PROMPT_DIR_FOREGROUND", "sh_prompt_dir_foreground", "DIR_FOREGROUND", " dir_foreground "} {
		if got := prompttheme.Key(spelling); got != "DIR_FOREGROUND" {
			t.Errorf("Key(%q) = %q, want DIR_FOREGROUND", spelling, got)
		}
	}
}

func TestAKeyHoldsAScalarOrAListAndNeverBoth(t *testing.T) {
	store := prompttheme.NewStore("test")

	store.SetList("LEFT_ELEMENTS", "dir", "vcs")
	store.SetText("LEFT_ELEMENTS", "status")
	if v, _ := store.Lookup("LEFT_ELEMENTS"); v.IsList() {
		t.Error("a scalar set over a list left a list behind")
	}

	store.SetList("LEFT_ELEMENTS", "dir", "vcs")
	v, _ := store.Lookup("LEFT_ELEMENTS")
	if !v.IsList() || !slices.Equal(v.List(), []string{"dir", "vcs"}) {
		t.Errorf("a list set over a scalar read back as %#v", v.List())
	}
}

func TestAScalarReadAsAListIsItsWords(t *testing.T) {
	// dash has no arrays, and a list still has to be expressible in a
	// session running it: the words of a scalar are that expression.
	settings := prompttheme.NewSettings(assignments(t, "preset", "LEFT_ELEMENTS", "dir vcs newline prompt_char"))
	want := []string{"dir", "vcs", "newline", "prompt_char"}
	if got := settings.List("LEFT_ELEMENTS"); !slices.Equal(got, want) {
		t.Errorf("List = %#v, want %#v", got, want)
	}
}

func TestAListReadAsAScalarIsTheWayAListIsWritten(t *testing.T) {
	store := prompttheme.NewStore("preset")
	store.SetList("LEFT_ELEMENTS", "dir", "vcs")
	settings := prompttheme.NewSettings(store)
	if got := settings.Str("LEFT_ELEMENTS", ""); got != "dir vcs" {
		t.Errorf("Str = %q, want %q", got, "dir vcs")
	}
}

func TestSetToEmptyIsAnAnswerAndAbsentIsNot(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t, "preset", "DIR_PREFIX", ""))

	if !settings.Has("DIR_PREFIX") {
		t.Error("a setting emptied on purpose read as absent")
	}
	if got := settings.Str("DIR_PREFIX", "→"); got != "" {
		t.Errorf("an emptied setting fell back to the default: %q", got)
	}
	if settings.Has("DIR_SUFFIX") {
		t.Error("a setting nobody wrote read as present")
	}
	if got := settings.Str("DIR_SUFFIX", "→"); got != "→" {
		t.Errorf("an absent setting did not reach the default: %q", got)
	}
}

func TestAMalformedValueDegradesToTheDefault(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t,
		"preset",
		"MAX_LENGTH", "twelve",
		"SHOW", "perhaps",
		"TRUNCATE", "3",
		"BOLD", "yes",
	))

	if got := settings.Int("MAX_LENGTH", 40); got != 40 {
		t.Errorf("Int on a word = %d, want the default 40", got)
	}
	if got := settings.Bool("SHOW", true); !got {
		t.Error("Bool on a word did not reach the default")
	}
	if got := settings.Int("TRUNCATE", 40); got != 3 {
		t.Errorf("Int = %d, want 3", got)
	}
	if !settings.Bool("BOLD", false) {
		t.Error("yes did not read as true")
	}
}

func TestEmptyingAFlagTurnsItOff(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t, "preset", "BOLD", ""))
	if settings.Bool("BOLD", true) {
		t.Error("a flag emptied on purpose kept the default on")
	}
}

func TestTheChainIsThreeStepsMostSpecificFirst(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t,
		"preset",
		"FOREGROUND", "white",
		"DIR_FOREGROUND", "blue",
		"DIR_NOT_WRITABLE_FOREGROUND", "red",
	))

	for _, c := range []struct{ segment, state, want string }{
		{"dir", "not-writable", "red"},
		{"dir", "", "blue"},
		{"vcs", "clean", "white"},
		{"", "", "white"},
	} {
		if got := settings.Param(c.segment, c.state, "FOREGROUND", "green"); got != c.want {
			t.Errorf("Param(%q, %q) = %q, want %q", c.segment, c.state, got, c.want)
		}
	}
}

func TestAHyphenInASegmentNameIsAnUnderscore(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t, "preset", "COMMAND_EXECUTION_TIME_FOREGROUND", "yellow"))
	if got := settings.Param("command-execution-time", "", "FOREGROUND", ""); got != "yellow" {
		t.Errorf("a hyphenated segment name resolved to %q", got)
	}
}

func TestParamSetSeparatesEmptyFromAbsent(t *testing.T) {
	settings := prompttheme.NewSettings(assignments(t, "preset", "DIR_PREFIX", ""))
	if !settings.ParamSet("dir", "", "PREFIX") {
		t.Error("an emptied per-segment setting read as absent")
	}
	if settings.ParamSet("vcs", "", "PREFIX") {
		t.Error("a per-segment setting nobody wrote read as present")
	}
}

func TestALaterLayerWinsPerKey(t *testing.T) {
	preset := assignments(t, "preset", "FOREGROUND", "white", "DIR_FOREGROUND", "blue")
	file := assignments(t, "file", "DIR_FOREGROUND", "red")
	settings := prompttheme.NewSettings(preset, file)

	if got := settings.Str("DIR_FOREGROUND", ""); got != "red" {
		t.Errorf("the later layer did not win: %q", got)
	}
	if got := settings.Str("FOREGROUND", ""); got != "white" {
		t.Errorf("a key the later layer did not mention was lost: %q", got)
	}
}

func TestALaterListReplacesTheEarlierOne(t *testing.T) {
	preset := prompttheme.NewStore("preset")
	preset.SetList("LEFT_ELEMENTS", "dir", "vcs", "status")
	session := prompttheme.NewStore("session")
	session.SetList("LEFT_ELEMENTS", "dir")

	settings := prompttheme.NewSettings(preset, session)
	if got := settings.List("LEFT_ELEMENTS"); !slices.Equal(got, []string{"dir"}) {
		t.Errorf("the lists merged instead of replacing: %#v", got)
	}
}

func TestASettingSaysWhichLayerItCameFrom(t *testing.T) {
	preset := assignments(t, "preset", "FOREGROUND", "white", "DIR_FOREGROUND", "blue")
	session := assignments(t, "session", "DIR_FOREGROUND", "red")
	settings := prompttheme.NewSettings(preset, session)

	if layer, _ := settings.Origin("DIR_FOREGROUND"); layer != "session" {
		t.Errorf("Origin = %q, want session", layer)
	}
	if layer, _ := settings.Origin("FOREGROUND"); layer != "preset" {
		t.Errorf("Origin = %q, want preset", layer)
	}
	if _, ok := settings.Origin("BACKGROUND"); ok {
		t.Error("a key nobody set named a layer")
	}

	name, layer, ok := settings.ParamOrigin("dir", "", "FOREGROUND")
	if !ok || name != "DIR_FOREGROUND" || layer != "session" {
		t.Errorf("ParamOrigin = %q, %q, %v", name, layer, ok)
	}
}

func TestSessionVariablesAreReadThroughTheirFullName(t *testing.T) {
	asked := ""
	vars := prompttheme.NewVars("session", func(name string) (string, bool) {
		asked = name
		if name == "SH_PROMPT_DIR_FOREGROUND" {
			return "red", true
		}
		return "", false
	})
	settings := prompttheme.NewSettings(assignments(t, "preset", "DIR_FOREGROUND", "blue"), vars)

	if got := settings.Str("DIR_FOREGROUND", ""); got != "red" {
		t.Errorf("the session did not win: %q", got)
	}
	if asked != "SH_PROMPT_DIR_FOREGROUND" {
		t.Errorf("the variable asked for was %q", asked)
	}
	if got := settings.Str("FOREGROUND", "white"); got != "white" {
		t.Errorf("a variable the session does not hold resolved to %q", got)
	}
}

func TestKeysReportsWhatIsWrittenDownAndNotWhatWouldResolve(t *testing.T) {
	preset := assignments(t, "preset", "FOREGROUND", "white", "DIR_FOREGROUND", "blue")
	file := assignments(t, "file", "DIR_FOREGROUND", "red", "TRANSIENT", "always")
	vars := prompttheme.NewVars("session", func(string) (string, bool) { return "x", true })

	settings := prompttheme.NewSettings(preset, file, vars)
	want := []string{"DIR_FOREGROUND", "FOREGROUND", "TRANSIENT"}
	if got := settings.Keys(); !slices.Equal(got, want) {
		t.Errorf("Keys = %#v, want %#v", got, want)
	}
}

// assignments builds a store from alternating key and value arguments.
func assignments(t *testing.T, layer string, pairs ...string) *prompttheme.Store {
	t.Helper()
	if len(pairs)%2 != 0 {
		t.Fatalf("assignments: %d arguments, want pairs", len(pairs))
	}
	store := prompttheme.NewStore(layer)
	for i := 0; i < len(pairs); i += 2 {
		store.SetText(pairs[i], pairs[i+1])
	}
	return store
}
