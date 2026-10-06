// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// Which key each of this editor's actions arrives on before anyone rebinds
// anything.
//
// This is the editor's own dispatch stated as data, and it is here rather than
// beside a shell's names for the same reason Widget is: the keys are one
// editor's and the names are each dialect's. A shell's key-listing command —
// `bindkey` with nothing after it, `bind -p` — has to print a whole keymap,
// and every one of them would otherwise keep a private copy of this table
// written out by hand under its own names.
//
// **Two copies of it is the bug this file exists to stop.** The zsh dialect
// held the only copy until now and had already drifted from the dispatch: the
// editor kills back a word on `M-^H` as well as on `M-Delete`, which
// editing.md records as measured in both shells, and the hand-written table
// listed only the second — so `bindkey` reported that key as unbound in a
// shell where pressing it works. A second dialect writing its own copy is how
// that becomes two wrong answers instead of one, which is why the bash side
// derives its listing from this and names the actions rather than the keys.
//
// What is *not* here is every key the editor reads. `^C`, `^D`, `^G` and the
// two that accept a line are the editor's own control flow rather than
// actions a key can be bound to — there is no Widget for them, because a
// widget with nothing behind it would be a name a person could bind and press
// to no effect (see widgets.go). A dialect that wants to print a name for
// those keys supplies it, because what to call them is the same dialect
// question as the rest and the answers differ: one shell prints `accept-line`
// for Return and the other `accept-line` for Return and `self-insert` for
// every ordinary character.

// defaultKeys is the table. Unexported and copied out, so that a dialect
// holding the result cannot reach in and change what the next caller sees.
var defaultKeys = map[string]Widget{
	"\x01":     WidgetBeginningOfLine,
	"\x02":     WidgetBackwardChar,
	"\x06":     WidgetForwardChar,
	"\x05":     WidgetEndOfLine,
	"\x08":     WidgetBackwardDeleteChar,
	"\x09":     WidgetComplete,
	"\x0b":     WidgetKillLine,
	"\x0c":     WidgetClearScreen,
	"\x0e":     WidgetNextHistory,
	"\x10":     WidgetPreviousHistory,
	"\x12":     WidgetSearchHistoryBackward,
	"\x13":     WidgetSearchHistoryForward,
	"\x14":     WidgetTransposeChars,
	"\x15":     WidgetKillWholeLine,
	"\x17":     WidgetKillWordBefore,
	"\x18\x15": WidgetUndo,
	"\x19":     WidgetYank,
	"\x1f":     WidgetUndo,
	"\x7f":     WidgetBackwardDeleteChar,

	// The escape-prefixed keys. Both cases of each letter, which is measured
	// in both shells — see editing.md, where `M-B`, `M-F` and `M-D` do what
	// the lower-case letters do.
	"\x1bb":    WidgetBackwardWord,
	"\x1bB":    WidgetBackwardWord,
	"\x1bf":    WidgetForwardWord,
	"\x1bF":    WidgetForwardWord,
	"\x1bd":    WidgetKillWordAfter,
	"\x1bD":    WidgetKillWordAfter,
	"\x1b.":    WidgetInsertLastWord,
	"\x1b_":    WidgetInsertLastWord,
	"\x1b\x7f": WidgetKillWordBefore,
	// `M-^H` alongside `M-Delete`, which is the entry the one hand-written
	// copy of this table was missing.
	"\x1b\x08": WidgetKillWordBefore,

	// The keys a terminal sends as a control sequence, in both the forms a
	// terminal sends them in — `\e[` when the keypad is in its normal mode
	// and `\eO` when it is not.
	"\x1b[A":  WidgetPreviousHistory,
	"\x1b[B":  WidgetNextHistory,
	"\x1b[C":  WidgetForwardChar,
	"\x1b[D":  WidgetBackwardChar,
	"\x1bOA":  WidgetPreviousHistory,
	"\x1bOB":  WidgetNextHistory,
	"\x1bOC":  WidgetForwardChar,
	"\x1bOD":  WidgetBackwardChar,
	"\x1b[H":  WidgetBeginningOfLine,
	"\x1b[F":  WidgetEndOfLine,
	"\x1b[3~": WidgetDeleteChar,
	// Home and End again, in the numbered spelling, and two numbers each
	// because terminals disagree about which — see escape.go, where the same
	// four are read. The one hand-written copy of this table had none of the
	// four.
	"\x1b[1~": WidgetBeginningOfLine,
	"\x1b[7~": WidgetBeginningOfLine,
	"\x1b[4~": WidgetEndOfLine,
	"\x1b[8~": WidgetEndOfLine,

	// A paste's opening marker, which both shells list as a key — see
	// WidgetBracketedPaste. Only the opening one: the closing marker is read
	// by the paste, and on its own it is a sequence nothing acts on.
	"\x1b[200~": WidgetBracketedPaste,
}

// controlXSearchKeys are the keys EditorStyle.SearchOnControlX adds to the
// table: the two plain searches' second spellings, which one dialect's keymap
// has and the other's does not (#5904).
var controlXSearchKeys = map[string]Widget{
	"\x18r": WidgetSearchHistoryBackward,
	"\x18s": WidgetSearchHistoryForward,
}

// ControlXSearchBindings is controlXSearchKeys for a dialect's key listing,
// as a copy for the reason DefaultBindings gives.
func ControlXSearchBindings() map[string]Widget {
	out := make(map[string]Widget, len(controlXSearchKeys))
	for seq, w := range controlXSearchKeys {
		out[seq] = w
	}
	return out
}

// wordKeys are the keys EditorStyle.WordKeys adds to the table, in the emacs
// keymap only: the case keys, transpose-words and quoted-insert, which both
// dialects' emacs keymaps have (#6241, #6250).
//
// Measured 2026-10-06 with `bindkey -M emacs` on zsh 5.9.2 and `bind -p` on
// bash 5.3.20, and the upper-case spellings do the same as the lower in both
// — bash lists them as do-lowercase-version. What the keys do is not quite
// the same in the two: see EditorStyle.CapitalizeTakesTheFirstCharacter and
// TransposeWordsReachesTheLineEnd.
var wordKeys = map[string]Widget{
	"\x1bu": WidgetUpCaseWord,
	"\x1bU": WidgetUpCaseWord,
	"\x1bl": WidgetDownCaseWord,
	"\x1bL": WidgetDownCaseWord,
	"\x1bc": WidgetCapitalizeWord,
	"\x1bC": WidgetCapitalizeWord,
	"\x1bt": WidgetTransposeWords,
	"\x1bT": WidgetTransposeWords,
	"\x16":  WidgetQuotedInsert,
}

// quotedInsertKey is `^V`, which one dialect has in vi insert mode too. See
// EditorStyle.QuotedInsertInViInsert.
const quotedInsertKey = "\x16"

// wideEmacsKeys are the keys EditorStyle.WideEmacsKeymap adds to the table
// beyond wordKeys, in the emacs keymap only: what one dialect's emacs keymap
// binds and the other's either leaves alone or binds to something else
// (#6241).
//
// Measured 2026-10-06 with `bindkey -M emacs` on zsh 5.9.2, and the upper-case
// spellings are bound to the same widgets as the lower. bash has nothing on
// `M-q`, `M-'`, `M-a` or `^X u`.
var wideEmacsKeys = map[string]Widget{
	"\x1bq": WidgetPushLine,
	"\x1bQ": WidgetPushLine,
	"\x1b'": WidgetQuoteLine,
	"\x1ba": WidgetAcceptAndHold,
	"\x1bA": WidgetAcceptAndHold,
	"\x18u": WidgetUndo,
	"\x1b?": WidgetWhichCommand,
	"\x1bh": WidgetRunHelp,
	"\x1bH": WidgetRunHelp,
	"\x1bx": WidgetExecuteNamedCmd,
}

// viInsertKeys are the keys EditorStyle.ViQuotedInsert adds to vi insert
// mode's table: vi-quoted-insert on `^V`, zsh's viins keymap's (#6251).
var viInsertKeys = map[string]Widget{
	"\x16": WidgetViQuotedInsert,
}

// ViInsertBindings is viInsertKeys for a dialect's key listing, as a copy.
func ViInsertBindings() map[string]Widget {
	out := make(map[string]Widget, len(viInsertKeys))
	for seq, w := range viInsertKeys {
		out[seq] = w
	}
	return out
}

// WideEmacsBindings is what EditorStyle.WideEmacsKeymap puts on the emacs
// keymap, wordKeys included, for a dialect's key listing — as a copy for the
// reason DefaultBindings gives.
func WideEmacsBindings() map[string]Widget {
	out := WordBindings()
	for seq, w := range wideEmacsKeys {
		out[seq] = w
	}
	return out
}

// WordBindings is wordKeys for a dialect's key listing, as a copy for the
// reason DefaultBindings gives.
func WordBindings() map[string]Widget {
	out := make(map[string]Widget, len(wordKeys))
	for seq, w := range wordKeys {
		out[seq] = w
	}
	return out
}

// defaultKey is what this editor does with a key nobody rebound: the shared
// table, and the dialect's own additions to it.
func (e *editor) defaultKey(seq string) (Widget, bool) {
	if w, ok := defaultKeys[seq]; ok {
		return w, true
	}
	if e.searchOnControlX {
		if w, ok := controlXSearchKeys[seq]; ok {
			return w, true
		}
	}
	if !e.viEditing() {
		if e.wideEmacsKeymap || e.wordKeys {
			if w, ok := wordKeys[seq]; ok {
				return w, true
			}
		}
		if e.wideEmacsKeymap {
			if w, ok := wideEmacsKeys[seq]; ok {
				return w, true
			}
		}
	} else if e.quotedInsertInViInsert && seq == quotedInsertKey {
		return WidgetQuotedInsert, true
	}
	if e.viQuotedInsertKey && e.viEditing() && !e.viCommand {
		if w, ok := viInsertKeys[seq]; ok {
			return w, true
		}
	}
	return WidgetNone, false
}

// DefaultBindings is the key each of this editor's actions arrives on with
// nothing rebound, as the bytes a terminal sends.
//
// For a dialect's key-listing command, which has to answer for a whole keymap
// and not only for what somebody changed. The caller gets a copy, because the
// answer to "what does this key do" must not be changed by whoever asked it
// last.
func DefaultBindings() map[string]Widget {
	out := make(map[string]Widget, len(defaultKeys))
	for seq, w := range defaultKeys {
		out[seq] = w
	}
	return out
}
