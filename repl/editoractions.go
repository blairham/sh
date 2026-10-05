// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"slices"
	"unicode/utf8"
)

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
// out is exactly the shape they have. The two that really do read a key were
// refused at first, and both refusals turned out wrong — they read through
// the editor's own buffer while the key loop waits on the call, so there is
// one reader still. See Perform below.
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
	// What comes back carries the action's status in Line.Status, which is
	// 0 for every action but the incremental search.
	//
	// false is an action this editor will not perform from here, with the
	// line back untouched so a caller that reports the refusal and carries
	// on has not lost anything. This editor has none any more — the last
	// was the search, until #5895 — but the answer stays in the contract
	// for an editor that has one.
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

	// Paste reads the paste whose opening marker began this keystroke,
	// through its closing marker, and hands back its text **without putting
	// it in the line**.
	//
	// For the one spelling of the paste action that does not insert: zsh's
	// `zle .bracketed-paste NAME` stores the text in the parameter and leaves
	// the line alone, which is how a widget gets to look at a paste before
	// deciding what of it to keep. Measured against zsh 5.9.2, a paste of
	// `a⏎b` read that way leaves `$'a\nb'` in the parameter and the buffer as
	// it was. The text is the same text Perform would have inserted, line
	// endings as newlines, by the same reader — see paste.go.
	Paste() string

	// ReadKeyByte reads the next byte of input the way the editor reads the
	// first byte of a key — what was pushed back before the terminal — and
	// hands it over without acting on it. false is input that has ended.
	//
	// For an action that reads a key sequence of its own and decides what it
	// means: zsh's `read-command`, which a widget calls to learn what the
	// next key would run without running it. Nothing is read past and nothing
	// is turned into an abandoned line: a `^C` among keys pushed back is a
	// byte of what was pushed, as it is in a paste.
	ReadKeyByte() (byte, bool)

	// InputPending reports whether a byte is already in hand, pushed back or
	// delivered, so that reading one more would not wait on the terminal.
	// It is what decides whether a key that is complete as read, and also
	// the start of a longer one, is read on.
	InputPending() bool

	// ChangeNumber closes the change the line is in and reports its number:
	// a later change is a new one, and UndoTo with this number puts the line
	// back as it stands now. zsh's `UNDO_CHANGE_NO`, and its `split-undo`,
	// which is the same closing with the number not asked for.
	ChangeNumber(in Line) int

	// UndoTo takes back every change made since the one numbered n, and
	// hands back the line it left. zsh's `zle undo N`. false is a number
	// below the first change, which takes the line back to how it began and
	// says so. See undo.go.
	UndoTo(n int, in Line) (Line, bool)

	// CutBuffer is the text of the last kill, which is what a yank inserts,
	// and SetCutBuffer replaces it. zsh's `$CUTBUFFER`: a widget reads it to
	// join a kill of its own onto the one before, and assigning it — or
	// `zle copy-region-as-kill STRING` — makes the next yank insert what was
	// assigned (#5916). Measured 2026-10-04 through a pseudo-terminal against
	// zsh 5.9.2: `CUTBUFFER=hello` then `zle yank` inserts `hello`, the next
	// widget reads `hello` back, and after `zle backward-kill-word` over `one
	// two` it reads `two`.
	//
	// Asked for rather than carried in Line, because the kill is not the
	// line: it outlives it, and a front end that builds a Line of its own
	// knowing nothing about kills must not be able to empty one.
	CutBuffer() string
	SetCutBuffer(text string)

	// Message draws text on a row under the line, where it stays while the
	// line is edited until another replaces it, an empty one takes it away,
	// or the line ends. zsh's `zle -M` (#5942).
	Message(text string)

	// UndoLimit is the change an undo may not take the line back past, and
	// SetUndoLimit moves it. zsh's `$UNDO_LIMIT_NO`: 0 is none, and it is
	// the line's — measured 2026-10-04 against zsh 5.9.2, a limit a widget
	// set holds for the keys after it and is 0 again on the next line. The
	// numbers are ChangeNumber's. A numbered undo, UndoTo, is not limited.
	UndoLimit() int
	SetUndoLimit(n int)

	// WidgetCalled says a widget call that is not one of this editor's own
	// actions has finished: a widget of the shell's called from another, or
	// an action the shell performs itself. Neither is a kill, so a kill after
	// it starts afresh where it would have joined.
	//
	// Whether a kill joins the one before is decided by the widget call
	// before it, not by the keystroke — measured 2026-10-04 through a
	// pseudo-terminal against zsh 5.9.2, a yank after each (#5918):
	//
	//	w() { zle backward-kill-word; zle backward-kill-word }   one kill
	//	^W, then w() { zle k }, k() { zle backward-kill-word }   one kill
	//	w() { zle k; zle backward-kill-word }                    two kills
	//	^W, then w() { zle copy-region-as-kill X; zle backward-kill-word }
	//	                                                         two kills
	//	w() { zle backward-kill-word }, then ^W                  two kills
	//
	// The editor sees its own actions; this is how it hears about the rest.
	// An assignment to `CUTBUFFER` is not a call and does not end a run of
	// kills: one between two kills is joined onto.
	WidgetCalled()
}

// Every action this editor has can be performed from outside it, the two that
// read keys of their own included.
//
// **There used to be a refusal here, and it was measured wrong twice.** The
// reasoning both times was that an action which reads a key would be
// re-entering the read loop mid-keystroke. It is not: such an action reads
// through the editor's own buffer, and the key loop is waiting on the widget
// call rather than on the terminal, so there is still exactly one reader.
//
// A completion went first (#3043). It may stop to ask whether to list, and
// measured 2026-09-18 against zsh 5.9.2, a widget whose body is `zle
// complete-word` fills in the common part on one press and lists on the next
// — Tab's own two-keystroke rule, reached by name.
//
// The incremental search went second (#5895), and it was the one that cost
// most: zsh-autosuggestions wraps every widget, the search included, and its
// wrapper calls the original by name — so with the refusal, `C-r` printed an
// error and searched nothing for everyone running it. Measured 2026-10-04
// against zsh 5.9.2, a wrapper around `zle .history-incremental-search-backward`
// gets the whole search: its row under the line, its keys, and its ending,
// with the line it found in `$BUFFER` and the code after the call running
// before the key that ended the search does. See searchEnd.status for what
// the call answers.

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
	ownKeys := in.Keys != "" && in.Keys == string(a.e.keyBytes)
	a.e.take(in)
	if w == WidgetSelfInsert && ownKeys {
		// `self-insert` types the last character of the keys it is called
		// for, and that includes the keystroke's own: measured 2026-10-04
		// against zsh 5.9.2, a widget on `^Xm` that runs `zle .self-insert`
		// inserts `m`, and one on `ESC q` inserts `q` (#5926). take adopts
		// only keys that differ from the keystroke's — a `read-command`'s —
		// so a sequence's own last character was never what got typed, and
		// the line got whatever printable key came before it.
		a.e.adoptKeys(in.Keys)
	}
	a.e.actionStatus = 0
	// Whether this call's kill joins is the call before it's to say, and the
	// first call of a keystroke asks the keystroke before. See WidgetCalled.
	if a.e.called {
		a.e.killedBefore = a.e.killing
	}
	a.e.killing, a.e.called = false, true
	// Through runWidget and not a copy of it, which is the whole point: a key
	// bound to `up-line-or-history` and a widget that calls `zle
	// up-line-or-history` must be the same action, including where the cursor
	// lands and what the walk leaves behind at each step. A second
	// implementation here is how the two would come to disagree.
	//
	// As many times as the count says, which is the line's Numeric — zsh's
	// `$NUMERIC`, whether typed before the key or assigned by the widget.
	// See countedAction.
	w, times := countedAction(w, in.Numeric)
	back := -1
	if w == WidgetSelfInsert && in.Numeric != nil {
		// Typed as many times as the count says, and for a negative count
		// with the cursor left in front of what was typed — what a count
		// before a printable key does (see typeCounted). Measured from a
		// widget: `NUMERIC=3; zle .self-insert` on `^Xn` makes `ab` with the
		// cursor at 1 into `annnb` at 4, -3 into `annnb` at 1, and 0 types
		// nothing.
		times = *in.Numeric
		if times < 0 {
			times, back = -times, a.e.pos
		}
	}
	if w.takesItsCount() {
		// Told the count rather than played it: the call's own, which is
		// `$NUMERIC` as the widget left it, and no count at all when it was
		// unset, whatever the keystroke that ran the widget spent.
		saved := a.e.keyNumeric
		a.e.keyNumeric = in.Numeric
		a.e.runWidget(Binding{Widget: w}, a.e.live(a.prompt))
		a.e.keyNumeric = saved
		times = 0
	}
	for i := range times {
		if i > 0 && a.e.killing {
			// One kill for the whole count, measured: two words killed
			// with a count of 2 come back from one yank.
			a.e.killedBefore = true
		}
		a.e.runWidget(Binding{Widget: w}, a.e.live(a.prompt))
	}
	if back >= 0 {
		a.e.pos = back
	}
	if w == WidgetSendBreak {
		// Asked for by name, which rings no bell. See sendBreak.
		a.e.breakQuiet = true
	}
	if w.IsIncrementalSearch() {
		// The keys the widget is about are now the key that ended the
		// search: measured 2026-10-04 against zsh 5.9.2, `$KEYS` after the
		// call is `^M` when Return ended it, `^E` for `C-e`, `^G` for
		// `C-g`, and empty after `C-c`. The keystroke that ran the widget
		// is still the one the editor records as the last widget — that is
		// the binding's, and the binding is unchanged.
		a.e.keyBytes = append(a.e.keyBytes[:0], a.e.searchKey...)
	}
	out := a.e.give()
	out.Status = a.e.actionStatus
	return out, true
}

// countedActions is the actions a count performed from outside the editor
// plays more than once, each with the action a negative count turns it into.
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, from a
// widget on the line `aa bb cc dd ee` with the cursor at 7 that sets
// `NUMERIC` and calls the action: a count of 2 moves or deletes two
// characters or words, -2 does the same in the other direction, and 0 does
// nothing at all. `beginning-of-line` and `end-of-line` go by the same rule —
// -2 sends each to the other end and 0 leaves the cursor where it was — and
// going to an end twice is going there once (#5941).
var countedActions = map[Widget]Widget{
	WidgetForwardChar:        WidgetBackwardChar,
	WidgetBackwardChar:       WidgetForwardChar,
	WidgetForwardWord:        WidgetBackwardWord,
	WidgetBackwardWord:       WidgetForwardWord,
	WidgetKillWordAfter:      WidgetKillWordBefore,
	WidgetKillWordBefore:     WidgetKillWordAfter,
	WidgetDeleteChar:         WidgetBackwardDeleteChar,
	WidgetBackwardDeleteChar: WidgetDeleteChar,
	WidgetBeginningOfLine:    WidgetEndOfLine,
	WidgetEndOfLine:          WidgetBeginningOfLine,
}

// countedAction is the action a count makes of w, and how many times to
// perform it. With no count, or an action the count does not reach, that is
// w once — what every other action does whatever the count, and what a key
// pressed with a count already did by its own route (see prefixarg.go).
func countedAction(w Widget, numeric *int) (Widget, int) {
	opposite, ok := countedActions[w]
	if numeric == nil || !ok {
		return w, 1
	}
	n := *numeric
	if n < 0 {
		w, n = opposite, -n
	}
	return w, n
}

func (a editorActions) Redisplay(in Line) {
	a.e.take(in)
	a.e.redraw(a.e.live(a.prompt))
}

func (a editorActions) PushKeys(s string) { a.e.pushKeys(s) }

func (a editorActions) Paste() string {
	text, _ := a.e.readPaste()
	return string(text)
}

func (a editorActions) ReadKeyByte() (byte, bool) {
	var buf [1]byte
	for {
		n, err := a.e.nextByte(buf[:])
		if err != nil {
			return 0, false
		}
		if n == 1 {
			return buf[0], true
		}
	}
}

func (a editorActions) InputPending() bool { return a.e.inputPending() }

func (a editorActions) ChangeNumber(in Line) int {
	a.e.take(in)
	return a.e.changeNumber()
}

func (a editorActions) CutBuffer() string { return string(a.e.killed) }

func (a editorActions) SetCutBuffer(text string) { a.e.killed = []rune(text) }

func (a editorActions) Message(text string) {
	a.e.message = text
	a.e.redraw(a.e.live(a.prompt))
}

func (a editorActions) UndoLimit() int { return a.e.undoLimit }

func (a editorActions) SetUndoLimit(n int) { a.e.undoLimit = n }

func (a editorActions) WidgetCalled() { a.e.killing, a.e.called = false, true }

func (a editorActions) UndoTo(n int, in Line) (Line, bool) {
	a.e.take(in)
	reached := a.e.undoTo(n)
	return a.e.give(), reached
}

// take adopts the line an action outside the editor is holding, and give hands
// it back.
//
// The cursor is clamped on the way in for the reason Line.Cursor gives: an
// action that walked off the end is asking for the end, and refusing it would
// make every widget that sets `CURSOR=$#BUFFER` a special case.
//
// And what is drawn after the line, which is part of what the action is
// holding: a widget that set `POSTDISPLAY` and then called `zle .self-insert`
// still has it afterwards in zsh 5.9.2, measured 2026-10-04, and an editor
// that kept its own copy handed back the one from before the widget ran —
// the empty string, on every keystroke that had not drawn one yet (#5864).
//
// **What the action did to the line is a change**, kept the way a key's is,
// so that an undo asked for afterwards can take it back. Measured 2026-10-04
// against zsh 5.9.2 through a pseudo-terminal, a widget on the line `xy`
// running `BUFFER=one; zle .split-undo; LBUFFER+=two; zle .undo` is left with
// `one`: the assignments were each a change. Taking the line over silently
// left the stack holding only what keys had done, so an undo inside a widget
// took back the last key before it instead.
//
// And the keys the action says it is about, where they are not the ones this
// keystroke read: `read-command` reads a key for a widget, and a `zle
// self-insert` after it inserts *that* key. See adoptKeys.
func (e *editor) take(in Line) {
	line := []rune(in.Buffer)
	if !slices.Equal(line, e.line) {
		e.changes = append(e.changes, snapshot{line: e.line, pos: e.pos})
	}
	e.line = line
	e.pos = min(max(in.Cursor, 0), len(e.line))
	e.postdisplay = in.Postdisplay
	if in.Keys != "" && in.Keys != string(e.keyBytes) {
		e.adoptKeys(in.Keys)
	}
}

func (e *editor) give() Line {
	var prebuffer string
	if e.prebuffer != nil {
		prebuffer = e.prebuffer()
	}
	return Line{Prebuffer: prebuffer, Buffer: string(e.line), Cursor: e.pos, Postdisplay: e.postdisplay, Last: e.last, ViCommand: e.viCommand, Numeric: e.keyNumeric, Keys: string(e.keyBytes)}
}

// adoptKeys makes keys an action read the ones a self-insert types.
//
// A byte at a time where the keys are part of a character, because that is
// how they arrive: measured 2026-10-04 against zsh 5.9.2, `read-command` over
// a pushed `é` answers twice, `self-insert` with one byte in `$KEYS` each
// time. So the first half is held and types nothing, and the second completes
// the character. Otherwise the last character of the keys is what is typed.
//
// A line ending types a newline, which is what a paste puts in the line for
// one — see pastedRunes — and is the case a paste read a key at a time
// arrives with: zsh's paste function runs `zle .self-insert` for every pasted
// key whose own widget it does not run, the newlines among them, so that the
// paste stays text. Any other control character types nothing, which is what
// this editor does with one pressed and with one pasted.
func (e *editor) adoptKeys(keys string) {
	if len(e.partialKey) > 0 || len(keys) == 1 && keys[0] >= utf8.RuneSelf {
		e.partialKey = append(e.partialKey, keys...)
		e.typedKey = 0
		if !utf8.FullRune(e.partialKey) {
			return
		}
		keys, e.partialKey = string(e.partialKey), nil
	}
	r, _ := utf8.DecodeLastRuneInString(keys)
	if r == '\r' || r == '\n' {
		e.typedKey = '\n'
		return
	}
	if r == utf8.RuneError || r < 0x20 || r == del {
		e.typedKey = 0
		return
	}
	e.typedKey = r
}

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
