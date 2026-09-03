// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// EditorStyle is what a dialect draws while a line is being typed, as opposed
// to what it draws in the prompt.
//
// Separate from PromptStyle because it answers a different question. A prompt
// is text the shell was given and transforms; this is text the shell adds of
// its own accord, and the dialects disagree about whether to add it at all.
type EditorStyle struct {
	// Interrupt is what marks a line abandoned with ^C, drawn where the
	// cursor was before the line ends.
	//
	// Two of the four draw `^C` and two draw nothing. Measured with the
	// output drained after every keystroke — without that the terminal
	// flushes what it has queued when the interrupt arrives, and the
	// measurement is of a screen that never existed:
	//
	//	bash    P> echo hi^C
	//	dash    P> echo hi^C
	//	zsh     P> echo hi
	//	ksh93   P> echo hi
	//
	// In dash it is not the shell's doing at all — dash has no line editor,
	// so the terminal echoes the interrupt character itself. A shell that
	// takes the terminal raw has to draw it deliberately or not, which is
	// what makes this a choice rather than something inherited.
	//
	// Empty draws nothing, which is two of the four dialects' answer and
	// also what a front end told nothing does.
	Interrupt string

	// ListQuery is what is asked before printing a large number of matches,
	// instead of printing them. Two verbs: how many matches there are, and
	// how many rows they would take.
	//
	//	bash   Display all 120 possibilities? (y or n)
	//	zsh    zsh: do you wish to see all 120 possibilities (10 lines)?
	//
	// One of them counts the rows and the other does not, which is why both
	// numbers are passed and a wording may ignore the second.
	//
	// Empty asks nothing and prints, which is what the two dialects with no
	// line editor of their own have no answer about, and what a front end
	// told nothing does.
	ListQuery string

	// ListQueryEchoesTheKey writes the key that answered the question back
	// to the screen. zsh does; bash does not.
	ListQueryEchoesTheKey bool

	// ListQueryAcceptsOnlyYesOrNo keeps asking until one of them arrives,
	// ringing the bell at anything else. bash does. zsh takes the first key
	// whatever it is and treats everything but `y` as no.
	ListQueryAcceptsOnlyYesOrNo bool
}
