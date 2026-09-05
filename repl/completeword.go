// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// The word under the cursor, taken apart the way completion needs it.
//
// Three questions, and they are the same three every time Tab is pressed:
// where does the word begin, what is the literal text it names once the
// quoting is undone, and how does a name have to be written so that the
// parser reads back the file that was chosen. `docs/spec/completion.md` has
// the measurements behind the answers.

// wordBreak reports whether an unquoted rune ends the word under the cursor.
//
// Whitespace and the characters the grammar itself ends a word with. bash
// breaks at more than this — readline's set also holds `@ = : " '` — and the
// price is that `--opt=value` and `host:path` split in the middle of a path
// someone is typing. zsh breaks at these and no more, which is also the
// answer the core grammar gives on its own.
func wordBreak(r rune) bool {
	switch r {
	case ' ', '\t', '\n', ';', '|', '&', '<', '>', '(', ')':
		return true
	}
	return false
}

// wordSpan finds where the word under the cursor begins and which quote, if
// any, is still open at the cursor.
//
// It scans forward from the start of the line rather than backward from the
// cursor, because whether a space ends a word is a question about everything
// to its left: the space in `"a b` is inside a quotation and the one in `a b`
// is not, and nothing local to the space says which.
//
// A word therefore always begins unquoted — an open quotation suppresses the
// break that would have started a new word — which is what lets the rest of
// this file take a word's text alone and know the quoting inside it.
func wordSpan(line []rune, pos int) (start int, quote rune) {
	for i := 0; i < pos; {
		r := line[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			}
			i++
		case quote == '"':
			// Inside double quotes a backslash escapes only a few
			// characters, but it never *ends* one, so skipping the pair is
			// right for the purpose of finding the word.
			if r == '\\' && i+1 < pos {
				i += 2
				continue
			}
			if r == '"' {
				quote = 0
			}
			i++
		case r == '\\':
			i += 2
		case r == '\'' || r == '"':
			quote = r
			i++
		case wordBreak(r):
			i++
			start = i
		default:
			i++
		}
	}
	return start, quote
}

// wordStart is where the word under the cursor begins.
func wordStart(line []rune, pos int) int {
	start, _ := wordSpan(line, pos)
	return start
}

// wordQuote is the quotation still open at the end of a word.
//
// Taken from the word's own text, which is enough because a word begins
// unquoted; see wordSpan.
func wordQuote(word string) rune {
	_, q := wordSpan([]rune(word), len([]rune(word)))
	return q
}

// dequote is the literal text a typed word names: quotes removed, escapes
// applied. It is what gets matched against what is on the disk.
func dequote(word string) string {
	var b strings.Builder
	var quote rune
	r := []rune(word)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				b.WriteRune(c)
			}
			i++
		case quote == '"':
			if c == '\\' && i+1 < len(r) && escapedInDoubleQuotes(r[i+1]) {
				b.WriteRune(r[i+1])
				i += 2
				continue
			}
			if c == '"' {
				quote = 0
			} else {
				b.WriteRune(c)
			}
			i++
		case c == '\\':
			if i+1 < len(r) {
				b.WriteRune(r[i+1])
			}
			i += 2
		case c == '\'' || c == '"':
			quote = c
			i++
		default:
			b.WriteRune(c)
			i++
		}
	}
	return b.String()
}

// escapedInDoubleQuotes reports whether a backslash before this rune means
// the rune itself. Inside double quotes a backslash is otherwise an ordinary
// character, which is why `"a\ b"` is a name with a backslash in it.
func escapedInDoubleQuotes(r rune) bool {
	switch r {
	case '"', '\\', '`', '$', '\n':
		return true
	}
	return false
}

// wordPrefix is the part of a typed word that every candidate keeps
// unchanged: the directory already typed, along with an opening quote.
//
// It is taken from the text as typed rather than rebuilt from the literal
// path, so `~/Deve` completes to `~/Developer/` and not to the home
// directory spelled out — which is what both shells do, and what someone who
// typed a tilde meant.
func wordPrefix(word string) string {
	if i := lastPathSeparator(word); i >= 0 {
		return word[:i+1]
	}
	if strings.HasPrefix(word, "'") || strings.HasPrefix(word, `"`) {
		return word[:1]
	}
	return ""
}

// lastPathSeparator is the index of the final unescaped `/` in a typed word,
// or -1. A `\/` is not one: it is a slash someone escaped, and cutting the
// word there would take the backslash for the directory and leave the slash
// with the name.
// A slash inside single quotes is a separator like any other: single quotes
// take away the backslash's meaning, not the slash's.
func lastPathSeparator(word string) int {
	last, quote := -1, byte(0)
	for i := 0; i < len(word); i++ {
		switch c := word[i]; {
		case quote == '\'':
			switch c {
			case '\'':
				quote = 0
			case '/':
				last = i
			}
		case quote == '"':
			switch {
			case c == '\\' && i+1 < len(word):
				i++
			case c == '"':
				quote = 0
			case c == '/':
				last = i
			}
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			quote = c
		case c == '/':
			last = i
		}
	}
	return last
}

// hasPathSeparator reports whether a word names a path rather than a bare
// name — which is what makes a command word complete against the filesystem
// instead of against PATH.
func hasPathSeparator(word string) bool { return lastPathSeparator(word) >= 0 }

// escapeName writes a name so that reading the line back gives the name
// again.
//
// What has to be escaped depends on the quotation the word is already
// inside, which is the whole reason this takes one: a single quote inside
// double quotes is an ordinary character, and a dollar sign is not.
//
// atStart says the name begins the word, which is the only place `~` and `#`
// mean anything.
func escapeName(name string, quote rune, atStart bool) string {
	switch quote {
	case '\'':
		// Only the quote itself is special, and it cannot be escaped inside
		// its own quotation: the quotation is closed, the quote is written
		// with a backslash, and the quotation is opened again.
		return strings.ReplaceAll(name, "'", `'\''`)
	case '"':
		var b strings.Builder
		for _, r := range name {
			if escapedInDoubleQuotes(r) && r != '\n' {
				b.WriteRune('\\')
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	var b strings.Builder
	for i, r := range name {
		if specialUnquoted(r) || (i == 0 && atStart && (r == '~' || r == '#')) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// specialUnquoted reports whether a rune has to carry a backslash when it
// appears in an unquoted word.
//
// Everything the grammar reads as something other than itself: the quoting
// characters, the expansions, the operators, the pattern characters and the
// whitespace that would end the word. `!` is in it because both shells put a
// backslash there, and it costs nothing to agree.
func specialUnquoted(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\\', '\'', '"', '`', '$', '&', ';', '|',
		'<', '>', '(', ')', '*', '?', '[', ']', '{', '}', '!':
		return true
	}
	return false
}

// escapeUnits splits escaped text into the pieces that may not be cut in
// half: a backslash and what it escapes are one piece.
//
// It is what makes the common prefix of several matches safe. `x\$a` and
// `x\&b` agree on `x` and on nothing else; a prefix taken rune by rune would
// hand back `x\`, and a trailing backslash escapes whatever is typed next.
func escapeUnits(s string) []string {
	r := []rune(s)
	out := make([]string, 0, len(r))
	for i := 0; i < len(r); i++ {
		if r[i] == '\\' && i+1 < len(r) {
			out = append(out, string(r[i:i+2]))
			i++
			continue
		}
		out = append(out, string(r[i]))
	}
	return out
}
