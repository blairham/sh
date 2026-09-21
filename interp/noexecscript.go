// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"syscall"

	"github.com/blairham/sh/syntax"
)

// A file the kernel will not start is a shell script.
//
// `execve` answers ENOEXEC for a file that has the execute bit and is not an
// executable image — no `#!` line and no magic the operating system knows.
// POSIX XCU says the shell then runs the file **as a shell script**, and every
// member of the panel does. Measured 2026-09-13 with
// `printf 'echo ran-as-script "$@" "$0"\n' > ne.scr && chmod +x ne.scr`:
//
//	dash, bash 5.3.15, bash as `sh`, bash 3.2.57, zsh 5.9.2, ash
//	                 `ran-as-script a b <path>`, status 0
//	ksh93u+          the same, except `$0` is the word as typed
//	ours (before)    `<shell>: line 1: ne.scr: fork/exec <path>: exec
//	                 format error`, status 126
//
// Unanimous, so it is a correction rather than an axis — and a Go string was
// leaking into a shell diagnostic on the way out. A script with no `#!` is
// ordinary: `Makefile` recipes, hand-written git hooks, anything a tool
// generated and `chmod +x`'d. Every one of them failed here (#2580).
//
// # Which shell runs it
//
// A fresh one. Measured with a script that prints `$0`, `$#`, the exported
// and unexported variables around it and the caller's functions: the script
// sees the **exported** environment and nothing else — no functions, no
// unexported variables, no options. So this is not a `source`; it is the
// nearest thing a library may do to what a real shell does, which is a child
// Runner seeded from the environment the command would have been given, with
// `$0` the path and the rest of the words the positional parameters.
//
// *Whose* shell is where the panel's implementations part, and it parts in a
// place that is not ours to reproduce. bash and ksh93 run the file with
// themselves; zsh and dash hand it to `/bin/sh`, which on the machine the
// panel runs on is bash 3.2 — measured, a `who.scr` printing `$BASH_VERSION`
// reports 3.2.57 under both. That is a fact about `/bin/sh` on this machine
// rather than about zsh or dash, and reproducing it would mean starting a
// shell nobody named. So the file is run by **this** shell's dialect, which
// is what bash and ksh93 do and what zsh and dash do everywhere `/bin/sh` is
// the shell in question. Every construct the panel was measured on here is in
// the common denominator, so the columns agree whichever reading is taken.
//
// # What must still fail
//
// Not every ENOEXEC is a script. A genuine binary for another architecture
// answers ENOEXEC too, and reading it as a script would turn a clear failure
// into a spray of `command not found`. The panel looks before it leaps, and
// what it looks for is measured rather than guessed — a NUL byte in the
// file's **first line**, bounded by a sample of about eighty bytes:
//
//	file                                bash 5.3  bash 3.2  zsh  ksh93  ash
//	`echo a\0b\n…`      NUL in line 1        126       126  126    126    0
//	`\0echo zero\n`     NUL at byte 0        126       126  126    126    0
//	`echo one\necho t\0wo\n`  NUL later        0         0    0      3     0
//	`echo nulearly\n\0\0rest\n`  NUL past
//	                    the newline            0         0    0      3     0
//	`echo x…x\0\n`      NUL at byte 78       126       126  126    126    0
//	`echo x…x\0\n`      NUL at byte 205        0         0    0    126     0
//	`\x7fELF echo …`    ELF magic, no NUL    126         0    0      0     0
//
// So the rule the five agree on is the first line and a NUL in it; the sample
// bound is ksh93's alone to disagree about, and the ELF magic is bash 5.3's.
// A real ELF or Mach-O header contains NULs within a few bytes, which is why
// the narrow rule is enough for the case it exists for.
//
// BusyBox ash is the column that does not look at all — it reads every one of
// those as a script, a Mach-O header included. That is the axis, and it is
// Semantics.BinaryContentIsNotRunAsAScript.
const binarySample = 80

// notAnExecutableImage reports the one errno this fallback is for.
//
// ENOEXEC specifically, never "the start failed". A fallback keyed on any
// exec failure would read a file the policy withheld, a file on a `noexec`
// mount and a binary for the wrong architecture as shell scripts, and would
// hide exactly the failures a person needs to see.
func notAnExecutableImage(err error) bool {
	return errors.Is(err, syscall.ENOEXEC)
}

// looksBinary reports whether an image the kernel refused holds a NUL byte in
// its first line — see the table above for the measurement that drew the
// bound here rather than over the whole file.
func looksBinary(image []byte) bool {
	if i := bytes.IndexByte(image, '\n'); i >= 0 {
		image = image[:i]
	}
	if len(image) > binarySample {
		image = image[:binarySample]
	}
	return bytes.IndexByte(image, 0) >= 0
}

// notAnImage is the failure a file with binary content in it is reported
// with: the errno the kernel gave, so that every dialect's wording for
// ENOEXEC comes out of the one place the other exec failures come out of.
var notAnImage = syscall.ENOEXEC

// imageVerdict is what a failed start turns out to have been.
type imageVerdict int

const (
	// imageNotOurs is every failure this fallback is not for: an errno that
	// is not ENOEXEC, and a file this shell may not read. The caller reports
	// the start failure it already had, which keeps a withheld file looking
	// withheld rather than turning it into a second diagnostic.
	imageNotOurs imageVerdict = iota
	// imageBinary is a file with binary content in it — reported, never run.
	imageBinary
	// imageScript is the ordinary case: shell text with no `#!` line.
	imageScript
)

// classifyImage reads a file the kernel refused and says which of the three
// it is.
//
// One reader for both doors — a command word and `exec` — because the gate,
// the errno and the binary check are the same question however the file was
// reached, and a second copy of them is how one door learns something the
// other does not.
func (r *Runner) classifyImage(ctx context.Context, path string, err error) ([]byte, imageVerdict) {
	if !notAnExecutableImage(err) {
		return nil, imageNotOurs
	}
	// The read is an open to the gate for the reason `.`'s is: this pulls a
	// file into the interpreter and then runs it, which is the re-entry the
	// seams exist for.
	open := r.act(Action{Kind: ActionOpen, Path: path})
	if r.openQuietlyDenied(open) {
		return nil, imageNotOurs
	}
	image, readErr := r.readFileGated(ctx, &open, path)
	if readErr != nil {
		return nil, imageNotOurs
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: open})
	if looksBinary(image) &&
		r.ask(r.sem().BinaryContentIsNotRunAsAScript,
			"a file with binary content in it not being read as a shell script") {
		return image, imageBinary
	}
	return image, imageScript
}

// imageAsScript answers a command word whose start failed because the file is
// not an executable image, and reports whether it answered at all.
//
// The status is the script's own. `printf 'exit 7\n' > e.scr` and `./e.scr`
// is 7 in all seven columns, as is a `$?` of 0 for a script that ends well.
func (r *Runner) imageAsScript(ctx context.Context, action Action, path string, argv, env []string, err error) (int, bool) {
	image, verdict := r.classifyImage(ctx, path, err)
	switch verdict {
	case imageNotOurs:
		return 0, false
	case imageBinary:
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		// Reported with the errno rather than with Go's wrapper, so the
		// sentence comes out of the same place every other exec failure's
		// does — and so the dialect that has a phrase for this one can say
		// it. See Diagnostics.BinaryFileReason.
		return r.cannotRun(&pathError{name: argv[0], resolved: path, err: notAnImage}, naming{
			bare:     r.diag().NotFound,
			fallback: "%[1]s: not found",
		}), true
	}
	status := r.runImageAsScript(ctx, r.imageZero(argv[0], path, false), path, argv, env, image)
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: status})
	return status, true
}

// execImageAsScript is the same question asked by `exec`, which reaches the
// file by a different door and must not answer differently.
//
// `exec ./ne.scr a b` runs the script and does not come back in any column —
// measured, `exec ./ne.scr a b; echo NOT-REACHED` prints the script's output
// and no `NOT-REACHED` in all six local columns. So a replacement that failed
// with ENOEXEC runs the script and then ends this shell with its status,
// which is what the successful replacement-by-child path beside it does.
//
// The binary half ends the shell too, and by the road every other failed
// `exec` takes: measured, `exec ./elf64_hdr.bin; echo NOT-REACHED` prints no
// NOT-REACHED anywhere, at 126 in five columns.
func (r *Runner) execImageAsScript(ctx context.Context, action Action, path string, argv, env []string, err error) (int, bool) {
	image, verdict := r.classifyImage(ctx, path, err)
	switch verdict {
	case imageNotOurs:
		return 0, false
	case imageBinary:
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		return r.execEnds(r.execCannotRun(
			&pathError{name: argv[0], resolved: path, err: notAnImage})), true
	}
	status := r.runImageAsScript(ctx, r.imageZero(argv[0], path, true), path, argv, env, image)
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: status})
	// The shell stops, and the EXIT trap does not run, for the reason the
	// replacement beside this one says: the trap died with the process the
	// exec replaced, and standing in for that replacement means saying so.
	r.status = status
	r.exitTrap = nil
	r.stopTheShell()
	return status, true
}

// imageZero is the `$0` a file run as a shell script is given.
//
// Measured 2026-09-13 across the panel. Reached as `./ne.scr` it is `./ne.scr`
// in every column; reached as a bare name off PATH it is the path the search
// resolved in dash, both bash builds, bash as `sh`, zsh and ash, and the word
// as typed in ksh93 — which is Semantics.ScriptImageSeesTheResolvedPath, and
// is the same split NamesResolvedPath records for a diagnostic asked here
// about a parameter.
//
// A word with a slash in it is the word in every column but one, and the
// exception is `exec`: `exec ./ne.scr` hands the script `/tmp/…/ne.scr` in
// all three bash columns and `./ne.scr` in zsh, ksh93 and dash. That is the
// same habit NamesResolvedPath already names — bash absolutises an operand
// `exec` was given and leaves a command word as written — so it is read from
// there rather than given a second field that would drift from it.
//
// The axis is asked at the disagreement and nowhere else: with a slash in the
// word the two readings are the same string, and a shell with nothing to
// decide must not be able to refuse.
func (r *Runner) imageZero(word, path string, byExec bool) string {
	if strings.ContainsRune(word, '/') {
		if byExec && r.diag().NamesResolvedPath {
			return path
		}
		return word
	}
	if r.ask(r.sem().ScriptImageSeesTheResolvedPath, "the `$0` a file run as a shell script is given") {
		return path
	}
	return word
}

// runImageAsScript runs the file in a shell of its own.
//
// A **fresh** Runner rather than a clone: a clone is a subshell, which is the
// one thing this is measurably not — the script sees no function and no
// unexported variable of the shell that started it. Building it from the
// exported fields alone is what makes that true by construction, where
// stripping a copy would leave whichever unexported field the next change
// adds.
//
// What crosses is what crosses to any other child: the environment the
// command was going to be given, the three streams as this command's
// redirections left them, the descriptors past them in the layout a child
// gets, and the working directory. What does not is everything a process
// boundary would have stopped.
func (r *Runner) runImageAsScript(ctx context.Context, name, path string, argv, env []string, image []byte) int {
	child := &Runner{
		// The file is the program, which is what decides `$0`'s rule, the
		// name a diagnostic gives the shell, and the route letters.
		Route:       RouteScriptFile,
		Dialect:     r.Dialect,
		Semantics:   r.Semantics,
		Diagnostics: r.Diagnostics,
		AxisRemedy:  r.AxisRemedy,
		Name:        name,
		Invocation:  r.Invocation,
		Params:      append([]string(nil), argv[1:]...),
		Env:         env,
		Dir:         r.Dir,
		Stdin:       r.Stdin,
		// And what *its* children inherit in place of that, which is the
		// caller's and not this shell's to change: the script this stands in
		// for is a child of ours, so everything below it is one too. See
		// Runner.ChildStdin.
		ChildStdin:      r.ChildStdin,
		Stdout:          r.Stdout,
		Stderr:          r.Stderr,
		Gate:            r.Gate,
		Events:          r.Events,
		Session:         r.Session,
		GuardConcurrent: r.GuardConcurrent,
		Terminal:        r.Terminal,
		// The table a child gets, in the layout a fresh shell is handed one
		// in: entry i is descriptor 3+i and a nil is a number closed there.
		// Nearly the slice an external command would have been given — see
		// imageFiles for the one measured difference, which is that the
		// dialect withholding `exec`'s descriptors from a command does not
		// withhold them from this.
		InheritedFiles: r.imageFiles(),
		// The hooks about starting, waiting for and signaling *other*
		// processes. A shell started by an execve would have inherited every
		// one of these, and they are what a script needs to run commands of
		// its own and have its jobs behave.
		WaitForCommand: r.WaitForCommand,
		PollCommand:    r.PollCommand,
		Foreground:     r.Foreground,
		SignalGroup:    r.SignalGroup,
		TakeInterrupt:  r.TakeInterrupt,
		GetRlimit:      r.GetRlimit,
		ProcessAnchor:  r.ProcessAnchor,
		// And deliberately **not** the hooks that change *this* process.
		//
		// A real shell gives the script a process of its own, so nothing it
		// does to that process comes back; this one is running inside the
		// shell that started it, and every one of them would come back:
		//
		//	ReplaceProcess  an `exec` in the script would replace the shell
		//	                that is still waiting for it. The `exec` still
		//	                works — execbuiltin falls back to the child route
		//	                it already uses whenever a replacement is not
		//	                available — and what it loses is being the same
		//	                process, which it was never going to be here.
		//	DieBySignal     a script killed by a signal would take the whole
		//	                shell with it, where its caller is supposed to
		//	                report the death and carry on.
		//	SetRlimit       a limit the script lowered would stay lowered for
		//	                the shell afterwards, and a lowered hard limit
		//	                cannot be raised again by anybody.
		//
		// `umask` is the fourth of that shape and is the one exception: it
		// has to be able to reach the process, because the files the script
		// creates really are created by it — a child it spawns inherits the
		// mask at the fork, and without the hook a `umask 077` in such a
		// script would do nothing at all. What the fork would have done for
		// free — keeping that mask off the caller — is the mask being the
		// Runner's own rather than the process's. See umaskscope.go.
		SetUmask: r.SetUmask,
	}
	// The mask itself, handed over rather than read back off the process:
	// ensureUmask has already emptied the process's, so a fresh shell asking
	// it what the mask is would be told 0. This is what a fork would have
	// copied, and the child's own changes stay in the child because the field
	// is the child's.
	child.umask, child.maskKnown = r.umask, r.maskKnown
	// Everything the front end does to a Runner past its fields — the
	// dialect's builtins, its ties, its prompt table. Carried on as well as
	// applied, so that a shebang-less script which itself runs one gets the
	// same shell the first did. See Runner.SetUp.
	child.SetUp = r.SetUp
	if r.SetUp != nil {
		r.SetUp(child)
	}
	// The file the shell was given, which is the name it answers with rather
	// than the path it opened: a diagnostic raised inside the script names it
	// the way `$0` does, which on the PATH route is the resolved path in six
	// columns and the word in ksh93.
	child.SetScriptFile(name)
	src := string(image)
	f, perr := syntax.Parse(src, r.dialect().On(syntax.RouteFromScriptFile))
	if perr != nil {
		// Reported as the file's own failure, by the dialect that would have
		// reported it had this shell been started on the file — which is
		// what the child is. The shell is named rather than the operand: on
		// the script route every column prints the script's path, including
		// the one that shortens its own name elsewhere.
		child.errf("%s", child.diag().ParseDiagnostic(child.name(), "", perr, src))
		return child.diag().StatusForParseError(perr)
	}
	status, runErr := child.Run(ctx, f)
	if runErr != nil && !errors.Is(runErr, fs.ErrClosed) {
		// A run the context stopped, or a construct this shell has not got.
		// The status the child reached is still the answer; the error has
		// already been said wherever it was raised.
		return status
	}
	return status
}
