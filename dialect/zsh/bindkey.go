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
//     do. The same rule `whence -m` follows: an answer that cannot be
//     generated is refused rather than invented.
//   - **`-p`, `-R`, `-N`, `-A`, `-D` and `-d`** — prefix bindings, ranges of
//     keys, and making, aliasing or destroying a keymap — are refused as not
//     implemented, in the wording `whence` uses for the same case, so a script
//     can tell a shell that lacks something from a typo.

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
	"backward-kill-line":    repl.WidgetKillWholeLine,
	"backward-kill-word":    repl.WidgetKillWordBefore,
	"vi-backward-kill-word": repl.WidgetKillWordBefore,
	"kill-word":             repl.WidgetKillWordAfter,
	"yank":                  repl.WidgetYank,
	"transpose-chars":       repl.WidgetTransposeChars,
	"up-line-or-history":    repl.WidgetPreviousHistory,
	"up-history":            repl.WidgetPreviousHistory,
	"down-line-or-history":  repl.WidgetNextHistory,
	"down-history":          repl.WidgetNextHistory,
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
	"undo":                                repl.WidgetUndo,
	"vi-undo-change":                      repl.WidgetUndo,
	"insert-last-word":                    repl.WidgetInsertLastWord,

	// The three that move between insert and command mode, which are the only
	// vi-only actions repl names — see repl/widgets.go, which carries the
	// decision and why the motions are not among them. This shell's names.
	"vi-cmd-mode":   repl.WidgetViCommandMode,
	"vi-insert":     repl.WidgetViInsertMode,
	"vi-insert-bol": repl.WidgetViInsertMode,
	"vi-add-next":   repl.WidgetViAppendMode,
	// The key that means "this key does nothing", which is what `-r` leaves
	// behind and what `bindkey` prints for a key nobody bound.
	undefinedKey: repl.WidgetNone,
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
	repl.WidgetBeginningOfLine:         "beginning-of-line",
	repl.WidgetEndOfLine:               "end-of-line",
	repl.WidgetBackwardChar:            "backward-char",
	repl.WidgetForwardChar:             "forward-char",
	repl.WidgetBackwardWord:            "backward-word",
	repl.WidgetForwardWord:             "forward-word",
	repl.WidgetKillLine:                "kill-line",
	repl.WidgetKillWholeLine:           "kill-whole-line",
	repl.WidgetKillWordBefore:          "backward-kill-word",
	repl.WidgetKillWordAfter:           "kill-word",
	repl.WidgetYank:                    "yank",
	repl.WidgetTransposeChars:          "transpose-chars",
	repl.WidgetPreviousHistory:         "up-line-or-history",
	repl.WidgetNextHistory:             "down-line-or-history",
	repl.WidgetPreviousHistoryMatching: "up-line-or-search",
	repl.WidgetNextHistoryMatching:     "down-line-or-search",
	repl.WidgetSearchHistoryBackward:   "history-incremental-search-backward",
	repl.WidgetClearScreen:             "clear-screen",
	repl.WidgetDeleteChar:              "delete-char",
	repl.WidgetBackwardDeleteChar:      "backward-delete-char",
	repl.WidgetComplete:                "expand-or-complete",
	repl.WidgetUndo:                    "undo",
	repl.WidgetInsertLastWord:          "insert-last-word",
	repl.WidgetViCommandMode:           "vi-cmd-mode",
	repl.WidgetViInsertMode:            "vi-insert",
	repl.WidgetViAppendMode:            "vi-add-next",
}

// editorControlKeys are the keys the editor reads that are not actions a key
// can be bound to — accepting a line, and the `^D` that is end-of-input on an
// empty one — under this shell's names for them.
//
// Separate from the derived table because repl has no Widget for any of them
// and deliberately does not: a widget constant with nothing behind it would
// be a name a person could bind and press to no effect. What to call the keys
// is still this shell's question, which is why the answer is here.
var editorControlKeys = map[string]string{
	"\x04": "delete-char-or-list",
	"\x0a": "accept-line",
	"\x0d": "accept-line",
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
	return out
}

// registerBindkey installs the builtin.
func registerBindkey(r *interp.Runner) {
	r.Register("bindkey", bindkeyBuiltin)
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
		if km == repl.KeymapMain {
			// A key left at the editor's own default is left out, so that the
			// table stays the override layer repl/bindings.go describes.
			// There is nothing to compare against in the command map — see
			// keymapBindings.
			if def, standard := defaultBindings[seq]; standard && def == widget {
				continue
			}
		}
		if _, defined := widgetDefinitionOf(r, widget); defined {
			out[seq] = repl.Binding{Function: widget}
			continue
		}
		out[seq] = repl.Binding{Widget: bindkeyWidgets[widget]}
	}
	return out
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
// **Two commands ask for it and only one of them is the option**, which is
// measured and is the reason repl asks a dialect rather than reading the
// core's editing mode: in zsh 5.9.2 under a pty, `bindkey -v` gives a working
// command mode and leaves `set -o` reporting both `emacs off` and `vi off`,
// while `set -o vi` gives the same command mode and reports `vi on`. So either
// is enough, and neither can be read off the other.
func ViEditing(r *interp.Runner) bool {
	switch currentKeymap(r) {
	case "viins", "vicmd":
		return true
	}
	return r.EditingMode() == interp.EditingModeVi
}

// readBindings is one keymap's table: the defaults with whatever was changed
// laid over them.
func readBindings(r *interp.Runner, keymap string) map[string]string {
	out := map[string]string{}
	for seq, w := range defaultBindings {
		out[seq] = w
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

// bindkeyLetters are the option letters this builtin answers to.
const bindkeyLetters = "lLeavrsM"

// bindkeyUnimplemented are the letters this shell has that this one does not,
// refused as missing rather than as unknown — the same split `whence` makes,
// so a script can tell a gap from a typo.
const bindkeyUnimplemented = "pRNADd"

func bindkeyBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := bindkeyOptions(r, args)
	if code != 0 {
		return code
	}
	if opts.list {
		for _, name := range keymapsNow(r) {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", name)
		}
		return 0
	}
	if opts.selected != "" {
		r.SetVar(bindkeyMap, opts.selected)
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
		r.SetVar(bindkeyMap, opts.keymap)
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
		listBindings(r, opts.commands)
		return 0
	case len(rest) == 1:
		showBinding(r, rest[0], opts.commands)
		return 0
	default:
		return bindPairs(r, rest, false)
	}
}

// bindkeyOpts is what the letters asked for.
type bindkeyOpts struct {
	list     bool   // -l: name the keymaps
	commands bool   // -L: list as the commands that would set it
	remove   bool   // -r: unbind
	strings  bool   // -s: bind to text rather than to a widget
	keymap   string // -M or -a: the keymap this command acts on
	selected string // -e or -v: the keymap to make current
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
			// A string binding is kept as the text it types, quoted the way
			// the listing prints it, which is how `bindkey '^X'` tells the two
			// kinds apart when it says one back.
			target = quoteKeyString(target)
		}
		changeBinding(r, decodeKeySequence(words[i]), target)
	}
	return 0
}

// listBindings is the whole keymap, sorted by the bytes each key sends —
// measured, which is why `^_` comes before a space and `^?` after a tilde.
func listBindings(r *interp.Runner, commands bool) {
	table := readBindings(r, currentKeymap(r))
	seqs := make([]string, 0, len(table))
	for seq := range table {
		if table[seq] != undefinedKey {
			seqs = append(seqs, seq)
		}
	}
	sort.Strings(seqs)
	for _, seq := range seqs {
		writeBinding(r, seq, table[seq], commands)
	}
}

// showBinding answers for one key. A key nobody bound is `undefined-key` and
// still status 0 — measured; asking about an unbound key is a question with an
// answer, not a failure.
func showBinding(r *interp.Runner, spelled string, commands bool) {
	seq := decodeKeySequence(spelled)
	widget, bound := readBindings(r, currentKeymap(r))[seq]
	if !bound {
		widget = undefinedKey
	}
	writeBinding(r, seq, widget, commands)
}

func writeBinding(r *interp.Runner, seq, widget string, commands bool) {
	prefix := ""
	if commands {
		prefix = "bindkey "
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

// encodeKeySequence writes a sequence back the way this shell prints one:
// control characters as a caret, the high half as `\M-`, and the four
// characters that would end or reopen the double quotes escaped.
func encodeKeySequence(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			out.WriteString(`\M-`)
			c &= 0x7f
		}
		switch {
		case c == 0x7f:
			out.WriteString("^?")
		case c < 0x20:
			out.WriteByte('^')
			out.WriteByte(c + '@')
		case c == '"' || c == '\\' || c == '$' || c == '`':
			out.WriteByte('\\')
			out.WriteByte(c)
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// quoteKeyString spells the text a `-s` binding types, which the listing shows
// in quotes of its own where a widget name would go.
func quoteKeyString(s string) string {
	return `"` + encodeKeySequence(s) + `"`
}
