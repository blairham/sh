// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "context"

// The editor, reached from inside an action the shell is running.
//
// shellwidget.go is the round trip in the ordinary direction: the editor hands
// out the line, something outside it runs, the line comes back. That file used
// to say this direction was deliberately not offered, and the reason it gave
// was a real one —
//
//	the editor's actions read the terminal and redraw, so running one from
//	inside a call would be re-entering the read loop
//
// — but it is true of **two** of this editor's actions and of none of the
// others. Moving the cursor, killing a word, yanking, undoing and walking
// history are transformations of the line plus state the editor already owns;
// there is no read loop to re-enter for any of them, and `Line` in and `Line`
// out is exactly the shape they have. The two that really do read a key stay
// refused, which is what performable below is for.
//
// **What made the difference is that the round trip alone is not enough for
// the commonest thing a real widget does.** A plugin that walks history for a
// query it did not match falls back to the editor's own walk:
//
//	if [[ -z $_history_substring_search_query ]]; then
//	  ...
//	  zle up-line-or-history
//
// and a shell where that line is a refusal has an up arrow that prints an
// error and recalls nothing (#2267). The same plugin then asks for a redraw
// and puts a keystroke back, which is the other two methods here.
//
// # Why the handle is a context value and not a field on Shell
//
// Every other seam in this package is a function field on Shell, and this one
// deliberately is not. What runs `zle up-line-or-history` is a **builtin
// registered on the Runner** — several frames inside the function the widget
// is, reached by the interpreter rather than by anything here. A parameter on
// Shell.RunWidget would arrive at the dialect's front door and *still* have to
// be carried from there down to the builtin, so it would buy a wider signature
// for every dialect and solve nothing.
//
// The dynamic extent of one call is what a context is, and that is precisely
// the lifetime of this handle: it is put on the context the widget's function
// is called under and it is gone when the call returns. So a dialect asks for
// it where it needs it, with [ActionsFrom], and keeps nothing.
//
// It also answers a question the dialect would otherwise need a flag for. A
// `zle` outside a widget — in a script, in a hook, on a `-c` line — finds no
// handle, and that *is* the refusal.

// Actions is this editor, as an action the shell is running sees it.
//
// Good for the length of one call and no longer. The editor takes the line
// back when the call returns, so a handle stored past that would be writing
// into a line nobody is editing.
type Actions interface {
	// Perform applies one of this editor's own actions to the line and hands
	// back what the action left.
	//
	// The line goes *in* as well as out, which is the part that makes this
	// usable: the action outside has been editing the line too, so a widget
	// that rewrote the buffer and then asked for the cursor to be moved to
	// the end means the end of what it just wrote, not the end of whatever
	// the editor last saw. Passing it in on every call is also what keeps the
	// two copies from drifting over a run of them.
	//
	// false is an action this editor will not perform from here. There are
	// two of those and they are the two that read a key — see performable.
	// The line comes back untouched with it, so a caller that reports the
	// refusal and carries on has not lost anything.
	Perform(w Widget, in Line) (Line, bool)

	// Redisplay draws the line as it stands now.
	//
	// A widget's changes reach the screen when it returns whether it asks for
	// this or not — see runShellWidget, whose redraw is unconditional. What
	// this is for is a widget that wants the screen right *before* it does
	// something slow, or before it reads a key of its own: without it the
	// person is looking at the line as it was until the widget finishes.
	Redisplay(in Line)

	// PushKeys puts characters where this editor will read them next, ahead
	// of anything the terminal has already delivered and ahead of anything
	// pushed before them.
	PushKeys(s string)
}

// performable reports whether an action can be run from outside the editor.
//
// Everything this editor does except the two that read a key of their own: a
// reverse incremental search is a mode with its own loop, and a completion may
// stop to ask whether to print a long listing. Running either from inside a
// widget really would be re-entering the read loop mid-keystroke — the thing
// the file comment above says cannot be done — and the difference from the
// rest is not a matter of degree. The others never touch the input.
//
// A closed list rather than a flag on each action, because the question is
// asked in exactly one place and a flag would be a field on nothing.
func performable(w Widget) bool {
	switch w {
	case WidgetSearchHistoryBackward, WidgetComplete:
		return false
	}
	return true
}

// editorActions is the handle: the editor, plus the prompt the line in front
// of it was drawn under.
//
// The prompt has to be carried because every draw needs it and an action
// outside the editor has no way to know it — it is what the session decided
// this line's prompt looks like, held for the length of the line.
type editorActions struct {
	e      *editor
	prompt drawnPrompt
}

func (a editorActions) Perform(w Widget, in Line) (Line, bool) {
	if !performable(w) {
		return in, false
	}
	a.e.take(in)
	// Through runWidget and not a copy of it, which is the whole point: a key
	// bound to `up-line-or-history` and a widget that calls `zle
	// up-line-or-history` must be the same action, including where the cursor
	// lands and what the walk leaves behind at each step. A second
	// implementation here is how the two would come to disagree.
	a.e.runWidget(w, a.prompt)
	return a.e.give(), true
}

func (a editorActions) Redisplay(in Line) {
	a.e.take(in)
	a.e.redraw(a.prompt)
}

func (a editorActions) PushKeys(s string) { a.e.pushKeys(s) }

// take adopts the line an action outside the editor is holding, and give hands
// it back.
//
// The cursor is clamped on the way in for the reason Line.Cursor gives: an
// action that walked off the end is asking for the end, and refusing it would
// make every widget that sets `CURSOR=$#BUFFER` a special case.
func (e *editor) take(in Line) {
	e.line = []rune(in.Buffer)
	e.pos = min(max(in.Cursor, 0), len(e.line))
}

func (e *editor) give() Line { return Line{Buffer: string(e.line), Cursor: e.pos} }

// actionsKey is how the handle rides the widget call's context. A private type
// so nothing outside this package can collide with it or read it out by
// guessing the key.
type actionsKey struct{}

// WithActions puts the handle on the context a widget's function will run
// under. See the file comment for why it goes here rather than in a signature.
//
// Exported, where the rest of this seam's plumbing is not, because this
// package is not the only thing that runs a widget. A dialect's own tests
// drive one without a session — and so would an embedder that has a Runner
// and a line and no editor of this package's — and without a way to say what
// the editor is, every such caller can only exercise the refusal. The
// counterpart is [ActionsFrom].
func WithActions(ctx context.Context, a Actions) context.Context {
	return context.WithValue(ctx, actionsKey{}, a)
}

// ActionsFrom is the editor the shell action running under this context is
// inside, where it is inside one.
//
// false is every other case — a script, a startup file, a hook, a `-c` line —
// and a dialect can read it as "there is no line being edited", which is the
// refusal every shell with an editor gives for the same spelling used outside
// a widget.
func ActionsFrom(ctx context.Context) (Actions, bool) {
	a, ok := ctx.Value(actionsKey{}).(Actions)
	return a, ok
}
