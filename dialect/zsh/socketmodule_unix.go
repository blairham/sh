// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/blairham/sh/interp"
)

// The `zsh/net/socket` module: one builtin, and a Unix-domain socket is the
// whole of it.
//
// Measured 2026-09-09 against zsh 5.9.2 with `zsh -f`. `zsocket` has three
// forms — connect, listen and accept — and each of them ends the same way: a
// descriptor is put in the shell's own table and `$REPLY` is the number, so
// the socket is spoken to with `print -u $REPLY` and `read -u $REPLY` and
// closed with `exec {REPLY}>&-`. That is the reason the manual gives for it
// being a builtin at all, and it is the reason it needs
// [interp.Runner.OpenDescriptor]: nothing else in this package can hand a
// script a descriptor it did not open by redirection.
//
// The one real caller on this machine is a prompt segment that asks a VPN
// daemon over its socket. The line that made #1634 a startup failure is the
// unguarded `zmodload -F zsh/net/socket b:zsocket` above it, which runs
// whether or not the daemon is installed.

func registerSocketModule(r *interp.Runner) {
	r.Register("zsocket", zsocketBuiltin)
}

// zsocketOpts is what the letters asked for.
type zsocketOpts struct {
	accept   bool // -a
	listen   bool // -l
	nowait   bool // -t
	verbose  bool // -v
	target   int  // -d
	retarget bool
}

func zsocketBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts, rest, code := zsocketOptions(r, args)
	if code != 0 {
		return code
	}
	switch {
	case opts.accept:
		if len(rest) == 0 {
			r.Diagnosef("-a requires an argument\n")
			return 1
		}
		return zsocketAccept(r, opts, rest[0])
	case opts.listen:
		if len(rest) == 0 {
			r.Diagnosef("-l requires an argument\n")
			return 1
		}
		return zsocketListen(r, ctx, opts, rest[0])
	case len(rest) == 0:
		// Measured, and worded as the builtin words it: the sentence names
		// the command rather than a letter, because no letter is missing.
		r.Diagnosef("zsocket requires an argument\n")
		return 1
	}
	return zsocketConnect(r, ctx, opts, rest[0])
}

func zsocketOptions(r *interp.Runner, args []string) (opts zsocketOpts, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			switch word[i] {
			case 'a':
				opts.accept = true
			case 'l':
				opts.listen = true
			case 't':
				opts.nowait = true
			case 'v':
				opts.verbose = true
			case 'd':
				value, ok := systemLetterValue(word, &rest, i)
				fd, err := strconv.Atoi(value)
				if !ok || err != nil || fd <= 0 {
					// Zero is refused with the rest, measured: `zsocket -l -d
					// 0` is `0 is an invalid argument to -d`. A shell's
					// standard input is not a number a socket may be moved on
					// to by asking.
					r.Diagnosef("%s is an invalid argument to -d\n", value)
					return opts, nil, 1
				}
				opts.target, opts.retarget = fd, true
				i = len(word)
			default:
				r.Diagnosef("bad option: -%c\n", word[i])
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

// zsocketConnect is the plain form: open a connection to a socket somebody
// else is listening on.
func zsocketConnect(r *interp.Runner, ctx context.Context, opts zsocketOpts, path string) int {
	// A connected socket is a two-way channel, so the question is the
	// stronger of the two permissions rather than the weaker: a script that
	// may only read a path must not be able to send down a socket at it.
	// Refused the way a refused redirection is, because there is no honest
	// way to carry on — see interp.AllowModify (#1819).
	if !r.AllowModify(ctx, shellPath(r, path)) {
		return 1
	}
	f, err := zsocketOpen(shellPath(r, path), func(fd int, addr *syscall.SockaddrUnix) error {
		return syscall.Connect(fd, addr)
	})
	if err != nil {
		r.Diagnosef("connection failed: %s\n", sysErrnoText(r, err))
		return 1
	}
	return zsocketPublish(r, opts, f, path+" is now on fd")
}

// zsocketListen is `-l`: bind a socket at a path and start listening on it.
//
// The listener is *not* unlinked when this shell's own net.Listener value goes
// away, which has to be said out loud because the standard library's default
// is the other way: a listener that removed the path on close would take the
// socket out from under the descriptor the script is left holding. The
// descriptor is what the script was given, so the path has to outlive the
// value that made it.
func zsocketListen(r *interp.Runner, ctx context.Context, opts zsocketOpts, path string) int {
	// Binding creates a filesystem object at the path and leaves it there —
	// the paragraph above is about it outliving the descriptor — so this is
	// as much a write as `> path` is, and was not asked about at all until
	// #1819: `zsocket -l` under `default deny` returned 0 and left a socket
	// outside the boundary.
	if !r.AllowModify(ctx, shellPath(r, path)) {
		return 1
	}
	f, err := zsocketOpen(shellPath(r, path), func(fd int, addr *syscall.SockaddrUnix) error {
		if err := syscall.Bind(fd, addr); err != nil {
			return err
		}
		return syscall.Listen(fd, zsocketBacklog)
	})
	if err != nil {
		r.Diagnosef("could not bind to %s: %s\n", path, sysErrnoText(r, err))
		return 1
	}
	return zsocketPublish(r, opts, f, path+" listener is on fd")
}

// zsocketAccept is `-a`: take one connection off a listener the script already
// has, named by the descriptor `-l` gave it.
//
// `-t` is the difference between asking and waiting, and it is a *silent*
// failure when nothing is pending — measured, status 1 with nothing written
// and `$REPLY` left holding whatever it held. A refusal that said something
// would be a refusal a polling loop printed once per turn.
func zsocketAccept(r *interp.Runner, opts zsocketOpts, arg string) int {
	fd, err := strconv.Atoi(arg)
	if err != nil {
		r.Diagnosef("could not accept connection: %s\n", syscall.EBADF.Error())
		return 1
	}
	sys, ok := r.SystemDescriptor(fd)
	if !ok {
		// A number nothing is open at is the one failure `-t` swallows and
		// the plain form reports. Measured both ways: `zsocket -a 61` is
		// `could not accept connection: bad file descriptor` and `zsocket -a
		// -t 61` is a bare 1 — which is the letter being consistent rather
		// than lenient, since with `-t` the question is only whether a
		// connection is waiting and a descriptor that is not there has none.
		// A descriptor that *is* there and is not a socket still complains
		// under `-t`, because that failure comes from the accept.
		if opts.nowait {
			return 1
		}
		r.Diagnosef("could not accept connection: %s\n", syscall.EBADF.Error())
		return 1
	}
	if opts.nowait {
		// The descriptor goes non-blocking for the length of one call and
		// comes back, so a listener left over from `-l` is the same listener
		// afterwards and a later `zsocket -a` without `-t` still waits.
		if err := syscall.SetNonblock(sys, true); err != nil {
			r.Diagnosef("could not accept connection: %s\n", sysErrnoText(r, err))
			return 1
		}
		defer func() { _ = syscall.SetNonblock(sys, false) }()
	}
	taken, _, err := syscall.Accept(sys)
	if err != nil {
		if opts.nowait && zsocketWouldBlock(err) {
			return 1
		}
		r.Diagnosef("could not accept connection: %s\n", sysErrnoText(r, err))
		return 1
	}
	syscall.CloseOnExec(taken)
	f := os.NewFile(uintptr(taken), "zsocket")
	// The peer of a Unix-domain connection has no name, which is why zsh's
	// own verbose line has two spaces in it: `new connection from  is on fd`.
	return zsocketPublish(r, opts, f, "new connection from  is on fd")
}

// zsocketWouldBlock reports the one failure `-t` is asking about: nothing is
// waiting. Two errno values mean it and a system may use either, so both are
// read as the same answer.
func zsocketWouldBlock(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}

// zsocketBacklog is what a listener here asks the kernel to queue before it
// starts refusing connections.
//
// **One**, measured against zsh on this machine: its listener refuses a second
// connection with `connection refused` while the first is still unaccepted.
// The standard library's listener asks for a hundred and twenty-eight, which
// is not a larger version of the same behavior — a `while zsocket $sock; do`
// loop written to stop on the refusal would not stop.
//
// The number is what is *asked for*; what is delivered, and what a full queue
// then does, is the operating system's. Measured 2026-09-10 against real zsh on
// both, one connection at a time until it stopped answering:
//
//	macOS 26 / zsh 5.9.2   queues 1, and the next connection is refused
//	Debian 12 / zsh 5.9    queues 2, and the next connection **blocks**
//
// This shell answers the same as zsh on each from this one constant, so the
// divergence is the kernel's and not the dialect's. It is why the assertion
// lives in socketbacklog_darwin_test.go and why the corpus does not probe the
// depth: a case counting how many a listener holds would hang on half the
// machines it runs on, which is what it did to a CI run before it was
// measured.
const zsocketBacklog = 1

// zsocketOpen makes a Unix stream socket and does one thing with it: bind and
// listen, or connect.
//
// The socket is made here rather than through the standard library's dialer
// and listener, and the backlog above is the reason: neither of those lets the
// number be chosen, and one of them unlinks the path when its value is closed
// — which would take the socket file out from under the descriptor the script
// was just handed.
//
// The descriptor is close-on-exec, taken under the lock that makes the flag
// atomic against a fork happening in another goroutine — the same care the
// standard library takes for the same reason, and the reason the flag is set
// here rather than left to the caller.
func zsocketOpen(path string, use func(fd int, addr *syscall.SockaddrUnix) error) (*os.File, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, err
	}
	if err := use(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// zsocketPublish puts the descriptor in the shell's table, tells the script
// its number, and writes the verbose line `-v` asked for.
func zsocketPublish(r *interp.Runner, opts zsocketOpts, f *os.File, said string) int {
	fd := opts.target
	if opts.retarget {
		r.SetDescriptor(fd, f)
	} else {
		fd = r.OpenDescriptor(f)
	}
	r.SetVar("REPLY", strconv.Itoa(fd))
	if opts.verbose {
		// Standard output rather than standard error, measured: it is
		// information the caller asked for and not a complaint.
		_, _ = fmt.Fprintf(r.Out(), "%s %d\n", said, fd)
	}
	return 0
}
