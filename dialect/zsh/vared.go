// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"sort"
	"strconv"
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
// # The editing half, which is the session's
//
// At a prompt the value goes into the line editor, is edited, and what was
// accepted is stored. That needs a seam running from the shell *to* the
// editor, which is the only one of its kind: repl holds the editor and
// restores the terminal's own line discipline before every command, so a
// builtin reading a line has to be handed the editor and the raw-mode round
// trip with it. [interp.Runner.EditLine] is that seam and repl/lineread.go
// fills it in (#2914).
//
// Measured 2026-09-18 through a pseudo-terminal against zsh 5.9.2 under `-i`
// with a scratch rc:
//
//	v=hello; vared v          draws `hello`, cursor at its end; typing appends
//	vared -p 'P ' v           draws `P hello` — the prompt and nothing else
//	vared -c newv             an empty line, and the name is created from it
//	vared -c -a newa          the same, split into an array
//	vared -c -A newm          the same, read as keys and values
//	a=(one two); vared a      `one two`, joined on the first character of IFS
//	vared 'a[2]'              the element, written back to the element alone
//	vared -h v                Up recalls the session's own history; without
//	                          it Up rings the bell and recalls nothing
//	^C during the edit        130, the variable untouched, and **the rest of
//	                          the line does not run**
//	vared -e v, ^D when empty 1, the variable untouched
//
// **Two rows of #2914's own table are corrected by that run.** It says `^C`
// is status 1 — it is 130, and it abandons what the shell was reading, which
// is why `vared v; print after` prints nothing. And it says `^D` on an empty
// line does the same as `^C` — without `-e` it does neither: zsh's `^D` there
// is `delete-char-or-list`, and on an empty line it offered to list all 1064
// commands. This editor's `^D` is not that action, so without `-e` the key
// does nothing here; see repl's lineStart.endOnEndOfInput.
//
// # What is still refused, by name
//
// `-t`, `-r`, `-M`, `-m`, `-i`, `-f` and `-g` are stored and then refused: a
// terminal other than the shell's, a right-hand prompt, a keymap to read the
// line under, and widgets to run at each end of it are each a real feature
// this editor has no half of. Refusing by name is the rule the rest of this
// dialect follows — a letter accepted and ignored is a script that thinks it
// got what it asked for.
//
// `vared 1` from inside a *widget* is a different problem and stays refused
// through the same door: the editor is already reading a key there, where at a
// prompt it is idle between commands. #2914 splits that half out.
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
	var array, assoc, create, history, endOnEndOfInput bool
	var prompt, unimplemented string
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
			case 'h':
				history = true
			case 'e':
				endOnEndOfInput = true
			case 'g', 'M', 'm', 'i', 'f', 'r', 't':
				// Accepted, stored and not acted on — a keymap, a widget to
				// run at each end of the read, a right-hand prompt, and a
				// terminal other than the shell's. Each is a real feature
				// this editor has no half of, so a run that named one is
				// refused by name below rather than edited as though the
				// letter had not been there. The first one named is what the
				// refusal says, which is the order the letters were read in.
				if unimplemented == "" {
					unimplemented = string(rune(letter))
				}
			}
			if !strings.ContainsRune(varedArgLetters, rune(letter)) {
				continue
			}
			// The argument is the rest of the word or the word after it,
			// which is the shape every letter-with-a-value in this dialect
			// has. An argument that is not there is its own complaint and
			// names the letter.
			if at+1 < len(word) {
				// The argument was attached, so the rest of the word is it
				// and there are no more letters in this word. `break` is the
				// whole of that — walking `at` to the end first would be an
				// assignment nothing reads.
				if letter == 'p' {
					prompt = word[at+1:]
				}
				break
			}
			if i+1 >= len(args) {
				r.Diagnosef("argument expected: -%c\n", letter)
				return 1
			}
			i++
			if letter == 'p' {
				prompt = args[i]
			}
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
	if r.EditLine == nil {
		// A terminal, and a front end that offered no editor to re-enter.
		// Named rather than answered with the sentence above, which would be
		// a wrong sentence where the terminal is plainly there.
		r.Diagnosef("the line editor cannot be re-entered from a command yet\n")
		return 1
	}
	if unimplemented != "" {
		r.Diagnosef("-%s is not implemented yet\n", unimplemented)
		return 1
	}
	line, end := r.EditLine(interp.LineEdit{
		Prompt:          prompt,
		Initial:         varedInitial(r, name),
		History:         history,
		EndOnEndOfInput: endOnEndOfInput,
	})
	switch end {
	case interp.LineEditInterrupted:
		// **130, and the rest of the line does not run.** Measured 2026-09-18
		// through a pseudo-terminal against zsh 5.9.2: `vared v; print after`
		// interrupted with `^C` prints nothing and the next `$?` is 130 — so
		// the interrupt abandons what the shell was reading rather than
		// failing one command. #2914 records this as "the variable is left as
		// it was, status 1", which is the status of the *other* ending.
		r.StopTheScript(varedInterruptStatus)
		return varedInterruptStatus
	case interp.LineEditEndOfInput:
		// Only reachable with `-e`, which is what asked for the key to end
		// the read. Measured: the variable is left as it was and the status
		// is 1.
		return 1
	case interp.LineEditUnavailable:
		return 1
	}
	varedStore(r, name, line, array, assoc)
	return 0
}

// varedInterruptStatus is what an abandoned edit answers: 128 plus the
// interrupt, which is what a command killed by one answers everywhere.
const varedInterruptStatus = 130

// varedInitial is the text the line starts from.
//
// Measured 2026-09-18 through a pseudo-terminal against zsh 5.9.2, one shape
// at a time, with the value joined on the **first character of IFS** rather
// than on a space — `IFS=:` with `a=(one two)` draws `one:two`, which is the
// same rule `$*` joins on:
//
//	v=hello        vared v        hello
//	a=(one two)    vared a        one two, and what is accepted is split again
//	a=(one two)    vared 'a[2]'   two, and only that element is written back
//	typeset -A m=(k1 v1)  vared m  k1 v1 — the pairs, joined the same way
//	vared -c newv                  nothing; the line starts empty
//
// An association's pairs are drawn in sorted key order, which is this tree's
// answer everywhere an association is listed: the shells promise no order and
// a deterministic one is worth having. See interp.AssocArray.
func varedInitial(r *interp.Runner, name string) string {
	if base, index, ok := varedElement(r, name); ok {
		elems, _ := r.GetArray(base)
		if index >= 1 && index <= len(elems) {
			return elems[index-1]
		}
		return ""
	}
	// The compound shapes first, and that order is load-bearing rather than
	// tidy: a scalar read of an association answers its *first value* and a
	// scalar read of an array answers its first element, so asking GetVar
	// first draws `v1` where the line should hold `k1 v1`.
	if table, ok := r.GetAssoc(name); ok {
		keys := make([]string, 0, len(table))
		for k := range table {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, 2*len(keys))
		for _, k := range keys {
			pairs = append(pairs, k, table[k])
		}
		return r.JoinOnIFS(pairs)
	}
	if elems, ok := r.GetArray(name); ok {
		return r.JoinOnIFS(elems)
	}
	if v, ok := r.GetVar(name); ok {
		return v
	}
	return ""
}

// varedStore writes back what was accepted.
//
// **The kind already there decides, and the letters decide only for a name
// that is not there yet.** That is what `-a` and `-A` mean — they say which
// kind to *create*, which is why they are ignored without `-c` and why this
// asks the store first. A name that holds an array gets the line split back
// into elements whatever the letters said.
func varedStore(r *interp.Runner, name, line string, array, assoc bool) {
	if base, index, ok := varedElement(r, name); ok {
		elems, _ := r.GetArray(base)
		for len(elems) < index {
			elems = append(elems, "")
		}
		if index >= 1 {
			elems[index-1] = line
		}
		r.SetArray(base, elems)
		return
	}
	if _, ok := r.GetAssoc(name); ok || assoc {
		r.SetAssoc(name, varedPairs(r.SplitOnIFS(line)))
		return
	}
	if _, ok := r.GetArray(name); ok || array {
		r.SetArray(name, r.SplitOnIFS(line))
		return
	}
	r.SetVar(name, line)
}

// varedPairs reads a flat list as an association's keys and values, which is
// how `-c -A` builds one: measured, `kk vv` accepted for `vared -c -A newm`
// makes `$newm[kk]` `vv`. A trailing key with no value takes the empty string.
func varedPairs(fields []string) map[string]string {
	out := make(map[string]string, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		value := ""
		if i+1 < len(fields) {
			value = fields[i+1]
		}
		out[fields[i]] = value
	}
	return out
}

// varedElement splits `a[2]` into the array and the one-based index, and
// reports whether the operand was of that shape at all.
//
// The index is arithmetic in the shell being modeled; only a literal number is
// read here, and anything else is treated as a plain name — which is what this
// builtin's own existence check already does with the operand.
func varedElement(r *interp.Runner, name string) (base string, index int, ok bool) {
	text, rest, cut := strings.Cut(name, "[")
	if !cut || !strings.HasSuffix(rest, "]") {
		return "", 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(rest, "]"))
	if err != nil {
		return "", 0, false
	}
	if _, isArray := r.GetArray(text); !isArray {
		return "", 0, false
	}
	return text, n, true
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
