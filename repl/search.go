// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Incremental search: `C-r`, `C-s`, and the pattern searches.
//
// The single most-used thing anyone does with a shell history, and the reason
// the file is worth keeping at all. The up arrow walks a list; this asks it a
// question, and the difference matters once the list is longer than a screen.
//
// A mode rather than a key, which is why it has a loop of its own: while it is
// running, an ordinary character extends the query instead of going into the
// line, and every other key means "stop, and do what you normally do". That
// last part is measured and is the part a naive implementation gets wrong —
// `C-r cho C-e` in both bash and zsh leaves search, keeps the line it found,
// *and* moves the cursor to the end. The key is not swallowed by the mode it
// closed. See pushBack — with one exception, a newline, which
// HistoryStyle.SearchNewlineAcceptsTheLine measures.
//
// In its own file because the key it hangs off is one line of editor.go's
// switch, and the mode is a hundred lines that has nothing else to do with
// how a line is drawn.

// The four searches are one mechanism (#5904).
//
// zsh has four incremental searches — backward and forward, each plain or
// with the query read as a pattern — and the editor had only the first. They
// are not four modes but two answers, a direction and a matcher, held by one
// loop: `C-s` inside a `C-r` search turns it round, and a pattern search
// takes `C-r` and `C-s` the same way a plain one does. Measured 2026-10-04
// through a pty against zsh 5.9.2 with `FLOW_CONTROL` off (see below), with
// `echo …alpha one`, `echo …bravo two`, `echo …alpha three`, `echo …charlie`
// in the history, oldest first:
//
//	keys                          shows                       cursor
//	C-r alpha                     alpha three, bck-i-search   start of the match
//	  C-r                         alpha one
//	  C-s                         alpha one, fwd-i-search     end of the match
//	  C-s                         alpha three
//	  C-s                         failing fwd-i-search        line kept
//	  C-r                         alpha three, bck-i-search   start again
//	Up×4 (alpha one), C-s a       bravo two                   after its `A`
//	  l                           alpha three
//
// **The search is a position, not an entry.** It starts at the cursor in the
// line being shown and walks from there: backward to the last match starting
// at or before it, forward to the first starting at or after it, and on into
// the older or newer entries. The line being typed is searched first — `echo
// zz charlie` with `C-r ch` lands on that line's `charlie`, and a second `C-r`
// on its `echo` — and a second match in one entry is a step of its own. bash
// 5.3 answers both rows the same way. Repeating the key steps past the match
// on the screen; turning round, or typing, searches from it, so a match that
// still holds stays.
//
// The cursor is the direction's in zsh: the start of the match going back,
// the end going forward. bash leaves it at the start both ways — `C-r alpha
// C-r C-s` with `stty -ixon` puts it on the `a` — which is
// HistoryStyle.SearchForwardCursorAtMatchEnd.
//
// A pattern search reads the query with the shell's own matcher, unanchored,
// and takes the longest match from where it starts — `v*o` over `bravo two`
// leaves the cursor after `two`. Its wording is the plain search's.
//
// What `C-s` itself is: zsh leaves the terminal's flow control on while
// `FLOW_CONTROL` is set, which is the default, so a `C-s` typed at the prompt
// stops the output and never reaches the editor; bash does the same. This
// editor takes the terminal raw and has always received the key, so here it
// searches — a pre-existing difference, not one this mechanism makes.

// searchDir is which way a search walks: older entries, or newer ones.
type searchDir int

const (
	searchBackward searchDir = -1
	searchForward  searchDir = +1
)

// incrementalSearch runs a search until something ends it.
//
// The line and the cursor are left where the search wants them, and the byte
// that ended the mode — if it means something outside it — is pushed back for
// the caller's own switch to read on its next turn.
//
// How it ended is the answer, for the one caller that can see it: a widget
// that ran the search by name, which is how every plugin wrapping the key
// reaches it (#5895). The key loop has nothing to do with it and drops it.
//
// **Run from inside a widget, this loop is nested and that is safe**, for the
// reason a completion's listing question is: it reads through nextByte, the
// editor's own buffer, and the key loop is waiting on the widget call rather
// than on the terminal, so there is still exactly one reader. The key that
// ends the search is pushed back the same way either route, and is read by
// the key loop once the widget returns — which is when zsh runs it too.
func (e *editor) incrementalSearch(prompt drawnPrompt, dir searchDir, pattern bool) searchEnd {
	// What to put back if the search is abandoned. Both shells restore the
	// line as it was before `C-r`, which is the whole use of `C-g`: a search
	// that cannot be backed out of is one people stop starting.
	saved, savedPos, savedAt := append([]rune(nil), e.line...), e.pos, e.browsing
	// Kept where the arrows can find it, so that a search and the up arrow are
	// one walk rather than two: after `C-r` lands on an entry, Down comes back
	// towards the line that was being typed instead of towards nothing.
	e.drafts[e.browsing] = saved

	s := searchState{
		e: e, typed: e.typedLine(saved), dir: dir, pattern: pattern,
		// The cursor in the line on the screen, which is where the walk
		// starts. An empty query shows that line, measured: `C-r` on a
		// half-typed line draws the search prompt with the line still after
		// it.
		at: e.browsing, off: e.pos,
	}
	e.searchKey = e.searchKey[:0]

	e.drawSearch(prompt, s.query, s.dir, s.failed, s.invalid)
	var buf [1]byte
	for {
		// Through nextByte and not the reader, for the reason confirmList
		// gives: the rest of the search string may already be buffered, and a
		// read of the descriptor would wait for bytes this editor has been
		// given. `C-r` and what follows it arrive in one write from anything
		// but a human.
		n, err := e.nextByte(buf[:])
		if err != nil {
			// The input ended mid-search. The line goes back to what it was,
			// and the caller's own read reports the same error a moment later.
			e.line, e.pos, e.browsing = saved, savedPos, savedAt
			e.redraw(prompt)
			return searchAbandoned
		}
		if n == 0 {
			continue
		}
		switch c := buf[0]; c {
		case ctrlR, ctrlS:
			way := searchBackward
			if c == ctrlS {
				way = searchForward
			}
			if way != s.dir {
				// Turned round: the same query from the match on the screen,
				// which stays when it still matches.
				s.dir = way
				s.seek(true)
				break
			}
			// One more step the same way: past the match on the screen, so a
			// repeated key walks rather than finding the same place again. A
			// failed step keeps the line that did match on the screen and
			// the query as it was: measured, both shells only change the
			// wording and ring the bell, so the next backspace goes back to a
			// search that works.
			s.seek(false)
		case ctrlG:
			e.line, e.pos, e.browsing = saved, savedPos, savedAt
			e.redraw(prompt)
			e.searchKey = append(e.searchKey, c)
			return searchAbandoned
		case ctrlC:
			if !e.searchInterruptAborts {
				e.searchKey = append(e.searchKey, c)
				// The line is abandoned, which is the key loop's to do: the
				// byte goes back for it like any other key that ends the
				// mode. See HistoryStyle.SearchInterruptAbortsTheSearch for
				// the dialect where it does not.
				e.redraw(prompt)
				e.pushBack(c)
				return s.ended()
			}
			// No key at all, measured: zsh's `$KEYS` is empty after a search
			// `C-c` abandoned, where it is `^G` after one `C-g` abandoned.
			e.line, e.pos, e.browsing = saved, savedPos, savedAt
			e.redraw(prompt)
			return searchAbandoned
		case backspace, del:
			if len(s.query) == 0 {
				// Nothing left to take away. A backspace here is not a
				// request to delete from the line — the line is a search
				// result, not something being typed.
				e.write(bell)
				continue
			}
			s.query = s.query[:len(s.query)-1]
			s.seek(true)
		default:
			if c < 0x20 {
				// Every other control byte means the mode is over and the key
				// meant what it always means. Redrawn with the real prompt
				// first, so the search wording is off the screen before the
				// caller draws anything of its own.
				e.redraw(prompt)
				if c != '\n' || e.searchNewlineAccepts {
					// The exception, and it is one key in one dialect: a
					// newline is the *search's* in two of the three columns
					// that have this mode, so the line it found stays on the
					// line and goes on being edited. See
					// HistoryStyle.SearchNewlineAcceptsTheLine, where the
					// three answers are measured.
					e.pushBack(c)
				}
				e.searchKey = append(e.searchKey, c)
				return s.ended()
			}
			r, err := e.readRune(c)
			if err != nil {
				e.line, e.pos, e.browsing = saved, savedPos, savedAt
				e.redraw(prompt)
				return searchAbandoned
			}
			s.query = append(s.query, r)
			s.seek(true)
		}
		e.drawSearch(prompt, s.query, s.dir, s.failed, s.invalid)
	}
}

// searchState is where a search has got to: the query, the way it is
// walking, and the match on the screen as an entry and a rune offset.
type searchState struct {
	e *editor
	// typed is the line that was being edited when the search began, which
	// is the entry one past the newest — the place a backward walk starts
	// from and a forward one can come back to.
	typed   string
	query   []rune
	dir     searchDir
	pattern bool
	// at and off are the match on the screen, or the cursor before there is
	// one. at may be len(history), which is the typed line.
	at, off int
	failed  bool
	// invalid is a pattern query the shell will not compile yet — `t[` on
	// its way to `t[a]`. The line stays and the wording says so; it is
	// neither a match nor a failure. Measured, zsh 5.9.2 draws `invalid
	// bck-i-search: tw[_` with the last match still on the line.
	invalid bool
}

// typedLine is the line that was being typed before any walk began: the line
// on the screen when the walk has not left it, and otherwise what the walk
// kept of it when it did.
func (e *editor) typedLine(onScreen []rune) string {
	if e.browsing >= len(e.history) {
		return string(onScreen)
	}
	return string(e.drafts[len(e.history)])
}

// text is entry i, with the typed line one past the newest.
func (s *searchState) text(i int) string {
	if i >= len(s.e.history) {
		return s.typed
	}
	return s.e.history[i]
}

// seek moves to the next match from where the search is: at or past the match
// on the screen when inclusive, strictly past it otherwise. It rings the bell
// and keeps the screen as it is when there is none.
func (s *searchState) seek(inclusive bool) {
	s.invalid = false
	if s.pattern && s.e.searchMatch != nil && len(s.query) > 0 {
		// Matched against its own text, which walks the matcher through
		// every character of it: a bracket nothing closes is only noticed
		// when the scan reaches it, and an entry that misses on its first
		// character never would.
		q := string(s.query)
		if _, ok := s.e.searchMatch(q, q); !ok {
			s.invalid = true
			return
		}
	}
	i, off, length := s.find(inclusive)
	if i < 0 {
		s.failed = true
		s.e.write(bell)
		return
	}
	s.failed = false
	s.at, s.off = i, off
	e := s.e
	e.line = []rune(s.text(i))
	// At the match rather than at either end, which is measured and is the
	// useful half: the thing that was searched for is where the edit is about
	// to happen. Which end of it is the direction's.
	pos := off
	if s.dir == searchForward && e.searchForwardEndsAtMatchEnd {
		pos = off + length
	}
	e.pos = min(max(pos, 0), len(e.line))
	// The arrows carry on from here rather than from wherever they were when
	// the search started, which is what both shells do and is the only reading
	// that makes sense: the entry on the screen is the one Up goes back from.
	e.browsing = i
}

// find is the next match, as an entry, a rune offset and a length in runes,
// or -1.
func (s *searchState) find(inclusive bool) (int, int, int) {
	if len(s.query) == 0 {
		// An empty query is not a position to walk character by character:
		// it shows the entry it is on, and a step goes to the next entry
		// whole, which is what this editor's `C-r C-r` has always done.
		if inclusive {
			return s.at, s.off, 0
		}
		next := s.at + int(s.dir)
		if next < 0 || next > len(s.e.history) || (s.dir == searchBackward && next >= len(s.e.history)) {
			return -1, 0, 0
		}
		return next, 0, 0
	}
	// The entry on the screen first, from the match or the cursor.
	if off, length, ok := s.within(s.text(s.at), s.off, inclusive); ok {
		return s.at, off, length
	}
	// Then the entries beyond it, each searched whole: its last match going
	// back, its first going forward.
	if s.dir == searchBackward {
		if !s.pattern {
			// The contiguous scan and, behind it, the ranked fallback — the
			// same pair this search has always had. The ranked pass starts
			// from the entry on the screen when the query merely grew, since
			// a subsequence match there is still the best one in reach.
			if i, off := s.e.lastBefore(s.query, s.at-1); i >= 0 {
				return i, off, len(s.query)
			}
			from := s.at - 1
			if inclusive {
				from = s.at
			}
			if i, off := s.e.rankBack(s.query, from); i >= 0 {
				return i, off, len(s.query)
			}
			return -1, 0, 0
		}
		for i := s.at - 1; i >= 0; i-- {
			if off, length, ok := s.within(s.text(i), len([]rune(s.text(i))), true); ok {
				return i, off, length
			}
		}
		return -1, 0, 0
	}
	for i := s.at + 1; i <= len(s.e.history); i++ {
		if off, length, ok := s.within(s.text(i), 0, true); ok {
			return i, off, length
		}
	}
	return -1, 0, 0
}

// within is the match in one entry nearest to from in the search's
// direction: the last starting at or before it going back, the first at or
// after it going forward — strictly so when not inclusive.
func (s *searchState) within(text string, from int, inclusive bool) (int, int, bool) {
	if !s.contains(text) {
		return 0, 0, false
	}
	runes := []rune(text)
	if s.dir == searchBackward && s.pattern && s.e.searchMatch != nil {
		return s.patternBack(runes, from, inclusive)
	}
	if s.dir == searchBackward {
		last := from
		if !inclusive {
			last--
		}
		for i := min(last, len(runes)); i >= 0; i-- {
			if length, ok := s.matchAt(runes, i); ok {
				return i, length, true
			}
		}
		return 0, 0, false
	}
	first := from
	if !inclusive {
		first++
	}
	for i := max(first, 0); i <= len(runes); i++ {
		if length, ok := s.matchAt(runes, i); ok {
			return i, length, true
		}
	}
	return 0, 0, false
}

// contains is whether an entry holds a match anywhere, asked before walking
// its positions so a line that cannot match costs one comparison.
func (s *searchState) contains(text string) bool {
	if s.pattern && s.e.searchMatch != nil {
		matched, _ := s.e.searchMatch("*"+string(s.query)+"*", text)
		return matched
	}
	return strings.Contains(text, string(s.query))
}

// matchAt is whether a match starts at rune i, and how many runes it takes:
// the query's own length for a plain search, and the longest stretch the
// pattern matches for a pattern one.
func (s *searchState) matchAt(runes []rune, i int) (int, bool) {
	if i > len(runes) {
		return 0, false
	}
	rest := runes[i:]
	if !s.pattern || s.e.searchMatch == nil {
		if len(rest) < len(s.query) || string(rest[:len(s.query)]) != string(s.query) {
			return 0, false
		}
		return len(s.query), true
	}
	q := string(s.query)
	if matched, _ := s.e.searchMatch(q+"*", string(rest)); !matched {
		return 0, false
	}
	for j := len(rest); j >= 0; j-- {
		if matched, _ := s.e.searchMatch(q, string(rest[:j])); matched {
			return j, true
		}
	}
	return 0, false
}

// patternBack is a pattern search's match going back, which is the mirror of
// going forward rather than the plain search's rule: the match that **ends**
// last, and of those the one starting first. Measured 2026-10-04 on zsh
// 5.9.2, over `echo …charlie`: `h` lands on charlie's `h` — the last one —
// while `h*e` lands on the `h` of `echo`, and `*two` over `echo …bravo two`
// on the first column. A plain query cannot tell the two rules apart, which
// is why the plain search keeps its own.
func (s *searchState) patternBack(runes []rune, from int, inclusive bool) (int, int, bool) {
	last := from
	if !inclusive {
		last--
	}
	last = min(last, len(runes))
	q := string(s.query)
	for end := len(runes); end >= 0; end-- {
		for start := 0; start <= min(last, end); start++ {
			if matched, _ := s.e.searchMatch(q, string(runes[start:end])); matched {
				return start, end - start, true
			}
		}
	}
	return 0, 0, false
}

// searchEnd is how an incremental search ended.
type searchEnd int

const (
	// searchFound ended on a line the query matched, or on an empty query.
	searchFound searchEnd = iota
	// searchFailing ended while nothing older matched — the line that last
	// did is still on the screen, and kept.
	searchFailing
	// searchAbandoned put the line back as it was before the search.
	searchAbandoned
	// searchInvalid ended on a pattern query the shell would not compile.
	searchInvalid
)

// ended is how a search that a key ended, rather than abandoned, ended.
func (s *searchState) ended() searchEnd {
	if s.invalid {
		return searchInvalid
	}
	return searchEnded(s.failed)
}

func searchEnded(failed bool) searchEnd {
	if failed {
		return searchFailing
	}
	return searchFound
}

// status is the ending as a widget's call of the search answers it.
//
// Measured 2026-10-04 against zsh 5.9.2 through a pseudo-terminal, with a
// widget wrapping `zle .history-incremental-search-backward` the way
// zsh-autosuggestions does and reading `$?` after the call:
//
//	C-r bra Return            0   the match kept, the line then runs
//	C-r Return                0   an empty query is not a failing one
//	C-r bra C-e               0   the match kept, C-e then moves the cursor
//	C-r zzz Return            1   failing: the line as it was, and runs
//	C-r bra C-r (no older)    1   failing: the match kept
//	C-r bra C-g               3   the line put back
//	C-r bra C-c               3   the same, in the dialect where C-c aborts
//
// and the other three searches the same way, measured the same day: forward
// found 0, forward failing 1, forward `C-g` 3, forward `C-e` 0 with `$KEYS`
// `^E`, and a pattern search found 0 and failing 1 — with one number of its
// own, 2, for a pattern search ended while its query would not compile.
//
// Three is zsh's own number and not a convention this package invents: it is
// what the measurement said, on both of the keys that abandon.
func (s searchEnd) status() int {
	switch s {
	case searchFailing:
		return 1
	case searchAbandoned:
		return 3
	case searchInvalid:
		return 2
	}
	return 0
}

// findBack is the entry at or before from that the query names, and where in
// it the match begins.
//
// Two passes, contiguous first. The newest entry that *contains* the query is
// the answer whenever there is one, which is what this editor has always done
// and what the panel shells were measured for; only a query that nothing
// contains reaches the ranked subsequence match in searchrank.go, so `gco`
// finds `git checkout origin/main` where it used to ring the bell (#1311).
//
// The order is what makes the new behavior strictly additive. A query that
// matched before matches the same line, in the same sequence under a repeated
// `C-r`, for the same cost — the ranking pass is not run at all. Putting the
// ranked pass first would have re-decided every search anybody already had.
//
// Rune offsets out, byte offsets in: the cursor sits between characters, and a
// match after a multi-byte one would otherwise be drawn several columns to the
// right of where it is.
// from is an index inside the list or one below the oldest: both callers start
// from either the entry on the screen or the newest, so there is no clamp here
// and no branch that nothing can reach.
func (e *editor) findBack(query []rune, from int) (int, int) {
	if i, off := e.lastBefore(query, from); i >= 0 {
		return i, off
	}
	return e.rankBack(query, from)
}

// lastBefore is findBack's contiguous pass alone: the newest entry at or
// before from that contains the query, and where its last occurrence begins.
//
// The last occurrence, because a backward walk meets it first: in `dup dup`,
// `C-r dup` lands on the second and a further `C-r` on the first — measured
// in zsh 5.9.2 and bash 5.3 alike.
func (e *editor) lastBefore(query []rune, from int) (int, int) {
	q := string(query)
	for i := min(from, len(e.history)-1); i >= 0; i-- {
		if q == "" {
			return i, 0
		}
		if j := strings.LastIndex(e.history[i], q); j >= 0 {
			return i, utf8.RuneCountInString(e.history[i][:j])
		}
	}
	return -1, 0
}

// drawSearch puts the search on the screen, in whichever of the two shapes
// this dialect uses.
func (e *editor) drawSearch(prompt drawnPrompt, query []rune, dir searchDir, failed, invalid bool) {
	wording := or(e.searchPrompt, defaultSearchPrompt)
	if failed {
		wording = or(e.searchFailed, defaultSearchFailed)
	}
	if dir == searchForward {
		wording = or(e.searchForward, defaultSearchForward)
		if failed {
			wording = or(e.searchForwardFailed, defaultSearchForwardFailed)
		}
	}
	if invalid {
		wording = or(e.searchInvalid, defaultSearchInvalid)
		if dir == searchForward {
			wording = or(e.searchForwardInvalid, defaultSearchForwardInvalid)
		}
	}
	// Through drawPrompt, because the editor's arithmetic wants the cells as
	// well as the bytes and a dialect could put a color in its wording.
	text := drawPrompt(fmt.Sprintf(wording, string(query)))
	if !e.searchBelow || e.cols() <= 0 {
		// The prompt is replaced by the search, which is bash's shape and the
		// substrate's own. It is also the only shape available without a
		// terminal width: a second row cannot be placed under a line whose
		// last row is not known.
		e.redraw(text)
		return
	}
	// zsh's shape: the prompt and the line stay, and the search goes on a row
	// below them. Nothing has to erase that row — the next redraw clears to
	// the end of the screen from the prompt's row, which is where this one
	// starts from too — and that is also what takes it away when the search
	// ends.
	e.redraw(prompt)
	e.below(prompt, text.text)
}

// below draws one row of text under the last row of the line and comes back to
// where the cursor was.
func (e *editor) below(prompt drawnPrompt, text string) {
	cols := e.cols()
	end := (prompt.cells + len(e.line)) / cols
	down := end - e.row + 1
	col := (prompt.cells + e.pos) % cols

	var b strings.Builder
	for range down {
		// A newline rather than a cursor-down, so the row exists: moving down
		// past the bottom of the screen does nothing, and the text would land
		// on the line instead of under it.
		b.WriteString("\r\n")
	}
	b.WriteString("\x1b[K")
	b.WriteString(text)
	b.WriteString("\r\x1b[")
	b.WriteString(itoa(down))
	b.WriteString("A")
	if col > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(col))
		b.WriteString("C")
	}
	e.write(b.String())
}

// pushBack keeps a byte for the caller's next read.
//
// One byte is all this mode ever needs, and it is for the reason the mode
// exists: exactly one key ends the search, and it is that key the caller has
// to see. It goes on the front of the same queue pushKeys uses rather than in
// a field of its own, because two places putting input back with two answers
// to "which comes first" is how a pushed key gets read out of order.
func (e *editor) pushBack(c byte) { e.pushed = append([]byte{c}, e.pushed...) }

// pushKeys is the same for an action outside the editor putting characters
// back — see Actions.PushKeys, and editoractions.go for what reaches it.
//
// In front of whatever is already pushed, and that is measured rather than
// convenient: two pushes inside one widget come back newest first, each push's
// own characters in the order they were given. Pushing `ab` and then `cd`
// leaves `cdab` on the line.
func (e *editor) pushKeys(s string) { e.pushed = append([]byte(s), e.pushed...) }

// bell is what a terminal is asked to do about a search that found nothing.
// Both shells ring it, and it is the only report available: there is nowhere
// to put a sentence while a line is being typed.
const bell = "\a"
