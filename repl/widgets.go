// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// What a key can be made to do, and how a dialect says so.
//
// A shell that lets a person rebind keys needs two halves that must not know
// each other. The editor knows *how* to move a word or kill a line and has no
// idea what anyone calls it; the shell's own vocabulary of names —
// `backward-word`, `kill-whole-line`, one shell's spelling and not the other's
// — is the dialect's, and the two shells that have a line editor do not agree
// about it: the key that walks history back is `up-line-or-history` in one and
// `previous-history` in the other.
//
// So the substrate names the *actions* and each dialect maps its own names
// onto them, which is the rule the whole tree is built on — the core defines
// the question and a dialect answers it. Nothing here is a shell's word for
// anything.
//
// The list is deliberately what this editor already does and no more. A widget
// constant with nothing behind it would be a name a dialect could bind and a
// person could press to no effect, which is worse than a name that is not
// offered: an unknown name is refused or ignored in the open, and a known one
// that does nothing looks like a broken key.

// Widget is one editing action a key can be bound to.
type Widget int

// The actions this editor performs. WidgetNone is the zero value and is what a
// binding to nothing means — see Bindings, where it is how a removed binding
// is spelled.
const (
	WidgetNone Widget = iota
	WidgetBeginningOfLine
	WidgetEndOfLine
	WidgetBackwardChar
	WidgetForwardChar
	WidgetBackwardWord
	WidgetForwardWord
	WidgetKillLine
	WidgetKillWholeLine
	WidgetKillWordBefore
	WidgetKillWordAfter
	WidgetYank
	WidgetTransposeChars
	WidgetPreviousHistory
	WidgetNextHistory
	WidgetSearchHistoryBackward
	WidgetClearScreen
	WidgetDeleteChar
	WidgetBackwardDeleteChar
	WidgetComplete
	WidgetUndo
	WidgetInsertLastWord
)

// runWidget performs one action and redraws where the action changed the line.
//
// Every branch draws exactly once, which is why the redraw is here rather than
// in the actions: an action that moved the cursor and one that changed the
// text both leave the screen wrong, and the two kinds are told apart by which
// call is made and not by anything the action reports.
func (e *editor) runWidget(w Widget, prompt drawnPrompt) {
	switch w {
	case WidgetNone:
		// A key bound to nothing. Doing nothing is the whole of it, and it is
		// how a removed binding stops the editor's own default from running.
	case WidgetBeginningOfLine:
		e.moveTo(0, prompt)
	case WidgetEndOfLine:
		e.moveTo(len(e.line), prompt)
	case WidgetBackwardChar:
		e.moveTo(e.pos-1, prompt)
	case WidgetForwardChar:
		e.moveTo(e.pos+1, prompt)
	case WidgetBackwardWord:
		e.moveTo(e.backwardWord(), prompt)
	case WidgetForwardWord:
		e.moveTo(e.forwardWord(), prompt)
	case WidgetKillLine:
		e.killForwardTo(len(e.line))
		e.redraw(prompt)
	case WidgetKillWholeLine:
		e.killToStart()
		e.redraw(prompt)
	case WidgetKillWordBefore:
		e.killTo(e.wordStartBeforeCursor())
		e.redraw(prompt)
	case WidgetKillWordAfter:
		e.killForwardTo(e.endOfWord())
		e.redraw(prompt)
	case WidgetYank:
		e.yank()
		e.redraw(prompt)
	case WidgetTransposeChars:
		e.transpose()
		e.redraw(prompt)
	case WidgetPreviousHistory:
		e.browse(-1, prompt)
	case WidgetNextHistory:
		e.browse(+1, prompt)
	case WidgetSearchHistoryBackward:
		e.reverseSearch(prompt)
	case WidgetClearScreen:
		e.write("\x1b[H\x1b[2J")
		e.row = 0
		e.redraw(prompt)
	case WidgetDeleteChar:
		e.deleteForward()
		e.redraw(prompt)
	case WidgetBackwardDeleteChar:
		e.deleteBackward()
		e.redraw(prompt)
	case WidgetComplete:
		e.complete(e.comp)
		e.redraw(prompt)
	case WidgetUndo:
		e.undoLine()
		e.redraw(prompt)
	case WidgetInsertLastWord:
		// The one action that draws itself, because walking the history for a
		// second copy is part of what it does — see lastarg.go.
		e.insertLastArg(prompt)
	}
}
