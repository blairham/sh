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
// binding to nothing means — see Binding below, whose zero value is how a
// removed binding is spelled.
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
	// Putting the character that was typed into the line.
	//
	// **This one is different from every other name here, and the difference
	// is the reason it took a while to arrive.** The rest are actions a key is
	// *bound* to; this is what the key loop does when nothing else claims the
	// key, so for a long time it was not a Widget at all — there was nothing
	// to bind it to and nothing to name.
	//
	// What makes it one is that a shell can redefine it. A syntax highlighter
	// works by wrapping every widget in `$widgets` and re-colouring the line
	// after each one, and the widget it most needs is the one that runs when a
	// person types — so a shell whose table has no `self-insert` is a shell
	// where a highlighter loads, binds nothing that matters, and never sees a
	// keystroke (#2485). Measured: `${#widgets}` is 386 in zsh against 43
	// here, and `${+widgets[self-insert]}` is 1 against 0.
	//
	// The editor still performs it. What the name buys is that the key loop
	// asks, on a printable key, whether the shell has put something in front
	// of it — see EditorStyle.SelfInsertWidget.
	WidgetSelfInsert

	WidgetPreviousHistory
	WidgetNextHistory

	// The same walk, restricted to entries that begin with what the line
	// already says. A shell with a line editor offers both, and a system
	// startup file may bind the arrows to either — macOS's `/etc/zshrc` binds
	// them to the searching pair, which is how these came to be needed
	// (#2435).
	//
	// **What "begins with" means is measured and is not the obvious guess.**
	// Driven through a pseudo-terminal against zsh 5.9.2 on 2026-09-12, with
	// `echo one`, `ls -la`, `echo two`, `print hello` in history:
	//
	//	typed        pressed      line becomes
	//	echo         Up           echo two      then echo one, then no further
	//	echo zz      Up           echo two      ← the rest of the line is ignored
	//	ec           Up           echo two
	//	echo + ^A    Up           echo two      ← the cursor is not the question
	//	(empty)      Up           print hello   ← a plain walk
	//	zzz          Up           zzz           ← no match leaves the line alone
	//
	// The second row is the discriminating one: no entry begins with `echo
	// zz`, and it matched anyway. So the text searched for is the line's
	// **first word**, not the whole buffer. The fourth rules out the other
	// plausible reading — the text before the cursor — which would have been
	// empty there and given a plain walk.
	//
	// The cursor lands at the end of the recalled line in every row, and
	// walking back down to the bottom brings back what was being typed.
	//
	// One case is recorded rather than implemented, because black-box probing
	// did not explain it: recall an entry, *edit* it, then press the other
	// direction, and zsh leaves the line alone with `$HISTNO` unmoved, where
	// the entry it would walk to is still a match. See browseMatching.
	WidgetPreviousHistoryMatching
	WidgetNextHistoryMatching
	WidgetSearchHistoryBackward
	WidgetClearScreen
	WidgetDeleteChar
	WidgetBackwardDeleteChar
	WidgetComplete
	WidgetUndo
	WidgetInsertLastWord

	// The three that move between this editor's two states, and the only
	// vi-only actions with a constant here.
	//
	// **The decision, and why it went this way** (#1427). A command mode
	// brings two kinds of new thing: the *motions* — `w`, `b`, `f`, `$` — and
	// the handful of actions that get a person between insert and command
	// mode. Only the second kind is named here, and the split is the same one
	// this file is built on rather than a line drawn to keep the list short.
	//
	// A Widget is **what one key does on its own**. A motion is half of an
	// action: `w` on its own moves the cursor, and the same `w` after `d`
	// names a *range* — from where the cursor was, up to but not including
	// where it landed — and `dw` is one action made of the two. The range is
	// the part a Widget cannot carry, because a Widget answers "what did this
	// key do" and a motion has to answer "how far" to something else. So the
	// motions are a vocabulary of their own beside this list, in vi.go, where
	// the operator grammar that reads them is; a Widget for `w` would be a
	// name a dialect could bind whose meaning changed depending on what was
	// pressed before it, which is not what any other name in this list does.
	//
	// The mode changes are the other way round. Each is complete on one key,
	// each leaves the editor in a state a person can see, and — the part that
	// decides it — **both dialects already name all three and people already
	// bind them.** `bindkey -M viins jk vi-cmd-mode` is the commonest line in
	// a vi user's rc file; `bind -m vi-insert '"jk": vi-movement-mode'` is its
	// counterpart. Leaving them out would put those lines in the same
	// position `vi-command` bindings were in before this change — accepted,
	// stored and inert.
	//
	// There is no Widget for leaving *command* mode by pressing Escape,
	// because Escape in command mode does nothing at all: measured, it is not
	// an action, it is a key with nothing on it.
	WidgetViCommandMode
	WidgetViInsertMode
	WidgetViAppendMode
)

// Keymap is which table of bindings the editor reads.
//
// One editor, two states, and a dialect that keeps a table per keymap has to
// be told which one to answer for. Before there was a command mode this
// question had one answer and was not asked: both dialects' binding builtins
// already kept a `vi-command` — `vicmd` in the other's spelling — table, and
// what was written into it was stored and never read, because nothing could
// make it current. See bindings.go and Shell.KeyBindings.
type Keymap int

const (
	// KeymapMain is the table for typing a line: the emacs keymap, or vi's
	// insert keymap, whichever this session selected. The two are one table
	// here — what an editing mode changes is the *command* map and whether
	// Escape reaches it.
	KeymapMain Keymap = iota

	// KeymapViCommand is the table vi command mode reads. A session that is
	// not editing the vi way never asks for it.
	KeymapViCommand
)

// Binding is what a key sequence was rebound to.
//
// One of two things, and never both: an action this editor performs, or the
// name of an action the *shell* performs — see shellwidget.go, and Function
// there for why a name and not a callable. The zero value is a key bound to
// nothing, which is how a removed binding is spelled.
//
// A struct rather than the Widget alone because the two cannot be one value: a
// shell action has no constant, since it is code the shell was handed at run
// time and there is nothing for this package to enumerate. Both live in one
// table rather than two, because the table is read by *prefix* — a key that
// begins a longer sequence is waited for — and two tables would be two
// answers to "does anything start with this byte" with no way to be sure they
// agreed.
type Binding struct {
	// Widget is the editor's own action, where the key names one.
	Widget Widget

	// Function is the name of an action the shell performs, where the key
	// names one of those instead. Empty is the ordinary case.
	//
	// A name rather than a function value, and that is the seam rather than a
	// convenience: what running it means — finding the definition, giving it
	// the line under whatever this shell calls the line, putting the shell's
	// status back afterwards — is the dialect's, and Shell.RunWidget is where
	// the dialect is asked. A callable here would have made this package the
	// one holding a shell's idea of a call.
	Function string
}

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
	case WidgetSelfInsert:
		// The key this keystroke is about — see editor.typedKey, which is
		// what makes `zle .self-insert` from inside a wrapper insert the
		// character the person actually pressed.
		if e.typedKey != 0 {
			e.change(e.typedBefore, func() { e.insert(e.typedKey) })
			e.typing = true
		}
		e.redraw(prompt)
	case WidgetPreviousHistory:
		e.browse(-1, prompt)
	case WidgetNextHistory:
		e.browse(+1, prompt)
	case WidgetPreviousHistoryMatching:
		e.browseMatching(-1, prompt)
	case WidgetNextHistoryMatching:
		e.browseMatching(+1, prompt)
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
	case WidgetViCommandMode:
		e.enterViCommand(prompt)
	case WidgetViInsertMode:
		e.leaveViCommand(e.pos, prompt)
	case WidgetViAppendMode:
		e.leaveViCommand(e.pos+1, prompt)
	}
}
