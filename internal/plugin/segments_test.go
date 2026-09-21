// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/repl"
)

// The segment role, driven the way a session drives it.
//
// Every fixture here is a POSIX shell script, for the reason helper_test.go
// gives: a fixture written in Go would test the host against a peer built
// from the same message types, which is the one peer that cannot check the
// claim that a plugin needs no library.

// promptWait bounds a wait on a plugin that is expected to publish.
//
// It turns a hang into a failure and is deliberately not tuned to what the
// work costs — the work is one line of JSON each way — for the reason
// driver's sessionBudget is not tuned either.
const promptWait = 10 * time.Second

// awaitSegment waits for a source to hold an answer for one element.
//
// A poll rather than a hook, because the whole claim under test is that
// nothing about this is synchronous: there is no moment the host can be told
// "the plugin has answered now" that is not itself a thing to wait for.
func awaitSegment(t *testing.T, source repl.PromptSegmentSource, element string) repl.PromptSegment {
	t.Helper()
	deadline := time.Now().Add(promptWait)
	for time.Now().Before(deadline) {
		if out, ok := source.PromptSegment(element); ok {
			return out
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the plugin never published a %q segment", element)
	return repl.PromptSegment{}
}

// A plugin that declares only segments is a complete plugin, and what it
// draws reaches the shell.
func TestASegmentPluginIsToldTheContextAndPublishesAnAnswer(t *testing.T) {
	h := launch(t, "weather", plugin.Options{})
	if got := h.Commands(); len(got) != 0 {
		t.Errorf("Commands = %v, want none", got)
	}
	source := h.PromptSegments()
	if source == nil {
		t.Fatal("a plugin that declared a segment supplies no source")
	}
	if got := source.PromptSegments(); len(got) != 1 || got[0] != "weather" {
		t.Fatalf("PromptSegments = %v, want [weather]", got)
	}

	// Before anything is told, nothing is held — which is what the first
	// prompt of a session draws for a plugin segment, and it is deliberately
	// not a placeholder.
	if _, ok := source.PromptSegment("weather"); ok {
		t.Error("a segment answered before the plugin was told anything")
	}

	source.PromptContext(repl.PromptInfo{Dir: "/tmp/somewhere"})
	out := awaitSegment(t, source, "weather")
	if out.Content != "sunny-in-somewhere" {
		t.Errorf("content = %q, want it computed from the directory it was told", out.Content)
	}
	// The three things beside the text, each of which a segment from outside
	// the binary has to be able to say for the roster not to be a privileged
	// class: an icon *key* rather than a glyph, a state to take colors from,
	// and fields a content template can reach.
	if out.Icon != "VCS" {
		t.Errorf("icon = %q, want the key the plugin named", out.Icon)
	}
	if out.State != "fine" {
		t.Errorf("state = %q, want the state the plugin named", out.State)
	}
	if out.Fields["WHERE"] != "somewhere" {
		t.Errorf("fields = %v, want WHERE=somewhere", out.Fields)
	}
}

// Publishing says a redraw would differ, and a republished identical answer
// says nothing at all.
func TestPublishingSaysARedrawWouldDifferAndOnlyThen(t *testing.T) {
	relayed := &syncBuffer{}
	h := launch(t, "weather", plugin.Options{Stderr: relayed})
	source := h.PromptSegments()

	// The plugin says on its own standard error what it was told, and the
	// host relays it. That is what makes this test possible at all: a
	// republished *identical* answer is invisible in the answer, so
	// "published again" has to be read from the plugin rather than inferred
	// from the shell — and inferring it is how this test would pass while
	// the deduplication was gone.
	toldTimes := func(dir string) int {
		return strings.Count(relayed.String(), "told "+dir+"\n")
	}
	awaitTold := func(dir string, want int) {
		t.Helper()
		deadline := time.Now().Add(promptWait)
		for time.Now().Before(deadline) {
			if toldTimes(dir) >= want {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("the plugin was told %q %d times, want %d; it said %q",
			dir, toldTimes(dir), want, relayed.String())
	}

	var mu sync.Mutex
	published := 0
	source.PublishPrompt(func() {
		mu.Lock()
		published++
		mu.Unlock()
	})
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return published
	}

	source.PromptContext(repl.PromptInfo{Dir: "/tmp/first"})
	awaitSegment(t, source, "weather")
	awaitTold("/tmp/first", 1)
	deadline := time.Now().Add(promptWait)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if count() != 1 {
		t.Fatalf("a first answer published %d times, want 1", count())
	}

	// The same directory again, and waited for, because at most one context
	// is ever in flight: sending two without waiting would replace the first
	// with the second and the repeat would never reach the plugin at all.
	// The plugin republishes the same answer — which is what a poll does —
	// and the session must not be woken for a prompt that would render
	// identically.
	source.PromptContext(repl.PromptInfo{Dir: "/tmp/first"})
	awaitTold("/tmp/first", 2)

	source.PromptContext(repl.PromptInfo{Dir: "/tmp/second"})
	deadline = time.Now().Add(promptWait)
	for time.Now().Before(deadline) {
		if out, ok := source.PromptSegment("weather"); ok && out.Content == "sunny-in-second" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := count(); got != 2 {
		t.Errorf("%d publishes after a repeated answer and a new one, want 2", got)
	}
}

// A plugin that publishes a segment it never declared is refused, and the
// one it did declare still works.
//
// Refused rather than accepted, because the declaration is the whole of what
// makes a collision reportable: an element that appeared partway through a
// session would make a prompt's shape depend on what had already run.
func TestASegmentOutsideTheDeclarationIsRefused(t *testing.T) {
	h := launch(t, "sneakysegment", plugin.Options{})
	source := h.PromptSegments()
	source.PromptContext(repl.PromptInfo{Dir: "/tmp/x"})
	if out := awaitSegment(t, source, "declared"); out.Content != "honest" {
		t.Errorf("the declared segment drew %q", out.Content)
	}
	if out, ok := source.PromptSegment("undeclared"); ok {
		t.Errorf("an undeclared segment was accepted and drew %q", out.Content)
	}
}

// A plugin that declared no segments supplies no source at all, so a shell
// running only command plugins consults nobody.
//
// Nil rather than an empty source, and the typed-nil hazard is why
// PromptSegments is a method: a caller's `if source != nil` has to be right.
func TestAPluginWithNoSegmentsSuppliesNoSource(t *testing.T) {
	h := launch(t, "greet", plugin.Options{})
	if source := h.PromptSegments(); source != nil {
		t.Errorf("a plugin that declared no segments supplied %v", source)
	}
}

// A plugin that declares nothing at all is still refused, and the sentence
// names the third role now that there is one.
func TestAPluginWithNoSurfaceAtAllIsStillRefused(t *testing.T) {
	_, err := plugin.Launch(t.Context(), plugin.Options{
		Path:   fixture(t, "nocommands"),
		Stderr: &syncBuffer{},
	})
	if err == nil {
		t.Fatal("a plugin declaring no surface was accepted")
	}
	if !strings.Contains(err.Error(), "segments") {
		t.Errorf("the refusal does not mention segments: %v", err)
	}
}

// Shutting a segment plugin down leaves nothing running.
//
// The feed is a goroutine per plugin and the leak class #690 is this
// repository's standing example of. The wait is split across Close — told to
// stop before the input goes, waited for after — because a feed parked in a
// write is unblocked by the stream going and by nothing else, and a Close
// that waited first would deadlock.
func TestClosingASegmentPluginEndsItsFeed(t *testing.T) {
	h := launch(t, "weather", plugin.Options{})
	source := h.PromptSegments()
	source.PromptContext(repl.PromptInfo{Dir: "/tmp/x"})
	awaitSegment(t, source, "weather")

	done := make(chan error, 1)
	go func() { done <- h.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(promptWait):
		t.Fatal("Close did not return: the segment feed was waited for before its stream went")
	}
	// And publishing afterwards is harmless rather than a write to a dead
	// connection: a plugin's answer arriving after the shell has gone is an
	// ordinary outcome.
	source.PromptContext(repl.PromptInfo{Dir: "/tmp/y"})
}
