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
		name := joinFields(r.expandWord(rd.Word))

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
		if r.Dir != "" && !filepath.IsAbs(path) {
			path = filepath.Join(r.Dir, path)
		}
		action := Action{Kind: ActionOpen, Path: path, Write: flags != os.O_RDONLY}
		if !r.allowed(ctx, action) {
			return closers, nil
		}

		f, err := os.OpenFile(path, flags, 0o666)
		if err != nil {
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			r.diagf("%s\n", Wording(r.diag().CannotOpen, "cannot open %s: %s",
				name, r.diag().openReason(err)))
			r.status = 1
			r.redirErr = true
			return closers, nil
		}
		closers = append(closers, f)

		switch fd {
		case -1:
			r.Stdout, r.Stderr = f, f
		case 0:
			r.Stdin = f
		case 2:
			r.Stderr = f
		default:
			r.Stdout = f
		}
	}
	return closers, nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// joinFields is what a redirection target does with a word that expanded to
// more than one field. One is the normal case; more than one is ambiguous and
// the shells differ, so this takes the first and does not pretend otherwise.
func joinFields(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
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
func (d Diagnostics) openReason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if d.FileNotFound != "" && errors.Is(err, fs.ErrNotExist) {
		return d.FileNotFound
	}
	return err.Error()
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
