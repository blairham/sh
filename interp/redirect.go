// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
// This slice handles the plain file redirections. Descriptor duplication,
// here-strings and here-document bodies are not here yet; the parser produces
// them and this refuses them rather than ignoring them.
func (r *Runner) applyRedirs(ctx context.Context, rs []*syntax.Redirect) ([]io.Closer, error) {
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

	for _, rd := range rs {
		fd := 1
		if rd.N != nil {
			if n, ok := atoi(rd.N.Literal()); ok {
				fd = n
			}
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
			if err := r.dupFd(fd, name); err != nil {
				r.diagf("%v\n", err)
				r.status = 1
				r.redirErr = true
				return closers, nil
			}
			continue
		}

		// A here-document and a here-string are input the shell already
		// holds, so there is no file to open and nothing for the gate to
		// see — the bytes never leave this process on their way in.
		if rd.Op.IsHeredoc() || rd.Op == syntax.TokTLess {
			body := r.heredocBody(rd)
			r.Stdin = strings.NewReader(body)
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
		if rd.N == nil && flags == os.O_RDONLY {
			fd = 0
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
		closers = append(closers, f)

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
			// A descriptor this shell cannot address is refused, not rounded
			// down to one it can. `default` used to land on stdout, so
			//
			//	exec 3>out.txt
			//
			// opened the file, said nothing, exited 0 — and sent every later
			// `echo` into it, because the 3 was dropped and the redirection
			// applied to stdout. A side channel took the script's whole
			// output with it.
			//
			// Three named streams is what this shell has; see dupFd, which
			// says the same thing about `>&N`. Saying so is better than
			// quietly meaning something else.
			return closers, fmt.Errorf("not implemented yet: redirecting file descriptor %d, which needs a descriptor table", fd)
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
	same := len(fields) == 1 && fields[0] == plain && len(braceExpand(rd.Word)) == 1
	if same {
		return plain, false
	}

	if !r.ask(r.sem().RedirectTargetIsAnOrdinaryWord, "a redirection target expanded as an ordinary word") {
		if r.unspecified {
			return "", true
		}
		// Expanded and no more: whatever it came to is the name, spaces and
		// pattern characters included.
		return plain, false
	}
	// bash's reading, and braces make words as surely as splitting does:
	// `> {a,b}` names two files and so names none.
	braced := len(braceExpand(rd.Word)) > 1 && r.ask(r.sem().BraceExpansion, "brace expansion")
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
// Only 0, 1 and 2 are addressable. A shell with an `exec 3>file` would need a
// descriptor table; this has three named streams, and saying so is better than
// accepting `3>&1` and quietly doing nothing with it.
func (r *Runner) dupFd(fd int, target string) error {
	if target == "-" {
		switch fd {
		case 0:
			r.Stdin = closedFd{}
		case 2:
			r.Stderr = closedFd{}
		default:
			r.Stdout = closedFd{}
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
		return fmt.Errorf("%d: bad file descriptor", m)
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
		return fmt.Errorf("%d: bad file descriptor", fd)
	}
	return nil
}

// closedFd is a descriptor that has been closed with `>&-`. Reading or writing
// it fails the way the kernel would.
type closedFd struct{}

func (closedFd) Write([]byte) (int, error) { return 0, syscall.EBADF }
func (closedFd) Read([]byte) (int, error)  { return 0, syscall.EBADF }
