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
// Two of the physical lines are not lines of the entry at all. A line the
// reader **joined to the next one** — one ending in a backslash, outside a
// quote — is half of one line and not a line of its own, so the pair is
// written with the backslash, the newline and any separator all gone: `echo
// \` / `A` is `echo A`. And a line that is only a **comment** ran nothing, so
// it is dropped and leaves a newline behind it for the line after: `for i in
// a b` / `# mid` / `do` is `for i in a b` ⏎ `do`. Both were measured against
// the real list, and both matter beyond the cosmetic — `echo \; A` and `for
// i in a b; # mid; do …` are each an entry that does something other than
// what ran when it is run again.
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

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

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
	// pending is what a line that ran nothing left behind for the next
	// separator to use. See join.
	pending join
}

// join is the separator a line contributing no command leaves behind it.
//
// A comment line and a blank line are both lines the parser got no command
// out of, and the panel writes a different separator for each — measured on
// bash 5.3.20, one `for` loop at a time with the lines between `for i in a b`
// and `do` varied:
//
//	(blank)                 -> `for i in a b;  do …`
//	(blank) (blank)         -> `for i in a b;  do …`
//	# c                     -> `for i in a b` ⏎ `do …`
//	# c  # c                -> `for i in a b` ⏎ `do …`
//	# c  (blank)            -> `for i in a b do …`
//	(blank) # c             -> `for i in a b; ` ⏎ `do …`
//	(blank) # c (blank)     -> `for i in a b;  do …`
//
// Seven rows and one rule: the **first** blank of a run is an empty line in
// the entry and the ones after it are not, a comment is never a line of the
// entry at all, and whichever kind came last decides the separator the next
// real line is written with. A model where a comment leaves a newline that
// the following blank then overrides is the only one all seven agree with —
// rows five and six are the pair that rules out both "collapse the run" and
// "each line writes its own separator".
type join uint8

const (
	// joinNone is the ordinary state: the separator comes from the text.
	joinNone join = iota
	// joinSpace is what a blank line leaves.
	joinSpace
	// joinNewline is what a comment leaves — a `;` after a comment would be
	// commented out, so it cannot be one.
	joinNewline
)

// At is what the reader was inside when it took one physical line, and the
// grammar it was reading under.
//
// A struct rather than three parameters, so that the next fact about a line
// has one place to go: this already grew one — the second half of the
// here-document question — and the growth was a signature change at every
// caller.
type At struct {
	// Open is [syntax.Parser.OpenQuote]'s spelling of what the parser was
	// still inside when it asked for this line. Empty for a line that
	// begins a command or continues one outside any quote.
	Open string
	// HeredocExpands is [syntax.Parser.OpenHeredocExpands]: the
	// here-document Open names was opened with an unquoted delimiter, so
	// its body is shell text and the reader resolves a line continuation in
	// it. Measured, the entry holds what the reader made of it — `cat
	// <<EOD` / `x\` / `y` / `EOD` comes back with `xy` on one line — where
	// the same document under `<<'EOD'` keeps both lines and the backslash.
	HeredocExpands bool
	// Dialect is the grammar this line was read under, which is what says
	// whether a `#` opens a comment at all.
	Dialect syntax.Dialect
}

// resolvesAContinuation reports whether the reader joined a line ending in a
// backslash to the one after it, rather than leaving both as they were.
func (a At) resolvesAContinuation() bool {
	return a.Open == "" || a.Open == heredocOpen && a.HeredocExpands
}

// Add takes one physical line, with what the reader was inside when it took
// it.
//
// A line the parser got no command out of is not always a line of the entry.
// A **comment** inside a compound command is dropped and leaves a newline
// behind it: measured, `for i in a b` / `# mid` / `do` / `echo $i` / `done`
// is `for i in a b` ⏎ `do echo $i; done` in bash 5.3.20, where keeping the
// comment and joining it with a `;` would record `for i in a b` followed by
// text that is commented out — an entry that hangs waiting for a `do` when it
// is run again. A comment standing where a command could begin is an entry of
// its own and reaches this as the first line of one, so it is only dropped
// with a command already collected. See [join] for the blank-line rows.
func (e *Entry) Add(line string, at At) {
	kind := lineOrdinary
	if at.Open == "" {
		// Inside a quote or a here-document body a `#` is text and a blank
		// line is body, so neither question is asked there.
		kind = classify(line, at.Dialect)
	}
	if len(e.lines) > 0 {
		switch kind {
		case lineCommentOnly:
			e.pending = joinNewline
			return
		case lineBlank:
			if e.pending != joinNone {
				e.pending = joinSpace
				return
			}
		}
		e.seps = append(e.seps, e.separator(at))
	}
	e.lines = append(e.lines, line)
	// A line read inside a here-document is the one kind that ends an entry
	// with a newline, and only where the command *ends* there.
	e.endsInBody = at.Open == heredocOpen
	switch kind {
	case lineBlank:
		e.pending = joinSpace
	case lineCommentOnly, lineEndsInComment:
		e.pending = joinNewline
	default:
		e.pending = joinNone
	}
}

// lineKind is what one physical line contributes to the entry.
type lineKind uint8

const (
	// lineOrdinary is a line carrying text a command is made of.
	lineOrdinary lineKind = iota
	// lineBlank is a line of nothing but blanks.
	lineBlank
	// lineCommentOnly is a line that is a comment and nothing else.
	lineCommentOnly
	// lineEndsInComment is a line carrying text with a comment after it —
	// `if true # c`. The text is the entry's; the comment is why the line
	// after it cannot be joined with a `;`. Measured: bash 5.3.20 records
	// `if true # c` ⏎ `then echo hi; fi`.
	lineEndsInComment
)

// classify reads one line the way the shell read it.
//
// Asked of [syntax.ShellWords] rather than answered here, which is the rule
// the gate already states for the quote a line begins inside: a `#` is a
// comment only where a word could begin, so `a#b` is one word and `echo "a #
// b"` is two, and a scanner written here would be a second answer to a
// question the lexer already has. The `#` test in front of it is not a second
// answer — a line holding no `#` anywhere cannot hold a comment, and that is
// almost every line.
func classify(line string, d syntax.Dialect) lineKind {
	if strings.TrimRight(line, " \t") == "" {
		return lineBlank
	}
	if !strings.Contains(line, "#") || d.Comments == syntax.CommentsOrdinaryText {
		return lineOrdinary
	}
	words := syntax.ShellWords(line, d, syntax.ShellSplit{
		Comments:       syntax.CommentsKept,
		NewlineIsBlank: true,
	})
	if len(words) == 0 || !strings.HasPrefix(words[len(words)-1], "#") {
		return lineOrdinary
	}
	if len(words) == 1 {
		return lineCommentOnly
	}
	return lineEndsInComment
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
// arriving, which at says was read inside a quote or a here-document.
func (e *Entry) separator(at At) string {
	if e.spaceNext {
		e.spaceNext = false
		return " "
	}
	switch e.pending {
	case joinSpace:
		return " "
	case joinNewline:
		return "\n"
	}
	if at.resolvesAContinuation() && syntax.EndsWithContinuation(e.lines[len(e.lines)-1]) {
		// The reader joined these two physical lines into one before the
		// parser saw either, so the entry holds one line and not two:
		// measured, bash 5.3.20 records `echo \` / `A` as `echo A` and `echo
		// one \` / `two three` as `echo one two three`. Joining them with a
		// `;` records a command nobody ran — `echo \` and then `A`.
		//
		// Only where the reader resolves it, which is outside a quote and
		// inside a here-document whose delimiter was not quoted. Inside a
		// quote the backslash and the newline both stay: `echo "a\` / `b"`
		// comes back over two lines with the backslash still on the first,
		// in a shell that nonetheless prints `ab`. A here-document under a
		// **quoted** delimiter keeps them for the same reason — the reader
		// resolved nothing there either (#4103).
		last := e.lines[len(e.lines)-1]
		e.lines[len(e.lines)-1] = last[:len(last)-1]
		return ""
	}
	if at.Open != "" {
		if at.Open == heredocOpen {
			e.heredoc = true
		}
		return "\n"
	}
	// A blank line does not reach here — it leaves joinSpace behind it, and
	// the switch above is what answers for it.
	last := strings.TrimRight(e.lines[len(e.lines)-1], " \t")
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
