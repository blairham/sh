// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// What completion makes of a word at the start of a continuation line, where
// the lines already entered decide it and the line itself cannot (#6242).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, in a
// directory holding one file `zfile`: each command below, Return, then `^D`
// on the empty continuation line.
//
//	entered       ^D at the continuation prompt
//	for x in 1    lists zfile        an argument
//	for x         lists zfile
//	case x in     lists zfile
//	repeat 2      lists zfile
//	echo a \      lists zfile        the line goes on
//	ls |          every command      a command
//	echo a &&     every command
//	{             every command
//	f() {         every command
//	echo $(       every command
//	if true       every command
//	while true    every command
//	echo "a       \a                 inside the quote: nothing matches
//	echo 'a       \a
//
// Tab with `zf` typed agrees: `zfile` after `for x in 1`, nothing inside the
// quote, `zfgrep zforce zformat` after `if true`. bash's completion reads the
// line alone, and gives `zf` at `for x in 1`'s continuation every command it
// matches — so this is EditorStyle.CompletionReadsTheContinuation's, and the
// zero value is the line alone.

// continuationHeaders are the words a line of the table above begins with when
// the newline after it does not start a command: what is expected next is a
// keyword or a pattern, and zsh completes an argument there.
var continuationHeaders = map[string]bool{
	"for": true, "foreach": true, "select": true, "case": true, "repeat": true,
}

// continuationContext reads the lines already entered: whether a word at the
// start of the next line is a command, and whether that word is inside a quote
// the earlier lines opened — where nothing can match it, because the word
// begins at the quote and holds the newline.
func continuationContext(pre string) (command, quoted bool) {
	if openQuote(pre) {
		return false, true
	}
	text := strings.TrimSuffix(pre, "\n")
	if strings.HasSuffix(text, "\\") {
		// A backslash took the newline away, so the line goes on.
		rs := []rune(strings.TrimSuffix(text, "\\"))
		return commandPosition(rs, len(rs)), false
	}
	last := text
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		last = text[i+1:]
	}
	fields := strings.Fields(last)
	if len(fields) > 0 && continuationHeaders[fields[0]] && !strings.ContainsAny(last, ";{}()") {
		for _, f := range fields[1:] {
			if f == "do" {
				return true, false
			}
		}
		return false, false
	}
	// Anything else ended a command with the newline, or with an operator
	// before it, so a command comes next.
	return true, false
}

// openQuote reports whether the text leaves a single or a double quote open.
// A backslash escapes outside single quotes, as everywhere.
func openQuote(s string) bool {
	var q rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && q != '\'':
			escaped = true
		case q == 0 && (r == '\'' || r == '"'):
			q = r
		case r == q:
			q = 0
		}
	}
	return q != 0
}

// continuationAnswers is whether a word that starts at start in the line is
// decided by the lines entered before it: a session that reads them, at a
// continuation prompt, with nothing but blanks before the word on this line.
func (e *editor) continuationAnswers(start int) (pre string, ok bool) {
	if !e.completionReadsContinuation || e.prebuffer == nil {
		return "", false
	}
	for _, r := range e.line[:start] {
		if r != ' ' && r != '\t' {
			return "", false
		}
	}
	pre = e.prebuffer()
	return pre, pre != ""
}
