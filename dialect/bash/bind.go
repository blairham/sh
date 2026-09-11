// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `bind` maps a key sequence onto an editor function, in readline's
// vocabulary.
//
// The naming half of the split repl/widgets.go describes: the editor performs
// the actions and this says what this shell calls them. It is the counterpart
// of the zsh dialect's `bindkey`, and the two files are the same shape on
// purpose — what differs is the vocabulary, which is the whole reason the
// mapping is in a dialect. The keys themselves are neither dialect's: both
// derive their default listing from repl.DefaultBindings, so a key the editor
// acts on cannot be reported as unbound by one shell and bound by the other.
//
// Measured 2026-09-07 against bash 5.3.15, under a pseudo-terminal so that
// line editing is enabled — `bind` in a shell without it says `warning: line
// editing not enabled` and does nothing, so every answer below was taken with
// a terminal on the other end.
//
// **The finding that shaped this is that the two argument forms are not the
// same form.** `bind 'keyseq:function'` reads the left side as a *key name* —
// one key, spelled `\C-l` or `Control-l` or `DEL` or `x` — and
// `bind '"keyseq": function'`, with double quotes inside the word, reads it as
// a whole escape sequence. Measured, and the difference is silent:
//
//	bind "\C-l":clear-screen         binds ^L                 (a key name)
//	bind "\C-x\C-a":beginning-of-line  binds nothing, status 0  (not a key name)
//	bind '"\C-x\C-a": beginning-of-line'  binds ^X^A
//	bind "\e[Z":beginning-of-line    binds a backslash, status 0
//
// So a shell that accepted the second line as a two-byte sequence would be
// binding a key real bash leaves alone. Both forms are read here, and the
// first is a single key.
//
// **An unknown function name is not an error and is not stored.** `bind '"^X^T":
// no-such-widget'` is status 0 and silence, and the key goes on doing what it
// did — `bind -p` has no row for it. That is the opposite of what the other
// shell with an editor does, which keeps the unknown name so the key stops
// working, and both were measured the same way. It is also what a real rc file
// needs from this one: a plugin's `bind` for a function this shell has not got
// leaves the keyboard exactly as it was.
//
// What is deliberately not built, and why each is a refusal rather than a
// guess:
//
//   - **`-l` names this editor's functions, not readline's 176.** Answering
//     `vi-match` because bash does would be naming something nothing here
//     performs, and `bind -q` is a question a person asks to find out what a
//     key does. The same rule `bindkey` follows on the other side, and the
//     same rule `whence -m` follows: an answer that cannot be generated is
//     refused rather than invented.
//   - **`-v`, `-V` and `-f`** — readline's variables and reading an inputrc —
//     are refused as not implemented. There is no readline here to hold a
//     variable, so printing `bind-tty-special-chars is set to `on'` would be
//     reporting a setting nothing reads.
//   - **`-m emacs-meta` and `-m emacs-ctlx`** are refused as not implemented
//     rather than as invalid, because bash really has them: they are
//     readline's *prefix* keymaps, and this editor reads a key sequence whole
//     rather than through a prefix map (see repl/escape.go), so a binding
//     recorded in one could never fire. That is the same refusal `bindkey -p`
//     makes on the other side, for the same reason.
//
// **`-x` and `-X` are built**, and the file beside this one holds them:
// bindx.go is what running a shell command from a key *means* here, measured
// through a pseudo-terminal against bash 5.3.15. This file is still only the
// naming half — which key runs what — and the two kinds of target share one
// slot per key, which is measured rather than assumed.
//
// What `-m vi-command` gets is acceptance, not refusal, and the distinction
// matters: it is a keymap this shell plainly has and `set -o vi` selects
// `vi-insert`, so a binding in the command map is stored and simply never
// current — because this editor has no command mode. The zsh dialect's
// `vicmd` is the same documented partial, and closing it would serve both at
// once, which is why it is filed on its own rather than folded in here.

// bindStore is the table of what a person rebound, in the Runner's variables
// under a name no script can reach — the way `shopt` keeps its own state, and
// what gives a subshell its own copy.
const bindStore = ".bash.bind"

// bindRecord is how many slots of the store one binding takes: the keymap, the
// key sequence, what it is bound to, and which *kind* of thing that is.
//
// The fourth slot is what `bind -x` needed. The first three could tell a
// function from a macro by looking — a macro is stored under the quotes the
// listing prints it in, and no readline function name has any — and a shell
// command can be spelled either way, so what a target *is* has to be recorded
// rather than inferred. Inferring it would have made `bind -x '"\C-t": f'` and
// `bind '"\C-t": f'` the same binding, which they are not: one runs a command
// and the other names an editor action, and only the second is listed by
// `bind -p`.
const bindRecord = 4

// bindKindCommand marks a target that is shell command text, put on the key by
// `bind -x`. The empty kind is a function name or a macro.
const bindKindCommand = "x"

// bindEntry is what one key is bound to: the target, and whether the target is
// a shell command rather than the name of an editor action or a macro's text.
type bindEntry struct {
	target  string
	command bool
}

// bindKeymaps are the keymaps this shell has, mapped to the one this
// implementation keeps a table for.
//
// Three tables under seven names, measured: `bind -m vi-command '"^X^Q":
// clear-screen'` is visible from `bind -m vi` and from `bind -m vi-move` and
// not from `bind -m vi-insert`, and `-m emacs-standard` and `-m emacs` are one
// table too. An eighth and ninth name exist and are not here — see
// bindPrefixKeymaps.
var bindKeymaps = map[string]string{
	"emacs":          "emacs",
	"emacs-standard": "emacs",
	"vi":             "vi-command",
	"vi-command":     "vi-command",
	"vi-move":        "vi-command",
	"vi-insert":      "vi-insert",
}

// bindPrefixKeymaps are readline's two prefix maps: the one reached by ESC and
// the one reached by `^X`. Named so the refusal can be "not implemented"
// rather than "invalid", which is a different and worse answer for a keymap
// bash has.
var bindPrefixKeymaps = map[string]bool{"emacs-meta": true, "emacs-ctlx": true}

// bindFunctions is this shell's names for the things the editor does.
//
// readline's vocabulary, and it overlaps the other shell's without being it:
// the key that walks history back is `previous-history` here and
// `up-line-or-history` there, which is the reason each dialect holds its own
// map rather than the substrate holding one.
//
// Several actions answer to more than one name, and each of those is a
// measured synonym rather than a convenience: `insert-last-argument` and
// `yank-last-arg` are on the same keys in bash 5.3, and so are
// `unix-line-discard` and `backward-kill-line`.
//
// **`kill-whole-line` is deliberately absent.** This editor's kill on `^U`
// takes the line back to its start and leaves what is ahead of the cursor,
// which is readline's `unix-line-discard`; readline's `kill-whole-line` takes
// the whole line including that, and is bound to no key in bash 5.3. Naming it
// here would be offering a name that does something else.
var bindFunctions = map[string]repl.Widget{
	"beginning-of-line":      repl.WidgetBeginningOfLine,
	"end-of-line":            repl.WidgetEndOfLine,
	"backward-char":          repl.WidgetBackwardChar,
	"forward-char":           repl.WidgetForwardChar,
	"backward-word":          repl.WidgetBackwardWord,
	"forward-word":           repl.WidgetForwardWord,
	"kill-line":              repl.WidgetKillLine,
	"unix-line-discard":      repl.WidgetKillWholeLine,
	"backward-kill-line":     repl.WidgetKillWholeLine,
	"backward-kill-word":     repl.WidgetKillWordBefore,
	"kill-word":              repl.WidgetKillWordAfter,
	"yank":                   repl.WidgetYank,
	"transpose-chars":        repl.WidgetTransposeChars,
	"previous-history":       repl.WidgetPreviousHistory,
	"next-history":           repl.WidgetNextHistory,
	"reverse-search-history": repl.WidgetSearchHistoryBackward,
	"clear-screen":           repl.WidgetClearScreen,
	"delete-char":            repl.WidgetDeleteChar,
	"backward-delete-char":   repl.WidgetBackwardDeleteChar,
	"vi-rubout":              repl.WidgetBackwardDeleteChar,
	"complete":               repl.WidgetComplete,
	"undo":                   repl.WidgetUndo,
	"vi-undo":                repl.WidgetUndo,
	"yank-last-arg":          repl.WidgetInsertLastWord,
	"insert-last-argument":   repl.WidgetInsertLastWord,
	"vi-yank-arg":            repl.WidgetInsertLastWord,
}

// bindFunctionNames is the name a listing prints for each action — the
// canonical one where bindFunctions holds several, which is the name bash's
// own default keymap uses for the key.
var bindFunctionNames = map[repl.Widget]string{
	repl.WidgetBeginningOfLine:       "beginning-of-line",
	repl.WidgetEndOfLine:             "end-of-line",
	repl.WidgetBackwardChar:          "backward-char",
	repl.WidgetForwardChar:           "forward-char",
	repl.WidgetBackwardWord:          "backward-word",
	repl.WidgetForwardWord:           "forward-word",
	repl.WidgetKillLine:              "kill-line",
	repl.WidgetKillWholeLine:         "unix-line-discard",
	repl.WidgetKillWordBefore:        "backward-kill-word",
	repl.WidgetKillWordAfter:         "kill-word",
	repl.WidgetYank:                  "yank",
	repl.WidgetTransposeChars:        "transpose-chars",
	repl.WidgetPreviousHistory:       "previous-history",
	repl.WidgetNextHistory:           "next-history",
	repl.WidgetSearchHistoryBackward: "reverse-search-history",
	repl.WidgetClearScreen:           "clear-screen",
	repl.WidgetDeleteChar:            "delete-char",
	repl.WidgetBackwardDeleteChar:    "backward-delete-char",
	repl.WidgetComplete:              "complete",
	repl.WidgetUndo:                  "undo",
	repl.WidgetInsertLastWord:        "yank-last-arg",
}

// editorControlKeys are the keys the editor reads that are not actions a key
// can be bound to — accepting a line, and the `^D` that ends input on an empty
// one — under this shell's names for them.
//
// repl has no Widget for any of them and deliberately does not: a widget with
// nothing behind it would be a name a person could bind and press to no
// effect. What to call the keys is still this shell's question, and the two
// dialects answer it differently — `delete-char` on `^D` here, where the other
// says `delete-char-or-list`.
//
// `delete-char` on `^D` is also measured: `bind -q delete-char` in bash 5.3
// answers `"\C-d", "\e[3~"`, so the name really covers both keys.
var editorControlKeys = map[string]string{
	"\x04": "delete-char",
	"\x0a": "accept-line",
	"\x0d": "accept-line",
}

// defaultBindings is what each keymap does before anyone changes it: the keys
// this editor acts on, under this shell's names for them.
//
// Derived from repl.DefaultBindings rather than written out, because the keys
// are the editor's and only the names are this shell's. See
// repl/defaultkeys.go, whose test pins that table against the editor's own
// dispatch — which is what keeps this listing from claiming a key the editor
// ignores, or missing one it acts on.
//
// Not bash's whole keymap, and not one table per keymap. All three keymaps
// start from this, because what differs between readline's emacs and vi maps
// is a command mode this editor has not got; see the file comment.
var defaultBindings = buildDefaultBindings()

func buildDefaultBindings() map[string]string {
	out := map[string]string{}
	for seq, w := range repl.DefaultBindings() {
		if name := bindFunctionNames[w]; name != "" {
			out[seq] = name
		}
	}
	for seq, name := range editorControlKeys {
		out[seq] = name
	}
	return out
}

// registerBind installs the builtin.
func registerBind(r *interp.Runner) {
	r.Register("bind", bindBuiltin)
}

// KeyBindings is what a person has rebound in this session, as the editor's
// own vocabulary.
//
// The dialect's answer to driver.Shell.KeyBindings, and it reports only the
// *changes*: a key nobody mentioned is absent, and reaches the editor's own
// dispatch.
//
// Only the current keymap's changes, which is what makes `set -o vi` mean
// something here — a binding written with `-m vi-insert` is live once vi mode
// selects that map, and inert until then. Measured: `set -o vi` makes
// `vi-insert` the map `bind` answers from.
//
// A key bound to a macro rather than to a function is present and bound to
// nothing, so it does nothing rather than going on doing what it did. Typing
// text from a key is not something this editor can be asked to do, and the
// listing says so plainly by showing the text back — see bindMacro.
func KeyBindings(r *interp.Runner) map[string]repl.Binding {
	out := map[string]repl.Binding{}
	for seq, bound := range readBindings(r, currentKeymap(r)) {
		if bound.command {
			// A key `bind -x` put a shell command on. The command text rides
			// Function, which repl does not look inside — see
			// repl/shellwidget.go, and RunWidget in bindx.go for this shell's
			// half of the round trip.
			out[seq] = repl.Binding{Function: bound.target}
			continue
		}
		if def, standard := defaultBindings[seq]; standard && def == bound.target {
			continue
		}
		out[seq] = repl.Binding{Widget: bindFunctions[bound.target]}
	}
	return out
}

// readBindings is one keymap's table: the defaults with whatever was changed
// laid over them. A key removed with `-r` is absent rather than present and
// empty, which is what `bind -p` leaves out and what `-q` reports as unbound.
func readBindings(r *interp.Runner, keymap string) map[string]bindEntry {
	out := map[string]bindEntry{}
	for seq, name := range defaultBindings {
		out[seq] = bindEntry{target: name}
	}
	flat, _ := r.GetArray(bindStore)
	for i := 0; i+bindRecord <= len(flat); i += bindRecord {
		if flat[i] != keymap {
			continue
		}
		seq, target, kind := flat[i+1], flat[i+2], flat[i+3]
		if target == "" && kind == "" {
			delete(out, seq)
			continue
		}
		out[seq] = bindEntry{target: target, command: kind == bindKindCommand}
	}
	return out
}

// changeBinding records one change against a keymap, replacing any earlier one
// for the same sequence. The zero entry is a removal.
//
// One slot per key, which is measured rather than assumed: `bind -x` on a key
// and then an ordinary `bind` on the same key leaves `bind -X` empty and
// `bind -p` showing the function, so the two kinds of binding share a slot
// rather than living in tables of their own.
func changeBinding(r *interp.Runner, keymap, seq string, bound bindEntry) {
	kind := ""
	if bound.command {
		kind = bindKindCommand
	}
	flat, _ := r.GetArray(bindStore)
	for i := 0; i+bindRecord <= len(flat); i += bindRecord {
		if flat[i] == keymap && flat[i+1] == seq {
			flat[i+2], flat[i+3] = bound.target, kind
			r.SetArray(bindStore, flat)
			return
		}
	}
	r.SetArray(bindStore, append(flat, keymap, seq, bound.target, kind))
}

// currentKeymap is the keymap `bind` acts on without `-m`, which is the
// editing mode's and nothing else — bash has no `bind` option that selects
// one, the way the other shell's `bindkey -e` and `-v` do. `set -o vi`
// selects `vi-insert`, measured.
//
// Neither mode selected leaves the map the shell started in, because there is
// nothing else to read: readline keeps whichever map was last current, and the
// three maps here carry the same defaults, so the only observable difference
// is which map a `-m` binding was recorded against.
func currentKeymap(r *interp.Runner) string {
	if r.EditingMode() == interp.EditingModeVi {
		return "vi-insert"
	}
	return "emacs"
}

// bindUsage is the line a refused option prints after the complaint, which
// bash prints and this one does too — with the letters this shell has.
const bindUsage = "bind: usage: bind [-lpsvPSVX] [-m keymap] [-f filename] [-q name] [-u name] " +
	"[-r keyseq] [-x keyseq:shell-command] [keyseq:readline-function or readline-command]"

// bindLetters are the option letters this builtin answers to.
const bindLetters = "lpPqrsSumxX"

// bindUnimplemented are the letters bash has that this shell does not, refused
// as missing rather than as unknown — the same split `whence` makes, so a
// script can tell a gap from a typo.
//
// `-x` and `-X` were here until #1428 and are not any more: the seam they
// needed had already landed with `zle -N` on the other side of the tree, and
// what was missing was this shell's half — which of its parameters the line
// arrives in, what changing them does, and what a listing looks like. All of
// that is measured now; see bindx.go and bindListCommands.
const bindUnimplemented = "vVf"

// bindOpts is what the letters asked for.
type bindOpts struct {
	list     bool   // -l: name the functions
	print    bool   // -p: list bindings as `bind` would take them back
	describe bool   // -P: list bindings as prose
	macros   bool   // -s, -S: the bindings that type text
	command  bool   // -x: the operand binds a shell command rather than a function
	commands bool   // -X: list the bindings that run a shell command
	query    string // -q: which keys run this function
	unbind   string // -u: take this function off every key
	remove   string // -r: take this key sequence off
	keymap   string // -m: the keymap this command acts on
}

func bindBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	if !r.Interactive {
		// A shell with no line editor still answers every question `bind`
		// asks — measured, `bash -c 'bind -p'` prints the whole keymap and
		// `bash -c 'bind -m nosuch -q x'` still refuses the keymap at 1 —
		// and it remarks first that nothing it is told will be pressed.
		// Ahead of the option reading, because bash prints it ahead of the
		// bad-option complaint too.
		r.Diagnosef("bind: warning: line editing not enabled\n")
	}
	opts, rest, code := bindOptions(r, args)
	if code != 0 {
		return code
	}
	keymap := opts.keymap
	if keymap == "" {
		keymap = currentKeymap(r)
	}
	switch {
	case opts.list:
		return bindListFunctions(r)
	case opts.query != "":
		return bindQuery(r, keymap, opts.query)
	case opts.unbind != "":
		return bindUnbindFunction(r, keymap, opts.unbind)
	case opts.remove != "":
		// Measured, `-r` takes a key off whatever it was bound to, `-x`
		// commands included: `bind -r "\C-t"` after a `bind -x` on that key
		// leaves `bind -X` printing nothing, at status 0.
		changeBinding(r, keymap, decodeBindSequence(opts.remove), bindEntry{})
		return 0
	case opts.macros:
		bindListMacros(r, keymap)
		return 0
	case opts.commands:
		bindListCommands(r, keymap)
		return 0
	case opts.print || opts.describe:
		bindListBindings(r, keymap, opts.describe)
		return 0
	}
	// Everything left is a binding to make. `bind` with nothing at all prints
	// nothing and reports success, measured — it is not the listing that `-p`
	// is.
	for _, word := range rest {
		if code := bindOperand(r, keymap, word, opts.command); code != 0 {
			return code
		}
	}
	return 0
}

// bindOperand makes one binding, of whichever kind the options asked for.
func bindOperand(r *interp.Runner, keymap, word string, command bool) int {
	if command {
		return bindCommand(r, keymap, word)
	}
	return bindOne(r, keymap, word)
}

// bindCommand is `-x`: a key that runs a shell command.
//
// **Only the quoted form is accepted, and that is measured rather than a
// simplification.** `bind -x '"\C-t": f'` binds the key; `bind -x "\C-t:f"`
// is `bind: \C-t:f: first non-whitespace character is not `"'` at status 1 —
// so the key-name form `bind` itself takes is not a form `-x` takes. The
// reason is visible in what the right-hand side *is*: a readline function name
// is one word and a shell command is arbitrary text, colons and all, so
// without the quotes there would be nothing to tell the key from the command.
//
// The command text is stored as it stands — not parsed, not looked up, and not
// judged. There is nothing to judge: `bind -X` prints back exactly what was
// given, and a command that does not work says so when the key is pressed, at
// the place a person can see it. That is the opposite of the rule for a
// *function* name, which is checked and dropped when unknown, and both halves
// are measured.
func bindCommand(r *interp.Runner, keymap, word string) int {
	spelled, command, ok := splitBinding(word)
	if !ok || !strings.HasPrefix(spelled, "\"") {
		r.Diagnosef("bind: %s: first non-whitespace character is not `\"'\n", word)
		return 1
	}
	seq, ok := decodeBindTarget(spelled)
	if !ok {
		return 0
	}
	changeBinding(r, keymap, seq, bindEntry{target: command, command: true})
	return 0
}

// bindOptions reads the leading option words.
//
// An unknown letter is `-Z: invalid option` and the usage line and 2, measured
// — bash's own shape for this builtin. Nothing is done after it.
func bindOptions(r *interp.Runner, args []string) (opts bindOpts, rest []string, code int) {
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
			case strings.IndexByte("mqru", letter) >= 0:
				// The letters that take an argument, which is the rest of the
				// word where there is one and the next word otherwise.
				value := word[i+1:]
				if value == "" {
					if len(rest) == 0 {
						return opts, nil, bindNeedsArgument(r, letter)
					}
					value, rest = rest[0], rest[1:]
				}
				if code := setBindArgument(r, &opts, letter, value); code != 0 {
					return opts, nil, code
				}
				i = len(word)
			case strings.IndexByte(bindLetters, letter) >= 0:
				setBindLetter(&opts, letter)
			case strings.IndexByte(bindUnimplemented, letter) >= 0:
				r.Diagnosef("bind: -%c is not implemented yet\n", letter)
				return opts, nil, 2
			default:
				r.Diagnosef("bind: -%c: invalid option\n", letter)
				_, _ = fmt.Fprintf(r.Err(), "%s\n", bindUsage)
				return opts, nil, 2
			}
		}
	}
	return opts, rest, 0
}

func setBindLetter(opts *bindOpts, letter byte) {
	switch letter {
	case 'l':
		opts.list = true
	case 'p':
		opts.print = true
	case 'P':
		opts.describe = true
	case 's', 'S':
		opts.macros = true
	case 'x':
		opts.command = true
	case 'X':
		opts.commands = true
	}
}

// setBindArgument takes one letter's argument, and is where `-m` judges the
// keymap name — the one refusal that happens while the options are still
// being read, because bash makes it there too: `bind -m nosuch -q x` complains
// about the keymap and never runs the query.
func setBindArgument(r *interp.Runner, opts *bindOpts, letter byte, value string) int {
	switch letter {
	case 'q':
		opts.query = value
	case 'u':
		opts.unbind = value
	case 'r':
		opts.remove = value
	case 'm':
		switch {
		case bindKeymaps[value] != "":
			opts.keymap = bindKeymaps[value]
		case bindPrefixKeymaps[value]:
			// A keymap bash really has and this shell does not: readline
			// reaches it through a prefix, and this editor reads a sequence
			// whole. Saying it is invalid would be a worse answer than saying
			// it is missing.
			r.Diagnosef("bind: -m %s is not implemented yet\n", value)
			return 2
		default:
			r.Diagnosef("bind: `%s': invalid keymap name\n", value)
			return 1
		}
	}
	return 0
}

// bindNeedsArgument is an argument-taking letter whose word ended the list.
// Measured: bash says the option is missing an argument and prints the usage.
func bindNeedsArgument(r *interp.Runner, letter byte) int {
	r.Diagnosef("bind: -%c: option requires an argument\n", letter)
	_, _ = fmt.Fprintf(r.Err(), "%s\n", bindUsage)
	return 2
}

// bindListFunctions is `-l`: the names this editor answers to, sorted.
//
// Every name in the map and not only the canonical ones, because each of them
// is a name `bind` will take — which is what the list is for.
func bindListFunctions(r *interp.Runner) int {
	names := make([]string, 0, len(bindFunctions))
	for name := range bindFunctions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", name)
	}
	return 0
}

// bindListBindings is `-p` and `-P`: every function and the keys it is on,
// sorted by the function's name, and the keys within one sorted by the bytes
// they send — measured in both forms.
//
// A function on no key is a row too, which is the half of the listing that
// says what this editor does *not* have on a key: `# name (not bound)` for
// `-p` and `name is not bound to any keys` for `-P`. The second has no full
// stop after it and `-q`'s version of the same sentence does, measured in
// bash 5.3 — two spellings of one thought, and not a typo here.
func bindListBindings(r *interp.Runner, keymap string, describe bool) {
	keys := keysByFunction(r, keymap)
	for _, name := range sortedFunctionNames() {
		seqs := keys[name]
		switch {
		case len(seqs) == 0 && describe:
			_, _ = fmt.Fprintf(r.Out(), "%s is not bound to any keys\n", name)
		case len(seqs) == 0:
			_, _ = fmt.Fprintf(r.Out(), "# %s (not bound)\n", name)
		case describe:
			_, _ = fmt.Fprintf(r.Out(), "%s can be found on %s.\n", name, quotedSequences(seqs))
		default:
			for _, seq := range seqs {
				_, _ = fmt.Fprintf(r.Out(), "\"%s\": %s\n", encodeBindSequence(seq), name)
			}
		}
	}
}

// bindListMacros is `-s` and `-S`: only the keys bound to text rather than to
// a function. Nothing at all where none were made, measured.
func bindListMacros(r *interp.Runner, keymap string) {
	table := readBindings(r, keymap)
	seqs := make([]string, 0, len(table))
	for seq, bound := range table {
		if !bound.command && isBindMacro(bound.target) {
			seqs = append(seqs, seq)
		}
	}
	sort.Strings(seqs)
	for _, seq := range seqs {
		_, _ = fmt.Fprintf(r.Out(), "\"%s\": %s\n", encodeBindSequence(seq), table[seq].target)
	}
}

// bindListCommands is `-X`: only the keys `bind -x` put a shell command on,
// sorted by the bytes the key sends, and nothing at all where none were made.
//
// Measured against bash 5.3.15 through a pseudo-terminal, and the shape is not
// the one every other listing here uses: there is **no colon** between the key
// and what it runs, and the command is quoted where `bind -p` leaves a
// function name bare.
//
//	"\C-n" "echo hi there; pwd"
//	"\C-t" "names"
//
// So a `-X` line is not a word `bind -x` would take back, which `-p`'s lines
// are for `bind`. That is bash's answer and not a slip here.
func bindListCommands(r *interp.Runner, keymap string) {
	table := readBindings(r, keymap)
	seqs := make([]string, 0, len(table))
	for seq, bound := range table {
		if bound.command {
			seqs = append(seqs, seq)
		}
	}
	sort.Strings(seqs)
	for _, seq := range seqs {
		_, _ = fmt.Fprintf(r.Out(), "\"%s\" \"%s\"\n", encodeBindSequence(seq), table[seq].target)
	}
}

// bindQuery is `-q`: which keys run one function.
//
// A name this editor has not got is `unknown function name` and 1, and a name
// it has with no key on it is a sentence and 1 as well — measured, and the two
// are different answers to different questions, which is why an unbound name
// is not silently the same as an unknown one.
func bindQuery(r *interp.Runner, keymap, name string) int {
	if _, known := bindFunctions[name]; !known {
		r.Diagnosef("bind: `%s': unknown function name\n", name)
		return 1
	}
	seqs := keysByFunction(r, keymap)[name]
	if len(seqs) == 0 {
		_, _ = fmt.Fprintf(r.Out(), "%s is not bound to any keys.\n", name)
		return 1
	}
	_, _ = fmt.Fprintf(r.Out(), "%s can be invoked via %s.\n", name, quotedSequences(seqs))
	return 0
}

// bindUnbindFunction is `-u`: take one function off every key it is on.
// An unknown name is the same complaint `-q` makes, measured.
func bindUnbindFunction(r *interp.Runner, keymap, name string) int {
	if _, known := bindFunctions[name]; !known {
		r.Diagnosef("bind: `%s': unknown function name\n", name)
		return 1
	}
	for _, seq := range keysByFunction(r, keymap)[name] {
		changeBinding(r, keymap, seq, bindEntry{})
	}
	return 0
}

// bindOne makes one binding, in either of the two forms the file comment
// measures.
func bindOne(r *interp.Runner, keymap, word string) int {
	spelled, target, ok := splitBinding(word)
	if !ok {
		// Neither form. bash's complaint names the whole word and says which
		// character it wanted, measured with `bind -x`'s shape.
		r.Diagnosef("bind: %s: first non-whitespace character is not `\"'\n", word)
		return 1
	}
	seq, ok := decodeBindTarget(spelled)
	if !ok {
		// A key name that names no key. Silence and success, which is what
		// bash gives — see the file comment, where `bind "\C-x\C-a":…` binds
		// nothing at status 0.
		return 0
	}
	if text, quoted := unquoteMacro(target); quoted {
		changeBinding(r, keymap, seq, bindEntry{target: bindMacro(text)})
		return 0
	}
	if _, known := bindFunctions[target]; !known {
		// An unknown function name is not an error and is not stored, so the
		// key goes on doing what it did. Measured; see the file comment.
		return 0
	}
	changeBinding(r, keymap, seq, bindEntry{target: target})
	return 0
}

// splitBinding takes a `keyseq:function` word apart.
//
// The colon that separates them is the *last* one that could, because a key
// sequence may contain one: `bind '":": beginning-of-line'` binds the colon
// key. In the quoted form the closing quote settles it and the colon after it
// is the separator; in the key-name form there is no quote and the first colon
// is the separator, since a key name never contains one.
func splitBinding(word string) (spelled, target string, ok bool) {
	if strings.HasPrefix(word, "\"") {
		end := strings.Index(word[1:], "\"")
		if end < 0 {
			return "", "", false
		}
		rest := strings.TrimLeft(word[end+2:], " \t")
		if !strings.HasPrefix(rest, ":") {
			return "", "", false
		}
		return word[:end+2], strings.TrimSpace(rest[1:]), true
	}
	colon := strings.Index(word, ":")
	if colon < 0 {
		return "", "", false
	}
	return word[:colon], strings.TrimSpace(word[colon+1:]), true
}

// decodeBindTarget reads the left side of a binding, in whichever of the two
// forms it was written.
//
// The quoted form is a whole sequence and every byte of it counts. The bare
// form is one key by name, and a name that is not one key is refused — which
// is what keeps `bind "\C-x\C-a":…` from binding a two-byte sequence bash
// leaves alone.
func decodeBindTarget(spelled string) (string, bool) {
	if strings.HasPrefix(spelled, "\"") && strings.HasSuffix(spelled, "\"") && len(spelled) >= 2 {
		return decodeBindSequence(spelled[1 : len(spelled)-1]), true
	}
	return decodeKeyName(spelled)
}

// decodeKeyName reads readline's name for a single key.
//
// Measured by binding each and reading it back: `\C-l`, `C-l` and `Control-l`
// are the same byte, `\M-l`, `M-l` and `Meta-l` set the top bit, `DEL` is
// 0x7f, `SPACE` and `SPC` are a space, `ESC` is escape, `RET` and `RETURN` and
// `NEWLINE` and `LFD` are the two line enders, `RUBOUT` is 0x7f as well, and
// a single ordinary character is itself. Anything else names no key.
func decodeKeyName(name string) (string, bool) {
	if named, ok := bindNamedKeys[strings.ToUpper(name)]; ok {
		return named, true
	}
	for _, p := range bindKeyPrefixes {
		if len(name) > len(p.prefix) && strings.EqualFold(name[:len(p.prefix)], p.prefix) {
			rest, ok := decodeKeyName(name[len(p.prefix):])
			if !ok || len(rest) != 1 {
				return "", false
			}
			return string(p.apply(rest[0])), true
		}
	}
	if len(name) == 1 {
		return name, true
	}
	return "", false
}

// bindNamedKeys are the keys readline lets a person spell by name, upper-cased
// because the names are matched without regard to case.
var bindNamedKeys = map[string]string{
	"DEL":     "\x7f",
	"RUBOUT":  "\x7f",
	"ESC":     "\x1b",
	"ESCAPE":  "\x1b",
	"SPACE":   " ",
	"SPC":     " ",
	"RET":     "\r",
	"RETURN":  "\r",
	"NEWLINE": "\n",
	"LFD":     "\n",
	"TAB":     "\t",
}

// bindKeyPrefixes are the modifier prefixes a key name may carry, longest
// first so that `Control-` is not read as `C-` with a stray `ontrol-`.
var bindKeyPrefixes = []struct {
	prefix string
	apply  func(byte) byte
}{
	{"Control-", controlByte},
	{"Meta-", metaByte},
	{"\\C-", controlByte},
	{"\\M-", metaByte},
	{"C-", controlByte},
	{"M-", metaByte},
}

// controlByte is the byte a control-modified key sends: the top three bits
// cleared, with the one exception both spellings agree on — `\C-?` is DEL and
// not the 0x1f that clearing the bits of `?` would give.
func controlByte(c byte) byte {
	if c == '?' {
		return 0x7f
	}
	return c & 0x1f
}

// metaByte is the byte a meta-modified key sends, which readline makes by
// setting the top bit rather than by prefixing an escape — which is why it
// prints back in octal and not as `\e` and the letter.
func metaByte(c byte) byte { return c | 0x80 }

// bindMacro spells the text a key was bound to, quoted the way the listing
// prints it, which is how `bind -p` tells the two kinds of binding apart when
// it prints one back.
//
// Stored and listed, and inert at the keyboard: typing text from a key is not
// something this editor can be asked to do. The other dialect's `bindkey -s`
// is the same partial, recorded in docs/spec/editing.md so that neither is a
// surprise.
func bindMacro(text string) string { return "\"" + encodeBindSequence(text) + "\"" }

// isBindMacro says whether a stored target is text rather than a function
// name, which the quotes settle — no function name has any.
func isBindMacro(target string) bool { return strings.HasPrefix(target, "\"") }

// unquoteMacro reads a `"text"` target back into the text it types.
func unquoteMacro(target string) (string, bool) {
	if len(target) >= 2 && strings.HasPrefix(target, "\"") && strings.HasSuffix(target, "\"") {
		return decodeBindSequence(target[1 : len(target)-1]), true
	}
	return "", false
}

// keysByFunction inverts one keymap: for each function name, the keys it is
// on, in the order the bytes sort. Macros and `-x` commands are left out,
// since neither names a function.
func keysByFunction(r *interp.Runner, keymap string) map[string][]string {
	out := map[string][]string{}
	for seq, bound := range readBindings(r, keymap) {
		if bound.command || isBindMacro(bound.target) {
			// A shell command is left out for the reason a macro is: it names
			// no function, so `-p`, `-P`, `-q` and `-u` have nothing to say
			// about it. Measured — `bind -p` has no row for a key `-x` bound,
			// and `bind -q` answers `unknown function name` at 1 for the
			// command's text.
			continue
		}
		out[bound.target] = append(out[bound.target], seq)
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out
}

// sortedFunctionNames is every canonical name, sorted, which is the order a
// listing walks.
//
// The canonical names only, because a listing prints one row per action and
// the synonyms are names `bind` will *take* rather than names it says back.
func sortedFunctionNames() []string {
	names := make([]string, 0, len(bindFunctionNames))
	for _, name := range bindFunctionNames {
		names = append(names, name)
	}
	names = append(names, "accept-line")
	sort.Strings(names)
	return names
}

// quotedSequences is the comma-separated list `-P` and `-q` print.
func quotedSequences(seqs []string) string {
	quoted := make([]string, 0, len(seqs))
	for _, seq := range seqs {
		quoted = append(quoted, "\""+encodeBindSequence(seq)+"\"")
	}
	return strings.Join(quoted, ", ")
}

// decodeBindSequence reads the escape notation a whole key sequence is written
// in, which is readline's and not the caret notation the other shell also
// takes.
//
// Measured by binding each form and reading it back: `\C-x` is the byte with
// the top three bits cleared, `\e` and `\E` are escape, `\t` and the rest are
// C's escapes, `\x41` is hexadecimal, `\101` is octal, and `\M-q` sets the top
// bit.
func decodeBindSequence(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			b, width := decodeBindEscape(s[i+1:])
			out.WriteByte(b)
			i += 1 + width
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// decodeBindEscape reads one backslash escape, having been given what follows
// the backslash, and answers with the byte and how much of the text it used.
func decodeBindEscape(s string) (byte, int) {
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
			return controlByte(s[2]), 3
		}
		return c, 1
	case 'M':
		if len(s) > 2 && s[1] == '-' {
			return metaByte(s[2]), 3
		}
		return c, 1
	case 'x', 'X':
		return decodeBindNumber(s[1:], 16, 2)
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return decodeBindNumber(s, 8, 3)
	default:
		// A backslash before anything else is that thing, which is how `\\`
		// and `\"` reach the sequence.
		return c, 1
	}
}

// decodeBindNumber reads up to most digits in the given base and answers with
// the byte and how much of the text it used, the escape letter included where
// there was one.
func decodeBindNumber(s string, base, most int) (byte, int) {
	used := 0
	for used < most && used < len(s) && isBindDigit(s[used], base) {
		used++
	}
	if used == 0 {
		return s[0], 1
	}
	n := 0
	for i := 0; i < used; i++ {
		n = n*base + bindDigitValue(s[i])
	}
	width := used
	if base == 16 {
		// The `x` itself, which the octal form does not have.
		width++
	}
	return byte(n), width
}

func isBindDigit(c byte, base int) bool {
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

func bindDigitValue(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	return int(c|0x20) - 'a' + 10
}

// encodeBindSequence writes a sequence back the way this shell prints one.
//
// Measured against bash 5.3, which is not the caret notation the other shell
// prints: escape is `\e`, another control byte is `\C-` and the letter it
// clears down from, DEL is `\C-?`, a byte with the top bit set is three octal
// digits, and a backslash or a double quote is escaped.
func encodeBindSequence(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x1b:
			out.WriteString(`\e`)
		case c == 0x7f:
			out.WriteString(`\C-?`)
		case c == 0:
			// NUL is the one control byte whose letter is not a letter:
			// clearing the bits of `@` gives it, and that is how bash prints
			// it back — measured, `\C-@`.
			out.WriteString(`\C-@`)
		case c < 0x20:
			out.WriteString(`\C-`)
			out.WriteByte(c + 'a' - 1)
		case c >= 0x80:
			_, _ = fmt.Fprintf(&out, `\%03o`, c)
		case c == '"' || c == '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
