// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "unicode"

// A word's case changed from the cursor on, and two words swapped — `M-u`,
// `M-l`, `M-c` and `M-t` in zsh's emacs keymap (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// the line and cursor read back with a widget calling `zle -M`. A word is the
// one the motion keys use: letters, digits and the dialect's word characters.
//
//	line          cursor   key              line after       cursor
//	echo abc def  7 (c)    M-u              echo abC def     8
//	echo abc def  4        M-u              echo ABC def     8     ← the next word
//	echo abc def  12       M-u              unchanged        12
//	echo aBC dEF  6 (B)    M-c              echo aBc dEF     8
//	echo aBC dEF  8        M-c              echo aBC Def     12
//	3ab, _ab      0        M-c              3Ab, _Ab               ← first letter
//	foo-bar       0        M-c              Foo-bar                ← `-` is a word char
//	aa bb cc dd   0        ESC 2 M-u        AA BB cc dd      5
//	aa bb cc      6        ESC - M-u        aa bb CC         6     ← cursor stays
//	ab cd         0        ESC 0 M-c        unchanged        0
//
// So each of a count's words is the non-word characters up to it and then the
// word, and the case goes over all of it — punctuation has none to change. A
// negative count does as many words forward and leaves the cursor where it
// was, and nought does nothing.
//
// transpose-words, same apparatus, `aa bb cc` (cursor at each position):
//
//	cursor        line after       cursor
//	0, 1          unchanged        unchanged  ← no word before `aa`
//	2, 3, 4       bb aa cc         5          ← 2 is the blank: the next word
//	5 … 8         aa cc bb         8
//
//	aa bb cc dd   11   ESC 2 M-t   aa dd cc bb    11  ← dd and the second
//	                                                    word back swap; the
//	                                                    words between stay
//	aa bb cc dd   11   ESC 9 M-t   unchanged      11
//	aa bb cc dd   4    ESC 0 M-t   unchanged      5   ← a word swapped with itself
//	aa bb cc dd   6    ESC - M-t   aa cc bb dd    6   ← cursor stays
//	aa bb ␠␠      7    M-t         bb aa ␠␠       5   ← nothing after: the one before
//
// The word is the one under the cursor, else the next one, else the last one
// before it; the other is the count's word back from it; what lies between
// stays where it was. The cursor goes to the end of the later of the two, and
// a negative count does the same swap and leaves it alone.

// wordCase is which case caseWords puts a word in.
type wordCase int

const (
	upperCase wordCase = iota
	lowerCase
	// capitalCase is the first letter of each word upper and the rest lower.
	// The first *letter*: a digit or a word character before it is passed
	// over and the letter after it still counts as first (`3ab` is `3Ab`).
	capitalCase
)

// countAsGiven is the count the keystroke or the call carries, and one where
// there is none. Nought is kept: for the actions that read it, a count of
// nought does nothing or does the action on nothing.
func (e *editor) countAsGiven() int {
	if e.keyNumeric == nil {
		return 1
	}
	return *e.keyNumeric
}

// caseWords changes the case of n words from the cursor on. See the table
// above for what a word is here and what a negative count does.
func (e *editor) caseWords(n int, to wordCase) {
	stay := n < 0
	if stay {
		n = -n
	}
	if stay && e.caseBackward {
		e.caseWordsBackward(n, to)
		return
	}
	i := e.pos
	for range n {
		for i < len(e.line) && !e.isWordRune(e.line[i]) {
			i++
		}
		first := true
		for i < len(e.line) && e.isWordRune(e.line[i]) {
			r := e.line[i]
			switch {
			case to == upperCase:
				r = unicode.ToUpper(r)
			case to == lowerCase:
				r = unicode.ToLower(r)
			case first && e.capitalizeFirstCharacter:
				// The first character, whatever it is: `3AB` is `3ab`. See
				// EditorStyle.CapitalizeTakesTheFirstCharacter.
				r, first = unicode.ToUpper(r), false
			case unicode.IsLetter(r) && first:
				r, first = unicode.ToUpper(r), false
			case unicode.IsLetter(r):
				r = unicode.ToLower(r)
			}
			e.line[i] = r
			i++
		}
	}
	if !stay {
		e.pos = i
	}
}

// caseWordsBackward changes the case of the n words before the cursor — from
// the start of the nth word back to the cursor — and leaves the cursor where it
// was. See EditorStyle.NegativeCaseCountGoesBackward.
func (e *editor) caseWordsBackward(n int, to wordCase) {
	from := e.pos
	for range n {
		for from > 0 && !e.isWordRune(e.line[from-1]) {
			from--
		}
		for from > 0 && e.isWordRune(e.line[from-1]) {
			from--
		}
	}
	first := true
	for i := from; i < e.pos; i++ {
		r := e.line[i]
		if !e.isWordRune(r) {
			first = true
			continue
		}
		switch {
		case to == upperCase:
			r = unicode.ToUpper(r)
		case to == lowerCase:
			r = unicode.ToLower(r)
		case first && (e.capitalizeFirstCharacter || unicode.IsLetter(r)):
			r, first = unicode.ToUpper(r), false
		default:
			r = unicode.ToLower(r)
		}
		e.line[i] = r
	}
}

// transposeWords swaps the word at the cursor with the one n words before it,
// and reports whether there was one to swap it with. See the table above for
// which word is "at the cursor".
func (e *editor) transposeWords(n int) bool {
	if e.transposeWordsCount {
		return e.transposeWordsAsReadline(n)
	}
	stay := n < 0
	if stay {
		n = -n
	}
	start, end, ok := e.wordAtCursor()
	if !ok {
		return false
	}
	if e.transposeToLineEnd && end <= e.pos {
		// Nothing after the cursor is a word, so the word is the last one
		// and everything after it. See
		// EditorStyle.TransposeWordsReachesTheLineEnd.
		end = len(e.line)
	}
	if n == 0 {
		// The word swapped with itself: nothing moves but the cursor.
		e.pos = end
		return true
	}
	// The other word, n back from this one.
	s1, e1 := start, end
	for range n {
		i := s1
		for i > 0 && !e.isWordRune(e.line[i-1]) {
			i--
		}
		if i == 0 {
			return false
		}
		e1 = i
		for i > 0 && e.isWordRune(e.line[i-1]) {
			i--
		}
		s1 = i
	}
	swapped := make([]rune, 0, len(e.line))
	swapped = append(swapped, e.line[:s1]...)
	swapped = append(swapped, e.line[start:end]...)
	swapped = append(swapped, e.line[e1:start]...)
	swapped = append(swapped, e.line[s1:e1]...)
	swapped = append(swapped, e.line[end:]...)
	e.line = swapped
	if !stay {
		e.pos = end
	}
	return true
}

// wordAtCursor is where transpose-words's own word lies: the word the cursor
// is in, or else the next word after it, or else the last word before it.
func (e *editor) wordAtCursor() (start, end int, ok bool) {
	i := e.pos
	for i < len(e.line) && !e.isWordRune(e.line[i]) {
		i++
	}
	if i == len(e.line) {
		// Nothing after the cursor is a word: the last one before it.
		i = e.pos
		for i > 0 && !e.isWordRune(e.line[i-1]) {
			i--
		}
		if i == 0 {
			return 0, 0, false
		}
		end = i
		for i > 0 && e.isWordRune(e.line[i-1]) {
			i--
		}
		return i, end, true
	}
	start = i
	for start > 0 && e.isWordRune(e.line[start-1]) {
		start--
	}
	end = i
	for end < len(e.line) && e.isWordRune(e.line[end]) {
		end++
	}
	return start, end, true
}

// transposeWordsAsReadline is transposeWords under
// EditorStyle.TransposeWordsCountAsReadline, where the rows are.
func (e *editor) transposeWordsAsReadline(n int) bool {
	if n <= 0 {
		// Nought does nothing; a negative count does nothing and rings.
		return n == 0
	}
	start, end, ok := e.wordAtCursor()
	if !ok {
		return false
	}
	if e.transposeToLineEnd && end <= e.pos {
		end = len(e.line)
	}
	s1, e1 := e.wordBefore(start)
	if s1 < 0 {
		s1, e1 = start, end
	}
	for range n - 1 {
		if s, en, ok := e.wordAfter(end); ok {
			start, end = s, en
			continue
		}
		if s, en := e.wordBefore(s1); s >= 0 {
			s1, e1 = s, en
		}
	}
	if s1 == start {
		return false
	}
	swapped := make([]rune, 0, len(e.line))
	swapped = append(swapped, e.line[:s1]...)
	swapped = append(swapped, e.line[start:end]...)
	swapped = append(swapped, e.line[e1:start]...)
	swapped = append(swapped, e.line[s1:e1]...)
	swapped = append(swapped, e.line[end:]...)
	e.line = swapped
	e.pos = end
	return true
}

// wordBefore is where the last word ending at or before i lies, and -1 where
// there is none.
func (e *editor) wordBefore(i int) (start, end int) {
	for i > 0 && !e.isWordRune(e.line[i-1]) {
		i--
	}
	if i == 0 {
		return -1, -1
	}
	end = i
	for i > 0 && e.isWordRune(e.line[i-1]) {
		i--
	}
	return i, end
}

// wordAfter is where the first word starting at or after i lies.
func (e *editor) wordAfter(i int) (start, end int, ok bool) {
	for i < len(e.line) && !e.isWordRune(e.line[i]) {
		i++
	}
	if i == len(e.line) {
		return 0, 0, false
	}
	start = i
	for i < len(e.line) && e.isWordRune(e.line[i]) {
		i++
	}
	return start, i, true
}
