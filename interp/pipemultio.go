// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "io"

// pipeAsAMultioTarget carries the pipe end a pipeline element sits on into
// that element's own redirection list, for the dialect where a redirection
// **joins** a stream rather than replacing it.
//
// `print one >foo | cat` writes `one` to `foo` *and* down the pipe there, and
// `cat a | cat <b` reads the pipe and then `b`. The shell already joins two
// files — `>foo >bar` and `<a <b` both work — and what was missing is that
// the pipe is one more target on the side it occupies. So this is not a
// second mechanism: it seeds the same maps eachTarget and eachSource already
// build, and Semantics.RedirectsUseEveryTarget still decides, which is what
// makes `unsetopt multios` put the old behavior back without a second
// branch.
//
// Measured 2026-09-29 on zsh 5.9.2, script files:
//
//	print one >foo | cat            `one` through the pipe, `foo` holds it
//	print one >foo >baz | cat       through the pipe and both files
//	print seed >foo; print one >>foo | cat   appends, and through the pipe
//	unsetopt multios; print one >foo | cat   the file alone, pipe empty
//	cat a | cat <b                  the pipe first, then `b`
//	unsetopt multios; cat a | cat <b   `b` alone
//
// **Only the element's own list joins.** A redirection on a command *inside*
// a brace group, a subshell or a function replaces the pipe as it always did
// — `{ print one >foo } | cat` writes `foo` and nothing reaches `cat` — while
// the group's *own* list joins: `{ print one } >foo | cat` does both. That is
// why these are one-shot: the first redirection list to run in the element
// takes them, and anything deeper finds them gone.
type pipeAsAMultioTarget struct {
	// out is the writing end this element's standard output already is, when
	// the element is not the last. nil otherwise, including for the last
	// element, whose standard output is the pipeline's own.
	out io.Writer
	// in is the reading end this element's standard input already is, when
	// the element is not the first.
	in io.Reader
}

// ownPipeMultio moves the element's pipe ends into the slot its own
// redirection list reads, and is called once per command by the dispatcher.
//
// Two fields rather than one, because a redirection list is not the thing
// that bounds this: a brace group or a function body with no redirections of
// its own runs none, so a single field taken by `applyRedirs` survived into
// the *first command inside* the group — `{ print one >foo } | cat` sent
// `one` down the pipe where the reference does not. The dispatcher is the
// boundary that actually matches the measurement, because every command goes
// through it whether or not it redirects anything.
func (r *Runner) ownPipeMultio() {
	r.pipeMultioOwn, r.pipeMultio = r.pipeMultio, pipeAsAMultioTarget{}
}

// takePipeMultio hands the element's own pipe ends to its redirection list,
// and clears them so a second list in the same command cannot take them
// twice.
func (r *Runner) takePipeMultio() pipeAsAMultioTarget {
	held := r.pipeMultioOwn
	r.pipeMultioOwn = pipeAsAMultioTarget{}
	return held
}
