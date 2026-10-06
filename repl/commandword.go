// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"slices"
	"unicode"
)

// commandWord is the command word of the command the cursor is in: the last
// word in command position that begins at or before the cursor. Empty where
// there is none — an empty line, or a line of nothing but assignments.
//
// What which-command and run-help ask about. Measured 2026-10-06 through a
// pseudo-terminal against zsh 5.9.2, `zsh -f`, `M-?` with `which-command`
// aliased to print what it was given:
//
//	line                      cursor     command
//	echo abc def              anywhere   echo
//	ls -l; echo x             0 to 5     ls       ← the `;` is still ls's
//	ls -l; echo x             7 on       echo
//	ls -l; ␠                  end        ls       ← nothing after the `;` yet
//	FOO=1 ls -l               end        ls       ← an assignment is passed over
//	a=1 b=2                   end        none     ← and a bell
//	time ls, nocorrect ls, ! ls          ls
//	if true; then ls          end        ls       ← and `then`, `do`, `{`, `(`
//	builtin echo, command ls, exec ls, noglob ls, - ls   the first word
//	echo "a;b" x, echo 'a|b', echo $(ls)             echo  ← quoted, not a separator
//
// So the separators are the unquoted `;`, `&`, `|`, newline and the opening
// `(` and `{`; reserved words that begin a command and three precommand
// modifiers are passed over and nothing else is.
func commandWord(line []rune, cursor int) string {
	found, atCommand := "", true
	for i := 0; i < len(line) && i <= cursor; {
		switch line[i] {
		case ' ', '\t':
			i++
			continue
		case ';', '&', '|', '\n', '(':
			atCommand = true
			i++
			continue
		case ')':
			i++
			continue
		}
		start := i
		i = wordEnd(line, i)
		word := string(line[start:i])
		switch {
		case !atCommand, isAssignment(word):
		case word == "{" || passedOver[word]:
			// A group's brace begins a command the way `(` does, and is a
			// word of its own only where a blank follows it.
		default:
			found, atCommand = word, false
		}
	}
	return found
}

// passedOver is the words in command position that are not the command: the
// reserved words that begin one, and the precommand modifiers zsh looks past.
var passedOver = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "while": true, "until": true,
	"do": true, "!": true, "time": true, "nocorrect": true,
}

// isAssignment is whether a word is NAME=… .
func isAssignment(word string) bool {
	for i, r := range word {
		switch {
		case r == '=':
			return i > 0
		case r == '_' || unicode.IsLetter(r) || i > 0 && unicode.IsDigit(r):
		default:
			return false
		}
	}
	return false
}

// wordEnd is where the word starting at i ends: at the first unquoted blank
// or separator, with quotes, a backslash and `$(…)` or backquotes read whole.
func wordEnd(line []rune, i int) int {
	depth := 0
	for i < len(line) {
		r := line[i]
		switch {
		case r == '\\':
			i += 2
			continue
		case r == '\'':
			i++
			for i < len(line) && line[i] != '\'' {
				i++
			}
		case r == '"':
			i++
			for i < len(line) && line[i] != '"' {
				if line[i] == '\\' {
					i++
				}
				i++
			}
		case r == '`':
			i++
			for i < len(line) && line[i] != '`' {
				i++
			}
		case r == '$' && i+1 < len(line) && line[i+1] == '(':
			depth++
			i++
		case r == ')' && depth > 0:
			depth--
		case depth == 0 && (r == ' ' || r == '\t' || r == ';' || r == '&' || r == '|' || r == '\n' || r == '(' || r == ')'):
			return i
		}
		i++
	}
	return min(i, len(line))
}

// askAboutTheCommand is which-command and run-help: the line put aside, as
// push-line puts it, and `with` and the command word run in its place.
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt: `M-?` on `echo abc def` with the cursor at 10 draws
// `which-command echo` over the line, prints `echo`, and the next prompt holds
// `echo abc def` with the cursor at 10; `which-command echo` is in the history
// afterwards. `M-h` is the same with `run-help`, which is `man` by default.
// On a line with no command word — empty, or only assignments — both ring
// and leave the line alone (#6241).
func (e *editor) askAboutTheCommand(with string, prompt drawnPrompt) {
	word := commandWord(e.line, e.pos)
	if with == "" || word == "" {
		e.ring()
		e.actionStatus = 1
		return
	}
	e.bufferStack = append(e.bufferStack, snapshot{line: slices.Clone(e.line), pos: e.pos})
	e.line = []rune(with + " " + word)
	e.pos = len(e.line)
	e.redraw(prompt)
	e.acceptRequested = true
}
