// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/tty"
	"github.com/blairham/sh/interp"
)

// `zsh/zpty`: one builtin, which runs a command with a pseudo-terminal of its
// own so that a program insisting on a terminal can be driven from a script.
//
// The contract is `docs/spec/pty.md`, measured against zsh 5.9.2 on
// 2026-09-19, 2026-09-20 and 2026-09-21; what is written here is the reading
// of it rather than a second copy. Three things about the shape are worth
// having in front of the code.
//
// **It needed a seam on the substrate before it could be written at all.** A
// dialect has three extension points — the vectors, a registered builtin, a
// sourced prelude — and none of them starts a command concurrently with
// descriptors of the caller's choosing. [interp.Runner.StartConcurrent] is
// that seam: `coproc`'s own machinery, generalized off the pipes it was
// written around, so the terminal case and the pipe case are one boundary
// rather than two. See interp/concurrentcommand.go.
//
// **A pty command is not a job.** Measured 2026-09-20: `zpty -b P 'sleep 2'`
// leaves `jobs` empty and `$!` at 0, which is why the seam keeps a table of
// its own rather than putting the command in the job table.
//
// **The table is the subshell's and the command in it is shared.** The four
// rows that say so are at the top of interp/concurrentcommand.go, and what
// they cost here is nothing: the store below is an ordinary shell parameter,
// which a subshell copies, and the handles are the seam's, which a subshell
// copies the same way.

// zptyStore is the table, kept as a flat array under a name no script can
// spell — the idiom `sched`, `zstyle` and `zmodload` all use here, and the
// one that gives a subshell its own copy.
//
// Five fields an entry: the name, the numbers the control end and the
// terminal end sit at in this runner's descriptor table, the option letters
// the command was started with, and the command as it was written. The open
// files themselves are in the descriptor table and not here, which is
// `zsystem flock`'s arrangement and for its reason: one table, not two to
// keep in step.
//
// **Both ends are kept, and the terminal end is kept for a measured reason.**
// Closing the terminal side of a pair discards whatever the control side has
// not yet read — measured on macOS 2026-09-21 — so a command that printed and
// exited would read back empty, where real zsh still answers with what it
// wrote. So the pair lives as long as the *entry* rather than as long as the
// command, and a read stops on the command having finished instead of on an
// end-of-file that must not be produced.
//
// **Newest first**, which is measured rather than incidental. Three commands
// started as P, Q, R list as R, Q, P under both `zpty` and `zpty -L`
// (2026-09-20), so a new entry goes on the front.
const zptyStore = ".zsh.zpty"

// zptyFields is how many array elements one entry takes.
const zptyFields = 5

// zptyPoll is how often a blocking read re-asks the terminal.
//
// A poll rather than a blocking read, for the reason `zsystem flock`'s wait
// is one: a read already made cannot be called back, and this shell has to
// stay answerable to the context its caller can cancel. Ten milliseconds, the
// same interval and for the same argument — a caller cannot ask for a slower
// answer than it asked for, and being a hundredth of a second early is not a
// difference a script can act on.
const zptyPoll = 10 * time.Millisecond

// zptyEntry is one row of the table.
type zptyEntry struct {
	name string
	// fd is the control end of the pair, in this runner's descriptor table.
	// It is what `$REPLY` was set to when the command started — measured,
	// `zpty -b P 'sleep 2'` leaves `REPLY` at a small number a `sysread -i`
	// can then use.
	fd int
	// terminalFd is the command's end, kept in the table as the shell's own
	// so that nothing inherits it and nothing duplicates it. See zptyStore.
	terminalFd int
	echo       bool
	nonstop    bool // -b: the pair is non-blocking in both directions
	command    string
}

// zptyOpts is what one `zpty` command line asked for.
type zptyOpts struct {
	echo      bool // -e
	nonstop   bool // -b
	del       bool // -d
	write     bool // -w
	noNewline bool // -n
	read      bool // -r
	matchOnly bool // -m
	test      bool // -t
	listing   bool // -L
}

// zptyBuiltin is every form of `zpty`.
func zptyBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts, rest, code := zptyOptions(r, args)
	if code != 0 {
		return code
	}
	switch {
	case opts.del:
		return zptyDelete(r, rest)
	case opts.write:
		return zptyWrite(r, opts, rest)
	case opts.read:
		return zptyRead(r, ctx, opts, rest)
	// `-t` is two builtins under one letter: with `-r` it polls the read and
	// is handled above, and alone it asks whether the command is running.
	case opts.test:
		return zptyTest(r, rest)
	case len(rest) == 0:
		return zptyList(r, opts.listing)
	}
	return zptyStart(r, ctx, opts, rest)
}

// zptyOptions reads the option words at the front, and stops at the first
// word that is not one.
//
// It stops there rather than reading the whole line, because everything after
// the name is the *command* and a command may perfectly well begin with a
// dash. An unknown letter is `bad option: -Q` at status 1, measured.
func zptyOptions(r *interp.Runner, args []string) (zptyOpts, []string, int) {
	var opts zptyOpts
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		for i := 1; i < len(word); i++ {
			switch word[i] {
			case 'e':
				opts.echo = true
			case 'b':
				opts.nonstop = true
			case 'd':
				opts.del = true
			case 'w':
				opts.write = true
			case 'n':
				opts.noNewline = true
			case 'r':
				opts.read = true
			case 'm':
				opts.matchOnly = true
			case 't':
				opts.test = true
			case 'L':
				opts.listing = true
			default:
				r.Diagnosef("bad option: -%c\n", word[i])
				return opts, nil, 1
			}
		}
		rest = rest[1:]
	}
	return opts, rest, 0
}

// zptyStart is `zpty [-e] [-b] NAME [arg ...]`.
//
// **The arguments are joined with spaces and read as one command**, not taken
// as an argv. The measurement that settles it is a parse error rather than a
// quoting difference: `v=inherited; zpty P print -n "<$v>"` is a parse error
// about `>`, because the word the shell built was `print -n <inherited>` and
// re-reading that made it a redirection. Two facts come with it — the name is
// **not** registered when the command will not parse, and a whole pipeline is
// a legal command.
func zptyStart(r *interp.Runner, ctx context.Context, opts zptyOpts, rest []string) int {
	name := rest[0]
	if len(rest) == 1 {
		r.Diagnosef("missing command\n")
		return 1
	}
	if _, _, found := zptyFind(r, name); found {
		r.Diagnosef("pty command name already used: %s\n", name)
		return 1
	}
	text := strings.Join(rest[1:], " ")
	parser := r.ParseWithAliases(text, Dialect())
	file := parser.Parse()
	if err := parser.Err(); err != nil {
		// Named for the command text rather than for the builtin, which is
		// what zsh does with it: the sentence is the *re-reading* of a word
		// this builtin built, so it is neither the builtin's complaint nor
		// the bare shell's. What is not reproduced is the location — zsh
		// sites it at a source of its own called `(zpty)` and this shell has
		// no second source to site it in, so the line is the caller's own.
		r.DiagnoseAsf("(zpty)", "%s\n", Diagnostics().ParseFailure(err))
		return 1
	}
	control, terminal, err := pty.Open()
	if err != nil {
		r.Diagnosef("can't create pty: %v\n", err)
		return 1
	}
	// Echo off unless `-e`, which is the setting neither tty.Raw nor
	// tty.Cbreak is — see internal/tty.SetEcho for the probe that
	// discriminates, since `cat` and `read` both answer the same either way.
	// A terminal that will not take the setting is still a terminal, so the
	// command starts either way.
	_ = tty.SetEcho(terminal, opts.echo)
	_, started := r.StartConcurrent(ctx, interp.ConcurrentCommand{
		Name: name,
		// All three of the command's descriptors, which is what makes
		// `[[ -t 1 ]]` true in there and what puts its complaints in the
		// output a `-r` reads. CloseEnds is left off: see zptyStore for the
		// measurement that says a terminal end closed with the command takes
		// the command's output with it.
		In: terminal, Out: terminal, Err: terminal,
		Run: func(ctx context.Context, sub *interp.Runner) error {
			return sub.RunPart(ctx, file)
		},
	})
	if !started {
		_ = control.Close()
		_ = terminal.Close()
		r.Diagnosef("pty command name already used: %s\n", name)
		return 1
	}
	// The shell's own end of the pair, as the shell's own rather than as a
	// file the script opened: an external command must not inherit it and a
	// second `zpty` must not take a copy of it. See
	// interp.Runner.OpenNearEnd, where the second half is what a pair
	// measurably stops working without.
	fd := r.OpenNearEnd(control)
	terminalFd := r.OpenNearEnd(terminal)
	// Measured: `$REPLY` is the control end's number, and it is a descriptor
	// this shell can use — `sysread -i $REPLY got` reads the command's output
	// through it.
	r.SetVar("REPLY", strconv.Itoa(fd))
	zptyWriteStore(r, append([]zptyEntry{{
		name: name, fd: fd, terminalFd: terminalFd,
		echo: opts.echo, nonstop: opts.nonstop, command: text,
	}}, zptyReadStore(r)...))
	return 0
}

// zptyDelete is `zpty -d [name ...]`, and `zpty -d` alone deletes every one.
//
// A name nobody started is `no such pty command` at status 1 and does **not**
// stop the rest: measured, `zpty -d NOPE P` complains, answers 1, and leaves
// P deleted.
func zptyDelete(r *interp.Runner, rest []string) int {
	entries := zptyReadStore(r)
	if len(rest) == 0 {
		for _, e := range entries {
			zptyEnd(r, e)
		}
		zptyWriteStore(r, nil)
		return 0
	}
	code := 0
	for _, name := range rest {
		e, _, found := zptyFind(r, name)
		if !found {
			zptyNoSuch(r, name)
			code = 1
			continue
		}
		zptyEnd(r, e)
		zptyWriteStore(r, zptyWithout(zptyReadStore(r), name))
	}
	return code
}

// zptyEnd stops one command and lets go of the shell's end of it.
//
// Stopping is [interp.Concurrent.Stop], which is the nearest this shell has to
// the HUP zsh sends: a command between commands ends at the next one and an
// external child of it is killed outright, because that is what canceling
// the context it was started with means.
//
// It is deliberately *not* conditional on the command still running. A
// subshell's delete reaches the process while the parent keeps the name, so
// the parent's own `-d` arrives at a command that is already gone and has to
// answer 0 — measured, and the row is at the top of
// interp/concurrentcommand.go.
func zptyEnd(r *interp.Runner, e zptyEntry) {
	if live, ok := r.ConcurrentNamed(e.name); ok {
		live.Stop()
		r.ForgetConcurrent(e.name)
	}
	r.CloseDescriptor(e.fd)
	r.CloseDescriptor(e.terminalFd)
}

// zptyTest is `zpty -t NAME`: whether the command is still running.
//
// Three answers rather than two, and the middle one is what an implementation
// gets wrong by forgetting that a command can end without its name going:
// 0 while it runs, 1 and **silent** once it has ended, and 1 with a
// diagnostic once the name is gone.
func zptyTest(r *interp.Runner, rest []string) int {
	if len(rest) == 0 {
		// Every other form of this builtin takes a name here, and a bare
		// `-t` has nothing to ask about.
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if len(rest) > 1 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	if _, _, found := zptyFind(r, rest[0]); !found {
		zptyNoSuch(r, rest[0])
		return 1
	}
	live, ok := r.ConcurrentNamed(rest[0])
	if !ok || !live.Running() {
		return 1
	}
	return 0
}

// zptyWrite is `zpty -w [-n] NAME [string ...]`.
//
// The strings are joined with spaces and a newline is added unless `-n`. With
// no string at all this shell's standard input is copied to the terminal, and
// `-n` is not applied to that.
func zptyWrite(r *interp.Runner, opts zptyOpts, rest []string) int {
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	e, _, found := zptyFind(r, rest[0])
	if !found {
		zptyNoSuch(r, rest[0])
		return 1
	}
	w, ok := r.WriterForFd(e.fd)
	if !ok {
		zptyNoSuch(r, rest[0])
		return 1
	}
	if len(rest) == 1 {
		// The copy may stop short where the pair is non-blocking, which is
		// the status 0 zsh answers for a `print -n piped | zpty -w P`.
		_, _ = io.Copy(w, r.In())
		return 0
	}
	text := strings.Join(rest[1:], " ")
	if !opts.noNewline {
		text += "\n"
	}
	if _, err := io.WriteString(w, text); err != nil {
		return 1
	}
	return 0
}

// zptyRead is `zpty -r [-mt] NAME [param [pattern]]`, which is three commands
// sharing a letter.
//
// With only a name the output is copied to this shell's standard output; with
// a parameter at most one line is read into it; with a pattern as well the
// read runs until the whole string read so far matches, and **stops at the
// match** — `zpty -w P abcDONExyz; zpty -r P l '*DONE*'` leaves `l=abcDONE`
// with `xyz` still in the terminal.
//
// # The statuses
//
// Re-measured 2026-09-21 on zsh 5.9.2, because the finer split
// docs/spec/pty.md recorded first does not reproduce. Six reads, each with
// no pattern so that nothing could hang:
//
//	written                                    zsh 5.9.2
//	-r G a   (wrote `hi`, then exited)         0, a = hi
//	-r G b   (nothing left)                    2
//	-r G c   (again)                           2
//	-r H d   (wrote nothing, exited)           2
//	-r I f   (non-blocking, still running)     1
//	-r I g   (the same pty, after it exited)   2
//
// So it is the *command* that decides, not the pattern: something read is 0,
// nothing read from a command that has finished is 2, and nothing read from
// one that is still running is 1. The earlier reading — no pattern is 1 and a
// pattern is 2 — held for one sequence and not for these.
//
// **A read that comes back with nothing leaves the parameter as it was.**
// That is what `-t` is measured to do and it is what this does for every
// failing read: the values that build's failing reads leave behind are an
// argument word and a previous read's text, which are artifacts rather than
// behavior.
func zptyRead(r *interp.Runner, ctx context.Context, opts zptyOpts, rest []string) int {
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if len(rest) > 3 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	e, _, found := zptyFind(r, rest[0])
	if !found {
		zptyNoSuch(r, rest[0])
		return 1
	}
	param, pattern := "", ""
	if len(rest) > 1 {
		param = rest[1]
	}
	if len(rest) > 2 {
		pattern = rest[2]
	}
	reader, ok := r.ReaderForFd(e.fd)
	if !ok {
		zptyNoSuch(r, rest[0])
		return 1
	}
	sys, addressable := r.SystemDescriptor(e.fd)
	// `-t` polls first and answers 1 at once when nothing is waiting,
	// leaving the parameter where it found it.
	if opts.test && !zptyReadable(sys, addressable) {
		return 1
	}
	running := func() bool {
		live, ok := r.ConcurrentNamed(e.name)
		return ok && live.Running()
	}
	got, matched := zptyCollect(r, ctx, e, reader, sys, addressable, running, param, pattern)
	if len(got) == 0 || (opts.matchOnly && !matched) {
		return zptyNothingRead(r, e)
	}
	if param == "" {
		_, _ = r.Out().Write(got)
		return 0
	}
	r.StoreThroughOperand(param, string(got))
	return 0
}

// zptyCollect reads until the read is done, and says whether the pattern
// matched.
//
// One byte at a time, because every stopping condition here is about the
// string read *so far*: a pattern is matched against it after each byte and a
// line ends at the first newline, so a reader that took a block would have to
// give the rest back.
func zptyCollect(r *interp.Runner, ctx context.Context, e zptyEntry, reader io.Reader, sys int, addressable bool, running func() bool, param, pattern string) ([]byte, bool) {
	var got []byte
	var one [1]byte
	for {
		switch {
		case pattern != "" && len(got) > 0 && r.MatchPattern(pattern, string(got)):
			return got, true
		case pattern == "" && param != "" && len(got) > 0 && got[len(got)-1] == '\n':
			return got, false
		}
		if e.nonstop {
			// Non-blocking: only what is immediately available.
			if !zptyReadable(sys, addressable) {
				return got, false
			}
		} else if !zptyWaitReadable(ctx, sys, addressable, running) {
			return got, false
		}
		n, err := reader.Read(one[:])
		if n > 0 {
			got = append(got, one[0])
			continue
		}
		// End of input, which on a pseudo-terminal whose other side has gone
		// is an error on one platform and an end-of-file on the other. Both
		// mean the same thing here and neither is a complaint to a script.
		if err != nil {
			return got, false
		}
	}
}

// zptyNothingRead is the status a read with nothing to show answers: 2 where
// the command has finished and 1 where it is still going. See zptyRead.
func zptyNothingRead(r *interp.Runner, e zptyEntry) int {
	if live, ok := r.ConcurrentNamed(e.name); ok && live.Running() {
		return 1
	}
	return 2
}

// zptyReadable asks whether anything is waiting, without waiting.
func zptyReadable(sys int, addressable bool) bool {
	if !addressable {
		return false
	}
	none := time.Duration(0)
	ready, _, _, err := fdset.Ready([]int{sys}, nil, nil, &none)
	return err == nil && len(ready) > 0
}

// zptyWaitReadable is the blocking half, and it is a poll for two reasons.
//
// The first is zptyPoll's: a read already made cannot be called back, and a
// shell that made one would stop answering the context its caller can cancel.
//
// The second is what ends the wait. The terminal end is deliberately held open
// past the command — see zptyStore — so a read never finds end-of-file and
// there is nothing for a blocking read to come back from. What ends it is the
// *command*: `zpty -r P` copies until the command exits, and one asked for a
// line waits only as long as something can still write one. The order matters
// and it is the one race here: the command is asked about **before** the last
// poll, so a command that wrote and exited between the two answers still has
// its bytes read rather than losing them to the wrong question's timing.
func zptyWaitReadable(ctx context.Context, sys int, addressable bool, running func() bool) bool {
	if !addressable {
		return false
	}
	for {
		alive := running()
		if zptyReadable(sys, addressable) {
			return true
		}
		if !alive {
			return false
		}
		if ctx == nil {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(zptyPoll):
		}
	}
}

// zptyList is `zpty` and `zpty -L`.
//
// The first writes the process, the name and the command as it was given; the
// second writes the call that would reproduce it, flags included. Both newest
// first, and both 0 over an empty table.
//
// `(finished)` where a pid would be is measured rather than invented: a
// command that has ended lists that way in zsh 5.9.2, which is the only thing
// this shell could honestly print anyway — a command here is a goroutine and
// the number beside a live one is the job identity the substrate invented for
// it.
func zptyList(r *interp.Runner, reproducible bool) int {
	for _, e := range zptyReadStore(r) {
		if reproducible {
			letters := ""
			if e.echo {
				letters += " -e"
			}
			if e.nonstop {
				letters += " -b"
			}
			_, _ = io.WriteString(r.Out(), "zpty"+letters+" "+e.name+" "+quoteStyleWord(e.command)+"\n")
			continue
		}
		_, _ = io.WriteString(r.Out(), "("+zptyWho(r, e)+") "+e.name+": "+quoteStyleWord(e.command)+"\n")
	}
	return 0
}

// zptyWho is what a listing prints where a pid goes.
func zptyWho(r *interp.Runner, e zptyEntry) string {
	live, ok := r.ConcurrentNamed(e.name)
	if !ok || !live.Running() {
		return "finished"
	}
	return strconv.Itoa(live.Ident())
}

// zptyNoSuch is the one refusal `-r`, `-w`, `-d` and `-t` share, word for
// word and status for status.
func zptyNoSuch(r *interp.Runner, name string) {
	r.Diagnosef("no such pty command: %s\n", name)
}

func zptyFind(r *interp.Runner, name string) (zptyEntry, int, bool) {
	for i, e := range zptyReadStore(r) {
		if e.name == name {
			return e, i, true
		}
	}
	return zptyEntry{}, 0, false
}

func zptyWithout(entries []zptyEntry, name string) []zptyEntry {
	out := entries[:0]
	for _, e := range entries {
		if e.name != name {
			out = append(out, e)
		}
	}
	return out
}

func zptyReadStore(r *interp.Runner) []zptyEntry {
	flat, _ := r.GetArray(zptyStore)
	out := make([]zptyEntry, 0, len(flat)/zptyFields)
	for i := 0; i+zptyFields <= len(flat); i += zptyFields {
		fd, err := strconv.Atoi(flat[i+1])
		if err != nil {
			continue
		}
		terminalFd, err := strconv.Atoi(flat[i+2])
		if err != nil {
			continue
		}
		out = append(out, zptyEntry{
			name:       flat[i],
			fd:         fd,
			terminalFd: terminalFd,
			echo:       strings.ContainsRune(flat[i+3], 'e'),
			nonstop:    strings.ContainsRune(flat[i+3], 'b'),
			command:    flat[i+4],
		})
	}
	return out
}

func zptyWriteStore(r *interp.Runner, entries []zptyEntry) {
	flat := make([]string, 0, len(entries)*zptyFields)
	for _, e := range entries {
		letters := ""
		if e.echo {
			letters += "e"
		}
		if e.nonstop {
			letters += "b"
		}
		flat = append(flat, e.name, strconv.Itoa(e.fd), strconv.Itoa(e.terminalFd), letters, e.command)
	}
	r.SetArray(zptyStore, flat)
}

// registerZptyModule installs `zsh/zpty`'s one builtin.
//
// The whole module, and the name shadows nothing on any machine — the test
// #1668 set for whether a name may be registered at all, and the reason
// `zsh/stat`'s `stat` and `zsh/files`' nine plain spellings are not.
//
// Nothing is registered where this build cannot open a pseudo-terminal, which
// is zselect's rule and for its reason: a `zpty` that answered without a
// terminal would walk a script past the `zmodload zsh/zpty 2>/dev/null ||
// return` it wrote, into a capture that can never produce anything. Left
// unregistered the builtin refuses by its own name and `zmodload zsh/zpty`
// refuses with it.
func registerZptyModule(r *interp.Runner) {
	if !zptySupported {
		return
	}
	r.Register("zpty", zptyBuiltin)
}
