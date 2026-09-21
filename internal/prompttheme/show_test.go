// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"strings"
	"testing"
)

// The asked half of the report.

// namedResolver answers one element and says where it came from, which is
// what a session function and a plugin both look like from here.
type namedResolver struct {
	layer string
	name  string
}

func (n namedResolver) Name() string { return n.layer }

func (n namedResolver) Resolve(element string) (Segment, bool) {
	if element != n.name {
		return nil, false
	}
	return SegmentFunc(func(*Settings, *Context) (Rendered, bool) {
		return Rendered{Content: "x"}, true
	}), true
}

func describeFixture(t *testing.T) Report {
	t.Helper()
	preset := NewStore("preset:lean")
	preset.SetText("DIR_FOREGROUND", "31")
	preset.SetText("LEFT_ELEMENTS", "dir mine newline prompt_char")
	session := NewStore("session")
	session.SetText("DIR_FOREGROUND", "4")
	session.SetText("RIGHT_ELEMENTS", "weather")

	roster := NewRoster()
	roster.Compile("dir", SegmentFunc(func(*Settings, *Context) (Rendered, bool) {
		return Rendered{}, false
	}))
	roster.Compile("prompt_char", PromptChar("$"))
	roster.Consult(namedResolver{layer: "a session function", name: "mine"})

	return Describe(NewSettings(preset, session), roster, LoadIcons("ascii"), []string{"a note"})
}

// A value reports the layer that answered it and not the layer that also
// holds it.
func TestTheReportNamesTheLayerThatAnswered(t *testing.T) {
	report := describeFixture(t)
	found := false
	for _, s := range report.Settings {
		if s.Key != "DIR_FOREGROUND" {
			continue
		}
		found = true
		if s.Value != "4" || s.Layer != "session" {
			t.Errorf("DIR_FOREGROUND = %q from %q, want 4 from session", s.Value, s.Layer)
		}
	}
	if !found {
		t.Fatalf("DIR_FOREGROUND is not in the report: %+v", report.Settings)
	}
}

// Each element says what draws it, and the three answers are told apart.
func TestTheReportNamesWhatDrawsEachElement(t *testing.T) {
	report := describeFixture(t)
	want := map[string]string{
		"dir":         builtInSource,
		"mine":        "a session function",
		"prompt_char": builtInSource,
		"weather":     "",
	}
	seen := map[string]bool{}
	for _, e := range report.Elements {
		seen[e.Name] = true
		if got, ok := want[e.Name]; !ok {
			t.Errorf("the report has an element nothing configured: %q", e.Name)
		} else if e.Source != got {
			t.Errorf("%s is drawn by %q, want %q", e.Name, e.Source, got)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("the report left out the element %q", name)
		}
	}
}

// The line an element is on survives a `newline`, so a two-row configuration
// reports as two rows.
//
// A one-row report would pass every other assertion in this file, which is
// the same blind spot a one-row prompt is in a terminal test.
func TestTheReportSaysWhichLineAnElementIsOn(t *testing.T) {
	report := describeFixture(t)
	lines := map[string]int{}
	for _, e := range report.Elements {
		lines[e.Name] = e.Line
	}
	if lines["dir"] != 0 {
		t.Errorf("dir is on line %d, want 0", lines["dir"])
	}
	if lines["prompt_char"] != 1 {
		t.Errorf("prompt_char is on line %d, want 1 — the newline did not split the side", lines["prompt_char"])
	}
}

// Asking does not make a complaint. A report is a question and not a prompt.
func TestAskingDoesNotAddToTheNotYetList(t *testing.T) {
	roster := NewRoster()
	roster.Compile("dir", PromptChar("$"))
	store := NewStore("session")
	store.SetText("LEFT_ELEMENTS", "dir weather")
	Describe(NewSettings(store), roster, nil, nil)
	if got := roster.NotYet(); len(got) != 0 {
		t.Errorf("asking about elements recorded %v as not yet drawn", got)
	}
}

// The report says it is not the whole of the configuration, because it
// cannot be: a session's variables are not enumerable.
func TestTheReportSaysItIsPartial(t *testing.T) {
	text := strings.Join(describeFixture(t).Lines(), "\n")
	if !strings.Contains(text, "no shell offers to enumerate its variables") {
		t.Errorf("the report did not say what it cannot see:\n%s", text)
	}
}

// A configuration that names no elements is reported as not drawing, which
// is the first line because every other line means something different
// underneath it.
func TestAConfigurationWithNoElementsIsReportedAsNotDrawing(t *testing.T) {
	report := Describe(NewSettings(NewStore("session")), NewRoster(), nil, nil)
	if report.Drawing {
		t.Error("a configuration naming no elements reported as drawing")
	}
	if got := report.Lines()[0]; !strings.Contains(got, "drawing: no") {
		t.Errorf("the first line is %q, want it to say the theme is not drawing", got)
	}
}
