// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `bind -x` runs a shell command from a key, with the line in front of it.
//
// The other half of bind.go: that file says which key runs what, and this one
// says what running it means here. The seam itself is neither dialect's —
// repl.Binding.Function carries a name repl never looks inside and
// driver.Shell.RunWidget is where the dialect is asked to run it — so what
// rides Function here is the *command text*, where the other shell with an
// editor puts a function's name on it. See repl/shellwidget.go, which says in
// as many words that what running it means is the dialect's.
//
// Measured 2026-09-11 against bash 5.3.15 on darwin, through a pseudo-terminal
// so that line editing is on, driving real keys at a real prompt. Every answer
// below is from that session and not from documentation.
//
// **The line arrives in two parameters and goes back out of them.** With
// `abcdef` typed and the cursor at the end, a command bound to a key reads
// `READLINE_LINE=abcdef` and `READLINE_POINT=6`. A command that assigns
// `READLINE_LINE=NEWTEXT; READLINE_POINT=3` leaves the editor drawing
// `NEWTEXT` with the cursor three characters in — measured as four backspaces
// off the end of a seven-character line — and pressing return then submits
// `NEWTEXT`. Either parameter alone is enough: assigning only `READLINE_POINT`
// redraws with the cursor moved, and typing after it inserts at the new place;
// assigning only `READLINE_LINE` to something shorter brings the cursor back
// to the end of what is left rather than leaving it off the end.
//
// `READLINE_POINT` counts **characters, not bytes**: with `caféx` typed — five
// characters, six bytes — the cursor at the end reads 5.
//
// **The three parameters exist only while the command runs.** Measured at the
// next prompt, `${READLINE_LINE-UNSET}` is `UNSET`, and so are the other two.
// That is the same discipline the other dialect's `$BUFFER` keeps, and it is
// the reason they are produced parameters here rather than stored ones.
//
// **The command's status does not reach `$?`.** A command bound to a key that
// returns 42 leaves `echo $?` at the next prompt saying 0. So the status goes
// in — the command sees what the last command left — and does not come out.
//
// **It runs in the current shell.** A key bound to `MARKER=yes` leaves
// `$MARKER` set at the next prompt, so this is not a subshell and a command
// that changes the shell means it.
//
// **It runs with the editor's line discipline, not the shell's.** `stty -a`
// from a bound command reports `-icanon ... -echo`, where the same `stty` run
// as an ordinary command reports `icanon ... echo`. So anything that prints
// from a key prints while the editor holds the terminal — which is #1356's
// concern and is load-bearing rather than incidental. Nothing has to be done
// about it here: this editor holds the terminal the same way for the length of
// the call, so the two agree by construction rather than by arrangement.
//
// Two parameters bash has here and this does not, left out rather than filled
// in with a value that would be a lie:
//
//   - `READLINE_MARK`, which bash sets to the mark's position. This editor has
//     no mark, so there is no position to report and any number would be one
//     invented here. Measured present in bash, at 0, and recorded in
//     docs/spec/editing.md as a standing difference.
//   - `READLINE_ARGUMENT`, which bash sets only when a numeric argument was
//     typed — measured, `M-5` then the key gives `READLINE_ARGUMENT=5` and the
//     key alone leaves it *unset*, not empty. This editor has no numeric
//     argument at all (see the "still missing" list in docs/spec/editing.md),
//     so it is always the second of those two, which is a shape bash produces
//     itself rather than a difference a script can be surprised by.
//
// One difference that is not a naming choice and is worth stating plainly:
// bash **exports** these parameters, so an external program bound to a key
// reads them out of its environment — measured, `env | grep -c READLINE` from
// a bound command answers 3. Here they are produced parameters, which a shell
// function reads exactly as bash's are read and an external program does not
// see at all. Producing them is what makes them vanish again at the end of the
// call, which is the behavior a script can actually observe from the prompt;
// exporting them as well needs a way to mark a produced parameter exported,
// which interp has not got. Recorded rather than papered over.

// The names the line is kept under while a bound command runs, in the Runner's
// variables where no script can reach them — the way bind.go keeps the binding
// table, and what gives a subshell its own copy.
const (
	readlineLineStore  = ".bash.readline.line"
	readlinePointStore = ".bash.readline.point"
)

// readlineParameters are the two a bound command reads and writes the line
// through. They exist only while one is running; see the file comment.
var readlineParameters = []string{"READLINE_LINE", "READLINE_POINT"}

// RunWidget runs the shell command a key was bound to, over the line.
//
// The dialect's answer to driver.Shell.RunWidget: repl hands out the line,
// this publishes it under the names a `bind -x` command reads, runs the
// command text, and hands back whatever the command left. `command` is that
// text and not a name to look up, which is the whole of what differs from the
// other dialect's answer to the same seam.
//
// false is an empty command — a key bound to nothing to run — and the line
// comes back untouched.
func RunWidget(r *interp.Runner, ctx context.Context, command string, in repl.Line) (repl.Line, bool) {
	if command == "" {
		return in, false
	}
	setReadlineLine(r, in)
	openReadlineParameters(r)
	// Deferred rather than called at the end, for the reason the other
	// dialect's are: a panic in the command is caught *outside* this call —
	// repl runs it behind the same guard a typed line runs behind — so a
	// straight-line close would be skipped and the parameters would outlive
	// the call. A script at the next prompt would then find `$READLINE_LINE`
	// set, which is the one thing measured says it must never be, and only
	// after a crash nobody would connect it to.
	defer closeReadlineParameters(r)
	// The status goes in and does not come out, measured: the command sees
	// what the last command left, and what the command leaves is not what the
	// next one reads.
	status := r.ExitStatus()
	// Evaluated as command text rather than called as a name, which is the
	// seam EvalVariable exists for — building `eval "$x"` and parsing that
	// would run the text through the grammar twice and report a syntax error
	// against a line nobody wrote. Named for the builtin that took the text,
	// which is the only name there is: bash binds text, not a variable, so
	// there is nothing else a diagnostic could send a person back to.
	r.EvalVariable(ctx, "bind -x", command)
	r.SetExitStatus(status)
	return readlineLine(r), true
}

// openReadlineParameters gives the command its two parameters, produced rather
// than stored so that an assignment to one is visible to the other and so that
// closing them afterwards leaves the names unset rather than empty.
func openReadlineParameters(r *interp.Runner) {
	r.SetDynamic("READLINE_LINE", readlineBuffer)
	r.SetDynamicWriter("READLINE_LINE", func(rr *interp.Runner, value string) {
		// The stored point is left alone and every *read* of it clamps — see
		// readlinePoint. Clamping here as well would be a second copy of one
		// rule, which is the shape this tree has been bitten by: a line that
		// just got shorter still reports the cursor at its end, because the
		// reader is where the length is known.
		rr.SetVar(readlineLineStore, value)
	})
	r.SetDynamic("READLINE_POINT", func(rr *interp.Runner) string {
		return strconv.Itoa(readlinePoint(rr))
	})
	r.SetDynamicWriter("READLINE_POINT", func(rr *interp.Runner, value string) {
		// A value that is not a number is taken as 0, which is what reading it
		// back through readlinePoint does with anything it cannot parse.
		rr.SetVar(readlinePointStore, value)
	})
}

// closeReadlineParameters takes them away again, so a script that is not
// running a bound command finds them unset.
func closeReadlineParameters(r *interp.Runner) {
	for _, name := range readlineParameters {
		r.UnsetDynamic(name)
	}
	r.SetVar(readlineLineStore, "")
	r.SetVar(readlinePointStore, "")
}

// readlineBuffer is the line as it stands.
func readlineBuffer(r *interp.Runner) string {
	line, _ := r.GetVar(readlineLineStore)
	return line
}

// readlinePoint is the cursor, clamped into the line it points at.
//
// **The clamp is here and nowhere else**, and this is the only place it can be
// right: the line can get shorter after the point was stored —
// `READLINE_LINE=ab` on an eight-character line — so a point checked when it
// was written is not a point still in range when it is read. Measured: a
// command that sets only `READLINE_LINE` to something shorter leaves the
// cursor at the end of what is left.
func readlinePoint(r *interp.Runner) int {
	raw, _ := r.GetVar(readlinePointStore)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return min(max(n, 0), len([]rune(readlineBuffer(r))))
}

func setReadlineLine(r *interp.Runner, in repl.Line) {
	r.SetVar(readlineLineStore, in.Buffer)
	r.SetVar(readlinePointStore, strconv.Itoa(in.Cursor))
}

func readlineLine(r *interp.Runner) repl.Line {
	return repl.Line{Buffer: readlineBuffer(r), Cursor: readlinePoint(r)}
}
