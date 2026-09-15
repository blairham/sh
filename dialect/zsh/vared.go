// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `vared`: edit a variable's value with the line editor.
//
// # Why this is here and the other two of #1405 are not
//
// That issue named `vared`, `zcompile` and `zregexparse` together and said to
// order them by evidence rather than by guesswork — sweep a real plugin tree
// and see which is called. Swept on this machine, over `~/.zi` and
// `/opt/homebrew/share/zsh`:
//
//	zcompile      25 files
//	vared          3 files
//	zregexparse    0 files
//
// `zcompile` was built on that evidence and is in zcompile.go (#2079), which
// makes the issue's own table stale for one of its three rows. `zregexparse`
// is a completion-system internal and stays absent: #1282 recorded that
// `compinit` is not close — the file is refused, `zmodload zsh/complete`
// refuses by name, `compdef` and `compctl` have no completer behind them — so
// building the one parser buys nothing, and zmodload.go carries that decision
// where a reader looking for the name will find it.
//
// This is the third, and its three callers are a plugin manager asking a
// question at a prompt (`vared -cp 'Create under an organization? (y/n): '
// isorg`) and an autosuggestion widget re-entering the editor (`vared 1`).
//
// # What a script sees, which is the whole of it without a terminal
//
// Measured 2026-09-15 against zsh 5.9.2, `-f`, with standard input closed:
//
//	vared                         not enough arguments                   1
//	vared a b                     too many arguments                     1
//	vared 1bad                    not an identifier: `1bad'              1
//	vared nosuch                  no such variable: nosuch               1
//	v=x; vared v                  can't access terminal                  1
//	vared -c nosuch               can't access terminal                  1
//	vared -Q v                    bad option: -Q                         1
//	vared -p                      argument expected: -p                  1
//	vared -aA v                   specify only one of -a and -A          1
//	v=x; vared -a v               -a ignored, then can't access terminal 1
//	vared -c -a nosuch            can't access terminal — no warning     1
//	v=x; vared -t /dev/null v     /dev/null: not a terminal              1
//
// **Every run without a terminal ends at `can't access terminal`**, with a
// pipe on standard input as much as with nothing — measured both ways. So the
// refusals above are not a subset of what this builtin does outside a
// session; they are all of it.
//
// The order is measured rather than assumed, and three pairs pin it: `vared
// -Q -p` is the bad option and not the missing argument, `vared -a 1bad` is
// the `-a ignored` warning and *then* the bad name, and `vared -t /dev/null
// -c v` is the terminal named by the letter and not the one the shell has.
//
// # The editing half
//
// It is refused by name, and the sentence says which half is missing rather
// than pretending the terminal is unreachable — a shell at a prompt has one,
// and answering `can't access terminal` there would be a wrong sentence
// rather than a missing feature.
//
// What it needs is a seam this package cannot reach: repl holds the editor and
// restores the terminal's own line discipline before every command, so a
// builtin that wanted to read a line would have to be handed the editor and
// the raw-mode round trip with it. Every seam repl offers today runs the other
// way — repl calling the shell — and #2914 carries the measured table for the
// one that would run this way.
//
// # `-a` and `-A` are ignored without `-c`, which is measured and not a guess
//
// They say what *kind* of variable to create, so they mean something only
// beside the letter that creates one: with `-c` the warning is not written at
// all. Both together is a refusal of the line.

// varedArgLetters take a value. `-f` is here and is easy to miss: `vared -f v
// w` is `no such variable: w`, so the `v` went to the letter.
const varedArgLetters = "Mmtprif"

// varedLetters is every letter this builtin takes, the argument-taking ones
// included. Measured a letter at a time: `-Q`, `-X` and `-n` are `bad option`,
// and these are not.
const varedLetters = "aAcgheMmtprif"

func registerVared(r *interp.Runner) { r.Register("vared", varedBuiltin) }

func varedBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	var array, assoc, create bool
	var operands []string
	ended := false
	for i := 0; i < len(args); i++ {
		word := args[i]
		switch {
		case ended || word == "" || word[0] != '-' || word == "-":
			operands = append(operands, word)
			continue
		case word == "--":
			ended = true
			continue
		}
		for at := 1; at < len(word); at++ {
			letter := word[at]
			if !strings.ContainsRune(varedLetters, rune(letter)) {
				r.Diagnosef("bad option: -%c\n", letter)
				return 1
			}
			switch letter {
			case 'a':
				array = true
			case 'A':
				assoc = true
			case 'c':
				create = true
			}
			if !strings.ContainsRune(varedArgLetters, rune(letter)) {
				continue
			}
			// The argument is the rest of the word or the word after it,
			// which is the shape every letter-with-a-value in this dialect
			// has. An argument that is not there is its own complaint and
			// names the letter.
			if at+1 < len(word) {
				at = len(word)
				break
			}
			if i+1 >= len(args) {
				r.Diagnosef("argument expected: -%c\n", letter)
				return 1
			}
			i++
			break
		}
	}
	if array && assoc {
		r.Diagnosef("specify only one of -a and -A\n")
		return 1
	}
	switch {
	case len(operands) == 0:
		r.Diagnosef("not enough arguments\n")
		return 1
	case len(operands) > 1:
		r.Diagnosef("too many arguments\n")
		return 1
	}
	name := operands[0]
	if (array || assoc) && !create {
		// Ahead of the name check, measured: `vared -a 1bad` writes the
		// warning and then refuses the name.
		letter := "a"
		if assoc {
			letter = "A"
		}
		r.Diagnosef("-%s ignored\n", letter)
	}
	if !isVaredName(name) {
		r.Diagnosef("not an identifier: `%s'\n", name)
		return 1
	}
	if !create && !varedNameIsSet(r, name) {
		r.Diagnosef("no such variable: %s\n", name)
		return 1
	}
	if !r.Terminal {
		// The whole of what a script outside a session ever sees, and it is
		// the same answer with a pipe on standard input: the editor needs a
		// terminal and not a reader.
		r.Diagnosef("can't access terminal\n")
		return 1
	}
	// A session, where the terminal is there and the editor is not reachable
	// from here. Named rather than answered with the sentence above, which
	// would be a wrong sentence at a prompt. See the note at the top of this
	// file and #2914.
	r.Diagnosef("the line editor cannot be re-entered from a command yet\n")
	return 1
}

// isVaredName is the identifier check this builtin makes, and it is far
// narrower than the name it is about — which is measured rather than inferred
// from what a declaration would accept.
//
// **Only a word that starts with a digit and is not all digits is refused.**
// Measured 2026-09-15 on zsh 5.9.2: `1bad` and `12abc` and `1bad[1]` are “not
// an identifier“, while `0`, `1`, `12`, `@` and `?` all pass — they are
// parameters the shell has — and so do `a b`, `a-b`, `a[` and the empty word,
// which pass this and are then refused by the *existence* check in their own
// words. A tighter reading here wrote `not an identifier` for `a[1]` where
// that shell edits the element.
func isVaredName(name string) bool {
	if name == "" || name[0] < '0' || name[0] > '9' {
		return true
	}
	return strings.TrimLeft(name, "0123456789") == ""
}

// varedNameIsSet is whether the shell has the name at all.
//
// Three shapes for a variable, because the scalar reading alone would call an
// array set — it reads the first element — and would not call an association
// set at all. And two shapes for a name: `a[1]` asks about `a`, which is what
// makes `vared 'a[1]'` reach the editor over a set array and `vared
// 'nosuch[1]'` refuse in the whole operand's words.
//
// **A parameter the shell answers for itself is set whether or not it holds
// anything**, and that is measured rather than reasoned from: `vared 9` with
// no positional parameters at all reaches the terminal check, and so do
// `vared '@'` and `vared '?'`. So a run of digits and a one-character special
// name are set by being *spellable*, and only an ordinary name is looked up.
//
// Asked by shape rather than through `${+name}`, which was the first reading
// and is wrong twice: it writes `bad substitution` for `a b` and for the empty
// word — names this builtin refuses in its own words — and it answers 0 for a
// positional nothing has set, where that shell says the name is there.
func varedNameIsSet(r *interp.Runner, name string) bool {
	if base, _, ok := strings.Cut(name, "["); ok && strings.HasSuffix(name, "]") {
		name = base
	}
	if name == "" {
		return false
	}
	if strings.TrimLeft(name, "0123456789") == "" {
		// A positional parameter, set by being spellable.
		return true
	}
	if len(name) == 1 && !isVaredNameRune(rune(name[0])) {
		// `@`, `?`, `*`, `#`, `-`, `$`, `!` — the parameters that are a
		// punctuation mark, and the same answer for the same reason.
		return true
	}
	if _, ok := r.GetVar(name); ok {
		return true
	}
	if _, ok := r.GetArray(name); ok {
		return true
	}
	_, ok := r.GetAssoc(name)
	return ok
}

// isVaredNameRune is a character an ordinary name is made of.
func isVaredNameRune(c rune) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
