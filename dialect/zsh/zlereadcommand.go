// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// What a widget function asks of the editor that no key does.
//
// Two widgets in zsh exist to be called from a function and mean nothing on
// a key: `read-command`, which reads the next key sequence and says what it
// would run without running it, and `split-undo`, which ends the change the
// line is in. With them come `zle -K`, which says which keymap the keys after
// it are read in, and the two numbers an undo is steered by, `$KEYS` and
// `$UNDO_CHANGE_NO`.
//
// They arrived together because one function needs all of them. zsh's
// `bracketed-paste-magic`, bound as the paste widget, takes the paste into a
// parameter, pushes it back, reads it a key at a time with `read-command` so
// that each key can run the widget it is bound to, and then undoes back to
// the change number it noted and pushes the result back as one paste. Where
// any of those was missing the paste ran as typed keys — its first line run
// by its first newline — or reported an error on every paste (#5880).

// widgetFunctionActions are the widgets a function calls and no key runs, by
// name. Built in, so `zle -la` lists them and `.read-command` reaches them
// past a redefinition, and with no repl.Widget behind them because the editor
// has no key that does either.
var widgetFunctionActions = map[string]func(*interp.Runner, repl.Actions, []string) int{
	"read-command":        readCommand,
	"split-undo":          splitUndo,
	"copy-region-as-kill": copyRegionAsKill,
}

// copyRegionAsKill is `zle copy-region-as-kill STRING`: the string put where
// the next yank takes it from, and the line left alone. Measured against zsh
// 5.9.2: line and cursor untouched, a following yank inserts the string, and
// a builtin kill straight after it starts afresh rather than joining it.
//
// Only the spelling with a string. Without one it copies from the mark to the
// cursor, and this editor has no mark to copy from, so that is refused by
// name rather than answered with something else. A second operand is
// ignored, measured: `zle copy-region-as-kill a b` kills `a`.
func copyRegionAsKill(r *interp.Runner, _ repl.Actions, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("copy-region-as-kill without a string is not implemented yet\n")
		return 1
	}
	// Into the kill `$CUTBUFFER` reads, which callBuiltinWidget hands to the
	// editor when this returns. See repl.Actions.CutBuffer.
	r.SetVar(zleCutBuffer, args[0])
	return 0
}

// readCommand is `zle read-command`: read one key sequence the way the
// editor would, set `$REPLY` to the widget it is bound to and `$KEYS` to the
// keys, and run nothing.
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, a widget
// that pushes keys with `zle -U` and calls `zle .read-command` four times:
//
//	pushed a, ^A, \e[A, \eb    self-insert a / beginning-of-line ^A /
//	                           up-line-or-history \e[A / backward-word \eb,
//	                           each at status 0
//	pushed é                   self-insert once, the whole character in
//	                           $KEYS, under a UTF-8 LANG; under LANG=C or
//	                           none, twice, one byte each time (#5949)
//	pushed ^X q                undefined-key, $KEYS ^Xq, status 0 — the
//	                           prefix and the byte that went nowhere both
//	                           read
//	pushed ^X ^U               undo
//
// and an argument after the name is ignored, and `$LASTWIDGET` is
// `.read-command` afterwards, which is the caller's to set.
//
// A key that is complete as read and also begins a longer binding is read on
// only where another byte is already in hand; zsh waits `$KEYTIMEOUT` for
// one, and this editor's reads do not time out, so a key that is not already
// there is taken as the shorter one. Where the byte read on leads nowhere it
// is put back and the shorter binding is the answer.
//
// The command keymap is refused rather than answered: its keys are a second
// dispatch in this editor and not a table — see repl/vi.go — so a name read
// out of the table would be wrong for every key nobody bound there.
func readCommand(r *interp.Runner, a repl.Actions, _ []string) int {
	if keymap, _ := r.GetVar(zleKeymap); keymap == "vicmd" {
		r.Diagnosef("read-command in the vicmd keymap is not implemented yet\n")
		return 1
	}
	table := readBindings(r, currentKeymap(r))
	var seq []byte
	matched, matchedLen := "", 0
	for {
		b, ok := a.ReadKeyByte()
		if !ok {
			return 1
		}
		seq = append(seq, b)
		if len(seq) == 1 {
			if char, whole := readCharacter(r, a, table, seq); whole {
				r.SetVar("REPLY", "self-insert")
				r.SetVar(zleKeys, string(char))
				return 0
			}
		}
		name, exact := readCommandBinding(table, seq)
		if exact {
			matched, matchedLen = name, len(seq)
		}
		if longerBindingThan(table, string(seq)) && (!exact || a.InputPending()) {
			continue
		}
		if !exact && matchedLen > 0 {
			// Read on past a complete key and found nothing: the bytes after
			// it go back for the next read.
			a.PushKeys(string(seq[matchedLen:]))
			seq = seq[:matchedLen]
		}
		if matchedLen != len(seq) {
			matched = undefinedKey
		}
		break
	}
	if strings.HasPrefix(matched, `"`) {
		r.Diagnosef("read-command over a key bound to a string is not implemented yet\n")
		return 1
	}
	r.SetVar("REPLY", matched)
	r.SetVar(zleKeys, string(seq))
	return 0
}

// readCharacter reads the rest of a character the locale spells in more than
// one byte, where seq holds its first byte and nothing is bound to that byte
// alone, and reports whether what it read is one whole character — which
// types itself, as any character does that nothing is bound to.
//
// Measured 2026-10-04 against zsh 5.9.2 (#5949): under a UTF-8 LANG a pushed
// `é` is one key, `$KEYS` holding both bytes; under LANG=C it is two. A
// widget that loops over read-command and strips each key off a copy of the
// text never makes progress on half a character, so the difference is a hang
// and not a detail. The bytes are read only where they are already in hand,
// as the longer bindings above are; a character cut short is the byte read so
// far, and the caller goes on with it as before.
func readCharacter(r *interp.Runner, a repl.Actions, table map[string]string, seq []byte) ([]byte, bool) {
	if seq[0] < utf8.RuneSelf {
		return nil, false
	}
	if _, bound := table[string(seq)]; bound {
		return nil, false
	}
	char := seq
	for len(char) < utf8.UTFMax && !utf8.FullRune(char) && a.InputPending() {
		b, ok := a.ReadKeyByte()
		if !ok {
			break
		}
		char = append(char, b)
	}
	if len(char) > 1 && r.CharacterLength(string(char)) == len(char) {
		return char, true
	}
	if len(char) > 1 {
		// Not one character after all: the bytes past the first go back.
		a.PushKeys(string(char[1:]))
	}
	return nil, false
}

// readCommandBinding is the widget seq is bound to, in this keymap's table
// or as a printable byte, which types itself.
//
// The table holds what this editor acts on and what somebody bound, and not
// the self-inserting keys, which zsh's emacs keymap lists by range — `" "`
// to `"~"` and every byte with the high bit set — and which this editor types
// without a binding. A byte somebody did bind answers from the table.
func readCommandBinding(table map[string]string, seq []byte) (string, bool) {
	if name, bound := table[string(seq)]; bound {
		return name, true
	}
	if len(seq) == 1 && seq[0] >= ' ' && seq[0] != 0x7f {
		return "self-insert", true
	}
	return "", false
}

// longerBindingThan reports whether some binding in the table begins with seq
// and goes on past it.
func longerBindingThan(table map[string]string, seq string) bool {
	for bound := range table {
		if len(bound) > len(seq) && strings.HasPrefix(bound, seq) {
			return true
		}
	}
	return false
}

// splitUndo is `zle split-undo`: the change the line is in ends here, so an
// undo afterwards takes back what follows and stops at this. Measured
// 2026-10-04 against zsh 5.9.2, status 0 with or without an argument, and
// `BUFFER=one; zle .split-undo; LBUFFER+=two; zle .undo` leaves `one`.
func splitUndo(r *interp.Runner, a repl.Actions, _ []string) int {
	a.ChangeNumber(widgetLine(r))
	return 0
}

// undoChangeNumberName is the parameter a widget reads the current change's
// number from, to hand to `zle undo` later.
const undoChangeNumberName = "UNDO_CHANGE_NO"

// openUndoChangeNumber gives the widget `$UNDO_CHANGE_NO`, answered by the
// editor on each read.
//
// Reading it closes the change the line is in, so the number is one an undo
// can return to: measured 2026-10-04 against zsh 5.9.2, read twice with
// nothing between it is the same number, and read again after `BUFFER=one`
// it is one more. `integer-local-readonly-special` there, the same word the
// queue counters carry. See repl's changeNumber for how it is counted.
//
// Opened beside the widget's other parameters and not among them, because
// it needs the editor and they do not: a widget run with no editor on the
// other end has no number to give, and the name is then simply not there.
func openUndoChangeNumber(r *interp.Runner, a repl.Actions, scope int) {
	r.SetDynamic(undoChangeNumberName, func(rr *interp.Runner) string {
		return strconv.Itoa(a.ChangeNumber(widgetLine(rr)))
	})
	r.MarkInteger(undoChangeNumberName)
	r.MarkReadonly(undoChangeNumberName)
	r.MarkLocal(undoChangeNumberName)
	r.SetDynamicDeclaration(undoChangeNumberName, interp.ProducedDeclaration{Integer: true, Base: 10, LocalToScope: scope})
	// And the limit an undo stops at, which is the editor's to keep: it
	// outlives the widget that set it and is 0 on the next line (#5898).
	// Measured 2026-10-04 against zsh 5.9.2, `integer-local-special` and 0
	// in a widget that has set nothing, and outside a widget not there at
	// all — so a widget that assigns it without `local`, as
	// bracketed-paste-magic does, leaves no global behind.
	r.SetDynamic(undoLimitName, func(*interp.Runner) string {
		return strconv.Itoa(a.UndoLimit())
	})
	r.SetDynamicWriter(undoLimitName, func(_ *interp.Runner, value string) {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			a.SetUndoLimit(n)
		}
	})
	r.MarkInteger(undoLimitName)
	r.MarkLocal(undoLimitName)
	r.SetDynamicDeclaration(undoLimitName, interp.ProducedDeclaration{Integer: true, Base: 10, LocalToScope: scope})
}

// undoLimitName is the parameter a widget sets the undo limit through.
const undoLimitName = "UNDO_LIMIT_NO"

// closeUndoParameters takes the two away again, attributes and all, so a
// script between keystrokes finds ordinary names.
func closeUndoParameters(r *interp.Runner) {
	for _, name := range []string{undoChangeNumberName, undoLimitName} {
		r.UnsetDynamic(name)
		r.UnsetDynamicDeclaration(name)
		r.UnmarkInteger(name)
	}
}

// selectWidgetKeymap is `zle -K NAME`: the keys read after it, for the rest
// of the line, are read in that keymap.
//
// Measured 2026-10-04 against zsh 5.9.2. Inside a widget, `zle -K vicmd`
// leaves `$KEYMAP` reading `vicmd` and `zle -K main` reading `main`, both at
// status 0; a keymap that does not exist is status 1 and not a word; and
// `zle -K` alone is `not enough arguments for -K` and with two names `too
// many arguments for -K`, each at status 1. Outside a widget the count is
// still asked first, and then it is `can only be called from widget
// function`.
//
// This editor has two key dispatches, the one a line is typed in and vi's
// command mode, and it changes between them only by its own vi keys. So
// naming the keymap the line is already being read in is answered — that is
// what a function that switched for a while and is putting it back asks —
// and naming the other one, or a keymap this editor has no dispatch for, is
// refused out loud rather than reported as done.
func selectWidgetKeymap(r *interp.Runner, ctx context.Context, args []string) int {
	switch {
	case len(args) == 0:
		r.Diagnosef("not enough arguments for -K\n")
		return 1
	case len(args) > 1:
		r.Diagnosef("too many arguments for -K\n")
		return 1
	}
	if _, inside := repl.ActionsFrom(ctx); !inside || !insideWidget(r) {
		r.Diagnosef("can only be called from widget function\n")
		return 1
	}
	name := args[0]
	if !slices.Contains(keymapsNow(r), name) {
		return 1
	}
	current, _ := r.GetVar(zleKeymap)
	if keymapIsCommandMode(r, name) != keymapIsCommandMode(r, current) || !keymapHasADispatch(r, name) {
		r.Diagnosef("-K: changing to the %s keymap is not implemented yet\n", name)
		return 1
	}
	r.SetVar(zleKeymap, name)
	return 0
}

// keymapIsCommandMode reports whether the keymap is the one vi's command mode
// reads keys in.
func keymapIsCommandMode(_ *interp.Runner, name string) bool { return name == "vicmd" }

// keymapHasADispatch reports whether this editor reads keys in the keymap at
// all: `main`, whichever keymap `main` is currently, and vi's command mode.
func keymapHasADispatch(r *interp.Runner, name string) bool {
	id, _ := keymapID(r, name)
	return name == "main" || name == "vicmd" || id == currentKeymap(r)
}
