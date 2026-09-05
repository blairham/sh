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
func (r *Runner) applyRedirs(ctx context.Context, rs []*syntax.Redirect, compound bool) ([]io.Closer, error) {
	r.redirErr = false
	r.redirFds = nil
	if len(rs) == 0 {
		return nil, nil
	}
	var closers []io.Closer
	// What this command has already opened for each stream, so a second
	// redirection of the same one can be combined with the first where the
	// dialect combines them. Per command rather than per runner: the stream
	// it started with is not one of its targets.
	opened := map[int]io.Writer{}
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
			r.Stdin = strings.NewReader(body)
			continue
		}

		name, bad := r.redirectTarget(rd)
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
		if rd.Op == syntax.TokGreatAmp || rd.Op == syntax.TokLessAmp {
			if rd.N == nil && rd.Op == syntax.TokLessAmp {
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
						if c, ok := held.(io.Closer); ok {
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
			if err := r.dupFd(fd, name); err != nil {
				r.diagf("%v\n", err)
				r.status = 1
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
		switch rd.Op {
		case syntax.TokGreat, syntax.TokClobber:
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if r.noclobber && rd.Op == syntax.TokGreat {
				// Under `set -C` a plain `>` refuses to truncate a file that
				// already exists. `>|` is the documented override, and is
				// the one operator on this list that means the same thing in
				// every shell measured.
				flags |= os.O_EXCL
			}
		case syntax.TokDGreat:
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		case syntax.TokLess:
			flags = os.O_RDONLY
		case syntax.TokLessGreat:
			// `<>` opens for reading and writing, creating the file and
			// never truncating it — `exec 3<> state` keeps what the file
			// held. Unanimous and POSIX, and with no descriptor number it
			// is standard input, exactly as a plain `<` is.
			flags = os.O_RDWR | os.O_CREATE
		case syntax.TokAmpGreat, syntax.TokAmpDGreat:
			// Both streams to one file.
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if rd.Op == syntax.TokAmpDGreat {
				flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
			}
			fd = -1
		default:
			return closers, fmt.Errorf("not implemented yet: the %s redirection", rd.Op)
		}
		if rd.N == nil && (flags == os.O_RDONLY || rd.Op == syntax.TokLessGreat) {
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

		path := name
		if name != "" && r.Dir != "" && !filepath.IsAbs(path) {
			// A name that is not there is not a relative one. Joining it to
			// the working directory turns "no name" into *the directory*,
			// which then opens: `cd /tmp; cat < $unset` read the directory
			// rather than failing, and only because the shell had been told
			// where it was. Every shell reports that it cannot open "".
			path = filepath.Join(r.Dir, path)
		}
		action := r.act(Action{Kind: ActionOpen, Path: path, Write: flags != os.O_RDONLY})
		if !r.allowed(ctx, action) {
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

		f, err := os.OpenFile(path, flags, 0o666)
		if err != nil {
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			// Two verbs, positional because the shells order them
			// differently: %[1]s is the name as written and %[2]s the
			// reason. Two wordings because two of the four say "create"
			// rather than "open" when the redirect was making the file.
			creating := flags != os.O_RDONLY
			format, fallback := r.diag().CannotOpen, "cannot open %[1]s: %[2]s"
			if creating {
				format, fallback = r.diag().CannotCreate, "cannot create %[1]s: %[2]s"
			}
			if r.noclobber && rd.Op == syntax.TokGreat && errors.Is(err, fs.ErrExist) &&
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
			r.Stdin = f
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
		r.ask(r.sem().RedirectsWriteToEveryTarget, "a command redirecting one stream to several files") {
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
	same := len(fields) == 1 && fields[0] == plain && len(r.braceExpand(rd.Word)) == 1
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
	braced := len(r.braceExpand(rd.Word)) > 1 && r.ask(r.sem().BraceExpansion, "brace expansion")
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
	if rd.Op == syntax.TokTLess {
		// A here-string is one line, and its word is expanded like any other.
		return strings.Join(r.expandWordNoSplit(rd.Word), "") + "\n"
	}
	if rd.Heredoc == nil {
		return ""
	}
	body := rd.Heredoc.Literal()
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
func (r *Runner) dupFd(fd int, target string) error {
	if target == "-" {
		switch fd {
		case 0:
			r.Stdin = closedFd{}
		case 2:
			r.Stderr = closedFd{}
		case 1:
			r.Stdout = closedFd{}
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
// entry from ten up, clear of the single digits a script addresses itself.
func (r *Runner) nextFreeFd() int {
	fd := 10
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
func (r *Runner) builtinWriteStatus(name string, st int) int {
	err := r.writeFailed
	r.writeFailed = nil
	if err == nil || r.unspecified {
		return st
	}
	if errors.Is(err, syscall.EPIPE) {
		r.signalDeath("PIPE", syscall.SIGPIPE)
		return r.status
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
type closedFd struct{}

func (closedFd) Write([]byte) (int, error) { return 0, syscall.EBADF }
func (closedFd) Read([]byte) (int, error)  { return 0, syscall.EBADF }

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
func (r *Runner) setFdVar(ref, value string) {
	base, sub, ok := r.subscriptOperand(ref)
	if !ok {
		r.setVar(ref, value)
		return
	}
	if r.assocDeclared(base) {
		r.setAssocElem(base, sub, value)
		return
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		// The same silence a bad subscript gets from the reading half. The
		// redirection itself has already happened, and the number it chose
		// has nowhere to go — which is the shape of the case the dialect
		// answers with FdVariableBadCloseIsAnError on the way in.
		return
	}
	r.setArrayElem(base, idx, sub, value)
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
