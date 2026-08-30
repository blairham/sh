// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blairham/sh/internal/syntax"
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

		var flags int
		switch rd.Op {
		case syntax.TokGreat, syntax.TokClobber:
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		case syntax.TokDGreat:
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		case syntax.TokLess:
			flags = os.O_RDONLY
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
			r.errf("sh: cannot open %s: %v\n", name, err)
			r.status = 1
			return closers, nil
		}
		closers = append(closers, f)

		switch fd {
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
