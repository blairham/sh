// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// A completion listing too tall for the terminal, paged a screenful at a time
// under a prompt of its own, where the session asks for that (#6153).
//
// zsh does this through zsh/complist: with the module loaded and LISTPROMPT
// set — which is what the completion system's `list-prompt` style leaves
// behind — a long listing is not asked about first. It is drawn a screenful
// at a time, and the editor waits for a key under each screenful. Measured
// 2026-10-06 through a pseudo-terminal against zsh 5.9.2, 120 columns by 40
// rows, `x ` and Tab over a thousand names that list in fifty rows of
// twenty, with `LISTPROMPT='%SAt %l %m %p%s'`:
//
//	Tab               the line scrolls off, 39 rows, and `At 39/50 989/1000
//	                  Top` in standout on the last row
//	then Return       one row more, and `At 40/50 990/1000 80%`
//	then Tab          the rest — 11 rows — and the line drawn under them
//	then Tab again    the menu starts: the listing has been drawn
//	then `q`          the listing stops where it is, and `q` is typed into
//	                  the line, drawn under it
//	then ^G           the listing stops, and the key is gone
//
// and a listing that fits — 39 rows under a one-row line on 40 rows — is drawn
// whole and the cursor goes back up to the line, as any listing does. A
// two-row prompt does not change that boundary: only the line's own rows
// count. Without zsh/complist loaded, LISTPROMPT changes nothing and the
// LISTMAX question is asked as before.
//
// The keys are the `listscroll` keymap's defaults, which zshmodules(1)
// lists by widget: the ones that accept a line or move down a line scroll one
// row, the completion widgets scroll a screenful, `send-break` stops and
// drops its key, and every other key stops and is then read as usual. This
// editor never enters the keymap itself — complist.go says why — so a person's
// `bindkey -M listscroll` does not reach the pager, and the defaults are read
// from their bytes here.

// ListScrollView is where a paged listing stands when it stops for a key:
// the prompt drawn there reports it.
type ListScrollView struct {
	// LastLine is the last row drawn, counted from one, and Lines is how
	// many rows the listing has — zsh's `%l`.
	LastLine, Lines int
	// LastMatch is the number of the last match on the last row drawn,
	// counted from one in the order the listing reads, and Matches is how
	// many there are — zsh's `%m`.
	LastMatch, Matches int
	// Top is the first screenful, before anything has scrolled — where zsh's
	// `%p` says `Top` rather than how far down the last row is.
	Top bool
}

// pagesListings reports whether this session pages a long listing.
func (e *editor) pagesListings() bool {
	if e.listScroll == nil {
		return false
	}
	_, ok := e.listScroll(ListScrollView{})
	return ok
}

// listPage is how many rows a screenful of this listing is, or zero where it
// is not paged: where the session does not page, or where the listing and the
// line's own rows fit on the terminal.
func (e *editor) listPage(rows, lineRows int) int {
	height := e.rows()
	if height < 2 || !e.pagesListings() || rows+lineRows <= height {
		return 0
	}
	return height - 1
}

// scrollListing draws a listing a screenful at a time and reports how many of
// its rows it drew. It stops at the end of the listing, or on a key that
// stops it, which is then read as an ordinary key: see the file comment.
func (e *editor) scrollListing(laid listing, page int) int {
	rows := laid.rows
	shown := 0
	show := func(n int) {
		for ; n > 0 && shown < len(rows); n-- {
			e.write(rows[shown])
			e.write(e.newline())
			shown++
		}
	}
	show(page)
	for shown < len(rows) {
		text, _ := e.listScroll(ListScrollView{
			LastLine:  shown,
			Lines:     len(rows),
			LastMatch: laid.last[shown-1],
			Matches:   laid.matches,
			Top:       shown == page,
		})
		e.write(text)
		key, ok := e.listScrollKey()
		// The prompt's row is taken back whatever the key does: the next
		// row of the listing goes there, or the line does.
		e.write("\r" + e.moves().eraseToRowEnd())
		if !ok {
			break
		}
		switch key {
		case scrollLine:
			show(1)
		case scrollPage:
			show(page)
		default:
			return shown
		}
	}
	return shown
}

// scrollKey is what a key does to a paged listing.
type scrollKey uint8

const (
	// scrollStop stops the listing; the key, if it is to be read, has been
	// put back.
	scrollStop scrollKey = iota
	scrollLine
	scrollPage
)

// listScrollKey reads the key a paged listing waits for, and reports what it
// does. ok is false where input has ended.
//
// One row for Return, a line feed, ^N and the down arrow in either of its
// spellings; a screenful for Tab; ^G stops and is dropped; anything else
// stops and is put back to be read as an ordinary key — the bytes of an
// escape sequence that is not the down arrow included.
func (e *editor) listScrollKey() (scrollKey, bool) {
	c, ok := e.waitByte()
	if !ok {
		return scrollStop, false
	}
	switch c {
	case '\r', '\n', ctrlN:
		return scrollLine, true
	case '\t':
		return scrollPage, true
	case 0x07:
		return scrollStop, true
	case esc:
		read := []byte{esc}
		if e.inputPending() {
			if b, ok := e.waitByte(); ok {
				read = append(read, b)
				if (b == '[' || b == 'O') && e.inputPending() {
					if d, ok := e.waitByte(); ok {
						read = append(read, d)
						if d == 'B' {
							return scrollLine, true
						}
					}
				}
			}
		}
		e.pushKeys(string(read))
		return scrollStop, true
	}
	e.pushBack(c)
	return scrollStop, true
}

// waitByte is the next key's byte, serving the shell's watched descriptors
// while it waits as a question does. ok is false where input has ended.
func (e *editor) waitByte() (byte, bool) {
	for {
		e.serveWhileAsking()
		var buf [1]byte
		n, err := e.nextByte(buf[:])
		if err != nil {
			return 0, false
		}
		if n == 1 {
			return buf[0], true
		}
	}
}
