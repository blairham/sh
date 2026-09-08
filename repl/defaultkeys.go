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
