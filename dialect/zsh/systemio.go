// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/interp"
)

// The three builtins of `zsh/system` that move bytes: `sysopen`, `sysread`
// and `syswrite`.
//
// Measured 2026-09-10 against zsh 5.9.2 (Homebrew, aarch64) with `zsh -f` and
// no startup files. The module's other three names — `sysseek`, `syserror`
// and `zsystem` — are in systemlock.go and systemseek.go, which are #1749 and
// were written after this file left them refused by name.
//
// # Why these three and why now
//
// `zmodload zsh/system` was already 0 here, because the module's *parameters*
// and its math function are implemented and a missing builtin never holds a
// module shut — a script is told about one of those at the word that runs it.
// That rule was right and the answer it gave was still the wrong thing to
// happen to this machine's startup: a prompt theme writes
//
//	sysopen -r -o cloexec -u _p9k__worker_resp_fd <(…) || return
//
// with the `|| return` doing exactly what it is there for, and the theme's
// asynchronous worker never started. `sysopen` is written nine times in that
// one theme and `sysread` fifteen, so it was never going to be one call site.
//
// # `sysopen` is `exec {fd}<file` with the flags spelled out
//
// The shape that matters is `-u name`: it allocates a descriptor, puts the
// *number* in the named parameter, and leaves the descriptor in the shell's
// own table so `read -u $name` and `print -u $name` reach it. That is the same
// act as `exec {name}<file`, so it resolves the name by the same route — see
// [interp.Runner.SetDescriptorVariable], which is why `-u 'h[k]'` works here
// as it does there. A `-u` whose value is all digits is a *number* instead,
// and the descriptor goes on it.
//
// `-r`, `-w` and `-a` are the direction and `-o` a comma-separated list of
// open flags. Measured, and none of it is guessed:
//
//	written        opened
//	(none)         read-only — `print -u $fd` there is `bad mode on fd`
//	-r             read-only
//	-w             write-only, and **not** truncated
//	-a             write-only, appending
//	-rw            read-write
//	-ra            read-write, appending
//
// The absence of truncation is the half a shell would get wrong by reaching
// for `>`: `sysopen -w` on an eleven-byte file leaves eleven bytes, and
// `-o trunc` is what empties it.
//
// # `-o cloexec` is a mark on the table, not a flag on the open
//
// Go opens every file close-on-exec, so passing the flag through would have
// registered the name and changed nothing observable: this shell hands a
// descriptor to an external child by rebuilding the boundary out of its own
// table, so the table is what has to be told. See sysopenPublish and
// [interp.Runner.KeepDescriptorFromChildren]. Measured against zsh both ways,
// which is what makes it a behavior rather than a detail: a plain `sysopen -w`
// descriptor is writable from `/bin/sh -c 'echo y >&11'` and a `-o cloexec`
// one is `bad file descriptor` there.
//
// # Two places this does not reproduce zsh, both measured
//
// **A numeric `-u` above 9 is reachable here and is not always there.**
// `sysopen -r -u 20 f` is status 0 in zsh 5.9.2 and `read -u 20` then fails;
// so does `-u 10`, and so does `-u 07`, while `-u 7`, `-u 9` and `-u 11` all
// work. That is zsh's own descriptor bookkeeping showing through — 10 is the
// number it moves its own descriptors out to and 11 is where a fresh
// allocation lands — and reproducing it would mean reproducing a `sysopen`
// that reports success and leaves the descriptor unreachable, which is the
// exact shape of failure #1737 was filed about. Here every number a caller
// names is the number the descriptor goes on. Nothing in the tree this shell
// starts with writes a numeric `-u`; the corpus pins 7 and 8, where the two
// shells agree.
//
// **`-o nonblock` on a process substitution loses the command's output.** The
// flag makes the open return without waiting for a peer, which is what it is
// for and what both shells do — and this shell's `<(cmd)` is a named pipe
// whose writer is a goroutine polling for a reader, so an open that returns
// immediately lets the command that named the path *finish* before that poll
// succeeds, and the pipe is unlinked from under it. `sysopen -r -o nonblock
// -u fd <(print -n hi)` then reads end-of-file where zsh reads `hi`. The cause
// is the substitution's pipe lifetime rather than this builtin — see interp's
// procSub and openFifoWriteEnd — and it is filed separately. A plain
// `sysopen -r` on the same substitution is byte-identical, and that is the
// form the workload writes.
//
// # `sysread` and `syswrite` are one system call each
//
// `sysread` does **one** read and reports what it got, which is the difference
// between it and `read`: it does not wait for a line, it does not split, and a
// short read is a success. The status is where it says what happened, and the
// five values are the whole of its vocabulary — 1 a usage error, 2 the read
// failed, 3 the copy `-o` asked for failed, 4 the wait timed out, 5 nothing
// more is coming. A prompt theme's receive loop reads all five:
//
//	if sysread -i $fd -t 0 'buf[$#buf+1]'; then … else (( $? == 4 )) || return; fi
//
// so a shell that collapsed "timed out" and "end of file" into one nonzero
// would make that loop give up on the first idle turn.
//
// `syswrite` writes its one operand's bytes and keeps writing until they have
// all gone or the descriptor refuses, which is what makes `while syswrite
// $'\x05'; do …; done` a loop that ends when the far side does.

// sysopenBuiltin is `sysopen [-arw] [-m mode] [-o opts] -u fd file`.
func sysopenBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := systemOptions(r, args, "mou", "arw")
	if code != 0 {
		return code
	}
	// The operand count first, ahead of every other complaint. Measured, and
	// it is the order that decides which of two things a caller hears about:
	// `sysopen -o bogus` with no file is `not enough arguments` rather than
	// `unsupported option`, because a command with nothing to open has not
	// got as far as the flags it would have opened it with.
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	dest, ok := opts.value('u')
	if !ok {
		// The one refusal that names no letter and no operand, because
		// neither is wrong: `sysopen file` has said what to open and not
		// where to put it.
		r.Diagnosef("file descriptor not specified\n")
		return 1
	}
	number, numeric := sysDescriptorNumber(dest)
	if !numeric && !sysAssignable(dest) {
		r.Diagnosef("not an identifier: %s\n", dest)
		return 1
	}
	flags, cloexec, code := sysopenFlags(r, opts)
	if code != 0 {
		return code
	}
	perm, code := sysopenMode(r, opts)
	if code != 0 {
		return code
	}
	f, err := sysOpenFile(shellPath(r, rest[0]), flags, perm)
	if err != nil {
		r.Diagnosef("can't open file %s: %s\n", rest[0], sysErrnoText(err))
		return 1
	}
	sysopenPublish(r, f, dest, number, numeric, cloexec)
	return 0
}

// sysopenPublish puts the open file in the shell's table and tells the script
// where it went.
//
// The two halves of `-u` part here and nowhere else: a number is a place the
// caller chose, and a name is a place the shell chooses and then reports.
func sysopenPublish(r *interp.Runner, f *os.File, dest string, number int, numeric, cloexec bool) {
	fd := number
	if numeric {
		r.SetDescriptor(fd, f)
	} else {
		fd = r.OpenDescriptor(f)
		r.SetDescriptorVariable(dest, fd)
	}
	if cloexec {
		r.KeepDescriptorFromChildren(fd)
	}
}

// sysopenFlags turns the direction letters and `-o`'s list into the flags one
// open needs, and reports whether the descriptor is to be kept from children.
func sysopenFlags(r *interp.Runner, opts systemOpts) (flags int, cloexec bool, code int) {
	switch {
	case opts.on('r') && (opts.on('w') || opts.on('a')):
		flags = os.O_RDWR
	case opts.on('w'), opts.on('a'):
		flags = os.O_WRONLY
	default:
		// Read-only is what no direction at all means, which is worth
		// stating because the other reading — "whatever the file allows" —
		// is what a shell reaching for os.O_RDWR would produce, and it would
		// make `sysopen -u fd /etc/hosts` fail for a caller that only meant
		// to read.
		flags = os.O_RDONLY
	}
	if opts.on('a') {
		flags |= os.O_APPEND
	}
	for _, word := range opts.values('o') {
		for _, name := range strings.Split(word, ",") {
			flag, keep, known := sysopenFlag(sysopenFlagName(name))
			if !known {
				r.Diagnosef("unsupported option: %s\n", name)
				return 0, false, 1
			}
			flags |= flag
			cloexec = cloexec || keep
		}
	}
	return flags, cloexec, 0
}

// sysopenFlagName is one `-o` word reduced to the spelling the table is
// written in: measured case-insensitive, and with the `O_` a caller may have
// copied off the manual page taken back off.
func sysopenFlagName(name string) string {
	lower := strings.ToLower(name)
	return strings.TrimPrefix(lower, "o_")
}

// sysopenMode is `-m`, the permissions a file this command creates is made
// with.
//
// Octal and nothing else — `-m u=rw` is `invalid mode u=rw` in zsh, where
// `zf_chmod`'s symbolic forms would have worked — and 0666 where the letter
// was not written, which is the number every open without a mode uses and is
// what the umask then takes bits off.
func sysopenMode(r *interp.Runner, opts systemOpts) (os.FileMode, int) {
	text, ok := opts.value('m')
	if !ok {
		return 0o666, 0
	}
	mode, err := strconv.ParseUint(text, 8, 32)
	if err != nil {
		r.Diagnosef("invalid mode %s\n", text)
		return 0, 1
	}
	return os.FileMode(mode), 0
}

// sysreadStatus is what `sysread` says about a call rather than about a
// command line: the four ways one read can end, and the fifth is 0.
//
// Numbers rather than a named type because the shell's own status is a number
// and a caller reads them as `(( $? == 4 ))`. They are measured, one at a
// time, and the corpus holds all five.
const (
	sysreadUsage    = 1 // a bad option, a name that is not one
	sysreadReadFail = 2 // the descriptor could not be read
	sysreadCopyFail = 3 // `-o`'s copy could not be written
	sysreadTimeout  = 4 // `-t` elapsed with nothing waiting
	sysreadEnd      = 5 // end of input
)

// sysreadDefaultBuffer is how much one `sysread` asks for when `-s` did not
// say. Measured indirectly — a read of a longer stream stops at 8192 — and it
// is a ceiling rather than a promise: a short read is a success.
const sysreadDefaultBuffer = 8192

// sysreadBuiltin is `sysread [-c countvar] [-i fd] [-o fd] [-s size]
// [-t timeout] [param]`.
func sysreadBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := systemOptions(r, args, "ciost", "")
	if code != 0 {
		return code
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	// `REPLY` where no parameter was named, which is the same default `read`
	// has and the reason `sysread` alone is a usable command.
	dest := "REPLY"
	if len(rest) == 1 {
		dest = rest[0]
	}
	count, counting := opts.value('c')
	for _, name := range []string{dest, count} {
		if name == "" {
			continue
		}
		if !isIdentifier(name) {
			return sysreadName(r, name)
		}
	}
	in, code := opts.number(r, 'i', 0)
	if code != 0 {
		return code
	}
	out, copying := opts.value('o')
	size, code := opts.number(r, 's', sysreadDefaultBuffer)
	if code != 0 {
		return code
	}
	wait, timed, code := opts.duration(r, 't')
	if code != 0 {
		return code
	}
	if timed {
		if status := sysreadWait(r, in, wait); status != 0 {
			return status
		}
	}
	buf, status := sysreadOnce(r, in, size)
	if status != 0 {
		return status
	}
	if counting {
		r.SetVar(count, strconv.Itoa(len(buf)))
	}
	if copying {
		// **`-o` diverts rather than duplicates.** Measured, and it is the one
		// thing about this builtin a reading of the manual would get backwards:
		// `print -n hello | sysread -c n -o 2 buf` writes the five bytes on to
		// descriptor 2, sets `n` to 5, and leaves `buf` — and `REPLY` — unset.
		// So `-o` is where the bytes went, not a copy of where they also went,
		// and a shell that assigned as well would leave a caller's parameter
		// holding data it had already passed on.
		return sysreadCopy(r, out, buf)
	}
	r.SetVar(dest, string(buf))
	return 0
}

// sysreadName is what a destination this shell cannot assign to gets, and it
// is two different refusals wearing one function.
//
// A name that is not an identifier at all — `sysread 'bad name'` — is zsh's
// own `not an identifier`, measured, and is a usage error in both shells.
//
// A **subscripted** name is not that. zsh takes `sysread 'buf[$#buf+1]'`,
// which is how a prompt theme's receive loop accumulates a response, and what
// it does with it is splice characters into the string `buf` already holds —
// because a subscript on a scalar names a character there. This shell reads
// `${s[2]}` that way already (see ScalarSubscriptIsACharacter) and *assigns*
// through it as though the name were an array, so `buf=xy; buf[3]=Z` is
// `xyZ` in zsh and an array of three elements here. That is a bug in the
// assignment path rather than in this builtin, and the one thing this builtin
// must not do is reach for the array route and hand a caller a `buf` that
// looks assigned and is the wrong shape. So it refuses by the name it was
// given, which is the same answer `zmodload -F` gives for a feature this
// shell has not got.
func sysreadName(r *interp.Runner, name string) int {
	if sysAssignable(name) {
		r.Diagnosef("%s: a subscripted parameter is not implemented yet\n", name)
		return sysreadUsage
	}
	r.Diagnosef("not an identifier: %s\n", name)
	return sysreadUsage
}

// sysreadWait is `-t`: hold until the descriptor has something, and say so if
// it never does.
//
// The wait is the kernel's rather than a sleep-and-look loop, so `-t 0` costs
// one call and `-t 1000` wakes the instant a byte lands. A descriptor this
// shell cannot ask about — one that is not a descriptor at all, in a Runner
// something else embedded — is a *read* failure rather than a timeout,
// because the caller's question was about that descriptor and the honest
// answer is that there is none.
func sysreadWait(r *interp.Runner, fd int, wait time.Duration) int {
	sys, ok := r.SystemDescriptor(fd)
	if !ok {
		return sysreadReadFail
	}
	ready, asked := fdset.ReadableWithin(sys, wait)
	switch {
	case !asked:
		return sysreadReadFail
	case !ready:
		return sysreadTimeout
	}
	return 0
}

// sysreadOnce is the read itself: one call, up to size bytes, and the status
// that describes how it ended.
func sysreadOnce(r *interp.Runner, fd, size int) ([]byte, int) {
	if size <= 0 {
		// Measured: `sysread -s 0` is 5, the same as end of input, and not a
		// usage error — a read of nothing can never return anything, which is
		// the same thing end of input means to the caller.
		return nil, sysreadEnd
	}
	reader, ok := r.ReaderForFd(fd)
	if !ok {
		return nil, sysreadReadFail
	}
	buf := make([]byte, size)
	n, err := reader.Read(buf)
	if n > 0 {
		return buf[:n], 0
	}
	if err == nil || errors.Is(err, io.EOF) {
		return nil, sysreadEnd
	}
	return nil, sysreadReadFail
}

// sysreadCopy is `-o`: the bytes just read, written on to another descriptor
// as well as into the parameter.
func sysreadCopy(r *interp.Runner, out string, buf []byte) int {
	fd, err := strconv.Atoi(out)
	if err != nil {
		r.Diagnosef("integer expected: %s\n", out)
		return sysreadUsage
	}
	if syswriteAll(r, fd, buf) != nil {
		return sysreadCopyFail
	}
	return 0
}

// syswriteBuiltin is `syswrite [-c countvar] [-o fd] data`.
func syswriteBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := systemOptions(r, args, "co", "")
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	count, counting := opts.value('c')
	if counting && !isIdentifier(count) {
		r.Diagnosef("not an identifier: %s\n", count)
		return 1
	}
	fd, code := opts.number(r, 'o', 1)
	if code != 0 {
		return code
	}
	if err := syswriteAll(r, fd, []byte(rest[0])); err != nil {
		// No sentence, measured: the status is the whole of what is said, and
		// it is the same 2 a read failure gets from `sysread`. A `while
		// syswrite …; do; done` against a reader that has gone would
		// otherwise print once per turn.
		return 2
	}
	if counting {
		r.SetVar(count, strconv.Itoa(len(rest[0])))
	}
	return 0
}

// syswriteAll writes every byte or reports why it could not, which is what
// makes `syswrite` a command a loop can be built on.
//
// A partial write is not a failure and not an end: the call is made again for
// what is left. Only an error stops it, and the caller reads that as a status.
func syswriteAll(r *interp.Runner, fd int, buf []byte) error {
	w, ok := r.WriterForFd(fd)
	if !ok {
		return errors.ErrUnsupported
	}
	for len(buf) > 0 {
		n, err := w.Write(buf)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		buf = buf[n:]
	}
	return nil
}

// sysDescriptorNumber reads a `-u` value that is a descriptor rather than a
// parameter name.
//
// All digits and nothing else, which is measured on both sides of the line:
// `-u 07` opens on descriptor 7 and `-u 3x` and `-u -1` are each `not an
// identifier`, so a leading `-` is not a number here however much strconv
// would take it.
func sysDescriptorNumber(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return n, true
}

// sysAssignable reports whether a name is one this shell could assign to:
// an identifier, or an identifier with a subscript after it.
//
// The subscripted form is here because `sysopen -u 'h[k]'` works in zsh and
// because [interp.Runner.SetDescriptorVariable] resolves it — the same route
// `exec {h[k]}<file` takes. See sysreadName for why `sysread` treats the two
// halves of this differently from `sysopen`.
func sysAssignable(name string) bool {
	if base, _, found := strings.Cut(name, "["); found {
		return strings.HasSuffix(name, "]") && isIdentifier(base)
	}
	return isIdentifier(name)
}

// sysErrnoText is the system's own sentence for a failure, without the
// operation and path the standard library wraps round it — each of these
// builtins has already said what it was doing.
func sysErrnoText(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	return err.Error()
}

// systemOpts is the option words one of these three builtins was given, kept
// as they arrived rather than as a struct of fields.
//
// A shared reader because the three share a grammar exactly — letters bundle,
// a letter that takes a value takes the rest of its word or the next one, `--`
// ends them, and the two refusals are worded the same in all three. What
// differs between them is only *which* letters are in each set, which is the
// argument this type takes.
//
// The last of a repeated letter wins and every `-o` is kept, which is measured
// and is two different rules for a reason: `sysopen -u fd -u g file` puts the
// descriptor in `g` and leaves `fd` empty, while `-o cloexec -o sync` opens
// with both. A value that replaces and a list that accumulates are different
// kinds of option and the builtin says which by asking value or values.
type systemOpts struct {
	// flags are the letters that stand alone, in the order first seen.
	flags map[byte]bool
	// args are the values letters took, in the order they were written, so a
	// caller can have the last of them or all of them.
	args map[byte][]string
}

func (o systemOpts) on(letter byte) bool { return o.flags[letter] }

// value is the last value a letter took, and whether the letter appeared with
// one at all.
func (o systemOpts) value(letter byte) (string, bool) {
	got := o.args[letter]
	if len(got) == 0 {
		return "", false
	}
	return got[len(got)-1], true
}

// values is every value a letter took, in order.
func (o systemOpts) values(letter byte) []string { return o.args[letter] }

// number is a letter's value read as a descriptor or a size, with what the
// letter means when it was not written.
//
// A negative is refused with the rest: `sysread -i -1` and `sysread -s -1` are
// each `integer expected: -1` in zsh, not a descriptor going the other way.
func (o systemOpts) number(r *interp.Runner, letter byte, missing int) (int, int) {
	text, ok := o.value(letter)
	if !ok {
		return missing, 0
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		r.Diagnosef("integer expected: %s\n", text)
		return 0, 1
	}
	return n, 0
}

// duration is `-t`'s value: seconds, and a fraction of one is allowed —
// `sysread -s 1 -t 0.25` is written six times in one prompt theme.
func (o systemOpts) duration(r *interp.Runner, letter byte) (time.Duration, bool, int) {
	text, ok := o.value(letter)
	if !ok {
		return 0, false, 0
	}
	seconds, err := strconv.ParseFloat(text, 64)
	if err != nil {
		r.Diagnosef("integer expected: %s\n", text)
		return 0, false, 1
	}
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds * float64(time.Second)), true, 0
}

// systemOptions reads the leading option words of one of these builtins.
//
// valued names the letters that take a value and plain the letters that do
// not, which is the whole of what separates the five grammars. A letter in
// neither set is `bad option: -x` and a valued letter with nothing after it is
// `argument expected: -x` — both measured, in all five builtins, and both
// stop the command where they are found rather than being collected.
//
// **A dash followed by a digit is not an option word**, which is measured in
// every one of them and is not universal in the module: `sysseek -u 7 -1`
// seeks backwards and answers 2, `syswrite -1` writes the two characters,
// `sysread -1` is `not an identifier: -1`, and `sysopen -1` reaches `file
// descriptor not specified` — all of which say the word was taken as an
// operand. `zsystem flock -1` is `unknown option: 1`, so that builtin's own
// reader (see systemlock.go) does not have this rule and is right not to.
func systemOptions(r *interp.Runner, args []string, valued, plain string) (systemOpts, []string, int) {
	opts := systemOpts{flags: map[byte]bool{}, args: map[byte][]string{}}
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 && !isDigit(rest[0][1]) {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			letter := word[i]
			switch {
			case strings.IndexByte(plain, letter) >= 0:
				opts.flags[letter] = true
			case strings.IndexByte(valued, letter) >= 0:
				value, ok := systemLetterValue(word, &rest, i)
				if !ok {
					r.Diagnosef("argument expected: -%c\n", letter)
					return opts, nil, 1
				}
				opts.args[letter] = append(opts.args[letter], value)
				i = len(word)
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

// registerSystemIO installs `sysopen`, `sysread` and `syswrite`.
//
// The module's other three are registered by their own files, beside the
// parameters, in system.go. They are here in comment only because this is
// where they were once *not* registered, and the reason is worth keeping: for
// as long as that was true they were still in zmodload.go's feature list, so
// `zmodload -F zsh/system b:zsystem` refused **by that name** and a script
// calling one got `command not found` on the line that called it. That is the
// shape `zsh/files` uses for the nine plain names it will not register
// (#1668), and it is what makes a narrow module honest rather than hollow —
// the state to be in while a module is half-written, and not the state to
// leave it in.
func registerSystemIO(r *interp.Runner) {
	if !systemIOSupported {
		// A platform with no descriptors of this shape. Registering the names
		// anyway would be the hollow module the feature table exists to
		// avoid; left unregistered they refuse by their own names.
		return
	}
	r.Register("sysopen", sysopenBuiltin)
	r.Register("sysread", sysreadBuiltin)
	r.Register("syswrite", syswriteBuiltin)
}

// isDigit is the character test every option reader in this module makes: a
// dash followed by one is a number rather than an option. See systemOptions
// and, for the same rule reached from the other module, zselectOptions.
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// systemLetterValue is the value a letter takes: the rest of its own word
// where there is one, and the next word otherwise.
//
// Shared with `zsocket`, which had it first and whose `-d` takes a number the
// same way. One copy rather than two, because the rule it carries — that
// `-u fd` and `-ufd` are the same option — is the kind that gets fixed in one
// place and left broken in the other.
func systemLetterValue(word string, rest *[]string, i int) (string, bool) {
	if i+1 < len(word) {
		return word[i+1:], true
	}
	if len(*rest) == 0 {
		return "", false
	}
	value := (*rest)[0]
	*rest = (*rest)[1:]
	return value, true
}
