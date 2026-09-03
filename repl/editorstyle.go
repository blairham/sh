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
}
