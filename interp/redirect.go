// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/blairham/sh/syntax"
)

// applyRedirs opens the files a command's redirections name and points the
// runner's streams at them for the duration of that command.
//
// Every open goes through the gate for the same reason every exec does: a
// sandbox that only gates execution has not gated the thing that writes to
// the filesystem.
//
// File redirections, descriptor duplication, here-strings and here-document
// bodies are all here; what the shell cannot express is refused rather than
// ignored.
func (r *Runner) applyRedirs(ctx context.Context, rs []*syntax.Redirect, compound bool, owner redirOwner) ([]io.Closer, error) {
	// Whose process these redirections are for, saved and put back rather
	// than cleared, so a command reached from inside a here-document body
	// cannot leave its own answer behind for the next redirection in this
	// list.
	//
	// Defensive rather than load-bearing today, and that is measured: a
	// mutant that clears the field instead of restoring it survives the
	// whole of interp/ and dialect/, because every route from a body to
	// another command — a command substitution, a process substitution —
	// goes through clone(), which copies this field by value and cannot
	// write back. Restoring costs one word and is what keeps that true of a
	// site that one day does not clone.
	outerOwn := r.redirOwner
	r.redirOwner = owner
	defer func() { r.redirOwner = outerOwn }()
	r.redirErr = false
	// Beside redirErr because it is the same flag's number: a failure with a
	// status of its own is only ever this command's.
	r.redirFailureStatusOfItsOwn = 0
	r.badDupTarget = false
	r.redirFds = nil
	// Cleared for every command, including one with no redirections at all,
	// which is what makes the answer *this* command's rather than whatever
	// was last true. See Runner.outputClosedByThisCommand.
	r.outputClosedByThisCommand = false
	// This is where a pipeline element's pipe counts as installed, so the
	// input it replaced stops being available to anything from here on —
	// including a process substitution written as a redirection *operand*,
	// which is measured: `printf "PIPE\n" | cat < <(cat)` reads the pipe in
	// every shell that has the construct, where the same substitution as an
	// argument reads the shell's input in one of them. Before the length
	// check, because a command with no redirections has still reached the
	// point its words are behind it. See Runner.shellStdin.
	r.shellStdin = nil
	if len(rs) == 0 {
		return nil, nil
	}
	var closers []io.Closer
	// What this command has already aimed each stream at, so a second
	// redirection of the same one can be combined with the first where the
	// dialect combines them.
	//
	// Per command rather than per runner, and the stream the command started
	// with is not in here *until the command names it*. `>&1` names it, which
	// is why this is written by the duplication path as well as by the
	// opening one: under the dialect that writes to every target, `echo x >&1
	// >b` reaches the terminal and the file both, and `echo x >b >&1` writes
	// the file twice, because by then the thing being duplicated is already
	// the fan-out (#1261). Reading the earlier note as "the shell's own
	// stream is never a target" is what left the duplication out.
	opened := map[int]io.Writer{}
	// The same set for the other direction, which the same axis governs: a
	// descriptor read from twice arrives as both sources in the order they
	// were written, rather than as the last one alone.
	sources := map[int]io.Reader{}
	// Saved streams are restored when the command finishes, which is why the
	// caller closes what comes back rather than this doing it.
	savedOut, savedIn, savedErr := r.Stdout, r.Stdin, r.Stderr
	closers = append(closers, closerFunc(func() error {
		r.Stdout, r.Stdin, r.Stderr = savedOut, savedIn, savedErr
		return nil
	}))
	// The descriptor table is restored the same way the streams are, and by
	// the same mechanism that lets `exec`'s outlive the command: copy on the
	// first write, and a closer that puts the original back. `exec` skips
	// every closer, so what it wrote stays written.
	fdsTouched := false
	// Held outside the closure because a move under FdMoveDuplicatesThenCloses
	// has to reach *into* the save: the source's close is not a redirection
	// the command takes back, so the table that is put back afterwards must
	// not have it either. See closeMovedSource.
	var savedFds map[int]any
	var savedExecFds map[int]bool
	saveFds := func() {
		if fdsTouched {
			return
		}
		fdsTouched = true
		savedFds = r.fds
		// The marks travel with the table, because they are about the table:
		// a command that redirects a number `exec` had opened is using that
		// number for itself, and when the command ends the number goes back
		// to being `exec`'s.
		savedExecFds = r.execFds
		r.fds = maps.Clone(r.fds)
		r.execFds = maps.Clone(r.execFds)
		closers = append(closers, closerFunc(func() error {
			r.fds, r.execFds = savedFds, savedExecFds
			return nil
		}))
	}
	// dropFdVarDescriptor takes back a descriptor the shell had picked for a
	// `{name}` redirection whose name would not take the number.
	//
	// The open happened before the store was tried — it has to, since the
	// number the name receives is the number the open produced — so a
	// refused store leaves a descriptor nothing published and nothing can
	// reach. Measured 2026-09-22 on bash 5.3.20: `readonly v=42` and then
	// three refused `{v}>` redirections in a row, and the next `exec
	// {q}</dev/null` still answers 10. Here it answered 13, because each
	// refusal kept the number and the file behind it — which moves every
	// descriptor a script picks afterwards.
	//
	// The file is closed only where this redirection owns the close. A
	// descriptor that does *not* outlive its command already has its closer
	// on the list above, and closing it twice is what a second close here
	// would be.
	dropFdVarDescriptor := func(fd int, own io.Closer, persists bool) {
		delete(r.fds, fd)
		delete(r.execFds, fd)
		if persists && own != nil {
			_ = own.Close()
		}
	}
	// closeMovedSource is the second half of `N<&M-`: M is closed, and
	// forKeeps says whether the command takes that back when it ends.
	//
	// No descriptor is really closed here. The move duplicated the source
	// first, so the open file behind it has another name and ending it would
	// take that one with it — the same rule `exec {s}>&1; exec {s}>&-` is
	// about (#2127). Dropping the name is the whole of a move's close.
	closeMovedSource := func(m int, forKeeps bool) {
		switch m {
		case 0:
			r.Stdin = closedFd{}
			if forKeeps {
				savedIn = closedFd{}
			}
		case 1:
			r.Stdout = closedFd{}
			// The same note dupFd's close carries: one dialect stays quiet
			// about a failed write exactly when the command that wrote
			// closed the stream itself.
			r.outputClosedByThisCommand = true
			if forKeeps {
				savedOut = closedFd{}
			}
		case 2:
			r.Stderr = closedFd{}
			if forKeeps {
				savedErr = closedFd{}
			}
		default:
			if !forKeeps {
				// There has to be something to put the number back from.
				// The destination's own save is not enough: `0<&5-` writes
				// a named stream and never touches the table, and the
				// relocating form still owes 5 back when the command ends.
				saveFds()
			}
			delete(r.fds, m)
			delete(r.execFds, m)
			if forKeeps && fdsTouched {
				// Clone before writing: the saved table is the one the
				// Runner had, and a sibling may be holding it.
				savedFds = maps.Clone(savedFds)
				delete(savedFds, m)
				savedExecFds = maps.Clone(savedExecFds)
				delete(savedExecFds, m)
			}
		}
	}

	// Where a failed open is reported is the dialect's answer, and the three
	// they give differ only when the redirect and its command are on
	// different lines — `while read x` … `done < missing` is the shape, and
	// an installed script is where it turned up: the loop opens on line 5
	// and the redirect is on line 11.
	//
	// Restored afterwards, so the command itself is still reported where it
	// was written: only the opening moves.
	commandLine := r.line
	defer func() { r.line = commandLine }()
	for _, rd := range rs {
		// Every dialect reports a *simple* command at the line it began
		// on — measured on a command split by backslashes, where the
		// redirect is two physical lines below its command word and all
		// four still name the command's. They differ only about a
		// compound command, so the axis is asked only there.
		r.line = commandLine
		if compound {
			switch r.diag().RedirectFailureLine {
			case LineOfRedirect:
				r.line = r.lineOf(rd.Pos())
			case LineBeforeRedirect:
				if n := r.lineOf(rd.Pos()); n > 1 {
					r.line = n - 1
				} else {
					r.line = n
				}
			}
		}
		fd := 1
		// `{name}>f` is the lexer's token with the braces kept, and means
		// the shell picks the descriptor and the name receives its number.
		fdVar := ""
		if rd.N != nil {
			if lit := rd.N.Literal(); len(lit) > 2 && lit[0] == '{' {
				fdVar = lit[1 : len(lit)-1]
			} else if n, ok := atoi(lit); ok {
				fd = n
			}
		}
		// `<#` and `>#` move where a descriptor next reads or writes. No
		// file is opened, closed or duplicated, so this is settled before
		// anything below goes looking for one — and before the gate, which
		// has nothing to see: the descriptor was opened already and this
		// only changes where in it the next byte comes from.
		if rd.Op.IsSeek() {
			if rd.N == nil && rd.Op == syntax.TokLessHash {
				// The reading operator with no number is standard input,
				// exactly as a plain `<` is.
				fd = 0
			}
			if !r.seekRedirect(rd, fd) {
				r.redirErr = true
				return closers, nil
			}
			continue
		}

		// A here-document and a here-string are input the shell already
		// holds, so there is no file to open and nothing for the gate to
		// see — the bytes never leave this process on their way in. Before
		// the target is computed, because their word is a *body* rather than
		// a filename: reading it as a target asked the ordinary-word axis
		// about a word no shell reads that way — `<<< $two` is one line of
		// input in all four, never an ambiguous redirect.
		if rd.Op.IsHeredoc() || rd.Op == syntax.TokTLess {
			body := r.heredocBody(rd)
			if r.redirErr {
				// The body could not be expanded, and the process the
				// redirection was for is where that happened — so the
				// command is what was given up, not the shell. See
				// giveUpTheCommand.
				return closers, nil
			}
			// The descriptor it was written for, which this branch used to
			// throw away: the body went to standard input whatever number
			// stood in front of the operator, so `cat 3<<X` fed the document
			// to the command, `exec 3<<X` left nothing behind, `{v}<<X`
			// allocated nothing, and `cat <<X 3<<Y` read Y. Unanimous across
			// bash 5.3, ksh93, zsh and dash — and a correction rather than an
			// axis, because every column answers all four the same way
			// (#2743).
			//
			// A here-document has a *direction* the operator fixes, so an
			// unwritten number is 0 here where the loop's own default is 1.
			hfd := 0
			if rd.N != nil {
				hfd = fd
			}
			// And `{v}<<X` picks a descriptor the same way `{v}<file` does,
			// with the same axis deciding whether it outlives the command.
			persists := false
			if fdVar != "" {
				persists = r.ask(r.sem().FdVariableOutlivesTheCommand,
					"a variable-named descriptor outliving its command") &&
					r.FdVariableDescriptorOutlivesTheCommand()
				if r.unspecified {
					r.redirErr = true
					return closers, nil
				}
				hfd = r.nextFreeFd(-1)
			}
			if r.refuseFdOverLimitFor(fdVar, hfd, "") || r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			// The text is put on a real descriptor, which is what lets a
			// child that names the number itself read it. See
			// Runner.heredocReader.
			src, closer := r.heredocReader(body)
			if closer != nil && !persists {
				closers = append(closers, closer)
			}
			// A body joins the set as an opened file does: `cat <f <<<hi` is the
			// file and then the line, measured, so the source need not be a file
			// to be one of several.
			held := r.eachSource(hfd, src, sources)
			switch hfd {
			case 0:
				r.Stdin = held
			case 1, 2:
				// The document is readable and the number is not writable,
				// which is what a descriptor opened for reading is. Measured
				// on `echo hi 1<<R`: bash reports `write error: Bad file
				// descriptor`, and ksh93 and zsh lose the text in silence —
				// the same two answers they give a stream closed with `>&-`,
				// which is the axis that already decides it.
				w := readOnlyStream{Reader: held}
				if hfd == 1 {
					r.Stdout = w
					// And it is *this command's own* redirection that made
					// the stream unwritable, which is the question the one
					// dialect that words a failed write asks — measured, it
					// says nothing when the writing command wrote the
					// redirection itself and complains when something else
					// did. See Runner.outputClosedByThisCommand.
					r.outputClosedByThisCommand = true
				} else {
					r.Stderr = w
				}
			default:
				if !persists {
					saveFds()
				}
				r.setFd(hfd, held)
				r.redirWrote(hfd)
				if fdVar != "" {
					if !r.setFdVar(fdVar, itoa(hfd)) {
						dropFdVarDescriptor(hfd, closer, persists)
						r.redirErr = true
						return closers, nil
					}
				}
			}
			continue
		}

		// A write into the ends a coprocess published under a name is where
		// this shell notices the coprocess has ended, and it is asked before
		// the target is expanded so that the expansion is the one the notice
		// leaves behind: with the array taken back, `>&${CP[1]}` has no word
		// for a target and is the ambiguous redirect bash reports. See
		// Runner.coprocNoticedByAWrite.
		r.coprocNoticedByAWrite(rd, fd, fdVar)

		// The target is expanded where the command runs, which decides what
		// becomes of a write inside it and of a failure — and, whoever runs
		// the command, an expansion that failed is not a name and is not
		// opened. See redirtarget.go.
		names, bad := r.redirectTargetForItsProcess(rd)
		if bad {
			return closers, nil
		}
		// The word as one name, for the questions that are about the *word*
		// rather than about what it opens. Several words are not a
		// descriptor, which is measured: `v=(1 2); echo x >&$v` makes two
		// files in the shell that fans a target out, because the operator
		// takes its csh reading when the word is not a number.
		name := strings.Join(names, " ")

		// `N<&M-` and `N>&M-` are the move operators: duplicate M onto N and
		// close M, as one operator rather than as a duplication followed by a
		// separate close. The suffix is read here, before anything else looks
		// at the word, because two later readings would otherwise take it
		// first: `5-` is not a descriptor spec, and a bare `>&5-` is a word
		// that names no descriptor, which is the csh file reading. Both are
		// the right answer where the operator does not exist and the wrong
		// one where it does — bash moves for `>&5-` and makes no file.
		moveFrom := -1
		if src, isMove := fdMoveSource(name); isMove {
			form := r.fdMove()
			if r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			if form != FdMoveIsNotAnOperator {
				// atoi cannot fail: fdMoveSource answered for digits.
				moveFrom, _ = atoi(src)
				name = src
			}
		}

		// Whether this redirection takes the coprocess's end away from the
		// letter that reached it, decided where the target is read and acted
		// on where the duplication succeeded.
		handOverCoprocEnd := false

		// `p` is the running coprocess in the two dialects that reach one by
		// a letter, and it is read here — ahead of the csh reading below,
		// which would otherwise make a file called `p` out of a bare `>&p`
		// in a dialect where that word is the facility's. See
		// Semantics.CoprocessNamedByARedirection.
		if r.coprocNamesARedirectionTarget(rd.Op, name) {
			end, running := r.coprocRedirectEnd(rd.Op)
			if !running {
				// Not an open that failed: the duplication's own refusal,
				// with the word where a number usually stands.
				r.diagf("%v\n", r.errBadFd(-1, Wording(
					r.diag().CoprocessDuplicationTargetName, "%[1]s", name), fd))
				r.status = r.diag().redirectFailureStatus()
				r.redirErr = true
				return closers, nil
			}
			name = itoa(end)
			// And in the dialect that hands the end over rather than
			// lending it, the letter stops reaching a coprocess. Recorded
			// here and acted on after the duplication, which still has to
			// read the number — and never where the duplication failed.
			handOverCoprocEnd = true
		}

		// `N>&M` and `N<&M` duplicate a descriptor, and `N>&-` closes one.
		// No file is opened, so the gate has nothing to see: this rearranges
		// streams the shell already holds.
		//
		// Copying the stream *as it is now* is the whole of it, and is why
		// order matters — `>f 2>&1` sends both to the file and `2>&1 >f`
		// sends only stdout there, because the second one copied stdout
		// before it was redirected. All four shells agree, and getting it
		// right needs no special case: the loop already runs left to right.
		// `>&word` is the csh spelling of `&>word` in the dialects that kept
		// it, and it is that spelling exactly: the same flags, the same both
		// streams, the same `set -C`, the same words for an open that failed.
		// Rebinding the operator rather than copying the open is the whole of
		// it — a second copy is a second place to forget noclobber.
		op := rd.Op
		// And whether the redirection wrote a descriptor of its own, which
		// one dialect allows and which changes what the spelling means: see
		// cshOnANumber, read below where the descriptor is settled.
		numbered := false
		if r.greatAmpNamesAFile(rd, fd, fdVar, name) {
			op = syntax.TokAmpGreat
			numbered = rd.N != nil && fdVar == "" && fd != 1
		} else if r.unspecified {
			r.redirErr = true
			return closers, nil
		}

		if op == syntax.TokGreatAmp || op == syntax.TokLessAmp {
			if !isDescriptorSpec(name) {
				r.refuseDupTarget(rd, name)
				return closers, nil
			}
			if rd.N == nil && op == syntax.TokLessAmp {
				fd = 0
			}
			// One dialect will not take a duplication target wider than a
			// single digit, and refuses before it looks at what is open.
			if r.refuseWideDupTarget(name) || r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			if fdVar != "" {
				if name == "-" {
					// `exec {name}>&-` closes the descriptor the variable
					// holds. The close is for keeps — this path never joins
					// the save — so the pipe or file behind it really ends,
					// which is what lets a coprocess see its input finish.
					v, okv := r.fdVarValue(fdVar)
					n, okn := atoi(v)
					if !okv || !okn {
						if r.ask(r.sem().FdVariableBadCloseIsAnError,
							"closing through a variable that holds no descriptor") {
							r.diagf("%s\n", Wording(r.diag().FdVariableWithoutADescriptor,
								"%[1]s: ambiguous redirect", fdVar))
							r.status = 1
							r.redirErr = true
							return closers, nil
						}
						if r.unspecified {
							r.redirErr = true
							return closers, nil
						}
						continue
					}
					if held, ok := r.fds[n]; ok {
						delete(r.fds, n)
						// Only when this was the last name for it. A
						// duplicate is a second name for one open file, so
						// ending the file because one name went would take
						// the others with it — and the sharpest case is the
						// one a prompt does on every start: `exec {s}>&1`
						// then `exec {s}>&-` left the shell with no stdout
						// at all (#2127).
						if c, ok := held.(io.Closer); ok && !r.fdAliased(held) {
							_ = c.Close()
						}
					}
					r.redirWrote(n)
					r.coprocEndClosedByName(fdVar, n)
					continue
				}
				// `exec {name}>&2` picks a fresh descriptor aimed where the
				// target aims now, and the name receives its number.
				//
				// The relocating form gives the source's number back before
				// it chooses, so `{v}<&$w-` answers with `$w`'s own number
				// and the move is a rename. The duplicating form chooses
				// first and closes after, so the name gets the next one up.
				fd = r.nextFreeFd(moveFrom)
			}
			persists := fdVar != "" &&
				r.ask(r.sem().FdVariableOutlivesTheCommand, "a variable-named descriptor outliving its command") &&
				r.FdVariableDescriptorOutlivesTheCommand()
			if r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			// The number, before the duplication that would use it. There is
			// no file here for a refusal to have created, which is the whole
			// of why this side is checked earlier than the other.
			if r.refuseFdOverLimitFor(fdVar, fd, name) || r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			if fd > 2 && !persists {
				saveFds()
			}
			if err := r.dupFd(fd, name, r.dupTargetText(rd, moveFrom), opened); err != nil {
				r.diagf("%v\n", err)
				// A duplication that fails is a redirection that failed, and
				// carries the same number as one whose file would not open.
				// It said 1 here whatever the dialect answered, so the one
				// dialect that reports 2 reported 2 for `>nodir/f` and 1 for
				// `>&3` — the same event, two numbers, and only the first of
				// them measured.
				r.status = r.diag().redirectFailureStatus()
				r.redirErr = true
				return closers, nil
			}
			if handOverCoprocEnd {
				r.coprocEndHandedOver(rd.Op)
			}
			// The close half of a move, once the duplication it follows has
			// happened. Never where the two numbers are the same — `5<&5-`
			// relocates a descriptor onto itself, which is a descriptor that
			// is still open, and closing it would be a close the script did
			// not write.
			//
			// A destination that outlives its command takes the close with
			// it whatever the form says, and that is not a nicety: giving
			// the source back needs a save of the table, and a save is
			// exactly what a persisting `{name}` redirection must not have,
			// since restoring it would take the install back too. One
			// operation, one lifetime.
			if moveFrom >= 0 && moveFrom != fd {
				closeMovedSource(moveFrom, persists || r.sem().FdMove == FdMoveDuplicatesThenCloses)
			}
			r.redirWrote(fd)
			if fdVar != "" {
				if !r.setFdVar(fdVar, itoa(fd)) {
					// A duplication opens nothing of its own — the entry is
					// a second name for a file something else holds — so
					// only the name goes.
					dropFdVarDescriptor(fd, nil, persists)
					r.redirErr = true
					return closers, nil
				}
			}
			continue
		}

		var flags int
		switch op {
		case syntax.TokGreat, syntax.TokClobber, syntax.TokClobberBang:
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if r.noclobber && op == syntax.TokGreat {
				// Under `set -C` a plain `>` refuses to truncate a file that
				// already exists. `>|` is the documented override, and is
				// the one operator on this list that means the same thing in
				// every shell measured; `>!` is that same override in its
				// other spelling rather than a second operator, and which
				// spellings exist was settled by the grammar flag long before
				// the token arrived here.
				flags |= os.O_EXCL
			}
		case syntax.TokDGreat, syntax.TokDGreatClobber, syntax.TokDGreatBang:
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
			if op == syntax.TokDGreat {
				// One dialect puts noclobber on `>>` as well: appending to a
				// name that is not there is a refusal rather than a new file.
				// Dropping O_CREATE is the whole of it — the open then fails
				// with ENOENT and the ordinary cannot-create wording says
				// what happened, which is what that dialect prints. `>>|` and
				// `>>!` are the override, and keep the flag.
				blocks, unanswered := r.noclobberBlocksAppend()
				if unanswered {
					r.redirErr = true
					return closers, nil
				}
				if blocks {
					flags &^= os.O_CREATE
				}
			}
		case syntax.TokGreatSemi:
			// The command writes into a file of its own, so this is an
			// ordinary create-and-truncate — of the temporary, not of the
			// target, which is not opened at all. `set -C` is measured not
			// to reach it: `set -C; echo x >; f` over an existing `f`
			// replaces it in ksh93, which follows from the target never
			// being truncated.
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		case syntax.TokLess:
			flags = os.O_RDONLY
		case syntax.TokLessGreat:
			// `<>` opens for reading and writing, creating the file and
			// never truncating it — `exec 3<> state` keeps what the file
			// held. Unanimous and POSIX, and with no descriptor number it
			// is standard input, exactly as a plain `<` is.
			flags = os.O_RDWR | os.O_CREATE
		case syntax.TokAmpGreat, syntax.TokAmpGreatClobber, syntax.TokAmpGreatBang,
			syntax.TokAmpDGreat, syntax.TokAmpDGreatClobber, syntax.TokAmpDGreatBang:
			// Both streams to one file, and the override marker reaches these
			// two operators as well in the one dialect that has it: `&>|`,
			// `&>!`, `&>>|` and `&>>!` are the same exemption `>|` is.
			appending := op == syntax.TokAmpDGreat ||
				op == syntax.TokAmpDGreatClobber || op == syntax.TokAmpDGreatBang
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			switch {
			case appending:
				flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
				if op == syntax.TokAmpDGreat {
					// The same axis: `&>>` is an append, and the dialect that
					// puts noclobber on `>>` puts it here too.
					blocks, unanswered := r.noclobberBlocksAppend()
					if unanswered {
						r.redirErr = true
						return closers, nil
					}
					if blocks {
						flags &^= os.O_CREATE
					}
				}
			case op == syntax.TokAmpGreat && r.noclobber:
				// `set -C` refuses this truncation exactly as it refuses a
				// plain `>`, and unanimously: `set -C; : > f; echo hi &> f`
				// is a refusal in all six of the panel, each in its own
				// words. The override is spelled after the whole operator
				// rather than inside it — `>|&` is a syntax error in all six,
				// which is what the corpus first recorded and then
				// over-generalized — but `&>|` and `&>!` are not, so the
				// exemption is a token of its own and lands on the cases
				// beside this one.
				flags |= os.O_EXCL
			}
			if !numbered {
				fd = -1
			}
		default:
			return closers, fmt.Errorf("not implemented yet: the %s redirection", rd.Op)
		}
		if rd.N == nil && (flags == os.O_RDONLY || op == syntax.TokLessGreat) {
			fd = 0
		}
		// Whether a descriptor the shell picks outlives the command is the
		// axis ksh93 answers alone, and it is asked once for the redirection
		// rather than once per name: it is a question about the operator.
		persists := false
		if fdVar != "" {
			// The session's own switch can only take the descriptor back
			// sooner, never keep it longer — see
			// Runner.FdVariableDescriptorOutlivesTheCommand.
			persists = r.ask(r.sem().FdVariableOutlivesTheCommand,
				"a variable-named descriptor outliving its command") &&
				r.FdVariableDescriptorOutlivesTheCommand()
			if r.unspecified {
				r.redirErr = true
				return closers, nil
			}
		}

		// One redirection per name, which is one name in every shell but the
		// one that fans a target out — see redirectTarget, which is where the
		// several come from and where the two axes that allow them are asked.
		// The descriptor is the same number for all of them, so the fan-out
		// and fan-in below join them exactly as two redirections written out
		// would be joined; `{name}` is the exception and takes a fresh number
		// per name, leaving the variable holding the last, which is measured.
		for _, name := range names {
			// The shell picks the descriptor: the next free number from its
			// base up, clear of the single digits a script says `2>&1`
			// about. Inside the loop, because a `{name}` target that came to
			// several words takes a number *each*: `v=(f g); exec {n}<$v`
			// leaves `n` holding the second of two, and `<&$n` reads the
			// second file — measured on zsh 5.9.2, which is the only shell
			// that has both halves.
			if fdVar != "" {
				fd = r.nextFreeFd(-1)
			}
			// A name that is not there is not a relative one. Joining it to the
			// working directory turns "no name" into *the directory*, which then
			// opens: `cd /tmp; cat < $unset` read the directory rather than
			// failing, and only because the shell had been told where it was.
			// Every shell reports that it cannot open "". atDir is where that
			// rule lives now — it was written here first, and the file tests had
			// the identical bug because the rule had not reached the resolution
			// they shared (#1189).
			if r.restricted && flags != os.O_RDONLY {
				// A restricted shell writes nowhere it was not already
				// writing. Every operator that *opens* a file to write is
				// refused — `>`, `>>`, `>|`, `<>`, `&>` and `&>>` — and a
				// descriptor duplication is not one of them: measured,
				// `echo x >&2` and `echo x 2>&1` are silent at 0 in a
				// restricted shell while `echo x > f` and `echo x &> f` are
				// this sentence at 1. So the test is the open and its
				// direction, which is what `flags` already holds, rather
				// than a list of tokens that would have had to be kept in
				// step with the parser.
				//
				// Input is untouched for the same reading: `cat < f`, a
				// here-document and a here-string are all silent at 0, and
				// none of the three opens anything to write.
				//
				// The word as the script wrote it, before atDir joins it to
				// the working directory — measured, `echo x > f` names `f`.
				// And the command does not run: r.redirErr is what stops it,
				// exactly as an unopenable target does.
				r.restrictedRedirect(name)
				r.status = restrictedStatus
				r.redirErr = true
				return closers, nil
			}
			path := r.atDir(name)
			// `< /dev/stdin` and `< /dev/fd/0` are the command's own standard
			// input, which is not the process's here: a here-string or a pipe
			// into a function is a stream this shell holds and never put on
			// descriptor 0, so opening the path read what the process was
			// started with. `f() { read l < /dev/stdin; }; f <<<x` reads `x`
			// in bash 5.3.20, zsh 5.9.2 and ksh93u+, and read nothing here
			// (#3404). So it is the duplication `<&0` already is, and nothing
			// is opened — the same reading `.` takes (#3402).
			if flags == os.O_RDONLY && fdVar == "" && (path == "/dev/stdin" || path == "/dev/fd/0") {
				if fd > 2 {
					saveFds()
				}
				if err := r.dupFd(fd, "0", "", opened); err != nil {
					r.diagf("%v\n", err)
					r.status = r.diag().redirectFailureStatus()
					r.redirErr = true
					return closers, nil
				}
				r.redirWrote(fd)
				continue
			}
			action := r.act(Action{Kind: ActionOpen, Path: path, Write: flags != os.O_RDONLY})
			// Unless the path is a pipe this shell made for a substitution in
			// this very command: `cmd > >(inner)` redirects to a name the
			// interpreter chose, so a policy refusing it refuses the construct
			// rather than an access the script asked for. ownPipe carries the
			// argument. The open still happens and is still recorded below.
			if !r.ownPipe(path) && !r.allowed(ctx, action) {
				// A refused open is an open that did not happen, and the command
				// must not run without it. Returning quietly let it run with the
				// stream it was redirecting *away from*: `echo x > denied` wrote
				// to the terminal and reported success, which is the shape of
				// failure a gate exists to prevent — the write goes somewhere
				// the script did not ask for and nothing says so.
				//
				// The status is the one any unopenable redirect gives, because
				// that is what this is to the script. A caller that needs to tell
				// a refusal from a failure has the event, which says which it
				// was.
				r.status = r.diag().redirectFailureStatus()
				r.redirErr = true
				return closers, nil
			}

			// `>;` never opens the target. The command writes into a
			// temporary file in the target's **own directory**, and the
			// rename at the end of the command is what makes the write
			// visible — so a command that fails leaves the target exactly as
			// it was, and one that fails over a target that did not exist
			// creates nothing.
			//
			// Beside the target rather than in a temporary directory,
			// because a rename across filesystems is not a rename: it would
			// be a copy with a window in the middle, which is the one thing
			// this operator exists to avoid.
			//
			// Under `exec` the redirection outlives the command, so there is
			// no end for the rename to happen at and no status for it to
			// read. ksh93 refuses that text outright — `exec >; f` is a
			// syntax error there — and this shell has no parse rule keyed on
			// a command's name, so it opens the target directly instead. The
			// divergence is a refusal we do not make; what it must not be is
			// a temporary file nothing ever renames or removes.
			renameTo := ""
			if op == syntax.TokGreatSemi && r.redirectForBuiltin != "exec" {
				renameTo, path = path, renameOnSuccessTemp(path)
				// Gated as its own open, because it is its own file: the
				// target's permission was asked about above and this is a
				// second name in the same directory. Both have to be
				// allowed for the write to happen, which is the honest
				// reading — a policy that may see this directory at all
				// sees both.
				action = r.act(Action{Kind: ActionOpen, Path: path, Write: true})
				if !r.allowed(ctx, action) {
					r.status = r.diag().redirectFailureStatus()
					r.redirErr = true
					return closers, nil
				}
			}
			// A background job's pid is settled before an open that may never
			// return, so that `&` can hand the shell back. See
			// settleBackgroundJobBeforeABlockingOpen: this is where a job whose
			// first act blocks would otherwise leave the shell waiting for a pid
			// that is not coming.
			r.settleBackgroundJobBeforeABlockingOpen(path)
			// The path as the shell must open it, which is the path the
			// command was given in all but one shape: a substitution inside
			// another one's body publishes a number the enclosing pipe is
			// still parked on here. See Runner.substOpenPath.
			f, fellBack, err := r.openThroughNoclobber(ctx, &action, r.substOpenPath(path), flags)
			if errors.Is(err, errRefused) {
				// The gate let the *name* through and refused what the name
				// reached — a link into a denied place. Reported here rather
				// than through allowed(), and reported with the name the script
				// wrote: see verifyOpened for why the audit record and the
				// diagnostic say different things. To the script this is the
				// refusal above, word for word.
				r.reportRefusal(action)
				r.status = r.diag().redirectFailureStatus()
				r.redirErr = true
				return closers, nil
			}
			if err != nil {
				r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
				// Two verbs, positional because the shells order them
				// differently: %[1]s is the name as written and %[2]s the
				// reason. Two wordings because two of the four say "create"
				// rather than "open" when the redirect was making the file.
				// The noclobber fallback creates nothing, and one dialect
				// words it as the open it is: see openThroughNoclobber.
				creating := flags != os.O_RDONLY &&
					(!fellBack || !r.diag().NoclobberFallbackIsAnOpen)
				format, fallback := r.diag().CannotOpen, "cannot open %[1]s: %[2]s"
				if creating {
					format, fallback = r.diag().CannotCreate, "cannot create %[1]s: %[2]s"
				}
				if r.noclobber && (op == syntax.TokGreat || op == syntax.TokAmpGreat) &&
					errors.Is(err, fs.ErrExist) &&
					r.diag().NoclobberRefusal != "" {
					// Half the panel has a sentence for this one refusal — the
					// file `set -C` would not overwrite — and the other half
					// words it as any other failed create, which is what an
					// empty wording leaves in place.
					format = r.diag().NoclobberRefusal
				}
				if name == "" && r.diag().EmptyRedirectTarget != "" {
					// One dialect says something shorter for a name that is not
					// there, and says it the same way in both directions.
					r.diagf("%s\n", Wording(r.diag().EmptyRedirectTarget, "", name))
					r.status = r.diag().redirectFailureStatus()
					r.redirErr = true
					return closers, nil
				}
				r.diagf("%s\n", Wording(format, fallback,
					name, r.diag().openReason(err, creating)))
				r.status = r.diag().redirectFailureStatus()
				r.redirErr = true
				return closers, nil
			}
			// The open that happened, not only the one that failed: an audit
			// trail fed by the error path alone held every file the shell could
			// not open and none it could.
			r.emit(ctx, Event{Kind: EventAccess, Action: action})
			// The name as the script named it, kept for the one caller that has
			// to word a failure of its own *after* this succeeded. See
			// Runner.openedName.
			r.openedName = name
			// And only now the number, because the order is measured: under a
			// limit of twenty, `exec 20>fresh` complains and the file is there
			// afterwards. The open happens and the descriptor it produces is what
			// cannot be moved to the number the script asked for.
			if r.refuseFdOverLimitFor(fdVar, fd, name) || r.unspecified {
				_ = f.Close()
				r.redirErr = true
				return closers, nil
			}
			switch {
			case renameTo != "":
				// The rename is the close, and it has to be *this* closer
				// rather than an extra one beside it: closing the file twice
				// is what a plain closer and a rename closer together would
				// do. See renameOnSuccess, which reads the status the
				// command ended at.
				closers = append(closers, renameOnSuccess{
					r: r, f: f, temp: path, target: renameTo,
				})
			case !persists:
				// A descriptor that outlives the command must not be closed
				// when it ends, which is the same exemption `exec` already has
				// — exec skips every closer.
				closers = append(closers, f)
			}

			switch fd {
			case -1:
				w := r.eachTarget(-1, f, opened)
				r.Stdout, r.Stderr = w, w
			case 0:
				r.Stdin = r.eachSource(0, f, sources)
			case 1:
				r.Stdout = r.eachTarget(1, f, opened)
			case 2:
				r.Stderr = r.eachTarget(2, f, opened)
			default:
				// A descriptor beyond the three named streams goes into the
				// table, where `>&N` finds it. `default` used to land on stdout,
				// so `exec 3>out.txt` sent every later `echo` into the file —
				// then it was refused outright, and now the number is kept.
				if !persists {
					saveFds()
				}
				// And a number repeated in one redirection list joins its
				// targets exactly as a named stream does, under the dialect
				// that joins them: `exec 3>a 3>b; echo hi >&3` fills both
				// files in zsh 5.9.2 and only `b` in the other five, and
				// `exec 3<fa 3<fb; cat <&3` reads both in order there.
				// Measured 2026-09-12.
				//
				// The named streams have had this since #1261 and a numbered
				// one had not, which is #734's third residue: the fan-out was
				// written where the switch happened to land rather than for
				// every descriptor, so the same script said different things
				// about 1 and about 3.
				//
				// Which direction to join is the open's own flags. `<>` is
				// left alone deliberately: it is one descriptor that both
				// reads and writes, and joining one half of it would model
				// half of the pair.
				var held any = f
				switch {
				case flags == os.O_RDONLY:
					held = r.eachSource(fd, f, sources)
				case flags&os.O_RDWR == 0:
					held = r.eachTarget(fd, f, opened)
				}
				r.setFd(fd, held)
				r.redirWrote(fd)
				if fdVar != "" {
					if !r.setFdVar(fdVar, itoa(fd)) {
						dropFdVarDescriptor(fd, f, persists)
						r.redirErr = true
						return closers, nil
					}
				}
			}
			if numbered {
				// The csh form on a descriptor that is not standard output
				// is `N> word 2>&N` and not the both-streams `&> word`: the
				// file lands on the number the script named, and standard
				// error is pointed at it as well. Measured on zsh 5.9.2,
				// 2026-09-13, with a command writing `O` to stdout and `E`
				// to stderr in an empty directory: `3>&qq` and `0>&qq` put
				// `E` in the file and leave `O` on the terminal, where
				// `1>&qq` and the bare `>&qq` take both.
				//
				// `2>&qq` writes `E` *twice*, and that falls out rather than
				// being arranged: the second target for standard error is
				// the descriptor the first one just opened, so the shell
				// that writes to every target of a stream writes to this one
				// twice. `unsetopt multios` leaves one copy, which is the
				// same axis answering the other way.
				r.Stderr = r.eachTarget(2, f, opened)
			}
		}
	}
	return closers, nil
}

// eachTarget combines a stream's targets where the dialect writes to all of
// them, and returns the newest otherwise.
func (r *Runner) eachTarget(fd int, f io.Writer, opened map[int]io.Writer) io.Writer {
	if prev, ok := opened[fd]; ok &&
		r.ask(r.sem().RedirectsUseEveryTarget, "a command redirecting one stream to several targets") {
		// Marked as what it is rather than left to be recognized by type. A
		// stream over several files is the one shape that cannot be handed to
		// a process replacement as a descriptor number, and the shell that
		// built it is the only thing that knows; inferring it from "not an
		// *os.File" would sweep in an embedder's buffer, which is a different
		// case with a different answer. See namedStreamsCanBePlaced.
		f = multiTarget{Writer: io.MultiWriter(prev, f), last: fileOrNil(f)}
	}
	opened[fd] = f
	return f
}

// heredocReader puts a here-document's or a here-string's body somewhere a
// descriptor can point at, and hands back what reads it and what closes it.
//
// The text is this process's own — a string the parser produced — and a
// string has no descriptor number. That is enough for every read *this* shell
// does and for nothing a child does: `childFiles` rebuilds the table by
// number for a command it starts, an entry it cannot turn into an *os.File
// answers nil, and a nil is a descriptor closed over there. So `sh -c 'cat
// <&3' 3<<X` said `3: Bad file descriptor` where all four columns of the
// panel print the body (#2759).
//
// Which medium is [Semantics.HeredocBody], and it is measured rather than
// chosen: the panel splits two-two, and a script can tell them apart.
//
// A medium that could not be made falls back to the text itself. That is the
// old answer, which is wrong only for a child naming the number — the shell's
// own reads are unaffected — so a machine out of descriptors or with no
// writable temporary directory keeps working rather than failing a
// redirection every shell performs.
func (r *Runner) heredocReader(body string) (io.Reader, io.Closer) {
	if r.sem().HeredocBody == HeredocBodyInATemporaryFile {
		// Named here and opened exclusively rather than through
		// os.CreateTemp, which is forbidden in this tree for a reason that
		// applies exactly here: it resolves an empty directory through
		// os.TempDir, which is the *process* environment's `$TMPDIR`, where
		// the shell's answer is the script's — `r.tempHome()`, which reads
		// the variable. A script that moved the name moved where its own
		// private text goes. `newSubstFile` names its file the same way.
		path := filepath.Join(r.tempHome(),
			".sh-heredoc-"+strconv.Itoa(os.Getpid())+"-"+
				strconv.FormatUint(heredocSpoolSeq.Add(1), 10))
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return strings.NewReader(body), nil
		}
		// Unlinked at once and read through the descriptor that is already
		// open on it, so nothing is left behind by a shell that is killed
		// and nothing in the filesystem names a script's private text.
		_ = os.Remove(path)
		if _, err := f.WriteString(body); err != nil {
			_ = f.Close()
			return strings.NewReader(body), nil
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			_ = f.Close()
			return strings.NewReader(body), nil
		}
		return f, f
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return strings.NewReader(body), nil
	}
	// The write side is a goroutine because a body larger than the pipe
	// buffer would otherwise block the shell against a reader that has not
	// started yet. It ends either way: the write completes, or the read end
	// is closed and the write fails — Go reports that as an error on a pipe
	// rather than raising SIGPIPE, which it does only for the two standard
	// streams.
	go func() {
		_, _ = io.WriteString(pw, body)
		_ = pw.Close()
	}()
	return pr, pr
}

// fileOrNil is the real file behind a target, where there is one.
//
// It is what a *child* is given for a numbered descriptor the shell fanned
// out, and it is the last target named rather than all of them — see
// multiTarget.last.
func fileOrNil(w any) *os.File {
	f, _ := w.(*os.File)
	return f
}

// eachSource concatenates a stream's sources where the dialect reads from all
// of them, and returns the newest otherwise.
//
// The mirror of eachTarget across the one axis both of them ask, and it needs
// no marker type of its own: a fan-out cannot be handed to a process
// replacement as a descriptor number, where a concatenation can — os/exec
// gives a child a pipe for any reader that is not a file, which is what the
// shell that has this does too. Measured on zsh 5.9.2: `cat <f` sees a
// regular file on its input and `cat <f <g` sees a pipe.
func (r *Runner) eachSource(fd int, in io.Reader, sources map[int]io.Reader) io.Reader {
	if prev, ok := sources[fd]; ok &&
		r.ask(r.sem().RedirectsUseEveryTarget, "a command reading one stream from several sources") {
		joined := io.MultiReader(prev, in)
		if fd > 2 {
			// A number the *table* holds, which is the one direction that
			// needs a marker: standard input is handed to a child as a
			// stream and os/exec makes the pipe, where a number above two
			// has to be a real descriptor. See fanSource.
			in = fanSource{Reader: joined, last: fileOrNil(in)}
		} else {
			in = joined
		}
	}
	sources[fd] = in
	return in
}

// fanSource is a numbered descriptor read from several files at once, and the
// last of those files.
//
// The concatenation is what *this shell* reads through — `cat <&3` after
// `exec 3<f 3<g` is both files in order, which is the whole of #734's third
// residue. The file is what a **child naming the number itself** is handed,
// and it is the last one because that is what the number held before the
// fan-in existed: `childFiles` rebuilds the table by descriptor number and a
// concatenation has no number, so the alternative to naming one file is
// naming none and closing 3 in the child — which would be a new wrong answer
// where there was an old one.
//
// The shell that has this forks a process to do the joining, so its child
// sees a pipe carrying both files. Reaching that from here means building the
// pipe and a copier per extra descriptor, with a lifetime tied to a child
// this function does not start; it is the remaining half and it is written
// down in docs/spec/semantics.md rather than guessed at.
//
// multiTarget.last is the same thing on the writing side, for the same
// reason.
type fanSource struct {
	io.Reader
	last *os.File
}

// multiTarget is a stream the shell built out of more than one target, under
// the dialect that writes to every one of them.
//
// It is a marker before it is a writer: the io.Writer inside is an ordinary
// multi-writer and does the work, and the type exists so that `exec cmd` can
// tell this stream from a file without guessing.
type multiTarget struct {
	io.Writer
	// last is the newest of the targets, where it is a real file, and it is
	// what a child naming the descriptor *number* is handed. Nil for the
	// named streams, which reach a child as streams and need no number. See
	// fanSource for the whole of the argument.
	last *os.File
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// redirectTarget is the name — or names — a redirection opens, and says
// whether the shell refused it.
//
// Two answers, and this had a third that is nobody's: it expanded the target
// the way an argument is expanded — split into fields and matched as a
// pattern — and then quietly took the first field. So `e="a b"; echo hi > $e`
// wrote to `a`, and `e="x*"` truncated whichever file happened to match,
// which the script never named.
//
// **Several names is the shell that does not split one.** A target that
// comes to more than one word is `ambiguous redirect` under the ordinary-word
// reading, and under the other reading it is one redirection per word — which
// the fan-out and fan-in then join, so `v=(f g); cat <$v` reads both files
// and `v=(a b); echo hi >$v` fills both. Measured 2026-09-12 on zsh 5.9.2,
// with the option that joins them turned off as the control: `unsetopt
// multios` there gives the *joined* name, one file called `f g`, which is
// what this shell used to do always (#1792).
//
// So the two questions are asked in that order, and the second is the fan's
// own axis rather than a new one. A dialect that does not split a target and
// does not join several of them is not in the panel, and its answer here is
// the joined name it always was.
//
// **Braces make words here too, and the fan's axis is what decides it.** A
// target is brace-expanded before any of the above, so `: > d/{a,b,c}` is
// three names rather than one file whose name holds the braces — which is
// what this shell wrote until #4455. Measured 2026-09-25 in a fresh directory
// per shell, `-f` throughout:
//
//	                              : > d/{a,b,c}         : > {a,b}
//	zsh 5.9.2                     d/a, d/b, d/c         a and b
//	zsh 5.9.2, unsetopt multios   one `d/{a,b,c}`       one `{a,b}`
//	bash 5.3.20, bash 3.2.57      ambiguous redirect    ambiguous redirect
//	ksh93u+ 2012-08-01            one `d/{a,b,c}`       one `{a,b}`
//	dash 0.5.12                   one `d/{a,b,c}`       one `{a,b}`
//
// The second row is the one that settles where the question belongs. Turning
// that shell's option off does not leave the braces expanded and the fan
// joined — it stops the expansion, so the target is the one name it is
// written as, and not the `a b` a join of two names would give. That is the
// same switch taking a third consequence with it, which is exactly the
// argument [Semantics.RedirectsUseEveryTarget] already makes for being one
// axis rather than two; a second field here would be a second place to forget
// it. ksh93 brace-expands an argument and reaches the same answer from the
// other side, having no fan at all.
//
// bash is the fourth row and needs no new question: it reads the target as an
// ordinary word, so the two names the braces make are two words, and two words
// there is the ambiguity it already reports.
func (r *Runner) redirectTarget(rd *syntax.Redirect) ([]string, bool) {
	// Whether there is a group that *could* make words, which costs nothing
	// to ask: both halves read the spans and expand nothing, so neither runs
	// a substitution that an expansion below would run again. The second
	// half is what a count with the endpoints left unexpanded cannot see —
	// `> {1..$n}` is one word to it and three to the shell that reads the
	// endpoint first.
	//
	// The word is then expanded once, after the questions rather than
	// before: expanding the target as written and then expanding each word
	// the braces made would run `> $(f){a,b}`'s command three times where
	// the argument path runs it twice.
	braces := !r.noBraceExpand &&
		(r.braceCount(rd.Word) > 1 || r.braceRangeShaped(rd.Word))

	var (
		fields, words []string
		plain         string
	)
	if !braces {
		fields, words, plain = r.expandRedirectTargetViews(rd.Word)

		// Asked only where the readings differ, which is almost never: `> f`
		// and `> "$e"` are one word under all three, and so is a pattern that
		// matches nothing. Asking every time would refuse every redirection in
		// the core over a question that decides nothing.
		//
		// Braces count as differing: a target that expands to several words is
		// several words to the dialect that expands one — which is why this is
		// reached only by a word with no group in it.
		same := len(fields) == 1 && fields[0] == plain &&
			len(words) == 1 && words[0] == plain
		if same {
			return []string{plain}, false
		}
	}

	// The wider question first, so that a run with no dialect at all is
	// refused by the name a script can act on: the count a target may come
	// to is what `> $two` is about, and the narrower question below decides
	// nothing once this one has no answer.
	ordinary := r.ask(r.sem().RedirectTargetIsAnOrdinaryWord, "a redirection target expanded as an ordinary word")
	if !ordinary && r.unspecified {
		// Refused, so the command must not run: acting on either reading
		// after saying the shells disagree would be answering the question
		// anyway.
		r.redirErr = true
		return nil, true
	}

	// POSIX's own sentence, and the narrowest of the three questions
	// here: the word after a redirection operator is not field-split and
	// not pathname-expanded, whatever else the shell makes of it. So
	// `cat < only-*.txt` opens a file by that name and fails even where
	// one matches, and `e="a b"; > $e` writes a file called `a b`.
	//
	// Both views collapse onto the text, which is the one that never
	// split and never matched. The *count* survives: a target that
	// expanded to nothing is still nothing, and the reading below turns
	// that into `ambiguous redirect` in the one column that does — it is
	// how many words there are that this axis does not answer.
	//
	// Reached only where the views already differ, so `> f` and a
	// pattern that matched nothing ask it nothing. Four columns answer
	// no outright, and the two that answer yes both move to no in POSIX
	// mode — see Runner.SetPosixMode (#3207).
	pathname := r.ask(r.sem().RedirectTargetTakesPathnameExpansion,
		"a redirection target field-split and matched as a pattern")
	if !pathname && r.unspecified {
		r.redirErr = true
		return nil, true
	}

	// Whose reading the braces belong to, asked in the order the two
	// questions above are asked in: the shell that reads a target as an
	// ordinary word expands them and calls what they make ambiguous, and the
	// shell that reads a target as a list of names expands them under the
	// same switch that gives it the list. See this function's own comment for
	// the row that puts the second question here rather than on a field of
	// its own.
	//
	// The words are built only once both say yes, and how many there are is
	// read off the words rather than off the count above — `> {1..$(f)}` is
	// three names in the shell that reads an endpoint before the range and
	// one in the shell that does not, and only the expansion knows which.
	braced, fromTheNames := false, false
	if braces && r.ask(r.sem().BraceExpansion, "brace expansion") &&
		(ordinary || r.ask(r.sem().RedirectsUseEveryTarget,
			"a redirection target's braces making several names")) {
		if made := r.braceExpand(rd.Word); len(made) > 1 {
			braced = true
			if !ordinary {
				fields, words, plain = r.redirectTargetViewsOf(made)
				fromTheNames = true
			}
		}
	}
	if braces && !fromTheNames {
		// Either the braces made no more than the one word, or they made
		// several and the word is about to be refused for it: both want the
		// target as written, expanded once.
		fields, words, plain = r.expandRedirectTargetViews(rd.Word)
	}

	if !pathname {
		if len(fields) > 0 {
			fields = []string{plain}
		}
		if len(words) > 0 {
			words = []string{plain}
		}
	}

	if !ordinary {
		if len(words) > 1 {
			if r.ask(r.sem().RedirectsUseEveryTarget,
				"a redirection target that came to several words") {
				return words, false
			}
			if r.unspecified {
				r.redirErr = true
				return nil, true
			}
			// Not joined: the words the reading produced, written out with a
			// space between them, which is the single filename the shell
			// with the option turned off opens.
			return []string{plain}, false
		}
		if len(words) == 1 {
			// One word, and it is the word rather than the text: the
			// dialect that reaches here matches a pattern written in the
			// source, so `cat <p?` opens the one file it found where the
			// text view still holds `p?`. zsh alone among the four, now
			// that RedirectTargetTakesPathnameExpansion is asked above —
			// ksh93, dash and BusyBox ash match no pattern at all, and
			// arrive here with the text already in this view (#3207).
			return words, false
		}
		// Nothing at all, which is a name of no characters and is opened as
		// one: `v=(); cat <$v` reports the empty name in the shell that has
		// the reading.
		return []string{plain}, false
	}
	// bash's reading, and braces make words as surely as splitting does:
	// `> {a,b}` names two files and so names none. The count is what decides
	// it, so the words themselves are never built on this side — the
	// redirection is refused before anything is opened.
	if !braced && len(fields) == 1 {
		return fields[:1], false
	}
	// Anything but exactly one word, which includes none: an empty variable
	// is as ambiguous as two filenames, because neither says where to write.
	//
	// **Unless the expansion failed**, which is the one way to reach no words
	// without ever having had a count. This complaint is about *how many*
	// words the target came to, and a word that could not be computed came to
	// none of them — so the failure's own sentence is the whole of what the
	// script is told, and the redirection is refused without a second line.
	// See redirtarget.go, which states the invariant this was breaking: a
	// target whose expansion failed is one diagnostic, in every column.
	//
	// Keyed on the **failure** and not on the count, and the pair that says
	// so holds the count fixed at each of its two wrong values: `e=; : > $e`
	// is no words with nothing failed and is ambiguous, and `e="a b"; : >
	// $e$(( 1/0 ))` is the several-word shape with a failure in it and is
	// not. Measured 2026-09-26 on bash 5.3.20 and 3.2.57, which are the only
	// two columns that reach here at all (#4688).
	if r.targetExpansionFailed() {
		r.redirErr = true
		return nil, true
	}
	r.diagf("%s\n", Wording(r.diag().AmbiguousRedirect, "%[1]s: ambiguous redirect", rd.Text))
	r.redirErr = true
	r.status = 1
	return nil, true
}

// atoiSigned reads a decimal integer that may carry a sign, which atoi does
// not: a descriptor number never has one and a `shift` count may.
//
// The sign is unanimous where it is read at all — `shift +1` moves one in all
// six shells in the panel — and a `-` in front of the digits is what makes a
// count out of range rather than a word that is not a number.
func atoiSigned(s string) (int, bool) {
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	n, ok := atoi(s)
	if !ok {
		return 0, false
	}
	if neg {
		return -n, true
	}
	return n, true
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// heredocBody produces the text a here-document or here-string feeds in.
//
// The delimiter's quoting decides whether the body is expanded, which is a
// property of how the delimiter was *written* and is why the lexer had to
// record it rather than resolve it. A quoted delimiter makes the whole body
// literal; an unquoted one leaves it subject to expansion.
func (r *Runner) heredocBody(rd *syntax.Redirect) string {
	if rd.Op != syntax.TokTLess {
		// A here-document body is text one dialect reads at *expansion*
		// time rather than with the script's line, and it names a refusal
		// inside it after the construct for that reason. A here-**string**
		// is not: measured 2026-09-17 on bash 5.3.20, `cat <<<"$(echo hi;
		// for)"` is located plainly where the same substitution in a body is
		// `command substitution:`, whatever command the redirection is for —
		// an external one, a builtin, and a function all agree. See
		// Runner.substFailureRoute.
		was, wasLine := r.inBodyReadAtExpansion, r.expansionBodyLine
		r.inBodyReadAtExpansion, r.expansionBodyLine = true, 0
		// And that it is a here-document body rather than the other text
		// that field covers, which is a second question asked of the same
		// two lines of the same file: a substitution in a body that will
		// not parse is graded by the here-document's own axis wherever it
		// is written, and the same substitution in a *backquoted* body is
		// not. The here-string above is excluded from both for the same
		// reason — its word is read with the script's line. See
		// Runner.substParseErrorEscapesASubshell.
		wasHeredoc := r.inHeredocBody
		r.inHeredocBody = true
		if rd.Heredoc != nil && rd.Heredoc.Start.Line > 0 {
			// And where the body sits in the file. The body is lexed again
			// from its own text — see Runner.rawSpans — so everything in it
			// is numbered from the body's first line, and a refusal inside a
			// substitution written there was reported at line 1 of a file
			// whose here-document began on line 4. Measured 2026-09-17 on
			// bash 5.3.20: `command substitution: line 4:` for the line the
			// body holds it on, and line 4 again for a two-line body's second
			// line, which is the file's numbering throughout.
			r.expansionBodyLine = int(rd.Heredoc.Start.Line)
		}
		defer func() {
			r.inBodyReadAtExpansion, r.expansionBodyLine = was, wasLine
			r.inHeredocBody = wasHeredoc
		}()
	}
	if r.redirOwner == redirOwnerThisShell {
		// The shell runs this command itself and there is no other process
		// anywhere, so the body is expanded here and a failure in it is this
		// shell's to place. See heredocbodyfailure.go, which holds the panel
		// for that.
		return r.expandBodyInThisShell(func() string { return r.heredocText(rd) })
	}
	// The redirection belongs to a command this shell runs as a process of
	// its own, or to a `( … )` a real shell forks for. Either way there is
	// another process for the body to have been expanded in, and which axis
	// says whether it was is the owner's — see heredocprocess.go for the two
	// measurements and for why they are two.
	return r.confineToTheProcess(func() string { return r.heredocText(rd) })
}

// heredocText is the expansion itself, with nothing said about whose process
// it happens in.
func (r *Runner) heredocText(rd *syntax.Redirect) string {
	if rd.Op == syntax.TokTLess {
		// A here-string is one line, and its word is expanded like any other.
		return strings.Join(r.expandWordNoSplit(rd.Word), "") + "\n"
	}
	if rd.Heredoc == nil {
		return ""
	}
	body := rd.Heredoc.Literal()
	// A body that ran to the end of the input is the one that may not end in
	// a newline, and whether the shell supplies the missing one is a
	// disagreement — see UnterminatedHeredocGainsATrailingNewline. Asked here
	// rather than in the lexer because the two shells read the same text and
	// hand the command different bytes, which is a semantics question and not
	// a grammar one.
	//
	// A body with nothing in it is not that shape and never reaches the
	// question: `cat <<E` with the input ending on the next byte hands the
	// command no bytes at all in every shell of the panel, bash included.
	// The axis is about the *last line* of a body having no newline after
	// it, and an empty body has no last line — supplying one there wrote a
	// bare newline nobody asked for, at status 0, which is this burndown's
	// own shape (#2298).
	if rd.HeredocAtEOF && body != "" && !strings.HasSuffix(body, "\n") {
		if r.ask(r.sem().UnterminatedHeredocGainsATrailingNewline,
			"a newline on an unterminated here-document's last line") {
			body += "\n"
		} else if r.unspecified {
			// Refused, so the command must not run on either reading — the
			// same rule redirectTarget follows, and for the same reason:
			// handing the body over after saying the shells disagree would
			// be answering the question anyway.
			r.redirErr = true
		}
	}
	if rd.Heredoc.Spans[0].Quoting != syntax.Unquoted {
		return body
	}
	// The lexer kept the body raw, so its expansions have to be found now.
	// Passing it through as one literal span looks equivalent and silently
	// expands nothing, which is the mistake this comment exists to prevent.
	//
	// Expanded but never split or globbed: a here-document is one blob of
	// input, not a list of fields.
	return r.expandRawText(body)
}

// openReason strips the layers Go's os package adds to an errno.
//
// A shell says "No such file"; os.OpenFile says "open b: no such file or
// directory", which repeats the name the caller is about to print and reads
// like a Go program rather than a shell.
func (d Diagnostics) openReason(err error, creating bool) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		// dash writes its own text for this one errno, and writes a
		// *different* one depending on which way the file was being opened:
		// a read that finds nothing is "No such file", a write that cannot
		// make one is "Directory nonexistent". The OS says "No such file or
		// directory" for both.
		if creating && d.DirectoryNotFound != "" {
			return d.DirectoryNotFound
		}
		if !creating && d.FileNotFound != "" {
			return d.FileNotFound
		}
	}
	// reason() capitalizes, because that is what the C string says and what
	// three of the four print; reasonText() puts it back down for the one
	// that lowercases everything. Go's own errno strings are lowercase, which
	// is why this went through neither before and matched nobody.
	return d.reasonText(reason(err))
}

// refuseFdOverLimit reports a descriptor number this process could not hold,
// where the dialect is one that looks.
//
// No shell in the panel has a ceiling of its own. The one that bites is the
// kernel's limit on open files, and two of the five hand the errno straight
// back: with `ulimit -n 20`, bash answers `exec 20>f` with `20: Bad file
// descriptor` and status 1 while `exec 19>f` is silent, and ksh93 refuses the
// same numbers in its own words. dash and zsh report success and leave the
// descriptor unusable, so the number is not checked there at all.
//
// Three things keep the axis off the common path, and each of them is a
// question this would otherwise ask about every redirection in every script.
// The named streams are always there; a Runner with no GetRlimit has no limit
// to be asked about, and a library that was given none is not the place to
// invent one; and a number below the limit is nobody's disagreement. What is
// left is exactly the case the panel splits on.
//
// A number the *shell* picked is checked too, and that is measured rather than
// assumed: `ulimit -n 6; exec {v}>f` fails in all three shells that have the
// construct, because the number they pick is over the limit like any other.
// They word it three ways — `cannot duplicate fd`, `cannot open`, `cannot move
// fd 3` — and this says what it says about a number the script wrote, which is
// the shape of the failure without the sentence. Only reachable under a limit
// below ten, since that is where the picking starts.
func (r *Runner) refuseFdOverLimit(fd int) bool {
	if fd <= 2 || r.GetRlimit == nil {
		return false
	}
	soft, _, err := r.GetRlimit(ResourceOpenFiles)
	if err != nil || soft == RlimitInfinity || int64(fd) < soft {
		return false
	}
	if !r.ask(r.sem().FdNumberBoundedByOpenFileLimit,
		"a descriptor number at the process's limit on open files") {
		return false
	}
	r.diagf("%s\n", Wording(r.diag().FdNumberOverLimit, "%[1]d: %[2]s",
		fd, r.diag().reasonText(reason(syscall.EBADF))))
	r.status = r.diag().redirectFailureStatus()
	return true
}

// refuseFdOverLimitFor routes the two refusals a number over the process's
// limit can get: a number the **script** wrote is the dialect's axis above,
// and a number this shell **picked** for a `{name}` redirection is the event
// below, which every shell with the construct refuses.
func (r *Runner) refuseFdOverLimitFor(fdVar string, fd int, target string) bool {
	if fdVar != "" {
		return r.refusePickedFdOverLimit(fd, target)
	}
	return r.refuseFdOverLimit(fd)
}

// refusePickedFdOverLimit reports a descriptor the *shell* picked for a
// `{name}` redirection that this process could not hold.
//
// A different event from [Runner.refuseFdOverLimit], and the panel splits
// differently on it. A number the script wrote is refused by two of the five;
// a number the shell picked is refused by **all three that have the
// construct** — measured 2026-09-22 under `ulimit -n 8` from a script file,
// where bash 5.3.20, ksh93 and zsh 5.9.2 each refuse `exec {v}</dev/null` at
// 1, and dash and BusyBox ash have no such redirection to refuse. So there is
// no axis here: what the three disagree about is the wording, which
// Diagnostics.FdPickedNumberUnusable carries beside CannotOpen.
//
// Only reachable under a limit below ten, since that is where the picking
// starts — and the open has already happened where there is one, which is the
// order the sentences describe: the file is opened and the descriptor it
// produced is what cannot be moved to the number.
func (r *Runner) refusePickedFdOverLimit(fd int, target string) bool {
	if fd <= 2 || r.GetRlimit == nil {
		return false
	}
	soft, _, err := r.GetRlimit(ResourceOpenFiles)
	if err != nil || soft == RlimitInfinity || int64(fd) < soft {
		return false
	}
	d := r.diag()
	why := d.reasonText(reason(syscall.EINVAL))
	if w := d.FdPickedNumberUnusable; w != "" {
		// Located by name alone, which is the whole of what is special about
		// this sentence: the shell that writes it leaves the line out of
		// this one message and puts it back for the refusal underneath.
		located := r.locatedByNameAlone
		r.locatedByNameAlone = true
		r.diagf("%s\n", Wording(w, "cannot duplicate fd: %[1]s", why))
		r.locatedByNameAlone = located
	}
	if target == "" {
		target = d.RedirectWithoutATargetName
	}
	r.diagf("%s\n", Wording(d.CannotOpen, "%[1]s: %[2]s", target, why))
	r.status = d.redirectFailureStatus()
	return true
}

// refuseWideDupTarget is one dialect's refusal of `>&10`.
//
// The other four read the number and fail at run time if nothing is open
// there — `10: Bad file descriptor`, status 1, and the script carries on.
// This one will not take the *word* at all, whatever it names.
//
// Three things about it were measured rather than assumed, and each one
// decides where the check lives:
//
//   - **It is not a parse refusal**, though it is worded as one. `sh -n -c
//     'echo hi >&10'` accepts the input and exits 0, and a script whose
//     second line has it prints its first line before stopping. So the
//     question belongs to the semantics vector and to the dialect's
//     wording, not to syntax.Dialect: the grammar takes it everywhere.
//   - **It is the width and not the value.** `>&08` names descriptor 8 and
//     is refused just the same, so this cannot be folded into the axis about
//     numbers the open-file limit will not give out.
//   - **It is the expanded word.** `n=10; echo hi >&$n` is refused where
//     `n=9` is not, so the check has to come after the target is expanded,
//     which is where it is.
//
// The refusal ends the script, and that travels with the answer rather than
// being an axis of its own: one shell in the panel refuses, and it stops.
// The status is the fatal one the vector already carries.
//
// Reading the *file* is what settled the panel, not the child's status: one
// bash build parks its own saved streams at descriptor 10, so `echo hi >&10`
// prints `hi` there and reports success without anything crossing a
// boundary. `<&10` in the same build is `Bad file descriptor`, which is the
// tell.
func (r *Runner) refuseWideDupTarget(target string) bool {
	if len(target) < 2 || !allDigits(target) {
		return false
	}
	if !r.ask(r.sem().MultiDigitDuplicationTargetIsAnError,
		"a duplication target of more than one digit") {
		return false
	}
	r.diagf("%s\n", Wording(r.diag().MultiDigitDuplicationTarget,
		"%[1]s: bad file descriptor number", target))
	r.fatalQuiet()
	return true
}

// errBadFd is what a duplication reports when the number it names is not
// open.
//
// The text is the errno's, not a spelling of its own, so it goes through the
// same wording as every other strerror a redirection quotes: the substrate
// capitalizes it and the dialect that lowercases everything gets to. Written
// out here in lowercase, it matched that one dialect and nobody else.
//
// The sentence around it is the dialect's too — ksh93 puts the errno in a
// bracket after a verb — and so is *which spelling of the target* it names:
// bash quotes the word the script wrote where the other two print the number
// it came to.
//
// written is the name to print, and empty means the number. Which it is has
// already been decided by dupTargetText, because the answer is not one flag:
// a *move* splits the two shells the other way round, with ksh93 quoting the
// word and bash naming the number. See Diagnostics.DuplicationSourceNotOpen,
// NamesTheDuplicationTargetAsWritten and NamesTheMoveSuffixInTheTarget.
func (r *Runner) errBadFd(fd int, written string, target int) error {
	name := itoa(fd)
	if written != "" {
		name = written
	}
	// The descriptor the redirection was *aiming at*, which one column names
	// beside the source and no other names at all. A verb rather than
	// something the sentence could work out, because it is not derivable from
	// the source: 0 for `<&`, 1 for a bare `>&`, and the number written in
	// front of the operator otherwise. See
	// Diagnostics.DuplicationSourceNotOpen.
	aimedAt := itoa(target)
	wording := r.diag().DuplicationSourceNotOpen
	if ceiling := r.sem().DescriptorNumberCeiling.number(); ceiling > 0 && fd >= ceiling {
		// Past the shell's own ceiling the refusal is about the *number*
		// rather than about what is open at it, and it carries no errno —
		// which is the one refusal about a descriptor that column writes
		// without one. An empty wording leaves the ordinary sentence
		// standing, which is what a shell with no ceiling wants and what the
		// core wants. See Semantics.DescriptorNumberCeiling.
		if over := r.diag().FdNumberOverCeiling; over != "" {
			wording = over
		}
	}
	return errors.New(Wording(wording, "%[1]s: %[2]s",
		name, r.diag().reasonText(reason(syscall.EBADF)), aimedAt))
}

// seekRedirect moves a descriptor's position, which is what `<#((expr))` and
// `>#((expr))` do and the whole of what they do: nothing is opened, nothing
// is closed, and the change outlives the command, because the position
// belongs to the open file and not to the number.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null:
//
//	printf abcdefghij > f; exec 3< f
//	read -n4 v <&3                     [abcd]
//	exec 3<#((0)); read -n2 v <&3      [ab]
//	exec 3<#((6)); read -n2 v <&3      [gh]
//	printf 0123456789 > g; exec 4<> g
//	exec 4>#((3)); printf XY >&4       g holds 012XY56789
//
// And it is not restored when a command that wrote one ends:
// `{ read -n2 v <&3; } 3<#((6))` leaves the next read of 3 at 8, where
// putting the position back would leave it at 2.
//
// A seek past the end is not an error — the read after it returns nothing at
// status 1 — and a negative offset is refused.
func (r *Runner) seekRedirect(rd *syntax.Redirect, fd int) bool {
	f, open := r.seekableFd(fd)
	if !open {
		// A different sentence from the one a duplication writes about a
		// number nothing is open at, which is measured: `exec 6>&7` is
		// `7: cannot open [Bad file descriptor]` and `exec 6<#((0))` is
		// `6: bad file unit number [Bad file descriptor]`.
		r.diagf("%s\n", Wording(r.diag().SeekDescriptorNotOpen, "%[1]s: %[2]s",
			itoa(fd), r.diag().reasonText(reason(syscall.EBADF))))
		r.status = r.diag().redirectFailureStatus()
		return false
	}
	cur, err := f.Seek(0, io.SeekCurrent)
	var end int64
	if err == nil {
		end, err = f.Seek(0, io.SeekEnd)
		if err == nil {
			_, err = f.Seek(cur, io.SeekStart)
		}
	}
	if err == nil && !regularFile(f) {
		// A character device the kernel *will* seek is still not a file with
		// a position in it, and this shell is the one that has to say so:
		// macOS takes an lseek on /dev/zero where ksh93 answers
		// `0: not seekable`. Asked only of an *os.File — an embedder's own
		// seekable stream is taken at its word.
		err = syscall.ESPIPE
	}
	if err != nil {
		// A pipe or a terminal has no position to move, and asking for one
		// is where that is found out. A different sentence from the one a
		// refused *offset* earns, which is measured: with standard input on
		// a pipe `exec 0<#((0))` is `0: not seekable`, and with it on a
		// regular file and a negative offset it is `-1: invalid seek
		// offset`.
		r.diagf("%s\n", Wording(r.diag().SeekStreamHasNoPosition, "%[1]s: not seekable", itoa(fd)))
		r.status = r.diag().redirectFailureStatus()
		return false
	}
	// CUR and EOF stand for the position and the size **inside the
	// expression only**: measured, `CUR=99; exec 3<#((CUR))` seeks to where
	// the descriptor stands and leaves `CUR` holding 99 afterwards, and
	// `exec 3<#((EOF-3))` reads the last three bytes. An assignment the
	// expression makes still reaches the shell — `exec 3<#((zz=6))` sets
	// `zz` — so the two names are saved and put back rather than evaluated
	// in a scope of their own.
	undo := []savedVar{r.saveVar("CUR"), r.saveVar("EOF")}
	r.setVar("CUR", strconv.FormatInt(cur, 10))
	r.setVar("EOF", strconv.FormatInt(end, 10))
	names, bad := r.redirectTargetForItsProcess(rd)
	r.restoreVars(undo)
	if bad {
		return false
	}
	written := strings.Join(names, " ")
	off, ok := atoiSigned(written)
	if !ok || off < 0 {
		return r.refuseSeekOffset(written)
	}
	if _, err := f.Seek(int64(off), io.SeekStart); err != nil {
		return r.refuseSeekOffset(written)
	}
	return true
}

// refuseSeekOffset reports an offset the descriptor will not take, which after
// the check above is a negative one and a seek the kernel refused outright.
// A stream with no position at all is the *other* sentence — see
// Diagnostics.SeekStreamHasNoPosition.
func (r *Runner) refuseSeekOffset(written string) bool {
	r.diagf("%s\n", Wording(r.diag().SeekOffsetRefused, "%[1]s: invalid seek offset", written))
	r.status = r.diag().redirectFailureStatus()
	return false
}

// regularFile reports whether a seekable stream is a file with a position in
// it, rather than a device that merely tolerates the call. Anything that is
// not one of this process's own files is taken at its word: an embedder may
// hand a Runner a stream of its own, and this has nothing to ask it.
func regularFile(s io.Seeker) bool {
	f, ours := s.(*os.File)
	if !ours {
		return true
	}
	st, err := f.Stat()
	return err == nil && st.Mode().IsRegular()
}

// seekableFd is the open file behind one of this shell's descriptor numbers,
// where there is one that can be positioned.
//
// The named three answer for themselves and everything above them comes out
// of the table, which is the same split [Runner.SystemDescriptor] makes. A
// here-document's text, an embedder's buffer and a pipe are all "not one"
// here: none has a position, and saying so is the honest answer rather than
// seeking something that is not the file the script meant.
func (r *Runner) seekableFd(fd int) (io.Seeker, bool) {
	var held any
	switch fd {
	case 0:
		held = r.stdin()
	case 1:
		held = r.stdout()
	case 2:
		held = r.stderr()
	default:
		var open bool
		if held, open = r.fds[fd]; !open {
			return nil, false
		}
	}
	s, ok := held.(io.Seeker)
	return s, ok
}

// dupFd points one descriptor at another, or closes it.
//
// 0, 1 and 2 are the named streams; everything above them lives in the
// descriptor table, which is what makes `exec 6>&1; echo hi >&6` work. A
// caller redirecting into the table must call the applyRedirs save first,
// so the write can be taken back when the command ends.
//
// opened is the command's target set, and a *write* duplication of a named
// stream joins it exactly as an opened file does. The dialect that writes to
// every target of a repeated redirection was dropping `>&` from the fan-out,
// because this path rebound the stream and the fan-out never learned of it:
// `echo x >&1 >b` wrote the file and nothing else where zsh writes both
// (#1261). What the duplication contributes is the descriptor *as it stands
// now*, which is why `echo x >b >&1` fills the file twice there — by then
// standard output already is the fan-out, so duplicating it adds the file a
// second time. Measured; and the order dependence is the loop's, not a rule
// of its own.
//
// A close empties the set rather than adding to it, which is the same
// measurement read from its other end: `echo x >b >&- >c` leaves `b` empty
// and puts the line in `c` alone, so the targets named before the close are
// not merely bypassed, they are gone.
//
// The reading side stays out of it. Input has a fan-out of its own in that
// dialect — `cat <a <b` reads both files in order — and this shell has none,
// for either operator, so joining `<&` to a set nothing else fills would
// model half of a feature.
func (r *Runner) dupFd(fd int, target, written string, opened map[int]io.Writer) error {
	if target == "-" {
		// Whatever this command had aimed at the number, it no longer has.
		delete(opened, fd)
		// What the number was aimed at, read before it stops being aimed at
		// it: a pipe this shell made is closed for real once nothing else
		// names it, which is what delivers the end-of-file a `>(cmd)` body
		// is reading until. See Runner.closeOwnPipe, and note the order —
		// the slot is emptied first, or the aliasing check would find the
		// entry being dropped and call the file shared with itself.
		dropped := r.fds[fd]
		switch fd {
		case 0:
			dropped = r.Stdin
			r.Stdin = closedFd{}
		case 2:
			dropped = r.Stderr
			r.Stderr = closedFd{}
		case 1:
			dropped = r.Stdout
			r.Stdout = closedFd{}
			// Recorded, because one dialect stays quiet about a failed write
			// exactly when the command that wrote closed the stream itself.
			// See Runner.outputClosedByThisCommand.
			r.outputClosedByThisCommand = true
		default:
			// Closing a descriptor that was never open is not an error in
			// any shell measured, so neither is deleting a missing entry.
			delete(r.fds, fd)
		}
		r.closeOwnPipe(dropped)
		return nil
	}
	m, ok := atoi(target)
	if !ok {
		return fmt.Errorf("%s: ambiguous redirect", target)
	}
	var src any
	switch m {
	case 0:
		src = r.stdin()
	case 1:
		src = r.stdout()
	case 2:
		src = r.stderr()
	default:
		v, held := r.fds[m]
		if !held {
			return r.errBadFd(m, written, fd)
		}
		src = v
	}
	// A descriptor the script closed is not a descriptor. `N>&M` reads M
	// before it writes fd, and a closed M fails the *redirection* — the
	// command never runs, so there is nothing for a write to fail at.
	//
	// The two are easy to conflate because both end in EBADF, and the
	// conflation is what #1345 was: closedFd both reads and writes, so the
	// duplication above copied it happily and the command then failed on the
	// write it did attempt. That answers a script correctly only by accident,
	// and stops being an accident the moment the command writes nothing:
	// `exec 2>&-; true >&2` is a failure in all six of the panel and was a
	// silent success here, in every dialect.
	//
	// A number above two is already refused this way, by being absent from
	// the table — see the default arm. Only the three named streams could be
	// *present and closed*, which is the whole of why the check is here and
	// not there.
	if _, closed := src.(closedFd); closed {
		return r.errBadFd(m, written, fd)
	}
	switch fd {
	case 0:
		rd, ok := src.(io.Reader)
		if !ok {
			return r.errBadFd(m, written, fd)
		}
		r.Stdin = rd
	case 2, 1:
		w, ok := src.(io.Writer)
		if !ok {
			return r.errBadFd(m, written, fd)
		}
		// A target like any other, so a second one joins the first where the
		// dialect joins them and replaces it everywhere else.
		w = r.eachTarget(fd, w, opened)
		if fd == 2 {
			r.Stderr = w
		} else {
			r.Stdout = w
		}
	default:
		r.setFd(fd, src)
	}
	return nil
}

// nextFreeFd is the number the shell picks for `{name}>f`: the first free
// entry from the dialect's base up, clear of the single digits a script
// addresses itself. Where that base is is a disagreement — see
// Semantics.FirstAllocatedDescriptor.
//
// freeing is a number to count as free although it is held, or -1 for none.
// It is the source of a relocating `{name}<&$w-`, whose close has not
// happened yet and cannot: the source has to be read before it is given up.
// Counting it free here is the same answer as closing it first, without the
// ordering that would need — and the answer is observable, since that form
// hands the name the number it moved from where the duplicating one hands it
// the next number up.
func (r *Runner) nextFreeFd(freeing int) int {
	if freeing >= 0 && r.sem().FdMove != FdMoveRelocates {
		freeing = -1
	}
	fd := r.sem().FirstAllocatedDescriptor.number()
	for {
		if _, held := r.fds[fd]; !held || fd == freeing {
			return fd
		}
		fd++
	}
}

// dupTargetText is the target as written, for the message a duplication makes
// when nothing is open at the number it names.
//
// A move is where the two shells that have one disagree about what "as
// written" means. bash reads the `-` as the operator's and names the number
// alone — `exec 6<&5-` with 5 closed is `5: Bad file descriptor` — where
// ksh93 quotes the whole word it was handed and says `5-: cannot open`. Both
// name what they parsed; they parsed the same text into different pieces.
func (r *Runner) dupTargetText(rd *syntax.Redirect, moveFrom int) string {
	if moveFrom >= 0 {
		if r.diag().NamesTheMoveSuffixInTheTarget {
			return rd.Text
		}
		return ""
	}
	if r.diag().NamesTheDuplicationTargetAsWritten {
		return rd.Text
	}
	return ""
}

// shellOwnedFd marks a descriptor the shell opened for its own plumbing
// rather than one the script asked for by number.
//
// A coprocess's near ends are the only case so far. They are in the table so
// that `>&${C[0]}` can find them, but they are not the script's to hand out,
// and every descriptor in the table is otherwise rebuilt into an external
// child's — see childFiles. Inheriting these would be worse than untidy: a
// child holding the write end open means the coprocess never reads
// end-of-file, which is the same failure a process substitution's writing end
// has and answers the same way — by reaching exactly the commands it is meant
// to and no others. See newProcSubPipe.
//
// It is what bash does, measured rather than assumed, and measured on the
// harder half: with a coprocess running, an external child finds nothing
// open on either number the shell reports in ${C[0]} and ${C[1]} — and
// nothing open on 3 after `exec 3>&${C[1]}` either, though the shell itself
// still writes through that 3 and the coprocess still receives it. So the
// mark travels with a duplication rather than being shed by one, which is
// what a wrapper value gives for free.
//
// The wrapper *is* the mark. It still reads, writes and closes as the file
// does, so everything reaching through the table is unaffected; it is simply
// not an *os.File, which is the one question childFiles asks.
type shellOwnedFd struct{ *os.File }

// setFd records a descriptor beyond the named three.
func (r *Runner) setFd(fd int, v any) {
	if r.fds == nil {
		r.fds = map[int]any{}
	}
	r.fds[fd] = v
	// A number written again is a different descriptor, and a close-on-exec
	// mark is about the one that was there. See
	// Runner.KeepDescriptorFromChildren, which puts it on after this.
	delete(r.cloexecFds, fd)
}

// builtinWriteStatus folds a failed output write into a builtin's status.
//
// The write already happened and already failed — into a descriptor closed
// with `>&-`, most plainly — so there is nothing to undo; the question is
// whether the command is said to have worked. Three of the panel say no and
// report 1, and two of those say so on stderr; zsh keeps the builtin's own
// status and quietly loses the text. So the status is a semantics axis and
// the message is the dialect's wording, empty where nothing is said.
//
// A builtin that already failed keeps its own status: the write's 1 only
// replaces a success, never a complaint the builtin had already made.
//
// A broken pipe is not one of these and is answered before any of it. Writing
// into a pipe nobody is reading is how `yes | head` ends the thing writing,
// and a real shell's builtin is killed by SIGPIPE where it stands: it says
// nothing and it does not carry on. That is unanimous, and measured — a
// builtin writing more than a pipe will hold into `| true` leaves an empty
// standard error and never reaches the next command, in all four shells.
//
// We reached neither answer. Two dialects announced the write, in the wording
// meant for `>&-`, and all four then ran the rest of the shell that should
// have died — which is also why the corpus wandered: in `printf x | { read -d
// : v; }` the reader refuses the option and leaves, and whether the writer's
// small write beats the closing read end is a coin flip the machine gets to
// call. Idle it never lost; under sixteen spinning loads it lost 3 times in
// 200, and the reference dash lost 0 in 200 either way.
//
// The death is conditional on the signal, and that is the half this missed.
// EPIPE is not the death; the death is SIGPIPE, and a write only raises a
// fatal one because nothing is stopping it. A script that ignores SIGPIPE
// stops it, and so does one that handles it, and the kernel then hands the
// errno back to a writer that is still running. Measured on a builtin writing
// more than a pipe will hold into a reader that has gone, and the panel is
// unanimous on the part that matters: with SIGPIPE ignored, and again with it
// handled, every one of dash, bash 5.3, bash 3.2, ksh93 and zsh reaches the
// command after the failed write. Four of them also say something, in the same
// wording they use for `>&-`, which is what puts this back on the ordinary
// path rather than on one of its own — and where a handler is set, all four
// run it.
//
// So EPIPE asks about the disposition rather than about the errno. Only the
// default action is a death; anything the script arranged leaves an ordinary
// failed write, where the existing axis decides the status and the existing
// wording decides the text, and a handler is delivered the way every other
// handler is — recorded here and run between commands.
//
// Which disposition is in force is a question about *where* the write
// happened, and the trap table answers it: a pipeline element carries an
// inherited ignore and not an inherited handler, so a handled SIGPIPE outside
// the pipeline leaves the writer inside it dying exactly as an untrapped one
// does. That is measured too.
func (r *Runner) builtinWriteStatus(name string, st int) int {
	err := r.writeFailed
	r.writeFailed = nil
	if err == nil || r.unspecified {
		return st
	}
	if errors.Is(err, syscall.EPIPE) {
		arranged := r.signalArranged("PIPE")
		if arranged == signalFatal {
			r.signalDeath("PIPE", syscall.SIGPIPE)
			return r.status
		}
		// Not a death, so the signal is answered here rather than left to the
		// runtime's copy of it. A handler runs between commands and not at
		// the write, so it is delivered rather than run — to whichever shell
		// set it, which for an element that trapped PIPE for itself is the
		// element. The *outer* shell's handler is nobody's here, and stays
		// so: signalArranged read the element's own table, so an inherited
		// handler was never one of these.
		r.brokenPipeAbsorbed(arranged == signalHandledBy)
	}
	// Which axis decides is the errno's, not one axis for every failed write:
	// a closed descriptor and a pipe nobody is reading are the same event to
	// five of the panel and opposite events to the other two. See
	// Semantics.BrokenPipeWriteErrorFailsTheCommand for the measurement.
	fails := false
	if errors.Is(err, syscall.EPIPE) {
		fails = r.ask(r.sem().BrokenPipeWriteErrorFailsTheCommand,
			"a builtin's failed write into a broken pipe failing the command")
	} else {
		fails = r.ask(r.sem().BuiltinWriteErrorFailsTheCommand,
			"a builtin's failed write failing the command")
	}
	// The builtin's own complaint comes first where both are said. Measured
	// 2026-09-12 on the one dialect that has both sentences, writing into a
	// broken pipe: `zsh:echo:6: write error: broken pipe` and then
	// `zsh:6: write error: broken pipe`, in that order and for `printf` as
	// well as `echo`.
	if fails {
		if w := r.diag().BuiltinWriteError; w != "" {
			r.diagf("%s\n", fmt.Sprintf(w, name, r.diag().reasonText(reason(err))))
		}
	}
	// Said whether or not the axis failed the command, and that is the whole
	// reason this is a second wording. zsh answers the closed-descriptor axis
	// No and would otherwise never reach BuiltinWriteError; it still says
	// something on every route but one, and the route it stays quiet on is
	// the one where the writing command's own redirections closed the stream.
	// See Diagnostics.InheritedClosedStreamWriteError for the measurements.
	if w := r.diag().InheritedClosedStreamWriteError; w != "" && !r.outputClosedByThisCommand {
		// Not the builtin's own complaint, and the dialect that has this
		// sentence says so by leaving the builtin out of the location:
		// `exec 1>&-; echo hi` is `zsh:1: write error: …` where echo's own
		// messages are `zsh:echo:1: …`, and inside a function it is
		// `f: write error: …`. Measured 2026-09-12.
		outer := r.inBuiltin
		r.inBuiltin = ""
		r.diagf("%s\n", fmt.Sprintf(w, name, r.diag().reasonText(reason(err))))
		r.inBuiltin = outer
	}
	if !fails {
		return st
	}
	if st == 0 {
		st = 1
	}
	return st
}

// closedFd is a descriptor that has been closed with `>&-`. Reading or writing
// it fails the way the kernel would.
//
// It is a marker as much as a stream. Everything the shell does through the
// descriptor itself already fails here, but the value also has to survive as
// far as childIn and childOut, which is what tells an external command that
// the number is closed rather than merely empty. So nothing wraps one — see
// lockWriter, where the same rule already keeps a real file unwrapped.
type closedFd struct{}

func (closedFd) Write([]byte) (int, error) { return 0, syscall.EBADF }
func (closedFd) Read([]byte) (int, error)  { return 0, syscall.EBADF }

// readOnlyStream is a named stream a here-document was written to, which is
// open for reading and not for writing.
//
// It exists for `1<<X` and `2<<X` alone — a document aimed at a number the
// shell keeps as a writer. Everywhere else a read-only descriptor is an
// *os.File the kernel refuses the write on, and there is no file here: the
// body is text this process holds, so the refusal has to be written down.
type readOnlyStream struct{ io.Reader }

func (readOnlyStream) Write([]byte) (int, error) { return 0, syscall.EBADF }

// closedInChild is the value that says *closed there* to a process this shell
// starts, and it is a nil *os.File because that is the only spelling there is:
// [os.ProcAttr] documents a nil entry in Files as the descriptor being closed
// when the process starts, and os/exec hands an *os.File through to
// [os.StartProcess] untouched. The table above descriptor 2 already says it
// this way — see childFiles, where a gap is a nil and a nil is a close.
//
// Never assigned. It is a typed nil rather than an untyped one because the
// type is the whole of what it carries.
var closedInChild *os.File

// childIn and childOut are a named stream on its way to an external command:
// the stream itself, or a closed descriptor where the script closed one.
//
// Handing closedFd over unchanged is the silent-wrong-answer bug #1260, and
// the mechanism is worth writing down because nothing about it looks wrong at
// the call site. os/exec connects a child straight to an *os.File and builds a
// pipe for anything else, copying between the two; closedFd is not a file, so
// the child was given a pipe, and the first copy failed with EBADF and closed
// it. The child then read **end-of-file** — an *empty* descriptor, which is a
// different thing from a closed one and the one thing it must not be confused
// with. `exec 0<&-; cat` reported success and printed nothing, where dash,
// bash 5.3, bash 3.2, ksh93 and zsh all say `cat: stdin: Bad file descriptor`
// and exit 1. Unanimous, so this is the core's answer and not a dialect's.
//
// Both directions, and the write side is the same hole rather than a
// sympathetic fix: `exec 1>&-; /bin/echo hi` answered 0 in silence against a
// unanimous `echo: fflush: Bad file descriptor` at 1.
//
// The two halves of the descriptor table already agreed on this and only the
// named streams did not, which is why the fix is here and not deeper: a
// *numbered* descriptor closed with `exec 3>&-` reaches a child closed, and so
// does a named stream handed to a *replacement*, because both of those routes
// go through the nil-is-a-close table. Only the forked child's 0, 1 and 2 are
// built by os/exec, and that is the one place the marker was being spent.
func childIn(v io.Reader) io.Reader {
	if _, closed := v.(closedFd); closed {
		return closedInChild
	}
	return v
}

func childOut(v io.Writer) io.Writer {
	if _, closed := v.(closedFd); closed {
		return closedInChild
	}
	return v
}

// fdVarValue reads the descriptor number a `{name}` token names.
//
// The name may carry a subscript where the dialect allows one — `{a[1]}` is
// the element, and `exec {COPROC[1]}>&-` is how a coprocess's feed is closed
// by the array the shell put its near ends in. The lexer has already decided
// that the brackets are part of the name; what arrives here is the text
// between the braces, so this is where a name and an element part company.
//
// The subscript is read the way every other subscript in this package is: a
// declared associative name takes it as a key and any other takes it as an
// expression, which is what makes `{a[i+1]}` mean what `${a[i+1]}` means.
func (r *Runner) fdVarValue(ref string) (string, bool) {
	if n, ok := positionalFdVar(ref); ok {
		if n < 1 || n > len(r.Params) {
			return "", false
		}
		return r.Params[n-1], true
	}
	base, sub, ok := r.subscriptOperand(ref)
	if !ok {
		return r.getVar(ref)
	}
	if r.assocDeclared(base) {
		v, held := r.AssocArrays[base][sub]
		return v.scalar(), held
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		return "", false
	}
	elems, isArray := r.arrayElems(base)
	if !isArray {
		return "", false
	}
	return r.elemAt(base, elems, idx)
}

// setFdVar gives the name the number the shell picked, which is the other
// half of the same rule: `exec {a[2]}>f` opens the file and leaves the
// descriptor in that element.
//
// storeThroughOperand and not a resolution of its own, because "put this
// value where that name points" has to mean one thing wherever it is
// spelled: a declared table takes the subscript as a key, a pair takes the
// span, and a single subscript reaches setArrayElem — which is where a name
// holding a *string* has the value spliced into its characters rather than
// becoming an array. A second walk of the brackets here is how one of the
// two would come to answer `{buf[$#buf+1]}` differently from `buf[$#buf+1]`.
// It reports whether the number landed. A store the shell refuses is the
// redirection failing, and a redirection that failed is a command that does
// not run: measured 2026-09-17 on bash 5.3.20, `declare -n s; { echo z; }
// {s}>/dev/null` prints no `z` and leaves 1 behind. Here the group ran and
// the status was 0, so a `{name}>` aimed at a name the shell cannot write
// opened the file and carried on as if it had.
//
// The refusal is **two sentences** there and this wrote one. The first is
// the store's own — whatever it is refused for — reported under the command
// word the redirection belongs to, which is r.redirForCommandWord; the
// second is this one, naming the operand as written. Measured on bash
// 5.3.20, a script file:
//
//	declare -n s; exec {s}>/dev/null    exec: `10': not a valid identifier
//	                                    s: cannot assign fd to variable    1
//	declare -n s; true {s}>/dev/null    true: `10': …  + the same second     1
//	declare -n s; { echo z; } {s}>…     `10': …        + the same second     1
//	readonly s=1; exec {s}>/dev/null    s: readonly variable
//	                                    s: cannot assign fd to variable    1
//
// So the second sentence is written for **any** refused store and not for
// one of them, and the first names the command word where there is one —
// which is why the brace group's is unprefixed and the external command's
// says `/bin/echo`. See Diagnostics.CannotAssignFdToVariable (#3491).
func (r *Runner) setFdVar(ref, value string) bool {
	if n, ok := positionalFdVar(ref); ok {
		// A positional parameter receives the number, which is how a
		// function that was handed a descriptor hands back the one the
		// shell picked. Out of range writes nothing: there is no position
		// to widen the list to that a later `shift` would keep straight.
		if n >= 1 && n <= len(r.Params) {
			r.Params[n-1] = value
		}
		return true
	}
	// The command word speaks for the store's own refusal, which is where
	// the shell puts it: put aside and given back, the way every other site
	// that lends a speaker does it.
	outerSpeaker, outerFailed := r.fdVarSpeaker, r.assignFailed
	// The transfer the store may raise is put aside with them. A name the
	// shell will not write is refused the way an assignment to it is, and an
	// assignment to a readonly name gives up the function it is in — but a
	// *redirection* aimed at one does not: measured 2026-09-22 on bash
	// 5.3.20, `readonly v=42; f() { exec {v}>>/dev/null; echo after; }; f`
	// prints both sentences, then `after`, and leaves the function at 0,
	// where the bare `v=1` in the same place ends the function at 1. So the
	// refusal belongs to the redirection and the transfer is not the
	// redirection's to carry.
	outerCtl, outerDepth := r.ctl, r.ctlDepth
	r.fdVarSpeaker, r.assignFailed = r.redirForCommandWord, false
	_, refused := r.storeThroughOperand(ref, value)
	failed := refused || r.assignFailed || r.ctl == controlExit
	r.fdVarSpeaker, r.assignFailed = outerSpeaker, outerFailed || failed
	if !failed {
		return true
	}
	r.ctl, r.ctlDepth = outerCtl, outerDepth
	if w := r.diag().CannotAssignFdToVariable; w != "" {
		r.diagf("%s\n", Wording(w, "%[1]s: cannot assign fd to variable", ref))
	}
	return false
}

// positionalFdVar reads a `{name}` that names a positional parameter.
//
// All digits or nothing, which is measured: `{01}` and `{99}` name
// positions in the one shell that takes them, and `{1a}` and `{1_}` are
// refused there as `not an identifier`. `{0}` is a position like any other
// to this reader — `$0` is not in Params, so it simply has no descriptor,
// which is the same answer a name nobody set gets.
//
// The lexer admits a digit-leading name only where the dialect says so —
// see syntax.Dialect.FdVariablePositional — so a shell without the feature
// never reaches here with one.
func positionalFdVar(ref string) (int, bool) {
	if ref == "" {
		return 0, false
	}
	for i := range len(ref) {
		if ref[i] < '0' || ref[i] > '9' {
			return 0, false
		}
	}
	n, ok := atoi(ref)
	return n, ok
}

// redirWrote records that the redirections being applied have just written a
// descriptor beyond the three named streams.
//
// Two things come of it. The caller learns which numbers to mark when the
// redirections turn out to be `exec`'s and outlive the command; and the mark
// comes *off* here, because a command redirecting a number `exec` had opened
// is using that number for itself. Measured: `exec 3>f` keeps the descriptor
// from an external command in one dialect, and `exec 3>f; cmd 3>&3` hands it
// over there after all — restating the number on the command is what brings
// it back. The save puts the mark on again when the command ends.
func (r *Runner) redirWrote(fd int) {
	if fd <= 2 {
		return
	}
	r.redirFds = append(r.redirFds, fd)
	delete(r.execFds, fd)
}

// markExecOpened records that these numbers were opened by `exec`'s own
// redirection list, which is the one thing that distinguishes them from any
// other descriptor in the table.
//
// Called where the redirections are found to outlive their command, because
// that is what `exec` is: the caller keeps them rather than closing them, and
// only then is it known that this was not an ordinary command's redirect.
// A number that was closed rather than opened is not marked — the mark is
// about a descriptor that is there to hand over.
func (r *Runner) markExecOpened(fds []int) {
	for _, fd := range fds {
		if _, held := r.fds[fd]; !held {
			continue
		}
		if r.execFds == nil {
			r.execFds = map[int]bool{}
		}
		r.execFds[fd] = true
	}
}

// GreatAmpTargetForm is what `>&word` does with a word that is not a
// descriptor number.
//
// A form rather than a flag because the three answers are three different
// things, and because the difference between two of them is a word that
// expanded to nothing — which is exactly what a script reaches when it writes
// `>&"${COPROC[1]}"` in a shell that has no such array.
//
// Measured 2026-09-06 with `echo hi >&qq` in an empty directory: bash 5.3.15,
// bash 3.2.57, bash 5.3.15 run as `sh` and zsh 5.9.2 create the file and put
// `hi` in it; ksh93u+ refuses with `qq: bad file unit number` and carries on;
// dash refuses the *word* while parsing, so nothing in the line runs. The
// core is the intersection, and the intersection has no such form.
type GreatAmpTargetForm int

const (
	// GreatAmpTargetUnspecified is no answer, and is refused like any other.
	GreatAmpTargetUnspecified GreatAmpTargetForm = iota
	// GreatAmpTargetIsADescriptor keeps `>&` a duplication and nothing else:
	// a word that is neither a number nor `-` is refused. ksh93 and dash.
	GreatAmpTargetIsADescriptor
	// GreatAmpTargetNamesAFile reads a word that named something as a file
	// and sends *both* output streams to it, which is the csh spelling of
	// `&>word` — and it is that spelling exactly, down to honoring `set -C`
	// and reporting a failed open in the ordinary words. A word that expanded
	// to nothing is still refused, because there is no name in it. bash.
	GreatAmpTargetNamesAFile
	// GreatAmpTargetNamesAnyFile is the same, and takes a word that expanded
	// to nothing as a name too: the open is attempted and fails on the empty
	// path, which is why zsh answers `>&""` with `no such file or directory:`
	// and nothing after the colon.
	GreatAmpTargetNamesAnyFile
)

func (f GreatAmpTargetForm) String() string {
	switch f {
	case GreatAmpTargetIsADescriptor:
		return "GreatAmpTargetIsADescriptor"
	case GreatAmpTargetNamesAFile:
		return "GreatAmpTargetNamesAFile"
	case GreatAmpTargetNamesAnyFile:
		return "GreatAmpTargetNamesAnyFile"
	}
	return "GreatAmpTargetUnspecified"
}

// FdMoveForm is what a trailing `-` on a duplication target means: `6<&5-`
// and `6>&5-`, the operators that make 6 a copy of 5 and close 5 in one
// step, so that a script moves a descriptor rather than leaving two names
// for one open file.
//
// A form rather than a flag because the two shells that have it disagree
// about what the move *is*, and the disagreement is visible twice from one
// answer — see the two constants. Measured 2026-09-12 across the panel:
//
//	exec 5< f; exec 6<&5-     bash 5.3, bash-as-sh, bash 3.2 and ksh93 move
//	                          it at status 0; zsh answers `file number
//	                          expected` at 1; dash answers `Syntax error:
//	                          Bad fd number` at 2 and ash `redir error` at 2
//
// None of the three refusals is a parse refusal, though two are worded as
// one: the same text inside `if false; then … fi` runs clean in every column,
// so the question belongs here and not to syntax.Dialect.
//
// The core has no such form. The intersection of six shells does not contain
// it, and the three that refuse do not agree on what to say or what status
// to end at.
type FdMoveForm int

const (
	// FdMoveUnspecified is no answer, and is refused like any other.
	FdMoveUnspecified FdMoveForm = iota
	// FdMoveIsNotAnOperator leaves the `-` in the target word, where
	// whatever refuses a word that is not a descriptor refuses it. zsh,
	// dash and ash — and their three refusals differ, which is the point of
	// routing them all back through the existing path rather than giving
	// the move a refusal of its own.
	//
	// zsh is the one worth naming, because it does not merely refuse: `<&`
	// wants a number there and says so, while `>&5-` falls to that shell's
	// csh reading of `>&word` and opens a file named `5-`. Both are the
	// absence of this operator rather than two answers to it.
	FdMoveIsNotAnOperator
	// FdMoveDuplicatesThenCloses is two steps in written order: copy the
	// source onto the destination, then close the source — and only the
	// copy is a redirection the command takes back. bash.
	//
	// Both halves are observable. `exec 5<f; true 6<&5-` leaves 5 closed
	// once the command has ended, where the plain close `true 5<&-` is
	// undone like any other redirection; and `{v}<&$w-` picks the
	// destination number before the source is closed, so the name receives
	// a number one higher than the one it moved from.
	FdMoveDuplicatesThenCloses
	// FdMoveRelocates is one operation: the descriptor changes number.
	// Both halves are the command's redirection, so both are taken back —
	// `exec 5<f; true 6<&5-` leaves 5 open — and the number the source
	// gives up is free for `{v}` to receive, so `{v}<&$w-` answers with
	// `$w`'s own number. ksh93.
	FdMoveRelocates
)

func (f FdMoveForm) String() string {
	switch f {
	case FdMoveIsNotAnOperator:
		return "FdMoveIsNotAnOperator"
	case FdMoveDuplicatesThenCloses:
		return "FdMoveDuplicatesThenCloses"
	case FdMoveRelocates:
		return "FdMoveRelocates"
	}
	return "FdMoveUnspecified"
}

// DuplicationTargetErrorForm is what becomes of the shell when `<&word` or
// `>&word` names something that is not a descriptor.
//
// Every shell in the panel refuses the word. What they do next splits them
// three ways, and one of the three is conditional on the *command* the
// redirection was written on, which is why this cannot be a flag: two
// independent flags would admit a shell that is fatal on a builtin and fatal
// everywhere at once, which is a reading nothing exhibits.
//
// Measured 2026-09-12 and 2026-09-13 with `exec 6<&qq; echo "st=$?"; echo
// reached` in an empty directory:
//
//	bash 5.3, bash-as-sh, bash 3.2  `qq: ambiguous redirect`, status 1, on it goes
//	ksh93                           `qq: bad file unit number`, status 1, on it goes
//	zsh                             `file number expected`, status 1, and the
//	                                shell ends — but only on a builtin
//	dash                            `Syntax error: Bad fd number`, status 2, over
//	ash                             `redir error`, status 2, over
//
// **Neither of the last two is a parse refusal**, though both are worded as
// one: `if false; then exec 6<&qq; fi; echo reached` prints `reached` and
// exits 0 in both, and `echo A; echo hi >&qq` prints `A` first. So the
// grammar takes the text everywhere and the answer is the vector's — the same
// reasoning, and the same probe, as MultiDigitDuplicationTargetIsAnError.
//
// The status of a shell that stops is FatalErrorStatusIsOne's, which is why
// dash and ash exit 2 without this needing a status of its own; and a
// subshell that stops takes only itself, which is what `( exec 6<&qq ); echo
// reached` shows in both.
type DuplicationTargetErrorForm int

const (
	// DuplicationTargetErrorUnspecified is no answer, and is refused like
	// any other.
	DuplicationTargetErrorUnspecified DuplicationTargetErrorForm = iota
	// DuplicationTargetErrorCarriesOn reports the word at status 1 and runs
	// the next command. bash and ksh93, and the standard's reading: XCU
	// makes a redirection error fatal for a special builtin alone, which
	// RedirectErrorOnSpecialBuiltinFatal already answers.
	DuplicationTargetErrorCarriesOn
	// DuplicationTargetErrorEndsTheShellOnABuiltin reports it at status 1
	// and ends a non-interactive shell, but only where the command it is
	// written on runs *in* the shell. zsh alone, and the boundary is the
	// command rather than the redirection: measured 2026-09-06, `cat <&""`
	// and `/bin/echo hi <&""` complain and carry on, while `read x <&""`,
	// `echo hi <&""`, `true <&""` and `: <&""` end it — the same word, the
	// same complaint, and a builtin on the left.
	//
	// Not RedirectErrorOnSpecialBuiltinFatal, which zsh answers No and which
	// would not reach `read` or `echo` in any case. Nor is it redirection
	// failure in general: an ordinary one on a zsh builtin — `read x
	// 3>/nope/x`, `read x <&9` — complains and carries on there too.
	DuplicationTargetErrorEndsTheShellOnABuiltin
	// DuplicationTargetErrorEndsTheShell reports it and ends the shell
	// whatever the command was — a builtin, a function or `/bin/echo` — at
	// the fatal status. dash and ash.
	DuplicationTargetErrorEndsTheShell
)

func (f DuplicationTargetErrorForm) String() string {
	switch f {
	case DuplicationTargetErrorCarriesOn:
		return "DuplicationTargetErrorCarriesOn"
	case DuplicationTargetErrorEndsTheShellOnABuiltin:
		return "DuplicationTargetErrorEndsTheShellOnABuiltin"
	case DuplicationTargetErrorEndsTheShell:
		return "DuplicationTargetErrorEndsTheShell"
	}
	return "DuplicationTargetErrorUnspecified"
}

// duplicationTargetError resolves the axis, and only where a word after `<&`
// or `>&` really has been refused. `<&2` is nobody's question, so a dialect
// that has not answered this still duplicates.
func (r *Runner) duplicationTargetError() DuplicationTargetErrorForm {
	f := r.sem().DuplicationTargetError
	if f == DuplicationTargetErrorUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a duplication target that is not a descriptor")))
		r.status = 2
		r.unspecified = true
	}
	return f
}

// fdMoveSource splits `5-` into the descriptor it names and the fact that a
// move was written. It answers only for a run of digits followed by the
// suffix, which is what keeps the axis from being asked about `-` on its own
// — the plain close, which every shell in the panel has — or about any other
// word ending in a dash.
func fdMoveSource(word string) (string, bool) {
	src, ok := strings.CutSuffix(word, "-")
	if !ok || src == "" || !allDigits(src) {
		return "", false
	}
	return src, true
}

// fdMove resolves the axis, and only where a trailing `-` has actually been
// written after a run of digits. `6<&5` is nobody's question, so a dialect
// that has not answered this still duplicates.
func (r *Runner) fdMove() FdMoveForm {
	f := r.sem().FdMove
	if f == FdMoveUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a trailing `-` on a duplication target")))
		r.status = 2
		r.unspecified = true
	}
	return f
}

// noclobberBlocksAppend reports whether `set -C` stops an append from
// creating a file, and whether the axis had no answer to give.
//
// Asked only under noclobber, which is the only place the axis decides
// anything: a shell that clobbers freely must not be made to answer a
// question about a refusal it never reaches, and an unanswered axis refuses.
// `>>` is the commonest redirection there is, so asking it unconditionally
// would make a Runner built from a bare Semantics unable to append at all.
//
// The second result is read from the axis rather than from r.unspecified,
// which is sticky: a caller that tested the flag would refuse an operator it
// never asked about as soon as anything earlier had left it set.
func (r *Runner) noclobberBlocksAppend() (blocks, unanswered bool) {
	if !r.noclobber {
		return false, false
	}
	a := r.sem().NoclobberBlocksAppendCreate
	blocks = r.ask(a, "whether noclobber stops an append from creating a file")
	return blocks, a == Unspecified
}

// isDescriptorSpec says the word after `>&` or `<&` is asking for a
// descriptor rather than naming anything: a run of digits, or the `-` that
// closes one.
//
// The width is not asked here. `>&10` is a descriptor spec everywhere, and
// the one dialect that will not take a target wider than a digit refuses it
// as a *number* it does not like — see refuseWideDupTarget — rather than by
// reading it as a filename.
func isDescriptorSpec(word string) bool {
	return word == "-" || (word != "" && allDigits(word))
}

// greatAmpNamesAFile answers whether this `>&word` is the csh spelling of
// `&>word` — see GreatAmpTargetForm.
//
// The leading descriptor number is part of the question rather than the whole
// of it, and the two forms split on it. Measured 2026-09-13 with `echo hi
// 2>&qq` in an empty directory: bash 5.3.15, bash 3.2.57 and bash-as-sh
// answer `qq: ambiguous redirect`, BusyBox ash answers `redir error` and
// ksh93 `qq: bad file unit number`, all with no file left behind — where the
// bare `>&qq` writes one in every column but ksh93's. zsh 5.9.2 creates the
// file for both spellings.
//
// So a number refuses under GreatAmpTargetNamesAFile and opens under
// GreatAmpTargetNamesAnyFile. This used to be written down as an invariant of
// the operator, which made zsh answer `file number expected` for a line real
// zsh runs (#2494).
//
// **A written `1` is not a number here.** `1>&qq` is the bare `>&qq` in bash
// and in ash — both streams into the file, at status 0 — measured the same
// day, and only a *different* number is refused. Reading "a descriptor was
// written" as the question refuses a spelling three of the columns run, which
// is the same over-generalization one step along.
func (r *Runner) greatAmpNamesAFile(rd *syntax.Redirect, fd int, fdVar, target string) bool {
	if rd.Op != syntax.TokGreatAmp || isDescriptorSpec(target) {
		return false
	}
	switch r.greatAmpTarget() {
	case GreatAmpTargetNamesAFile:
		return (rd.N == nil || (fdVar == "" && fd == 1)) && target != ""
	case GreatAmpTargetNamesAnyFile:
		return true
	}
	return false
}

// greatAmpTarget resolves the axis, and only where a `>&` really does name
// something other than a descriptor. `>&2` is nobody's question, so a dialect
// that has not answered this still runs it.
func (r *Runner) greatAmpTarget() GreatAmpTargetForm {
	f := r.sem().GreatAmpTarget
	if f == GreatAmpTargetUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("`>&` naming something that is not a descriptor")))
		r.status = 2
		r.unspecified = true
	}
	return f
}

// refuseDupTarget reports the word after `>&` or `<&` that is not a
// descriptor, in the dialect's own words.
//
// Two verbs, because the shells name two different things: `%[1]s` is the
// target *as it was written* and `%[2]s` is what it expanded to. bash names
// the first — `<&""` is `"": Bad file descriptor`, quotation marks and all —
// and ksh93 names the second, which for that case is nothing at all.
//
// The empty word has a wording of its own wherever a shell gives it one,
// because two of them do: bash calls a word that came to nothing a bad
// descriptor and a word that came to something an ambiguous redirect, and
// ksh93 says `: cannot open` for the first and `bad file unit number` for the
// second. Leaving it empty means "the same thing either way", which is zsh's
// answer.
func (r *Runner) refuseDupTarget(rd *syntax.Redirect, target string) {
	general := Wording(r.diag().DuplicationTargetIsNotADescriptor,
		"%[2]s: ambiguous redirect", rd.Text, target)
	if target == "" && r.diag().EmptyDuplicationTarget != "" {
		general = Wording(r.diag().EmptyDuplicationTarget, "", rd.Text, target)
	}
	r.diagf("%s\n", general)
	r.status = 1
	r.redirErr = true
	switch r.duplicationTargetError() {
	case DuplicationTargetErrorEndsTheShell:
		// Two dialects end the shell over this whatever the command was, and
		// they are the two that word it as a syntax error without it being
		// one. The status is the fatal one the vector already carries, so
		// both reach 2 without a number of their own.
		r.fatalQuiet()
	case DuplicationTargetErrorEndsTheShellOnABuiltin:
		// And one ends it only when the command runs *in* the shell. The
		// command is not known here, so the fact travels to where it is.
		r.badDupTarget = true
	}
}

// fdAliased reports whether anything else this shell still holds open refers
// to the same descriptor — another entry in the table, or one of the three
// named streams.
//
// It is asked before a close and never before a write, because sharing is
// only interesting at the end of a descriptor's life: two names for one file
// read and write identically, and differ solely in what closing one of them
// means. The scan is over a table that holds a handful of entries in the
// worst case, which is why the count is recomputed rather than carried —
// a stored count is another thing to get wrong in every path that moves a
// descriptor, and there are several.
//
// Comparability is checked rather than assumed: `==` on two interface values
// panics when the dynamic type is not comparable, and a descriptor here may
// hold any writer an embedder supplied. An uncomparable one is reported as
// unaliased, which keeps the old behavior for it rather than inventing a new
// one — this is a question about identity, and a type that cannot answer it
// has not said "no".
func (r *Runner) fdAliased(held any) bool {
	if held == nil {
		return false
	}
	if t := reflect.TypeOf(held); t == nil || !t.Comparable() {
		return false
	}
	for _, v := range r.fds {
		if v == held {
			return true
		}
	}
	return any(r.Stdin) == held || any(r.Stdout) == held || any(r.Stderr) == held
}

// renameOnSuccess is the close of a `>;` redirection: the temporary file the
// command wrote into is renamed over the target if the command succeeded, and
// thrown away if it did not.
//
// A closer rather than a step of its own because the moment is the same one —
// the command has ended, and its redirections are being taken down. That is
// also what makes the status readable here: [Runner.status] holds what the
// command ended at by the time the deferred closers run, on both routes into
// them (a simple command's and a compound one's).
//
// The target's permissions are carried over where it already existed, because
// a rename brings the temporary file's mode with it and a replaced file that
// silently became world-readable would be a worse answer than no operator at
// all. Measured on ksh93u+ 2026-09-14: `chmod 741 f` then `echo new >; f`
// leaves the mode at `-rwxr----x`.
type renameOnSuccess struct {
	r      *Runner
	f      *os.File
	temp   string
	target string
}

func (t renameOnSuccess) Close() error {
	err := t.f.Close()
	finishRenameOnSuccess(t.temp, t.target, t.r.status == 0)
	return err
}

// renameOnSuccessTemp names the file a `>;` writes into: a hidden name in the
// target's **own directory**, because a rename across filesystems is not a
// rename — it would be a copy with a window in the middle, which is the one
// thing this operator exists to avoid.
//
// Numbered rather than random, the way a process substitution's own files
// are: the pid keeps two shells apart and the counter keeps one shell's
// several apart, and both are needed because a loop can reach this twice
// before the first rename has happened.
func renameOnSuccessTemp(target string) string {
	return filepath.Join(filepath.Dir(target),
		".sh-rename-"+strconv.Itoa(os.Getpid())+"-"+
			strconv.FormatUint(renameOnSuccessSeq.Add(1), 10))
}

var renameOnSuccessSeq atomic.Uint64

// heredocSpoolSeq numbers the files a here-document's body is spooled into,
// beside the pid, for the reason renameOnSuccessTemp gives: a loop reaches
// this twice before the first is closed.
var heredocSpoolSeq atomic.Uint64

// finishRenameOnSuccess is the filesystem half of a `>;`, in a function of its
// own so that what reaches the filesystem is nameable.
//
// Outside the gate deliberately, and the reason is the open above it: the
// target was asked about as an `ActionOpen` with `Write` set and the
// temporary as a second one, so a policy that refused either never got here.
// What is left is completing a write two gate consultations have already
// allowed, on a path this shell chose and a path the script named.
func finishRenameOnSuccess(temp, target string, succeeded bool) {
	if !succeeded {
		// The command failed, so the target is left exactly as it was — and
		// a target that did not exist is still not there. Removing the
		// temporary is the whole of that; nothing else happened to the
		// target at any point.
		_ = os.Remove(temp)
		return
	}
	if st, err := os.Stat(target); err == nil {
		// Only where the target is there to have a mode. A new file keeps
		// the temporary's own, which is what a shell creating it with `>`
		// would have given it.
		_ = os.Chmod(temp, st.Mode().Perm())
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
	}
}
