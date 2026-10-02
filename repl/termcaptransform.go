// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
)

// The editor's control sequences under their termcap names, for a shell that
// lets a script stand something else in for them.
//
// zsh's `zle -T tc f` is the case: the manual says the function "is passed the
// termcap code that would be output as its first argument; if the operation
// required a numeric argument, that is passed as a second argument", and what
// it leaves in REPLY is written instead. Carriage returns and newlines are not
// passed through it, and neither are attributes. Measured 2026-10-02 on zsh
// 5.9.2 through a pseudo-terminal with a function writing `<$1:$2>`:
//
//	the erase before the prompt          <cd>
//	the erase after it                   <ce>
//	one column left                      <le>
//	eighteen columns left, then right    <LE:18><RI:18>
//	the attribute resets before <cd>     \e[0m\e[27m\e[24m, untouched
//
// This editor composes its output as strings rather than capability by
// capability, so the names are recovered at the one place every string goes
// out — editor.write — by recognizing the sequences it writes for those
// operations. That is a reading of this editor's own output and not of a
// terminal's: the sequences are the ones this package writes for the
// operations, and nothing else in the stream is touched.

// termcapTransform is TransformTermcap bound to ctx, or nil.
func (s Shell) termcapTransform(ctx context.Context) func(code, arg string) (string, bool) {
	if s.TransformTermcap == nil {
		return nil
	}
	return func(code, arg string) (string, bool) { return s.TransformTermcap(ctx, code, arg) }
}

// termcapMoves names the cursor movements by their final byte: the name of a
// single step and of a counted one.
var termcapMoves = map[byte][2]string{
	'A': {"up", "UP"},
	'B': {"do", "DO"},
	'C': {"nd", "RI"},
	'D': {"le", "LE"},
}

// transformTermcapSequences rewrites every control sequence in s that is one
// of the editor's terminal operations through transform, leaving the rest of
// s as it is. transform's false is "nothing installed", and s then goes out
// unchanged.
func transformTermcapSequences(s string, transform func(code, arg string) (string, bool)) string {
	if !strings.ContainsAny(s, "\x1b\b") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		code, arg, n := termcapAt(s[i:])
		if n == 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		out, ok := transform(code, arg)
		if !ok {
			return s
		}
		b.WriteString(out)
		i += n
	}
	return b.String()
}

// termcapAt recognizes one terminal operation at the start of s: its termcap
// name, its count where it carries one, and how many bytes it is. n is 0 for
// anything else.
func termcapAt(s string) (code, arg string, n int) {
	if s[0] == '\b' {
		return "le", "", 1
	}
	if !strings.HasPrefix(s, "\x1b[") {
		return "", "", 0
	}
	j := 2
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j >= len(s) {
		return "", "", 0
	}
	digits := s[2:j]
	switch final := s[j]; final {
	case 'J':
		if digits == "" {
			return "cd", "", j + 1
		}
	case 'K':
		if digits == "" {
			return "ce", "", j + 1
		}
	case 'A', 'B', 'C', 'D':
		names := termcapMoves[final]
		if digits == "" || digits == "1" {
			return names[0], "", j + 1
		}
		if digits[0] != '0' {
			return names[1], digits, j + 1
		}
	}
	return "", "", 0
}
