// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// A prompt segment computed outside this process.
//
// docs/spec/prompt-theme.md's fourth extension layer, and the second user of
// the publish-and-redraw contract promptasync.go built: "some segments have
// to do real work — query a cluster, ask a daemon, read something over a
// socket. Those belong outside this process."
//
// # The exclusion this answers, and how
//
// docs/design/plugins.md excludes the hot path, and the answer is not an
// argument about how fast a prompt is. **A source here is never asked a
// question the shell waits on.** It is *told* what the prompt is being drawn
// for, and it answers with whatever it has — which before its first answer
// arrives is nothing, and an element that renders nothing costs no space.
// When it has something it publishes, and the prompt is redrawn in place.
//
// So the process boundary is not on the path between pressing return and
// seeing a prompt, at all, ever, and the latency of a slow or wedged source
// is bounded by nothing because nothing is waiting for it. That is the
// difference between this and the completion role docs/design/plugins.md
// still excludes: a completer is asked, on the editor's own goroutine, per
// keystroke.
//
// # Why the seam is here and not in the engine
//
// internal/prompttheme imports no protocol and knows no process. It resolves
// an element name through a Resolver, which is the same interface a session's
// own shell functions arrive through — a segment from outside the binary is
// the same kind of thing whether it is three lines of shell or a daemon.
// What this file adds is the shape a *process* takes: a declared name list, a
// context that is told rather than asked, and a way to say a redraw would
// differ.
//
// Nothing in this package knows what a plugin is. A front end composes
// whatever supplies segments, exactly as it composes a history recorder.

// PromptSegment is what a source has for one element right now.
//
// "Right now" is the whole of the contract. A source is asked for this on the
// path to drawing a prompt and must answer immediately with whatever it holds
// — a cached answer, a stale one, or nothing. It must not compute, must not
// wait, and must not dial: the reason the role exists is that the work
// happens somewhere else, and asking here would put it back.
//
// An empty Content with no Fields is **declining**, which is what every other
// segment does with nothing to say. It is how a source that has not answered
// yet costs no space, and it is deliberately not a placeholder: repl invents
// no spinner and no ellipsis, because a source that wants to say "working on
// it" says so by rendering that text, in its own words.
type PromptSegment struct {
	// Content is the segment's text, in the theme's value markup, so a source
	// may color a piece of itself.
	Content string

	// Icon names an entry in the icon table, never a glyph. A source that
	// drew its own glyph would make an icon table a lie, exactly as a
	// compiled-in segment would.
	Icon string

	// State selects the middle step of the configuration's lookup chain, so a
	// source's two states take their own colors without the configuration
	// having to know how the source computes them.
	State string

	// Fields are what the source computed, reachable from the content
	// template as ${NAME}. Substituted and never expanded.
	Fields map[string]string
}

// PromptSegmentSource is a supply of segments from outside this binary.
//
// A front end attaches one through Shell.PromptSegments; nothing in this
// package creates one. The five methods are the whole of it and each is here
// because the thing on the other end is a *process* rather than a function:
//
//   - the names are declared up front, because a name that appeared partway
//     through a session would make a prompt's shape depend on what had
//     already run — the same rule docs/design/plugins.md states about
//     command names;
//   - the context is **told**, so that a source computing something about
//     the working directory has the directory without asking a shell that
//     is not waiting to be asked;
//   - the answer is whatever is held, immediately;
//   - and publishing is how the source says a redraw would differ.
type PromptSegmentSource interface {
	// Name says where these segments come from, for what `prompt show`
	// prints and for the collision report. A person whose segment stopped
	// drawing because something else claimed the name is owed the name of
	// what claimed it.
	Name() string

	// PromptSegments is every element this source draws, declared once.
	PromptSegments() []string

	// PromptSegment is what the source has for an element now, or false for
	// an element it does not draw and for one it has no answer for yet.
	PromptSegment(element string) (PromptSegment, bool)

	// PromptContext says what the next prompt is being drawn for. A
	// notification and not a request: it returns immediately, whatever the
	// source does with it, and a source that is behind may drop one — the
	// prompt after this one will say the same thing again.
	PromptContext(PromptInfo)

	// PublishPrompt hands the source the way to say that a redraw would
	// differ, and nil when the session ends. The function is safe from any
	// goroutine and publishing twice before a redraw costs one redraw.
	PublishPrompt(notify func())
}
