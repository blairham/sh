// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// The three things a completion key can do besides completing: draw the
// matches without touching the line, delete a character where there is one and
// draw them where there is not, and walk the matches one keystroke at a time.
//
// complete.go is the completion itself — what the matches are, and what
// replacing a word with one means. This file is about the *keys*, which is
// where the three differ from each other and from Tab: they ask the same
// question and do different things with the answer.
//
// # Measured
//
// 2026-09-18 through a pseudo-terminal against zsh 5.9.2 under `-i` with a
// scratch rc, in a directory holding `uniq_alpha`, `uniq_beta`, `uniq_gamma`
// and `zzsolo`, each widget bound to a key of its own so that Tab's own
// two-keystroke rule could not be mistaken for the action. The table is in
// widgets.go beside the constants; the two probes that decide the shape are
// repeated below where they decide something.

// listChoices draws the matches for the word under the cursor and leaves the
// line exactly as it is.
//
// The first keystroke lists, and so does the second: there is no
// second-keystroke rule here, because there is nothing for a first keystroke
// to have done. That is the difference from Tab and it is measured rather than
// assumed — `list-choices` pressed twice listed twice, where Tab's first press
// fills in the common prefix and only its second lists.
//
// A single match is listed rather than inserted, which is the other half of
// "without touching the line": `cat zzs` with `zzsolo` the only match drew the
// name and left `cat zzs` on the line.
func (e *editor) listChoices(c Completer, prompt drawnPrompt) {
	_, word, matches := e.candidates(c)
	if len(matches) == 0 {
		// Nothing to draw, and a bell, as for a Tab that matches nothing.
		// Measured 2026-10-06 through a pseudo-terminal: zsh 5.9.2 writes
		// `\a` for `^D` at the end of `echo ab`, and bash 5.3.20 for `M-?`
		// and `M-=` on a word nothing matches (#6247).
		e.ring()
		e.redraw(prompt)
		return
	}
	shown := displayCandidates(matches, word)
	if e.confirmList(shown, prompt) {
		e.returnToTheLine(e.list(shown, prompt), prompt)
	}
	e.redraw(prompt)
}

// deleteCharOrList deletes the character under the cursor, or lists where
// there is no character under it.
//
// The cursor decides and nothing else, the empty line included. That is the
// part worth measuring rather than guessing: this is what `^D` is bound to in
// one of the two shells with an editor, and `^D` on an empty line ends the
// session, so the plausible reading is that the action ends it. Bound to a key
// that is not `^D` and pressed on an empty line, it offered to list all 1064
// commands instead. Ending input belongs to the key, which is why this editor's
// `^D` is still its own case in the key loop and not a binding to this.
func (e *editor) deleteCharOrList(c Completer, prompt drawnPrompt) {
	if e.pos < len(e.line) {
		e.change(false, func() { e.deleteForward() })
		e.redraw(prompt)
		return
	}
	e.listChoices(c, prompt)
}

// menuWalk is a menu completion in flight: the matches it is walking, where in
// them it stands, and where in the line the word it is replacing begins.
//
// now and before are the pair every run-of-keystrokes state in this editor
// keeps — see the key loop, which rolls one into the other before the key
// acts. The walk lasts exactly as long as the keystrokes are adjacent;
// anything else pressed between two of them ends it, measured, and the next
// press starts a fresh completion.
type menuWalk struct {
	now, before bool

	// words is what the walk cycles through, in the order the completer gave
	// them, and index is where it stands. start is the rune index the word
	// being replaced begins at — held because after the first insertion the
	// word under the cursor is a *match*, so recomputing it would walk from
	// `uniq_alpha` rather than from `uniq`.
	words []string
	index int
	start int
	// home is where in words the word the walk began from is, which
	// landing on rings; 0 where the walk does not return to it. See
	// EditorStyle.MenuReturnsToTheWord.
	home int
}

// menuComplete puts the next match in the line, in the given direction.
//
// Three answers, decided by how many matches there are and by whether a walk
// is already going:
//
//   - A walk already going steps by one and wraps. Measured: alpha, beta,
//     gamma, alpha going forward, and gamma, beta going backward — so the
//     first backward press lands on the *last* match rather than on the first.
//   - One match is an ordinary completion, trailing space and all. `cat zzs`
//     became `cat zzsolo `, which is what Tab does with it.
//   - Nothing is nothing.
//
// The listing it draws on the first press is the listing option's and not the
// menu's: with `unsetopt autolist` the same keystroke inserted `uniq_alpha` and
// drew nothing, while `list-choices` went on listing. See menuStarted.
func (e *editor) menuComplete(c Completer, builtin bool, step int, prompt drawnPrompt) {
	if e.stepMenu(step, prompt) {
		return
	}
	if builtin && e.tabOnABlankLine(prompt) {
		return
	}
	var matches []Candidate
	var did completionOutcome
	e.change(false, func() { matches, did = e.completeAs(c, e.menuReason(e.completedBefore, MenuAsked), step) })
	if did == completionStartedAMenu {
		e.menuStarted(matches, prompt)
		return
	}
	if did == completionFoundNothing && e.menuReturns {
		// Nothing to walk: the bell, as readline's menu-complete rings for
		// `zz` matching nothing. See EditorStyle.MenuReturnsToTheWord.
		e.ring()
	}
	e.redraw(prompt)
}

// menuReason is whether a completion key starting now starts a menu, and why,
// given whether the key before it was a completion and what the key itself
// asks for.
//
// The repeat is zsh's AUTO_MENU, and it is not "the second Tab". Measured
// 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `x a` typed over
// `alpha1`, `alpha2` and `zz`, Tab after Tab:
//
//	options             Tab 1      Tab 2              Tab 3
//	(defaults)          `alpha`    the listing        \a `alpha1`
//	unsetopt autolist   `alpha`    \a `alpha1`        `alpha2`
//
// So the menu waits for the listing where there is one to wait for: a repeat
// whose key would be the first to list the matches lists them, and the key
// after it starts the menu. Over `always` and `auto`, where the first Tab has
// nothing to fill in and lists, the second starts it. A completion between
// them that settled the word, or a key that was not a completion, starts the
// count again — `x a`, Tab, `^E`, Tab lists twice.
func (e *editor) menuReason(wasTab bool, asked MenuReason) MenuReason {
	if e.autoMenu && e.completionBefore != completionLeftNothing &&
		(e.completionBefore == completionListedIt || !e.listsOn(wasTab)) {
		return MenuOnARepeat
	}
	if asked != MenuNotStarted || e.menuFirst {
		return MenuAsked
	}
	return MenuNotStarted
}

// completionLeft is what the completion key before this one left behind, which
// is what decides whether this one starts a menu. See menuReason.
type completionLeft uint8

const (
	// completionLeftNothing: the key before was not a completion, or it
	// settled the word or found nothing.
	completionLeftNothing completionLeft = iota
	// completionLeftItAmbiguous: it filled in what the matches agree on, or
	// had nothing to fill in, and drew no listing.
	completionLeftItAmbiguous
	// completionListedIt: it left the word ambiguous and drew the listing.
	completionListedIt
)

// startMenu begins a menu completion over words, which replace the word that
// begins at start: at the match numbered at — see MenuCompletion — or, where
// at is zero, at the first match going forward and the last going back.
func (e *editor) startMenu(start int, words []string, at, step int) {
	n := len(words)
	first := 0
	switch {
	case at > 0:
		first = (at - 1) % n
	case at < 0:
		first = ((at % n) + n) % n
	case step < 0:
		first = n - 1
		if e.menuReturns {
			// The last match, and not the word itself after it.
			first = n - 2
		}
	}
	e.menu = menuWalk{now: true, words: words, index: first, start: start}
	e.replaceWord(start, words[first])
}

// stepMenu moves a menu completion in flight on by step, wrapping, and reports
// whether there was one to move.
func (e *editor) stepMenu(step int, prompt drawnPrompt) bool {
	if !e.menu.before || len(e.menu.words) < 2 {
		return false
	}
	n := len(e.menu.words)
	e.menu.index = (e.menu.index + step + n) % n
	e.change(false, func() { e.replaceWord(e.menu.start, e.menu.words[e.menu.index]) })
	e.menu.now = true
	if e.menu.home > 0 && e.menu.index == e.menu.home {
		// Back at the word the walk began from. See
		// EditorStyle.MenuReturnsToTheWord.
		e.ring()
	}
	e.redraw(prompt)
	return true
}

// menuStarted finishes the keystroke that started a menu completion: the bell,
// where the dialect rings one, and the listing, where the listing option is on
// and the matches are not already drawn.
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `x a` over
// `always` and `auto`: the keystroke that starts a menu writes `\a` and then
// the first match, whichever route started it — the Tab after a listing, a
// first Tab under MENU_COMPLETE, a `menu-complete` widget and a
// `reverse-menu-complete` one — and the keystrokes that walk it write no bell.
// The listing came with it on a first press (MENU_COMPLETE, and the widgets)
// and was left as it stood where a Tab had already drawn it. See
// EditorStyle.BellRingsWhenAMenuStarts.
func (e *editor) menuStarted(matches []Candidate, prompt drawnPrompt) {
	if e.bellsOnAMenu {
		e.ring()
	}
	if e.listsMatches && e.completionBefore != completionListedIt && e.confirmList(matches, prompt) {
		e.returnToTheLine(e.list(matches, prompt), prompt)
	}
	e.completionNow = completionListedIt
	e.redraw(prompt)
}
