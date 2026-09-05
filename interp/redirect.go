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
		r.fds = maps.Clone(r.fds)
		closers = append(closers, closerFunc(func() error {
			r.fds = saved
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
			if fdVar != "" {
				if name == "-" {
					// `exec {name}>&-` closes the descriptor the variable
					// holds. The close is for keeps — this path never joins
					// the save — so the pipe or file behind it really ends,
					// which is what lets a coprocess see its input finish.
					v, okv := r.getVar(fdVar)
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
			if fd > 2 && !persists {
				saveFds()
			}
			if err := r.dupFd(fd, name); err != nil {
				r.diagf("%v\n", err)
				r.status = 1
				r.redirErr = true
				return closers, nil
			}
			if fdVar != "" {
				r.setVar(fdVar, itoa(fd))
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
		action := Action{Kind: ActionOpen, Path: path, Write: flags != os.O_RDONLY}
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
			if fdVar != "" {
				r.setVar(fdVar, itoa(fd))
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
		f = io.MultiWriter(prev, f)
	}
	opened[fd] = f
	return f
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// joinFields is what a redirection target does with a word that expanded to
// more than one field. One is the normal case; more than one is ambiguous and
// the shells differ, so this takes the first and does not pretend otherwise.
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
			return fmt.Errorf("%d: bad file descriptor", m)
		}
		src = v
	}
	switch fd {
	case 0:
		rd, ok := src.(io.Reader)
		if !ok {
			return fmt.Errorf("%d: bad file descriptor", m)
		}
		r.Stdin = rd
	case 2, 1:
		w, ok := src.(io.Writer)
		if !ok {
			return fmt.Errorf("%d: bad file descriptor", m)
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
func (r *Runner) builtinWriteStatus(name string, st int) int {
	err := r.writeFailed
	r.writeFailed = nil
	if err == nil || r.unspecified {
		return st
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
