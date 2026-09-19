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
		// Nothing to draw. The shell being measured rings the bell here; this
		// editor rings it nowhere — a Tab that matches nothing is silent too
		// — and ringing it for this one action alone would be a difference
		// between two keys rather than a bell.
		e.redraw(prompt)
		return
	}
	shown := displayCandidates(matches, word)
	if e.confirmList(shown, prompt) {
		e.list(shown, prompt)
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
		e.change(false, e.deleteForward)
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
// It draws no listing. The shell being measured drew one on the first press,
// and that listing is not the menu's: with `unsetopt autolist` the same
// keystroke inserted `uniq_alpha` and drew nothing, while `list-choices` went
// on listing. A menu implemented as "complete and also list" would have passed
// the first probe and been wrong about what the action is.
func (e *editor) menuComplete(c Completer, step int, prompt drawnPrompt) {
	if e.menu.before && len(e.menu.words) > 1 {
		n := len(e.menu.words)
		e.menu.index = (e.menu.index + step + n) % n
		e.change(false, func() { e.replaceWord(e.menu.start, e.menu.words[e.menu.index]) })
		e.menu.now = true
		e.redraw(prompt)
		return
	}
	start, word, matches := e.candidates(c)
	words := insertableWords(matches)
	switch len(words) {
	case 0:
	case 1:
		e.change(false, func() { e.replaceWord(start, words[0]+completionSuffix(word, words[0])) })
	default:
		first := 0
		if step < 0 {
			first = len(words) - 1
		}
		e.menu = menuWalk{now: true, words: words, index: first, start: start}
		e.change(false, func() { e.replaceWord(start, words[first]) })
	}
	e.redraw(prompt)
}
