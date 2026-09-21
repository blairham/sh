// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"

	"github.com/blairham/sh/internal/prompttheme"
)

// Attaching a source of segments computed outside this process to a theme.
//
// promptsegments.go is the seam; this is the half that wires one to the
// engine. The engine sees a Resolver — the same interface a session's own
// shell functions arrive through — which is the constraint the extension path
// rests on: a segment from outside the binary may use no fact and no
// capability a compiled-in one cannot, and the way to keep that true is for
// there to be one interface and not two.

// Consult attaches a source of segments computed outside this process.
//
// Three things happen here and each is a rule from the spec rather than
// plumbing.
//
// The source is consulted **below** the session's own functions and above the
// compiled-in segments, which is the spec's resolution order: a shell
// function in the session, then a plugin, then a built-in. A collision at any
// step is named by the roster rather than quietly resolved.
//
// It is handed the theme's own Publish, so a segment arriving from another
// process redraws the prompt somebody is sitting in front of. That is the
// same one byte in the same wake the repository-status cache raises — one
// async contract, two users — and a source that publishes a thousand times
// while a command runs costs one redraw.
//
// And it is remembered, so that every prompt tells it what it is being drawn
// for. Told and not asked: PromptContext returns immediately whatever the
// source does with it.
func (t *Theme) Consult(source PromptSegmentSource) {
	if source == nil {
		return
	}
	t.mu.Lock()
	t.sources = append(t.sources, source)
	t.mu.Unlock()
	t.roster.ConsultBelow(outsideSegments{source: source})
	source.PublishPrompt(t.Publish)
}

// tellSources says what the next prompt is being drawn for.
//
// Every source, every prompt, before anything is rendered — so a source
// computing something about the working directory has the directory by the
// time it starts, rather than learning it from the prompt after the `cd`.
//
// Nothing is waited for here and nothing may be: this runs on the path
// between pressing return and seeing a prompt, which is the path the whole
// role exists to stay off.
func (t *Theme) tellSources(info PromptInfo) {
	t.mu.Lock()
	sources := make([]PromptSegmentSource, len(t.sources))
	copy(sources, t.sources)
	t.mu.Unlock()
	for _, source := range sources {
		source.PromptContext(info)
	}
}

// releaseSources takes the publisher back when the session ends, so a source
// that outlives it publishes into nothing rather than into a closed wake.
func (t *Theme) releaseSources() {
	t.mu.Lock()
	sources := make([]PromptSegmentSource, len(t.sources))
	copy(sources, t.sources)
	t.mu.Unlock()
	for _, source := range sources {
		source.PublishPrompt(nil)
	}
}

// outsideSegments is a PromptSegmentSource as the engine sees it.
//
// The adapter is four lines and it is the whole coupling between the engine
// and the idea of a process. What it does *not* do is the interesting half:
// it never waits, never retries and never substitutes a placeholder for an
// answer that has not arrived. A source with nothing to say declines, and the
// layout pass is over whatever rendered — so an unanswered segment costs no
// space, exactly as an absent tool does.
type outsideSegments struct {
	source PromptSegmentSource
}

func (o outsideSegments) Name() string { return o.source.Name() }

func (o outsideSegments) Resolve(element string) (prompttheme.Segment, bool) {
	if !o.declares(element) {
		return nil, false
	}
	return prompttheme.SegmentFunc(func(_ *prompttheme.Settings, _ *prompttheme.Context) (prompttheme.Rendered, bool) {
		out, ok := o.source.PromptSegment(element)
		if !ok || (out.Content == "" && len(out.Fields) == 0) {
			// Nothing to say is declining, which is what every other segment
			// does with nothing to say — and it is what a source that has not
			// answered yet looks like. See PromptSegment: repl invents no
			// placeholder, because one would be this implementation's taste
			// presented as behavior and would move the prompt twice where an
			// answer moves it once.
			return prompttheme.Rendered{}, false
		}
		return prompttheme.Rendered{
			Content: out.Content,
			Icon:    out.Icon,
			State:   out.State,
			Fields:  out.Fields,
		}, true
	}), true
}

// declares answers from the names the source stated up front rather than by
// asking it.
//
// The declaration is what makes a collision reportable: a name that appeared
// partway through a session would make a prompt's shape depend on what had
// already run, and there would be no moment at which two sources claiming one
// name could be told apart.
func (o outsideSegments) declares(element string) bool {
	for _, name := range o.source.PromptSegments() {
		if strings.EqualFold(strings.TrimSpace(name), element) {
			return true
		}
	}
	return false
}
