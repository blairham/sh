// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A prompt expansion is the fifth site of the boundary mechanism in
// fileabandon.go — `.` and `eval` are the first, a startup file the second, a
// typed line the third, a hook chain the fourth.
//
// The unit given up is the *prompt expansion*. A prompt is a value a shell
// renders on the way to doing something else: drawing a line for a person,
// putting a trace in front of a command, answering `${(%%)v}`, `print -P` or
// `${v@P}` in the middle of a word. An error inside it has no statement of its
// own to cost, which is why it cost the whole script instead — measured, a
// `${(%%)PROMPT}` whose value names a math function nobody registered took the
// rest of the script with it and exited 1, where the shell it models reports
// the same sentence, renders what it had, runs the command and exits 0.
//
// That is not a toy shape. A prompt theme's whole `PROMPT` is parameters, and
// one of them calling a function the theme has not registered yet turns "this
// segment came out empty" into "nothing after this point runs".

// expandPromptText is the substitution pass of a prompt rendering, and the
// boundary an error inside it stops at.
//
// It is [Runner.Expand] with a catch around it, and it is the only route
// [RenderPromptValue] takes: writing the catch at the readers instead would be
// four copies of it, and a prompt reader added later would get none.
//
// What a given-up pass hands back is the dialect's answer and is measured both
// ways — see [PromptStyle.FailedExpansionKeepsWhatItDrew].
func (r *Runner) expandPromptText(text string) string {
	if text == "" {
		return ""
	}
	out, head, ok := r.expandRawSpans(text)
	if ok {
		return out
	}
	if !r.giveUpThePromptExpansion() {
		// A request to stop, or the one operand a dialect reads as one. The
		// unwinding stands and the value is not going to be looked at.
		return out
	}
	if r.promptStyle.FailedExpansionKeepsWhatItDrew {
		return head
	}
	return text
}

// giveUpThePromptExpansion ends the prompt expansion a fatal error happened
// in rather than the shell, reporting whether there was such an error to end.
//
// The axis [Semantics.ParamErrorIsAnExitRequest] is asked here, and it is
// asked because it was measured here rather than because the boundary above
// asks it: `${(%%)v}` with `${NOPE?gone}` in the value ends the shell before
// the command in the shell that documents the operand as exiting, and the
// same value one dialect over reports and renders. That is the split
// [Runner.GiveUpTheFile] already has at a startup file, which is why this
// calls it rather than restating it.
//
// The second half is what [Runner.GiveUpTheFile] does not do, and it is why
// this is a function and not the call alone. A prompt expansion happens
// *inside* a word of a command that is still going to run, so a failure that
// reported itself and set no control flow — a bad substitution, in the
// dialects that word it that way — has to be cleared as well, or the command
// holding the word is refused for an error this boundary just caught. That is
// the same fact giveUpTheCommand records from the other side: expandErr is
// read by the command path, so a boundary either owns it or the command does.
func (r *Runner) giveUpThePromptExpansion() bool {
	caught := r.GiveUpTheFile()
	if !caught && r.ctl == controlExit {
		// Still unwinding: nothing was caught and the flag below belongs to
		// whatever is unwinding rather than to this pass.
		return false
	}
	if r.expandErr {
		r.expandErr = false
		caught = true
	}
	return caught
}
