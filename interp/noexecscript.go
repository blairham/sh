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

// ErrBinaryScript is a file offered to a shell as a *script* whose content is
// not shell text — the script-operand half of the question looksBinary asks of
// a file the kernel refused.
//
// An error of its own rather than an errno, because no system call failed: the
// file opened and was read, and the shell then declined to run what came out.
// See Diagnostics.ScriptBinaryContent, which is what a dialect says about it.
var ErrBinaryScript = errors.New("cannot execute binary file")

// BinaryScriptContent reports whether a file's content is the kind a shell
// refuses to read as a script, by the rule looksBinary draws: a NUL byte in
// the first line, within the sample bound.
//
// Exported for the front end, which meets the same question at the other door
// — a script *operand*, where nothing has been executed and there is no
// ENOEXEC to key on. One rule for both, so a file that is not a script at one
// door is not a script at the other.
func BinaryScriptContent(image []byte) bool { return looksBinary(image) }

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

// interpreterWords splits a file's `#!` line into the two words the kernel
// would have read off it: the program to start and the one argument such a
// line may carry.
//
// One argument and not several. Measured 2026-09-25 on zsh 5.9.2, a line
// reading `#!myinterp   one   two  ` hands the interpreter a single argument
// spelled `  one   two` — everything after the first separator, with the
// trailing blanks taken off and the leading ones kept.
//
// ok is false for a file with no `#!` line at all, which is a different
// answer from a `#!` naming nothing: the second is what
// Semantics.EmptyInterpreterLineIsNotAScript is asked about.
func interpreterWords(image []byte) (name, arg string, ok bool) {
	line, _, _ := bytes.Cut(image, []byte("\n"))
	rest, ok := bytes.CutPrefix(line, []byte("#!"))
	if !ok {
		return "", "", false
	}
	text := strings.TrimLeft(string(rest), " \t")
	if cut := strings.IndexAny(text, " \t"); cut >= 0 {
		// One separator is consumed and the rest is the argument, blanks and
		// all — see the measurement above.
		name, arg = text[:cut], strings.TrimRight(text[cut+1:], " \t")
	} else {
		name = strings.TrimRight(text, " \t")
	}
	return name, arg, true
}

// interpreterLine reads a file's `#!` line, for a start the kernel refused.
//
// It reads the file for the reason classifyImage does, through the same gate
// and for the same re-entry: a policy that hides a path hides what is written
// inside it too.
func (r *Runner) interpreterLine(ctx context.Context, path string) (name, arg string, ok bool) {
	open := r.act(Action{Kind: ActionOpen, Path: path})
	if r.openQuietlyDenied(open) {
		return "", "", false
	}
	image, readErr := r.readFileGated(ctx, &open, path)
	if readErr != nil {
		return "", "", false
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: open})
	return interpreterWords(image)
}

// interpreterNamed is the name on a file's `#!` line, for a start the kernel
// refused because something was not there.
//
// The file this shell found is present — the lookup opened it and the execute
// bit is on it — so an ENOENT out of the start is about the *interpreter* and
// not about the command. Nothing else can produce one at that door, which is
// why the errno is the whole of the test and the `#!` line is only read to
// find the name to print.
//
// Read in every dialect rather than only in the two that have a sentence for
// it. What the read decides is not only the wording: a dialect with nothing
// to say about the interpreter still has to number this failure the way it
// numbers a name that was not found, which is 127 and not the 126 a start
// this shell could not make otherwise reports (#4454).
func (r *Runner) interpreterNamed(ctx context.Context, path string, err error) string {
	if !errors.Is(err, syscall.ENOENT) {
		return ""
	}
	name, _, _ := r.interpreterLine(ctx, path)
	return name
}

// interpreterRetry records a start this shell is making a second time, with
// the interpreter a `#!` line named and a PATH search found.
//
// It is what keeps the retry to one level, and it is also what the second
// failure is reported against: the file whose line named the interpreter,
// with the word that line held — not the interpreter's own file and not
// whatever its own first line says. Measured on zsh 5.9.2 with an interpreter
// that is itself a script with an unresolvable `#!`.
type interpreterRetry struct {
	// name is the command word as the script wrote it.
	name string
	// resolved is the file whose `#!` line was read.
	resolved string
	// interpreter is the word that line held, as written.
	interpreter string
}

// interpreterFailure is the pathError a start the kernel refused with ENOENT
// is reported as: the file is present, so what was not there is the program
// its `#!` line named.
//
// missing, because that is what it is, and it is not only a wording: it is
// what numbers the failure 127 rather than 126 in the dialects that have no
// `bad interpreter` sentence of their own. Empty interpreter where the line
// could not be read, which leaves the caller's own reporting unchanged.
func (r *Runner) interpreterFailure(ctx context.Context, name, path string, err error) *pathError {
	named := r.interpreterNamed(ctx, path, err)
	if retry := r.interpRetry; retry != nil {
		// The interpreter this shell went and found would not start either.
		// The file at fault is still the one whose `#!` line named it, and
		// the word to print is the one that line held — see interpreterRetry.
		name, path, named = retry.name, retry.resolved, retry.interpreter
	}
	if named == "" {
		return nil
	}
	return &pathError{name: name, resolved: path, interpreter: named, missing: true, err: err}
}

// interpreterArgv is the command line a start refused with ENOENT should be
// tried again as, having looked the `#!` line's first word up on PATH — which
// is Semantics.SlashlessInterpreterIsPathSearched.
//
// It reports false for a word with a slash in it, a dialect that does not
// search and a search that found nothing, all of which fall through to the
// diagnostic, which is where `bad interpreter` is said. On true it has set
// Runner.interpRetry, and the caller clears it once the retry is over.
func (r *Runner) interpreterArgv(ctx context.Context, path string, argv []string, err error) ([]string, bool) {
	if r.interpRetry != nil || !errors.Is(err, syscall.ENOENT) {
		// One level. An interpreter that is itself a file with a `#!` this
		// shell cannot resolve would otherwise be searched for again, and
		// each round would add a word to the argv it is building.
		return nil, false
	}
	name, arg, ok := r.interpreterLine(ctx, path)
	if !ok || name == "" || strings.ContainsRune(name, '/') {
		// Nothing to search for. A name with a slash in it is a path and was
		// already handed to the kernel as one, which is unanimous.
		return nil, false
	}
	if !r.ask(r.sem().SlashlessInterpreterIsPathSearched,
		"a `#!` line naming an interpreter with no slash in it being looked up on PATH") {
		return nil, false
	}
	found, lookErr := r.lookPath(name)
	if lookErr != nil {
		// Not on PATH either, so the interpreter genuinely is not there and
		// the caller says so — the same sentence it would have said had the
		// name been written as a path.
		return nil, false
	}
	// The file as the kernel would have handed it over: the path the search
	// resolved for a bare name, and the word as written for one that already
	// had a slash in it. Relative is safe either way, because the child is
	// started in this runner's directory rather than the process's.
	script := path
	if strings.ContainsRune(argv[0], '/') {
		script = argv[0]
	}
	next := make([]string, 0, len(argv)+2)
	next = append(next, found)
	if arg != "" {
		next = append(next, arg)
	}
	next = append(next, script)
	next = append(next, argv[1:]...)
	r.interpRetry = &interpreterRetry{name: argv[0], resolved: path, interpreter: name}
	return next, true
}

// startViaNamedInterpreter answers a command word whose start the kernel
// refused, by running the file with the interpreter its `#!` line named and a
// PATH search found. It reports whether it answered at all.
func (r *Runner) startViaNamedInterpreter(ctx context.Context, path string, argv, env []string, err error) (bool, error) {
	next, ok := r.interpreterArgv(ctx, path, argv, err)
	if !ok {
		return false, nil
	}
	defer func() { r.interpRetry = nil }()
	return true, r.exec(ctx, next, env)
}

// reportStartFailure is the tail every door a start can fail at shares: the
// event, the diagnostic and the status.
//
// One place rather than three, because the three doors — a background job, a
// watched foreground command and a plain one — differ in how they wait and
// not at all in what they say when there was nothing to wait for. The bad
// interpreter above was the second thing this shell had learned at one door
// and not the others.
func (r *Runner) reportStartFailure(ctx context.Context, action Action, argv []string, path string, err error) int {
	r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
	if pe := r.interpreterFailure(ctx, argv[0], path, err); pe != nil {
		// Through cannotRun, so this failure is named the way every other
		// one at this door is — `exec` absolutises the path and a command
		// word does not, in the dialect that tells them apart.
		return r.cannotRun(pe, naming{
			bare:     r.diag().NotFound,
			fallback: "%[1]s: not found",
		})
	}
	r.diagf("%s: %v\n", argv[0], err)
	return 126
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
	// imageNoInterpreter is a file whose first line is a `#!` naming nothing.
	// Reported rather than run too, and for the same reason — the shell read
	// the file and declined what came out — but it is a second question with
	// a second answer behind it, so it is a second verdict rather than a
	// widening of the one above. See
	// Semantics.EmptyInterpreterLineIsNotAScript.
	imageNoInterpreter
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
	if name, _, ok := interpreterWords(image); ok && name == "" &&
		r.ask(r.sem().EmptyInterpreterLineIsNotAScript,
			"a file whose `#!` line names no interpreter being refused rather than read as a shell script") {
		// A `#!` with nothing on it is the one ENOEXEC that is not the
		// fallback's: the file says it wants an interpreter and does not say
		// which. Asked after the binary check and not instead of it, because
		// the two are different questions about the same read and a file can
		// only fail one of them.
		return image, imageNoInterpreter
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
	case imageBinary, imageNoInterpreter:
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
	case imageBinary, imageNoInterpreter:
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		return r.execFailed(&pathError{name: argv[0], resolved: path, err: notAnImage}), true
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
	// The **child's** grammar, not the caller's. The child is the shell this
	// file is being run by, and its vector is the one SetUp just composed;
	// reading the file with the caller's left the one half of the vector that
	// is reached before execution — the grammar — still crossing from the
	// shell that started it. `shopt -s extglob` in the caller and `@(a)b` in
	// a shebang-less script is the shape: the pattern parsed, because the
	// caller had the flag, and then matched literally, because the child did
	// not. The diagnostic below already names the child for the same reason
	// (#4149).
	f, perr := syntax.Parse(src, child.dialect().On(syntax.RouteFromScriptFile))
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
