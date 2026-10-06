// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `bindkey` maps a key sequence onto an editor widget.
//
// Measured 2026-09-05 against zsh 5.9.2, both with `-c` and one keystroke at a
// time under a pseudo-terminal, with a scratch HOME and no startup files.
//
// **The finding that shaped this is that an unknown widget is not an error.**
// `bindkey '^X^T' no-such-widget` is status 0 and silence, and the binding is
// stored: `bindkey '^X^T'` says `no-such-widget` back. That holds under a
// terminal with the editor loaded as much as under `-c`. So the interesting
// question — what to say to a name we do not have — turns out to have zsh's
// answer already: say nothing, keep it, and let the key do nothing. Which is
// exactly what a real rc file needs, because the two `bindkey` lines in the
// one that opened #840 name `history-substring-search-up` and `-down`, widgets
// belonging to a plugin that neither shell has without it.
//
// A name this editor *does* have is bound for real. `bindkey '^X^A'
// beginning-of-line` moves the cursor when the key is pressed, through the
// override layer in repl — see repl/bindings.go for why it is an override
// layer and not a keymap.
//
// What is deliberately not built, and why each is a refusal rather than a
// guess:
//
//   - **The default keymaps are this editor's keys, not zsh's 117.** Listing
//     `vi-match-bracket` for `^X^B` because zsh does would be naming a widget
//     nothing here performs, and `bindkey` is a question a person asks to find
//     out what a key does. What it answers is what this editor will actually
//     do. The same rule `compgen` follows: an answer that cannot be
//     generated is refused rather than invented.
//   - **`-p`, `-R`, `-D` and `-d`** — prefix bindings, ranges of keys, and
//     destroying a keymap — are refused as not implemented, in the wording
//     `whence` uses for the same case, so a script can tell a shell that
//     lacks something from a typo. Making and aliasing one, `-N` and `-A`,
//     are in keymaps.go (#5969).

// bindkeyStore is the table of what a person rebound, and bindkeyMap is which
// keymap is current.
//
// In the Runner's tables under names no script can reach, the way `emulate`
// keeps its mode — which is also what gives a subshell its own copy, measured:
// `(bindkey -r '^A')` leaves the parent's bindings alone.
const (
	bindkeyStore = ".zsh.bindkey"
	bindkeyMap   = ".zsh.keymap"
)

// keymapNames are the keymaps this shell starts with, in the order `bindkey
// -l` prints them, which is alphabetical with the dot-prefixed one first.
//
// All nine are named because `bindkey -M vicmd …` in an rc file must not fail
// for a keymap this shell plainly has; what differs is that only the two the
// editor can be driven from carry bindings.
//
// **Not the whole list a command sees** — a module creates keymaps when it
// loads, and `zmodload zsh/complist` is two lines before `bindkey -M
// menuselect …` in a real startup file. keymapsNow is what every reader here
// asks, and complist.go carries the argument for what such a keymap is and is
// not.
var keymapNames = []string{".safe", "command", "emacs", "isearch", "main", "vicmd", "viins", "viopp", "visual"}

// bindkeyWidgets is this shell's name for each thing the editor does.
//
// The dialect's half of the split repl/widgets.go describes: the substrate
// names the actions and this names them the way this shell does. The other
// shell with an editor calls four of these something else — `previous-history`
// where this one says `up-line-or-history` — which is the reason the mapping
// is here rather than there.
var bindkeyWidgets = map[string]repl.Widget{
	"beginning-of-line":     repl.WidgetBeginningOfLine,
	"vi-beginning-of-line":  repl.WidgetBeginningOfLine,
	"end-of-line":           repl.WidgetEndOfLine,
	"vi-end-of-line":        repl.WidgetEndOfLine,
	"backward-char":         repl.WidgetBackwardChar,
	"vi-backward-char":      repl.WidgetBackwardChar,
	"forward-char":          repl.WidgetForwardChar,
	"vi-forward-char":       repl.WidgetForwardChar,
	"backward-word":         repl.WidgetBackwardWord,
	"vi-backward-word":      repl.WidgetBackwardWord,
	"forward-word":          repl.WidgetForwardWord,
	"vi-forward-word":       repl.WidgetForwardWord,
	"kill-line":             repl.WidgetKillLine,
	"vi-kill-eol":           repl.WidgetKillLine,
	"kill-whole-line":       repl.WidgetKillWholeLine,
	"backward-kill-line":    repl.WidgetBackwardKillLine,
	"backward-kill-word":    repl.WidgetKillWordBefore,
	"vi-backward-kill-word": repl.WidgetKillWordBefore,
	"kill-word":             repl.WidgetKillWordAfter,
	"yank":                  repl.WidgetYank,
	"transpose-chars":       repl.WidgetTransposeChars,
	// The case keys and the word swap, on `M-u`, `M-l`, `M-c` and `M-t` in
	// the emacs keymap. See repl/casewords.go (#6241).
	"up-case-word":    repl.WidgetUpCaseWord,
	"down-case-word":  repl.WidgetDownCaseWord,
	"capitalize-word": repl.WidgetCapitalizeWord,
	"transpose-words": repl.WidgetTransposeWords,
	"quote-line":      repl.WidgetQuoteLine,
	"accept-and-hold": repl.WidgetAcceptAndHold,
	// Typing. Not a key anybody binds — it is what a printable key does when
	// nothing else claims it — but a name a shell can *redefine*, which is what
	// a syntax highlighter needs: it wraps every name in `$widgets`, and the one
	// that matters most is the one that runs when a person types (#2485).
	// Without it, `${+widgets[self-insert]}` read 0 here and 1 in the shell
	// being imitated, and a highlighter loaded and never saw a keystroke.
	"self-insert":          repl.WidgetSelfInsert,
	"up-line-or-history":   repl.WidgetPreviousHistory,
	"up-history":           repl.WidgetPreviousHistory,
	"down-line-or-history": repl.WidgetNextHistory,
	"down-history":         repl.WidgetNextHistory,
	// The searching pair, which walks only to entries beginning with the
	// line's first word. **macOS's `/etc/zshrc` binds the arrows to these**,
	// by `$terminfo[kcuu1]` and its fellows, so on that machine they are not
	// an advanced spelling a person opts into — they are what Up and Down do
	// in a shell with no user rc at all. Missing from this table, they were a
	// key bound to nothing (#2435).
	"up-line-or-search":                   repl.WidgetPreviousHistoryMatching,
	"down-line-or-search":                 repl.WidgetNextHistoryMatching,
	"history-incremental-search-backward": repl.WidgetSearchHistoryBackward,
	"clear-screen":                        repl.WidgetClearScreen,
	"delete-char":                         repl.WidgetDeleteChar,
	"vi-delete-char":                      repl.WidgetDeleteChar,
	"backward-delete-char":                repl.WidgetBackwardDeleteChar,
	"vi-backward-delete-char":             repl.WidgetBackwardDeleteChar,
	"expand-or-complete":                  repl.WidgetComplete,
	"complete-word":                       repl.WidgetComplete,
	// The prefix spelling is this editor's completion *exactly*, and that is
	// the one row of this table that is more accurate than the two above it.
	// This editor completes the text before the cursor and replaces only that
	// — repl's Completion says where the word starts and where the cursor is,
	// and nothing about where the word ends. Measured 2026-09-18 through a
	// pseudo-terminal against zsh 5.9.2, cursor placed after `uniq` in
	// `cat uniqXYZ`: `expand-or-complete-prefix` inserted the `_` the three
	// matches agree on and left `XYZ` alone, where `complete-word` rang the
	// bell, having found nothing that matches `uniqXYZ` whole.
	"expand-or-complete-prefix": repl.WidgetComplete,
	// The listing on its own key, and the menu. See repl/widgets.go, where
	// the measurements are, and repl/completemenu.go, which is the whole of
	// what they do. All six of the builtin completion widgets #3043 named
	// answer something now, so `zle -C` has no completer left that resolves
	// to nothing.
	"list-choices":            repl.WidgetListChoices,
	"delete-char-or-list":     repl.WidgetDeleteCharOrList,
	"menu-complete":           repl.WidgetMenuComplete,
	"menu-expand-or-complete": repl.WidgetMenuComplete,
	"reverse-menu-complete":   repl.WidgetMenuCompleteBackward,
	"undo":                    repl.WidgetUndo,
	"vi-undo-change":          repl.WidgetUndo,
	"insert-last-word":        repl.WidgetInsertLastWord,

	// The three that move between insert and command mode, which are the only
	// vi-only actions repl names — see repl/widgets.go, which carries the
	// decision and why the motions are not among them. This shell's names.
	"vi-cmd-mode":   repl.WidgetViCommandMode,
	"vi-insert":     repl.WidgetViInsertMode,
	"vi-insert-bol": repl.WidgetViInsertMode,
	"vi-add-next":   repl.WidgetViAppendMode,
	// A paste the terminal marked, on `^[[200~` in the standard keymap. See
	// repl.WidgetBracketedPaste, and callBuiltinWidget for the spelling that
	// stores the paste rather than inserting it.
	"bracketed-paste": repl.WidgetBracketedPaste,
	// What `up-line-or-beginning-search` and its partner are made of — see
	// repl/beginningsearch.go for the measurement (#5910).
	"history-beginning-search-backward": repl.WidgetHistoryBeginningSearchBackward,
	"history-beginning-search-forward":  repl.WidgetHistoryBeginningSearchForward,
	"up-line":                           repl.WidgetUpLine,
	"down-line":                         repl.WidgetDownLine,
	// The prompt drawn again, which a widget that changed what it shows asks
	// for (#5940).
	"reset-prompt":      repl.WidgetResetPrompt,
	"send-break":        repl.WidgetSendBreak,
	"recursive-edit":    repl.WidgetRecursiveEdit,
	"push-line":         repl.WidgetPushLine,
	"push-line-or-edit": repl.WidgetPushLineOrEdit,
	// The key that means "this key does nothing", which is what `-r` leaves
	// behind and what `bindkey` prints for a key nobody bound.
	undefinedKey: repl.WidgetNone,

	// The other three incremental searches. One mechanism with
	// history-incremental-search-backward; see repl's search.go (#5904).
	"history-incremental-search-forward":          repl.WidgetSearchHistoryForward,
	"history-incremental-pattern-search-backward": repl.WidgetPatternSearchHistoryBackward,
	"history-incremental-pattern-search-forward":  repl.WidgetPatternSearchHistoryForward,
}

// undefinedKey is what this shell calls a key with nothing on it.
const undefinedKey = "undefined-key"

// defaultBindings is what each keymap does before anyone changes it: the keys
// this editor acts on, under this shell's names for them.
//
// Derived from repl.DefaultBindings rather than written out again, because the
// keys are the editor's and only the names are this shell's — and because the
// copy that used to be written out here had drifted from the editor: it was
// missing `M-^H`, which kills a word back, and all four numbered spellings of
// Home and End, so `bindkey` reported five keys as unbound in a shell where
// pressing them works. See repl/defaultkeys.go, whose test pins the table
// against the editor's own dispatch.
//
// Not zsh's whole keymap, and not one table per keymap: what this lists is the
// keys this editor reads while a line is being *typed*, which the two `main`
// maps share. The command mode is a second dispatch rather than a second table
// of the same shape — see repl/vi.go — so `vicmd` carries what somebody bound
// into it and nothing else, which is also what KeyBindings reports for it.
var defaultBindings = buildDefaultBindings()

// widgetNames is this shell's canonical name for each action, which is the
// one a listing prints. The reverse of bindkeyWidgets, which cannot be
// inverted automatically because it is many names to one action on purpose:
// `vi-beginning-of-line` reaches the same place and is not what `bindkey`
// says back.
var widgetNames = map[repl.Widget]string{
	repl.WidgetBeginningOfLine:                "beginning-of-line",
	repl.WidgetEndOfLine:                      "end-of-line",
	repl.WidgetBackwardChar:                   "backward-char",
	repl.WidgetForwardChar:                    "forward-char",
	repl.WidgetBackwardWord:                   "backward-word",
	repl.WidgetForwardWord:                    "forward-word",
	repl.WidgetKillLine:                       "kill-line",
	repl.WidgetKillWholeLine:                  "kill-whole-line",
	repl.WidgetBackwardKillLine:               "backward-kill-line",
	repl.WidgetKillWordBefore:                 "backward-kill-word",
	repl.WidgetKillWordAfter:                  "kill-word",
	repl.WidgetYank:                           "yank",
	repl.WidgetTransposeChars:                 "transpose-chars",
	repl.WidgetUpCaseWord:                     "up-case-word",
	repl.WidgetDownCaseWord:                   "down-case-word",
	repl.WidgetCapitalizeWord:                 "capitalize-word",
	repl.WidgetTransposeWords:                 "transpose-words",
	repl.WidgetQuoteLine:                      "quote-line",
	repl.WidgetAcceptAndHold:                  "accept-and-hold",
	repl.WidgetSelfInsert:                     "self-insert",
	repl.WidgetPreviousHistory:                "up-line-or-history",
	repl.WidgetNextHistory:                    "down-line-or-history",
	repl.WidgetPreviousHistoryMatching:        "up-line-or-search",
	repl.WidgetNextHistoryMatching:            "down-line-or-search",
	repl.WidgetSearchHistoryBackward:          "history-incremental-search-backward",
	repl.WidgetClearScreen:                    "clear-screen",
	repl.WidgetDeleteChar:                     "delete-char",
	repl.WidgetBackwardDeleteChar:             "backward-delete-char",
	repl.WidgetComplete:                       "expand-or-complete",
	repl.WidgetListChoices:                    "list-choices",
	repl.WidgetDeleteCharOrList:               "delete-char-or-list",
	repl.WidgetMenuComplete:                   "menu-complete",
	repl.WidgetMenuCompleteBackward:           "reverse-menu-complete",
	repl.WidgetUndo:                           "undo",
	repl.WidgetInsertLastWord:                 "insert-last-word",
	repl.WidgetViCommandMode:                  "vi-cmd-mode",
	repl.WidgetViInsertMode:                   "vi-insert",
	repl.WidgetViAppendMode:                   "vi-add-next",
	repl.WidgetBracketedPaste:                 "bracketed-paste",
	repl.WidgetHistoryBeginningSearchBackward: "history-beginning-search-backward",
	repl.WidgetHistoryBeginningSearchForward:  "history-beginning-search-forward",
	repl.WidgetUpLine:                         "up-line",
	repl.WidgetDownLine:                       "down-line",

	repl.WidgetSearchHistoryForward:         "history-incremental-search-forward",
	repl.WidgetPatternSearchHistoryBackward: "history-incremental-pattern-search-backward",
	repl.WidgetPatternSearchHistoryForward:  "history-incremental-pattern-search-forward",
	repl.WidgetResetPrompt:                  "reset-prompt",
	repl.WidgetSendBreak:                    "send-break",
	repl.WidgetRecursiveEdit:                "recursive-edit",
	repl.WidgetPushLine:                     "push-line",
	repl.WidgetPushLineOrEdit:               "push-line-or-edit",
}

// editorControlKeys are the keys the editor reads that are not actions a key
// can be bound to — accepting a line, and the `^D` that is end-of-input on an
// empty one — under this shell's names for them.
//
// Separate from the derived table because repl has no Widget for most of them
// and deliberately does not: a widget constant with nothing behind it would
// be a name a person could bind and press to no effect. What to call the keys
// is still this shell's question, which is why the answer is here.
//
// **`^D` stays here now that repl.WidgetDeleteCharOrList exists, and that is
// deliberate.** The action and the key are not the same thing: measured
// 2026-09-18 through a pseudo-terminal against zsh 5.9.2, the action bound to
// a key that is not `^D` offers to list on an empty line, while `^D` itself
// ends the session. Turning this into a binding would hand the key to the
// action and take the end of input with it.
var editorControlKeys = map[string]string{
	"\x04": "delete-char-or-list",
	"\x0a": "accept-line",
	"\x0d": "accept-line",
}

// endOfInputKeys are the editorControlKeys whose meaning a redefinition of the
// widget they are named for cannot take away.
//
// One key, and the set exists to say *which* rather than to hold a list: the
// other two are `accept-line`, and there a redefinition taking the key is the
// point. `zle -N accept-line my-accept` is how every plugin wrapper works, and
// repl runs the wrapper and then ends the line for it — the key still accepts,
// through the function somebody wrote. See editor.runShellWidget.
//
// `^D` has no such half. The key *is* end of input, `repl` has no widget for
// it on purpose (see repl.WidgetDeleteCharOrList), and a redefined
// `delete-char-or-list` reached from this key would offer to list rather than
// end anything. So the key is dropped here whenever it is still bound to the
// name it is bound to by default, and reaches the editor's own dispatch.
//
// **This is #4422, and it is the hole the editorControlKeys comment above was
// written to describe without closing.** The drop used to be the `!defined`
// one below, which asks whether the *name* still means what it did. `compinit`
// ends by redefining the eight standard completion widgets —
//
//	for _i_line in complete-word delete-char-or-list expand-or-complete \
//	  expand-or-complete-prefix list-choices menu-complete \
//	  menu-expand-or-complete reverse-menu-complete; do
//	  zle -C $_i_line .$_i_line _main_complete
//	done
//
// — so on any rc that runs `compinit`, which is the ordinary shape and not an
// exotic one, `delete-char-or-list` was defined, the drop was skipped, and
// `\x04` arrived at the editor as an override bound to the completion widget.
// repl's matchBinding claims a key in that table before the key loop's own
// `case ctrlD` is reached, so `editor.stopped` never ran: measured against
// this machine's own `~/.zshrc`, `^D` at an empty prompt answered
//
//	zsh: do you wish to see all 1308 possibilities (437 lines)?
//
// and the session could not be ended from the keyboard at all.
//
// Rebinding the key *away* is still a rebinding and still reaches the editor:
// the drop asks that the key still hold its own default name, so `bindkey '^D'
// beginning-of-line` and `bindkey -r '^D'` both survive it.
var endOfInputKeys = map[string]bool{"\x04": true}

// sendBreakKey is `^G`, send-break in the emacs keymap.
const sendBreakKey = "\x07"

// emacsBindings is what the emacs keymap has beyond defaultBindings, which
// viins shares: `^G`, and the keys repl.EditorStyle.WideEmacsKeymap puts on
// the editor (#5913, #6241). The editor acts on them by itself, so this is
// the listing's half.
var emacsBindings = buildEmacsBindings()

func buildEmacsBindings() map[string]string {
	out := map[string]string{sendBreakKey: widgetNames[repl.WidgetSendBreak]}
	for seq, w := range repl.WideEmacsBindings() {
		out[seq] = widgetNames[w]
	}
	return out
}

func buildDefaultBindings() map[string]string {
	out := map[string]string{}
	for seq, w := range repl.DefaultBindings() {
		if name := widgetNames[w]; name != "" {
			out[seq] = name
		}
	}
	for seq, name := range editorControlKeys {
		out[seq] = name
	}
	// And the two searches' second spellings, which this keymap has and the
	// editor's shared table does not — bash puts spell correction on `^X s`.
	// See repl.EditorStyle.SearchOnControlX, which is what makes the editor
	// act on them (#5904).
	for seq, w := range repl.ControlXSearchBindings() {
		out[seq] = widgetNames[w]
	}
	return out
}

// registerBindkey installs the builtin.
func registerBindkey(r *interp.Runner) {
	r.Register("bindkey", func(r *interp.Runner, ctx context.Context, args []string) int {
		bootLineEditor(r)
		return bindkeyBuiltin(r, ctx, args)
	})
}

// KeyBindings is what a person has rebound in this session, as the editor's
// own vocabulary.
//
// The dialect's answer to driver.Shell.KeyBindings, and it reports only the
// *changes*: a key nobody mentioned is absent, and reaches the editor's own
// dispatch. A key bound to a widget this editor has not got and nothing
// defined is present and bound to nothing, which is what makes it do nothing
// rather than fall through to what it used to do — measured, that is zsh's
// answer too, since it stores the unknown name and the key stops working.
//
// **A widget somebody defined wins over one of the editor's own names.** That
// is what redefining one means, and it is the order `zle -N accept-line
// my-accept` in a real startup file asks for; it is also the only order in
// which a plugin's wrapper around a standard widget can work. See zle.go.
func KeyBindings(r *interp.Runner, km repl.Keymap) map[string]repl.Binding {
	out := map[string]repl.Binding{}
	for seq, widget := range keymapBindings(r, km) {
		def, defined := widgetDefinitionOf(r, widget)
		if km == repl.KeymapMain && endOfInputKeys[seq] && editorControlKeys[seq] == widget {
			// **End of input is the key's and a redefinition cannot take
			// it.** See endOfInputKeys, which carries the whole of why this
			// is asked before the `!defined` question rather than inside it.
			continue
		}
		if km == repl.KeymapMain && !defined {
			// A key left at the editor's own default is left out, so that the
			// table stays the override layer repl/bindings.go describes.
			// There is nothing to compare against in the command map — see
			// keymapBindings.
			//
			// **Only while the name still means what it did.** `compinit`
			// redefines `expand-or-complete` itself — `zle -C
			// expand-or-complete .expand-or-complete _main_complete` — and
			// leaves Tab bound to that same name, so the sequence and the
			// widget name both match the default and the binding is the whole
			// of the completion system. Dropping it here is what made the
			// shipped completions silent on an rc that redefines a standard
			// widget without rebinding a key, which is the ordinary shape
			// rather than an exotic one (#3039).
			if standard, isDefault := defaultBindings[seq]; isDefault && standard == widget {
				continue
			}
			// And the emacs keymap's own keys — `^G` at send-break, and the
			// rest of emacsBindings — which are the editor's own there — see
			// repl.EditorStyle.SendBreakOnControlG and WideEmacsKeymap — and
			// are listed rather than carried in defaultBindings because viins
			// shares that table and has none of them.
			if standard, isDefault := emacsBindings[seq]; isDefault && standard == widget && keymapBase(r, currentKeymap(r)) == "emacs" {
				continue
			}
		}
		if defined {
			// **A completion widget is answered by its completer, not by its
			// function.** See completionBinding, which carries the whole of
			// why — and note that the question is asked of the definition
			// rather than of the name, because the name is whatever the
			// completion loader chose to call it.
			if def.completer != "" {
				out[seq] = completionBinding(widget, def.completer)
				continue
			}
			out[seq] = repl.Binding{Function: widget}
			continue
		}
		out[seq] = repl.Binding{Widget: bindkeyWidgets[widget]}
	}
	return out
}

// completionBinding is what a key bound to a `zle -C` widget does here: the
// editor's own completion, named by the widget's *completer*, with the
// widget's own *function* asked first for the candidates.
//
// Both halves, since #2776. The rest of this comment is #2770, which is the
// first half and the reason the order is the way round it is; what the second
// half added is at the end.
//
// # The failure this exists to stop
//
// `zle -C name completer function` is two claims about one widget: that it
// behaves like the builtin completion widget `completer`, and that `function`
// is what produces the candidates. zle.go says at length what this shell has
// of the second — nothing. A completion widget's function reads the line
// through `$BUFFER` and cannot offer a match, because the whole of how a
// candidate is produced, filtered and shown is `compadd`, `compset` and
// `$compstate`, and this shell has none of them.
//
// That was a statement about a widget somebody invoked, and it was harmless
// while it stayed one. What made it a broken Tab on a real machine is that
// **a real startup file puts such a widget on the Tab key** (#2770). `compinit`
// ends by redefining the eight builtin completion widgets —
//
//	zle -C complete-word .complete-word _main_complete
//
// — and then, when `_expand` is among the configured completers, rebinding
// `^I` to one of them:
//
//	bindkey '^i' complete-word
//
// Both lines run to completion here, so `^I` arrives in this table bound to a
// name that *is* defined, and the editor called `_main_complete`. Measured
// 2026-09-14 through a pseudo-terminal against this machine's own `~/.zshrc`,
// typing `cat uniquef` and pressing Tab in a directory whose only match is
// `uniquefile_marker.txt`: nothing completed and the widget printed
//
//	_main_complete:94: command not found: compset
//	_setup:37: compstate: assignment to invalid subscript range
//
// where the same shell under `-f` completed the name correctly. **A real rc
// took a working completion away and put a failing one in its place**, which
// is worse than having no completion system at all, and is why the issue is a
// daily-driver blocker rather than a missing feature.
//
// # Why the completer is the honest answer
//
// The first of the widget's two claims is one this shell *can* keep. A key
// bound to `complete-word` with no `zle -C` in sight already runs the editor's
// completion — that is bindkeyWidgets, three lines up — and `zle -C` has not
// changed what the key is for. It named a different way of producing the
// candidates, and this shell has one way and only one. So the key goes on
// completing, and the function that cannot complete is not called.
//
// This is the same split complist.go makes for `zsh/complist` — the keymaps,
// which are real here, and not the widget, which is not — and the one
// filesmodule.go makes between `zf_rm` and `rm`. Take the half that is true.
//
// The lookup is bindkeyWidgets, the table every other widget name is answered
// from, with the leading `.` off: `.complete-word` reaches the builtin even
// when something has redefined the plain name, and the two spellings are the
// same action. **All eight completers are in that table since #3043** — the
// two that matter first, since zsh binds Tab to `expand-or-complete` and
// `compinit` rebinds it to `complete-word`, and then the six that answered
// WidgetNone until the editor grew the listing and the menu. A key bound to
// `menu-complete` or `list-choices` did nothing at all before that, in the
// open rather than diagnosing, which was honest and was still a dead key.
//
// # What the second half added, and what is still missing
//
// The function runs now, and what it collects with `compadd` is what the key
// offers — see compsys.go, where the parameters and the two builtins are, and
// repl.Binding.Candidates, which is how the name reaches the editor. **The
// order above is unchanged and is what makes that safe**: the function is
// asked first, and a completion that stops on an error costs a call rather
// than the key. A function that runs and offers nothing is an answer, though,
// and since the shipped completion system runs here it is the common one: the
// key rings and leaves the line, as zsh's does (#6214).
//
// It still does not run the rc's *own* completions, and the reason is no
// longer this file's. `_main_complete` reaches `_git` through `_arguments`,
// and `_arguments` is `comparguments` — one of the eight builtins of
// `zsh/computil`, none of which are here. A completion somebody wrote by hand
// works; the shipped completion system does not yet. See #3039 and #3040.
//
// Nor is it the module table, and that is worth saying because the issue was
// filed as though it were. `zmodload zsh/complete` is refused here, before
// this change and after it, and Tab completes anyway: `compinit` tolerates
// the refusal and binds the widget regardless, so **the loader is not the
// surface that gates the keystroke** — the missing `compset` builtin is the
// one the widget reaches first. Registering the module would have moved the
// diagnostic, not removed it.
func completionBinding(widget, completer string) repl.Binding {
	editorAction := bindkeyWidgets[strings.TrimPrefix(completer, ".")]
	if !editorAction.UsesCandidates() {
		// A completer this editor has not got. The key does nothing, and
		// naming a source of candidates for a completion that will not happen
		// would be a table saying something untrue — repl.Binding.Candidates
		// is empty for every key whose action does not use it.
		//
		// There are none left among the eight since #3043; this is what a
		// name outside the eight resolves to, and it is the honest answer for
		// whatever the next one turns out to be.
		return repl.Binding{Widget: editorAction}
	}
	return repl.Binding{
		Widget: editorAction,
		// And the widget's own name, so that the candidates its *function*
		// produces reach the editor too — which is the half this used to
		// drop. See repl.Binding.Candidates and compsys.go: the editor asks
		// the function first, and what it offers — nothing included — is
		// the key's answer.
		Candidates: widget,
	}
}

// keymapBindings is what the editor is told about one of its two states.
//
// **`vicmd` is the changes and nothing else**, and that is the whole of the
// difference. defaultBindings is what this editor does with a key while a line
// is being *typed*; what it does with a key in command mode is the mode — a
// dispatch rather than a table — and there is no table here that describes it.
// Starting from defaultBindings there would hand the editor an override for
// every key in it, each one either dead or meaning what it means while typing.
// Measured in the shipped binary: Return in command mode stopped accepting the
// line, because `accept-line` is the editor's own control flow and has no
// widget to be overridden with.
//
// A key `bindkey -M vicmd -r` removed is still present and bound to nothing,
// which is how a removal reaches the editor here and is why the store is read
// as it stands.
func keymapBindings(r *interp.Runner, km repl.Keymap) map[string]string {
	if km != repl.KeymapViCommand {
		return readBindings(r, currentKeymap(r))
	}
	out := map[string]string{}
	flat, _ := r.GetArray(bindkeyStore)
	for i := 0; i+3 <= len(flat); i += 3 {
		if flat[i] != "vicmd" {
			continue
		}
		out[flat[i+1]] = flat[i+2]
	}
	return out
}

// ViEditing reports whether this session has a command mode.
//
// **The keymap is the answer and the option is not**, which is measured and
// is the reason repl asks a dialect rather than reading the core's editing
// mode. In zsh 5.9.2 the two move together in one direction only:
//
//	bindkey -v        -> main is viins, and `[[ -o vi ]]` is false
//	setopt vi         -> main is viins, and `[[ -o vi ]]` is true
//	setopt vi; bindkey -e -> main is emacs, and `[[ -o vi ]]` is still true
//
// So an option read here would put the third row in vi mode with the editor
// plainly in emacs. The option reaches the keymap where it should — see
// editingOption in setopt.go, which is the seam #3140 was the absence of —
// and this reads the one piece of state both commands write.
//
// A keymap `bindkey -N` copied from one of the vi keymaps is that keymap as
// far as this question goes: the copy is what a vi-mode plugin makes and
// selects, and this editor's vi editing is a mode rather than a table, so
// the name alone would put it in emacs editing with vi's keys (#5969).
func ViEditing(r *interp.Runner) bool {
	switch keymapBase(r, currentKeymap(r)) {
	case "viins", "vicmd":
		return true
	}
	return false
}

// readBindings is one keymap's table: the defaults with whatever was changed
// laid over them.
func readBindings(r *interp.Runner, keymap string) map[string]string {
	out := map[string]string{}
	if keymap == "emacs" || keymap == "viins" {
		// The defaults are this editor's keys, and only the two keymaps the
		// editor is driven from have them. Every other keymap starts empty
		// here: `bindkey -M vicmd '^A'` is `undefined-key` in zsh 5.9.2,
		// where this answered from the emacs table, and `bindkey -M isearch
		// -L` lists nothing until something is bound (#5691).
		for seq, w := range defaultBindings {
			out[seq] = w
		}
	}
	if keymap == "emacs" {
		// `^G` is send-break in this keymap alone, measured with `bindkey`
		// on zsh 5.9.2; viins has list-expand there, which this shell does
		// not have. The editor acts on it by itself — see
		// repl.EditorStyle.SendBreakOnControlG — so this is the listing's
		// half (#5913). The same for the rest of emacsBindings (#6241).
		for seq, w := range emacsBindings {
			out[seq] = w
		}
	}
	flat, _ := r.GetArray(bindkeyStore)
	for i := 0; i+3 <= len(flat); i += 3 {
		if flat[i] != keymap {
			continue
		}
		out[flat[i+1]] = flat[i+2]
	}
	return out
}

// changeBinding records one change against the current keymap, replacing any
// earlier one for the same sequence.
func changeBinding(r *interp.Runner, seq, widget string) {
	keymap := currentKeymap(r)
	flat, _ := r.GetArray(bindkeyStore)
	for i := 0; i+3 <= len(flat); i += 3 {
		if flat[i] == keymap && flat[i+1] == seq {
			flat[i+2] = widget
			r.SetArray(bindkeyStore, flat)
			return
		}
	}
	r.SetArray(bindkeyStore, append(flat, keymap, seq, widget))
}

// currentKeymap is which keymap `bindkey` acts on without `-M`. `main` is an
// alias for whichever of emacs and viins was last selected, and this holds the
// resolved name.
func currentKeymap(r *interp.Runner) string {
	if m, ok := r.GetVar(bindkeyMap); ok && m != "" {
		return m
	}
	return "emacs"
}

// selectKeymap makes one keymap the current one — what `main` is an alias for
// — which is what `bindkey -v`, `bindkey -e` and the `vi` and `emacs` options
// all do and the only thing they have in common. See editingOption.
func selectKeymap(r *interp.Runner, name string) { r.SetVar(bindkeyMap, name) }

// bindkeyLetters are the option letters this builtin answers to.
const bindkeyLetters = "lLeavrsMNA"

// bindkeyUnimplemented are the letters this shell has that this one does not,
// refused as missing rather than as unknown — the same split `whence` makes,
// so a script can tell a gap from a typo.
const bindkeyUnimplemented = "pRDd"

func bindkeyBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := bindkeyOptions(r, args)
	if code != 0 {
		return code
	}
	if opts.keymap == "main" {
		// `main` is the current keymap under another name, so naming it is
		// naming nothing: `bindkey -M main '^Xq' foo` binds in emacs or
		// viins, whichever is selected, and `-M main -L` writes no `-M`.
		// Measured on zsh 5.9.2, where this stored the binding under a
		// keymap nothing reads (#5691).
		opts.keymap = ""
	}
	switch {
	case opts.newKeymap:
		return bindkeyNew(r, rest)
	case opts.link:
		return bindkeyLink(r, rest)
	case opts.list:
		return listKeymaps(r, rest, opts.commands)
	}
	if opts.selected != "" {
		selectKeymap(r, opts.selected)
		if len(rest) == 0 && !opts.commands && !opts.remove && !opts.strings && opts.keymap == "" {
			// Measured: `bindkey -v` and `bindkey -e` alone print nothing.
			// Selecting a keymap is not a request to see it, where `-M` and
			// `-a` with nothing after them are.
			return 0
		}
	}
	// `-M` and `-a` name the keymap this one command acts on without making it
	// current — measured, `bindkey -a` lists vicmd and leaves `bindkey '^A'`
	// answering from emacs afterwards.
	saved := currentKeymap(r)
	if opts.keymap != "" {
		// The keymap the name names, which is the name itself for a
		// built-in one nobody pointed elsewhere. See keymaps.go.
		id, _ := keymapID(r, opts.keymap)
		r.SetVar(bindkeyMap, id)
		defer r.SetVar(bindkeyMap, saved)
	}
	switch {
	case opts.remove:
		if len(rest) == 0 {
			return bindkeyShort(r, "-r")
		}
		for _, seq := range rest {
			changeBinding(r, decodeKeySequence(seq), undefinedKey)
		}
		return 0
	case opts.strings:
		if len(rest) < 2 {
			return bindkeyShort(r, "-s")
		}
		return bindPairs(r, rest, true)
	case len(rest) == 0:
		listBindings(r, opts.commands, opts.keymap)
		return 0
	case len(rest) == 1:
		showBinding(r, rest[0], opts.commands, opts.keymap)
		return 0
	default:
		return bindPairs(r, rest, false)
	}
}

// listKeymaps answers `bindkey -l`: the keymaps this shell has, or the ones
// named, and with `-L` the commands that would make each.
//
// **`-lL` is the one place a script can read which keymap is current.** `main`
// is an alias rather than a keymap, so the command that would recreate it is
// `bindkey -A <target> main` with the target written out — which is exactly
// the question `setopt vi` moves and the only scriptable answer to it. There
// is no `$KEYMAP` outside a widget and no other listing that says it, so
// without this the option's effect was observable only through a terminal,
// which is how it stayed broken (#3140).
//
// Measured on zsh 5.9.2: `.safe` is listed by `-l` and skipped by `-lL`,
// because it is built into the shell and no `bindkey` command would make it.
// A name no keymap has is `no such keymap` and status 1 under both, and the
// order is keymapsNow's — names given as operands are printed in the order
// they were written, which is also measured (`bindkey -lL emacs main`).
func listKeymaps(r *interp.Runner, names []string, commands bool) int {
	if len(names) == 0 {
		names = keymapsNow(r)
	} else {
		have := keymapsNow(r)
		for _, name := range names {
			if !containsWord(have, name) {
				// **With a colon, where `-M` says the same thing without
				// one.** Measured on zsh 5.9.2, both status 1: `bindkey -l
				// nosuch` is "no such keymap: `nosuch'" and `bindkey -M
				// nosuch ...` is "no such keymap `nosuch'". One shell, one
				// condition, two sentences — so the two routes cannot share
				// a wording however much they want to.
				r.Diagnosef("no such keymap: `%s'\n", name)
				return 1
			}
		}
	}
	for _, name := range names {
		switch {
		case !commands:
			_, _ = fmt.Fprintf(r.Out(), "%s\n", name)
		case name == ".safe":
		default:
			// A keymap is listed under the name it was made with, and every
			// other name for it as an alias of that one — `main` included,
			// which is how this listing says which keymap is selected. See
			// keymaps.go for why the made-with name and not a current one.
			id, _ := keymapID(r, name)
			if primary := keymapPrimary(r, id); primary != name {
				_, _ = fmt.Fprintf(r.Out(), "bindkey -A %s %s\n", primary, name)
				continue
			}
			_, _ = fmt.Fprintf(r.Out(), "bindkey -N %s\n", name)
		}
	}
	return 0
}

// bindkeyOpts is what the letters asked for.
type bindkeyOpts struct {
	list     bool   // -l: name the keymaps
	commands bool   // -L: list as the commands that would set it
	remove   bool   // -r: unbind
	strings  bool   // -s: bind to text rather than to a widget
	keymap   string // -M or -a: the keymap this command acts on
	selected string // -e or -v: the keymap to make current

	newKeymap bool // -N: make a keymap
	link      bool // -A: give a keymap another name
}

// bindkeyOptions reads the leading option words.
//
// An unknown letter is `bad option: -q` and 1 — this builtin's wording, and
// not `zstyle`'s `invalid option`, measured in both. Nothing is done after it.
func bindkeyOptions(r *interp.Runner, args []string) (opts bindkeyOpts, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			letter := word[i]
			switch {
			case letter == 'M':
				// The keymap is the next word, whether it is joined on or not.
				name := word[i+1:]
				if name == "" {
					if len(rest) == 0 {
						return opts, nil, bindkeyShort(r, "-M")
					}
					name, rest = rest[0], rest[1:]
				}
				if !containsWord(keymapsNow(r), name) {
					r.Diagnosef("no such keymap `%s'\n", name)
					return opts, nil, 1
				}
				opts.keymap = name
				i = len(word)
			case strings.IndexByte(bindkeyLetters, letter) >= 0:
				setBindkeyLetter(&opts, letter)
			case strings.IndexByte(bindkeyUnimplemented, letter) >= 0:
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return opts, nil, 1
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

func setBindkeyLetter(opts *bindkeyOpts, letter byte) {
	switch letter {
	case 'l':
		opts.list = true
	case 'L':
		opts.commands = true
	case 'r':
		opts.remove = true
	case 's':
		opts.strings = true
	case 'N':
		opts.newKeymap = true
	case 'A':
		opts.link = true
	case 'a':
		opts.keymap = "vicmd"
	case 'e':
		opts.selected = "emacs"
	case 'v':
		opts.selected = "viins"
	}
}

// bindkeyShort is the usage complaint, which names the letter that was short —
// `not enough arguments for -r`, measured, where `zstyle` names nothing.
func bindkeyShort(r *interp.Runner, flag string) int {
	r.Diagnosef("not enough arguments for %s\n", flag)
	return 1
}

// bindPairs binds each sequence-and-target pair. An odd number of words is
// tolerated the way this shell tolerates it: the last one alone is ignored.
func bindPairs(r *interp.Runner, words []string, text bool) int {
	for i := 0; i+1 < len(words); i += 2 {
		target := words[i+1]
		if text {
			// A string binding is kept as the text it types, read through the
			// same notation as the key — `bindkey -s '^Xb' '\x41'` types `A`
			// in zsh 5.9.2, where this kept the four characters (#5672) — and
			// quoted the way the listing prints it, which is how `bindkey
			// '^X'` tells the two kinds apart when it says one back.
			target = quoteKeyString(decodeKeySequence(target))
		}
		changeBinding(r, decodeKeySequence(words[i]), target)
	}
	return 0
}

// listBindings is the whole keymap, sorted by the bytes each key sends —
// measured, which is why `^_` comes before a space and `^?` after a tilde.
func listBindings(r *interp.Runner, commands bool, named string) {
	table := readBindings(r, currentKeymap(r))
	seqs := make([]string, 0, len(table))
	for seq := range table {
		if table[seq] != undefinedKey {
			seqs = append(seqs, seq)
		}
	}
	sort.Strings(seqs)
	for _, seq := range seqs {
		writeBinding(r, seq, table[seq], commands, named)
	}
}

// showBinding answers for one key. A key nobody bound is `undefined-key` and
// still status 0 — measured; asking about an unbound key is a question with an
// answer, not a failure.
func showBinding(r *interp.Runner, spelled string, commands bool, named string) {
	seq := decodeKeySequence(spelled)
	widget, bound := readBindings(r, currentKeymap(r))[seq]
	if !bound {
		widget = undefinedKey
	}
	writeBinding(r, seq, widget, commands, named)
}

// writeBinding says one binding back. named is the keymap `-M` or `-a` named
// on the command, empty for the current one.
func writeBinding(r *interp.Runner, seq, widget string, commands bool, named string) {
	prefix := ""
	if commands {
		prefix = "bindkey "
		if strings.HasPrefix(widget, `"`) {
			// A string binding is recreated with `-s`: without it the line
			// read back binds the key to a *widget* of that name. Measured,
			// `bindkey -s '^Xf' plain; bindkey -L` is `bindkey -s "^Xf"
			// "plain"` in zsh 5.9.2 (#5672).
			prefix += "-s "
		}
		// And the keymap the listing was asked about, so the line binds the
		// key there again: `bindkey -M emacs -L` writes `bindkey -M emacs`,
		// and vicmd is spelled `-a`, after any `-s`. Measured on zsh 5.9.2,
		// where this wrote every line as if for the current keymap (#5691).
		switch named {
		case "":
		case "vicmd":
			prefix += "-a "
		default:
			prefix += "-M " + named + " "
		}
		// A key that begins with a dash is written after `--`, so the line
		// read back does not take it for an option — except after `-M`
		// and its keymap, where zsh 5.9.2 writes none: `bindkey -L -- -x`
		// is `bindkey -- "-x" foo`, and with `-M emacs` it is `bindkey -M
		// emacs "-x" foo` (#5691).
		if strings.HasPrefix(seq, "-") && (named == "" || named == "vicmd") {
			prefix += "-- "
		}
	}
	_, _ = fmt.Fprintf(r.Out(), "%s\"%s\" %s\n", prefix, encodeKeySequence(seq), widget)
}

// decodeKeySequence reads the notation a key sequence is written in.
//
// Measured by binding each form and reading it back: `\C-x` and `^X` are the
// same byte, `^ ` is NUL because the caret clears the top three bits of
// whatever follows, `\e` and `\E` are escape, `\t` and the rest are C's
// escapes, `\x41` is hexadecimal and `\100` is octal, and `\M-q` sets the top
// bit rather than prefixing an escape — which is why it prints back as `\M-q`
// and not as `^[q`.
func decodeKeySequence(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == '^' && i+1 < len(s):
			out.WriteByte(caretByte(s[i+1]))
			i += 2
		case c == '\\' && i+1 < len(s):
			b, width := decodeKeyEscape(s[i+1:])
			out.WriteByte(b)
			i += 1 + width
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// caretByte is the byte a caret and one character name. The caret clears the
// top three bits, which is why `^ ` is NUL — with the one exception both
// spellings of it agree on: `^?` and `\C-?` are DEL, not the 0x1f that
// clearing the bits of `?` would give.
func caretByte(c byte) byte {
	if c == '?' {
		return 0x7f
	}
	return c & 0x1f
}

// decodeKeyEscape reads one backslash escape, having been given what follows
// the backslash, and answers with the byte and how much of the text it used.
func decodeKeyEscape(s string) (byte, int) {
	switch c := s[0]; c {
	case 'a':
		return 0x07, 1
	case 'b':
		return 0x08, 1
	case 'e', 'E':
		return 0x1b, 1
	case 'f':
		return 0x0c, 1
	case 'n':
		return 0x0a, 1
	case 'r':
		return 0x0d, 1
	case 't':
		return 0x09, 1
	case 'v':
		return 0x0b, 1
	case 'C':
		if len(s) > 2 && s[1] == '-' {
			return caretByte(s[2]), 3
		}
		return c, 1
	case 'M':
		// `\M-x` and `\Mx` alike, measured: the dash is optional.
		if len(s) > 2 && s[1] == '-' {
			return s[2] | 0x80, 3
		}
		if len(s) > 1 {
			return s[1] | 0x80, 2
		}
		return c, 1
	case 'x', 'X':
		return decodeKeyNumber(s[1:], 16, 2)
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return decodeKeyNumber(s, 8, 3)
	default:
		return c, 1
	}
}

// decodeKeyNumber reads up to most digits in the given base and answers with
// the byte and how much of the text it used, the escape letter included where
// there was one.
func decodeKeyNumber(s string, base, most int) (byte, int) {
	used := 0
	for used < most && used < len(s) && isDigitInBase(s[used], base) {
		used++
	}
	if used == 0 {
		return s[0], 1
	}
	n, err := strconv.ParseUint(s[:used], base, 16)
	if err != nil {
		return s[0], 1
	}
	width := used
	if base == 16 {
		// The `x` itself, which the octal form does not have.
		width++
	}
	return byte(n), width
}

func isDigitInBase(c byte, base int) bool {
	switch {
	case c >= '0' && c <= '7':
		return true
	case base == 8:
		return false
	case c == '8' || c == '9':
		return true
	case base == 16:
		return (c|0x20) >= 'a' && (c|0x20) <= 'f'
	default:
		return false
	}
}

// encodeKeySequence writes a sequence back the way this shell prints one,
// which is two layers: the key notation, and the double quotes the listing
// puts it in, so that `bindkey -L` read back as shell code binds the same
// bytes.
//
// The notation writes control characters as a caret, the high half as `\M-`,
// and a backslash and a caret escaped as `\\` and `\^`, so that neither reads
// as the start of a spelling. The quoting then escapes `"`, `$` and the
// backquote, and a backslash only where the double quotes would otherwise
// take it: before another of those four characters, or before the closing
// quote. Measured 2026-10-03 on zsh 5.9.2, `bindkey "$v" foo; bindkey -L
// "$v"`, as raw bytes (#5672):
//
//	\\ or \     "\\\\"     one backslash: notation \\, each one escaped
//	\\a       "\\\a"      backslash then a: notation \\a, the second kept
//	^         "\^"
//	\\^       "\\\\\^"    backslash then caret: notation \\\^
//	a"b       "a\"b"
//	\777      "\M-^?"
func encodeKeySequence(s string) string {
	var notation strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			notation.WriteString(`\M-`)
			c &= 0x7f
		}
		switch {
		case c == 0x7f:
			notation.WriteString("^?")
		case c < 0x20:
			notation.WriteByte('^')
			notation.WriteByte(c + '@')
		case c == '\\' || c == '^':
			notation.WriteByte('\\')
			notation.WriteByte(c)
		default:
			notation.WriteByte(c)
		}
	}
	n := notation.String()
	var out strings.Builder
	for i := 0; i < len(n); i++ {
		c := n[i]
		switch c {
		case '"', '$', '`':
			out.WriteByte('\\')
		case '\\':
			if i+1 == len(n) || strings.IndexByte(`"$\`+"`", n[i+1]) >= 0 {
				out.WriteByte('\\')
			}
		}
		out.WriteByte(c)
	}
	return out.String()
}

// quoteKeyString spells the text a `-s` binding types, which the listing shows
// in quotes of its own where a widget name would go.
func quoteKeyString(s string) string {
	return `"` + encodeKeySequence(s) + `"`
}
