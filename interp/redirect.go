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
	"reflect"
	"strings"
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
func (r *Runner) applyRedirs(ctx context.Context, rs []*syntax.Redirect, compound, ownProcess bool) ([]io.Closer, error) {
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
	outerOwn := r.redirForOwnProcess
	r.redirForOwnProcess = ownProcess
	defer func() { r.redirForOwnProcess = outerOwn }()
	r.redirErr = false
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
	saveFds := func() {
		if fdsTouched {
			return
		}
		fdsTouched = true
		saved := r.fds
		// The marks travel with the table, because they are about the table:
		// a command that redirects a number `exec` had opened is using that
		// number for itself, and when the command ends the number goes back
		// to being `exec`'s.
		savedExec := r.execFds
		r.fds = maps.Clone(r.fds)
		r.execFds = maps.Clone(r.execFds)
		closers = append(closers, closerFunc(func() error {
			r.fds, r.execFds = saved, savedExec
			return nil
		}))
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
		// A here-document and a here-string are input the shell already
		// holds, so there is no file to open and nothing for the gate to
		// see — the bytes never leave this process on their way in. Before
		// the target is computed, because their word is a *body* rather than
		// a filename: reading it as a target asked the ordinary-word axis
		// about a word no shell reads that way — `<<< $two` is one line of
		// input in all four, never an ambiguous redirect.
		if rd.Op.IsHeredoc() || rd.Op == syntax.TokTLess {
			body := r.heredocBody(rd)
			// A body joins the set as an opened file does: `cat <f <<<hi` is the
			// file and then the line, measured, so the source need not be a file
			// to be one of several.
			r.Stdin = r.eachSource(0, strings.NewReader(body), sources)
			if r.redirErr {
				// The body could not be expanded, and the process the
				// redirection was for is where that happened — so the
				// command is what was given up, not the shell. See
				// giveUpTheCommand.
				return closers, nil
			}
			continue
		}

		// The target is expanded where the command runs, which decides what
		// becomes of a write inside it and of a failure — and, whoever runs
		// the command, an expansion that failed is not a name and is not
		// opened. See redirtarget.go.
		name, bad := r.redirectTargetForItsProcess(rd)
		if bad {
			return closers, nil
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
		if r.greatAmpNamesAFile(rd, name) {
			op = syntax.TokAmpGreat
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
					continue
				}
				// `exec {name}>&2` picks a fresh descriptor aimed where the
				// target aims now, and the name receives its number.
				fd = r.nextFreeFd()
			}
			persists := fdVar != "" &&
				r.ask(r.sem().FdVariableOutlivesTheCommand, "a variable-named descriptor outliving its command")
			if r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			// The number, before the duplication that would use it. There is
			// no file here for a refusal to have created, which is the whole
			// of why this side is checked earlier than the other.
			if r.refuseFdOverLimit(fd) || r.unspecified {
				r.redirErr = true
				return closers, nil
			}
			if fd > 2 && !persists {
				saveFds()
			}
			if err := r.dupFd(fd, name, opened); err != nil {
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
			r.redirWrote(fd)
			if fdVar != "" {
				r.setFdVar(fdVar, itoa(fd))
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
			fd = -1
		default:
			return closers, fmt.Errorf("not implemented yet: the %s redirection", rd.Op)
		}
		if rd.N == nil && (flags == os.O_RDONLY || op == syntax.TokLessGreat) {
			fd = 0
		}
		// The shell picks the descriptor: the next free number from ten up,
		// clear of the single digits a script says `2>&1` about. Whether it
		// outlives the command is the axis ksh93 answers alone.
		persists := false
		if fdVar != "" {
			fd = r.nextFreeFd()
			persists = r.ask(r.sem().FdVariableOutlivesTheCommand,
				"a variable-named descriptor outliving its command")
			if r.unspecified {
				r.redirErr = true
				return closers, nil
			}
		}

		// A name that is not there is not a relative one. Joining it to the
		// working directory turns "no name" into *the directory*, which then
		// opens: `cd /tmp; cat < $unset` read the directory rather than
		// failing, and only because the shell had been told where it was.
		// Every shell reports that it cannot open "". atDir is where that
		// rule lives now — it was written here first, and the file tests had
		// the identical bug because the rule had not reached the resolution
		// they shared (#1189).
		path := r.atDir(name)
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

		// A background job's pid is settled before an open that may never
		// return, so that `&` can hand the shell back. See
		// settleBackgroundJobBeforeABlockingOpen: this is where a job whose
		// first act blocks would otherwise leave the shell waiting for a pid
		// that is not coming.
		r.settleBackgroundJobBeforeABlockingOpen(path)
		f, fellBack, err := r.openThroughNoclobber(ctx, &action, path, flags)
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
		if r.refuseFdOverLimit(fd) || r.unspecified {
			_ = f.Close()
			r.redirErr = true
			return closers, nil
		}
		if !persists {
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
			r.setFd(fd, f)
			r.redirWrote(fd)
			if fdVar != "" {
				r.setFdVar(fdVar, itoa(fd))
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
		f = multiTarget{io.MultiWriter(prev, f)}
	}
	opened[fd] = f
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
		in = io.MultiReader(prev, in)
	}
	sources[fd] = in
	return in
}

// multiTarget is a stream the shell built out of more than one target, under
// the dialect that writes to every one of them.
//
// It is a marker before it is a writer: the io.Writer inside is an ordinary
// multi-writer and does the work, and the type exists so that `exec cmd` can
// tell this stream from a file without guessing.
type multiTarget struct{ io.Writer }

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// redirectTarget is the name a redirection opens, and says whether the shell
// refused it.
//
// Two answers, and this had a third that is nobody's: it expanded the target
// the way an argument is expanded — split into fields and matched as a
// pattern — and then quietly took the first field. So `e="a b"; echo hi > $e`
// wrote to `a`, and `e="x*"` truncated whichever file happened to match,
// which the script never named.
func (r *Runner) redirectTarget(rd *syntax.Redirect) (string, bool) {
	fields, plain := r.expandRedirectTargetViews(rd.Word)

	// Asked only where the two readings differ, which is almost never: `> f`
	// and `> "$e"` are one word under both, and so is a pattern that matches
	// nothing. Asking every time would refuse every redirection in the core
	// over a question that decides nothing.
	//
	// Braces count as differing: a target that expands to several words is
	// several words to the dialect that expands one.
	same := len(fields) == 1 && fields[0] == plain && r.braceCount(rd.Word) == 1
	if same {
		return plain, false
	}

	if !r.ask(r.sem().RedirectTargetIsAnOrdinaryWord, "a redirection target expanded as an ordinary word") {
		if r.unspecified {
			// Refused, so the command must not run: acting on either reading
			// after saying the shells disagree would be answering the
			// question anyway.
			r.redirErr = true
			return "", true
		}
		// Expanded and no more: whatever it came to is the name, spaces and
		// pattern characters included.
		return plain, false
	}
	// bash's reading, and braces make words as surely as splitting does:
	// `> {a,b}` names two files and so names none.
	braced := !r.noBraceExpand && r.braceCount(rd.Word) > 1 &&
		r.ask(r.sem().BraceExpansion, "brace expansion")
	if !braced && len(fields) == 1 {
		return fields[0], false
	}
	// Anything but exactly one word, which includes none: an empty variable
	// is as ambiguous as two filenames, because neither says where to write.
	r.diagf("%s\n", Wording(r.diag().AmbiguousRedirect, "%[1]s: ambiguous redirect", rd.Text))
	r.redirErr = true
	r.status = 1
	return "", true
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
	if !r.redirForOwnProcess {
		return r.heredocText(rd)
	}
	// The redirection belongs to a command this shell runs as a process of
	// its own, so the body is expanded the way that process would expand it.
	// See heredocprocess.go for the measurement that draws the line there.
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
	if rd.HeredocAtEOF && !strings.HasSuffix(body, "\n") {
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
func (r *Runner) errBadFd(fd int) error {
	return fmt.Errorf("%d: %s", fd, r.diag().reasonText(reason(syscall.EBADF)))
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
func (r *Runner) dupFd(fd int, target string, opened map[int]io.Writer) error {
	if target == "-" {
		// Whatever this command had aimed at the number, it no longer has.
		delete(opened, fd)
		switch fd {
		case 0:
			r.Stdin = closedFd{}
		case 2:
			r.Stderr = closedFd{}
		case 1:
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
			return r.errBadFd(m)
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
		return r.errBadFd(m)
	}
	switch fd {
	case 0:
		rd, ok := src.(io.Reader)
		if !ok {
			return r.errBadFd(m)
		}
		r.Stdin = rd
	case 2, 1:
		w, ok := src.(io.Writer)
		if !ok {
			return r.errBadFd(m)
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
func (r *Runner) nextFreeFd() int {
	fd := r.sem().FirstAllocatedDescriptor.number()
	for {
		if _, held := r.fds[fd]; !held {
			return fd
		}
		fd++
	}
}

// shellOwnedFd marks a descriptor the shell opened for its own plumbing
// rather than one the script asked for by number.
//
// A coprocess's near ends are the only case so far. They are in the table so
// that `>&${C[0]}` can find them, but they are not the script's to hand out,
// and every descriptor in the table is otherwise rebuilt into an external
// child's — see childFiles. Inheriting these would be worse than untidy: a
// child holding the write end open means the coprocess never reads
// end-of-file, which is the leak /dev/fd process substitution was rejected
// for.
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
	// Before the axis, and that is the whole reason this is a second wording.
	// zsh answers the axis No and returns below without reaching
	// BuiltinWriteError; it still says something on every route but one, and
	// the route it stays quiet on is the one where the writing command's own
	// redirections closed the stream. See
	// Diagnostics.InheritedClosedStreamWriteError for the measurements, and
	// for why moving BuiltinWriteError in front of the axis instead would be
	// wrong.
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
	if !r.ask(r.sem().BuiltinWriteErrorFailsTheCommand, "a builtin's failed write failing the command") {
		return st
	}
	if w := r.diag().BuiltinWriteError; w != "" {
		r.diagf("%s\n", fmt.Sprintf(w, name, r.diag().reasonText(reason(err))))
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
		return v, held
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
func (r *Runner) setFdVar(ref, value string) {
	if n, ok := positionalFdVar(ref); ok {
		// A positional parameter receives the number, which is how a
		// function that was handed a descriptor hands back the one the
		// shell picked. Out of range writes nothing: there is no position
		// to widen the list to that a later `shift` would keep straight.
		if n >= 1 && n <= len(r.Params) {
			r.Params[n-1] = value
		}
		return
	}
	r.storeThroughOperand(ref, value)
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
// Only where the redirection names no descriptor of its own. `2>&qq` is a
// duplication in every shell that has the form at all: bash calls it an
// ambiguous redirect where the bare spelling writes a file, which is what
// makes the leading number the whole of the question.
func (r *Runner) greatAmpNamesAFile(rd *syntax.Redirect, target string) bool {
	if rd.Op != syntax.TokGreatAmp || rd.N != nil || isDescriptorSpec(target) {
		return false
	}
	switch r.greatAmpTarget() {
	case GreatAmpTargetNamesAFile:
		return target != ""
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
	// One dialect ends the shell over this, and only when the command it is
	// written on runs *in* the shell — see
	// DuplicationTargetErrorOnABuiltinIsFatal. The command is not known here,
	// so the fact travels to where it is.
	r.badDupTarget = true
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
