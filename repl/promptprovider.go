// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "time"

// The public prompt seam: how something other than the dialect contributes to
// a prompt.
//
// PromptStyle and promptrender.go answer a different question — what a dialect
// does to a prompt *parameter's* value, which is text the person set and the
// shell transforms. This is text the shell adds because the front end asked
// it to, and the facts it is added from are ones only this package has: what
// the last command exited with, how long it took, where the shell is, and how
// many jobs it is still looking after.
//
// Cost is the reason this can be a plugin role later where completion cannot.
// A prompt is drawn once per command; a completion is asked for once per
// keystroke. docs/design/plugins.md excludes the hot path and this is not one.
//
// What it does *not* have, and must not pretend to have, is a deadline. A
// provider is called synchronously and its answer is waited for, for the same
// two reasons written down in completer.go: a deadline reports something other
// than what happened, and abandoning the work leaks the goroutine doing it.
// A provider that blocks blocks the prompt, and that is stated rather than
// guarded against — the honest place for a timeout is the far side of a pipe,
// where there is a peer to drop, which is exactly what a plugin role would
// add. Writing a fake one here would make the in-process seam claim a
// property it cannot keep.
//
// A provider that *panics* is a different matter, and that one is contained:
// a session survives an interpreter bug on a typed line already, and a prompt
// that killed the shell would be worse than the line that did. See
// Shell.contributed.

// PromptInfo is what a prompt provider is told about the moment the prompt is
// being drawn.
//
// The facts are the ones a block record keeps, taken at the same moment and by
// the same rule — see closeBlock, which records both. That is deliberate:
// #495's store and a prompt provider read nearly the same facts about a
// command, and two places deciding independently what "the last command" was
// would disagree the first time somebody pressed return on an empty line.
type PromptInfo struct {
	// Continued reports whether this is the prompt for the rest of an
	// unfinished construct rather than for a new command — PS2 rather than
	// PS1. A provider that draws a directory usually wants to draw nothing
	// here, because what follows is the middle of something.
	Continued bool

	// Dir is the shell's working directory.
	//
	// The Runner's and not the process's: `cd` moves only the Runner's, so a
	// provider reading the process's would draw the directory the embedding
	// program was started in and never change it again.
	Dir string

	// Command is the last command as it was typed, before expansion. Empty
	// before anything has run, which is how a provider that draws something
	// about the previous command knows there is not one yet.
	//
	// A blank line is not a command and does not replace this, which is the
	// rule the block store already follows: a person pressing return should
	// not clear the duration of the command they are looking at.
	Command string

	// Status is what the last command exited with, which is `$?`. Zero before
	// anything has run, which is also what `$?` reads there.
	Status int

	// Duration is how long the last command took, wall clock around the whole
	// typed line. Zero before anything has run.
	Duration time.Duration

	// Jobs is how many jobs the shell is still looking after. A job that has
	// finished and whose notice has been given is not one.
	Jobs int

	// Columns is the terminal's width, or zero where there is nothing to ask
	// — a pipe, a closed terminal, a kernel that declines. Zero is not a
	// width of eighty: something that divides by it, or fills to it, is
	// broken in a way an unknown width is not, and the honest thing to do
	// with it is to draw nothing that needs one.
	Columns int

	// Root reports whether this shell is running as the superuser, and
	// Remote whether the session arrived over a network.
	//
	// The process for the first and the session's own variables for the
	// second, which is the split the rest of this package already makes: a
	// user id is the process's and `SSH_CONNECTION` is the shell's.
	Root   bool
	Remote bool

	// PrevDir is the working directory the previous prompt was drawn in.
	// Empty at the first prompt of a session, which is how something drawing
	// a change knows there has not been one yet.
	PrevDir string
}

// PromptProvider contributes text to a prompt.
//
// The text is drawn as it stands: no separator is added and no space, because
// a provider that wanted one would have no way to take it back. Anything in it
// that instructs the terminal rather than filling a cell has to be wrapped in
// NonPrinting, or every calculation the editor makes about where the cursor is
// will be wrong by the length of the escape sequence.
//
// Called once per prompt, on the loop's own goroutine, and its answer is
// waited for. Read the note at the top of this file before writing one that
// does I/O.
type PromptProvider interface {
	Prompt(info PromptInfo) string
}

// PromptProviderFunc adapts a function to PromptProvider.
type PromptProviderFunc func(PromptInfo) string

// Prompt calls f.
func (f PromptProviderFunc) Prompt(info PromptInfo) string { return f(info) }

// NonPrinting marks text that instructs the terminal rather than occupying
// cells on it, so that the width arithmetic does not charge the prompt for it.
//
// It is what a provider uses instead of writing an escape sequence plainly,
// and it is the same two bytes bash's `\[` and `\]` produce — measured through
// a pty, neither reaches the wire. Nesting is not a thing: an inner pair would
// end the outer one, so wrap each run of escape bytes once.
//
// The empty string is left alone. A marker pair around nothing is two bytes
// the drawing code then has to strip for no reason, and a provider building
// its text conditionally will hand this an empty string often.
func NonPrinting(escapes string) string {
	if escapes == "" {
		return ""
	}
	return markStart + escapes + markEnd
}

// ThemedPrompt is a whole prompt, drawn rather than expanded.
//
// It is not a PromptProvider's answer widened: a provider contributes text in
// front of the prompt parameter, and this *is* the prompt. The two shapes
// exist side by side because they answer different questions — see
// PromptTheme.
type ThemedPrompt struct {
	// Text is the prompt for a new command, newlines and all. The last line
	// is the one being typed on.
	Text string

	// Cont is the prompt for the rest of an unfinished construct.
	Cont string

	// Right is the prompt drawn against the right-hand edge of the row the
	// line is typed on.
	//
	// Separate from Text rather than part of it, because the editor hides it
	// when the typed line grows into it and draws it again when the line
	// shrinks back. Baked into Text it would be text that wrapped instead,
	// and a wrapped prompt smears on every repaint after it.
	//
	// Empty is the ordinary case and costs nothing. No dialect here has ever
	// drawn one — see repl/rightprompt.go for what it does and what was
	// measured to decide it.
	Right string
}

// PromptTheme draws the whole prompt from a configuration, instead of the
// prompt parameter being expanded and providers contributing in front of it.
//
// Three rules, and each of them is a property somebody could otherwise assume
// the wrong way round.
//
// **The parameter is neither read nor written while a theme is drawing.** So
// turning the theme off restores whatever the person had, because nothing
// touched it — and a theme cannot be talked about as "what PS1 is set to",
// because it is not set to anything.
//
// **A theme is the whole prompt, so providers do not contribute in front of
// it.** A contribution ahead of a frame is outside the frame, which is the
// "theme pretending to be a prefix" shape this seam exists to avoid, inverted.
// A front end wiring both gets the theme; that is a decision in its own source
// rather than something a person can reach, which is why it is documented here
// and not reported at every prompt.
//
// **It is called behind the same panic guard**, for the reason written at the
// top of this file: a prompt that took the session down over a decorative
// segment would be worse than the line that did.
//
// Answering false is how a theme says it is not drawing this session — no
// configuration asked for one — and the prompt is then the parameter, exactly
// as it would be if no theme were wired at all.
type PromptTheme interface {
	DrawPrompt(info PromptInfo) (ThemedPrompt, bool)
}

// PromptThemeFunc adapts a function to PromptTheme.
type PromptThemeFunc func(PromptInfo) (ThemedPrompt, bool)

// DrawPrompt calls f.
func (f PromptThemeFunc) DrawPrompt(info PromptInfo) (ThemedPrompt, bool) { return f(info) }

// PromptThemeProblems is a theme that can say what it read and did not honor.
//
// A setting has three states — absent, honored, and **set and ignored** — and
// the third is the one that is invisible by default: the person configured
// something, the tool accepted it, and the prompt quietly did something else.
// That is the silent-wrong-answer class this repository treats as its worst,
// applied to the most visible line on the screen, so a theme that can name
// its own findings is asked for them and they are said out loud.
//
// Optional. A theme with nothing to report does not implement it, and a
// session with such a theme costs nothing.
type PromptThemeProblems interface {
	// Problems is what was read and not honored, in whatever order the theme
	// found them. Each is a whole sentence: it is printed as it stands, so a
	// theme that returns a bare key name has said nothing a person can act
	// on.
	Problems() []string
}
