// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"sync"
	"testing"
)

// Segments computed outside this process, attached to a theme.
//
// Nothing here is a plugin: the seam is an interface and the test drives it
// directly, which is the point of the seam being one. internal/plugin's own
// tests drive the same interface from the far side of a real process.

// fakeSource is a PromptSegmentSource a test can move.
type fakeSource struct {
	layer string
	names []string

	mu       sync.Mutex
	held     map[string]PromptSegment
	told     []PromptInfo
	notify   func()
	released bool
}

func newFakeSource(layer string, names ...string) *fakeSource {
	return &fakeSource{layer: layer, names: names, held: map[string]PromptSegment{}}
}

func (f *fakeSource) Name() string { return f.layer }

func (f *fakeSource) PromptSegments() []string { return f.names }

func (f *fakeSource) PromptSegment(element string) (PromptSegment, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out, ok := f.held[element]
	return out, ok
}

func (f *fakeSource) PromptContext(info PromptInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.told = append(f.told, info)
}

func (f *fakeSource) PublishPrompt(notify func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notify = notify
	f.released = notify == nil
}

// arrived is the answer turning up on some other goroutine, which is the
// only thing a source outside this process ever does.
func (f *fakeSource) arrived(element string, out PromptSegment) {
	f.mu.Lock()
	f.held[element] = out
	notify := f.notify
	f.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (f *fakeSource) contexts() []PromptInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PromptInfo(nil), f.told...)
}

func (f *fakeSource) wasReleased() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released
}

// themeWith builds a theme over a fixed set of variables.
func themeWith(vars map[string]string) *Theme {
	return NewTheme(func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	})
}

// A segment that has not answered yet draws nothing, and costs no space.
//
// repl invents no placeholder: a source that wants to say "working on it"
// says so by rendering that text, in its own words.
func TestASegmentWithNoAnswerYetDrawsNothing(t *testing.T) {
	theme := themeWith(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "weather prompt_char"})
	source := newFakeSource("plugin weather", "weather")
	theme.Consult(source)

	drawn, ok := theme.DrawPrompt(PromptInfo{Dir: "/tmp"})
	if !ok {
		t.Fatal("the theme is not drawing")
	}
	if strings.Contains(drawn.Text, "sunny") {
		t.Errorf("an unanswered segment drew something: %q", drawn.Text)
	}

	source.arrived("weather", PromptSegment{Content: "sunny"})
	drawn, _ = theme.DrawPrompt(PromptInfo{Dir: "/tmp"})
	if !strings.Contains(drawn.Text, "sunny") {
		t.Errorf("the answer did not reach the prompt: %q", drawn.Text)
	}
}

// Every prompt tells every source what it is being drawn for.
func TestEveryPromptTellsTheSourcesWhatItIsFor(t *testing.T) {
	theme := themeWith(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "weather"})
	source := newFakeSource("plugin weather", "weather")
	theme.Consult(source)

	theme.DrawPrompt(PromptInfo{Dir: "/one", Status: 3, Jobs: 2})
	theme.DrawPrompt(PromptInfo{Dir: "/two"})

	told := source.contexts()
	if len(told) != 2 {
		t.Fatalf("the source was told %d times, want 2", len(told))
	}
	if told[0].Dir != "/one" || told[0].Status != 3 || told[0].Jobs != 2 {
		t.Errorf("the first context was %+v", told[0])
	}
	if told[1].Dir != "/two" {
		t.Errorf("the second context was %+v", told[1])
	}
}

// The resolution order is the spec's: a shell function in the session, then
// a source from outside the binary, then a segment compiled in.
//
// The two are wired at different moments — the theme installs the session's
// functions when it is built and a front end attaches a source afterwards —
// so the plain "later is more local" rule would invert this exactly. That is
// what ConsultBelow is for, and this is the test that would catch its
// removal.
func TestASessionFunctionWinsOverASourceWhichWinsOverTheBuiltIn(t *testing.T) {
	theme := themeWith(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "dir vcs"})
	// A source claiming both an element the session also defines and one it
	// does not, so the two steps are told apart by the same render.
	source := newFakeSource("plugin both", "dir", "vcs")
	source.arrived("dir", PromptSegment{Content: "FROM-SOURCE"})
	source.arrived("vcs", PromptSegment{Content: "BRANCH-OUTSIDE"})
	theme.Consult(source)
	theme.useSession(
		func(name string) bool { return name == "dir" },
		func(string) (string, bool) { return "FROM-SESSION", true },
	)

	drawn, ok := theme.DrawPrompt(PromptInfo{Dir: "/tmp"})
	if !ok {
		t.Fatal("the theme is not drawing")
	}
	if !strings.Contains(drawn.Text, "FROM-SESSION") {
		t.Errorf("the session's own function did not win: %q", drawn.Text)
	}
	if strings.Contains(drawn.Text, "FROM-SOURCE") {
		t.Errorf("a source outside the binary beat the session: %q", drawn.Text)
	}
	// And the element the session does not define is the source's rather
	// than the built-in repository segment's, which is the second step.
	if !strings.Contains(drawn.Text, "BRANCH-OUTSIDE") {
		t.Errorf("the source did not beat the built-in segment: %q", drawn.Text)
	}
}

// A collision is named rather than quietly resolved, and the sentence says
// which source won.
func TestACollisionBetweenASourceAndTheBuiltInIsNamed(t *testing.T) {
	theme := themeWith(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "vcs"})
	source := newFakeSource("plugin weather", "vcs")
	source.arrived("vcs", PromptSegment{Content: "mine"})
	theme.Consult(source)
	theme.DrawPrompt(PromptInfo{Dir: "/tmp"})

	problems := strings.Join(theme.Problems(), "\n")
	if !strings.Contains(problems, "plugin weather") || !strings.Contains(problems, "vcs") {
		t.Errorf("the collision was not named: %q", problems)
	}
	// And it is said once however many prompts are drawn. A sentence
	// appended per prompt grows a slice for the life of a session, and the
	// report drops a repeat, so it would do it invisibly.
	before := len(theme.Problems())
	for range 5 {
		theme.DrawPrompt(PromptInfo{Dir: "/tmp"})
	}
	if after := len(theme.Problems()); after != before {
		t.Errorf("the problem list grew from %d to %d over five prompts", before, after)
	}
}

// A source publishes through the theme, so a segment arriving from another
// process wakes the session that is sitting in front of the prompt.
func TestASourcePublishesThroughTheTheme(t *testing.T) {
	theme := themeWith(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "weather"})
	source := newFakeSource("plugin weather", "weather")
	theme.Consult(source)

	published := 0
	theme.PublishTo(func() { published++ })
	source.arrived("weather", PromptSegment{Content: "sunny"})
	if published != 1 {
		t.Errorf("the source's answer published %d times, want 1", published)
	}

	// And the session ending takes the wake back from every source, so one
	// that answers afterwards publishes into nothing.
	theme.PublishTo(nil)
	if !source.wasReleased() {
		t.Error("the source was not released when the session ended")
	}
	source.arrived("weather", PromptSegment{Content: "later"})
	if published != 1 {
		t.Errorf("a source published %d times after the session ended", published-1)
	}
}
