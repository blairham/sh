// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "context"

// An editing action the *shell* performs, and the line it is handed.
//
// widgets.go names what this editor does. Everything there is a Widget: a
// constant with code behind it, and a dialect's whole job is to say what its
// shell calls each one. This file is the other half, and it exists because
// every interactive shell with a line editor lets a person write an action of
// their own — a function, run with the line in front of it, allowed to change
// the line — and no vocabulary of constants can cover that. The action is
// arbitrary code the shell was given at run time.
//
// **What belongs here is the round trip, and nothing else.** The editor hands
// out the line as it stands, something outside it runs, and the line comes
// back. That is a capability: an action the editor does not implement, applied
// to state the editor owns. What the shell *calls* the action, how it was
// defined, what it calls the line while the action is looking at it, and which
// of a shell's parameters the cursor arrives in are all naming, and all of it
// is the dialect's — see dialect/zsh, where the parameters a widget function
// reads have that shell's names on them and nothing here knows any of them.
//
// Two things are deliberately *not* offered.
//
// A shell action cannot ask this editor to perform a Widget. It is the obvious
// next seam and it is a different one: the editor's actions read the terminal
// and redraw, so running one from inside a call would be re-entering the read
// loop, and the dialect that wants it has to be able to name the moment the
// screen is written. The dialect refuses that spelling by name instead, which
// is the rule this tree keeps for a gap — a refusal a script can see beats a
// call that appears to work.
//
// And there is no seam for an action to be *started* and finish later. A
// callback on a descriptor — how a plugin in one of these shells does
// asynchrony — needs this loop to wait on more than the terminal, which is a
// change to how a key is read rather than an addition beside it.

// Line is the line being edited, as an action outside the editor sees it.
//
// Cursor is an offset in characters and not in bytes, counted from 0, and it
// may equal len([]rune(Buffer)) — which is the cursor at the end, where it
// sits after typing. Out of range is not an error: the editor puts it back in
// range, because an action that walked off the end is asking for the end.
type Line struct {
	// Buffer is the whole line, both sides of the cursor.
	Buffer string

	// Cursor is how many characters of Buffer are before the cursor.
	Cursor int
}

// runShellWidget runs one of the shell's own actions over the line, and draws
// whatever it left behind.
//
// The redraw is unconditional, for the reason runWidget's are conditional: an
// action here is opaque, so there is no way to tell one that moved the cursor
// from one that rewrote the line from one that did neither — and an action
// that *printed* has moved the screen out from under the prompt whatever it
// did to the line. Drawing again is right for all three.
func (e *editor) runShellWidget(name string, prompt drawnPrompt) {
	if e.runFunc == nil {
		// A session whose front end offered no way to run one. The key is
		// still claimed — see matchBinding — so it does nothing, which is
		// what a binding to an action this shell cannot perform means.
		return
	}
	out, ok := e.runFunc(name, Line{Buffer: string(e.line), Cursor: e.pos})
	if !ok {
		// The shell declined to run it: no such action, or one whose
		// definition has gone. It has said so itself if it had anything to
		// say, and the line is left exactly as it was.
		return
	}
	e.line = []rune(out.Buffer)
	e.pos = min(max(out.Cursor, 0), len(e.line))
	e.redraw(prompt)
}

// shellWidgets is how a session runs an action the shell owns, with the ctx
// the session was started under closed over — the editor reads keys and has no
// context of its own to give one.
func (s Shell) shellWidgets(ctx context.Context) func(string, Line) (Line, bool) {
	if s.RunWidget == nil {
		return nil
	}
	guard := s.guard()
	return func(name string, in Line) (out Line, ok bool) {
		// Behind the guard a typed line and a hook already run behind, and
		// for the stronger version of the same reason: an action bound to a
		// key runs on a *keystroke*, so a panic in one would end a session
		// over a key somebody pressed by accident. A guarded panic leaves the
		// line alone.
		if guard.Do(func() { out, ok = s.RunWidget(ctx, name, in) }) {
			return Line{}, false
		}
		return out, ok
	}
}

// runElapsed runs whatever the shell had set aside for a time that has passed.
//
// Before the prompt hook rather than after it, which is the order that keeps
// the hook's contract: `precmd` exists to be the last thing before a prompt is
// drawn, and a scheduled command that ran after it would print over a prompt
// the hook had already decided the look of.
//
// Behind the same guard and with the same status discipline a hook gets: a
// scheduled command is code from a startup file that has been sitting in a
// table for minutes, so the status it leaves must not be what the next
// command's `&&` reads, and a panic in it must not end a session.
func (s Shell) runElapsed(ctx context.Context) {
	if s.RunScheduled == nil || s.Runner == nil {
		return
	}
	status := s.Runner.ExitStatus()
	s.guard().Do(func() { s.RunScheduled(ctx) })
	if s.Runner.Exited() {
		// A scheduled command that called `exit` ends the session, the way a
		// hook that did does. The status it set is left where the loop finds
		// it.
		return
	}
	s.Runner.SetExitStatus(status)
}
