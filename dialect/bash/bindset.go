// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"strings"

	"github.com/blairham/sh/interp"
)

// `bind 'set NAME VALUE'` sets a readline variable, the inputrc line real
// bashrc files use — `bind 'set completion-ignore-case on'`, `bind 'set
// enable-bracketed-paste off'` (#6264).
//
// Measured 2026-10-06 against bash 5.3.20, under `-c` and through a
// pseudo-terminal alike:
//
//	bind 'set completion-ignore-case on'   silent, status 0
//	bind 'set NoSuch on'                   readline: NoSuch: unknown variable name, 0
//	bind 'set'                             readline: : unknown variable name, 0
//	bind 'set Editing-Mode foo'            readline: Editing-Mode: could not set value to `foo', 0
//	bind 'set editing-mode'                readline: editing-mode: could not set value to `', 0
//	bind 'set Bell-Style none'             silent: a name is any case
//	bind '  set<TAB>bell-style<TAB>none'   silent: blanks before it and tabs between
//
// The status is 0 whatever is said. A boolean is on for `on` in any case, `1`
// or nothing, and off for anything else; `bell-style` takes none or off,
// visible, and audible, on or nothing; `editing-mode` vi or emacs; `keymap`
// a keymap's name. Every value is in any case, and other variables take any
// value.
//
// Two of them have something here to act on. `editing-mode` selects the
// editing mode, which `set -o` then reports — measured, `set -o` says vi is on
// after `bind 'set editing-mode vi'`, though `$SHELLOPTS` goes on saying
// emacs until it is next built. And `enable-bracketed-paste` asks the
// terminal to mark a paste, or stops asking, from the next prompt on: see
// repl.EditorStyle.BracketedPasteSetting. The rest are kept and not acted on;
// `bind -v` is still refused, so nothing reports them back.

// readlineVariables are the names `bind 'set'` takes, each with its kind: the
// 48 `bind -v` lists in bash 5.3.20, and three more it accepts and does not
// list.
var readlineVariables = map[string]readlineKind{
	"bind-tty-special-chars": rlBool, "blink-matching-paren": rlBool, "byte-oriented": rlBool,
	"colored-completion-prefix": rlBool, "colored-stats": rlBool, "completion-ignore-case": rlBool,
	"completion-map-case": rlBool, "convert-meta": rlBool, "disable-completion": rlBool,
	"echo-control-characters": rlBool, "enable-active-region": rlBool, "enable-bracketed-paste": rlBool,
	"enable-keypad": rlBool, "enable-meta-key": rlBool, "expand-tilde": rlBool, "force-meta-prefix": rlBool,
	"history-preserve-point": rlBool, "horizontal-scroll-mode": rlBool, "input-meta": rlBool,
	"mark-directories": rlBool, "mark-modified-lines": rlBool, "mark-symlinked-directories": rlBool,
	"match-hidden-files": rlBool, "menu-complete-display-prefix": rlBool, "meta-flag": rlBool,
	"output-meta": rlBool, "page-completions": rlBool, "prefer-visible-bell": rlBool,
	"print-completions-horizontally": rlBool, "revert-all-at-newline": rlBool, "search-ignore-case": rlBool,
	"show-all-if-ambiguous": rlBool, "show-all-if-unmodified": rlBool, "show-mode-in-prompt": rlBool,
	"skip-completed-text": rlBool, "visible-stats": rlBool,

	"bell-style": rlBell, "editing-mode": rlEditingMode, "keymap": rlKeymap,

	"comment-begin": rlString, "completion-display-width": rlString,
	"completion-prefix-display-length": rlString, "completion-query-items": rlString,
	"emacs-mode-string": rlString, "history-size": rlString, "keyseq-timeout": rlString,
	"vi-cmd-mode-string": rlString, "vi-ins-mode-string": rlString,
	"active-region-start-color": rlString, "active-region-end-color": rlString,
	"isearch-terminators": rlString,
}

// readlineKind is what values a readline variable takes.
type readlineKind int

const (
	rlString readlineKind = iota
	rlBool
	rlBell
	rlEditingMode
	rlKeymap
)

// readlineVariableStore is where a variable's value is kept, under a name no
// script can reach — the way bindStore keeps the bindings.
const readlineVariableStore = ".bash.readline."

// bracketedPasteSetting is the variable the line editor asks whether to
// bracket a paste. See repl.EditorStyle.BracketedPasteSetting.
const bracketedPasteSetting = readlineVariableStore + "enable-bracketed-paste"

// readlineSetLine reports whether a `bind` operand is a `set` line, and splits
// it into the name and the value.
func readlineSetLine(word string) (name, value string, ok bool) {
	rest := strings.TrimLeft(word, " \t")
	if !strings.HasPrefix(rest, "set") {
		return "", "", false
	}
	rest = rest[len("set"):]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", "", false
	}
	fields := strings.Fields(rest)
	switch len(fields) {
	case 0:
		return "", "", true
	case 1:
		return fields[0], "", true
	}
	return fields[0], fields[1], true
}

// bindSet is one `set` line. See the file comment for what it answers.
func bindSet(r *interp.Runner, name, value string) int {
	kind, known := readlineVariables[strings.ToLower(name)]
	if !known {
		_, _ = fmt.Fprintf(r.Err(), "readline: %s: unknown variable name\n", name)
		return 0
	}
	v, ok := readlineValue(kind, value)
	if !ok {
		_, _ = fmt.Fprintf(r.Err(), "readline: %s: could not set value to `%s'\n", name, value)
		return 0
	}
	key := strings.ToLower(name)
	r.SetVar(readlineVariableStore+key, v)
	if key == "editing-mode" {
		if set, found := r.Builtin("set"); found {
			set(r, context.Background(), []string{"-o", v})
		}
	}
	return 0
}

// readlineValue is a value as its variable keeps it, and whether the variable
// takes it at all.
func readlineValue(kind readlineKind, value string) (string, bool) {
	v := strings.ToLower(value)
	switch kind {
	case rlBool:
		if v == "" || v == "on" || v == "1" {
			return "on", true
		}
		return "off", true
	case rlBell:
		switch v {
		case "none", "off":
			return "none", true
		case "visible":
			return "visible", true
		case "audible", "on", "":
			return "audible", true
		}
		return "", false
	case rlEditingMode:
		if v == "vi" || v == "emacs" {
			return v, true
		}
		return "", false
	case rlKeymap:
		if _, ok := bindKeymaps[v]; ok || bindPrefixKeymaps[v] {
			return v, true
		}
		return "", false
	}
	return value, true
}
