// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// recursiveEdit is zsh's `recursive-edit`: the line is edited in a read loop
// of its own, from inside the widget that asked, until it is accepted or
// broken off, and then the widget goes on with the line as it was left.
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, from a
// widget on the line `ab` that calls it and then reports, typing `cd` and an
// ending key:
//
//	Return   status 0, the line `abcd`, `$KEYS` ^M, and the line not run
//	^G       status 1, `$KEYS` ^G
//	^C       status 1, `$KEYS` empty
//
// and in each case the outer line goes on being edited afterwards (#5899).
//
// The loop is the editor's own — keyLoop, the same one readLine runs — so a
// key does in here what it does anywhere: accepting and breaking off are the
// only two things that end differently, and each of them asks recursive
// rather than being told by the caller.
func (e *editor) recursiveEdit(prompt drawnPrompt) int {
	// The keystroke that ran the widget is still the one being handled out
	// here, whatever keys the edit reads: put back afterwards, so the widget
	// that called this is what the next key finds as the last one. The keys
	// are not put back — they are the ending key, which is what `$KEYS` says.
	binding := e.keyBinding
	e.recursive++
	e.recursiveBroke = false
	_, err := e.keyLoop(prompt)
	e.recursive--
	e.keyBinding = binding
	if err != nil || e.recursiveBroke {
		e.recursiveBroke = false
		return 1
	}
	return 0
}
