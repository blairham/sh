// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package histjoin makes one history entry out of the physical lines of one
// command.
//
// A shell's history list holds a *command* where its input held lines, so a
// compound command that took five lines to write is one entry — measured on
// bash 5.3.20 by reading its own list back:
//
//	if true / then / echo hi / fi        -> `if true; then echo hi; fi`
//	for i in 1 2 / do / echo $i / done   -> `for i in 1 2; do echo $i; done`
//	case x in / x) echo y ;; / esac      -> `case x in x) echo y ;; esac`
//	f() { / echo c / }                   -> `f() { echo c; }`
//	echo a | / cat                       -> `echo a | cat`
//
// So a `;` is written unless the text so far already ends in something that
// cannot take one — a keyword or operator still waiting for its command —
// where a space is written instead. A boundary the parser was inside a quote
// or a here-document at takes a newline, because a `;` there would be text
// rather than a separator.
//
// # Why this is a package and not a method on the gate
//
// Two readers arrive at the same question. `driver`'s history gate feeds the
// parser a physical line at a time and collects what it hands over, and
// `fc`'s editor road runs the lines an editor left behind and records them
// the same way — bash reaches both through one input stream, so one rule is
// what the panel has. The gate had the rule first; the second reader would
// have grown a copy of it, and a copy is how the next measurement lands in
// one of them and not the other.
package histjoin

import "strings"

// Entry collects the physical lines of one command.
//
// The zero value is an empty entry, ready to take its first line.
type Entry struct {
	// lines are the physical lines collected so far and seps the separator
	// that joins each one after the first to the one before it. Kept apart
	// so that joining happens once, when the command is complete.
	lines []string
	seps  []string
	// heredoc records that one of those separators was a here-document's,
	// which changes how the whole entry is joined. See String.
	heredoc bool
	// endsInBody records that the newest line was read inside a
	// here-document — its body or its delimiter — which is what puts a
	// newline at the end of the entry. See String.
	endsInBody bool
	// spaceNext makes the next separator a space, whatever the text says.
	spaceNext bool
}

// Add takes one physical line, with what the parser was still inside when it
// asked for it — [syntax.Parser.OpenQuote]'s spelling, empty for a line that
// begins a command or continues one outside any quote.
func (e *Entry) Add(line, open string) {
	if len(e.lines) > 0 {
		e.seps = append(e.seps, e.separator(open))
	}
	e.lines = append(e.lines, line)
	// A line read inside a here-document is the one kind that ends an entry
	// with a newline, and only where the command *ends* there.
	e.endsInBody = open == heredocOpen
}

// SpaceNext makes the separator before the next line a space.
//
// One thing asks for it: a `:p` line inside a compound command, which is in
// the entry and was never handed to the parser — measured, bash writes
// `if true; then echo echo abc echo inside; fi` there, with no `;` after the
// line it did not run.
func (e *Entry) SpaceNext() { e.spaceNext = true }

// Len is how many physical lines have been collected.
func (e *Entry) Len() int { return len(e.lines) }

// String is the one entry those lines make.
//
// A command holding a here-document keeps its newlines and ends with one,
// which is measured: `cat <<EOD` / `body` / `EOD` comes back out of `history`
// over three lines with a blank one after it. Nothing else can — a `;` inside
// a here-document's body would be body text.
func (e *Entry) String() string {
	var b strings.Builder
	for i, line := range e.lines {
		if i > 0 {
			b.WriteString(e.seps[i-1])
		}
		b.WriteString(line)
	}
	if e.heredoc && e.endsInBody {
		// Only where the command *ends* on the document's last line.
		// Measured: `echo $(cat <<EOF` / `x` / `EOF` / `)` comes back as the
		// four lines with no blank one after them, because the `)` that
		// closed the command was read after the document had ended.
		b.WriteString("\n")
	}
	return b.String()
}

// Take is the entry, with the collector emptied for the next command.
func (e *Entry) Take() string {
	entry := e.String()
	*e = Entry{}
	return entry
}

// heredocOpen is how the lexer spells a here-document's body as the thing it
// is still inside.
const heredocOpen = "<<"

// separator is what goes between the line already collected and the one
// arriving, which open says was read inside a quote or a here-document.
func (e *Entry) separator(open string) string {
	if e.spaceNext {
		e.spaceNext = false
		return " "
	}
	if open != "" {
		if open == heredocOpen {
			e.heredoc = true
		}
		return "\n"
	}
	last := strings.TrimRight(e.lines[len(e.lines)-1], " \t")
	if last == "" {
		// A blank line inside a command contributes nothing that a `;` could
		// follow. Measured: `if true` / (blank) / `then` / `echo hi` / `fi`
		// comes back as `if true;  then echo hi; fi`, with the two spaces
		// that says the blank was kept and the semicolon was not doubled.
		return " "
	}
	if strings.HasSuffix(last, ")") && e.unclosedParen() {
		// A `case` pattern, which is the one `)` that is still waiting for a
		// command. Measured: `case foo in` / `foo)` / `echo one two` / `;;` /
		// `esac` comes back as `case foo in foo) echo one two; ;; esac`, with
		// no `;` after the pattern.
		//
		// The balance rather than the character, because a line ending in `)`
		// is far more often a substitution that closed, and that one *does*
		// take a `;`. Two shapes measured, and each is why one half of this
		// test is there: `if true; then` / `echo $(echo x)` / `fi` is `echo
		// $(echo x); fi`, so the character alone is not enough; and `echo
		// $((1 +` / `2))` / `echo after` is `2)); echo after`, so the balance
		// has to be read over the whole command and not over the line, where
		// `2))` looks unmatched on its own.
		return " "
	}
	for _, word := range awaitingACommand {
		if strings.HasSuffix(last, word) {
			// A word only counts as a word: `dado` does not end in `do`.
			head := last[:len(last)-len(word)]
			if isWordish(word) && head != "" && !isSeparatorByte(head[len(head)-1]) {
				continue
			}
			return " "
		}
	}
	return "; "
}

// unclosedParen reports whether the command collected so far has more `)`
// than `(` — which a case pattern does and a closed substitution does not.
func (e *Entry) unclosedParen() bool {
	opens, closes := 0, 0
	for _, line := range e.lines {
		opens += strings.Count(line, "(")
		closes += strings.Count(line, ")")
	}
	return closes > opens
}

// awaitingACommand are the line endings that take a space rather than a `;`,
// longest spelling first so that `||` is not read as `|`.
var awaitingACommand = []string{"&&", "||", ";;", "|", "{", "(", "then", "else", "do", "in"}

func isWordish(word string) bool {
	return word[0] >= 'a' && word[0] <= 'z'
}

func isSeparatorByte(b byte) bool {
	return b == ' ' || b == '\t' || b == ';' || b == '&' || b == '|' || b == '(' || b == '{'
}
