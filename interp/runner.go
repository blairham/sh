// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/blairham/sh/internal/histexpand"
	"github.com/blairham/sh/syntax"
)

// Runner executes a syntax tree.
//
// This is a first slice, and the honest boundary is stated rather than
// discovered: it runs simple commands, assignments, and-or lists, and file
// redirections. Pipelines, subshells, compound commands and builtins are not
// here yet. Everything it does not implement is refused with a message saying
// so, never silently skipped — a shell that quietly does nothing is worse than
// one that says it cannot.
type Runner struct {
	// Vars holds shell variables. A nil map is initialized on first use.
	Vars map[string]string
	// Arrays holds indexed array variables, which are a different kind of
	// thing from Vars rather than a formatting of one: an element can hold a
	// space without becoming two.
	Arrays map[string]Array
	// AssocArrays holds associative arrays — string keys, set apart from
	// Arrays because the two kinds read a subscript differently: an indexed
	// array evaluates it and an associative one takes it as written. A name
	// in this table *is* the `declare -A` attribute; declaring puts an empty
	// one here, which is how an assignment later knows which reading the
	// subscript gets.
	AssocArrays map[string]AssocArray
	// Params holds the positional parameters, $1 first. `$0` is not one of
	// them and is kept separate, because `shift` moves these and never
	// touches that.
	Params []string
	// Name is `$0`.
	Name string
	// exported names go into a command's environment; the rest do not.
	exported map[string]bool
	// Dir is the working directory. Empty means relative: paths stay as
	// written and the operating system resolves each use against wherever
	// the process happens to be, and `pwd` and $PWD report `.`. The Runner
	// never asks os.Getwd — the process has one answer and a program may
	// hold many Runners, so borrowing it here is how two embedded shells
	// come to share a cwd. A caller that wants absolute answers sets this;
	// driver does, at construction, because a shell binary is the one place
	// the process-wide question is the right one to ask.
	Dir string
	// Env is the environment: what commands inherit, and what `$name` falls
	// back to when no variable answers. Nil means empty, not the process's
	// own — the Runner never reads os.Environ, for the reason Dir never
	// falls back to os.Getwd: a script's view of the environment is whatever
	// its embedder handed in, and process state is everyone's. driver seeds
	// this from the process at construction, which is what makes the shell
	// binaries inherit normally.
	Env []string
	// Dialect is what nested input — a command substitution, an `eval` — is
	// parsed with. Nil means the core, not the zero value: the zero Dialect
	// is posix and would refuse constructs the outer parse had accepted.
	Dialect *syntax.Dialect

	// Diagnostics is how failure is reported and which status it carries.
	// Nil means the substrate's own.
	Diagnostics *Diagnostics
	// Semantics is where the shells disagree about what identical syntax
	// means, as distinct from which syntax they accept. Nil means bash's.
	Semantics *Semantics

	// Stdin, Stdout and Stderr are the shell's three streams, and nil means
	// *empty* — a reader with nothing in it and a writer that discards —
	// rather than the process's own.
	//
	// The same answer Env gives, for the same reason, and the reason is the
	// whole of this package's contract: a Runner is embedded in other
	// programs, and the process's streams belong to the program rather than
	// to any shell inside it. A borrowed stdout is worse than a borrowed
	// environment, because it is not read but *written*: an embedder that
	// wired two of the three would find a script's output interleaved into
	// its own terminal, with nothing to say where it came from.
	//
	// Silence is the safe failure here and noise is not, which is what makes
	// this the answer rather than a convenience. A shell binary wires all
	// three explicitly — driver does, at construction, exactly as it seeds
	// Env and Dir — so nothing that is a shell is affected by what nil means.
	//
	// Where a stream is handed to a child process, nil goes across as nil,
	// which os/exec spells /dev/null: the same emptiness, said the way a
	// process says it.
	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// AxisRemedy is what a caller wants said to a person who has just run
	// into an axis no dialect answered: the words that turn "the shells
	// disagree here and no dialect was chosen" from a statement of the
	// problem into a statement of the fix. Empty — the default — says
	// nothing extra, which is the honest answer for a Runner embedded in a
	// program that has no dialect flag to point at.
	//
	// Supplied rather than composed here, and for the reason this package
	// takes every other decision from its caller: the remedy is a sentence
	// about the *front end*, naming its flag and the shells it will accept,
	// and nothing under interp may name a shell — see the structure rule in
	// AGENTS.md. A remedy written here would also be wrong for every
	// embedder that is not this repository's own binary, which is most of
	// them. cmd/sh sets it, via driver.Shell.AxisRemedy; a dialect binary
	// leaves it empty, because a dialect that reaches an unanswered axis has
	// a gap in its vector rather than a flag its user forgot.
	//
	// It crosses into a subshell unchanged, like every other fact about the
	// invocation: the refusal reads the same wherever the script hit it.
	AxisRemedy string

	// Gate is consulted before every action that leaves this process. Nil
	// allows everything, so the zero value is usable.
	Gate Gate
	// Events receives what happened. Nil discards.
	Events Sink

	// Session identifies this shell to whatever is recording it, and is
	// carried on every Event this Runner emits.
	//
	// Supplied rather than invented: a run's identity is the front end's to
	// choose, because the front end is what has more than one record of the
	// run to line up — an audit stream and whatever it keeps of what it ran.
	// A Runner that generated its own would be a Runner whose identity nothing
	// else could know, which is the opposite of what an identity is for, and
	// it would put a source of randomness in a library that is meant to be
	// reproducible. Empty is a run with no identity, which is honest.
	//
	// A subshell shares it. A cloned Runner is the same session by every
	// meaning a consumer has for the word — one invocation, one policy, one
	// audit stream — so this crosses clone unchanged, and so does the numbering
	// behind Action.ID.
	Session string

	// ReplaceProcess makes `exec cmd` actually replace this process, and nil
	// — the default — makes it run the command as a child and then stop the
	// script with its status.
	//
	// It is opt-in because this package is a library. A real shell calls
	// execve and becomes the command; a Runner embedded in some other program
	// doing that would replace *that* program with whatever a script named,
	// which is not a shell feature but a way to lose a program. The core's
	// `cd` declines to call os.Chdir for the same reason and much smaller
	// stakes.
	//
	// A program that *is* a shell says so by setting this, and interp does
	// not provide the implementation: reaching for syscall.Exec is the
	// caller's decision to make, in the caller's own code, where it is
	// visible. cmd/sh and the dialect binaries set it via driver.
	//
	// It returns only on failure — a successful replacement does not come
	// back — and the error it returns is reported as the exec having failed.
	//
	// files is the descriptor table the replacement is to be given: entry i is
	// descriptor i, and a nil entry is a number that must not be open there.
	// It is a separate argument rather than something the hook can work out
	// for itself, because the numbers are the *script's* — a Runner's
	// descriptor 3 is some other number in the process, and which descriptors
	// are the script's to hand out is a question only this package can answer.
	//
	// It starts at 0 rather than at 3, which is where InheritedFiles is read
	// from and where os/exec's ExtraFiles begins. A child is handed its named
	// streams separately, by a package that will copy bytes through a pipe for
	// a stream that is not a file; a replacement is this process, so 0, 1 and
	// 2 have to say the right thing before the execve like every other number,
	// or `exec >log; exec cmd` writes to the terminal. Entries 0 to 2 are nil
	// wherever the stream is not a file, and the nil closes the number there
	// as it does anywhere else in the table.
	ReplaceProcess func(path string, argv, env []string, files []*os.File) error

	// DieBySignal ends this process with the signal a script sent it and had
	// no handler for, and nil — the default — stops the script with 128 plus
	// the number instead.
	//
	// Opt-in for the reason ReplaceProcess is. `kill -INT $$` in a real shell
	// kills the shell; the same line in a Runner embedded in some other
	// program would kill *that* program, and a library that can be talked
	// into killing its host by the text it was asked to interpret is not a
	// library. So interp decides *when* — after the script has stopped and
	// the EXIT trap has had its turn, which is the order a shell uses — and
	// the caller decides whether, in the caller's own code.
	//
	// It does not return on success. The status matters even so: a process
	// killed by a signal and one exiting with 128 plus the number are the
	// same number to `$?` and different to `wait`, which is a difference the
	// corpus records.
	DieBySignal func(sig syscall.Signal) error

	// GuardConcurrent runs work this shell put on a goroutine of its own — a
	// background job, either half of a pipeline, a coprocess, a process
	// substitution — and is where a panic in that work is dealt with.
	//
	// Opt-in for the reason ReplaceProcess and DieBySignal are, and for the
	// reason this package panics at all. interp panics when an invariant of
	// its own breaks, because a library that swallows a broken invariant
	// hands its caller a wrong answer where it could have handed back
	// nothing; a front end that has decided the process *is* a shell may
	// decide otherwise, and internal/panicguard is that decision. Nil leaves
	// the work bare, so a panic on it ends the process — which is the honest
	// answer and the one a library owes.
	//
	// `recover` reaches only the goroutine that panicked, which is why this
	// exists as a hook rather than as something a caller can wrap from
	// outside: the guard a front end already puts around a run is on the
	// calling goroutine, and every goroutine named above is out of its reach.
	//
	// It is called *on* the new goroutine, with the work as its argument;
	// starting the goroutine stays here. A hook that decided where the work
	// ran could run it on the calling one, and the shell waits for a
	// background job to report its process before it goes on — so a hook
	// meaning well would deadlock the shell it was protecting.
	//
	// The second argument is where a report about the panic goes: the
	// shell's own error stream, with the lock this package puts over a
	// caller's io.Writer already on it. Handed over rather than left to be
	// assumed, because a front end writing to the stream it supplied would
	// be a second writer with a lock of its own, and two locks over one
	// writer exclude nothing — which is the argument lockedWriter is built
	// on, arriving here from the other side.
	//
	// What it may not do is skip the work, and it is not asked to clean up
	// after it. Everything a goroutine of ours owes the rest of the shell —
	// a job's status, a pipe's end, an element's place in a pipeline — is
	// handed over once this returns, whether the work returned or panicked,
	// so what a recover here reports is a broken shell rather than a hung
	// one. That it happens *after* this returns is also the guarantee that a
	// report is written before whatever was waiting carries on.
	GuardConcurrent func(work func(), errs io.Writer)

	// SetUmask sets this process's file-creation mask and returns the one it
	// replaced. Nil — the default — means this shell has no umask to offer
	// and the builtin is refused.
	//
	// Opt-in for the reason ReplaceProcess and DieBySignal are. A umask is
	// *process* state: a Runner embedded in some other program would be
	// changing that program's mask for every file it writes afterwards, on
	// the say-so of the text it was asked to interpret. So interp decides
	// when, and the caller decides whether, in the caller's own code.
	//
	// One hook rather than a reader and a writer, because the system call is
	// one: it always sets, and returns what was there. Reading without
	// changing is setting the old value straight back, and the read that
	// learns this shell's starting mask is the same call that empties the
	// process's — which is what a caller is agreeing to here. Offering it
	// hands the mask over: the shell keeps it in a field of its own, masks
	// the modes it creates with by hand, and puts it back on the process
	// only for the length of a fork, so that a child inherits it. See
	// umaskscope.go, and #2949 for why a mask held in the process could not
	// be a body's own.
	SetUmask func(mask int) (old int, err error)

	// StopThisProcess stops this process the way `suspend` does: it does not
	// return until something sends SIGCONT. Nil — the default — means this
	// shell is not the process and cannot stop it, and the builtin refuses.
	//
	// Opt-in for the reason ReplaceProcess and DieBySignal are, and the
	// stakes are the same read a third time. A Runner is embedded in other
	// programs; a line of script that could stop the *host* process until
	// somebody found it and sent it a signal is a library that can be talked
	// into hanging its caller by the text it was asked to interpret. So
	// interp decides *when* — after the builtin has checked everything a
	// shell checks before stopping — and the caller decides *whether*, in
	// the caller's own code, where the system call is visible.
	//
	// It returns only on failure, and returning nil means the process was
	// stopped and has since been continued: `suspend` is 0 in a real shell
	// after a SIGCONT rather than at the moment of the stop, so a hook that
	// came back immediately would report a success the process never had.
	StopThisProcess func() error

	// WaitForCommand, when set, waits for a command this shell started and
	// reports how it ended — including that it *stopped* rather than
	// finished, which is what ^Z does and what an ordinary wait cannot say.
	//
	// Opt-in for the reason the other process hooks are, and for one more:
	// waiting is the caller's to own because reaping is. A Runner embedded in
	// a program that has its own child-handling must not race it, and a
	// library that called wait4 behind the embedder's back would.
	//
	// Nil means the ordinary wait, which cannot see a stopped command — so a
	// shell without this hook hangs on ^Z rather than returning to a prompt.
	WaitForCommand func(pid int) (Wait, error)

	// PollCommand is WaitForCommand without the waiting: it reports whether a
	// command this shell started has changed state since it was last asked,
	// and `changed` is false when it has not.
	//
	// It is what a job `bg` resumed is finished by. Nothing is blocked on such
	// a job — ^Z left it behind a wait that already returned — so the shell
	// has to ask, and it asks between commands, where it is the only waiter
	// and there is no race over who reaps the child.
	//
	// Nil means this shell cannot ask, and a resumed job then stays listed as
	// running until something waits for it by name.
	PollCommand func(pid int) (w Wait, changed bool, err error)

	// TakeInterrupt, when set, reports whether the person at the keyboard has
	// interrupted the shell since it was last asked, and forgets it.
	//
	// The front end's to answer because only the front end can hear it. interp
	// installs no signal handlers — a library that did would be taking them
	// from the program around it — so a ^C aimed at *this process*, which is
	// what one is whenever the shell is running something of its own rather
	// than an external command, is invisible here. Without it a loop of
	// builtins typed at a prompt cannot be stopped by anything short of
	// closing the terminal.
	//
	// Consulted at the top of every command, and only in an interactive
	// shell: what a *script* does with an interrupt is a different question,
	// answered by the signal itself rather than by somebody typing.
	TakeInterrupt func() bool

	// Foreground, when set, hands the terminal to a process group for as long
	// as it runs, and takes it back afterwards. A pgid of 0 means the shell
	// itself.
	//
	// It is what makes ^C and ^Z reach the command rather than the shell: the
	// kernel sends them to the terminal's *foreground* group, so a command in
	// a group of its own that was never given the terminal cannot be
	// interrupted or stopped at all. Setting WaitForCommand without this is
	// worse than setting neither — the command runs unreachable and the wait
	// has nothing to report.
	Foreground func(pgid int) error

	// SignalGroup, when set, sends a signal to a process group — the pid
	// given is the group's leader. Nil means this shell cannot resume a
	// stopped job, and `fg` and `bg` say so rather than pretending.
	//
	// A group rather than a process because a job is a pipeline as often as a
	// command: resuming only the first of three would leave the rest stopped.
	SignalGroup func(pgid int, sig syscall.Signal) error

	// GetRlimit and SetRlimit read and change this process's resource limits.
	// Nil — the default — means this shell has none to offer and `ulimit` is
	// refused.
	//
	// Opt-in for the reason SetUmask is, and more so: a limit outlives the
	// command that set it and binds everything the process does afterwards,
	// so a Runner embedded in another program must not be able to cap its
	// host's memory or its open files on the say-so of a script.
	//
	// Two hooks rather than one, because these are two system calls — unlike
	// the umask, which only ever swaps.
	GetRlimit func(res Resource) (soft, hard int64, err error)
	SetRlimit func(res Resource, soft, hard int64) error

	// HasRlimit reports whether this build's kernel has a given limit at
	// all. Nil — the default — means every resource this shell names, which
	// is what a test double wants and what the ten shared ones are.
	//
	// It exists because `ulimit -a` is a *table*, and a resource the kernel
	// does not have is not a row that reads zero: it is a row that is not
	// there. Measured across three shells on macOS, which has no
	// RLIMIT_NICE, RLIMIT_SIGPENDING, RLIMIT_MSGQUEUE, RLIMIT_RTPRIO or
	// RLIMIT_LOCKS — bash 5.3, zsh and dash each print their Linux table
	// without those rows, every surviving row byte-identical, including its
	// padding. So the row is dropped and nothing else moves.
	//
	// Separate from GetRlimit returning an error because the two mean
	// different things: "this kernel has no such limit" is a fact about the
	// build and is the table's business, where a failing getrlimit is a
	// fault and should still be reported.
	HasRlimit func(res Resource) bool

	// procSubs are the named pipes this command's process substitutions made,
	// waiting to be removed once it is done with them.
	procSubs []procSubPipe
	// enclosingProcSubs are the substitutions of the commands this one runs
	// inside — a function call's `f <(cmd)`, or the command a subshell,
	// pipeline element or command substitution was cloned from. Handed to a
	// child by number and never closed here: they belong to the command that
	// made them, and a command inside a function finishing is not that
	// command finishing. `f() { local p=$1; cat "$p"; }; f <(echo x)` is `x`
	// in bash 5.3.20 and was a bad file descriptor here, because the `local`
	// took the call's pipes with it on its way out.
	enclosingProcSubs []procSubPipe
	// heldProcSubs are the ones removeProcSubs could not finish with, because
	// one of this shell's *own* descriptors was still open on the pipe —
	// `exec > >(cmd)` is the shape. They keep until the shell itself ends,
	// which is the descriptor's real lifetime; see endHeldProcSubs.
	heldProcSubs []procSubPipe

	// bodies holds the writing substitutions' bodies no command waited for,
	// in the dialect that does not wait at the command that named one — see
	// Semantics.WritingSubstitutionIsWaitedForAtTheCommand. Their pipes and
	// names are already gone; what is left is a goroutine still writing into
	// a stream, and the join has to happen before whoever is reading that
	// stream stops reading it.
	//
	// **Shared with a clone rather than copied, and that is the measurement.**
	// A body writes into the stream it was given, and the question is which
	// scope owns that stream. Measured 2026-09-15 on bash 5.3.15 and ksh93,
	// with a body that outlives its input:
	//
	//	( printf P | tee >(read -r x; sleep .3; printf "[%s]" "$x") >/dev/null )
	//	printf AFTER
	//
	// writes `AFTER` and then `[PIPE]`, so a subshell and a pipeline element
	// are *not* the boundary: the bytes arrive after the whole script. The
	// same body inside a command substitution is a different answer — bash
	// captures it into the value, after everything the substitution's own
	// commands wrote — so that one *is* a boundary, and ownsBodies is what
	// says which runner is holding a list of its own.
	//
	// ksh93 loses those bytes instead of capturing them, which is a third
	// answer and not one worth reproducing: it is a dropped write.
	bodies      *pendingBodies
	ownsBodies  bool
	procSubHome *procSubDirs
	// pipeEnd is set on the runner that is a process substitution's *body*,
	// and is the end of that substitution's pipe the shell holds. It is
	// there so a job the body backgrounds can keep the pipe open past the
	// body's return, which is what a fork gives a real shell for nothing.
	// See substEnd.
	pipeEnd *substEnd
	// bodyAnchor is the process group a body a real shell would have *forked*
	// leads here: a subshell, a command substitution, a pipeline element, a
	// background job or a process substitution's body. Nil on the shell
	// itself, which has a process and needs no stand-in for one.
	//
	// One field for all five, deliberately. Each of them reconstructs the
	// body's lifetime differently — a subshell's is its own run, a job's is
	// the job's, a substitution's is the family's — and the *question* a
	// script asks inside them is the same one, so the answer is the same
	// mechanism with five different releases rather than five mechanisms.
	// See procanchor.go, and Runner.anchorForkedBody for where it is set.
	//
	// Copied by clone, which is what a body nested in another body wants
	// until it overwrites it: a `( … )` inside a `<(…)` whose own body never
	// asked still names the substitution's group, which contains it.
	bodyAnchor *procAnchor
	// substRan records that a command substitution reported a status during
	// the expansion just performed — see simple().
	substRan bool
	// expanded carries the right-hand side an assignment's caller has
	// already expanded, so assign() does not expand the same word a second
	// time — see Runner.assignValue.
	expanded *expandedAssign
	// streams holds one lock per stream the caller supplied, shared with
	// every subshell — see lockedWriter.
	streams *streamLocks

	// frames is the call stack: a function entered or a file sourced, and
	// nothing else. The script itself is not one of them — it is the floor
	// under them, which CallStack adds.
	frames []Frame
	// scriptFile is the file the shell was given, which only the front end
	// knows. Empty for `-c` and for a Runner nobody told.
	scriptFile string
	// lastErrno is the number the last system call this shell made on a
	// script's behalf failed with. See interp/errno.go.
	lastErrno int

	// funcOrigins is where each function's body came from. Two facts, in one
	// table because they are recorded at one moment and a route that wrote
	// half of one would be a route with no origin at all.
	funcOrigins map[string]funcOrigin
	// exportedFuncs are the functions written into a command's environment,
	// and funcExportPrefix/Suffix are what the entry is called. Only one
	// dialect carries functions that way, so the naming comes from it.
	// functionLayout and exportedFunctionLayout are how this shell arranges
	// a function it has to say back — see SetFunctionLayout.
	functionLayout         syntax.Layout
	exportedFunctionLayout syntax.Layout
	exportedFuncs          map[string]bool
	// readonlyFuncs are the functions a script has frozen, in the one dialect
	// with the notion — see Semantics.FunctionAttributeLetters. A name in here
	// cannot be redefined and cannot be unset, and both refusals are the
	// function table's rather than the variable table's: `readonly f` and
	// `readonly -f f` freeze two different things under one word.
	readonlyFuncs                      map[string]bool
	importedFuncs                      bool
	funcExportPrefix, funcExportSuffix string
	// undefinedFunctions is the dialect's answer to "has this function's body
	// been read yet, and what does a listing write where it has not" — see
	// SetUndefinedFunctions. Nil in a shell with no such thing, which is two
	// of the four.
	undefinedFunctions func(name string) (string, bool)
	// tracedFunctions is the dialect's answer to whether a function runs with
	// the trace on — see [Runner.SetTracedFunctions].
	tracedFunctions func(r *Runner, name string, keyword bool) bool
	// xtraceByMark is the trace a *function's own mark* turned on, which is
	// not the `set -x` option and does not behave like it: it lasts for one
	// body and does not reach a function that body calls. See
	// Runner.tracing, which is what every reader asks instead of reading the
	// option directly.
	xtraceByMark bool
	// loadUndefined is how the dialect reads the body of a name the shell is
	// still waiting for — see [Runner.SetUndefinedFunctionLoader].
	loadUndefined func(r *Runner, name string) bool
	// markUndefinedFunctions is the write half of the same seam: how this
	// shell marks a name to be defined later. Nil in a shell with no such
	// thing — see SetFunctionMarkedUndefined.
	markUndefinedFunctions func(r *Runner, names []string, letters string, remove bool) int
	// markedFunctions is the third of the same seam: which names hold the
	// marks a set of letters spells, which is what narrows a listing that
	// carried them and no operands. Nil in a shell with no such thing — see
	// SetMarkedFunctions.
	markedFunctions func(r *Runner, letters string) []string
	// reservedWords is the dialect's own answer to which words this shell
	// classes as reserved, for a shell whose table is not its parser's. Nil
	// in the four where the two are the same thing, and the grammar answers
	// then — see [Runner.SetReservedWords].
	reservedWords func(name string) bool

	// SetUp is what a front end does to a Runner beyond filling in its
	// exported fields — a dialect's own adjustment to the builtins, the ties
	// it installs, the prompt table — so that a shell *this* package has to
	// build for itself is built the same way.
	//
	// There is one such shell and it is not a subshell: the fresh one a file
	// the kernel would not start is run in, which must not inherit the
	// caller's functions or unexported variables and so cannot be a clone.
	// See noexecscript.go. Building it from the exported fields alone got
	// everything a field carries and nothing else, so a shebang-less script
	// found `$RANDOM`, `$SECONDS` and `$BASH_VERSION` empty and had none of
	// the builtins a dialect registers — a fresh shell in a sense no real
	// shell is fresh, since an execve of the same binary would have run all
	// of that on the way up.
	//
	// A field rather than a call, for the reason ReplaceProcess and
	// InheritedFiles are fields: composing a shell is the front end's job,
	// and interp may not reach for a dialect package — the import runs the
	// other way. Nil is a Runner nobody composed, which is every embedder
	// that builds one by hand and is the behavior this had before.
	SetUp func(*Runner)

	// InheritedFiles are the descriptors this shell was *started* with beyond
	// the three named streams — what `sh 3<&0 script` puts on 3 — laid out
	// the way childFiles hands them back: entry i is descriptor 3+i, and a
	// nil entry is a number nothing was inherited on.
	//
	// A fact the front end hands in, never one this package discovers, and
	// that is the whole of why it is a field. Finding them means reading the
	// *process's* open descriptors and asking which of them would survive an
	// exec, and a Runner embedded in another program would then publish that
	// program's own descriptors to a script it was asked to interpret — a
	// database handle or a listening socket reachable as `<&3` because the
	// embedder happened to hold one. So interp takes what it is given: driver
	// looks, because a binary that *is* the shell is the one place the
	// process-wide question is the right one to ask, and an embedder decides
	// for itself by filling this in or leaving it empty.
	//
	// The files are the shell's own. driver duplicates each inherited
	// descriptor rather than adopting it by number, so that closing one here
	// is closing a copy: a caller's descriptor is the caller's to close, and
	// a table entry that went out from under this process would leave every
	// later child built on a descriptor that is no longer there.
	InheritedFiles []*os.File

	// ProcessAnchor names a program that does nothing and exits when its
	// standard input reaches end of file. It is what lets a process
	// substitution's body have a **process group of its own** — see
	// procanchor.go for why a group is the answer to a question that looks
	// like it is about a process, and for which scripts ask it.
	//
	// A field for the same reason InheritedFiles above is one, and the same
	// split. Starting a copy of the shell is a fact about *this process* —
	// where its own program lives on disk — and a library may not reach for
	// it: a Runner embedded in some other program would be starting copies of
	// that program, which is not a placeholder but a second instance of
	// whatever the embedder is. So driver looks, because a binary that is the
	// shell is the one place the question is the right one to ask, and an
	// embedder decides by filling this in or leaving it empty.
	//
	// Empty is a shell whose substitution bodies have no group, which is what
	// every body had before this existed: the parameter that would report one
	// answers as it did, and nothing else changes.
	ProcessAnchor []string

	// JobControl says this shell reports its jobs to a person: it announces
	// one when it is backgrounded and says so when it ends.
	//
	// The front end's to set, and only for a prompt. A script is told
	// nothing by any shell in the panel — `sh -c 'sleep 1 &'` announces
	// nothing anywhere — and a Runner embedded in another program has no
	// person to tell.
	JobControl bool

	// Interactive says this shell is an interactive one, which `$-` reports
	// as `i` and which a script reads to tell a session from a batch run.
	//
	// The front end's to set, and it is a fact carried *in* rather than one
	// interp discovers: a library Runner has no standing to ask the process
	// whether anybody is watching, and the answer would be wrong anyway —
	// the embedder's standard input is not this shell's invocation. `-i`,
	// or nothing to run and a terminal, is the whole of the question and
	// `driver` is where it is asked.
	//
	// Not the same as JobControl, though a prompt sets both. This one says
	// what the shell *is*; that one says there is somebody to announce a job
	// to. Measured, ksh93 is the only shell in the panel that turns the
	// monitor on for `-i script.sh`, so the two do not move together.
	Interactive bool

	// AtPrompt says the text this runner is running was **typed at a
	// prompt**, which is a question about where the input came from rather
	// than about which shell is reading it — the third route beside a script
	// file and a program on standard input.
	//
	// It decides how a run-time diagnostic is located, through
	// Diagnostics.ForPrompt: measured 2026-09-11, no shell in the panel
	// writes a line number for one. `nosuchcmd` at a prompt is
	// `bash: nosuchcmd: command not found`, `zsh: command not found:
	// nosuchcmd` and `ksh: nosuchcmd: not found`, where the same failure in a
	// script names a line in all three. Every line typed at a prompt is line
	// 1, so a number there says nothing whatever it is (#2024, the run-time
	// half of #1892).
	//
	// The front end's to set, for the reason Interactive is: a library Runner
	// has no standing to ask the process where its program came from. Not the
	// same question as Interactive either — that one says what the shell *is*
	// and this one says what it is reading — and they part company inside a
	// **sourced file**, which is a file however it was reached. See
	// Runner.diag, which stops applying the prompt's wording there.
	AtPrompt bool

	// NotFoundHint is a second line to write after a bare name was not
	// found, and nothing where it returns the empty string or is nil.
	//
	// A seam and not a wording, because the only useful thing to say here is
	// a thing this package may not know. `whence` is a real builtin one
	// keystroke away in two of the presets, and to the core it is
	// indistinguishable from a typo — but `syntax` and `interp` may not
	// import a dialect or name a shell, which is what TestNothingHereIsAShell
	// enforces, so the core can never learn that. The binary that maps a name
	// to a preset can, and it is the only one that may: a binary claiming to
	// *be* bash must print what bash prints and nothing after it.
	//
	// Written after the diagnostic and never instead of it. The first line is
	// what a script parses, and it is byte-identical with or without this.
	NotFoundHint func(name string) string

	// UserHomeDir answers `~user`: the home directory of the user so named,
	// and false where the system has no such user.
	//
	// A hook rather than an `os/user` call down here, for the reason lookPath
	// reads this Runner's PATH rather than the process's — the core may not
	// ask the *process* a question the Runner should hold. A Runner with no
	// hook leaves `~user` exactly as it was written, which is the right
	// default for a library embedded in a program that has no business
	// reading a password file, and which is what this package did
	// unconditionally before (#2191).
	//
	// It answers for `~user` alone. A **named directory** — `hash -d
	// name=dir` and then `~name` — is a table this shell owns and is asked
	// first: measured on zsh 5.9.2, `hash -d root=/tmp; print -r -- ~root`
	// is `/tmp` where the same line without the assignment is `/var/root`.
	//
	// The binaries wire it to `os/user` in driver, where process-wide
	// questions belong. A test must not: `internal/testenv` exists because
	// a test that reads the real user database passes on a laptop and fails
	// on a runner with no such user.
	UserHomeDir func(name string) (string, bool)

	// Terminal says this shell has a terminal, which is the fact job control
	// turns on: the kernel hands SIGINT and SIGTSTP to whatever process group
	// owns one, so a shell with none has nothing to hand a job and nothing to
	// take back.
	//
	// The front end's to set, and carried *in* for the reason Interactive is:
	// a library Runner has no standing to ask the process what descriptors it
	// was handed, and the embedder's terminal is not this shell's.
	//
	// Not the same question as JobControl, and the difference is measured
	// rather than tidy. That one says there is a *person* to announce a job
	// to, which is a prompt and nothing else; this one is true of any route
	// started from a terminal, and it is the one `set -m` turns on: on a
	// pseudo-terminal, `set -m` inside plain `zsh -c` is granted and puts `m`
	// in `$-`, and the same string on a pipe is `can't change option: -m` at
	// 1. All five of the panel grant it with a terminal. Asking JobControl
	// there answered the person question with the terminal's name and refused
	// every script that had one (#1720).
	Terminal bool

	// LoginShell says this shell was started as a login shell — a dashed
	// `argv[0]`, which is what `login` and a terminal emulator's "run as a
	// login shell" does, or an explicit `-l` / `--login`.
	//
	// The front end's to set, and carried *in* for exactly the reason
	// Interactive is: which word started the process and what it looked
	// like is an invocation fact, and a library Runner reached through Run
	// never saw an argument vector. `driver` decides it — see
	// driver.LoginShell and source.loginShell, which take either route —
	// and hands it over here.
	//
	// What reads it is `$-`, through Semantics.LoginShowsLInDollarDash,
	// where the panel splits 4-2. Nothing else in this package does: which
	// startup files a login shell reads is `driver`'s, and it has the fact
	// first-hand there (#1034).
	LoginShell bool

	// StartupFilesSuppressed says the invocation asked this shell to read
	// none of its startup files — the escape hatch a person reaches for when
	// the file that breaks the shell is the one it reads to start.
	//
	// The front end's to set, for the reason LoginShell above is: which
	// options an argument vector carried is an invocation fact, and a
	// library Runner reached through Run never saw one. `driver` decides it
	// from Semantics.StartupFileOptions.SuppressAll — zsh's `-f` and
	// `--no-rcs` — and hands it over here.
	//
	// It is what the invocation *asked for* rather than a tally of the files
	// that were read, which is measured: `zsh -c cmd` reads `.zshenv` and
	// still answers `rcs` on, and `zsh -f -c cmd` reads nothing and answers
	// it off. Nothing in this package reads it; the one shell whose option
	// namespace publishes the fact is zsh, whose `rcs` is the name for it,
	// and which startup files are actually read is `driver`'s own question
	// with the fact first-hand there (#1864).
	StartupFilesSuppressed bool

	// withdrawnParams holds what a dialect's module selection took out of the
	// parameter tables, keyed by name, so the name reads as an ordinary unset
	// one until the selection puts it back. See withdrawnparameter.go.
	withdrawnParams map[string]withdrawnParameter

	// Dynamic holds parameters whose value is produced when they are read,
	// rather than stored: `LINENO` is wherever execution has reached, and
	// `RANDOM` is a different number every time. A dialect fills in the ones
	// it has through SetDynamic.
	Dynamic map[string]func(*Runner) string

	// DynamicArrays is the same for arrays, and the call stack is why it
	// exists: `BASH_SOURCE` and its relatives cannot be stored and stay
	// right. A dialect fills in the names it has through SetDynamicArray.
	DynamicArrays map[string]func(*Runner) []string

	// DynamicAssocs is the same again for *keyed* tables, and it is the one
	// of the three that could not be faked with either of the others: a
	// parameter that answers "which functions exist, and what is each one's
	// body" is a name-to-value map read at the moment it is asked, and an
	// indexed array of pairs is not the same thing to `${m[key]}`. A dialect
	// fills in the names it has through SetDynamicAssoc.
	DynamicAssocs map[string]func(*Runner) AssocArray

	// dynamicAssocElements is the *one-key* reading of a produced
	// association, where DynamicAssocs is the whole-table one. Both are
	// answers to the same question and the pair exists because producing
	// every key to hand back one is the difference between a lookup and a
	// walk: `$functions` holds a body per name and rendering all of them to
	// answer `${functions[precmd]}` was measured at thirty milliseconds a
	// read. Unexported for the reason dynamicAssocWriters is — nothing reads
	// this table back — and optional, since a producer cheap enough to run
	// whole needs no second entry point. See SetDynamicAssocElement.
	dynamicAssocElements map[string]func(*Runner, string) (string, bool)

	// dynamicArrayWriters is what an assignment to a produced *array* does.
	// Unexported for the reason dynamicAssocWriters is — nothing reads this
	// table back — and the same failure is what makes it necessary: the
	// producer answers ahead of the stored array, so a write with nowhere to
	// go is accepted in silence and read back as whatever the producer says.
	// See SetDynamicArrayWriter.
	dynamicArrayWriters map[string]func(*Runner, []string)

	// dynamicAssocWriters is what an assignment to one element of a produced
	// association does. Unexported because it is not a table anything reads
	// back — see SetDynamicAssocWriter for why a produced association a
	// script can write to must have one.
	dynamicAssocWriters map[string]func(*Runner, string, string, bool)

	// dynamicAssocEmptied names the produced associations whose table a
	// *replacing* whole-table assignment empties before the literal's keys go
	// in. The names not in it merge, which is what most of them measure as.
	// See SetDynamicAssocEmptiedByReplacement.
	dynamicAssocEmptied map[string]bool

	// dynamicWriters is the same for a produced *scalar*, and it exists for
	// the same reason: a parameter a script both reads and writes cannot have
	// its writes land in the stored table, because the producer answers ahead
	// of that table and the write would go somewhere nothing reads. See
	// SetDynamicWriter.
	dynamicWriters map[string]func(*Runner, string)

	// assigned holds what a script assigned to a *produced* parameter, which
	// is a message to whatever produces it rather than a value of its own.
	assigned map[string]string

	// randomSeed and randomDrawn are the seeded half of Runner.Randoms: the
	// number a script last assigned to `RANDOM`, and how many have been drawn
	// since. Zero values are an unseeded shell, which draws from the process
	// generator and is not reproducible — the state a shell is in until a
	// script asks for otherwise.
	//
	// Two plain integers rather than a generator, and that is what makes a
	// subshell right: clone copies the Runner by value, so a `( )` carries on
	// from where its parent had got to and its own draws leave the parent
	// where it was. A shared generator would have the subshell's reads
	// advancing the sequence a later read in the parent gets.
	randomSeed   uint64
	randomDrawn  uint64
	randomSeeded bool

	// dynamicDeclarations is how each produced parameter lists back, as the
	// dialect that registered it states it — see SetDynamicDeclaration.
	dynamicDeclarations map[string]ProducedDeclaration

	// listingDrawsNoReading is set while a whole-shell listing is building a
	// produced parameter's row under an answer that writes no drawn value —
	// see listedDeclarationOf. Not cloned: it is live only inside one
	// builtin's own call.
	listingDrawsNoReading bool

	// producedReading is the value a produced parameter last gave a *script*,
	// kept for the one listing form that writes the reading rather than
	// taking a new one — see ProducedListingLastReading.
	//
	// Written where an expansion reads the producer and nowhere else, which
	// is the whole of what makes it the right thing to write: a listing that
	// filled it would be recording its own output, and two listings a line
	// apart would then differ from each other in the shell whose listing is
	// supposed to be the thing that cannot.
	producedReading map[string]string

	// endedProducers are the produced parameters an `unset` and an assignment
	// have between them turned into ordinary names — see
	// Semantics.AssignmentRestoresAnUnsetProducedParameter and
	// producerEndedByUnset.
	//
	// A record rather than only a deletion from Dynamic, because the shell's
	// own parameters are registered lazily and *repeatedly*: ensureSpecials
	// runs at the head of every chunk a front end hands to RunPart, and this
	// parser reads a script incrementally, so a `LINENO` deleted by one line
	// was back before the next one ran. Deleting the producer alone was
	// measured doing exactly that — the RANDOM row moved and the LINENO row
	// did not, which is what a second probe is for (#2450).
	endedProducers map[string]bool

	// absentParams are the parameters a dialect's module *names* and this
	// shell has not got, to the sentence a read of one is refused with.
	//
	// The fourth of the produced-parameter seams and the only one that
	// produces nothing: the three above answer a read, and this one refuses
	// it by name so that an absent parameter cannot read as empty. A
	// dialect fills it in through SetAbsentParameter — see absentparam.go.
	absentParams map[string]string

	// absentElements are the produced associations that answer only the keys
	// their producer holds, to the sentence a read of any other key is
	// refused with.
	//
	// absentParams one level down, and needed for the same reason: a table
	// that answers a handful of the keys its name is asked for has no way to
	// say so through a value, because the value it would give — the empty
	// string — is also the true answer for a key the thing being viewed
	// genuinely has nothing under. A dialect fills it in through
	// SetAbsentElements; see absentparam.go.
	absentElements map[string]string

	// started is when this runner was made, which is what `SECONDS` counts
	// from in the dialects that have it.
	started time.Time

	// Clock is what this shell calls now. Nil is the wall clock.
	//
	// A hook rather than a call to time.Now() at each site, for the reason
	// RANDOM has one: a value the shell *produces* has to be sayable from
	// outside, or nothing that depends on it can be tested twice with the
	// same answer. `printf '%(%Y)T' -1` is the first thing that needed it,
	// and a corpus case cannot ask it — which is why the case pins a fixed
	// epoch and this hook pins the rest.
	//
	// It is not the process-wide state the library rule is about: reading a
	// clock changes nothing and two Runners cannot fight over it. What it is
	// about is a Runner answering from ambient state that its embedder
	// cannot see or set.
	Clock func() time.Time

	// optChar is how far into a clustered option `getopts` has read — `-ab`
	// is two options in one word, and OPTIND cannot say which of them is
	// next because it counts words. lastOptind is what this builtin last set
	// OPTIND to, so a script moving it itself is noticed and the position
	// inside the cluster dropped.
	optChar int
	// optWord is the word the scan is at, where that has parted company with
	// OPTIND: a positive value overrides the parameter, and 0 says the
	// parameter is the position.
	//
	// The two part only across a shell function call, and only in the dialect
	// that gives a call a scan of its own while leaving OPTIND the shell's —
	// see Semantics.GetoptsFunctionPosition, where dash and BusyBox ash
	// re-read a helper's arguments on a second call and every `$OPTIND` a
	// script reads is still the shell's number. Nothing else ever sets it,
	// and a shell that never calls a function never has one.
	optWord int
	// optindAssigned records that a script wrote OPTIND since the last
	// `getopts` wrote it. The value cannot say so on its own: resetting it to
	// 1 while a cluster is half read is an assignment of the value it already
	// had, and three of the four restart the word on the strength of the
	// assignment rather than the number.
	optindAssigned bool
	// optRanOut records that the `getopts` write in flight is the name
	// operand on the run that reports "no more options", which is the one
	// write whose freeze a dialect answers apart from every other write the
	// builtin makes — see Semantics.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal.
	//
	// Set around that one call rather than for the builtin, because the
	// question is which path inside `getopts` reached the freeze and nothing
	// in the value being written says so.
	optRanOut bool

	// removed are names `unset` took away that came from the environment
	// rather than from Vars.
	//
	// Nothing clears an entry, and nothing needs to: a later assignment puts
	// the name in Vars, which every lookup reads first, and exports append
	// after the environment is filtered. A `delete` here looked right and no
	// test could tell whether it was there.
	//
	// Deleting from Vars cannot hide those: the environment is a second
	// source and getVar reads both, so `unset PATH` left PATH exactly where
	// it was and every lookup still found it. A name here is gone until
	// something assigns it again.
	removed map[string]bool

	// declaredEmpty are names a *declaration* gave a value to, in the
	// dialect that considers a name declared without one to be set. The
	// shell reads such a name as empty and no child is told about it, which
	// is a distinction only a real child can see: `local -x FOO` and `local
	// -x FOO=` list identically in the shell that makes them differ, and
	// hand a command nothing and an empty entry respectively.
	//
	// Cleared by an assignment, and by a scope that shadowed the name going
	// away — the second measured rather than tidy: a function that declares
	// a local of that name leaves the caller's an *empty* export where it
	// had been told to no child at all, in the one shell that reads a
	// declaration this way. `unset` needs no third place and deliberately
	// has none: it takes the name out of Vars, and both roads back in go
	// through one of those two.
	declaredEmpty map[string]bool

	// declaredOnlyCompound are names whose array or table came from a
	// declaration's letters alone, with nothing yet written to them. See
	// compounddeclaredonly.go for the measurement and for why the set kept is
	// this one rather than its complement.
	declaredOnlyCompound map[string]bool

	// declaredBare are names a declaration brought into being with no
	// letters, no value and nothing in them — declared and unset at once.
	// See baredeclaration.go for the measurement, and for why the record is
	// carried by nameAttributes rather than saved and restored again here.
	declaredBare map[string]bool

	// compoundVariable is the set of names that are ksh93 compound
	// variables — `c=(a=1 b=2)` and `typeset -C c`, the fourth kind of thing
	// a name can be beside a scalar, an indexed array and a table.
	//
	// A set rather than a table of members, and that is the load-bearing
	// choice: a compound's members are *ordinary names spelled with a dot*,
	// stored in Vars, Arrays and AssocArrays like any other, so `${c.a}` is
	// the lookup it looks like and a member keeps its own attributes for
	// free. Measured, and it is the shell's own model rather than a
	// convenience — `a=1; a.b=2` is accepted there and `${!a.@}` answers
	// `a.b`, so a name has children whether or not it is a compound, and what
	// `typeset -C` adds is how the *parent* reads and lists.
	//
	// A second table holding the members would be that rule with two homes:
	// every write through `c.a=9`, `unset c.a`, `typeset -i c.n` and `read
	// c.x` would have to keep it in step, and the one that forgot would make
	// a member that reads back and does not list. See compoundvariable.go.
	compoundVariable map[string]bool

	// aliases is the table `alias` and `unalias` keep. Substitution happens
	// when a line is parsed, which is the other half of the feature and lives
	// in the parser rather than here; the two meet at [Runner.ExpandingAlias].
	//
	// Regular and *global* aliases share it, because the shell that has the
	// second kind shares one table for them: `alias -g dup=…` replaces a
	// regular `dup` rather than standing beside it, and the plain listing
	// shows both. See aliasDef.
	aliases map[string]aliasDef

	// suffixAliases is the second namespace, keyed on a command word's
	// extension rather than on the whole word — `alias -s txt=cat` makes
	// `./x.txt` run `cat ./x.txt`.
	//
	// A table of its own rather than a third flag on the one above, because
	// the shell it models keeps them apart everywhere it can be seen: the
	// two sets are never listed together, `unalias -a` empties one and
	// leaves the other, and a name may be in both at once.
	suffixAliases map[string]string

	// namedAliases is every name `alias` has named, defined or not, in a
	// dialect that remembers them — see
	// [Semantics.AliasRemembersTheNamesItNames], which is the only thing
	// that reads this.
	//
	// Not a third state inside [aliasDef], because a remembered name is not
	// a degenerate alias: `alias e1=` defines one with an *empty value*,
	// which lists as `e1=''` and is found by a lookup, and a remembered name
	// is in no listing and is found by nothing. A flag on the entry would
	// have every reader of the table — the two listings, the lookup, the
	// parser's expansion hook, `whence`, `type`, `command -v` — remember to
	// skip it, and the one that forgot would list an alias that stands for
	// nothing. A set beside the table is read by `unalias` alone.
	namedAliases map[string]bool

	// cmdHash is the command hash: a bare name PATH resolved, and where it
	// resolved to. cmdHashOrder is the same names in the order they were
	// first put there, which is the order a listing walks unless the dialect
	// sorts — see hashedCommandNames for why insertion order is the one we
	// can defend.
	//
	// A table of the shell's rather than a cache of the process's, for the
	// reason lookPath reads this Runner's PATH rather than the process's: two
	// Runners in one program must not share one. A subshell gets a copy, which
	// is measured — `(ls >/dev/null); hash` leaves the parent's table empty in
	// bash, zsh and dash. ksh93 alone carries it back out, which its virtual
	// subshells explain and which no field here claims yet.
	cmdHash      map[string]hashedCommand
	cmdHashOrder []string

	// namedDirs is the table behind `~name` — a directory this shell was
	// *told* about, written by `hash -d` in the one dialect that has the
	// construct and read by tildeSplit. See interp/nameddir.go, including
	// why it is not the user database and why it wins over one.
	namedDirs map[string]string

	// checksHashedCommand is `shopt -s checkhash`: look at the remembered
	// path before running it, and fall back to a fresh PATH search when it
	// has gone. It is the one dialect switch over
	// Semantics.CommandHashIsTrusted, and it only ever *lowers* trust — in
	// the three shells that never trusted the entry it changes nothing,
	// which is why there is one field and not an option per dialect.
	checksHashedCommand bool

	// aliasExpansion is whether a word being parsed *right now* is replaced
	// by what the table holds for it, and aliasExpansionBase is the answer
	// this shell started with.
	//
	// Two fields rather than one because the pair is what a mode needs. The
	// dialect decides the base — syntax.Dialect.AliasesExpandUnlessTold,
	// which the front end reads and hands here — and two things move the live
	// one off it: `shopt -s expand_aliases`
	// in the one dialect with the name, and POSIX mode, which turns it on for
	// as long as the mode lasts. Measured, and the reason leaving the mode
	// restores the *base* rather than what was set before entering it: `shopt
	// -s expand_aliases; set -o posix; set +o posix; shopt expand_aliases`
	// answers `off` in bash 5.3.
	//
	// **This is the option and nothing else** — not the option modulated by
	// the route the program arrived by, which is what it was until #2109.
	// Measured 2026-09-12: the route governs the shell's own program text and
	// nothing nested inside it, so an `eval`, a command substitution, a
	// sourced file and a trap body under a zsh `-c` string all expand while
	// the string itself does not. A runner that had been handed the route's
	// answer turned the table off for all four.
	//
	// Plain bools, so a subshell clone carries its own copy: `(shopt -s
	// expand_aliases)` is the subshell's business, the same as every other
	// option here.
	aliasExpansion     bool
	aliasExpansionBase bool

	// arithPrecedence is the order the binary arithmetic operators bind in,
	// where a dialect's option has moved it away from the one this shell
	// parses its own program text with. arithPrecedenceMoved is whether
	// anything has moved it at all, which cannot be read off the value:
	// the zero value is a real answer — C's order — and not an absence.
	//
	// It is here rather than only on syntax.Dialect because the option that
	// moves it is a *run-time* one. Measured 2026-09-15 on zsh 5.9.2:
	// `setopt c_precedences` changes what `$(( 1 << 2 + 1 ))` answers on a
	// line already read, and changes it inside a function whose body was
	// parsed before the option was touched. So the expression's tree is not
	// fixed at the time the file is read, and this is what tells the
	// evaluator to build it again — see Runner.arithTreeOver.
	//
	// Plain values, so a subshell clone carries its own copy, the same as
	// every other option here.
	arithPrecedence      syntax.ArithPrecedencePolicy
	arithPrecedenceMoved bool

	// killedBy is the signal this shell sent itself and had no handler for,
	// with the number kept beside it so the death does not have to look the
	// name up again.
	killedBy    string
	killedBySig syscall.Signal

	// stoppedBySignal records that an untrapped signal ended the script,
	// which is a wider fact than killedBy and deliberately a separate one:
	// where HangupIsAnOrderlyExit is taken, the script stops and there is no
	// death to re-raise afterwards. Anything asking "is this shell still
	// running its script" wants this; only the re-raise wants killedBy.
	stoppedBySignal bool

	// diedOfSig is the signal that ended the command whose status `status`
	// now holds, and zero where that status came from an ordinary exit.
	//
	// It is narrower than killedBy, which is only about a signal *this shell*
	// took and has to re-raise; an external command killed by SIGTERM sets
	// this and not that. It exists because a status is a lossy record of a
	// signal death — 143 could as easily be `exit 143` — and one shell
	// reports the signal itself where a pipeline substitutes the status.
	//
	// Cleared at the top of every command, so it can only ever describe the
	// command the status describes.
	diedOfSig syscall.Signal

	// status is the exit status of the last command run.
	status int
	// ctl carries break, continue and return out of a construct. They are
	// control flow rather than errors, so they are not returned as ones.
	ctl      control
	ctlDepth int
	// abandon says whether the controlExit being carried is an error the
	// shell reported or a request to stop, for the boundaries that give up
	// one file and catch only the first. See fileabandon.go.
	abandon abandonKind
	// errexitStopped says the controlExit being carried is `set -e` firing
	// rather than anything a script asked for, which is a question only a
	// try-always block asks — see Runner.transferEndsTheShell.
	//
	// A field of its own rather than a fourth abandonKind, because the two
	// are different questions: abandonKind is whether the stop was an *error
	// the shell reported*, and both of these are stops it was asked to make.
	// A fourth value there would have changed what fileabandon.go and
	// source.go do with it, which is a boundary neither of these crosses
	// differently (#1238).
	//
	// It qualifies the controlExit in hand and nothing more, so it is
	// cleared wherever one is raised for another reason, and saved and put
	// back with the rest of the transfer where a try-always block sets that
	// aside. The zero value is the safe one: an ordinary exit runs the
	// cleanup halves it unwinds through, which is what a site that has not
	// thought about it should get.
	errexitStopped bool
	// loopDepth is how many loops execution is inside right now, which is
	// what a ^Z has to break out of — see breakLoopsForAStop. Dynamic rather
	// than lexical: a loop that calls a function that loops is two, because
	// what the stop is inside is what matters.
	loopDepth int
	// callLoopFloor and subshellLoopFloor are the loop depths at the
	// innermost function call and the innermost `( )`, which is how far a
	// `break` written inside one can still see if that boundary stops it.
	//
	// Recorded on the way in and asked about at the `break`, because whether
	// either is a boundary is the dialect's and the answer is only visible
	// when the word is used — see Runner.loopControlReach. Kept apart
	// because the panel does not group them: one column makes both a
	// boundary and another makes neither.
	callLoopFloor     int
	subshellLoopFloor int
	// ctx is the context of the current Run, so expansion can reach it. A
	// command substitution runs commands, and threading a context through
	// every expander signature to reach one place would be worse.
	ctx context.Context
	// inBuiltin is the builtin currently speaking, for the one dialect that
	// names it in a diagnostic's location. Empty at every other moment, and
	// deliberately cleared by `.` and `eval` while they run borrowed text:
	// what a sourced script reports is the script's, not the builtin's.
	inBuiltin string
	// dotCurrentDirectoryFirst makes the next `.` resolve a bare operand
	// against the current directory before PATH. Set by
	// DotLooksInCurrentDirectoryFirst for the length of one call, and taken
	// by that call rather than left standing — see source.go.
	dotCurrentDirectoryFirst bool
	// speaker is the prelude function the script called, whose name and
	// whose caller's line every diagnostic raised inside it carries. Empty
	// while a script's own text runs, which is every other moment. See
	// prelude.go for the rule and for why the *outermost* such call owns it.
	speaker string
	// speakerLine is where the script called that function — a builtin has
	// no lines of its own, so the location names the call rather than the
	// body.
	speakerLine int
	// sourcingPrelude is on while the dialect's own shell text is being read,
	// which is what tells a function it defines from one a script does.
	sourcingPrelude bool
	// preludeFuncs are the declarations the prelude made, by name. Compared
	// by declaration rather than by name alone, so a script redefining one
	// takes the shell's voice away with it.
	preludeFuncs map[string]*syntax.FuncDecl
	// withdrawnBuiltins are the names a dialect's module selection has taken
	// out of the table — see withdrawnbuiltin.go. Separate from
	// disabledBuiltins because the two are not the same state: a disabled
	// builtin is still listed and can be enabled again, and a withdrawn one
	// is not there at all.
	withdrawnBuiltins map[string]bool
	// disabledBuiltins are the names `enable -n` has switched off. Kept
	// apart from custom so that switching one on again gets back whatever
	// was registered rather than the core's.
	disabledBuiltins map[string]bool
	// shellStdin is the standard input the shell had before this command's
	// redirections were applied, where that differs from Stdin. Nil is the
	// common case and means the two are the same stream.
	//
	// It exists for one reader: a process substitution's body, which one
	// dialect hands the shell's own input rather than the input of the
	// command the word stands in. The only place the two part is a pipeline
	// element, whose input is the pipe — and a pipe is a redirection of that
	// element, applied after its words are expanded. So runPipeline records
	// what the pipe replaced here, `command` carries it across exactly one
	// simple command, and applyRedirs clears it because that is the moment
	// the pipe is, in the reading this implements, installed.
	//
	// See Semantics.ProcessSubstitutionBodyReadsTheShellsInput, which is
	// where the panel split is written down, and procSub, which reads it.
	shellStdin io.Reader
	// midPipeline says this runner is an element of a pipeline whose status
	// it does not decide — everything but the last. Kept because a signal
	// that ends such an element is announced by one dialect and passed over
	// by the rest.
	midPipeline bool
	// timedPipeline is set by a `time` clause whose layout reports per
	// pipeline element, and consumed by the next pipeline dispatched — the
	// clause's own body and nothing nested inside it.
	timedPipeline *pipelineTiming
	// elemCPU is where this runner's external commands add the CPU their
	// processes used, while a per-element `time` report is collecting. Nil
	// almost always; a clone inherits it, so a subshell inside a timed
	// element still bills that element.
	elemCPU *cpuAccum
	// Route is where the program came from: a command string, a script
	// file, or standard input.
	//
	// Set by the front end, because that is what reads the invocation — the
	// same fact Interactive is, and carried in for the same reason. Three
	// things read it and each reads a different pair of the three: one
	// dialect answers a failed expansion with a different status under `-c`
	// than from a file, one answers a readonly reassignment differently the
	// same way, and `$-` shows `c` and `s` for two of the routes in some
	// shells and not others.
	//
	// One field rather than a bool per route, because they are answers to
	// one question and a second name for it is the one that would drift.
	Route Route

	// Invocation is the name the shell's own process was started under —
	// argv[0], not `$0`.
	//
	// The two are different facts and only one of them is here already:
	// Name is `$0`, which the front end may take from an operand, while
	// this is the word the process was executed as. Measured on both the
	// `-c` and the script route, the startup `$_` follows *this* one, so a
	// shell handed `-c 'echo "$_"' zeroname` reports the binary and not
	// `zeroname`, and the same binary reached through a symlink named `sh`
	// reports `sh` rather than where the symlink points.
	//
	// Carried in rather than read here for the reason Route and Interactive
	// are: os.Args answers for the process, a program may hold several
	// Runners, and a library that reaches past what it was handed cannot be
	// embedded. Empty is the honest answer for an embedded Runner that was
	// never told, and the shells that write it at startup then write
	// nothing.
	Invocation string

	// StandardInputOption says the invocation wrote `-s`.
	//
	// Nearly the same fact as Route being RouteStandardInput, and separate
	// for the one invocation where it is not: `sh -s -c cmd` runs the
	// command string in all four shells and still puts `s` in `$-` in all
	// four. So the letter follows either the route or the spelling, and the
	// spelling has to survive a route that overrode it.
	StandardInputOption bool

	// inFunc is the name of the function being run, for `$0`.
	inFunc string
	// locatesFunctions is whether a names-only function listing says where
	// each function was defined — see Runner.LocatesFunctions.
	locatesFunctions bool

	// recordsCallArgs and callArgs are the arguments of each call the shell
	// is inside, kept only while something asks for them. See
	// interp/callarguments.go.
	recordsCallArgs bool
	callArgs        []callArgs
	// debugActionDecides is whether a DEBUG action's *status* decides what
	// runs next — see Runner.DebugActionDecides.
	debugActionDecides bool
	// debugSkip is the answer one firing left behind: the command that
	// firing preceded must not run. Cleared where it is read, so an earlier
	// command's answer cannot skip a later one.
	debugSkip bool
	// funcLine is the line the function being run was written on, which one
	// dialect counts a message's line from instead of from the top of the
	// file.
	funcLine int
	// outsideCall is how many innermost frames the location has stepped out
	// of, which is nonzero only while [Runner.LocatedAtTheCall] runs. A count
	// rather than a flag because a nested use must put back what it found.
	outsideCall int
	// sourceDepth is how many sourced files are running, which is the other
	// place a `return` has something to return from. A count rather than a
	// flag because a sourced file may source another.
	sourceDepth int

	// currentShellSubstDepth is how many `${ …; }` bodies are running. Their
	// body is a frame a `return` leaves, which is what keeps `v=${ echo hi;
	// return 42; }` from being a `return` with nothing to return from. See
	// Runner.hasSomethingToReturnFrom and Runner.currentShellSubst.
	currentShellSubstDepth int
	// expandErr records that an expansion failed — a division by zero, a
	// number that is not one. The command does not run, which is what every
	// shell in the panel does and what the exit status has to say.
	expandErr bool
	// badSubscript records that the expansion which failed was a **subscript**
	// that would not evaluate, rather than any other unreadable expression.
	//
	// A second flag beside expandErr rather than a reading of it, because the
	// give-up they lead to differs by exactly this much: a bracketed
	// expression gives a `-c` string up whole and the same arithmetic outside
	// brackets resumes at the next command. Measured at seven bracketed sites
	// and four bare ones — see Runner.giveUpForABadSubscript, which is the
	// door this routes to (#3502). Cleared with expandErr, everywhere
	// expandErr is cleared.
	badSubscript bool

	// declarationSpeaker is the builtin whose operand a declaration's element
	// store is running for, kept while the store has that builtin's name out
	// of the location. The one complaint in that region which is the
	// *language's* rather than the store's still names it in one column —
	// see Runner.badSubscriptToADeclaration.
	declarationSpeaker string
	// prefixCheckedFirst records that this command's assignment prefix has
	// already been checked for a frozen name, ahead of its values and its
	// redirections — the order Semantics.PrefixToAFrozenNameIsCheckedFirst
	// answers. Set for one command and cleared when it is over, so the
	// dispatch routes skip the value of a frozen name and the ordinary check
	// does not report the same names a second time.
	prefixCheckedFirst bool
	// prefixTraceAssigns and prefixTraceValues hold this command's
	// assignment-prefix values, for the one command being traced. `set -x`
	// has to write the value it is about to hand over and the route that
	// applies it has to hand over the same one, and the expansion may run a
	// command substitution — so it happens once and both read it. Empty for
	// every command not under `set -x`, which is what keeps this from moving
	// the expansion of an untraced prefix. See
	// Runner.expandPrefixTraceValues.
	//
	// Two parallel slices rather than a map keyed by the assignment, which
	// is what this was: a prefix is one, two or three assignments, so the
	// scan is shorter than hashing a pointer — and a map here is a *table*
	// on the Runner, which every clone then has to own or be excused from
	// sharing. TestACloneOwnsEveryTable says so, and it is right to: a
	// per-command scratch value is not a table and should not have to
	// pretend to be one.
	prefixTraceAssigns []*syntax.Assign
	prefixTraceValues  []string
	// prefixHeldNames are the names the running builtin's own assignment
	// prefix is holding, and prefixKeptNames those of them a declaration
	// inside it has taken for the shell rather than for the command — see
	// Runner.keepThePrefixEntry and Semantics.DeclarationPromotesThePrefixEntry.
	//
	// Slices and not maps, for the reason the two above are: a prefix is one,
	// two or three assignments, so the scan is shorter than hashing, and a
	// map here would be a *table* on the Runner that every clone has to own
	// or be excused from sharing. Both are per-command scratch, saved and put
	// back around the builtin because a builtin can run another one.
	prefixHeldNames []string
	prefixKeptNames []string
	// prefixShadowed are the names a declaration inside the running builtin
	// has taken a *fresh* scope for while a prefix was holding them, so the
	// entry has moved into the cell the declaration made and the outer name
	// is no longer this command's to keep. See Runner.prefixEntryShadowed.
	prefixShadowed []string
	// prefixHeldUndo is what those names held before the running builtin's
	// prefix was applied, so a declaration that takes one into a scope of its
	// own can hand the take-back over to that scope. See
	// Runner.scopeTakesOverThePrefixEntry.
	prefixHeldUndo []savedVar
	// functionPrefixNames are the names the running function call's own
	// assignment prefix is holding — a second live prefix, one frame out from
	// the one above, and the one `f(){ local c; }` called as `c=2 f` meets.
	// Saved and put back around each call, so an inner call written without a
	// prefix of its own is holding nothing.
	functionPrefixNames []string
	// expandingWord is the word being expanded and expandingSpan which of
	// its spans, so a diagnostic about an expansion can name the text around
	// it: two dialects blame the word rather than the `${…}`, and by the
	// time anything has failed the word is a list of spans. Nil where an
	// expansion was reached from something that is not a word.
	expandingWord *syntax.Word
	expandingSpan int
	// expandingOuterWord is the *outermost* word of that nesting: the word
	// the command line holds, before any operand of an expansion inside it
	// took over expandingWord.
	//
	// The two are a pair because the two word-naming dialects disagree about
	// which one their sentence means, measured 2026-09-12 on an operand that
	// holds an expansion the grammar could not read:
	//
	//	                              ksh93                  bash 5.3
	//	"${v#${BAD}}"                 "${v#${BAD}}"          ${BAD}
	//	"pre${v#${BAD}}post"          "pre${v#${BAD}}post"   ${BAD}
	//	"${u:-${BAD}}"                "${u:-${BAD}}"         ${BAD}
	//
	// So the whole-word subject is the outermost word and the quoting-run
	// subject is the innermost — see badSubstitutionSubject. Reading both
	// from one field made each dialect right on whichever route the other
	// was not.
	expandingOuterWord *syntax.Word

	// scriptText is the source of the program the front end is running, and
	// runText the text some other route is running in its place. Read by one
	// diagnostic and written by the routes that run text; see runningText.
	scriptText string
	runText    runningText

	// expandingNestedInner marks the expansion of the *inner* of a nested
	// `${${…}}`, whose fields are read by the operator around them rather
	// than by the command line.
	//
	// It decides one thing: whether an empty field survives. On the command
	// line an unquoted empty element is no field at all, which is unanimous
	// and has nothing to do with splitting — `a=(one "" two); printf "[%s]"
	// ${a[@]}` is `[one][two]` in every column. As the inner of a nesting it
	// is a value the outer operator is about to read, and it survives:
	// measured on zsh 5.9.2, 2026-09-09, `${(j:,:)${(@)${a[@]}}}` on the same
	// array is `one,,two`, and the spelling without `(@)` is `one  two` —
	// the inner joined on IFS with the empty still between two separators.
	// Either way the element is there.
	//
	// Dropping it made an *alternation pattern* one alternative short and
	// still a valid pattern, so nothing failed and it matched the wrong
	// things (#1596).
	expandingNestedInner bool

	// splitWordLiterals arms the literal text of one word for field
	// splitting: the word a `-` or `+` substitutes is part of an unquoted
	// expansion's result, so the blanks written in it separate fields. It is
	// consumed by the loop that walks that word and cleared for everything
	// inside it. See splittingTheSubstitutedWord.
	splitWordLiterals splitLiterals
	// nestedHeld is one nested expansion's fields, handed from the list half
	// of a span's expansion to the scalar half so the inner runs once. See
	// nestedHold.
	nestedHeld nestedHold
	// sourceHeld is one parameter's source, kept for the other readers of
	// the same node in the same span so that an expansion reads its
	// parameter once — see sourceHold, where the three things a second read
	// costs are written down.
	sourceHeld sourceHold
	// subscriptHeld is the same for one node's subscript, which is
	// arithmetic and may move — see subscriptHold.
	subscriptHeld subscriptHold
	// subscriptSubsts is one subscript's substitutions, run once and kept
	// for every reader of the same brackets in the same expansion — see
	// subscriptSubstHold, which is why a subscript's `$( … )` runs once
	// rather than once per road to it.
	subscriptSubsts subscriptSubstHold
	// unspecified records that a script depended on an axis no dialect had
	// answered, so a caller can tell that from an ordinary failure.
	unspecified bool
	// globMissed records that a pattern matched nothing, so the no-match
	// axis can report it once the whole field is known.
	globMissed bool
	// keepRedirs records that this command's redirections outlive it, which
	// is `exec > log` and nothing else. The dispatcher clears it after acting
	// on it, so it cannot leak into the next command.
	keepRedirs bool
	// completions are the specs `complete` registered, kept verbatim so a
	// bash_completion.d file run in a non-interactive shell registers, lists
	// and removes them the way it would in bash — nothing here completes.
	completions map[string]completionSpec

	// verbose is `set -v`: each line is written back as it is read, which
	// the front end does — it is the one holding the raw text.
	verbose bool
	// errtrace is `set -E`: the ERR trap carries into functions the dialect
	// would otherwise bound it out of.
	errtrace bool
	// functrace is `set -T`: the same carriage for DEBUG and RETURN.
	functrace bool
	// lastArg is the previous simple command's last expanded argument, kept
	// for the `$_` the dialects that move it answer with.
	lastArg    string
	lastArgSet bool
	// inputLastArg is the same record kept under ksh93's rule: only a simple
	// command standing alone on a line at the top level of the input writes
	// it. atInputLevel is the gate, set for each such statement and cleared
	// the moment a command uses it, so everything a call or an `eval` goes on
	// to run is below the level. See interp/underscoreframe.go.
	inputLastArg    string
	inputLastArgSet bool
	atInputLevel    bool
	pendingInputArg underscorePending
	// noexec is `set -n`: read, never run, never unset — even the `set +n`
	// that would clear it is a command.
	noexec bool
	// onecmd is `set -t`, and unlike noexec it is *not* one-way: the front
	// end reads it once the line it was set on has finished, so a `set +t`
	// later on that same line is seen and cancels the stop — measured, `set
	// -o onecmd; set +o onecmd; echo B` on one line of a script runs the
	// line after it too.
	//
	// The shell it stops is the one *reading*, which is why the state lives
	// here and the stopping lives in the front end: this package runs what
	// it is handed and has no say in whether there is more.
	onecmd bool
	// keywordAssignments is `set -k`: with it on, a `name=value` word
	// standing *after* the command name is a prefix assignment rather than a
	// positional argument. See keywordassign.go, which carries the panel and
	// the two shapes recorded there rather than modeled.
	keywordAssignments bool
	// monitor is `set -m`. Background jobs already run in process groups of
	// their own here (see setProcessGroup), so in a non-interactive shell
	// what the option adds is the state itself: the listings and `$-`
	// answer with it, and nothing is announced — measured, no shell in the
	// panel tells a script about its jobs even with the option on. Whether
	// a shell with no terminal grants the request at all is the dialect's
	// (Semantics.MonitorNeedsATerminal). Distinct from JobControl, which is
	// the front end saying there is a *person* to report jobs to.
	monitor bool
	// tracksCommands is command tracking — the option bash lists as hashall
	// and ksh93 as trackall, `set -h` in both. It is permission to remember
	// where commands were found, and since #2554 there is a table behind it:
	// where the dialect reads the option as a stop rather than a preference,
	// turning it off empties nothing and fills nothing, and every spelling of
	// `hash` says so. See Semantics.HashObeysCommandTracking.
	//
	// Read through Runner.commandTracking and written through
	// Runner.setCommandTracking, never directly. Two of the panel start it
	// *on* and say so in `$-`, so the zero value is not their answer, and
	// one of those two turns it back off when it is interactive — a default
	// that moves with the invocation, which a field initialized once cannot
	// hold. tracksCommandsMoved is what parts "the script said so" from
	// "nobody has said anything yet"; until it is set the answer comes from
	// the startup letters, which is where the dialect already declared it
	// (#1951).
	tracksCommands      bool
	tracksCommandsMoved bool
	// stdinOptionMoved and stdinOption are `set -o stdin`, one dialect's name
	// for the route the program arrived by. The route is a fact the front
	// end carried in, so the option *reads* it until a script writes it —
	// which that shell allows, measured: `set +o stdin` on the standard-input
	// route takes the `s` out of `$-`, and `set -o stdin` under `-c` puts one
	// there. Two fields rather than one because "not moved" and "moved to
	// off" are different answers and the route decides only the first.
	stdinOptionMoved bool
	stdinOption      bool
	// debugOption is the same dialect's `debug`, which its own shipped build
	// acts on no more than this one does: the name is listed, remembered and
	// reported, and it earns no letter in `$-`. Measured — `set -o debug`
	// there is `debug on` in the listing and an unchanged `$-`.
	debugOption bool
	// histIgnoreDups is zsh's histignoredups, which its `set -h`
	// abbreviates. A script cannot see what it does, because a script has no
	// history — but an interactive session does: repl reads it through the
	// dialect's option namespace before it records a line, and a line
	// repeating the one before it is then left out of the list and out of the
	// file. So the state was kept truthfully here before anything read it,
	// and is now read.
	histIgnoreDups bool
	// histExpand is whether `!!` and its family are rewritten before a line
	// is parsed — bash and ksh93 spell it `histexpand` and `set -H`, and zsh
	// spells it `banghist`.
	//
	// Kept here and read by the front end for the reason histIgnoreDups is:
	// this package holds no list. It is *not* the same switch as histRecord
	// below — the two names are listed separately in `set -o` because they
	// are separate states, and `set -H` alone expands nothing.
	//
	// Separate states, and an **AND** wherever a front end reads them: this
	// comment used to say `set +o history` left the expander on over what
	// was already in the list, and that was reasoning rather than a
	// measurement. Measured 2026-09-16 in a script, `set +o history` stops
	// the expansion too, and a later `set -o history` starts it again over
	// the entries the list still holds. Which of the two switches a given
	// route reads is the front end's, and driver's history gate reads both.
	//
	// The default is the dialect's, applied by the front end: measured
	// 2026-09-15 through a pty with a two-row prompt, bash 5.3, bash 3.2,
	// bash-as-sh and zsh 5.9.2 all start an interactive session with it on
	// and a script with it off, and ksh93u+ starts with it off everywhere.
	// See Semantics.HistoryExpansion and HistoryExpansionAtAPrompt.
	histExpand bool
	// histExpandMoved says a script has written histExpand itself, so the
	// front end's default must not overwrite it. `set +H` in an rc file is
	// the whole reason: the file runs before the session's first prompt, and
	// a default applied afterwards would put back the thing it turned off.
	histExpandMoved bool
	// histMemory is the last substitution and search history expansion made,
	// which the next line's `:&` and `%` read. See histexpand.Memory.
	histMemory *histexpand.Memory
	// histRecord is bash's `set -o history`: whether accepted lines are added
	// to the list at all. On for an interactive session and off for a script,
	// measured the same way.
	histRecord bool
	// histEntries and histAdd reach the list itself, which a dialect owns.
	// See SetHistoryStore for why they take a runner rather than close over
	// one.
	histEntries func(*Runner) []string
	histAdd     func(*Runner, string)
	// histStart and histFinish are what the dialect does with its history
	// file when a script first turns the list on and when the shell ends,
	// and histStarted says the first of them has happened. See
	// SetHistoryFile.
	histFirst   func(*Runner) int
	histStart   func(*Runner)
	histFinish  func(*Runner)
	histStarted bool
	// histFromReader says the front end is putting the program's own
	// commands into that list, which is what makes a builtin's own line the
	// last entry. See SetHistoryListFilledByTheReader.
	histFromReader bool
	// tracksWindowSize is permission to keep $LINES and $COLUMNS abreast of
	// the terminal. bash spells it `checkwinsize` and zsh has no name for it
	// at all because it never stops doing it; the *capability* is neither
	// shell's, which is why the switch is here and the spelling is not.
	//
	// A script cannot see it — a script has no window — so like
	// histIgnoreDups the state is kept here and read by the front end, which
	// is the only thing holding a terminal to ask. Off by default because the
	// panel is split: bash 3.2 starts with it off, dash never assigns either
	// variable, and a core that assigned them anyway would be answering a
	// question three of the six columns do not ask. Each dialect turns it on
	// in Apply where its shell does.
	tracksWindowSize bool
	// windowTerminal is the terminal this shell found, remembered so that a
	// read after the script has redirected itself still has one to ask. See
	// Runner.terminal, which is where the remembering is and where the
	// measurement that says to remember it is; terminalSize named it, and
	// `read -k` now asks the same question about the same file.
	windowTerminal *os.File
	// windowRows and windowCols are how big that terminal was when $LINES and
	// $COLUMNS last answered, and windowSettled says the pair has been read at
	// least once. Together they are the memory the rule needs: a window that
	// *changed* supersedes what a script assigned, and "changed" cannot be
	// asked without a number to compare against. See windowSizeValue.
	windowRows, windowCols int
	windowSettled          bool
	// alwaysErrName and alwaysInterruptName are what the dialect spells the
	// two try-always parameters, empty where it has none. alwaysDepth is how
	// many always halves execution is inside, and the two values are what
	// those parameters read there. See alwaysstatus.go.
	alwaysErrName        string
	alwaysInterruptName  string
	alwaysDepth          int
	alwaysErrValue       int
	alwaysInterruptValue int
	// providesWindowSize says this shell has $COLUMNS and $LINES as
	// parameters of its own rather than as variables something assigns. See
	// ProvideWindowSize, and Runner.TracksWindowSize for the other reading.
	providesWindowSize bool
	// autoCd is permission to read a bare directory name as a `cd`. bash
	// spells it `autocd` and zsh spells it `autocd` too — the same name in
	// two option namespaces, which is exactly why the behavior cannot live in
	// either dialect: two copies of it would be two chances to fix one and
	// not the other.
	//
	// It is consulted only where command lookup has already failed and only
	// in an interactive shell, both measured — a directory named `echo` does
	// not shadow the builtin in bash 5.3 with the option on, and
	// `bash -c 'shopt -s autocd; subdir'` says `command not found`.
	autoCd bool
	// emptyCommandWordOffersNothing is the deviation behind bash's
	// `no_empty_cmd_completion`, and it is stored as the deviation on
	// purpose: this shell completes an empty command word against everything
	// that could run, so the zero value is what it already does and the field
	// records only a session that has asked it to stop.
	//
	// The name in the table is a negative and the switch here is not, which
	// is the whole reason the two are separate: `shopt -s
	// no_empty_cmd_completion` is a request to turn this capability *off*,
	// and a table that stored the option's own bit would have granted it by
	// storing a `true` that meant "on".
	emptyCommandWordOffersNothing bool
	// dotSearchesPathOff withholds the PATH search `.` makes for an operand
	// with no slash in it — bash's `sourcepath`, which is on by default and
	// is the only name any shell in the panel has for the question.
	//
	// Stored as the *negative* so the zero value is the default the option
	// has, which is on. A Runner that was never told about the switch
	// searches PATH, which is what every shell here does.
	dotSearchesPathOff bool
	// shiftPastEndQuiet withholds the complaint `shift` makes for a count
	// above `$#` — bash's `shift_verbose`, which is **off** by default there
	// and is the only name any shell in the panel has for the question.
	//
	// Stored as the negative so the zero value reports, which is what every
	// other column does: a dialect with nothing to say about the count says
	// it through an empty Diagnostics.ShiftTooMany, and this field is for the
	// one shell that has the sentence and withholds it. The other end of the
	// range is not governed by it — bash names a negative count with the
	// option off as well as on, measured.
	shiftPastEndQuiet bool
	// echoExpandsEscapes makes `echo` interpret its backslash escapes with no
	// `-e` in front of them — bash's `xpg_echo`, and the only name in the
	// panel for asking a shell to move to the other side of
	// Semantics.EchoInterpretsEscapes while it runs.
	//
	// The positive direction, because it names a capability the shell turns
	// *on*: a Runner that was never told follows its dialect's axis, which is
	// what the zero value already did.
	echoExpandsEscapes bool
	// localInherits makes a valueless local declaration take the value and
	// attributes of the name at the enclosing scope instead of starting
	// empty — bash's `localvar_inherit`, and the only name any shell in the
	// panel has for the question.
	//
	// The positive direction, because the fresh binding every shell with a
	// local scope makes is what the zero value already does. The letter that
	// asks for the same thing one declaration at a time is declareFlags'
	// `inherit`. See localinherit.go.
	localInherits bool
	// cdCorrectsSpelling is permission for `cd` to correct a misspelled
	// operand rather than refuse it — bash's `cdspell`, and the only shell in
	// the panel with a name for it. The correction itself is in
	// spellcorrect.go, where the completer's `dirspell` will reach the same
	// one rather than growing a second.
	//
	// Interactive only, like the two names beside it, and measured that way:
	// `bash -c 'shopt -s cdspell; cd subdri'` refuses exactly as a shell
	// without the option does.
	cdCorrectsSpelling bool
	// completionCorrectsSpelling is the same permission for the *completer*
	// — bash's `dirspell` — and it is a second field rather than a second
	// reader of cdCorrectsSpelling because bash has two names and a session
	// may hold either one alone.
	//
	// The correction it reaches is the one in spellcorrect.go, and what it
	// asks for is spelled differently there: `cd` shows a person the operand
	// in the shape it was typed, and the completer puts an absolute path in
	// the line. See Runner.CorrectedDirectory.
	completionCorrectsSpelling bool
	// completionExpandsDirectory is permission for the completer to write the
	// directory it actually *read* back into the line, rather than the text
	// that was typed — bash's `direxpand`.
	//
	// It is the other half of the pair, and the pair is not a convention:
	// measured through a pseudo-terminal against bash 5.3.15, `dirspell`
	// alone rings the bell and leaves the line as typed, because a correction
	// nothing may write back is a correction nobody sees. See
	// shellCompleter.paths, which is the only reader.
	completionExpandsDirectory bool
	// checksRunningJobsAtExit is permission to hold the exit for a job that
	// is still running, and to follow the sentence with the job table. Both
	// shells that hold an exit call it `checkjobs`; see
	// Runner.ChecksRunningJobsAtExit for the measurements and for why one
	// bit carries both halves.
	//
	// Off by default because bash starts it off, and turned on by the zsh
	// dialect in Apply, where that shell starts it on.
	checksRunningJobsAtExit bool
	// stoppedJobExitCheckOff is the deviation behind the *stopped* half of
	// the same question, stored as the deviation for the reason
	// emptyCommandWordOffersNothing is: the zero value has to be the shell
	// Semantics.StoppedJobsHoldTheExit already describes, and only a session
	// that has said `unsetopt checkjobs` is departing from it.
	stoppedJobExitCheckOff bool
	// keepsLastPipelineElement is a session asking for the last element of a
	// pipeline to run in this shell rather than in a subshell, overriding
	// Semantics.LastPipelineElementInCurrentShell for as long as it is set.
	//
	// A switch as well as an axis, for the reason stoppedJobExitCheckOff is:
	// one shell in the panel lets a script move the answer and the rest hold
	// it fixed. The zero value is "nobody has asked", so a Runner that has
	// never seen the option follows the axis — which is what keeps the two
	// shells that run the last element there anyway from needing it.
	//
	// See Runner.KeepsLastPipelineElement for the measurements, including the
	// monitor's part in it.
	keepsLastPipelineElement bool
	// editingMode is which of the two `set -o` editing modes is selected,
	// and it is one field because the two names are one state: `set -o vi`
	// in bash 5.3 and in ksh93 turns `emacs` off in the same breath.
	//
	// Three values and not a bool, which is the measurement rather than a
	// generalization: `set +o vi` and `set +o emacs` both leave *both* names
	// off in bash 5.3 and ksh93, so "neither" is a state a script can ask
	// for and a bool could not hold. Turning one on is the only way back.
	//
	// What it selects is which keymap a dialect's binding builtin acts on and
	// nothing more, because this editor has no command mode — the same
	// documented partial the zsh dialect's keymaps keep. The mode is here
	// rather than in repl because `set -o vi` is answerable in a script that
	// has no editor at all, and it is in the core rather than a dialect
	// because both names are the standard's and every shell in the panel has
	// them.
	editingMode EditingMode
	// posixMode is `set -o posix`, and the posixSaved fields are the answers
	// the axes it moves held before it was turned on, so turning it off
	// restores the dialect's rather than asserting the standard's opposite.
	// Named fields rather than a saved vector: an option changed *while*
	// posix mode is on is not part of the mode and must survive leaving it.
	//
	// One per axis, and they cannot be collapsed into one: the dialects do
	// not agree across them, so a single remembered answer would put the
	// wrong one back for whichever shell disagrees with the rest. zsh carries
	// on past a failed redirection on a special builtin and stops on `unset`
	// of a readonly name; ksh93 is the reverse. The listings below make the
	// same point in the other direction — zsh writes `export V='a b'` for
	// one and `typeset -r R=2` for the next.
	posixMode               bool
	posixSaved              Answer
	posixSavedUnsetReadonly Answer
	// posixSavedReassignReadonly is ReadonlyReassignmentFatal's, for the
	// same reason.
	posixSavedReassignReadonly Answer
	posixSavedSpecialReadonly  Answer
	// arithCommand counts the `(( ))` commands being evaluated, and
	// refusedInACommand says a readonly refusal was made inside one or inside
	// a builtin — see Runner.refuseReadonlyInACommand.
	arithCommand      int
	refusedInACommand bool
	// Whether a pattern written into a redirection's target is matched.
	// Saved like the rest: POSIX forbids it and both columns that do it were
	// measured to stop in the mode, so the mode asserts the standard's answer
	// on the way in — and has to put the dialect's own back on the way out,
	// since zsh matches one and would otherwise leave `set +o posix` having
	// quietly turned its own reading off (#3207).
	posixSavedTargetPattern Answer
	posixSavedForName       ForNameRunForm
	posixSavedFuncName      FuncNameRunForm
	posixSavedBadOption     Answer
	// The two `set` refusals, saved separately from each other and from the
	// bad-option axis above because the panel does not answer them alike:
	// BusyBox ash is fatal on the letter and not on the name, so one
	// remembered answer would put the wrong one back for it.
	posixSavedBadSetName   Answer
	posixSavedBadSetLetter Answer
	// Whether an assignment written in front of a special builtin is still
	// set on the next line. Saved like the rest, and for a reason the others
	// only half share: the two dialects that answer `No` are the two with a
	// POSIX mode, so this is the field that would be *most* wrong to assert
	// the standard's answer to on the way out.
	posixSavedAssignPrefix Answer
	// The three listing axes the mode moves, saved for the same reason and
	// separately for the same reason: no shell in the panel answers all
	// three alike, so one remembered form could not put three back. The
	// fourth listing axis, DeclareListing, is deliberately absent — `declare
	// -p` does not move, which is what makes this three fields and not a
	// single "listings" one.
	posixSavedExportListing   DeclarationListingForm
	posixSavedReadonlyListing DeclarationListingForm
	posixSavedBareListing     DeclarationListingForm
	// And the one axis the mode moves that is not on the semantics vector at
	// all: whether an alias may stand in for a word the grammar reserves is
	// a question about how a line is *read*, so it lives on the dialect and
	// the mode reaches it the way SetMatchOption reaches the grammar — by
	// replacing the Dialect rather than writing through it. Saved here for
	// the reason the others are: bash takes the alias and zsh takes it too,
	// so leaving the mode has to put the shell's own answer back rather than
	// assert the standard's opposite.
	posixSavedAliasReserved bool
	// And the second dialect field the mode moves, saved for the same reason
	// and separately because the two do not agree: bash expands a reserved
	// word's alias and moves the brace scan, zsh expands the alias and leaves
	// the brace scan alone, so one remembered value could not put both back.
	// See syntax.Dialect.QuoteProtectsTheClosingBraceInPosixMode.
	posixSavedQuoteProtects syntax.BraceQuotePolicy

	// fds are the descriptors beyond the three named streams — what
	// `exec 6>&1` saves and `>&6` finds again. Values are the io.Reader or
	// io.Writer the descriptor stood for when it was made, which is what
	// "copied as it is now" means. The table is the shell's own bookkeeping:
	// a child process sees the stream a redirection resolved to, not the
	// number, so a script that hands a bare descriptor number to a child for
	// the child's own use is not served by this.
	fds map[int]any
	// coproc is the pair of descriptors in that table that the running
	// coprocess is reached by. Kept apart from the array a dialect may also
	// publish them in, because one of the two shells with a coprocess
	// publishes no array and reaches them by a letter — see coproc.go.
	coproc *coprocEnds
	// execFds are the numbers in that table that `exec`'s own redirection
	// list opened, which one dialect keeps to itself when it runs anything.
	// A per-command redirection on the same number takes the mark off for
	// that command — see ExecOpenedFdReachesACommand, which is the only
	// reader.
	execFds map[int]bool
	// cloexecFds are the numbers in that table a builtin has marked as not
	// reaching what this shell runs — `sysopen -o cloexec` and nothing else
	// so far. See Runner.KeepDescriptorFromChildren, which is the only
	// writer, and childFiles, which is the only reader.
	cloexecFds map[int]bool
	// outputClosedByThisCommand says the redirection list now in effect for
	// this command closed the stream its output goes to — `echo hi >&-` and
	// not `exec 1>&-; echo hi`.
	//
	// One dialect needs the distinction to word a failed write: it says
	// `write error: bad file descriptor` for a write to a stream something
	// *else* closed and says nothing when the writing command closed it
	// itself, and both routes keep the same status. Measured — a close
	// restated on the command silences it even after `exec` parked one, and
	// a close on a group or a function call does not silence the commands
	// inside, so the question is neither `exec` nor the value at the stream
	// but who wrote the redirection. See
	// Diagnostics.InheritedClosedStreamWriteError, which is the only reader.
	//
	// Cleared at the top of every applyRedirs, which is what scopes it to one
	// command: a group's close does not travel to the commands in its body,
	// because each of those reaches applyRedirs of its own.
	outputClosedByThisCommand bool
	// openedName is the name the redirection just applied opened, as the
	// script named it rather than as the filesystem was asked for it — the
	// join with the working directory is not what a shell prints. Written by
	// applyRedirs on every open and read by readFileSubst, which is the only
	// reader: `$(<dir)` words a read that failed *after* a successful open,
	// and by then the word has been expanded and nothing else remembers what
	// it came to.
	openedName string
	// redirFds are the numbers beyond the named streams that the command
	// being set up has just written, in the order it wrote them. The caller
	// reads it once, to learn what to mark when a command's redirections
	// turn out to outlive it.
	redirFds []int
	// inheritedPublished says InheritedFiles has already been put into the
	// table. Copied by clone with the rest of the struct, which is what keeps
	// a subshell from publishing over the table it was cloned with.
	inheritedPublished bool
	// custom holds builtins registered by a shell built on this package. A
	// nil value is an explicit removal.
	custom map[string]Builtin
	// jobs are the background commands started by this shell.
	jobs []*Job
	// jobOrder is the order jobs became *notable*, oldest first: a job is
	// appended when it enters the table and again, moved to the end, every
	// time it stops. It is not the table's order, which is slot order, and
	// the two part company the moment a job stops after a later one started.
	//
	// A slice rather than a `lastJob` pointer because two markers are read
	// off it — the `+` of `%%` and the `-` of `%-` — and the second is not
	// "the job before this one in the table". Measured 2026-09-12 through a
	// pseudo-terminal, with two jobs stopped and a third backgrounded after
	// them: bash 5.3.15 marks the second `+`, the *first* `-`, and the third
	// not at all, which no reading of the table's order produces. See
	// markedJobs.
	jobOrder []*Job
	// jobIdents hands out the numbers a job with no process of its own
	// answers to — see Runner.inventJobIdent, which is where the whole of it
	// is. A pointer because it is shared down the clone chain rather than
	// copied with the struct: a subshell must not invent a number one of the
	// jobs it inherited already holds.
	jobIdents *atomic.Uint32
	// reaped are the jobs a `wait` has already reported the status of, newest
	// last and bounded by reapedJobsKept.
	//
	// They are out of the table: nothing lists them, `%1` does not name them,
	// and the number they held is free for the next job — which is the whole
	// of #2651. They are kept only so that a `wait` naming the same process id
	// a second time can still answer, which six of the seven measured columns
	// do. See Runner.reap and Semantics.WaitRemembersAReapedJob.
	reaped []*Job
	// procSubJobs is the process substitution bodies `$!` has named, where
	// the dialect says a substitution does. Beside the table and never in it:
	// `jobs` does not list them and a bare `wait` does not wait for them, and
	// only a `wait` naming the number reaches one. Bounded as reaped is. See
	// Semantics.ProcessSubstitutionIsTheLastBackgroundJob.
	procSubJobs []*Job
	// lastJobPID is `$!`, which is a *value* and not a reference to a job.
	//
	// Separate from jobOrder because the two stop being the same thing the
	// moment the job ends. The current job — what `%%` names and what a bare
	// `fg` picks — has to go when the job leaves the table, or those two
	// would name something that is not there. `$!` does not:
	// measured unanimous 2026-09-05, `sleep 0 & wait; echo "[$!]"` reports the
	// pid in bash 5.3.15, bash 3.2.57, bash 3.2 as `sh`, dash, ksh93u+ and
	// zsh 5.9.2, and so does the same script under `-i` on a pseudo-terminal
	// where the `Done` notice has already been printed and the job forgotten.
	//
	// It was one field, and the notice forgetting the job emptied `$!` with
	// it. That only showed on a route where something asks for the notices
	// between commands, which until now was the prompt alone.
	// Zero is a real answer here and not an absence — a background builtin
	// runs in this process and its job carries no pid of its own — so whether
	// one has ever been started is a fact of its own rather than a value the
	// number can carry.
	lastJobPID    int
	lastJobPIDSet bool
	// toldOfJobsAtExit says the chunk *before* this one showed the person the
	// jobs that would be abandoned, so the shell will not hold its exit for
	// them again; tellingOfJobsAtExit is this chunk saying so, and becomes
	// the other at the next one.
	//
	// Two fields because what suppresses the warning is the thing immediately
	// before it and not anything that has ever happened. Measured through a
	// pseudo-terminal: ^Z then `exit` warns and a second `exit` leaves; ^Z
	// then `jobs` then `exit` leaves, because the listing is the shell showing
	// the same thing on purpose; and ^Z, `jobs`, any other command, `exit`
	// warns again. One sticky flag gets the first three right and the fourth
	// wrong.
	toldOfJobsAtExit    bool
	tellingOfJobsAtExit bool
	// oldpwdSettled says the inherited OLDPWD has already been read and
	// judged, which happens once however many chunks a session runs.
	//
	// A flag rather than the presence of the name, because two of the three
	// answers leave nothing behind to look at: a value that is kept stays in
	// the environment where it arrived, and re-deciding per chunk would stat
	// it again on every line a person types. See settleInheritedOldpwd.
	oldpwdSettled bool
	// bg is set on the runner *inside* a background job, so the process it
	// starts can be recorded against the job.
	bg *Job
	// inJob is the background job whose processes this shell is starting,
	// which is a wider set of shells than bg is set on: every element of a
	// backgrounded pipeline is part of the job, and only the last of them
	// names it. Keeping the two apart is what lets `$!` stay one pid while
	// `kill %1` reaches the whole pipeline — see Job.took, and the comment
	// beside `sub.bg = nil` in pipeline.go for why only one element may
	// settle the number.
	inJob *Job
	// part is this shell's share of the job's start: `&` does not return
	// until every element of a backgrounded pipeline has one. Nil where this
	// shell is not one of several — see Job.expectPart.
	part *jobPart
	// scopes is the stack `local` unwinds. Shell scoping is dynamic, so
	// there is one set of variables and this records what to put back.
	scopes []*scope
	// redirErr records that a redirection failed to open. The command must
	// not run: a redirect that could not be applied would otherwise send its
	// output to the terminal, which is the loudest possible wrong answer.
	redirErr bool

	// badDupTarget records that the redirection which failed was `<&word` or
	// `>&word` naming something that is not a descriptor, rather than an open
	// that did not work. One dialect ends the shell over the first and not
	// the second, and only on a command that runs in the shell, so the
	// distinction has to reach the place that knows which command it was.
	badDupTarget bool
	// writeFailed records a builtin's output write that failed — into a
	// descriptor closed with `>&-`, most plainly. The write already
	// happened and went nowhere, so there is nothing to retry; the question
	// left is whether the command is said to have worked, and that is the
	// dispatcher's to fold in once the builtin returns. Cleared before each
	// builtin runs, so a failure is only ever read by the builtin it
	// belongs to.
	writeFailed error
	// printfOut is the writer of the `printf` pass that is running, or nil
	// when none is. It is here because two things outside that loop need to
	// reach it: a diagnostic written while a conversion is being formatted
	// waits for that conversion to resolve, so that a dialect writing
	// through keeps its output ahead of the message and a refusal can still
	// take the pass back; and the star reader is what takes it back (#2664).
	printfOut *printfWriter
	// arithValueSurvivesTheDivision turns on the reading in which a division
	// by zero leaves a value behind and the evaluation carries on with it.
	//
	// A mode rather than a plain dialect answer because the value can only be
	// *seen* in one place. Every other site an expression is written in
	// abandons the command over the failure — `echo $(( 1/0 ))`, `x=$((1/0))`
	// and `${a[1/0]}` all end the line in ksh93 as they do everywhere else —
	// so a rule applied there would change nothing observable and would risk
	// letting a failure through. A `printf` operand is the exception: the
	// complaint goes out, the conversion still runs, and the number it writes
	// is the one the evaluator was holding. See
	// Semantics.ArithDivisionByZeroYieldsAValue for the rows.
	arithValueSurvivesTheDivision bool

	// arithDivisionFailure is the first such failure held while that mode is
	// on, for the caller to raise once the expression has been read to the
	// end. Nil when none has happened.
	arithDivisionFailure error

	// printfConversionName is the conversion character being formatted, or
	// `.` where what is being read is a `*` operand. One column's `printf`
	// names it in a second complaint line about an operand its arithmetic
	// could not read — see Diagnostics.PrintfArithArgumentType — and the
	// number reader is several calls below the place that knows which
	// conversion asked.
	printfConversionName string
	// printfLostStars is how many `*` the prefix just scanned wrote that a
	// later run in the same field replaced. Their operands are still taken;
	// see printfLostStars, and printfSpecPrefix for why the count travels
	// here rather than out of the scan.
	printfLostStars printfLostStars
	// printfLongModifier says the conversion just scanned carried an `l`
	// among its length modifiers, which one column reads as "count this
	// field in characters" — see Semantics.PrintfLongModifierCountsCharacters.
	// Carried here for the reason printfLostStars is.
	printfLongModifier bool
	// line is where execution currently is, for diagnostics that name it.
	// Real shells report the line of the command that failed, so this is
	// updated per statement rather than per token.
	line int
	// prevLine is where the line stood before the command now running set
	// it: the line of the last command that actually ran, and 0 before
	// anything has.
	//
	// One construct reads it rather than its own line — a `case` subject
	// there is expanded before the line advances, so `$LINENO` in it is the
	// line of the command in front of the `case` and so is the location of
	// any complaint the subject makes. See
	// Semantics.CaseSubjectKeepsThePreviousLine.
	prevLine int
	// caseSubjectPrev is that line while a `case` subject is being expanded,
	// and zero everywhere else. Runner.lineNow is where it is taken up; it
	// is held here rather than written into line so that a subject reading
	// nothing about the line never raises the axis at all.
	caseSubjectPrev int
	// rangeRefused says the offset of the substring range being evaluated
	// was refused, so its length is not evaluated at all. One complaint per
	// range is unanimous — `${x:&&:&&}` is one line in bash 5.3 and one in
	// ksh93u+ — and we wrote the offset's and then the length's (#2818).
	rangeRefused bool
	// viewIsAlreadyFolded says the scalar view of an array is being written
	// from an element this name's attributes have already folded, so the
	// store does not fold it a second time. Set by storeArray around that
	// one call; see setVarAs.
	viewIsAlreadyFolded bool
	// setOptionStatus is what the last refused `set -o` name reports, which
	// is one of the four dialects' answers rather than a constant.
	setOptionStatus int
	// setRefusalOwed records that `set` has reported at least one bad option
	// and still owes the fatality that follows them, and setUsageOwed that
	// one of those refusals wanted a usage block under it.
	//
	// They exist because one dialect reports **every** bad option word before
	// either — see Semantics.SetReportsEveryBadOption — so the report and
	// what comes after it are no longer one step.
	//
	// Two flags and not one, because the usage block is not owed by every
	// refusal: a bad *letter* draws one where the dialect prints one at all,
	// and a bad `-o` name draws one only where SetInvalidOptionNameUsage
	// says so. Printing it because *something* was refused would give a
	// dialect that prints none under a name one anyway, which is a wrong
	// answer that the one shell answering this axis happens to hide.
	//
	// Nothing else may read either: both are set and consumed inside one
	// call of the builtin.
	setRefusalOwed bool

	// setSortsOperands is `set -s` in the dialect whose letter sorts: the
	// letter is read before the operands are, so what it asks for has to
	// wait for them. Set and consumed inside one call of the builtin; see
	// Semantics.SetSLetterSortsTheOperands.
	setSortsOperands bool

	// pendingOptionListing is the `set -o` listing a deferred parse still
	// owes, and which form it is in. One dialect writes one listing after
	// the whole option parse rather than at each `-o`, so the form has to
	// survive the words between; see Semantics.SetListsOptionsOnceAtTheEnd.
	//
	// Cleared at the top of every `set`, and read only by the one function
	// that wraps the parse: it is a value passed between two halves of one
	// call rather than shell state, and nothing outside that call may see
	// it.
	pendingOptionListing pendingListingForm
	setUsageOwed         bool
	// setUsageOwedBlock is which block that debt is for, recorded at the
	// refusal because the spelling that chooses it — a letter, an `-o` name,
	// a `--name` — is known there and nowhere later.
	setUsageOwedBlock string

	// setLettersWelded records that the letters being read are the ones that
	// were welded behind an `-o` in the same word, which the pass that
	// validates every letter before applying any cannot see. See
	// setWeldedLetters and Semantics.SetValidatesOptionLettersFirst.
	setLettersWelded bool
	// setRefusalSpelling is which of `set`'s two refusals owes the fatality
	// above — the long `-o` name or the option letter — because the two have
	// their own status and their own axis and one dialect answers them
	// differently. Recorded at the *first* refusal; see setRefusalStatus.
	setRefusalSpelling setRefusalSpelling
	// atInvocation marks a `set` option applied by the front end from the
	// words the shell was started with, rather than by the builtin from a
	// line of script. The panel words the two refusals differently — nobody
	// names `set` at an invocation, and the two shells that print a usage
	// block print the *shell's* there and the builtin's here — so the same
	// refusal has to know which it is. Set for the length of one call in
	// SetOptionLetters and SetNamedOption, which are the front end's only
	// way in.
	atInvocation bool
	// longSetOptionSpelling marks an option word written as `--name` rather
	// than as a letter or as an `-o` name. It changes nothing about which
	// option is moved and everything about the usage block under a refusal:
	// the one shell with this spelling prints a *different* usage for it,
	// the one naming its long options, so the block cannot be chosen from
	// the route alone. Set for the length of one call in SetLongOption and
	// around the builtin's own reading of the word.
	longSetOptionSpelling bool
	// fromEnvironment is the third of those: an option name that arrived in
	// the environment rather than in an argument vector or a script. It is
	// the plainest refusal of the three — the location and the sentence, with
	// nothing standing where `set` would and no usage block — and it is set
	// for the length of one call in ApplyInheritedShellOptions.
	fromEnvironment bool
	// outsideSetBuiltin marks an option request that did not come from
	// `set`: a dialect's own option builtin, or the environment's list. It
	// is what keeps a refusal from ending the script in the dialects where a
	// refused `set` option ends one — the fatality is the special builtin's
	// and not the option's. Set for the length of one call in
	// ApplyNamedOption; see endOnSetRefusal for the measurement.
	outsideSetBuiltin bool
	// throughCommandWord marks the builtin `command` is running right now,
	// whose specialness that word takes away. POSIX names this as the whole
	// reason `command` exists — a special builtin's failure is survivable in
	// front of it — and Runner.takeSpecialBuiltinFailure is the one place it
	// is read.
	//
	// Cleared by Runner.stmt, so it covers the builtin `command` named and
	// nothing that builtin goes on to run. See takeSpecialBuiltinFailure for
	// the measurement that draws the line there.
	throughCommandWord bool
	// defaultPathSearch is `command -p`: the lookup walks the standard
	// utility path rather than this shell's PATH, so a script that cannot
	// trust PATH can still reach a standard utility. Set around the lookup
	// and the exec alone — never around a builtin `command` ran, because the
	// script that builtin runs is the caller's. See interp/defaultpath.go.
	defaultPathSearch bool
	// allexport marks every assignment for the environment: `set -a`.
	allexport bool
	// extraOptions are the `set -o` names this dialect has beyond the ones
	// every shell has. Declared through AddSetOptions; see setoptions.go.
	extraOptions map[string]bool
	// negatedOptions are the names this dialect lists as the *opposite* of a
	// state the substrate holds — `clobber` for `noclobber` — mapped to the
	// substrate name. Declared through AddNegatedSetOptions.
	negatedOptions map[string]string
	// immovableOptions are the names this dialect lists and `set` will not
	// take, whatever the substrate could do with them. Declared through
	// AddImmovableSetOptions.
	immovableOptions map[string]bool
	// inertOptions are the names this dialect lists and takes in both
	// directions without the state ever moving. Declared through
	// AddInertSetOptions; `privileged` in ksh93 is the one. Not the same as
	// immovableOptions, which refuse the request out loud.
	inertOptions map[string]bool
	// defaultOnOptions are the `set -o` rows this shell's compiled default
	// has on, which is what `set --default` puts back and is not the same
	// thing as the state the shell starts in. Declared through
	// AddDefaultOnSetOptions; ksh93's four movers are the panel's only ones.
	defaultOnOptions map[string]bool

	// recordedOptions are the `set -o` names a script has moved that this
	// shell remembers and does not act on — see recordedOption in
	// setoptions.go. Absent means the name is still at the state the table
	// says a shell that has moved nothing is in, which is why this holds
	// what was asked for rather than a bit per name.
	recordedOptions map[string]bool
	// promptUser answers the login name the `%n` prompt escape reports,
	// brought in by whoever is allowed to ask the system for it. Nil in a
	// runner nobody told, where the escape is refused rather than guessed
	// at; a function that answers empty is a runner that was told and has
	// no answer, which is refused the same way.
	//
	// A function rather than a string because the asking is what costs.
	// Measured on darwin, `user.Current` is 0.83-1.10 ms — Directory
	// Services, with or without cgo — and it was paid by every `sh -c`
	// and every subshell to fill an escape that only a prompt can draw
	// (#1403). The rule it exists for is untouched: this package still
	// does not go asking, it calls back what the binary handed it, and it
	// does so when the prompt asks rather than when the shell starts.
	// Installed through SetPromptUser or SetPromptUserFunc; see extend.go.
	promptUser func() string
	// promptHost answers the machine's name the `%m` and `%M` prompt
	// escapes report, carried in the same way and for the same reason: a
	// Runner embedded in another program does not go asking the operating
	// system what host it is on. Nil in a runner nobody told, where both
	// escapes are refused rather than guessed at. Installed through
	// SetPromptHost or SetPromptHostFunc.
	promptHost func() string
	// promptHostParam is the parameter those same two escapes read instead,
	// where the dialect says the machine's name is a variable a script can
	// assign. Empty in a dialect that has no such parameter, which is bash:
	// measured, `\h` draws the system's name however `$HOSTNAME` is set.
	//
	// It wins over promptHost outright rather than serving as a fallback for
	// it. zsh's `$HOST` is assignable and `%m` follows the assignment on the
	// same line, so an answer kept from the first draw would be the one thing
	// this must not do. Installed through SetPromptHostParameter.
	promptHostParam string
	// promptStyle is the prompt-escape table this shell's dialect supplies —
	// the same table the prompt drawer reads, which is the whole of #1090.
	// The zero value has no escape character and so no escape language, which
	// is what a runner nobody told gets. Installed through SetPromptStyle;
	// see interp/prompt.go.
	promptStyle PromptStyle
	// promptVisual is what the renderings this shell has already done left
	// the terminal set to, so that a restoring code writes back a color an
	// *earlier* rendering chose. The shell's, not one walk's — see
	// promptVisualState in interp/prompt.go for the measurement.
	promptVisual promptVisualState
	// flagArgEscapes decodes the argument of an expansion flag the way the
	// `(p)` flag in front of it asks for — `${(pj:\n:)a}` joining on a real
	// newline rather than on a backslash and an `n`.
	//
	// A function the dialect supplies rather than a table here, because the
	// escape set is measured per shell and this package holds nobody's: the
	// same shell's `echo` and `print` disagree on `\c` and on whether octal
	// needs a leading zero, so "the escapes" is not one answer. Nil in a
	// runner nobody told, where `(p)` is refused by name rather than read as
	// a no-op — a `(p)` that quietly did nothing would join on the two
	// characters and answer at status 0. Installed through
	// SetFlagArgumentEscapes; see interp/expandflags.go.
	flagArgEscapes func(r *Runner, text string) string
	// expansionEscapes reads the escapes in a *value* the way the `(g)`
	// expansion flag asks for, with the flag's option letters saying which
	// parts of the set are live — `${(g::)v}` reading them one way and
	// `${(g:oe:)v}` another.
	//
	// A function the dialect supplies for the reason flagArgEscapes is one,
	// and the same one: the set is a measurement about a shell and this
	// package holds nobody's. Nil in a runner nobody told, where `(g)` is
	// refused by name rather than read as a no-op — the flag's commonest
	// argument is the empty one, and a value with no escape in it comes back
	// unchanged either way, so a no-op would pass the first thing anyone
	// tried and answer the rest at status 0. Installed through
	// SetExpansionEscapes; see interp/escapeflag.go.
	expansionEscapes func(r *Runner, text, opts string) string
	// parameterTypeWord words what a name *is* — `${(t)v}`'s answer — the
	// way this shell says it. A function the dialect supplies for the reason
	// flagArgEscapes is one: the vocabulary is a shell's and this package
	// holds nobody's, so a runner nobody told refuses the letter by name
	// rather than inventing a word for it. See SetParameterTypeWord.
	parameterTypeWord func(ParameterAttributes) string
	// reevalDepth bounds the `(e)` expansion flag's re-reading, because a
	// value that names itself would otherwise recur forever: `v='${(e)v}'`
	// re-reads text that asks for the same expansion again. It is the same
	// bound arithValueOf keeps for `x=x`, and for the same reason — a stack
	// overflow is not a diagnostic anyone can act on, and in a library it
	// takes the embedder down with it. See interp/reevalflag.go.
	reevalDepth int
	// optionNamespace is the wider set of names `[[ -o name ]]` reads, for a
	// dialect that has one. Nil in a shell whose option names are its
	// `set -o` names and nothing more, which is where `[[ -o ]]` falls back
	// to those. Installed through SetOptionNamespace; see extend.go.
	//
	// Each of the three is handed the *running* runner, exactly as a builtin
	// is, and for the same reason: a subshell is a cloned runner and these
	// fields are copied into it, so one that closed over the runner it was
	// installed on would go on reading and writing that one from inside every
	// subshell. Measured before it did (#1855): `( setopt autocd; ... )` read
	// the parent's options through `[[ -o ]]`, and a `set -o` inside a
	// subshell moved the *parent's* option and left the subshell's alone.
	optionNamespace func(r *Runner, name string) (on, known bool)
	// optionListing and optionMover are the same namespace reached by
	// `set -o`: the rows a listing writes, and what moving one name does.
	// Nil in a shell whose `set -o` names are the substrate's own, which is
	// three of the four presets. Installed together through SetOptionTable;
	// see extend.go.
	optionListing func(r *Runner) []ListedOption
	optionMover   func(r *Runner, name string, on bool) (moved, known bool)
	// shellOptionMover and shellOptionListing are a *second* option
	// namespace, the one a dialect keeps beside `set -o` rather than inside
	// it: bash's `shopt` names, which overlap `set -o`'s in nothing. Nil in
	// the four shells that have no such table, where the invocation letter
	// that reaches it is not named either. Installed together through
	// SetShellOptionNamespace; see extend.go.
	//
	// Here rather than only in the builtin because an *invocation* reaches
	// this namespace before any builtin can: `bash -O checkhash -c …` moves
	// one of these names from the command line, and the front end has no
	// dialect to ask. Handed the running runner for the reason the three
	// fields above are (#1855).
	shellOptionMover   func(r *Runner, name string, on bool) int
	shellOptionListing func(r *Runner, reissuable bool)
	// optionLetterNames are the `set` option letters this dialect spells its
	// own way, mapped to the names in its namespace. Nil where every letter
	// the shell has is one the panel shares. Installed through
	// SetOptionLetterNames; see extend.go.
	optionLetterNames map[rune]string
	// aroundFunctionCalls is what a dialect saves and restores around every
	// function call, whatever that call turns out to do. Each entry is
	// handed the running runner as the body is entered and hands back the
	// restore, which the call's scope runs as it unwinds; a nil restore asks
	// for nothing.
	//
	// The moment is the point, and it is one AtFunctionReturn cannot give:
	// the shell whose options are function-scoped saves its whole option
	// table when the body *starts*, so an option moved before the line that
	// asks for the scoping is restored too — measured. A save that waited
	// for that line would put back the wrong table.
	//
	// The runner is a parameter rather than something the closure caught,
	// because a subshell is a clone: a save that wrote through a captured
	// pointer would snapshot and restore the shell it was registered in
	// rather than the one running the call.
	aroundFunctionCalls []func(*Runner) func()
	// lineBase is how far into the script the input being run starts.
	//
	// A command substitution's body is parsed on its own, so its positions
	// count from one — the body is what was parsed, and a parser that
	// claimed otherwise would be lying about the string it was handed. The
	// script the body was *written* in did not start there, and every shell
	// in the panel reports a command inside `$( … )` at its line in the
	// file. So the offset is carried by whatever is running the body, which
	// is the thing that knows where it came from.
	lineBase int
	// lineOrigin is what the route calls its *first* line, less one: nought
	// where the first line is line 1, and -1 where it is line 0.
	//
	// Separate from lineBase and applied on top of it, because the two are
	// different facts about the same number. lineBase is how far into a
	// script the text being run starts, and borrowed text resets it — an
	// `eval`'s program is line 1 of itself. This is the whole route's
	// numbering and nothing inside the route resets it, which is what the
	// measurement says: BusyBox ash under `-c` writes `line 0` for its first
	// line *and* `eval: line 0:` for the first line of an `eval`'s text, so
	// the offset survives into the borrowed text that reset the other one.
	//
	// See Diagnostics.CommandStringLinesFromZero, which is where the value
	// comes from and which carries the panel's table.
	lineOrigin int
	// killed is the command being run, for the notice that a signal ended
	// it — the only message that has to render a command rather than name
	// one. Innermost wins, which is what the shell prints: a command inside
	// a function is reported as itself and not as the call.
	killed syntax.Command
	// signals is the signal-trap machinery, behind a pointer because clone
	// copies a Runner by value and a mutex cannot be copied — the race
	// detector says so, and it is right: handlers belong to the process, not
	// to one runner among several sharing it.
	signals *signalState
	// statusBefore is `$?` as it was before the current statement, which one
	// dialect shows to a signal handler instead of the current one.
	statusBefore int
	// inExitTrap says the EXIT trap's body is running, and
	// exitTrapEntryStatus is the status the shell had when it began. A bare
	// `exit` in the body reports that rather than the body's own last
	// command, in every dialect but one — see
	// ExitInTrapReportsEarlierStatus.
	inExitTrap          bool
	exitTrapEntryStatus int

	// redirectForBuiltin is the builtin whose redirections are being opened,
	// which is not the same as the builtin speaking: one dialect names where
	// a builtin's own complaint happened one way and everything else
	// another, and counts a redirection opened *for* a builtin as the
	// builtin's — while the dialect that writes the builtin's name into the
	// location does not name it for that message.
	redirectForBuiltin string

	// redirForCommandWord is the command word whose redirections are being
	// opened, builtin or not. A `{name}>f` whose store is refused names it —
	// `exec: `10': not a valid identifier`, `read: …`, `/bin/echo: …` — and
	// a compound command, which has no word, names nothing. Measured
	// 2026-09-17 on bash 5.3.20; see Runner.setFdVar.
	redirForCommandWord string

	// fdVarSpeaker is the command word lent to a `{name}>` store's own
	// refusal while that store runs, since the store is reached from a
	// redirection rather than from inside a builtin and r.inBuiltin is empty
	// there. See Runner.setFdVar and Runner.refuseNamerefAim.
	fdVarSpeaker string

	// redirForOwnProcess says the redirections being opened belong to a
	// command this shell will run as a process of its own, which is where a
	// real shell has already forked and so where a here-document body's
	// expansion happens somewhere the shell cannot see. Owned by
	// applyRedirs, which saves and restores it. See heredocprocess.go.
	redirForOwnProcess bool

	// canceledChunk records that the chunk that just finished stopped
	// because the caller canceled it, which releaseCancellation has already
	// put the shell back on its feet after. Read by Run, to keep the EXIT
	// trap out of a run that was asked to stop.
	canceledChunk bool

	// cancelWatch is what a caller's cancellation is watched for, set once
	// per chunk in RunPart and shared with every clone of this Runner. Nil
	// unless the caller supplied a cancelable context, which is what keeps
	// the per-command check to a comparison; see cancel.go.
	cancelWatch *cancelWatch

	// abandonLine is the input line the statement that gave up was on, so the
	// rest of that *line* is given up with it. Measured: `r=2; echo one` on
	// one line prints nothing, and `r=2` with `echo one` on the line after it
	// runs the echo.
	//
	// Written from Runner.giveUpLine and never from r.line, which is the line
	// of whatever failed rather than the line the shell is running: inside a
	// function body the two are different numbers and it is the caller's that
	// is given up (#3503).
	abandonLine int

	// inputLine is the last line of the top-level statement being run — the
	// line a give-up anywhere inside it takes with it. Set by the two
	// statement loops that read a shell's input, RunPart and the borrowed
	// text in interp/source.go; zero when neither is on the stack.
	//
	// A function body is deliberately *not* one of those loops, which is the
	// whole point of the field: it runs in this runner with no loop of its
	// own, so a give-up inside it has to reach the caller's line to find out
	// how much to give up. See Runner.giveUpLine for the rows.
	inputLine int

	// assignFailed marks an assignment that was refused rather than made,
	// so the status it left is not zeroed by the assignment that follows
	// it. `readonly x=1; x=2` reports and carries on in one dialect, and
	// carrying on with a status of 0 said the refusal had not happened.
	assignFailed bool

	// linePin overrides the line a node reports, for the dialect that names
	// where a trap fired rather than where in its body a failure was.
	linePin int

	// suppressedHead says the next command dispatched is a function's body
	// and fires no compound head of its own. Set by callFuncAs and cleared
	// by the dispatch it was set for, so nothing deeper inherits it.
	suppressedHead bool

	// elementFired says the next command dispatched is a pipeline element
	// whose DEBUG firing the pipeline itself has already made — one per
	// simple element in the shell, or one for the whole pipeline — so the
	// element fires nothing of its own, head or simple. Set by runPipeline
	// for each element and cleared by the dispatch it was set for, so
	// commands *inside* the element go on firing normally, which is what the
	// panel does. See Semantics.DebugTrapPipelines.
	elementFired bool

	// inCommandTrap marks a DEBUG, ERR or RETURN body as running, which is
	// the one thing enterTrapBody needs that the name of the condition would
	// tell it: one dialect numbers those three bodies differently from every
	// other trap body — Semantics.CommandTrapBodyLine. Set by the site that
	// runs a pseudo-trap and put back there, so a body that fires another
	// trap leaves this as it found it.
	inCommandTrap bool

	// inTrapBody marks any trap body as running — a signal's, EXIT's, or one
	// of the three pseudo-conditions'. Where inCommandTrap says *which*
	// condition this body belongs to, this one says only that the shell is
	// inside one, which is the question the running-command record asks. Set
	// by enterTrapBody and put back by the restore it hands out, so a nesting
	// leaves it as it found it.
	inTrapBody bool

	// running is the command the shell is running, for a dialect with a
	// parameter naming it — see RunningCommand.
	running RunningCommand

	// programEnd is the line after the script's last, which is where the
	// shell has got to once the script has run — what one dialect calls the
	// EXIT trap's line.
	programEnd int

	// exitTrap is the body of `trap … EXIT`, or nil when none is set. Only
	// EXIT is stored: the other signals need delivery, which is a separate
	// piece, and `trap` refuses them rather than accepting one and never
	// firing it.
	exitTrap *string
	// trapDepth is the function nesting the EXIT trap was set at, which zsh
	// alone needs: there a trap set inside a function fires when the
	// function returns rather than when the script ends.
	trapDepth int

	// errTrap, debugTrap and returnTrap are the pseudo-conditions: not
	// signals, so nothing about them touches os/signal, and not EXIT, so
	// none of them waits for the script to end. nil is "not set" and a
	// pointer to "" is "ignored", the same three states a signal trap has.
	// See pseudotrap.go for when each fires.
	errTrap    *string
	debugTrap  *string
	returnTrap *string
	// errTrapSpelling is the word the ERR trap was last named by, which one
	// dialect echoes back in its listing rather than canonicalizing: there
	// the condition answers to both `ERR` and `ZERR`, and `trap … ZERR;
	// trap` writes ZERR where `trap … ERR; trap` writes ERR. Empty until
	// something names it, and the canonical name is what a listing prints
	// then. See Semantics.TrapErrConditionIsAlsoZERR.
	errTrapSpelling string
	// trapFuncs is the condition each `TRAP…` function stands for, keyed by
	// the canonical condition and holding the function's name. Only the
	// dialect that reads a function name as a trap ever puts anything here
	// — see trapfunction.go — and the entry is what makes the listing print
	// the function rather than the action text the firing runs.
	trapFuncs map[string]string
	// errTrapFrame and debugTrapFrame are the function frame each trap was
	// set in, zero for the top level. The dialect that does not carry
	// these traps into functions suppresses them only inside a function
	// frame that is not the one that set them — measured: a trap set
	// inside a function fires there and at the top level afterwards, and
	// not inside a sibling's body, though the sibling's *call* still
	// fires DEBUG, because the call is a command outside it.
	errTrapFrame   int
	debugTrapFrame int
	// returnTrapFrame is the serial of the call frame the RETURN trap was
	// set in, zero for the top level. A function fires the trap only when
	// its own body set it — a sibling called afterwards does not, which is
	// what the serial distinguishes that a depth cannot.
	returnTrapFrame int
	// frameSerial numbers every frame ever pushed, so two frames at the
	// same depth are still two frames.
	frameSerial int
	// inErrTrap, inDebugTrap and inReturnTrap guard each trap against
	// running itself: a failing command inside the ERR action fires
	// nothing, which is measured, and a DEBUG action that fired DEBUG
	// would never finish.
	inErrTrap    bool
	inDebugTrap  bool
	inReturnTrap bool
	// errTrapFired says the ERR trap has already fired for the failure the
	// status now reports. A group, a loop, an `if` and a `case` report the
	// status their last command left, and judging that status again is how
	// one failure two compounds deep ran a handler three times (#2793).
	//
	// Cleared at the head of every statement, which is what makes it mean
	// *this* failure rather than the last one: the enclosing compound's own
	// statement cleared it before its body ran, so what the body sets is
	// still there when the compound is judged. A second iteration of a loop
	// clears it again and fires again, which is measured — `for i in 1 2;
	// do false; done` writes two E lines in every column that has the
	// condition, and one per failure is the rule rather than one per loop.
	//
	// A subshell needs nothing here for the same reason: the child is a
	// copy, so its firing is recorded on the copy and the parent judges the
	// failing subshell command with its own flag still clear — which is
	// what keeps zsh's two E lines for `(false)`.
	errTrapFired bool
	// arithZeroLeft says the command that just finished was an `(( ))` whose
	// value was zero, which one dialect does not count as a failure. Cleared
	// on the way into every command and on the way out of a simple one, so a
	// call whose body ended in one reports a failure of its own. See
	// Semantics.ArithCommandZeroIsAFailure.
	arithZeroLeft bool
	// stmtSerial counts statements begun, so a compound can tell whether its
	// body ran one — see reportsItsBody for why that decides whether the
	// compound is judged at all.
	stmtSerial uint64
	// execSerial is stmtSerial as it stood when a command last reached the
	// exec path, and lastSimpleRanOnPath is whether the last simple command
	// dispatched was that command rather than one the shell ran itself — a
	// builtin, a function, an `eval`. One dialect judges a pipeline's last
	// element twice for the second kind and once for the first; see
	// PipelineJudgedAsItsLastElement.
	execSerial          uint64
	lastSimpleRanOnPath bool
	// pipeLast is what the last multi-element pipeline left for its own
	// statement to judge. See judgePipeline.
	pipeLast pipelineLast
	// returnSeenStatus is `$?` as it was when `return` began. The RETURN
	// trap's body sees this rather than the argument the `return` carried
	// — measured: `f(){ trap 'echo R=$?' RETURN; return 3; }; f` prints
	// R=0 and then reports 3.
	returnSeenStatus int
	// traps is this runner's own signal-trap table when it stands for a
	// subshell, nil at the top level, where the table is the process's and
	// lives on signalState. A subshell begins with only the parent's
	// ignored signals — see inheritTraps in trapsubshell.go.
	traps map[string]string
	// selfPending is what a subshell raised on *itself* and has not handled
	// yet — today, the SIGPIPE of its own failed write. It is this runner's
	// own list rather than the shared one because the two are different
	// events: a signal aimed at `$$` from inside a subshell is the shell at
	// the top's to handle, and one the subshell caused by writing into a
	// broken pipe never left the subshell at all. Sharing the list would
	// make the parent run its own handler for a signal it never had.
	//
	// No lock: a subshell's runner is a copy owned by the goroutine running
	// it, which is also the only thing that records or takes from this.
	selfPending []string
	// inheritedIgnored marks the entries in traps that arrived across the
	// subshell boundary rather than being set inside it, because one
	// dialect lists an ignore it set and not one it inherited.
	inheritedIgnored map[string]bool
	// trapSnapshot is the listing the parent shell would have shown when
	// this subshell began, kept for the dialects whose `trap` still shows
	// it there, and dropped the moment this runner modifies any trap.
	// trapContexts is the kind of each boundary the snapshot has crossed,
	// innermost last, so the axes that decide whether it survived are asked
	// when something lists rather than at every clone.
	trapSnapshot []savedTrap
	trapContexts []trapContext
	// errTrapInherited, debugTrapInherited and returnTrapInherited say the
	// pseudo-trap crossed a subshell boundary rather than being set on this
	// side of it. Whether an inherited one still fires is the dialect's
	// answer; one set inside the subshell fires everywhere, which is why
	// the flag exists rather than asking inSubshell.
	errTrapInherited    bool
	debugTrapInherited  bool
	returnTrapInherited bool
	// inSubshell marks a runner that stands for a subshell or a command
	// substitution. Measured: the EXIT trap fires once, at the end of the
	// main script, and not in either of those — so the copy must know it is
	// a copy.
	inSubshell bool
	// inCommandSubst narrows that to a command substitution, which one
	// dialect treats differently from a subshell written out: bash says
	// nothing about a command killed inside `$(…)` and does report one
	// killed inside `( … )`.
	inCommandSubst bool
	// scriptStop is the box a shell and every clone of it share, holding a
	// substitution parse failure that ends the script rather than the
	// subshell it was written in. See substitutionstop.go, and
	// Semantics.SubstitutionParseErrorEscapesASubshell for the panel.
	scriptStop *scriptStop
	// umask is the file-creation mask this shell holds, and maskKnown says
	// it holds one. A mask is process state and a body of this shell is not
	// a process, so it is kept here instead and applied by hand at the two
	// points the kernel reads one — the mode an open asks for, and the mask
	// a child inherits at the fork. Copied by clone like any other field,
	// which is what makes a body's mask the body's own. See umaskscope.go.
	umask     int
	maskKnown bool
	// traceWait and traceDone order the trace lines of a pipeline without
	// ordering the pipeline itself: an element waits for the one before it
	// to have printed, then prints, then releases the next. Only the
	// printing is serialized.
	traceWait <-chan struct{}
	traceDone chan struct{}
	traceOnce *sync.Once
	// xtrace is `set -x`: every simple command is printed before it runs.
	xtrace bool
	// condTrace is the `[[ … ]]` being traced, or nil. See condTrace.
	condTrace *condTrace
	// nounset is `set -u`: expanding an unset parameter is an error.
	nounset bool
	// errexit is `set -e`: a command that fails ends the script.
	errexit bool
	// tested counts the contexts where a command's status is being *used*
	// rather than checked for failure — an `if` condition, a non-final
	// operand of `&&`, the operand of `!`. `set -e` does not fire while it
	// is above zero.
	//
	// A counter on the runner rather than a parameter, because the
	// exemption is inherited: a function called from an `if` condition has
	// it suppressed inside its body too, all the way down. That is measured,
	// unanimous across the panel, and the part of `set -e` most
	// implementations get wrong.
	tested int
	// noclobber is `set -C`: a plain `>` will not truncate an existing file.
	noclobber bool
	// noglob is `set -f`: pathname expansion does not happen. Only pathname
	// expansion — a `case` pattern still matches, because that is matching
	// and not expansion.
	noglob bool
	// noBraceExpand is the `braceexpand` option turned *off* — `set +B` in
	// the two shells that have the letter, `setopt ignorebraces` in the one
	// that spells it the other way round. `{a,b}` is then the word it was
	// written as, everywhere a brace would otherwise be read: a command's
	// arguments and a redirection's target alike.
	//
	// Stored as the negative so that the zero value is a shell that expands
	// braces, which is what every shell that has them does at startup. It is
	// a run-time switch and Semantics.BraceExpansion is the dialect's answer
	// to whether this shell has braces at all; a shell whose braces do not
	// expand never reads this, and a dialect that does not declare the option
	// has no way to move it (#1856).
	noBraceExpand bool

	// globSuspended is pathname expansion switched off for one nested
	// expansion, from the inside: the contexts where a word substitutes as
	// *text* rather than as a pattern — an arithmetic operand, and a `:-`
	// word in a position that has no matching.
	//
	// Separate from noglob rather than borrowing it, because that one is
	// observable: `$-` reports the option, and a runner that flipped it for
	// the length of an expansion answered for it while it was flipped.
	globSuspended bool
	// globUnsorted says the expansion running right now asked for the order
	// the directory itself gave, which is one value of the dialect's sort
	// parameter — see Semantics.SortOrderVariable. It reaches the listing
	// rather than the result because a sort destroys that order and nothing
	// can recover it: a walk that sorted each level and then "unsorted" the
	// answer would be inventing one.
	//
	// Held on the Runner for the length of one glob, the way globSuspended
	// is, because the listing is several calls below the walk and threading
	// a flag through every one of them would put the question to callers
	// that have nothing to do with it.
	globUnsorted bool

	// matchOptions is the run-time pattern behaviors a dialect's builtin has
	// switched on, one bit per MatchOption. A plain value so a subshell's
	// clone carries the state and its changes stay its own.
	//
	// Its width is matchOptionBits, which is checked where the options are
	// declared: it was a uint8 holding exactly eight of them, so the ninth
	// shifted off the end and read as off however it was set. Nothing failed
	// — the dialect turned it on, `shopt -p` reported it off, and the
	// behavior behind it never ran (#1862).
	matchOptions matchOptionSet

	// ignoredNamesLive says something has *assigned* the parameter named by
	// Semantics.IgnoredNamesVariable, which is what makes its patterns reach
	// a pathname expansion at all.
	//
	// A state rather than a lookup, because the parameter's own value does
	// not say it: a value arriving from the environment is carried, is
	// readable, and is not read — measured, `env GLOBIGNORE='*.txt' bash -c
	// 'echo *'` lists the `.txt` files. Assigning the parameter its own
	// value is what starts it, and unsetting it is what stops it. A plain
	// value, so a subshell carries the state its parent had and its own
	// assignments stay its own.
	ignoredNamesLive bool

	// pipefail is `set -o pipefail`: a pipeline reports its last *failing*
	// element instead of its last one. Not every dialect has the option, so
	// the field is only ever set through an axis.
	pipefail bool

	// pipefailRaised says the statement just run reported a failure that only
	// pipefail produced — its last element succeeded and an earlier one did
	// not. Kept because one shell's `set -e` does not count that as a failure
	// worth stopping for, which is a question only askable about this exact
	// case.
	//
	// Set for every pipeline, not only failing ones, so a stale yes cannot
	// outlive the statement that earned it.
	pipefailRaised bool
	// declaring names commands whose `name=value` arguments are assignments,
	// beyond the ones the core already knows. A dialect adds its own.
	declaring map[string]bool
	// precommands names the words that may stand in front of a command and
	// are read before its words are matched against the filesystem. A
	// dialect adds its own and the core has none — see precommand.go.
	precommands map[string]PrecommandModifier
	// pipeStatus is what the last pipeline's elements reported, and
	// pipeStatusName is what the dialect calls it. The record is only kept
	// when a dialect has named it, because nothing else can read it.
	pipeStatus     []int
	pipeStatusName string
	// regexMatchName is what the dialect calls the record of what the last
	// `=~` captured. With no name, nothing is recorded — see regexmatch.go.
	regexMatchName string
	// regexCaptureReport is whether a `=~` also reports what it matched
	// through the parameters a reporting pattern fills. Off unless a dialect
	// asks — see regexmatch.go.
	regexCaptureReport bool
	// shellOptsName is what the dialect calls the variable holding the long
	// names of the `set -o` options that are on. With no name there is no
	// such variable and nothing is seeded from the environment — see
	// shellopts.go.
	shellOptsName string
	// optionLists are every produced, readonly variable bound to an option
	// namespace, the core's included. A dialect may have a second table the
	// core knows nothing about; see Runner.SetOptionList.
	optionLists []optionList
	// readonly names refuse assignment.
	readonly map[string]bool
	// unsetRefused names refuse `unset` and nothing else — see
	// Runner.RefuseUnset.
	unsetRefused map[string]bool
	// freezing is the names the declaration now running is assigning to as
	// operands, and freezeAfter is the ones whose `-r` is waiting for those
	// assignments to land. `declare -ar A=(x y)` carries the value and the
	// attribute in one command, so the attribute cannot be what refuses the
	// value — see markReadonly and applyDeferredFreeze.
	freezing    map[string]bool
	freezeAfter []string
	// literalOperands is the subset of those names whose operand is an
	// *array literal* rather than a plain word. The declaration builtin
	// cannot see the shape for itself — the parser keeps the assignment
	// apart and hands the utility the bare name — and one dialect refuses a
	// type letter over exactly that shape. See
	// Semantics.TypeLetterAndAnArrayLiteralIsAnInconsistentType.
	literalOperands map[string]bool
	// compoundOperands is the narrower subset whose operand is a *compound*
	// literal — `c=(a=1)`, whose items are assignments — rather than an
	// element list. Both spellings are parenthesized and both answer
	// syntax.Assign.IsArray, so literalOperands above cannot tell them
	// apart, and one builtin's refusal turns on exactly the difference: a
	// compound may not be exported and an index array may. See
	// Runner.exportRefusesACompound.
	compoundOperands map[string]bool
	// retypingFrozen is the one name a frozen-scalar retype is under way for.
	// See the method of the same name for why it is a field.
	retypingFrozen string
	// badNameWordedAs is the builtin whose bad-name *sentence* a declaration
	// now in progress borrows, while still naming itself. Empty is the
	// ordinary case. See Diagnostics.ExportLetterTakesExportsBadName.
	badNameWordedAs string
	// indexedLetterHere is the subset of those names whose declaration also
	// carried the *indexed* container letter — `typeset -a a=([5]=q)` and not
	// `typeset -a a` followed by the assignment on the next line.
	//
	// It is the letter's presence on the same command that matters and not the
	// attribute it leaves behind, which is measurable rather than a guess:
	// ksh93u+ answers `typeset -A a=([5]=q)` for `typeset -a a; a=([5]=q)`,
	// for `a=(x y); a=([5]=q)` and for `a[0]=x; a=([5]=q)` — every route
	// where the name is already an indexed array — and `typeset -a a=([5]=q)`
	// for the one command carrying both. `typeset -a a=([1+1]=q)` is
	// `typeset -a a=([2]=q)` there, so the letter puts the subscript back to
	// being an *expression* rather than only changing the letter listed. See
	// Runner.literalSubscriptIsAKey.
	indexedLetterHere map[string]bool
	// tableLetterHere is the same record for the *table* letter — `typeset -A
	// m=(alpha one)` and not `typeset -A m` followed by the assignment on the
	// next line.
	//
	// One dialect needs the distinction and nothing else does: a literal of
	// bare words onto a table is refused there where the letter is on the same
	// command, and *converts* the name to an index array where it is not.
	// Measured 2026-09-13, with `typeset -A m=([a]=1)` in front of it,
	// `typeset -A m=(x y)` is `cannot append index array to associative array
	// m` and `m=(x y)` lists `typeset -a m=(x y)`. See
	// Semantics.BareElementsInATableLiteralEndTheScript.
	tableLetterHere map[string]bool
	// integer names evaluate what is assigned to them: with the attribute,
	// `n=5+2` stores 7 rather than the four characters. It is a property of
	// the name and not of the assignment, which is why it is recorded here.
	integer map[string]bool
	// integerBase is the base an integer name renders in — `typeset -i16 h`
	// makes 255 read back as `16#ff`. A property of the name like the
	// attribute itself, and consulted by every store afterwards rather than
	// by the declaration that named it. Absent, or 10, or a base the dialect
	// has no digits for, all mean plain decimal.
	integerBase map[string]int
	// floatPrecision is how many decimal places a float name renders in —
	// `typeset -F 3 x=1.5` makes it read back as `1.500`. Presence is the
	// attribute itself, the way the table is for an associative array: a
	// name in here is a float and a name that is not is not, so there is no
	// second map saying which. The value is the precision the letter named,
	// and zero is the letter with no number after it — which is ten, the
	// default both shells with the attribute print, rather than no places at
	// all. Measured 2026-09-07: `typeset -F x=1.5` is `1.5000000000`.
	floatPrecision map[string]int
	// floatExponent names the float parameters whose attribute came from the
	// `E` letter rather than from `F` — a **format** and not a width. See
	// interp/floatformat.go, where the two renderings are.
	floatExponent map[string]bool
	// fieldWidth is the width attribute a name carries — `typeset -L 5 s`
	// and its two neighbors. Presence is the attribute, the way it is for
	// floatPrecision; see fieldwidth.go, which holds the measurements and
	// the rule.
	fieldWidth map[string]fieldWidth
	// lowered and uppered are the case attributes — `declare -l` and `-u` —
	// which fold what is assigned to the name, the same shape integer has:
	// a property of the name that changes what a later assignment means.
	lowered map[string]bool
	uppered map[string]bool
	// hidden names keep their value out of every listing that would write
	// one — `typeset -H`. It is a property of the name like the others, but
	// the only one that changes nothing about a read: `$h` and `${h[k]}`
	// answer what they always did, and only a listing is any the wiser. One
	// shell in the panel spells the letter with this meaning; another spells
	// the same letter for a different attribute that does *not* hide, which
	// is why the dialect decides who may set it — see Semantics.DeclareOptions.
	hidden map[string]bool
	// localMarked names a parameter a *call* opened, which the shell reports
	// as local without there being a function scope holding it. It is what
	// localCell cannot see: a produced parameter is registered and taken away
	// again by the code running the call rather than by entering and leaving
	// a scope, so the fact the shell states about it has to be stated too.
	// See Runner.MarkLocal.
	localMarked map[string]bool
	// unique names keep only the first occurrence of each element —
	// `typeset -U`. A property of the name like the others, consulted by
	// every write rather than by the one declaration that set it, which is
	// what makes `path=( new "${path[@]}" )` drop the copy of `new` that
	// was already there. One shell in the panel spells the letter: bash
	// refuses it and ksh93 does not have it, so the dialect decides who may
	// set it — see Semantics.DeclareOptions.
	unique map[string]bool
	// nameref names the parameters that are **references to another
	// parameter**: the entry is the name — or the name and subscript — that
	// every read, write and `unset` through this one really lands on. See
	// interp/nameref.go, which is the whole of the mechanism.
	//
	// A table rather than a value on a cell for the reason every other
	// attribute here is one: the name is what carries it, so a shadowed
	// binding keeps its own and gets the outer one back on return —
	// nameAttributes carries it through localattributes.go with the rest.
	nameref map[string]string
	// hideInScope names the parameters carrying the hide-in-scope attribute
	// — `typeset -h`, and `typeset +h` to take it off. It is what makes a
	// local declaration of a name that is half of a tie an ordinary
	// parameter rather than the special one, and unlike every other
	// attribute here it decides nothing on its own: it decides only where a
	// scope has displaced the name. See hideinscope.go.
	hideInScope map[string]bool
	// tied holds the ties `typeset -T` made — see tiedscalar.go — under
	// both of each tie's names, so either half finds it.
	tied map[string]tie
	// declaredTypes holds the type names `typeset -T` registered, in the
	// order they were first written, for the dialect whose `T` letter names
	// a type — see interp/declaretype.go. Names and not definitions: a type
	// is a compound value this engine has no representation for (#2620), so
	// what is kept is the one thing the bare `-T` listing can write back.
	declaredTypes []string
	// mirroring says a tie is already writing the other half, which is what
	// keeps the two mirrors from calling each other forever.
	mirroring bool
	// lastSubst is the pattern and replacement `${x:s/l/r/}` last used, which
	// an empty pattern and `${x:&}` both reach for. Shell-wide rather than
	// per parameter — measured: a substitution made on one name is the one an
	// empty pattern on another name reuses, on the same line. A scalar, so a
	// subshell gets its own copy of it with the parent's contents the way
	// every other scalar on this struct does.
	lastSubst lastSubstitution
	// funcs holds defined functions.
	funcs map[string]*syntax.FuncDecl
	// disciplined names every variable a discipline function has ever been
	// defined for, so that the read path can find out there is nothing to
	// run with one map lookup instead of building `name.get` and asking the
	// function table for it. A hint rather than the truth: `unset -f g.get`
	// takes the hook away by taking the function away, and the name left
	// behind here only costs the lookup that then finds nothing. See
	// interp/discipline.go.
	disciplined map[string]bool
	// disciplineRunning is the hooks on the stack right now, by their full
	// `name.event` spelling — the guard that keeps a `.get` which reads its
	// own variable from being a loop. Per event and not per variable,
	// measured: a `.get` that *assigns* its variable does fire that
	// variable's `.set`.
	disciplineRunning map[string]bool
	// disciplineStatus is what the last `.set` or `.append` hook to run
	// returned, and whether one ran at all since the flag was cleared.
	//
	// It exists because a *plain* assignment answers with the hook's status
	// — measured on ksh93u+ 2012-08-01, 2026-09-16, `function s.set { return
	// 5; }; s=1` leaves 5 in `$?` while the assignment itself still happens
	// — and the store that runs the hook is several frames below the command
	// that has to report it. `typeset s=1`, `read s` and `for s in …` all
	// fire the same hook and all report 0 there, which is why the flag is
	// read by the bare-assignment path alone and not by the store.
	//
	// The last hook wins: `a=1 b=2` with a hook on each reports b's, and
	// `b=1 a=2` reports a's. A scalar, so a subshell gets its own copy.
	disciplineStatus    int
	disciplineStatusSet bool
	// The frame a script has pointed the shell's location parameters at, the
	// depth it did so from, and a name written over the top of it. See
	// interp/callstack.go, where all four are documented together.
	selectedFrame     int
	selectedFrameName string
	frameSelected     bool
	frameNamed        bool
	frameSelectedAt   int
	// mathFuncs holds the `functions -M` registrations: names arithmetic may
	// call, each naming a shell function to run. mathOrder is the order they
	// arrived in, because the listing walks it backwards. See mathfunc.go.
	mathFuncs map[string]mathFunc
	mathOrder []string
	// lastArith is the value of the last arithmetic expression this shell
	// evaluated, anywhere.
	//
	// It exists for one caller — a math function's return value is this and
	// not `REPLY`, which is measured and set out in mathfunc.go — and it is
	// the shell's rather than a call's on purpose: an evaluation from before
	// a registration was made is still what an implementation that evaluates
	// nothing hands back.
	lastArith arithNum
	// arithOutput is the output format the expression being evaluated carries
	// — `$(( [#16] 255 ))` — and nil for an expression with none, which is
	// every expression in every dialect without the construct.
	//
	// Held here rather than passed down because it has to reach a place the
	// value does not: an assignment *inside* the expression stores the
	// formatted text, so `x=5; (( x = [#16] 255 ))` leaves x holding `16#FF`.
	// Set while the expression under the specifier is evaluated and put back
	// afterwards, so nothing leaks into the next evaluation — measured,
	// `echo $(( [#16] 255 )); echo $(( 255 ))` is `16#FF` then `255`.
	arithOutput *syntax.ArithOutput
	// arithValueDepth counts how far inside a *stored value being read again
	// as an expression* this runner is: `x=y; y=5; $((x+1))` is one level in
	// while `y` is being looked up, and zero again once it has been.
	//
	// A field rather than an argument because the re-read goes back through
	// the whole evaluator — a value is an expression and not just a name, so
	// every operand in it is reached through the ordinary walk, which carries
	// no depth of its own. Two callers need it: the bound that stops `x=x`
	// from recurring forever, and Semantics.ArithRecursedNameMustBeSet, which
	// asks whether a name was reached through a value or written in the
	// expression itself.
	arithValueDepth int
	// arithSubscriptDepth counts how many array subscripts are being
	// evaluated around the expression now being read, so that a name inside
	// the brackets can be answered differently from the same name outside
	// them. One dialect refuses an unset one there; see
	// Semantics.ArithSubscriptNameMustBeSet.
	arithSubscriptDepth int

	// arithValueTopName is the name the expression under evaluation named
	// itself, as against the ones reached through its value. Only the
	// recursion bound reads it, and only one dialect blames that name.
	arithValueTopName string
	// indirection counts how many levels of *text being read again* this
	// runner is inside — an `eval`, a sourced file, a command substitution,
	// or a trap body other than EXIT's. One dialect repeats its trace
	// prefix's first character once per level and the rest do nothing with
	// it; see Runner.tracePrefixDepth and
	// Diagnostics.TracePrefixRepeatsAtIndirection.
	//
	// Not r.depth, which counts function calls: measured, a function call and
	// a subshell add no level and the four above each add one, so they are
	// different questions about different things. The trap body is the one
	// that is not a *route into* re-read text but text a condition reaches
	// for, and it is counted here because it measures the same way — see
	// Runner.enterTrapBody for the probes, and for why EXIT is exempt.
	indirection int
	// borrowedFiles counts how deep inside sourced *files* this runner is,
	// which decides whether a prompt's wording applies to what it is running.
	//
	// A file is a file however it was reached: measured 2026-09-11, zsh 5.9.2
	// sourcing a file at a prompt keeps the file's name and the line in every
	// diagnostic from it, where a line typed at that prompt carries neither.
	// Text handed to `eval` is not a file and is not counted here, which is
	// measured the same way — `eval "cd /nope"` at a prompt is located like
	// the prompt in every column that locates it at all. See Runner.AtPrompt
	// and Runner.diag (#2024).
	//
	// Not r.indirection, which counts every kind of re-read including `eval`
	// and a command substitution.
	borrowedFiles int
	// evalTextFloor is one more than the number of frames that stood when
	// the innermost `eval` began reading its text, and zero where no `eval`
	// is reading any. It is what tells a line the *evaluated text* holds
	// from a line something the text called holds, for the dialect that
	// gives that text a location of its own — see
	// Diagnostics.LocationNamesTheEvalText.
	//
	// A depth rather than a flag, and a frame count rather than a counter of
	// its own, because `eval` pushes no frame: everything it calls does, so
	// the frames standing above the mark are exactly the function bodies and
	// sourced files between the text and the failure. A function called from
	// evaluated text is named as the function, and the mark comes back into
	// force when it returns, without anything having to be saved and
	// restored at each call.
	//
	// Saved and restored around the read rather than pushed onto a stack:
	// only the innermost is ever asked, and `eval` inside `eval` overwrites
	// the outer mark with an equal one.
	evalTextFloor int
	// borrowed is the stack of text the shell is reading from somewhere
	// other than the file it was handed: a file `.` read, or the string
	// `eval` was given, innermost last.
	//
	// It exists because one dialect names that text in a *run-time*
	// diagnostic and not only in a parse failure — `dash: 3: ./p.sh: NOPE:
	// parameter not set` — and by the time a run-time diagnostic is written
	// the `sourced` value that knew the name is several calls up the Go
	// stack. See Runner.borrowedAtLocation (#1128).
	//
	// A stack rather than a saved-and-restored single value, unlike
	// evalTextFloor above: that one is asked only about the innermost, and
	// this one is the chain a dialect renders. Nothing renders more than the
	// innermost yet, and the stack is still what is kept, because the two
	// answers differ the moment a sourced file sources another and a
	// one-deep record would have to be rebuilt to say so.
	borrowed []borrowedText
	// depth bounds function recursion, because a shell script can recurse
	// and a stack overflow is not a diagnostic anyone can act on.
	depth int
	// funcFloor is the depth this shell body began at, so that "am I inside a
	// function call" can be asked about *this* shell rather than about the
	// process. clone raises it, because a subshell is a shell of its own: it
	// inherits the caller's frames as state but is not running inside them.
	//
	// It exists for one measured question — see tryclause.go — and the
	// measurement is what says a plain copy of depth would be wrong.
	funcFloor int
	// actionIDs numbers this session's actions — see actionid.go. A pointer
	// so that clone shares it rather than copying it: two subshells with
	// counters of their own would hand two different actions the same id, and
	// an id that is not unique is worse than none, because it looks joinable.
	actionIDs *atomic.Uint64
}

// maxDepth bounds nested function calls.
const maxDepth = 256

// clone copies the state for a subshell, so nothing it does escapes.
func (r *Runner) clone() *Runner {
	// Before the copy, so parent and subshell share one box for the pipe
	// directory rather than each making its own. See procSubHomeBox: a
	// substitution made only inside a subshell used to leave a directory the
	// parent could not clean up because it had never been told about it.
	r.procSubHomeBox()
	// And before the copy for the same reason, which is the whole of why the
	// box is made here rather than where it is written: a subshell that made
	// its own would record a stop nothing above it could ever read. Made on
	// every clone rather than lazily, so that a script with no subshell in it
	// carries a nil and pays one nil test per command. See scriptStop.
	if r.scriptStop == nil {
		r.scriptStop = &scriptStop{}
	}
	c := *r
	c.inSubshell = true
	// A subshell body is not running inside the frames the copy inherited.
	c.funcFloor = c.depth
	// A pending process substitution belongs to the command being built in
	// the runner that made it, not to a subshell cloned while it was being
	// built. Carrying them over meant the second `<(…)` of a command cloned
	// the first one's descriptor and then closed it on its way out, so
	// `cat <(echo one) <(echo two)` reported a bad file descriptor for the
	// half it had already opened.
	c.procSubs = nil
	// But the descriptors are still the subshell's to hand on, as a fork's
	// copy of them would be: `f() { cat "$1" | cat; }; f <(echo x)` is `x` in
	// bash 5.3.20. Handed on, never closed — see enclosingProcSubs.
	c.enclosingProcSubs = append(slices.Clone(r.enclosingProcSubs), r.procSubs...)
	// And the same for the ones the shell is still holding a descriptor onto.
	// They belong to whichever shell opened them: a subshell must not close
	// its parent's pipe on the way out, nor wait for a body the parent is
	// going to wait for itself. `( exec > >(cat) )` gets its own entry here
	// and joins it at its own end — see endHeldProcSubs.
	c.heldProcSubs = nil
	// The list is made here, on the *parent*, rather than where a body is
	// first deferred: made lazily on whoever needed it first, a pipeline
	// element would have made one of its own and owned it, and joining it at
	// the element's end is the answer this is not.
	if r.bodies == nil {
		r.bodies, r.ownsBodies = &pendingBodies{}, true
	}
	c.bodies = r.bodies
	// bodies is deliberately *not* cleared: a subshell and a pipeline element
	// write into the stream their caller gave them, so an unwaited-for body
	// they started is the caller's to join. A command substitution takes a
	// list of its own — see Runner.collectBodies, which is the one caller
	// that asks for one.
	c.ownsBodies = false
	// Every table the clone must own rather than share. One list, in one
	// place, with a test that fails when a new one is added — see
	// clonetables.go for why that is a check rather than a convention.
	c.ownTables(r)
	// A subshell begins with the parent's handled traps back at their
	// defaults — see trapsubshell.go for what crosses and what is only still
	// visible. After ownTables, which is what leaves it free to build the
	// three tables it owns from scratch.
	c.inheritTraps(r)
	return &c
}

// withRedirs applies a compound command's redirections around its body. Every
// compound node carries its own list because a redirection on one applies to
// everything inside it.
func (r *Runner) withRedirs(ctx context.Context, rs []*syntax.Redirect, body func() error) error {
	// A compound command's body runs in this shell, so its here-documents
	// expand here — measured, `while read x; do :; done <<END` with a body
	// of `$(( n++ ))` leaves the increment behind in all four shells.
	closers, err := r.applyRedirs(ctx, rs, true, false)
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	if err != nil {
		return err
	}
	if r.redirErr {
		return nil
	}
	return body()
}

// Verbose reports `set -v`, for the front end that holds the raw lines.
func (r *Runner) Verbose() bool { return r.verbose }

// ErrExit reports `set -e`, for the front end that has one thing to decide by
// it: a line the parser refused ends the script under the option and only the
// line without it. See syntax.File.Refused.
func (r *Runner) ErrExit() bool { return r.errexit }

// NoExec reports `set -n`: the program is read and never run.
//
// Exported for the front end, which has one thing to decide by it that the
// interpreter cannot — whether to say a remark the parse produced. One shell
// remarks on every backquote substitution it reads and does so only when it
// is not going to execute, so the answer belongs to the place that both holds
// the remarks and knows the option. See interp.RemarkOnlyWhenNotRunning.
func (r *Runner) NoExec() bool { return r.noexec }

// ExitStatus reports the status of the last command.
func (r *Runner) ExitStatus() int { return r.status }

// SetExitStatus sets the status of the last command, which is what `$?`
// reports and what a shell that stops here exits with.
//
// It exists for the things a front end knows about a run that the runner does
// not. A panic caught at a run boundary is one — interp panics on an internal
// bug because it is a library, and the shell around it decides the session
// survives — and the line that panicked has to leave a status behind, or `$?`
// goes on answering for the command before it and `&&` runs on as though
// nothing happened. A line that did not *parse* is the other: it never reaches
// a Runner at all, and where the dialect reads on past it the status has to
// survive into whatever runs next, or out of the shell where nothing does.
func (r *Runner) SetExitStatus(status int) { r.status = status }

// The three streams, resolved. A nil one is empty rather than the process's,
// which the field comments give the reasoning for; these are where that is
// implemented, and the reason they are functions at all.

func (r *Runner) stdout() io.Writer {
	if r.Stdout == nil {
		return io.Discard
	}
	return r.Stdout
}

// stdin is the shell's input, empty where the caller supplied none — the
// reading half of what stdout and stderr do.
func (r *Runner) stdin() io.Reader {
	if r.Stdin == nil {
		return emptyReader{}
	}
	return r.Stdin
}

func (r *Runner) stderr() io.Writer {
	if r.Stderr == nil {
		return io.Discard
	}
	return r.Stderr
}

// emptyReader is a stream with nothing in it, which is what a nil Stdin means.
//
// A type of its own rather than a shared strings.Reader, because these streams
// are read from more than one goroutine — a background job and each half of a
// pipeline read on their own — and a reader with a position in it, however
// certainly at its end, is state two of them would share. This one has none.
type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

// errf writes a diagnostic to the shell's error stream.
//
// The write error is discarded deliberately: this is already the error path,
// there is nowhere better to report a failure to report, and a shell whose
// stderr is closed should still run the command.
// printf writes to the shell's output stream.
//
// A failed write is recorded rather than returned, because none of the
// builtins writing through here could do anything with it at the site: the
// dispatcher folds it into the command's status once the builtin returns.
func (r *Runner) printf(format string, args ...any) {
	if _, err := fmt.Fprintf(r.stdout(), format, args...); err != nil {
		r.writeFailed = err
	}
}

func (r *Runner) errf(format string, args ...any) {
	if r.printfOut != nil && r.printfOut.hold(fmt.Sprintf(format, args...)) {
		// A `printf` conversion is being formatted, and what it says waits
		// for it: the output in front of it is released first, or the pass
		// is taken back, and only then does this go out. See printfWriter.
		return
	}
	_, _ = fmt.Fprintf(r.stderr(), format, args...)
}

// diagf writes a diagnostic with the dialect's own prefix.
//
// Every message goes through here rather than spelling "sh: " itself, because
// the prefix is the dialect's answer and not this package's: dash, bash, ksh93
// and zsh each name the location differently, and one of them names it not at
// all.
func (r *Runner) diagf(format string, args ...any) {
	r.errf("%s", r.diagLine(format, args...))
}

// diagLine is the text diagf would write, without choosing a stream for it.
//
// Split out because one message in the panel is not always a diagnostic: the
// "not found" `type` and `command -V` report goes to standard output in two of
// the four shells — see Diagnostics.TypeNotFoundOnStdout — and the prefix rule
// is the same wherever it lands.
func (r *Runner) diagLine(format string, args ...any) string {
	return r.diagLineNamed("", format, args...)
}

// diagLineNamed is diagLine with a construct named in the location. See
// Runner.locationPrefixNamed.
func (r *Runner) diagLineNamed(construct, format string, args ...any) string {
	msg := fmt.Sprintf(format, args...)
	if name := r.speaking(); name != "" && r.diag().BuiltinNamesTheShellAlone[name] {
		// A builtin that reports as the shell itself rather than as a line of
		// a script. See Diagnostics.BuiltinNamesTheShellAlone: the name is
		// the basename the shell was invoked by, on every route, and the
		// location the rest of this function would build is written by that
		// builtin nowhere.
		return path.Base(r.invokedAs()) + ": " + strings.TrimPrefix(msg, name+": ")
	}
	if r.speaker != "" && r.inBuiltin != "" && r.inBuiltin != r.speaker {
		// A builtin the dialect's own function called. The complaint reaches
		// the script as that function's — `pushd /nope` is `pushd: /nope: …`
		// in every shell that has the builtin — so the name the message opens
		// with is *replaced* rather than nested. That `cd` did the work is an
		// implementation detail of this prelude and of no other shell's.
		if rest, cut := strings.CutPrefix(msg, r.inBuiltin+": "); cut {
			msg = r.speaker + ": " + rest
		}
	}
	if name := r.speaking(); name != "" && r.diag().NamesBuiltinInLocation {
		// The builtin's name belongs in exactly one place. Most dialects put it
		// at the front of the message — `cd: /x: no such directory` — and this
		// one puts it in the location instead, so a message that also opens with
		// it would say it twice: `zsh:cd:1: cd: /x: …`.
		//
		// Stripping it here rather than at each of the sixteen sites that write
		// one keeps the rule in a single place, and keeps those messages readable
		// as the sentence every other dialect prints.
		msg = strings.TrimPrefix(msg, name+": ")
	}
	return r.locationPrefixNamed(construct) + msg
}

// lineOf is where a node is in the script, rather than in the string that was
// parsed to reach it. The two differ only inside a command substitution, and
// they differ by however far into the script the substitution was written.
func (r *Runner) lineOf(p syntax.Pos) int {
	if r.linePin != 0 {
		// One dialect names where a trap fired for every line of its body,
		// so the node's own line says nothing.
		return r.linePin
	}
	return int(p.Line) + r.lineBase + r.lineOrigin
}

// builtinIsSpeaking reports whether this diagnostic belongs to a builtin,
// which includes a redirection opened for one. Separate from naming the
// builtin, because the two dialects that ask want different answers for a
// failed redirection: ksh93 counts it as the builtin's and zsh does not.
func (r *Runner) builtinIsSpeaking() bool {
	return r.speaking() != "" || r.redirectForBuiltin != ""
}

// locationIsInsideEvalText reports whether the line a diagnostic is about was
// read from text `eval` is running rather than from a file or a function body
// below it. See Runner.evalTextFloor.
//
// It counts from the same depth [Runner.locationFile] and
// [Runner.locationIsInsideAFunctionBody] name theirs at, so a message located
// at the call it came from is asked about the frame it is located in and the
// three cannot disagree.
func (r *Runner) locationIsInsideEvalText() bool {
	return r.evalTextFloor > 0 && r.evalTextFloor-1 == len(r.frames)-r.outsideCall
}

// locationNameAndLine is the pair a location is written from: the name that
// stands where the shell's own would, and the line counted the way that name
// counts it.
//
// Three places a line can be read from, in the order they win. Text `eval`
// is running is named for itself, over both the others. A function body is
// named by the function and the line is the offset from the line the
// function was written on. Everything else is the shell's name, or the file
// the failing line was read from where the dialect names one — the sourced
// file while it runs, and the defining file inside a function called later.
//
// Which of the three applies is the innermost *frame*'s to say and not
// r.inFunc's — see [Runner.locationIsInsideAFunctionBody]. A function that
// sources a file is still the innermost function while the file runs, so
// asking the name gave that dialect's function rule to a line the function
// never contained (#2037).
//
// **One reader, for the diagnostic and for the trace prefix alike**, because
// in the dialect that has both they are the same location. Measured
// 2026-09-12 on zsh 5.9.2: a trace from the second line of a function is
// `+q:2>` where a diagnostic from it is `q:2:`, and a line read from a file
// that function sourced is `+./x_inc:1>` where a second copy of the rule had
// kept the trace on the function. That copy is what wrote `+q:0>` for every
// line of every function and never named a sourced file at all (#2134).
//
// functionCounts is the caller saying whether the function rule applies. A
// diagnostic a *builtin* is speaking is located at the call and never as a
// function — the dialect that names a function in place of a file names the
// builtin there instead, because to the script there is no function to name
// — and no builtin is ever speaking in a trace prefix, so the one caller asks
// and the other does not.
//
// inBody reports that the function rule is what answered, which the two
// callers need for opposite reasons: only that offset can be nought, and only
// the diagnostic leaves a nought out.
func (r *Runner) locationNameAndLine(functionCounts bool) (name string, line int, inBody bool) {
	d := r.diag()
	// lineNow and not line: a `case` subject is expanded before the line
	// advances in one dialect, and a complaint it makes carries that line.
	at := r.lineNow()
	if d.LocationNamesTheEvalText && d.EvalSourceName != "" && r.locationIsInsideEvalText() {
		return d.EvalSourceName, at, false
	}
	if functionCounts && d.LocationNamesTheFunction && r.locationIsInsideAFunctionBody() {
		return r.inFunc, at - r.funcLine, true
	}
	name = r.name()
	if d.LocationNamesTheCurrentFile {
		// locationFile rather than currentFile: a message located at the call
		// it came from is one frame further out than the shell is. At the top
		// level of a script the current file is the script, and under `-c` or
		// standard input there is no file at all — the stack answers the
		// shell's own name for both, so neither route changes here.
		if f := r.locationFile(); f != "" {
			name = f
		}
	}
	return name, at, false
}

// lineNow is the line to report or to hand `$LINENO`, which is r.line except
// inside a `case` subject in the one dialect that has not advanced the line to
// the `case` yet — see Semantics.CaseSubjectKeepsThePreviousLine and
// Runner.caseSubjectLine.
//
// The axis is consulted here and nowhere else, which is what keeps it off the
// common path: a `case $- in …` never reads the line and never raises it.
//
// The offer is taken down around the question, so an *unanswered* axis — whose
// refusal is itself a diagnostic, and so comes straight back through this
// function — asks once rather than forever.
func (r *Runner) lineNow() int {
	prev := r.caseSubjectPrev
	if prev == 0 || prev == r.line {
		return r.line
	}
	r.caseSubjectPrev = 0
	yes := r.ask(r.sem().CaseSubjectKeepsThePreviousLine,
		"a `case` subject reading the line of the command before it")
	r.caseSubjectPrev = prev
	if !yes {
		return r.line
	}
	return prev
}

// locationPrefixNamed is what goes in front of a diagnostic: the location
// above, with the builtin that is speaking where this dialect puts one, and
// with a construct named between the shell's name and the line where the
// message belongs to one — `<script>: command substitution: line 2: `.
//
// The construct is a parameter rather than a field on the runner, because it
// is true of one message and not of a stretch of the run: the name rides on
// the shell's the way a speaking builtin's does — see
// Diagnostics.withBuiltinInLocation — and nothing else in the same statement
// is inside the construct. Empty for every message that belongs to the script
// itself, which is nearly all of them. See
// Diagnostics.SubstitutionParseFailureNamesTheConstruct for the one dialect
// that asks for it and for what it is measured against.
func (r *Runner) locationPrefixNamed(construct string) string {
	d := r.diag()
	name, line, inBody := r.locationNameAndLine(r.speaker == "")
	if construct != "" {
		name += ": " + construct
	}
	if inBody {
		if line > 0 {
			// No borrowed name here: the function rule is the dialect
			// that names a function in place of a file, and the one
			// dialect that names borrowed text after the location does
			// not have it.
			return d.prefix(name, r.inBuiltin, r.builtinIsSpeaking(), line)
		}
		// Nothing to count, so nothing is written: `f: ` and not `f:0: `.
		return d.prefixWithoutLine(name, r.inBuiltin)
	}
	if r.speaker != "" {
		line = r.speakerLine
	}
	if d.BorrowedTextRendersTheCallStack && len(r.borrowed) > 0 {
		// One shell writes every borrowed text it is inside, each with the
		// line that entered the next, and names the innermost where the
		// script's own name would have gone. See
		// Diagnostics.BorrowedTextRendersTheCallStack for the six rows this
		// is read off, and borrowedStack for the chain itself.
		chain, innermost := r.borrowedStack(d, name)
		// prefixAfterTheFirstFrame rather than prefix: the innermost text is
		// not the first thing this shell wrote, so it names line 1 where the
		// shell's own name would have left it out.
		return chain + d.prefixAfterTheFirstFrame(innermost, r.speaking(), r.builtinIsSpeaking(), line)
	}
	if source, ok := r.borrowedNameBefore(d); ok {
		// The fourth arrangement of the same three fields: the name goes
		// between the shell's and the location rather than after it, which
		// is a prefix of its own rather than something to append. See
		// Runner.borrowedNameBefore.
		if b := r.speaking(); b != "" && d.NamesBuiltinInLocation {
			// And a builtin naming itself takes that slot, rather than
			// standing beside the text it was reached through. The same rule
			// the dialect that writes the name *after* the location follows
			// — see Runner.borrowedName and #2532 — measured here on BusyBox
			// v1.37.0, 2026-09-14, with `p.sh` holding `shift -1`:
			//
			//	./s.sh does `. /p.sh`      /s.sh: shift: line 2: Illegal number: -1
			//	the same under `-c`        /bin/ash: shift: line 2: …
			//	./e.sh does eval "shift -1"  /e.sh: shift: line 1: …
			//	an unset parameter in p.sh  /s.sh: /p.sh: line 2: NOPE: …
			//
			// so the file keeps the slot for what the *shell* says and loses
			// it to whatever builtin speaks for itself. The line is the one
			// inside the borrowed text either way.
			source = b
		}
		return d.forBorrowed().borrowedPrefix(name, source, line)
	}
	return d.prefix(name, r.speaking(), r.builtinIsSpeaking(), line) + r.borrowedName(d)
}

// borrowedText is one level of Runner.borrowed: what the text is called, and
// the line it was entered from.
//
// A named type over `sourced` rather than `sourced` itself, so that the stack
// says what it is a stack *of* — and so that the shape had somewhere to grow
// when the dialect that renders the whole chain arrived (#2461).
type borrowedText struct {
	sourced
	// callerLine is the line, in whatever text was running, that the `.` or
	// the `eval` was written on. It is `r.line` at the moment of the push,
	// which is that number for both — verified against every measured row of
	// Diagnostics.BorrowedTextRendersTheCallStack, including the one where
	// the `.` is inside a function and the line is the function's own.
	callerLine int
}

// locationFileOrName is the name a diagnostic carries when nothing borrowed
// is in front of it: the file this shell is reading where the dialect names
// one, and the shell's own name otherwise.
//
// The first component of a rendered chain, and the same choice
// locationNameAndLine makes — read from one place rather than two, because a
// parse failure and a run-time failure inside the same sourced file must not
// disagree about what the outermost frame is called.
func (r *Runner) locationFileOrName() string {
	if r.diag().LocationNamesTheCurrentFile {
		if f := r.locationFile(); f != "" {
			return f
		}
	}
	return r.name()
}

// borrowedStack renders the chain of borrowed texts above the innermost one,
// and names the innermost.
//
// The rendering is one shell's and is described on
// [Diagnostics.BorrowedTextRendersTheCallStack]: every component but the last
// is `<name>[<the line in it that entered the next>]: `, and the last is left
// to the caller, which writes a location after it for a run-time failure and
// nothing for a parse failure.
//
// `outer` is the name the diagnostic would have carried on its own — the
// script's path — which is the first component. The names after it are the
// borrowed texts' own, so a chain of two sourced files reads `.[2]` for the
// middle one in the shell that names the builtin rather than the path.
func (r *Runner) borrowedStack(d Diagnostics, outer string) (chain, inner string) {
	var b strings.Builder
	name := outer
	for i, t := range r.borrowed {
		if i == 0 && !d.locationNamesALineAt(t.callerLine) {
			// The outermost component is the shell's own name, and where
			// that name would carry no line neither does the frame: measured
			// 2026-09-12, `ksh -u -c '. ./p.sh'` is `/bin/ksh: .: line 3: …`
			// with no bracket, exactly as its plain `-c` diagnostic is
			// `/bin/ksh: NOPE: parameter not set` with no line. The route's
			// own location showing through rather than a rule about
			// brackets.
			//
			// **And it is the line rather than the route**, which the rows
			// this was first written from could not see: their `.` was on
			// line 1 of the program in every one. The same `.` one line lower
			// is `/bin/ksh[2]: .: line 3: …`, so the question is whether the
			// location names a line *at this line*, and the frames inside
			// keep theirs however small — `/bin/ksh: eval[1]: eval: line 1:`
			// (#2417).
			fmt.Fprintf(&b, "%s: ", name)
		} else {
			fmt.Fprintf(&b, "%s[%d]: ", name, t.callerLine)
		}
		name = t.sourceName(d)
	}
	return b.String(), name
}

// borrowedAtLocation is the text a diagnostic's line was read from, when that
// is text the shell borrowed rather than the file it was handed.
//
// **The innermost text still being read, with no test that the failing line
// came from it.** That is measured rather than assumed, and it is the
// surprising half. dash 0.5.12, `env -i` over a script file, four
// arrangements:
//
//	a file sourced by the script, failing in the file      names the file
//	a file sourced by the script, failing in a *function*
//	  the file called, whose body is in the outer script    names the file
//	text `eval` is running, failing in a function it called names `eval`
//	a function *defined* in a sourced file and called
//	  after the source returned                             names nothing
//
// So the third and fourth rows are what fix the rule: a function frame
// standing above the borrowed text does not end it, and the source returning
// does. A depth test like [Runner.locationIsInsideEvalText]'s would have got
// the first and last right and the middle two wrong — which is the shape of
// bug worth naming, because both halves of a wrong rule pass the obvious
// case.
func (r *Runner) borrowedAtLocation() (borrowedText, bool) {
	if len(r.borrowed) == 0 {
		return borrowedText{}, false
	}
	return r.borrowed[len(r.borrowed)-1], true
}

// borrowedName is the borrowed text's name where this dialect writes one
// beside a run-time diagnostic, and empty where it does not.
//
// Two fields have to agree before anything is written.
// [Diagnostics.BorrowedTextIsNamedAtRunTime] says the dialect names borrowed
// text here at all — see there for why the placement enum cannot answer that
// on its own — and the placement itself is [SourceNaming], the same enum the
// parse path reads through [Diagnostics.SourceReport]. Only one of its three
// values adds anything: SourceReplacesShell is already what
// LocationNamesTheEvalText and LocationNamesTheCurrentFile do, from
// locationNameAndLine above, so reaching for the name again would write it
// twice; SourceBeforeLocation is BusyBox ash's and ksh93's, and it goes in
// front of the location rather than after it, which is
// [Runner.borrowedNameBefore] and not this.
//
// Measured 2026-09-12, dash 0.5.12, `env -i` over a script file: a failure on
// line 3 of a file sourced from `./s.sh` is `./s.sh: 3: ./p.sh: NOPE:
// parameter not set`, and the same failure inside an `eval` is `./e.sh: 3:
// eval: NOPE: parameter not set` — the file by the path the script wrote, and
// the builtin's own name for text that came from no file.
//
// **A builtin's own complaint takes that slot instead, and nothing is written
// here** (#2532). The two names are alternatives rather than a pair: dash has
// one place after the location for a name, and a builtin naming itself fills
// it.
//
// The discriminator is [Runner.speaking] — the builtin that is naming itself —
// and deliberately not [Runner.builtinIsSpeaking], which the issue proposed
// and which is wider by a failed redirection opened for a builtin. Measured
// 2026-09-13, dash 0.5.12, `env -i`, each line inside a sourced file and again
// inside `eval`, both arrangements agreeing:
//
//	cd /nonexistent      ./s.sh: 1: cd: can't cd to …          no file name
//	shift 99             ./s.sh: 2: shift: can't shift …       no file name
//	nosuchcmd            ./s.sh: 1: ./i.sh: nosuchcmd: not found      named
//	readonly R=1; R=2    ./s.sh: 2: ./i.sh: R: is read only          named
//	echo hi > /bad/f     ./s.sh: 1: ./i.sh: cannot create …          named
//
// The last three are the shell speaking and keep the name; only the first two
// are a builtin speaking for itself. Taking the wider question would have
// dropped the name from the redirection row, where dash writes it.
func (r *Runner) borrowedName(d Diagnostics) string {
	if !d.BorrowedTextIsNamedAtRunTime {
		return ""
	}
	if r.speaking() != "" {
		return ""
	}
	b, ok := r.borrowedAtLocation()
	if !ok || b.naming(d) != SourceAfterLocation {
		return ""
	}
	return b.sourceName(d) + ": "
}

// borrowedNameBefore is borrowedName's other placement: the name of the
// borrowed text for a dialect that writes it between the shell's own name and
// the location, `ash: ./p.sh: line 2: NOPE: parameter not set`.
//
// It is the fourth arrangement of [Diagnostics.BorrowedTextIsNamedAtRunTime]
// and [SourceNaming] rather than a mechanism of its own, and the four are
// worth reading together: dash names the text after the location, bash and
// zsh put it where the shell's name goes and so need no second name here,
// ksh93 renders the whole chain, and BusyBox ash writes one name in front of
// the location.
//
// The rule for *which* text is named is the same one dash uses — the
// innermost borrowed text still being read, with no test that the failing
// line came from it — and that is measured rather than inherited. BusyBox
// v1.37.0, 2026-09-12, over a script file, the same four arrangements
// [Runner.borrowedAtLocation] records for dash:
//
//	a file sourced by the script, failing in the file      names the file
//	a file sourced by the script, failing in a function
//	  the file called, whose body is in the outer script    names the file
//	text `eval` is running, failing in a function it called names `eval`
//	a function *defined* in a sourced file and called
//	  after the source returned                             names nothing
//
// The last row is the one that keeps this honest: `./s.sh: line 1: NOPE:
// parameter not set` with no name, where the frame is a function and the
// source has returned.
func (r *Runner) borrowedNameBefore(d Diagnostics) (string, bool) {
	if !d.BorrowedTextIsNamedAtRunTime {
		return "", false
	}
	b, ok := r.borrowedAtLocation()
	if !ok || b.naming(d) != SourceBeforeLocation {
		return "", false
	}
	return b.sourceName(d), true
}

// fatalExpansion ends the script because a parameter could not be expanded —
// an unset one under `set -u`, or one `${x?}` was asked about.
//
// Its own status because one dialect answers it differently from every other
// way it stops, and differently again depending on how the shell was
// started: `bash -c 'set -u; echo $NOPE'` exits 127 and the same two lines
// in a file exit 1. Measured across eight other ways bash stops, none of
// which cares how it was invoked.
func (r *Runner) fatalExpansion(format string, args ...any) {
	r.diagf(format, args...)
	r.fatalExpansionQuiet()
}

// fatalParamError is fatalExpansion for `${x?word}` and `${x:?word}`, the one
// expansion failure a dialect may read as a request to stop rather than as an
// error — see Semantics.ParamErrorIsAnExitRequest for what that is measured
// against and where it is asked.
func (r *Runner) fatalParamError(format string, args ...any) {
	r.fatalExpansion(format, args...)
	r.abandon, r.errexitStopped = abandonParamError, false
}

// fatalExpansionQuiet is the same for a failure that has already reported
// itself, as fatalQuiet is to fatal.
func (r *Runner) fatalExpansionQuiet() {
	r.fatalQuiet()
	if n := r.diag().ExpansionFailureStatusFromCommandString; n != 0 && r.Route == RouteCommandString {
		r.status = n
	}
}

// name is what the shell calls itself in a diagnostic.
//
// Usually `$0`, which is the path it was invoked by — but one dialect answers
// with a fixed name instead, and the two are genuinely different questions:
// there, `$0` is still the whole path and only the diagnostic is short. That
// is why the answer is read from Diagnostics rather than written into
// Runner.Name, which would change `$0` with it.
func (r *Runner) name() string {
	// Only where the shell is what is being named. On the script route Name
	// is the script's path, and every shell in the panel prints that — the
	// dialect that shortens its own name shortens only its own. It is the
	// same three-way split `$0` is decided by, which is why the route is the
	// question rather than some second field saying the name is a file.
	if n := r.diag().SelfName; n != "" && r.Route != RouteScriptFile {
		return n
	}
	if r.Name == "" {
		return "sh"
	}
	return r.Name
}

// invokedAs is argv[0] — what the process was executed as, which is a
// different fact from Runner.name and is what one dialect's applet-backed
// builtins report under. Falls back to the diagnostic name where nothing
// carried an argv, which is a library Runner and a test.
func (r *Runner) invokedAs() string {
	if r.Invocation != "" {
		return r.Invocation
	}
	return r.name()
}

func (r *Runner) emit(ctx context.Context, e Event) {
	if r.Events == nil {
		return
	}
	// Every event carries where it came from, filled in one place so no
	// site can forget: the line a diagnostic about the action would name,
	// and the file that line is in. Safe from the goroutines events are
	// emitted on, because each one runs a clone of its own — the same
	// arrangement diagf already relies on.
	e.Line = r.line
	e.File = r.currentFile()
	// And which shell it came from, filled in the same one place and for the
	// stronger version of the same reason: a consumer joining two records of
	// one run needs this on every line, and a site that forgot it would
	// produce a record that silently belongs to nothing.
	e.Session = r.Session
	r.Events.Emit(ctx, e)
}

// allowed consults the gate. A refusal is reported and becomes a failing
// status rather than an abort: a denied command is a command that failed.
func (r *Runner) allowed(ctx context.Context, a Action) bool {
	if r.Gate == nil || r.Gate.Allow(ctx, a) == Allow {
		return true
	}
	r.emit(ctx, Event{Kind: EventDenied, Action: a})
	r.reportRefusal(a)
	r.status = 126
	return false
}

// Run executes a whole file, returning the last command's status.
//
// The context is consulted at every command, so a caller can stop a runaway
// script: cancel it, or give it a deadline, and Run returns with the
// context's own error — `errors.Is(err, context.Canceled)` or
// `context.DeadlineExceeded` — and the status a shell stopped from outside
// reports. See cancel.go for what that costs and what else Run's error can
// be.
func (r *Runner) Run(ctx context.Context, f *syntax.File) (int, error) {
	if err := r.RunPart(ctx, f); err != nil {
		if !r.canceledChunk {
			// A canceled run does not go on to run more of the script, and
			// the EXIT trap is more of the script. The other thing this
			// error can be — a construct this shell has not got — is the
			// script ending, and there the trap belongs.
			r.runExitTrap(ctx)
		}
		return r.status, err
	}
	return r.Finish(ctx), nil
}

// RunPart runs one chunk of a script and leaves the shell open for the next,
// which is what a front end reading its input a line at a time needs: the
// variables, functions and traps of one chunk are still there for the one
// after it.
//
// Nothing is torn down here. Finish does that, once, however many chunks ran.
func (r *Runner) RunPart(ctx context.Context, f *syntax.File) error {
	r.ctx = ctx
	// Read once, here, rather than per command: see cancel.go for why that
	// is the difference between honoring the context and paying for it.
	r.watch(ctx)
	// A chunk is a typed line, and whether the shell has just shown the person
	// its stopped jobs is a fact about the line before this one — see the two
	// fields for what that buys over remembering it forever.
	r.toldOfJobsAtExit, r.tellingOfJobsAtExit = r.tellingOfJobsAtExit, false
	r.ensureLineOrigin()
	r.ensurePWD()
	r.ensureSpecials()
	r.ensureImportedFunctions()
	// Before anything the script runs can create a file, because this is
	// what takes the mask off the process and puts it in this shell's own
	// hands. Once per process in a binary that is a shell: a clone inherits
	// the answer, and a second call does nothing. See umaskscope.go.
	r.ensureUmask()
	// Before the descriptors are published, because publishing them is itself
	// an action and the first one this session records.
	r.ensureActionIDs()
	r.publishInheritedFds(ctx)
	if r.started.IsZero() {
		r.started = r.Now()
	}
	r.programEnd = int(f.End().Line + 1)
	abandoned := 0
	// The field says which line a give-up inside this chunk gives up, and it
	// is put back afterwards for a caller that drives a runner both ways.
	outerInputLine := r.inputLine
	defer func() { r.inputLine = outerInputLine }()
	for i, st := range f.Stmts {
		if abandoned != 0 && r.lineOf(st.Pos()) <= abandoned {
			// The rest of the line the last statement gave up on goes with
			// it. Everything inside a construct has already unwound; this is
			// what makes `r=2; echo one` print nothing where the same two on
			// separate lines run the echo.
			//
			// Never cleared, because a later statement cannot be on an
			// earlier line: the numbers only go up, so a stale one matches
			// nothing. Clearing it was equivalent under mutation, which is
			// how that was established rather than assumed.
			//
			// `<=` rather than `==` for the same reason the line it compares
			// against is the statement's *end*: a compound that gave up
			// spans several lines and the `echo` behind its `fi` is on the
			// last of them, while a backslash-continued command ends before
			// the `;` that separates it from the next. Sequential statements
			// cannot start before the one ahead of them ends, so the two
			// spellings agree on every shape either can reach.
			continue
		}
		// The one place a statement is read at the level a shell reads its
		// input, which is the level one dialect's `$_` moves at.
		r.atInputLevel = r.aLoneSimpleCommandOnItsLine(f.Stmts, i)
		// And the level a give-up is measured from: whatever fails inside
		// this statement gives up this much of the input. See
		// Runner.giveUpLine.
		r.inputLine = r.inputLineOf(st)
		err := r.stmt(ctx, st)
		if arg, ok := r.takeInputLevelArgument(); ok {
			// The statement is over, so the line's own last argument is what
			// the next one reads. See interp/underscoreframe.go for the
			// dialect this is the whole of `$_` for.
			r.inputLastArg, r.inputLastArgSet = arg, true
		}
		r.atInputLevel = false
		if err != nil {
			return err
		}
		if r.ctl == controlAbandon {
			// The statement gave up; the shell has not. This is the one
			// place that consumes it, which is what keeps the give-up from
			// reaching Run the way a fatal error does.
			r.ctl, abandoned = controlNone, r.abandonLine
			continue
		}
		if r.ctl == controlExit {
			break
		}
	}
	if r.stoppedByTheCaller() {
		// The shell goes back into ordinary flow — the cancellation ended
		// this chunk and not the session; see releaseCancellation — and the
		// caller's own error is what says the chunk stopped. Read from the
		// context rather than remembered, because Canceled and
		// DeadlineExceeded are different answers and only the context knows
		// which this was.
		r.releaseCancellation()
		return ctx.Err()
	}
	return nil
}

// Exited reports whether the shell has been asked to stop, so a front end
// feeding it chunks knows not to read another.
func (r *Runner) Exited() bool { return r.ctl == controlExit }

// OneCommand reports `set -t` — that the shell should stop once the line it
// is running has finished.
//
// For a front end, because the option's whole effect is on reading: a line is
// run and then nothing more is read. Asked after each line rather than once,
// since the option can be set — and unset again — by the line itself.
//
// The command-string route is where the two shells that have the option
// disagree, so it is answered here rather than in the front end: measured, a
// two-line `-c` string that turns the option on runs to its end under bash and
// stops after the first line under ksh93. It used to be a route the shared
// driver excluded outright, which was one shell's answer written where every
// dialect reads it (#1716).
//
// Read rather than `ask`ed, for the reason SetInteractiveMonitor reads its
// axis: a front end consults this after every line, so an unanswered preset
// would put "the shells disagree" between every pair of commands rather than
// once. No preset that leaves it unanswered can have the option on in the
// first place — the long name is a dialect's to declare with AddSetOptions,
// and the letter hangs on Semantics.SetHasTheTLetter.
func (r *Runner) OneCommand() bool {
	if !r.onecmd {
		return false
	}
	return r.Route != RouteCommandString || r.sem().OneCommandStopsACommandString == Yes
}

// Finish ends the session and reports the status to exit with.
//
// It is separate from RunPart because the EXIT trap fires once at the end and
// not after every chunk — and it fires even when the *next* chunk failed to
// parse, which is unanimous: a script whose last line is a syntax error still
// runs its EXIT trap.
func (r *Runner) Finish(ctx context.Context) int {
	// Anything that arrived during the last command still runs, before the
	// EXIT trap does.
	r.ctx = ctx
	r.runPendingTraps(ctx)
	r.runExitTrap(ctx)
	r.runExitHook(ctx)
	r.finishHistoryFile()
	if !r.inSubshell {
		r.stopSignalsAndRestore()
	}
	// After the EXIT trap and before the death below, which is the only
	// window that covers both: the trap body can run a process substitution
	// of its own, and DieBySignal does not come back.
	//
	// Here rather than in the front end because every route out of a shell
	// passes through Finish and only some of them pass through any one
	// caller — a script, `-c`, a prompt, a Session that was closed, and a
	// library embedder calling Run. Wired into one of those and not the rest
	// is what shipped: CleanUp existed, was tested, and was called by nothing
	// outside interp's own suite, so every invocation of every dialect binary
	// that used `<(…)` left its directory in /tmp. See #1284.
	// Before the directory goes, and after the EXIT trap has had its say —
	// the trap writes to the shell's standard output, which for the shape
	// this is about *is* the pipe a body is reading. See endHeldProcSubs.
	r.endHeldProcSubs()
	r.cleanUpAtEnd()
	if r.killedBy != "" && !r.inSubshell && r.DieBySignal != nil {
		// Last, because a shell that is dying still runs its EXIT trap first
		// where the dialect says so. This does not come back.
		//
		// And only for the shell at the top, which is the same boundary
		// stopSignalsAndRestore is held behind four lines above. Finish is
		// reached by three runners that are not the shell — a `<(cmd)` body,
		// a `>(cmd)` body and a command substitution's — and in every shell
		// on the panel each of those is a *process of its own*. A fatal
		// signal one of them takes ends that process and the shell that
		// named it carries on: measured 2026-09-15 against bash 5.3, where
		// a body writing more than a pipe will hold into a reader that has
		// gone leaves `read x < <(…)` at status 0, and the same write inside
		// `x=$(…)` leaves `$?` at 141 with the next command still running.
		// The status is already what the signal made it — signalDeath set it
		// before it stopped the body — so dropping the raise here is the
		// whole difference between the two answers.
		//
		// Raising it anyway killed *this* process, which is the shell, and
		// that is the regression #3015 records: a body is a goroutine here
		// rather than a fork, so its death had nowhere else to land. Two
		// files of bash's own suite ended on SIGPIPE where bash exits 0.
		//
		// A signal a body aims at the shell on purpose — `<(kill -TERM $$)`
		// — does not come through here at all. It is recorded in the box the
		// clone shares with its parent and raised by the parent's own Finish;
		// see recordSharedDeath, which is where that boundary already lived.
		if err := r.DieBySignal(r.killedBySig); err != nil {
			r.diagf("kill: %v\n", err)
		}
	}
	return r.status
}

// runExitTrap runs `trap … EXIT` as the shell ends.
//
// Once per shell, and a subshell is a shell: its own EXIT trap fires when it
// ends, and the parent's does not fire there — which is two separate rules
// and not one. The parent's is already handled by inheritTraps, which hands a
// clone no EXIT trap at all; this used to carry the other half as a
// `!r.inSubshell` guard, which is what stopped a subshell's own trap from
// ever running (#2349).
//
// Measured 2026-09-12 against dash, bash 5.3, bash 3.2, ksh93 and zsh, which
// agree on every shape: `( trap … EXIT; exit 5 )` runs the handler and still
// reports 5, `( trap … EXIT; true )` runs it on the fallthrough, and
// `x=$( trap … EXIT; true )` captures what it wrote. See
// Runner.endSubshell for where the boundaries that are not a whole Run call
// this.
//
// The status is left as it is so the body can read `$?` — measured: a trap set
// after `false` sees 1, and in a subshell it sees the status the subshell is
// about to report. If the body exits with a status of its own, that wins,
// which is why the control flag is cleared first and consulted after.
//
// It reports whether the *body* left through `exit`, which is a question only
// this function can answer: it clears the control flag before running the body
// and forces it back to controlExit afterwards, so a caller reading the flag
// on either side of the call cannot tell `( trap 'exit 5' EXIT; true )` from
// `( trap 'echo T' EXIT; true )`. Runner.endSubshell needs exactly that
// distinction for zsh's exit hook, which fires for the first and not the
// second.
func (r *Runner) runExitTrap(ctx context.Context) (exitedInTheBody bool) {
	if r.exitTrap == nil {
		return false
	}
	// A shell that was killed rather than ended is a two-two split: bash and
	// ksh93 treat dying as exiting and run the trap, dash and zsh do not.
	// Asked only where there is a trap and a death to disagree about.
	if r.killedBy != "" && !r.ask(r.sem().ExitTrapRunsOnSignalDeath, "the EXIT trap after a fatal signal") {
		return false
	}
	body := *r.exitTrap
	// Cleared before running so the body cannot fire it again, and so a
	// `trap` inside it replaces rather than recurses.
	r.exitTrap = nil
	before := r.status
	r.ctl = controlNone
	// And the same treatment for a pending substitution stop, for the same
	// reason: the handler is more of the script and runs to its end, and the
	// stop is still the shell's afterwards. See Runner.holdScriptStop.
	defer r.holdScriptStop()()
	// Kept for a bare `exit` inside the body, which in three of the four
	// reports this rather than whatever the body's last command did.
	r.inExitTrap, r.exitTrapEntryStatus = true, before
	r.runTrapBody(ctx, "EXIT", body)
	// Cleared for hygiene rather than for effect: the EXIT trap is the last
	// thing a shell runs, so nothing reads this afterwards.
	r.inExitTrap = false
	exitedInTheBody = r.ctl == controlExit
	if !exitedInTheBody {
		// The body ran to the end without exiting, so the script keeps the
		// status it already had.
		r.status = before
	}
	r.stopTheShell()
	return exitedInTheBody
}

// runExitHook runs the dialect's exit hook — zsh's `zshexit` — as the shell
// ends.
//
// After runExitTrap, which is the measured order: a script with both wrote the
// trap's line and then the hook's. See Semantics.ExitHook for the rest of what
// was measured, including why a shell killed by a signal runs neither.
//
// The chain is [Runner.FireChain], as every hook chain in this shell is, and
// the closure carries the one rule this site does not share with the others.
// Everywhere else an item that exited ends the chain, because what it ended is
// the session. Here the session is already over, so `exit` has nothing left to
// end and all it can do is record the status the shell leaves with — measured:
// `exit 9` in the named hook did not stop the member after it, and an
// `exit 11` in that member is what the shell exited with. So the flag is
// cleared around each call and the last status an item asked for is kept.
func (r *Runner) runExitHook(ctx context.Context) {
	if r.inSubshell {
		// A subshell's ending is a boundary of its own, with a narrower rule
		// — see Runner.fireExitHook, which Runner.endSubshell calls there.
		return
	}
	r.fireExitHook(ctx)
}

// fireExitHook is the whole of running the hook, at whichever of the two
// boundaries reached it.
func (r *Runner) fireExitHook(ctx context.Context) {
	name := r.sem().ExitHook
	if name == "" || r.killedBy != "" {
		return
	}
	// The status the shell is leaving with. FireChain hands it to every item
	// and puts it back after the last, so this only has to record the one
	// case FireChain has no opinion about: an item that asked for another.
	leaving, asked := r.ExitStatus(), false
	// Put back rather than set to controlExit on the way out. Finish did not
	// touch this flag before the hook existed, and a shell that arrived here
	// without having exited must not leave here looking as though it had —
	// see the driver's `if r.Exited()` after the startup files, which is one
	// of the callers that reads it afterwards.
	entered := r.ctl
	r.FireChain(r.HookChain(name), func(fn string) {
		// Cleared before the call rather than once before the loop, because
		// the item before this one may have exited — and an item that ran
		// under the flag would not run at all.
		r.ctl = controlNone
		_, _ = r.CallFunction(ctx, fn)
		if r.ctl == controlExit {
			leaving, asked = r.ExitStatus(), true
		}
		// And cleared again, so FireChain's own "an item that exited ends the
		// chain" never fires here. That rule is right everywhere it is asked
		// and wrong at this one site, which is the whole of what this closure
		// is for.
		r.ctl = controlNone
	})
	r.SetExitStatus(leaving)
	r.ctl = entered
	if asked {
		r.stopTheShell()
	}
}

// runTrapBody parses and runs a trap's text, which is re-parsed at fire time
// because that is when a shell reads it.
//
// Whether the part that parsed runs before the failure is reported is a
// question: bash and dash read a line at a time, so `trap "echo a
// if" EXIT` prints `a` and then complains, and ksh93 reads the whole body
// first and prints nothing. zsh never reaches this — it reads the action
// when the trap is set and refuses one that will not parse.
func (r *Runner) runTrapBody(ctx context.Context, cond, body string) {
	defer r.enterTrapBody(cond)()
	// And the body is the text that runs, at the offset just settled, so a
	// refusal inside it quotes the body and never the lines of the script it
	// interrupted — measured on bash 5.3.20, `exit trap: line 1:
	// `v=$(echo hi; for)'`. Put back by enterTrapBody's restore. See
	// runningText.
	r.runText = runningText{text: body, base: r.lineBase, borrowed: true}
	p := r.ParseWithAliases(body, r.dialect())
	if r.ask(r.sem().TrapBodyRunsWhatParsed, "a trap body running the part of it that parsed") {
		r.runTrapBodyByLine(ctx, p, body)
		return
	}
	if r.unspecified {
		return
	}
	f := p.Parse()
	if err := p.Err(); err != nil {
		r.reportTrapParseFailure(err, body)
		return
	}
	r.runTrapStmts(ctx, f)
}

// runTrapBodyByLine runs each line as it parses, the way a shell reading its
// input runs what it has read. The line is the unit, not the statement, which
// is what NextLine already answers.
func (r *Runner) runTrapBodyByLine(ctx context.Context, p *syntax.Parser, body string) {
	for {
		f, ok := p.NextLine()
		if f != nil && f.Refused != nil {
			// A line of the body the reader could not finish, where only
			// that line goes — see syntax.File.Refused. Reported where this
			// body's other parse failures are reported, and then the next
			// line of the body runs, which is what the shell does with a
			// trap whose text holds one.
			r.reportTrapParseFailure(f.Refused, body)
			continue
		}
		if f != nil && !r.runTrapStmts(ctx, f) {
			return
		}
		if err := p.Err(); err != nil {
			r.reportTrapParseFailure(err, body)
			return
		}
		if !ok {
			return
		}
	}
}

// runTrapStmts runs one parsed chunk of a trap body, reporting whether to
// carry on.
//
// The control-flow check is a fast exit rather than the thing that stops an
// `exit` inside a body: stmt refuses to run anything once control flow is
// set, so a mutation of either check is equivalent. Kept because iterating
// the rest of the body — and, in the by-line path, parsing it — to do nothing
// is work for no reason.
func (r *Runner) runTrapStmts(ctx context.Context, f *syntax.File) bool {
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.diagf("trap: %v\n", err)
			return false
		}
		if r.ctl != controlNone {
			return false
		}
	}
	return true
}

// reportTrapParseFailure is what a trap body that will not parse produces.
//
// The same rendering a script's parse failure gets, because it is the same
// failure in text that arrived another way, and the lines it names are the
// body's adjusted by whatever enterTrapBody decided. Where the text came from
// is passed the way the front end passes `-c`, so the one dialect that names
// its input names this too and the three that do not are unaffected — a fixed
// string rather than a wording, for the same reason `-c` is one.
//
// Fatal under the rule a parse failure inside `.` or `eval` already gets:
// POSIX makes a special builtin's failure fatal to a non-interactive shell,
// and `trap` is one. The text arrives later than theirs but it is still text
// handed to a special builtin, and the panel answers it the same way here as
// there — dash ends the script, the other three carry on.
func (r *Runner) reportTrapParseFailure(err error, body string) {
	// The body's lines, moved the way every other diagnostic from inside it
	// is moved. A no-op in the two dialects that count a trap body from its
	// own first line, and what makes the third name the same line here as it
	// would for a command that failed in the same place.
	err = shiftParseError(err, r.lineBase)
	if r.ask(r.sem().TrapParseFailureNamesWhereItFired, "a trap body's parse failure naming where the trap fired") {
		r.reportTrapParseFailureAtFiringLine(err, body)
	} else if !r.unspecified {
		where := "trap"
		if r.inExitTrap {
			where = "exit trap"
		}
		r.errf("%s", r.diag().ParseDiagnostic(r.name(), where, err, body))
	}
	// No status of its own where the failure is not fatal: measured, a
	// signal trap whose body will not parse leaves `$?` at 0 in both
	// dialects that carry on. Setting the parse status here was invisible
	// because the fatal path overwrites it and the EXIT path puts the old
	// one back — a mutation that dropped it changed nothing.
	if r.ask(r.sem().BuiltinSyntaxErrorFatal, "a parse failure inside a special builtin being fatal") {
		r.fatalQuiet()
	}
}

// reportTrapParseFailureAtFiringLine writes the failure with the *runtime*
// location in front of it rather than the parse one.
//
// ksh93 alone, and the two lines in it are different numbers on purpose:
//
//	trap "echo a
//	if" USR1        set on line 2, fired from line 5
//
//	w5.sh: line 5: syntax error at line 6: `if' unmatched
//
// The location names where the trap fired and the wording names where in the
// body the parse gave out. The line is left out when it is the first, which
// is the same rule this dialect's other location style uses — and an EXIT
// trap counts as firing on line 1, so an EXIT body's failure carries none.
func (r *Runner) reportTrapParseFailureAtFiringLine(err error, body string) {
	d := r.diag()
	loc := d.prefixWithoutLine(r.name(), "")
	if at := r.firedAt(); at > 1 {
		loc = d.prefix(r.name(), "", false, at)
	}
	r.errf("%s%s\n", loc, d.ParseFailure(err))
	r.errf("%s", d.echoLine(r.name(), "", d.ParseFailureLine(err), err, body))
}

// shiftParseError moves a parse failure's lines by an offset, so a failure in
// text that was parsed on its own can name the lines the shell counts it at.
func shiftParseError(err error, by int) error {
	var se *syntax.Error
	if by == 0 || !errors.As(err, &se) {
		return err
	}
	moved := *se
	moved.Pos.Line += int32(by)
	if moved.ConstructLine > 0 {
		moved.ConstructLine += by
	}
	if moved.EndLine > 0 {
		moved.EndLine += by
	}
	return &moved
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) error {
	// A substitution somewhere below this shell could not parse its body, and
	// in the dialects that say so that ends the script rather than the
	// subshell it was written in. Taken here because this is the one point
	// every command passes through, and before the handlers because nothing
	// after the failure is the script's to run. See substitutionstop.go.
	if status, stopped := r.takeScriptStop(); stopped {
		// The status is the failure's and not whatever the command that held
		// the word happened to report. They are the same number wherever the
		// shell that failed is the one that stops, and two different numbers
		// where it is not — see scriptStop.status.
		r.status = status
		// The same kind the failure was raised under one shell down, and
		// for the same reason: what comes out of the box is the error that
		// shell reported, so a boundary above — an interactive prompt most
		// of all — has to see an error rather than a request to stop
		// (#3300). Without it `( v=$(echo hi; for) )` typed at a prompt
		// ended the session where the plain shape had been fixed, which is
		// the half a fix at the raise site alone cannot reach.
		r.ctl, r.abandon, r.errexitStopped = controlExit, abandonSubstParse, false
		return nil
	}
	// Whatever arrived while the previous command ran. A shell finishes what
	// it is doing and runs the handler between commands, which is measured
	// and unanimous — so this is the point where a signal becomes visible.
	r.runPendingTraps(ctx)
	if r.ctl != controlNone {
		return nil
	}
	// A statement is text the shell is running, so whatever `command` made
	// survivable is behind us: the word covers the builtin it named and not
	// the commands that builtin goes on to run. Cleared rather than saved,
	// because runWithoutFunctions is the only setter and it puts back what
	// it found.
	r.throughCommandWord = false
	// And the ERR trap has not fired for whatever this statement is about
	// to leave behind. See Runner.errTrapFired: the clear has to be here,
	// at the head of a statement, so that a compound clears it before its
	// body runs and still sees the body's firing when it is judged itself.
	r.errTrapFired = false
	// Counted before the handlers run, so the compound this statement may be
	// can tell afterwards whether its body ran a statement of its own. See
	// Runner.stmtSerial.
	r.stmtSerial++
	serial := r.stmtSerial
	r.statusBefore = r.status
	if st.Coprocess {
		// Before Background, because a coprocess is a background job with
		// pipes on its named streams rather than an ordinary one, and the
		// two ways of starting it differ in more than the streams.
		return r.coprocStmt(ctx, st)
	}
	if st.Background {
		return r.background(ctx, st)
	}
	if err := r.expr(ctx, st.Expr); err != nil {
		return err
	}
	if _, isChain := st.Expr.(*syntax.BinaryExpr); !isChain && !lastIsNegated(st.Expr) &&
		(!reportsItsBody(st.Expr) || r.stmtSerial == serial) {
		// A chain judges itself, inside expr, because only its final operand
		// counts and only when that operand actually ran.
		if p, ok := st.Expr.(*syntax.Pipeline); ok && len(p.Cmds) > 1 {
			// A pipeline whose last element ran in this shell is judged by
			// the dialect's reading of that element. See judgePipeline.
			r.judgePipeline(ctx)
		} else {
			r.checkErrExit(ctx)
		}
	}
	return nil
}

// reportsItsBody reports whether a statement is a compound command that
// answers with the status the last statement of its body left — a group, a
// loop, an `if`, a `case` — rather than with a status of its own.
//
// Such a statement is not judged by `set -e` or the ERR trap once its body
// has run a statement, because that statement was already judged or was
// exempt. Judged, a failure has already stopped the script or fired the trap
// and there is nothing left to add. Exempt — the left operand of an `&&` that
// short-circuited, a `!`, a condition — and judging the compound in its place
// ends the script over a status the shell was told not to act on:
//
//	set -e; for i in 1 2; do [ "$i" = 3 ] && echo three; done; echo survived
//
// writes `survived` in bash 5.3.20, bash 3.2, zsh 5.9.2, ksh93u+ 2012 and
// dash, and ended at 1 here in every column (#3344). POSIX XCU 2.8.1 says the
// same of `set -e`: a compound other than a subshell whose status came from a
// failure while the option was being ignored is not a failure it sees.
//
// A compound whose body never ran is still judged, because then the status is
// the compound's own: `{ :; } >/nonexistent/f` fails at the redirection.
// A subshell is not in the list, because its status is a child's exit — the
// child's own judgment happened in a copy — and every shell judges it.
func reportsItsBody(e syntax.Expr) bool {
	p, ok := e.(*syntax.Pipeline)
	if !ok || p.Negated || len(p.Cmds) != 1 {
		return false
	}
	return commandReportsItsBody(p.Cmds[0])
}

// commandReportsItsBody is reportsItsBody asked of one command, which is the
// question a pipeline's last element needs as well as a statement.
func commandReportsItsBody(c syntax.Command) bool {
	switch c.(type) {
	case *syntax.Group, *syntax.TryClause, *syntax.IfClause, *syntax.LoopClause,
		*syntax.ForClause, *syntax.ForArithClause, *syntax.CaseClause,
		*syntax.SelectClause, *syntax.RepeatClause:
		return true
	}
	return false
}

// lastIsNegated reports whether the command `set -e` would judge carries a
// `!`. Negation tests a status rather than requiring success, so `! true`
// yields 1 and does not end the script — measured, and unanimous.
//
// It asks about the *last* operand for the same reason the `&&` rule does:
// that is the one whose status the statement reports.
func lastIsNegated(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		return lastIsNegated(x.Y)
	case *syntax.Pipeline:
		return x.Negated
	case *syntax.TimeClause:
		// A bang on either side of `time` tests the status rather than
		// requiring success: `set -e; time ! false` carries on.
		return x.Negated || lastIsNegated(x.Pipeline)
	}
	return false
}

// checkErrExit judges a statement that failed: the ERR trap fires here, and
// `set -e` then ends the script.
//
// One place, after a whole statement, because that is the granularity the
// shells use: `false | true` does not fire and `true | false` does, and both
// are one statement whose status is the pipeline's. The ERR trap shares the
// gate — measured, a failure inside an `if` condition, an `&&` operand or a
// `!` fires neither — and fires whether or not `set -e` is on, which is also
// measured and unanimous among the shells that have the condition. When both
// apply, the trap runs first and the script then stops, in that order.
func (r *Runner) checkErrExit(ctx context.Context) {
	if r.tested != 0 || r.status == 0 || r.ctl != controlNone {
		return
	}
	if r.arithZeroLeft && (r.errexit || r.errTrapIsSet()) &&
		!r.ask(r.sem().ArithCommandZeroIsAFailure, "an `(( ))` whose value is zero being a failure `set -e` and ERR see") {
		return
	}
	pipefailOnly := r.pipefailRaised
	// Once per failure, not once per level that reports it. `set -e` is
	// deliberately outside this guard: it judges the statement it is given
	// and a failure inside a compound has already ended the script before
	// the compound is reached, so nothing here changes what it stops for.
	if !r.errTrapFired {
		r.runErrTrap(ctx)
	}
	if !r.errexit || r.ctl != controlNone {
		// Either nothing more to do, or the trap's own action already ended
		// the script — its `exit` wins, and judging the statement again
		// would overwrite the status that action chose.
		return
	}
	// Asked only here, where the answer decides something. A pipeline whose
	// failure came only from pipefail is a failure one shell does not stop
	// for — but with `set -e` off, or with the statement's failure already
	// accounted for, nothing turns on it and the core must not refuse.
	if pipefailOnly &&
		!r.ask(r.sem().ErrexitSeesPipefailFailure, "`set -e` stopping for a failure only pipefail saw") {
		return
	}
	// The status is the failing command's, not a status of its own —
	// `set -e; exit` reports what failed.
	//
	// abandonKind is set rather than left at whatever the failing statement
	// happened to leave: this is a request to stop, and reading it as an
	// error the shell reported would let a boundary that gives up one file
	// catch a `set -e` the shell has already decided to end over.
	r.ctl, r.abandon, r.errexitStopped = controlExit, abandonRequested, true
}

func (r *Runner) expr(ctx context.Context, e syntax.Expr) error {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		// Only the *last* command of an `&&`/`||` chain is subject to
		// `set -e`. The tree is left-associative, so everything but the
		// final operand is inside X, and one counter covers the lot.
		r.tested++
		err := r.expr(ctx, x.X)
		r.tested--
		if err != nil {
			return err
		}
		// && runs the right side when the left succeeded, || when it failed.
		// Both are one level and left-associative, which the tree already
		// encodes; nothing here re-decides it.
		runRight := (x.Op == syntax.TokAndAnd) == (r.status == 0)
		if !runRight {
			// The chain short-circuited, so the final operand never ran and
			// there is nothing for `set -e` to judge: `false && :` leaves
			// status 1 and does not end the script.
			return nil
		}
		if err := r.expr(ctx, x.Y); err != nil {
			return err
		}
		if !lastIsNegated(x.Y) {
			r.checkErrExit(ctx)
		}
		return nil
	case *syntax.Pipeline:
		return r.pipeline(ctx, x)
	case *syntax.TimeClause:
		return r.timeClause(ctx, x)
	}
	return r.unsupported(fmt.Sprintf("%T", e))
}

func (r *Runner) pipeline(ctx context.Context, p *syntax.Pipeline) error {
	// Consumed here so that only the timed clause's own body is measured
	// per element — a pipeline nested anywhere inside one of its elements
	// is that element's work, not a row of the report.
	timing := r.timedPipeline
	r.timedPipeline = nil
	if timing != nil {
		timing.grow(p.Cmds)
	}
	if p.Negated {
		// A negated pipeline is a status being *tested*, so `set -e` is
		// suspended for the whole of it — and the suspension is inherited,
		// exactly as an `if` condition's is. Skipping only the judgement of
		// the finished statement, which is what lastIsNegated does, leaves a
		// failure *inside* what the negation ran free to end the script:
		//
		//	set -e; f() { false; echo body; }; ! f; echo end
		//	set -e; ! { false; echo body; }; echo end
		//
		// bash, ksh93 and dash print both lines and carry on; so does zsh
		// for the group. This shell printed neither and stopped, which is
		// the shape `set -e` is least forgiving of — a script that says
		// "I am testing whether this fails" was ended by the failure it
		// asked about. Counted here rather than in stmt so that it covers
		// the pipeline wherever one is reached from.
		r.tested++
		defer func() { r.tested-- }()
	}
	r.pipefailRaised = false
	if len(p.Cmds) == 0 {
		// A `!` written with no pipeline after it, which one grammar flag
		// admits. Nothing runs, so the status the negation below inverts is
		// a success — `true; !` and `false; !` both answer 1, measured, so
		// what came before it is not carried through.
		r.status = 0
	} else if len(p.Cmds) == 1 {
		if timing != nil {
			// One element, run in the current shell like any other single
			// command; the element's externals bill its slot for as long
			// as it runs.
			saved := r.elemCPU
			r.elemCPU = &timing.elems[0].cpu
			start := time.Now()
			err := r.command(ctx, p.Cmds[0])
			timing.elems[0].wall = time.Since(start)
			r.elemCPU = saved
			if err != nil {
				return err
			}
		} else if err := r.command(ctx, p.Cmds[0]); err != nil {
			return err
		}
		r.recordSingleStatus(p)
	} else if err := r.runPipeline(ctx, p, timing); err != nil {
		return err
	}
	if p.Negated && r.negationInverts() {
		// `!` applies to the whole pipeline and inverts its status.
		if r.status == 0 {
			r.status = 1
		} else {
			r.status = 0
		}
	}
	return nil
}

// negationInverts reports whether the `!` in front of a pipeline moves the
// status.
//
// Always, when the pipeline ran: inverting is the whole of what a `!` is for,
// and no shell disagrees. The question is only for `set -n`, where nothing
// under the negation ran and the status being inverted was reported by no
// command — six of the seven panel columns leave it alone there and zsh
// inverts it anyway. See Semantics.UnrunNegationInvertsTheStatus, where the
// panel is measured.
//
// The vector is read only under `set -n`, so an ordinary negated pipeline
// neither consults it nor can refuse over it: asking on the running route
// would make `! grep -q pat file` unanswerable in the core, which is a
// question the shells never posed.
func (r *Runner) negationInverts() bool {
	if !r.noexec {
		return true
	}
	return r.ask(r.sem().UnrunNegationInvertsTheStatus,
		"a `!` inverting the status of a pipeline `set -n` never ran")
}

func (r *Runner) command(ctx context.Context, c syntax.Command) error {
	// Whatever ended the last command is not what ends this one. Cleared
	// here rather than beside each assignment to status, because this is the
	// one door every command goes through.
	r.diedOfSig = 0
	if r.noexec {
		// `set -n` — commands are read and never executed, and nothing turns
		// it back off: even `set +n` is a command. Syntax errors still
		// surface, because they happen on the way in, not here.
		return nil
	}
	// Where this command begins, not where the statement holding it did.
	// `true &&` on one line and the command on the next is two commands and
	// two lines, and every shell in the panel reports the second at its own
	// — taking the statement's line named the operator's instead, so a
	// not-found on line 12 of a `&&` chain was reported at the line the chain
	// started on.
	if c != nil {
		// Guarded because this dispatcher already tolerated a nil command —
		// it falls through to unsupported(%T) and reports rather than
		// crashing. Our own parser never produces one, so no test can see
		// the difference; an embedder building a tree by hand can, and this
		// package is a library.
		r.prevLine, r.line = r.line, r.lineOf(c.Pos())
	}
	// And this door is where an interrupt has to be noticed, because for a
	// loop of the shell's own commands there is no other: nothing in `while
	// :; do echo tick; done` blocks, waits or returns to anywhere else.
	//
	// *After* the line is recorded, and that is not tidiness. Giving up the
	// line means naming the line to give up, and on the first command of a
	// chunk the number is still zero — so an interrupt taken there matched no
	// statement, and everything after it on the line ran as though nothing
	// had happened.
	if r.takeInterrupt() {
		return nil
	}
	// And the same door for a caller that has asked the run to stop. Beside
	// the interrupt rather than in the loop drivers, for the reason the
	// interrupt is here: this is the one place a loop of the shell's own
	// commands passes through. See cancel.go.
	//
	// Returning here is a fast exit rather than the thing that stops the
	// command: canceled() sets the control flow, and every dispatcher below
	// refuses once that is set. Measured by mutation — dropping the return
	// leaves the behavior identical, and the difference is only whether a
	// stopped shell walks through a command's bookkeeping to find out it is
	// not running it.
	if r.canceled() {
		return nil
	}
	if c != nil {
		// And what the command *is*, for the one message that says a
		// command back rather than naming it: a signal that ends one is
		// reported with the command written out.
		//
		// Not updated inside a copy, because the *job* is what that message
		// names and the copy is running one. Measured: the dialect that
		// prints the command back says `( cmd )` for a subshell and `cmd`
		// for a group, a function body or an `if` — and only the subshell
		// is a job of its own. The Subshell node is recorded here, on the
		// way in, and the copy inherits it.
		if !r.inSubshell {
			r.killed = c
		}
	}
	// The input a pipeline's pipe replaced reaches this command's *words*
	// and no further. A compound command's body, a function's body and an
	// `eval`'s program all run after the element's redirections are in
	// place, and the shell that parts from the panel here agrees with it at
	// every one of them — `printf "PIPE\n" | { cat <(cat); }` reads the
	// pipe in all five. So anything that is not one simple command drops it
	// on the way in. See Runner.shellStdin.
	if _, simple := c.(*syntax.SimpleCmd); !simple {
		r.shellStdin = nil
	}
	// A command that is not a simple one fires the DEBUG trap here, where
	// the dialect's reading says a head fires at all. After the line record
	// and before the dispatch, so the trap names the head's own line, and an
	// action that sets control flow takes the command it was about to
	// precede with it — the same two rules the simple-command site follows.
	//
	// A function's body is the exception and is not a head in any column,
	// however it is written: measured, no shell in the panel writes a head
	// for one, though the one that writes most of them writes a head for a
	// `{ }` standing on its own. The caller says so for the one dispatch;
	// see callFuncAs, which fires the separate entry head itself.
	//
	// A pipeline element whose firing the pipeline itself has already made —
	// or deliberately withheld — is the other exception, and it covers the
	// simple site below as well as this one, which is why it is read here
	// and handed down rather than left in the field. Read and cleared at
	// this one door so that nothing *inside* the element inherits it; those
	// commands are commands in their own right and fire as usual.
	r.arithZeroLeft = false
	fired := r.elementFired
	r.elementFired = false
	head := !r.suppressedHead
	r.suppressedHead = false
	if head && !fired {
		r.debugCompoundHead(ctx, c)
		if r.debugTrapStopped() {
			return nil
		}
	}
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		// A simple command is the one kind that can *run* the failure it
		// reports — a function call, a `.`, an `eval` — and the dialects
		// disagree about whether it is then a second place the ERR trap
		// fires. Whether an ERR trap was set when the command began is part
		// of one column's answer, so it is read here, before the command
		// runs, and not afterwards when the body may have set one.
		set := r.errTrapIsSet()
		r.execSerial = 0
		serial := r.stmtSerial
		err := r.simple(ctx, x, fired)
		// What a function body, an `eval` or a `.` ended on is theirs; the
		// command reporting it is not an arithmetic command.
		r.arithZeroLeft = false
		// Whether this command was handed to the exec path rather than run
		// by the shell itself: a command a body ran is at a later statement,
		// so a function whose body ran an external command still ran here.
		// See Runner.lastSimpleRanOnPath.
		r.lastSimpleRanOnPath = r.execSerial == serial && serial != 0
		r.reopenErrJudgment(set)
		return err
	case *syntax.Group:
		return r.group(ctx, x)
	case *syntax.TryClause:
		return r.tryClause(ctx, x)
	case *syntax.Subshell:
		return r.subshell(ctx, x)
	case *syntax.IfClause:
		return r.ifClause(ctx, x)
	case *syntax.LoopClause:
		return r.loop(ctx, x)
	case *syntax.ForClause:
		return r.forClause(ctx, x)
	case *syntax.ForArithClause:
		return r.forArithClause(ctx, x)
	case *syntax.CaseClause:
		return r.caseClause(ctx, x)
	case *syntax.FuncDecl:
		return r.funcDecl(x)
	case *syntax.TestClause:
		return r.testClause(ctx, x)
	case *syntax.SelectClause:
		return r.selectClause(ctx, x)
	case *syntax.RepeatClause:
		return r.repeatClause(ctx, x)
	case *syntax.AnonFunc:
		return r.anonFunc(ctx, x)
	case *syntax.ArithCmdClause:
		return r.arithCmd(ctx, x)
	case *syntax.CoprocClause:
		return r.coprocClause(ctx, x)
	}
	return r.unsupported(fmt.Sprintf("%T", c))
}

// unsupported refuses rather than silently doing nothing.
func (r *Runner) unsupported(what string) error {
	return fmt.Errorf("not implemented yet: %s", what)
}

// simple runs a simple command. fired says the pipeline this command is an
// element of has already made — or withheld — its DEBUG firing, so there is
// none to make here; see Semantics.DebugTrapPipelines and Runner.elementFired.
func (r *Runner) simple(ctx context.Context, c *syntax.SimpleCmd, fired bool) error {
	// The DEBUG trap fires here, before anything about the command is even
	// expanded. A simple command fires it in every column that has the
	// condition; which *compound* heads fire it as well is the dialect's
	// answer and is fired from the dispatcher — see debugCompoundHead, whose
	// table replaced a claim written here that a compound heading fires
	// nothing, which no column in the panel does. An action that exits takes
	// the command it was about to precede with it.
	//
	// And this is where the command is recorded as the one the shell is
	// running, before its own words are expanded — so a command reading the
	// parameter that names it reads itself. See RunningCommand.
	//
	// The record is written whether the firing was made here or by the
	// pipeline this command is an element of: what a pipeline withholds is
	// the *firing*, and an element that did not record itself would leave
	// the parameter reading whichever element the pipeline fired for last.
	// Measured — `echo "A:[$BASH_COMMAND]" | sed …` names the `echo` in bash
	// 5.3.15, not the `sed`.
	r.recordRunning(c, WholeCommand)
	if !fired {
		r.runDebugTrap(ctx)
		if r.debugTrapStopped() {
			return nil
		}
	}
	r.unspecified, r.expandErr, r.badSubscript, r.assignFailed = false, false, false, false
	// Whatever this command's process substitutions opened is closed when the
	// command is done, whether it turned out to be a builtin, a function or
	// something on PATH.
	//
	// Here rather than beside the exec, which was where it started and was
	// wrong: `echo hi > >(tr a-z A-Z)` never reaches an exec at all, so the
	// pipe stayed open, `tr` waited for an end-of-file that was never coming,
	// and the substitution simply produced nothing.
	//
	// Only this command's own. Whatever was pending when it started is the
	// enclosing command's — the call a function body is running inside — and
	// is set aside for the length of this one and put back after it, so that
	// the call still holds its pipes for the next command in the body.
	enclosing, enclosingBefore := r.procSubs, r.enclosingProcSubs
	if len(enclosing) > 0 {
		r.enclosingProcSubs = append(slices.Clone(enclosingBefore), enclosing...)
		r.procSubs = nil
	}
	defer func() {
		r.removeProcSubs(r.takeProcSubs())
		if len(enclosing) > 0 {
			r.procSubs, r.enclosingProcSubs = enclosing, enclosingBefore
		}
	}()
	// `=cmd` is resolved across the whole command before any of it is
	// expanded, which is measured rather than assumed: `echo [[a == a]]`
	// reports the `==` and never reaches the `[[a`, so zsh has finished this
	// phase before pathname expansion begins on the first word.
	for _, w := range c.Args {
		r.expandEquals(w)
	}
	if r.expandErr {
		// Only what this pass just set. Testing r.ctl here as well made
		// every *later* command in an already-abandoned script report a
		// fresh failure of its own.
		r.failedExpansion()
		return nil
	}
	var argv []string
	// The leading words are scanned for the dialect's precommand modifiers
	// before any word is matched against the filesystem, which is the only
	// order `noglob` can be honored in: what it switches off is the stage
	// that would otherwise already have expanded the words behind it. See
	// precommand.go for the family and what was measured about each of them.
	scanning := len(r.precommands) > 0
	noglob := false
	// The words `set -k` takes out of the argument list and puts in front of
	// the command, in the order they were written. Collected here and applied
	// below by rewriting the command, so that from the moment one is promoted
	// it is an ordinary prefix assignment and every axis that already answers
	// for one answers for it — whether it reaches a child's environment,
	// whether it outlives a call, what `set -x` writes for it. See
	// keywordassign.go.
	var promoted []*syntax.Assign
	for i, w := range c.Args {
		if r.expandErr || r.ctl == controlExit {
			// The command is abandoned at its first failed expansion rather
			// than diagnosing every word that would fail. Unanimous in the
			// panel: `printf "[%s]" "${(q)x}" "${(qq)x}"` writes one line in
			// all six columns, and `set -u; printf "[%s]" "$a" "$b"` names
			// only `a`. Both wrote one line per bad word here.
			break
		}
		// A declaration utility's `name=value` arguments are assignments and
		// expand as ones, which is what keeps `typeset -i n=3*3` from being
		// read as a pattern. Only after the first word is expanded is it
		// known which utility this is, so the test is inside the loop.
		if i > 0 && len(argv) > 0 && assignShaped(w) {
			declaring := r.declarationCommand(c, argv)
			// `set -k` takes the word before the declaration route can,
			// where the dialect says it reaches that far. bash's does and
			// ksh93's does not, which is measured and is why this is asked
			// rather than assumed — see
			// Semantics.KeywordPromotesADeclarationsOperand. A non-declaring
			// command never asks it.
			if r.keywordAssignments && keywordPromotable(w) &&
				(!declaring || r.ask(r.sem().KeywordPromotesADeclarationsOperand,
					"`set -k` reaching a declaration utility's own operand")) {
				if a := keywordAssign(w); a != nil {
					promoted = append(promoted, a)
					continue
				}
			}
			if r.unspecified {
				break
			}
			if declaring {
				argv = append(argv, r.expandAssignArg(w))
				continue
			}
		}
		// expandWord split in two, so the match can be decided between the
		// halves rather than before the word is read.
		fields := r.expandWordEscaped(w)
		// Over fields and not over words: one word can produce several and
		// the front of that list is what carries the modifier —
		// `c=(noglob echo); $c a[b]c` prints the three characters.
		for scanning && len(fields) > 0 {
			m, ok := r.precommand(fields[0])
			if !ok {
				scanning = false
				break
			}
			if m == PrecommandNoGlob {
				// Taken away, and not matched itself: the modifier is a
				// word of the command line and the scan reads it before
				// anything is a pattern.
				noglob = true
				fields = fields[1:]
				continue
			}
			// Transparent: it stays — it is a builtin with work of its own
			// — and the scan carries on, so a modifier may stand behind it.
			argv = append(argv, r.globFieldsUnlessSuppressed(fields[:1], noglob)...)
			fields = fields[1:]
		}
		argv = append(argv, r.globFieldsUnlessSuppressed(fields, noglob)...)
	}
	if len(promoted) > 0 {
		// The command as though the assignments had been written in front of
		// it. A copy rather than a write through the pointer: the tree is the
		// script and a loop body runs it again, so promoting into `c` would
		// make the second pass through a `while` see assignments the source
		// does not hold — and `set +k` inside the loop would then be unable
		// to put them back.
		rewritten := *c
		rewritten.Assigns = append(append([]*syntax.Assign{}, c.Assigns...), promoted...)
		c = &rewritten
	}
	// An array assignment written as an operand — `local a=(x y)` — reaches
	// the utility as the bare name, and the array itself is applied once the
	// utility has made the name local. Splitting it that way is what lets
	// `local` do the one thing only it can do, which is decide the scope the
	// assignment then lands in.
	for _, a := range c.Assigns {
		if a.Operand {
			argv = append(argv, a.Name)
		}
	}
	// A command whose expansion failed, or depended on an axis no dialect
	// answered, does not run. Reporting and then running anyway would be the
	// silent wrong answer this whole structure exists to avoid.
	if r.unspecified {
		r.status = 2
		return nil
	}
	if r.expandErr {
		// A failed expansion abandons the rest of the list rather than
		// running the next command, in every shell measured — which is what
		// the note here said and what the code did not do. It ended the
		// *shell*, and the two readings are the same for a list that is the
		// whole of a line: `echo $((1/0)); echo after` prints nothing more
		// either way. On separate lines they part company, and one dialect
		// prints `after` there — see FailedExpansionAbandonsTheLine, which
		// is the axis, and #1171 for how a single-line probe hid it.
		//
		// *Which* non-zero status it carries is a second axis, asked inside.
		// The diagnostic was already written by whoever failed, so this adds
		// none.
		r.failedExpansion()
		return nil
	}
	if r.ctl == controlExit || r.ctl == controlAbandon {
		// An expansion raised an error of its own — an unmatched pattern,
		// where the dialect or a `shopt` name calls that an error rather
		// than passing it through. The command does not run, and nothing
		// below may overwrite the status it set.
		//
		// **Both control values, because the same failure reaches here as
		// either one.** How far an unmatched pattern unwinds is
		// FailedExpansionAbandonsTheLine's answer, so the shell that gives
		// up the statement arrives with controlAbandon and the shells that
		// stop arrive with controlExit — and a test naming only the second
		// let the first report the complaint and then run the command
		// anyway, printing the pattern it had just refused. r.failedHeading
		// one screen down reads `!= controlNone` for exactly this reason.
		return nil
	}

	if len(argv) == 0 {
		// A command that is only redirections runs the dialect's null
		// command, where the dialect has one. Substituting the word rather
		// than running it here is the whole of the implementation: the
		// command then goes through lookup, redirection, tracing and `$_`
		// exactly as a written one does, which is measured — `set -x; <f`
		// traces `more`, `$_` becomes `more` afterwards, a name that is not
		// there is `command not found` at 127, and a file that will not open
		// stops the command before it runs.
		name, hooked := r.nullCommand(c)
		switch {
		case hooked && name == "":
			// The hook with nothing in it, which is not the same shell as no
			// hook: the command is refused by name rather than quietly
			// opening its files and succeeding, and the refusal abandons the
			// script rather than leaving a status behind. Measured — `NULLCMD=;
			// >f; echo after` prints neither `after` nor a file, and the same
			// line inside `( )` ends the subshell alone. The redirections do
			// not happen either, which is what says the refusal comes before
			// them rather than after.
			r.fatal("%s\n", Wording(r.diag().RedirectionWithNoCommand,
				"redirection with no command"))
			return nil
		case hooked:
			argv = []string{name}
		}
	}

	// `$_` moves to this command's last expanded argument before it runs,
	// so the command's own expansion saw the previous one's — and a bare
	// assignment moves it to empty. Tracked unconditionally and cheaply;
	// whether a read of `$_` answers with it is the dialect's question,
	// asked where the read happens rather than on every command here.
	beforeLastArg, beforeLastArgSet := r.lastArg, r.lastArgSet
	r.noteInputLevelArgument(argv)
	if len(argv) > 0 {
		r.lastArg, r.lastArgSet = argv[len(argv)-1], true
	} else if len(c.Assigns) > 0 {
		r.lastArg, r.lastArgSet = "", true
	}

	if len(argv) == 0 {
		// Assignments with no command name persist, which is the difference
		// between `x=1` and `x=1 cmd`.
		//
		// The status cannot be decided until they have run. `x=1` succeeds
		// and `x=$(false)` reports what the substitution reported, which
		// once made zeroing *first* look right — but `$?` on a right-hand
		// side names the command before the assignment, so zeroing first
		// meant `E=$?` read 0. That is the most common idiom in shell and it
		// was silently returning success; /usr/sbin/apachectl exiting 0
		// where it should exit 1 is what surfaced it.
		//
		// So: run them with the previous status still in place, and decide
		// afterwards from whether a substitution reported anything.
		r.substRan = false
		// And whether a `.set` or `.append` discipline ran, whose status the
		// assignment answers with — see the block below and
		// Runner.disciplineStatus. Cleared here rather than by the store, so
		// that a hook fired by the *previous* command cannot decide this
		// one's status.
		r.disciplineStatusSet = false
		r.assignAll(ctx, c.Assigns)
		if r.ctl == controlExit || r.ctl == controlAbandon {
			// A readonly reassignment is fatal in three of the four shells
			// and abandons the statement in the fourth. Zeroing the status
			// here is what made it look survivable: the script stopped, and
			// then reported success for having done so.
			return nil
		}
		if r.expandErr {
			// A right-hand side that could not be expanded is the same
			// failure a command's arguments suffer, reached one block later:
			// the check above this switch runs *before* the assignments do,
			// so nothing had yet failed when it looked. Without this,
			// `x=$(( } ))` reported the arithmetic out loud and left 0 — so
			// `x=$((…)) || handle` never fired and `set -e` never tripped.
			//
			// The same door a command's failure goes through, which is what
			// keeps the two axes it asks from being answered twice: the
			// status is 1 or 2 by dialect, and bash alone gives up the line
			// and carries on at the next. Measured: `x=$(( } )); echo after`
			// prints no `after` anywhere in the panel, and on two lines bash
			// prints it where dash and zsh stop (#1191).
			r.failedExpansion()
			return nil
		}
		if r.disciplineStatusSet {
			// A discipline ran for one of these assignments, and what it
			// returned is what the command reports. Ahead of the
			// substitution's own status rather than beside it: measured on
			// ksh93u+ 2012-08-01, 2026-09-16, `function s.set { return 5; };
			// s=$(false)` is 5 and the same hook returning 0 is 0, so the
			// hook is the answer and not merely a way of failing.
			//
			// The *last* hook to run wins — `a=1 b=2` with a hook on each
			// reports b's and `b=1 a=2` reports a's — which is what
			// recording rather than combining gives.
			//
			// Only the bare assignment reads this. `typeset s=1`, `read s`
			// and `for s in one` fire the very same hook and all report 0
			// there, and each of them reaches its status through a builtin
			// or a clause rather than through here.
			r.status = r.disciplineStatus
		} else if !r.substRan && !r.assignFailed && !r.unspecified {
			// Nothing in them reported, so the assignment itself does, and
			// an assignment that happens cannot fail. One that was *refused*
			// did fail, which is what assignFailed carries: the dialect that
			// reports a readonly reassignment and carries on leaves 1 in
			// `$?`, and zeroing here said the refusal had not happened.
			//
			// After the fatal check above, not before: an assignment that
			// stopped the script has a status of its own and this would
			// report success for it. An assignment *refused* for want of a
			// dialect is the same case reached the other way — the check
			// above this block runs before the assignments do, so without
			// the guard here a refused subscript reported success.
			r.status = 0
		}
		// `>b` with no command still opens the file, and truncates it if it
		// exists. Returning early skipped that, so a redirection that was
		// the whole command did nothing at all — which is how `echo hi &>b`
		// in a dialect without `&>` came to leave no file behind, the exact
		// silent case the AmpersandRedirect comment warns about.
		if len(c.Redirs) > 0 {
			r.traceCommand(argv)

			// Assignments and a redirection with no command name. There is
			// no other process for a here-document body to expand in.
			closers, err := r.applyRedirs(ctx, c.Redirs, false, false)
			for _, cl := range closers {
				_ = cl.Close()
			}
			if err != nil {
				return err
			}
			if r.redirErr {
				return nil
			}
		}
		// No zeroing here: the status was decided above from what the
		// right-hand sides did. Zeroing here is what once made
		// `set -e; x=$(false)` carry on.
		return nil
	}

	// Whether this command's assignment prefix is traced, and where. Read
	// into locals because both questions are asked again below, once the
	// values have expanded, and neither answer may move in between: `x=1 set
	// +x` is the command that would move the first, and a function defined
	// by a redirection the second.
	tracesPrefix := r.tracesItsPrefix(c.Assigns, argv)
	prefixFollows := tracesPrefix && r.tracePrefixFollowsTheCommand(argv)
	if !tracesPrefix || prefixFollows {
		// With no prefix to write, and in the column that writes it behind
		// the command, the command's own line comes first and is unchanged.
		r.traceCommand(argv)
	}

	// On the record while the redirections are opened, so a dialect that
	// counts a redirection opened for a builtin as the builtin's own can
	// say so. Cleared before the builtin runs: from there on it is the
	// builtin itself that is speaking.
	if len(argv) > 0 {
		// The word itself, whatever it names: a refused `{name}>` store is
		// reported under it whether it is a builtin or a program — measured,
		// `declare -n s; /bin/echo hi {s}>/dev/null` is `/bin/echo: `10':
		// not a valid identifier` in bash 5.3.20.
		r.redirForCommandWord = argv[0]
		if _, ok := r.lookupBuiltin(argv[0]); ok {
			r.redirectForBuiltin = argv[0]
		}
	}
	// A frozen name in the prefix, in the dialect that checks it before
	// anything else this command does. Ahead of the redirections because
	// that is exactly what the order is about: the same shell reports the
	// name and then the file where the others report the file alone. Cleared
	// when the command is over so the flag never outlives it.
	defer func() { r.prefixCheckedFirst = false }()
	// And a subscripted name in the prefix, in the dialect that refuses one.
	// Here for the reason the frozen-name check is here: bash reports it
	// before it expands a value, before it writes a trace line and before it
	// opens a redirection, so `a[1]=$(echo side) f >/nope/x` writes this
	// complaint, then the file's, and never runs the substitution. See
	// interp/prefixsubscript.go.
	r.refuseSubscriptedPrefixes(c.Assigns)
	if r.refusePrefixesEarly(c.Assigns, argv) {
		return nil
	}
	defer func() { r.prefixTraceAssigns, r.prefixTraceValues = nil, nil }()
	if tracesPrefix && !prefixFollows {
		// Ahead of the redirections, which is measured and not incidental:
		// `z=1 cmd >/nope/f` writes `+ z=1` and `+ cmd` and *then* the
		// complaint about the file, in bash and in dash alike. After the
		// frozen-name check, so the column that refuses a prefix before it
		// evaluates anything still evaluates nothing.
		r.expandPrefixTraceValues(c.Assigns)
		r.tracePrefixAndCommand(c.Assigns, argv)
	}

	// And whether the command is one this shell runs itself, which decides
	// where a here-document body is expanded — see heredocprocess.go.
	closers, err := r.applyRedirs(ctx, c.Redirs, false, !r.commandRunsInThisShell(argv))
	r.redirectForBuiltin, r.redirForCommandWord = "", ""
	// Read here rather than in the defer: a builtin that runs a program of its
	// own — `eval`, `.` — applies redirections of its own on the way, and this
	// command's are the ones that were just applied.
	wroteFds := r.redirFds
	defer func() {
		if r.keepRedirs {
			// `exec > log` is the one command whose redirections outlive it.
			// Not closing them is the whole of that: the first closer is what
			// puts the saved streams back, and the rest hold files the script
			// still needs open.
			r.keepRedirs = false
			// And outliving the command is what makes them `exec`'s, which
			// one dialect needs to know: the descriptors it opened this way
			// are the ones it keeps to itself when it runs anything.
			r.markExecOpened(wroteFds)
			return
		}
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	if err != nil {
		return err
	}
	if closers == nil && len(c.Redirs) > 0 && r.status == 126 {
		// The gate refused an open; the status is already set.
		return nil
	}
	if r.redirErr {
		// The open failed — noclobber, a missing directory, a permission.
		// The status is already set and the command does not run.
		//
		// On a *special* builtin that is a fatal error to a non-interactive
		// shell wherever the POSIX rule is being kept, so the shell stops
		// rather than reaching the next command. Asked only there: on
		// anything else no shell in the panel stops, so there is nothing to
		// ask about `true 3>/nope/x`.
		if r.IsSpecialBuiltinHere(argv[0]) &&
			r.ask(r.sem().RedirectErrorOnSpecialBuiltinFatal, "a failed redirection on a special builtin ending the script") {
			stopped := r.ctl == controlExit
			r.fatalUsageQuiet()
			// The shell that stops exits with the *redirection's* status,
			// not the generic fatal one. The two are the same number in
			// every column but one, which is why this read as
			// FatalErrorStatusIsOne's for so long: dash is 2 and 2, bash,
			// ksh93 and zsh are 1 and 1. BusyBox ash is the column that
			// tells them apart — its fatal errors exit 2 (`shift` past the
			// end, `${v?}`, arithmetic, a `<&word` naming no descriptor) and
			// this one exits 1, for `exec`, `:`, `eval`, `.`, `export`,
			// under `set -C` and inside a function alike. Measured
			// 2026-09-16, BusyBox v1.37.0 in the pinned alpine image.
			//
			// Only where the dialect recorded a status of its own; the
			// substrate's zero keeps the fatal answer, so a preset that
			// never measured this does not move. And not where the failed
			// redirection had already stopped the shell on its own — a word
			// after `<&` that named no descriptor is the fatal status in ash
			// too (DuplicationTargetErrorForm).
			if st := r.diag().RedirectFailureStatus; st != 0 && !stopped {
				r.status = st
			}
			return nil
		}
		// One dialect ends the shell over a `<&word` that named something
		// other than a descriptor, and the boundary is the *command*: the
		// same word on an external command complains and carries on there.
		// A function is not a builtin here — the redirection is the caller's
		// and the shell it would end is the same one either way, and no
		// measurement puts a function on the fatal side.
		_, isBuiltin := r.lookupBuiltin(argv[0])
		_, isFunc := r.funcs[argv[0]]
		if r.badDupTarget && !isFunc && isBuiltin {
			r.fatalQuiet()
		}
		return nil
	}

	if prefixFollows {
		// ksh93 writes the assignment once the redirections are open and the
		// value has expanded, which is why this stands here and not beside
		// the branch above: `z=1 cmd >/nope/f` writes the command's line,
		// then the complaint, and no assignment line at all — the value is
		// never reached. Measured 2026-09-16 on ksh93u+ 2012-08-01.
		r.expandPrefixTraceValues(c.Assigns)
		r.tracePrefixAfterTheCommand(c.Assigns)
	}

	// A function shadows a builtin and an external command alike.
	if fn, ok := r.funcs[argv[0]]; ok {
		// A name the shell is still waiting to read a body for is read now,
		// and the definition the file leaves is what runs. Here rather than
		// inside the call, because the call is handed the declaration it
		// found and the load replaces it. See Runner.SetUndefinedFunctionLoader.
		if r.loadUndefinedFunction(ctx, argv[0]) {
			reloaded, still := r.funcs[argv[0]]
			if !still {
				// The loader took the name away — a file that did not
				// define it, which the dialect has already complained
				// about. There is nothing left to call.
				return nil
			}
			fn = reloaded
		}
		// A prefix to a frozen name is refused here, because the call below
		// is where this command ends: only the builtin path assigns through
		// setVarAs, so only that path ever met the refusal, and a prefix to
		// a function was taken in silence. `readonly x=1; x=2 f` reported 0
		// with nothing on stderr where all six panel columns complain.
		// The value is expanded before the refusal is reported, which is what
		// the external route has always done: three of the panel name the
		// expansion's failure and never mention the frozen name, and bash
		// checks the prefix first and never evaluates the value at all.
		// Ours is on the expand-first side, and this is the line that stops
		// the two routes answering it differently — a prefix to a function is
		// otherwise discarded, so its value was never expanded and bash's
		// answer fell out of a gap rather than a choice (#1219).
		if !r.prefixCheckedFirst {
			for _, a := range c.Assigns {
				if !a.Operand && r.readonly[a.Name] {
					r.expandWord(a.Value)
				}
			}
		}
		refused, stop := r.refusePrefixes(c.Assigns, prefixCommand{kind: prefixBeforeFunction}, !r.expandErr)
		if stop {
			return nil
		}
		// A numbered prefix is applied to the shell here, which is measured
		// and is why the acting spelling is called rather than the asking
		// one: `set -- a b; 1=X f` shows the function `X` in `$1`.
		//
		// And a named one is applied too, which is what the whole panel does
		// and what this shell did not: the prefix used to be dropped on the
		// floor here, so `f(){ echo "[$v]"; }; v=1; v=9 f` printed `[1]`
		// where all seven columns print `[9]` (#2407). What happens to it
		// *after* the call, and whether a command the function starts is
		// told about it, are the two axes taken back below.
		var undo []savedVar
		var callHeld []string
		for _, a := range c.Assigns {
			if a.Operand {
				continue
			}
			if r.prefixAssignsPositional(ctx, a) {
				continue
			}
			if refused && r.readonly[a.Name] {
				// The frozen name keeps what it holds, the same way the
				// builtin route leaves it: the body runs with the shell's
				// value, which is what every column shows for `readonly v=1;
				// v=2 f`. Its value was already expanded above, for the
				// refusal's ordering, so skipping it here is also what keeps
				// a command substitution in it from running twice.
				continue
			}
			if r.subscriptedPrefixDropped(a) {
				// Refused already, and refused ahead of its own value: the
				// body runs with the element the shell holds and nothing
				// here evaluates the subscript or the right-hand side.
				continue
			}
			if r.subscriptedPrefixTakenBack(a) {
				undo = append(undo, r.saveVar(a.Name))
			}
			// A prefix to a function persists here, so it is a store and
			// fires the discipline a store fires — with `.append` for `+=`,
			// which is the event the operator names. See
			// interp/prefixdiscipline.go.
			r.prefixStore(ctx, a, true)
			callHeld = append(callHeld, a.Name)
			// The export attribute for the duration, which the two readings
			// move in opposite directions rather than one of them leaving it
			// alone: where the prefix is the command's *environment* the name
			// gains the attribute, and where it is a plain assignment to this
			// shell the name **loses** it. So the axis is asked on every
			// name, including one that was exported before the call —
			// `export v=1; f(){ env; }; v=9 f` hands the child `v=9` in six
			// columns and tells it nothing in ksh93, where `typeset -p v`
			// inside the body prints a plain `v=9`. An earlier reading asked
			// only on a name that was not exported already, on the premise
			// that one that was reaches every child either way; the ksh93
			// column is what that premise is false in.
			answer := r.sem().PrefixToAFunctionIsExported
			if on := r.ask(answer, "an assignment before a function being exported for the call"); on || answer == No {
				if r.exported == nil {
					r.exported = map[string]bool{}
				}
				// Recorded as `false` rather than deleted, because deleting
				// it only makes the name unspoken and an unspoken name the
				// shell was born with is still exported. See isExported.
				r.exported[a.Name] = on
			}
		}
		// The names this call's prefix is holding, for a valueless
		// declaration inside the body that would otherwise hide them — see
		// Runner.prefixEntryShadowed. Put back rather than cleared, because
		// a function calls functions: an inner call written without a prefix
		// of its own is holding nothing, and the outer call's names come back
		// when it returns.
		outerCallHeld := r.functionPrefixNames
		r.functionPrefixNames = callHeld
		defer func() { r.functionPrefixNames = outerCallHeld }()
		// The line the *call* was written on, because the take-back below
		// runs once the body has moved the record to wherever its last
		// command was. A refusal of the axis there is about this command and
		// has to say so.
		callLine := r.line
		defer func() {
			bodyLine := r.line
			r.line = callLine
			r.takeBackFunctionPrefix(undo)
			r.line = bodyLine
		}()
		// `$_` belongs to the *call* and not to the body: whatever the last
		// command inside it was handed, the caller reads what the call was.
		// Unanimous across bash, ksh93 and zsh, and #3134's first defect.
		defer r.underscoreAcrossAFunctionCall(beforeLastArg, beforeLastArgSet)()
		return r.callFunc(ctx, fn, argv[1:])
	}

	// A builtin runs in this shell, which is the whole reason it is one:
	// `set` and `shift` change state a child process could not.
	if fn, ok := r.lookupBuiltin(argv[0]); ok {
		// An assignment prefixed to a builtin is visible to the builtin
		// while it runs — `IFS=: read x y` splits on the colon — and is
		// taken back afterward. The exception is a *special* builtin,
		// where POSIX has the assignment persist; dash and ksh93 follow
		// that and bash and zsh do not, so the dialect answers it.
		// A frozen name is decided before anything is assigned, and decided
		// once for the whole prefix: whether it is refused at all, whether
		// the builtin still runs, and whether the script ends here. Ahead of
		// the loop below because a refusal that costs the command must not
		// have applied the names in front of it first.
		kind := r.prefixCommandOf(argv)
		refused, stop := r.refusePrefixes(c.Assigns, kind, true)
		if stop {
			return nil
		}
		if kind.throughCommand && r.aPrefixSuppliesThePath(c.Assigns) {
			// `command` is a precommand word, so a PATH in front of it is a
			// PATH in front of whatever it runs — and the command hash
			// answers that the same way it answers a bare external command.
			// Before the loop below, because the loop is where the
			// assignment empties the table. See
			// holdCommandHashAcrossAPrefixedPath.
			defer r.holdCommandHashAcrossAPrefixedPath()()
		}
		var undo []savedVar
		var held []string
		for _, a := range c.Assigns {
			if a.Operand {
				// An argument to the builtin, not a prefix to it.
				continue
			}
			if r.prefixAssignsPositional(ctx, a) {
				// The parameters are not in the table the undo below saves,
				// so this one is not taken back — which is measured, not a
				// gap. See Runner.prefixAssignsPositional.
				continue
			}
			if refused && r.readonly[a.Name] {
				// Reported above, or deliberately not reported where the
				// dialect says a regular builtin's prefix is no refusal at
				// all. Either way the name keeps its value: the builtin runs
				// with what the shell already holds, which is what every
				// column shows a child through `export x; readonly x=1;
				// x=2 env`.
				continue
			}
			if r.subscriptedPrefixDropped(a) {
				// Refused already, ahead of its own value — see the function
				// route above and interp/prefixsubscript.go.
				continue
			}
			if prefixIsSubscripted(a) && r.subscriptedPrefixReachesAChild(kind) {
				// `command` in front of an external: a child runs it, an
				// array reaches no child's environment, and the element is
				// not this shell's to keep. The value is still expanded,
				// which is measured — `arr[1]=$(echo side >&2; echo v)
				// command /usr/bin/true` writes `side` — so it is expanded
				// here and nothing is done with it.
				_ = r.prefixExpansion(a)
				continue
			}
			if (!r.IsSpecialBuiltinHere(argv[0]) || !r.ask(r.sem().AssignmentPrefixPersistsOnSpecialBuiltin, "an assignment before a special builtin persisting")) &&
				r.subscriptedPrefixTakenBack(a) {
				undo = append(undo, r.saveVar(a.Name))
			}
			// Whether a discipline hears about it is whether there is a
			// store for it to hear about. A *regular* builtin's prefix is the
			// environment it is handed and nothing else, so the assignment
			// below is this shell showing it the value rather than a write
			// the name's owner is told of; a special builtin's persists, and
			// a prefix that reached here through `command` before an
			// external is the child's store. See interp/prefixdiscipline.go.
			r.prefixStore(ctx, a, kind.kind != prefixBeforeRegularBuiltin)
			held = append(held, a.Name)
			if kind.throughCommand {
				// `command` is a precommand word rather than a command, so
				// the prefix in front of it belongs to whatever it goes on
				// to run — and when that is an external, belonging to it
				// means being in its environment. Setting the name is not
				// enough for that: a child is handed the *exported* names,
				// so `v=1; v=9 command env` showed the child nothing where
				// bash 5.3, ksh93, zsh and dash all show it `v=9` (#2408).
				// Unanimous, so it is done here and not asked.
				//
				// Only through `command`. A prefix before a builtin that
				// runs text of its own is where the panel splits — bash
				// shows the child `v=9` for `v=9 eval env` and the other
				// three show it nothing — so exporting for every builtin
				// would take three columns with it to answer one.
				//
				// The attribute is put back by restoreVars along with the
				// value: savedVar records the export tri-state precisely so
				// a name that was never exported goes back to unspoken
				// rather than to a recorded `false`.
				if r.exported == nil {
					r.exported = map[string]bool{}
				}
				r.exported[a.Name] = true
			} else if on, moves := r.prefixExportAtABuiltin(); moves {
				// Every other builtin, where the panel does split and the
				// attribute moves in both directions: bash hands the builtin
				// an environment and exports the name, ksh93 reads the prefix
				// as an ordinary assignment and takes the attribute off a
				// name that had it, and the rest leave it exactly as it was.
				// See Semantics.PrefixExportAtABuiltin.
				if r.exported == nil {
					r.exported = map[string]bool{}
				}
				r.exported[a.Name] = on
			}
		}
		// The names this command's prefix is holding, for the declaration
		// that may keep one of them — and the outer pair put back first,
		// because a builtin can run another one. See Runner.prefixHeldNames.
		outerHeld, outerKept, outerShadowed := r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed
		outerUndo := r.prefixHeldUndo
		r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed = held, nil, nil
		r.prefixHeldUndo = undo
		defer func() {
			kept, shadowed := r.prefixKeptNames, r.prefixShadowed
			r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed = outerHeld, outerKept, outerShadowed
			r.prefixHeldUndo = outerUndo
			r.restoreVarsExcept(undo, kept, shadowed)
		}()
		// The builtin is on the record for the duration, so a dialect that
		// names it in a diagnostic's location can. Saved and put back rather
		// than cleared: a builtin can run another one.
		outer := r.inBuiltin
		r.inBuiltin = argv[0]
		// `readonly a=(x)` has to set the array *before* the name is locked,
		// because locking it first refuses the very assignment the command
		// was given. Every other declaration utility wants the opposite
		// order: `local a=(x)` must make the name local first, or the array
		// lands in the caller's scope.
		//
		// Which is also what puts `readonly` on the other side of it in the
		// dialect that reads the word as its own `typeset -r`: there a
		// `readonly -a A=(x y)` inside a function is declaring a local, so
		// assigning first lands the array in the *caller's* scope and the
		// local comes into being empty. Where that is the reading it takes
		// the second order and the deferred freeze with it, which is the
		// same answer `declare -ar A=(x y)` has always needed — see
		// Semantics.ReadonlyDeclaresALocal.
		locks := argv[0] == "readonly" && !r.readonlyScopesItsOperands()
		outerFreezing := r.freezing
		// Recorded whichever order the two halves run in, because the
		// refusal it feeds belongs to the *declaration* and `readonly -i
		// z=(1 2)` is refused in the same words as `typeset -i z=(1 2)`.
		outerLiterals := r.literalOperands
		r.literalOperands = arrayLiteralOperands(c)
		outerCompounds := r.compoundOperands
		r.compoundOperands = compoundLiteralOperands(c)
		// Recorded by the builtin as it reads its letters, and read by the
		// operand assignments that run after it — so it is cleared here
		// rather than seeded, and restored beside literalOperands for the
		// same reason: a builtin can run another one.
		outerIndexed := r.indexedLetterHere
		r.indexedLetterHere = nil
		outerTable := r.tableLetterHere
		r.tableLetterHere = nil
		if locks {
			r.assignOperands(ctx, c)
		} else {
			// `declare -ar A=(x y)` has the same problem `readonly` solves by
			// assigning first, and cannot solve it the same way — the array
			// has to land after the shadow. So the *freeze* waits instead:
			// the operand is part of the declaration that is applying `-r`,
			// so `-r` cannot be what refuses it. Without this the refusal
			// added below turned `declare -ar A=(x y)` into a refusal of its
			// own value, and `declare -Ar M=([k]=v)` with it.
			r.freezing = operandNames(c)
		}
		st := r.callBuiltin(ctx, argv[0], fn, argv[1:])
		r.inBuiltin = outer
		fatal := false
		if st == 0 && !locks {
			r.assignOperands(ctx, c)
			switch {
			case r.ctl == controlExit:
				// Something here was fatal, and a fatal error has already set
				// the status its dialect gives one. The builtin's own 0 must
				// not be written over it below: two of the four end the
				// script on a refused declaration operand and both reported 0
				// for having done so.
				//
				// Reached only where the builtin itself reported 0, which is
				// what keeps this away from the option refusals — those are
				// fatal in one dialect too, and the status they report is the
				// one the corpus pins.
				fatal = true
			case r.assignFailed:
				// A refused operand is the declaration's own failure, and
				// biDeclare cannot see it: the operand assignments land after
				// the builtin has returned, so the 0 it reported for the
				// attributes it did apply stood over the value it did not.
				// `readonly A; declare -a A=(p q)` said `A: readonly
				// variable` and reported 0. Every other spelling of the same
				// refusal already reports 1 — see biDeclare's own check.
				st = 1
			}
		}
		r.applyDeferredFreeze()
		r.freezing = outerFreezing
		r.literalOperands = outerLiterals
		r.compoundOperands = outerCompounds
		r.indexedLetterHere = outerIndexed
		r.tableLetterHere = outerTable
		if fatal {
			return nil
		}
		// A builtin can consult an axis of its own — `echo` asks about
		// backslash escapes — so the check is repeated after it runs as well
		// as before, and its status is discarded when one went unanswered.
		if r.unspecified {
			r.status = 2
			return nil
		}
		r.status = st
		return nil
	}

	// A builtin this shell does not have is refused rather than looked for on
	// PATH. See reserved.go: the alternative is running a child that changes
	// its own state and exits, which is how `umask 077` came to succeed and
	// do nothing.
	if r.reservedBuiltin(argv[0]) {
		return r.unsupported(argv[0] + ", which is a shell builtin and cannot be run as a command")
	}

	// An assignment prefix applies to this command's environment only.
	//
	// A frozen name is decided first, and for the whole prefix at once, the
	// same way the builtin route decides it. The report waits for the value
	// to have expanded below, because the order of the two is a dialect
	// question and not this one's.
	external := prefixCommand{kind: prefixBeforeExternal}
	env := r.environ()
	// The PATH the prefix supplies, if it supplies one, held so that the
	// search below is made with it — see reachPrefixedPath.
	prefixPath, pathFromPrefix := "", false
	for _, a := range c.Assigns {
		if a.Operand {
			// Unreachable as things stand — every name that takes an operand
			// assignment is a builtin, so no external command ever gets here
			// with one. Kept so that the two kinds are told apart wherever
			// assignments are read, rather than in some of the places.
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok {
			// A positional parameter in front of an external command is the
			// one place the construct does nothing: `set -- a b; 1=X
			// /bin/echo hi` leaves `a b`, and `1=X /usr/bin/env` shows the
			// child no `1`. Not applied and not exported, which is both
			// halves of that.
			continue
		}
		if r.prefixCheckedFirst && r.readonly[a.Name] {
			// Refused before anything was expanded, which is what the order
			// axis buys: the value is never evaluated, so `x=$((1/0)) cmd`
			// says nothing about the division in the column that checks
			// first. The name keeps its value and the child sees that, the
			// same as below.
			continue
		}
		if r.subscriptedPrefixDropped(a) {
			// Refused, and refused ahead of its own value: the child is
			// handed nothing for the name and no substitution in the
			// right-hand side runs. See interp/prefixsubscript.go.
			continue
		}
		value := r.prefixValue(a)
		if prefixIsSubscripted(a) {
			// A child runs this command, so an element store would be the
			// child's — and an array reaches no child's environment, so
			// there is nothing to hand over either. The value has expanded
			// above because that much is measured: `arr[1]=$(echo side >&2;
			// echo v) /usr/bin/true` writes `side` in ksh93 and zsh and
			// leaves the element where it was.
			continue
		}
		// The report waits for the value to have expanded, because the order
		// of the two is a dialect question and not this change's: bash checks
		// the prefix before it expands anything and before it opens a
		// redirection, so `x=$((1/0)) cmd` and `x=2 cmd >/nope/f` complain
		// about `x` there and about the expansion and the file everywhere
		// else. Ours is on the everywhere-else side on both, and staying
		// there is what keeps this change to the half the panel agrees on.
		if r.readonly[a.Name] {
			// Refused, so nothing is appended: the assignment did not happen,
			// and the child is handed this shell's own environment for the
			// name. The two are only distinguishable with the name exported,
			// which is how it was measured — `export x; readonly x=1; x=2
			// env` shows the child `x=1` in bash and bash 3.2, and no column
			// in the panel ever shows it `x=2`. We showed it `x=2` and said
			// nothing, which is the silent half of #1219: a refusal that
			// hands the refused value on is not a refusal.
			continue
		}
		if a.Name == "PATH" {
			// The last one wins, the same way the child's environment
			// resolves `PATH=/a PATH=/b cmd`: what is appended last is what
			// reaches it.
			prefixPath, pathFromPrefix = value, true
		}
		env = append(env, a.Name+"="+value)
	}
	if _, stop := r.refusePrefixes(c.Assigns, external, !r.expandErr); stop {
		return nil
	}
	if pathFromPrefix {
		// **And the search is made with it**, which is the half that was
		// missing: the child was handed the new PATH and this shell went on
		// looking with the old one, so `PATH=/nowhere ls` ran `ls` where
		// every column in the panel reports 127 (#2626). Applied to the
		// shell rather than threaded into the search as a second PATH, so
		// that lookPath goes on having one answer to "which PATH is this" —
		// which is the same reason it reads this Runner's rather than the
		// process's.
		defer r.reachPrefixedPath(prefixPath)()
	}
	return r.exec(ctx, argv, env)
}

// prefixValue is what an assignment prefix hands the command it stands in
// front of.
//
// The expansion, and — where the prefix was written `name+=value` — the
// name's current value in front of it. Unanimous: with `v` holding `14`,
// `v+=5 env` shows the child `v=145` in bash 5.3, bash 3.2, ksh93 and zsh,
// and every one of them leaves the shell's own `v` at `14` afterwards. We
// read the append operator on the assignment and then ignored it here, so
// the child was shown the tail alone — `PATH+=:/x cmd` handed the command a
// PATH of `:/x` (#2299).
//
// The name's value and not the environment's: an append reads what the shell
// holds, whether or not the name is exported, which is what the panel shows
// for a name that was never exported at all.
func (r *Runner) prefixValue(a *syntax.Assign) string {
	return r.prefixJoined(a, r.prefixExpansion(a))
}

// prefixExpansion is the prefix's own value, before an append has anything to
// join it to. Split out of prefixValue because a store fires the event the
// operator names, and `.append` is entered with the appended part alone —
// see interp/prefixdiscipline.go.
func (r *Runner) prefixExpansion(a *syntax.Assign) string {
	if value, ok := r.prefixTraceValue(a); ok {
		// Already expanded, to be written by `set -x` before the command
		// runs. Reading it back rather than expanding again is what keeps
		// `x=$(date) cmd` from running the substitution twice under a trace
		// and once without one — and it is *this* function and not
		// prefixValue that holds the check, because a store reaches the
		// expansion through here without the join.
		return value
	}
	return strings.Join(r.expandWord(a.Value), " ")
}

// prefixJoined puts the name's current value in front of an append's, and is
// the identity for a plain assignment.
func (r *Runner) prefixJoined(a *syntax.Assign, value string) string {
	if !a.Append {
		return value
	}
	old, _ := r.getVar(a.Name)
	return old + value
}

func (r *Runner) exec(ctx context.Context, argv, env []string) error {
	r.execSerial = r.stmtSerial
	// This runner's PATH, not the process's — see lookpath.go for why that
	// distinction is the whole bug and not a detail.
	path, lookErr := r.lookPath(argv[0])
	if lookErr != nil {
		// A word nothing would run may still be somewhere to go — see
		// autoCdInstead, which answers false in every shell that has not
		// asked for it. Before the exec gate rather than after, because a
		// word that turned out to be a `cd` is not an execution: nothing is
		// started, and the directory it moves to went past the file-system
		// gate on the way in.
		if st, took := r.autoCdInstead(ctx, argv); took {
			r.status = st
			return nil
		}
		path = argv[0]
	}
	if lookErr == nil {
		// The command hash: a bare name that PATH resolved, remembered at
		// the moment it is about to run. Before the gate rather than after,
		// because every shell in the panel hashes what it resolved whether
		// or not the run then succeeds — and a policy that refuses the exec
		// is this shell's own answer, not a fact about where the command is.
		//
		// A `command -p` search is the exception, and it is one for a reason
		// that is about the hash rather than about the run: what it resolved
		// was never on the caller's PATH. See Runner.rememberingLookups.
		if r.rememberingLookups() {
			r.hashCommandRun(argv[0], path)
		}
	}
	action := r.act(Action{Kind: ActionExec, Path: path, Args: argv})
	if !r.allowed(ctx, action) {
		return nil
	}
	if lookErr != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: lookErr})
		// 127 for a name that resolved to nothing and 126 for a file that is
		// there and will not start. Both unanimous, and collapsing them into
		// one number was the other half of this bug: `./noexec` reported
		// "command not found" where every shell says "Permission denied".
		r.status = r.cannotRun(lookErr, naming{
			bare:     r.diag().NotFound,
			fallback: "%[1]s: not found",
		})
		return nil
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})

	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	// A command names itself from argv[0], and what it should find there is
	// the word that was typed rather than the path PATH resolved to. os/exec
	// sets both from the one argument, so the two have to be pulled apart
	// again: `basename --bad` complains as `basename` in every shell in the
	// panel and complained as `/usr/bin/basename` here.
	cmd.Args[0] = argv[0]
	ownGroup := (r.bg != nil && r.monitor) || (r.bg == nil && r.WaitForCommand != nil)
	if ownGroup {
		// A process group of its own, which is what makes signaling and
		// terminal ownership answerable at all — for a foreground command as
		// much as a background one, once there is something able to notice it
		// stopped.
		//
		// A background job gets one **only while the monitor is on**, which is
		// what every shell in the panel does and what this did not (#1738).
		// Measured 2026-09-11 with the monitor off — the default on any route
		// that is not a prompt — by having the job ask the kernel for its own
		// group: bash 5.3.15, dash, ksh93 and zsh 5.9.2 all answer the
		// *shell's* group, and this answered a group of its own. A job in a
		// group of its own is a job the shell can signal as a group, hand the
		// terminal to and stop; promising that with the monitor off promises
		// something the option says is not happening.
		//
		// The foreground half is unchanged and deliberately so: it is not a
		// promise about job control but the only way a shell that is watching
		// a command can stop or interrupt it, and it is asked of
		// WaitForCommand — which is the front end saying there is something
		// able to notice.
		setProcessGroup(cmd)
	}
	if pgid, ok := r.anchoredGroup(); ok {
		// Inside a process substitution's body that has a process group of
		// its own, a command **joins that group** — which is what a real
		// shell's fork would have done without anybody deciding, and is what
		// makes the teardown the script writes reach the daemon it started.
		//
		// After the block above rather than instead of it: the two disagree
		// only about which group, and this is the narrower answer. A group of
		// its own would leave the body's children outside the group the body
		// handed the script, so `kill -- -$pgid` would reach the placeholder
		// and nothing else. See procanchor.go.
		setProcessGroupIn(cmd, pgid)
		// And then the group is the substitution's rather than this
		// process's, so a signal aimed at the job must be aimed at the
		// process: the group holds the anchor and every other command the
		// body started.
		ownGroup = false
	}
	cmd.Dir = r.Dir
	cmd.Env = env
	// The fields rather than the resolved streams, so that a nil one reaches
	// os/exec as nil and the child is given /dev/null. That is the same
	// emptiness the shell's own reads and writes get, spelled the way a
	// process spells it — and cheaper, since a non-file reader or writer
	// makes os/exec build a pipe and copy through it.
	//
	// A stream the script *closed* is the one thing that must not arrive as
	// that emptiness, and childIn and childOut are where the difference is
	// kept: an empty descriptor reads end-of-file and a closed one fails.
	cmd.Stdin = childIn(r.Stdin)
	cmd.Stdout = childOut(r.Stdout)
	cmd.Stderr = childOut(r.Stderr)
	// The descriptors past the three named streams, rebuilt into the child's
	// own table — see childFiles for why that has to be done by hand and why
	// the numbering is preserved rather than packed.
	cmd.ExtraFiles = r.childFiles()

	if r.bg != nil {
		// Started rather than run, so the pid can be recorded before it is
		// waited for — `$!` has to be answerable immediately. Through
		// startMasked, because a child inherits the file-creation mask at the
		// fork and this shell's is not the process's. See umaskscope.go.
		if err := r.startMasked(cmd); err != nil {
			// A file the kernel will not start may still be a shell script,
			// which is this shell's to run — see noexecscript.go. Asked at
			// every door a start can fail at, because a door that did not ask
			// would be the same bug in a different room.
			if st, ran := r.imageAsScript(ctx, action, path, argv, env, err); ran {
				r.status = st
				return nil
			}
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			r.diagf("%s: %v\n", argv[0], err)
			r.status = 126
			return nil
		}
		// The pid is final now, so anything waiting to read `$!` may proceed
		// while this goroutine blocks on the process. Once, and settlePID says
		// why: a job that starts a second external command reaches this again,
		// by which time the shell has already read the field.
		r.bg.settleStartedPID(cmd.Process.Pid, ownGroup)
		r.tookJobProcess(cmd.Process.Pid, ownGroup)
		r.status = r.waitForBackgroundProcess(cmd)
		r.releasedJobProcess(cmd.Process.Pid)
		r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
		return nil
	}

	// From here the shell starts a process and waits for it, which is where a
	// coprocess that ended is noticed — registered after the background
	// branch above, which returns before reaching this and whose wait is on a
	// goroutine that owns no coprocess. See Runner.retireCoproc.
	defer r.retireCoproc()

	if r.WaitForCommand != nil {
		return r.runWatched(ctx, cmd, argv, action, ownGroup)
	}

	err := r.startAndWait(cmd, ownGroup)
	r.addChildTime(cmd.ProcessState)
	var ee *exec.ExitError
	switch {
	case err == nil:
		r.status = 0
	case errors.As(err, &ee):
		r.status = r.exitStatus(err)
		if sig, killed := killedBy(err); killed {
			r.diedOfSig = sig
			r.reportKilled(sig, cmd.Process.Pid)
		}
	default:
		if st, ran := r.imageAsScript(ctx, action, path, argv, env, err); ran {
			r.status = st
			return nil
		}
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
	}
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
	return nil
}

// waitForBackgroundProcess waits for a background job's process, and — where
// the shell is in a position to notice — sees it *stop* rather than only end.
//
// os/exec's own Wait is an ordinary waitpid, so a job stopped by SIGSTOP or
// SIGTSTP never comes back from it: the process is still there, it is simply
// never going to finish. That is the whole of #2227. A `wait` for such a job
// blocked for as long as something outside the shell took to resume or kill
// it, and a `jobs` listing went on calling it `Running` for the same reason —
// nothing in this shell had been told otherwise.
//
// The caller's wait — the driver's, with WUNTRACED — is the one that can be
// told, which is why this reaches for r.WaitForCommand exactly as runWatched
// does for a foreground command. A stop is recorded on the job and the wait
// resumed: the job has not ended, and what the shell *does* about a job it
// now knows is stopped belongs to `wait` and to the listing rather than here.
//
// Only while the monitor is on, which is the same condition the job's own
// process group is given under, a few lines above. With it off a `&` job runs
// in the shell's own group, stopping it is not a job-control act at all, and
// no shell in the panel gives up a `wait` for one: measured 2026-09-12 with
// `sleep 97 & kill -STOP $!; wait`, bash 5.3.15, that bash as `sh`, bash
// 3.2.57, ksh93u+ and zsh 5.9.2 all sit there. So with it off this stays the
// plain os/exec wait it has always been, and nothing moves.
func (r *Runner) waitForBackgroundProcess(cmd *exec.Cmd) int {
	if !r.monitor || r.WaitForCommand == nil || r.bg == nil {
		return r.exitStatus(cmd.Wait())
	}
	pid := cmd.Process.Pid
	for {
		w, err := r.WaitForCommand(pid)
		if err != nil {
			// Nothing to be learned from the front end's wait, so fall back
			// to os/exec's: it is the one that still holds the child.
			return r.exitStatus(cmd.Wait())
		}
		if w.Stopped {
			r.bg.noteStopped(w.Signal)
			// Waited for again rather than answered. A stopped job has not
			// ended, and the next thing this wait returns is whatever
			// happens to it after something resumes it.
			continue
		}
		// The command has ended, so os/exec's bookkeeping is closed out the
		// way runWatched closes it — the child is already reaped by the wait
		// above, and this joins the goroutines copying its streams.
		_ = cmd.Wait()
		if cmd.ProcessState != nil {
			r.addChildTime(cmd.ProcessState)
		} else if r.elemCPU != nil {
			r.elemCPU.add(w.User, w.System)
		}
		status, _ := r.waitResult(w)
		return status
	}
}

// runWatched runs a foreground command through the caller's own wait, which is
// the only kind that can report a command that *stopped*.
//
// Started rather than run: the wait is the caller's, so this must not also be
// waiting — two waits on one child is a race over who reaps it, and the loser
// gets an error instead of a status.
func (r *Runner) runWatched(ctx context.Context, cmd *exec.Cmd, argv []string, action Action, ownGroup bool) error {
	if err := r.startMasked(cmd); err != nil {
		if st, ran := r.imageAsScript(ctx, action, cmd.Path, argv, cmd.Env, err); ran {
			r.status = st
			return nil
		}
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
	}
	pid := cmd.Process.Pid
	r.tookJobProcess(pid, ownGroup)
	defer r.releasedJobProcess(pid)
	// The command's own group is the terminal's foreground group while it
	// runs, so ^C and ^Z reach it rather than this shell. Taken back
	// afterwards however it ended — a shell that left the terminal with a
	// stopped job would have no way to read the next line.
	if r.Foreground != nil {
		if err := r.Foreground(pid); err != nil {
			// Not fatal: a shell with no controlling terminal — a script, a
			// pipeline — has no foreground group to set, and the command
			// still runs.
			r.Foreground = nil
		} else {
			defer func() { _ = r.Foreground(0) }()
		}
	}
	var w Wait
	for {
		var err error
		w, err = r.WaitForCommand(pid)
		if err != nil {
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			r.diagf("%s: %v\n", argv[0], err)
			r.status = 126
			return nil
		}
		if !stoppedWait(w) || r.monitor {
			break
		}
		// Stopped, and this shell is not watching jobs — so it waits the
		// command out rather than answering with it. Unanimous: measured
		// 2026-09-12 with a script whose background job stops its own
		// foreground command and lets it go again half a second later, bash
		// 5.3.15, bash 3.2.57, ksh93u+ and zsh 5.9.2 all report the
		// command's own 0 when it finally ends, for SIGSTOP and SIGTSTP
		// alike. With `set -m` bash 5.3 answers with the stop instead, which
		// is the branch below and what an interactive shell always reaches:
		// a shell with a terminal runs the monitor.
		//
		// There is nobody to tell either — a job the script cannot see is a
		// job it cannot resume — so waiting again is also the only ending
		// that does not strand the process.
	}
	if !stoppedWait(w) {
		// The command has ended, so os/exec's own bookkeeping can be closed
		// out: it copies to and from a stream that is not a file on
		// goroutines of its own, and nothing joins them but Wait. The error
		// is discarded because the child is already reaped — that *is* the
		// arrangement — and the reaping is not what this call is for.
		_ = cmd.Wait()
		if cmd.ProcessState != nil {
			r.addChildTime(cmd.ProcessState)
		} else if r.elemCPU != nil {
			// The caller's wait reaped the child, so os/exec never saw its
			// end — but the wait that reaped it read its usage, and Wait
			// carries the figures here.
			r.elemCPU.add(w.User, w.System)
		}
	}
	status, stopped := r.waitResult(w)
	r.status = status
	if w.Killed {
		r.diedOfSig = w.Signal
		// Told the signal directly rather than through an error: this path
		// exists because only the caller's own wait can see a command that
		// *stopped*, and it reports what ended one just the same.
		r.reportKilled(w.Signal, pid)
	}
	if stopped {
		// Still there, so it becomes a job rather than a result. The prompt
		// comes back and the command is waiting to be told to go on.
		r.addStoppedJob(pid, argv, w.Signal)
		// And the loops it was inside end, or a ^Z would leave the shell
		// going round again with the command it was waiting for suspended —
		// which is a loop that runs *faster* for having been suspended, and
		// cannot be stopped at all.
		r.breakLoopsForAStop()
	}
	if w.Killed && w.Signal == syscall.SIGINT && r.Interactive {
		// The interrupt went to the command rather than to this shell — the
		// terminal was its process group's — so there was no signal here to
		// hear, and what it did is the only evidence. Same ending either way:
		// the line is given up.
		r.abandonForInterrupt()
	}
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
	return nil
}

// environ is the environment a command receives: r.Env, minus what `unset`
// took away, plus exported names and carried functions. Never os.Environ —
// the process environment is shared by every Runner in the program, and a
// script's view of it must be whatever its embedder handed in, which for a
// shell binary is driver seeding Env at construction. Nil Env is therefore
// genuinely empty: nothing reaches a command but what the shell exported.
func (r *Runner) environ() []string {
	base := r.Env
	out := make([]string, 0, len(base)+len(r.Vars))
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			// Nothing to key on, so nothing can supersede or hide it; it is
			// passed along as written rather than dropped.
			out = append(out, kv)
			continue
		}
		if r.removed[k] {
			// A name the shell unset does not reach a command either: the
			// child would otherwise see what the parent cannot.
			continue
		}
		if !r.isExported(k) {
			// The attribute came off after the name was inherited. The shell
			// goes on reading it — see inheritedEnv — and a child no longer
			// sees it at all.
			continue
		}
		if _, own := r.Vars[k]; own {
			// Assigned since it was inherited, and the assignment supersedes
			// what came in rather than joining it below. Handing a child the
			// same name twice is not a tidiness question: the stale entry is
			// the *first* of the two, and execve leaves it that way.
			continue
		}
		if live, bound := r.producedOptionList(k); bound {
			// An option record is produced, so what a child must be handed
			// is this shell's options *now* and not the string this shell was
			// launched with. Nothing else in Vars can supersede it — it is
			// readonly and never stored — so this entry is the only place the
			// stale value could reach a command, and it is where it did:
			// a shell handed `xtrace` that then ran `set +x` would still have
			// been turning tracing on in everything it started. Measured, the
			// shell that has this variable hands the recomputed value down.
			//
			// Asked of every bound namespace rather than of the core's alone,
			// which is what stops a dialect's second one (`$BASHOPTS`) being
			// the copy that keeps its startup string.
			out = append(out, k+"="+live)
			continue
		}
		// A name **inherited** and never assigned since reaches a child as
		// the text that came in, `.get` discipline or no `.get` discipline —
		// which is not what a read of it in the shell answers. Measured on
		// ksh93u+ 2012-08-01, 2026-09-16, with `G=raw` in the environment:
		//
		//	function G.get { .sh.value=HOOKED; }; print "$G"     HOOKED
		//	the same, then `env`                                 G=raw
		//	the same, then `export G; env`                       G=raw
		//	the same, then `G=new; env`                          G=HOOKED
		//
		// So the hook is asked for a name the shell holds and not for one it
		// is merely passing along, and the row that assigns first says why:
		// the assignment moves the name into this shell's own table, which is
		// the loop below and where the hook is asked. This loop used to ask
		// it too, and passed it the *name* rather than the value — the first
		// half of a cut it had already made — so a script with any hook
		// anywhere in it handed every command it ran a `PATH=PATH`.
		out = append(out, kv)
	}
	// The functions this shell was told to carry, written as source because
	// there is nothing but a string to carry them in.
	out = append(out, r.functionEnviron()...)
	for k, v := range r.Vars {
		// Only exported names reach a command's environment; the rest are
		// the shell's own.
		if !r.isExported(k) {
			continue
		}
		if entry, compound := r.exportedCompound(k); compound {
			// A name holding an array or a table has no environment
			// representation, and the columns part over whether it reaches a
			// child at all — see
			// Semantics.ExportedCompoundReachesAChildAsItsFirstValue. The
			// scalar view this store keeps in step is *not* the answer: it
			// handed a child the first element in every dialect, and an
			// empty entry for an empty array, which is nobody's.
			if entry != "" {
				out = append(out, k+"="+entry)
			}
			continue
		}
		if r.declaredEmpty[k] {
			// Declared rather than assigned, so the name has no value of
			// its own and an exported name with no value reaches no child
			// in any shell measured — including the one whose declaration
			// makes the shell itself read it as empty.
			continue
		}
		// A child is told the case-folded value, not the text the assignment
		// carried: measured, `typeset -l v=AB; export v` puts `v=ab` in the
		// environment in every shell with the letter.
		//
		// The environment is **not** a read like any other, though, and the
		// width attribute is where the two part: measured 2026-09-12 on zsh
		// 5.9.2, `typeset -L 5 b=xy; export b` puts `b=xy` in the
		// environment — the raw text, five characters short of what `$b`
		// answers in the shell itself — while `typeset -u a=ab` in the same
		// run puts `a=AB` there. So the padding is a presentation of the
		// parameter and the fold is a property of the value, which is why
		// this calls the case one by name rather than the pair.
		//
		// And a `.get` discipline is asked, because the environment is a
		// *read* of the name and this is the one reader that was built from
		// the tables instead. Measured on ksh93u+ 2012-08-01, 2026-09-16:
		// `g=raw; function g.get { .sh.value=A; }; export g; env` hands the
		// child `g=A`, not the `raw` the store holds. See
		// interp/discipline.go.
		out = append(out, k+"="+r.exportedThroughDiscipline(k, r.readCaseFolded(k, v)))
	}
	// A **table** keeps no scalar view, so the loop above never sees one and
	// the dialect that hands a child its first value would hand it nothing.
	// Sorted, for the reason zeroValuedTypeExports is: a child's environment
	// must not depend on a map walk.
	out = append(out, r.exportedTables()...)
	for k, v := range r.hiddenExports {
		out = append(out, k+"="+v)
	}
	out = append(out, r.zeroValuedTypeExports()...)
	return out
}

// exportedTables is the environment entries the exported keyed tables earn,
// which no other pass produces: a table has no scalar view in Vars, so the
// walk over that map cannot see one.
func (r *Runner) exportedTables() []string {
	var out []string
	for k := range r.AssocArrays {
		if _, own := r.Vars[k]; own || !r.isExported(k) {
			continue
		}
		if entry, _ := r.exportedCompound(k); entry != "" {
			out = append(out, k+"="+entry)
		}
	}
	sort.Strings(out)
	return out
}

// exportedCompound is the environment entry an exported name holding an array
// or a table is given, and whether the name holds one at all.
//
// An empty string with compound true means no entry: either the dialect hands
// a child nothing for a compound, or the compound has nothing in it. The two
// come to the same thing for a child — measured, ksh93 refuses to export an
// empty array at all and the other two hand one nothing — so the emptiness is
// not a second question.
func (r *Runner) exportedCompound(name string) (string, bool) {
	values, held := r.compoundValues(name)
	if !held {
		return "", false
	}
	if !r.ask(r.sem().ExportedCompoundReachesAChildAsItsFirstValue,
		"an exported name holding a compound reaching a child at all") {
		return "", true
	}
	if len(values) == 0 {
		return "", true
	}
	return values[0], true
}

// compoundValues is what a name holds when it holds an array or a table, in
// the order that array or table lists them, and whether it holds one.
func (r *Runner) compoundValues(name string) ([]string, bool) {
	if a, ok := r.assocFor(name); ok {
		return a.values(), true
	}
	// The *stored* array and not arrayElems, which reads a scalar back as
	// the array of one it otherwise is — every exported name would have
	// looked like a compound and reached no child at all.
	if a, ok := r.Arrays[name]; ok {
		return r.readArray(a), true
	}
	if produce, ok := r.DynamicArrays[name]; ok {
		return produce(r), true
	}
	return nil, false
}

// zeroValuedTypeExports is the exported names whose declaration named a
// numeric *type* and which hold no value at all, each handed to a child as
// `0` — see Semantics.NumericTypeWithNoValueReachesAChildAsZero.
//
// Sorted, because the environment a child is handed must not depend on a map
// walk: two runs of the same script would otherwise order the entries
// differently and nothing in the shell would look wrong.
//
// The name really is unset in the shell that does this — `typeset -i Z;
// export Z` leaves `${Z+set}` empty and `typeset -p Z` writing `typeset -x -i
// Z` with no value — so this cannot come from the store. What it comes from
// is the attribute: the type is what the declaration named, and zero is what
// the type makes of nothing.
func (r *Runner) zeroValuedTypeExports() []string {
	var names []string
	for name, on := range r.exported {
		if !on || r.declaredNameHolds(name) {
			continue
		}
		if _, float := r.floatPrecision[name]; !float && !r.integer[name] {
			continue
		}
		if !r.ask(r.sem().NumericTypeWithNoValueReachesAChildAsZero,
			"an exported name of a numeric type with no value reaching a child as zero") {
			return nil
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, name+"=0")
	}
	return out
}

// hiddenExports yields the names a declaration left with no value of their
// own, and the value each one shadowed.
//
// A local that inherited the export attribute and was then left unset reaches
// a command with the value it hid, rather than with nothing: the shell reads
// `${FOO-UNSET}` as unset inside the function and a child is still told
// `FOO=bar`. That is the one reading measured for the combination, and it is
// reachable only from it — a dialect whose valueless declaration leaves the
// outer value showing has nothing hidden to tell a child about, and one whose
// local does not inherit the attribute records nothing here.
//
// The record dies with the scope that made it, which is what puts the outer
// value back on return. Innermost first, because the value handed over is the
// one the nearest declaration hid: two functions deep, the caller's local is
// what the callee shadowed and the global is out of reach behind it.
func (r *Runner) hiddenExports(yield func(name, value string) bool) {
	var seen map[string]bool
	for i := len(r.scopes) - 1; i >= 0; i-- {
		for name, value := range r.scopes[i].exportedShadow {
			if seen[name] {
				continue
			}
			if seen == nil {
				seen = map[string]bool{}
			}
			seen[name] = true
			if _, own := r.Vars[name]; own {
				// Assigned since, so the local has a value of its own — but
				// it reaches a child only if the local carries the export
				// attribute, and `+x` on the declaration takes it off. So
				// this is two cases and not one: with the attribute the loop
				// above has already handed the local's value over, and
				// without it that loop passed the name by and the binding
				// standing behind it is still the one a child is told about.
				//
				// Measured 2026-09-06, `env -i`, from a file and through
				// `-c` alike: `export FOO=bar; f() { local +x FOO=z; env; }`
				// hands the child `FOO=bar` in bash 5.3.15, bash as `sh` and
				// bash 3.2.57 — the outer value, not `z` — while the shell
				// itself reads `z`. `export FOO` inside the same function
				// puts the attribute back on the local and the child is then
				// told `z`, and with the outer binding never exported the
				// child is told nothing, which is what says it is the outer
				// binding speaking rather than a value the local kept.
				//
				// Skipping unconditionally told the child nothing at all,
				// because neither list claimed the name.
				if r.isExported(name) {
					// Skipped rather than emitted and superseded: a
					// duplicate name in an environment is settled by execve
					// keeping the first, and this list is deduplicated the
					// other way round.
					continue
				}
			} else if !r.removed[name] {
				// No value of its own and still visible — the dialect's
				// valueless declaration left the outer value showing — so
				// the loops above have it.
				continue
			}
			if !yield(name, value) {
				return
			}
		}
	}
}

// isExported says whether a name reaches a command's environment.
//
// Two sources, and the order between them is the whole of it. An explicit
// answer wins: `export` puts one there and `export -n` and `declare +x` put
// the opposite there, which is why the record is a tri-state — recorded true,
// recorded false, and never spoken about — rather than a set of names.
//
// Unspoken, the environment answers. **A name the shell inherited is already
// exported**, because being in a command's environment is what carrying it in
// the environment means, and POSIX has an imported variable keep the attribute
// for the shell's whole life. Nothing recorded that, so `FOO=bar sh -c 'FOO=baz;
// cmd'` gave the child `bar`: the new value went into r.Vars, r.exported never
// heard the name, and environ() went on handing out the entry the shell was
// born with. `PATH=/new:$PATH; make` is the same bug where it costs something —
// every child gets the old PATH while the shell's own lookup is right.
//
// `unset` is the one thing that takes the attribute off without saying so.
// It is a removal rather than an assignment, so the name goes back to being
// one this shell has never heard of, and assigning to it afterwards makes an
// ordinary shell variable. All four shells agree.
func (r *Runner) isExported(name string) bool {
	if on, spoken := r.exported[name]; spoken {
		return on
	}
	if r.removed[name] {
		return false
	}
	_, born := r.bornWith(name)
	return born
}

// inheritedEnv walks what the shell was born with, minus what `unset` took
// away: the names a lookup falls back to once the runner's own tables have
// come up empty.
//
// Deliberately not environ(), which is a different list in both directions.
// A carried function is in that one and is not a variable; a name whose export
// attribute has been taken off is a variable this shell can still read and no
// child is told about. Reading values out of the list meant for children is
// what made those two the same question.
func (r *Runner) inheritedEnv(yield func(name, value string) bool) {
	for _, kv := range r.Env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || r.removed[k] {
			continue
		}
		if !yield(k, v) {
			return
		}
	}
}

// bornWith is one name looked up in what the shell was born with, and says
// nothing about whether `unset` has since taken it away — that is the
// caller's question and each of the two asks it differently.
//
// The whole of this is a scan that does not split. inheritedEnv is the right
// shape for the four callers that want *every* name, and the wrong one for
// the two that want one: walking it called strings.Cut on all 88 entries of a
// real environment and consulted `removed` for each, then threw away
// everything but the entry that matched. That was 30% of a function-call
// benchmark — every variable lookup falling through the runner's own tables
// pays it — and the work was never needed, because an entry either starts
// with `NAME=` or cannot be the name however it splits (#2036).
//
// Deliberately no cached map. interp/clonetables.go records what one costs
// here: a table shared with a clone that runs on a goroutine ends the process
// with `fatal error: concurrent map read and map write`, which is not a Go
// panic and which panicguard cannot catch. This needs no new state to be
// fast.
//
// The first match wins, which is the answer the split walk gave for a
// duplicated name and is what a shell handed two `X=` entries reads.
func (r *Runner) bornWith(name string) (string, bool) {
	for _, kv := range r.Env {
		// The `=` has to be exactly where the name ends: `PATHX=1` must not
		// answer for `PATH`, and an entry with no `=` at all cannot match.
		if len(kv) > len(name) && kv[len(name)] == '=' && kv[:len(name)] == name {
			return kv[len(name)+1:], true
		}
	}
	return "", false
}

// inheritedValue is the value a name was born with, if the shell was handed
// one and `unset` has not taken it away.
func (r *Runner) inheritedValue(name string) (string, bool) {
	if r.removed[name] {
		return "", false
	}
	return r.bornWith(name)
}

// scope records the variables a function made local, and what they were.
type scope struct {
	saved   map[string]string
	existed map[string]bool
	// savedArrays is the same for arrays, which are a second table: shadowing
	// only Vars left `f() { local a; a=(x y); }` setting a *global* array,
	// because nothing had saved the array of that name to put back.
	savedArrays  map[string]Array
	arrayExisted map[string]bool
	// removedBefore is whether `unset` had already hidden the name when it
	// was shadowed, so a hiding `local` can be undone without resurrecting
	// an environment value the script had taken away — or forgetting one it
	// had not.
	removedBefore map[string]bool
	// declaredOnlyBefore is whether the name's compound value had come from a
	// declaration alone when it was shadowed, so a `local -A m` that empties
	// nothing does not leave the caller's table reading as though a
	// declaration had just made it. Saved beside the two compound tables and
	// put back with them. See compounddeclaredonly.go.
	declaredOnlyBefore map[string]bool
	// savedAssoc shadows the associative table the same way, attribute and
	// all: what comes back on exit is whether the name was associative as
	// much as what it held.
	savedAssoc   map[string]AssocArray
	assocExisted map[string]bool
	// savedReadonly is whether a shadowed name was frozen when the
	// declaration displaced it. Taking the attribute off is a change to the
	// runner's record and has to be put back like the value — otherwise the
	// outer name comes back assignable, which is a frozen name quietly
	// thawed by a function call.
	//
	// Written for *every* name a declaration shadows and not only for the
	// ones that arrived frozen, because the entry has two jobs and the
	// second one needs the false. A `false` here says the shell had nothing
	// frozen under this name when the call took it, so anything frozen by
	// the time the call returns was frozen *by the call* and goes away with
	// it: `f() { local -r y=1; }; f; y=2` leaves y writable in bash, ksh93
	// and zsh alike, and recording only the true left the caller's y frozen
	// for the rest of the script.
	//
	// So absent and false do not mean the same thing here, though they do in
	// r.readonly: absent means this scope never shadowed the name at all.
	savedReadonly map[string]bool
	// savedHideInScope is the hide-in-scope attribute a shadowed name
	// carried when the declaration displaced it, saved for the same two
	// reasons savedReadonly is: the outer name gets its own attribute back,
	// and one this call added goes away with the call. Absent and false do
	// not mean the same thing here either — absent means this scope never
	// shadowed the name. See hideinscope.go.
	savedHideInScope map[string]bool
	// hiddenShadow is whether the shadow this scope took over a name is a
	// *hidden* one — an ordinary parameter that merely happens to be spelled
	// like one of the shell's own, rather than a second view of it. Written
	// for every name the scope shadows, because absent and false are
	// different questions here too: absent means this scope never shadowed
	// the name, and the innermost scope that did is the one that answers.
	//
	// Separate from savedHideInScope, which is a *value* being kept for the
	// return, because the two disagree the moment either `-h` or `+h` is
	// written on the declaration — and separate from r.hideInScope, which
	// after the shadow describes the fresh binding and no longer the outer
	// one that governed it. See hideinscope.go.
	hiddenShadow map[string]bool
	// suspendedProducers is what a hidden shadow took out of the produced
	// tables, so that the name reads and writes as an ordinary parameter for
	// as long as the shadow stands and is the shell's own again on return.
	// See hideinscope.go.
	suspendedProducers map[string]suspendedProducer
	// savedAttrs is every other attribute a shadowed name carried — integer
	// and its base, float and its precision, the two case letters, unique
	// and hidden. Saved and taken off at the shadow and put back on return,
	// for the two reasons savedReadonly is: a local is a fresh binding that
	// inherits none of them, and one this call added goes away with the
	// call. Absent means this scope never shadowed the name; a zero value
	// means it shadowed one that carried nothing. See localattributes.go.
	savedAttrs map[string]nameAttributes
	// savedAssigned is what a *produced* parameter was last assigned when the
	// declaration displaced it, and assignedSpoken is whether it had been
	// assigned at all — the same tri-state savedExported keeps, and for the
	// same reason: "nothing was ever assigned" is a state, and a single map
	// could not tell it from "assigned the empty string".
	//
	// A produced parameter's assignment does not live in Vars, so shadowing
	// Vars alone shadowed nothing. `f() { local -i SECONDS=1024; }; f` left
	// SECONDS reading 1024 for the rest of the session, where zsh 5.9.2 has
	// it counting from the call again — and powerlevel10k measures its prompt
	// inside exactly that shape, `local -i COLUMNS=1024`, so the width it
	// forced for the measurement escaped into the session and every prompt
	// afterwards was drawn 1024 columns wide (#2107).
	savedAssigned  map[string]string
	assignedSpoken map[string]bool
	// savedExported and exportedSpoken are the export attribute a shadowed
	// name had, for the dialects where a local does not inherit it. Taking
	// the attribute off is a change to the runner's record and has to be put
	// back like the value, and the record is a tri-state — so what was there
	// is two maps rather than one: whether it had been spoken about at all,
	// and what it said.
	savedExported  map[string]bool
	exportedSpoken map[string]bool
	// exportedShadow is what an exported name held when this scope's
	// declaration took it out of view — the value a child is told for a
	// local that inherited the attribute and was left with none of its own.
	// See hiddenExports.
	exportedShadow map[string]string
	// keyword records that the function was defined with the `function` word
	// rather than with parentheses. ksh93 gives only those functions a local
	// scope, so `typeset` needs to know which kind it is standing in.
	keyword bool
	// sealed is what the enclosing calls' declarations were, in the dialect
	// whose bodies do not see them — put aside on the way in and put back on
	// the way out. Nil in every other dialect, and in this one for a call
	// with nothing declared below it. See staticscope.go.
	sealed map[string]sealedName
	// owner is the runner whose call pushed this scope.
	//
	// A subshell is a clone that shares the stack, so a scope reached from
	// one is not necessarily one it may write on — see Runner.ownScope,
	// where the difference is a trap the parent set being cleared by a
	// `( … )` written inside the same function.
	owner *Runner

	// optindShadowed says a declaration in this call made OPTIND local, and
	// savedOptChar and savedOptindAssigned are the half of the `getopts`
	// position that is not a parameter: how far into a clustered word the
	// scan had read, and whether the number standing there was one the
	// script wrote.
	//
	// Saved beside the value rather than with it, because it is not in Vars
	// and a scope that shadowed Vars alone handed the caller a cursor
	// pointing at the start of a word it had already part-read. What that
	// costs is not a wrong letter: a loop whose body calls a function
	// declaring `local OPTIND` starts the same word over every time round
	// and never runs out of options (#2226). Scalars rather than a map
	// because there is one name — see Semantics.GetoptsLocalOptindRestoresTheCursor.
	optindShadowed      bool
	savedOptChar        int
	savedOptindAssigned bool

	// optindCallCursor says this call was handed a `getopts` cursor of its
	// own on the way in, so the caller's intra-word half is held by the
	// call's restore rather than by a declaration's — see
	// Semantics.GetoptsFunctionPosition. It is what tells the declaration's
	// restore that "the body left the position where the declaration put
	// it" no longer means the caller's place is unchanged: the entry reset
	// moved it first, so the two answers do differ and the axis has to be
	// asked.
	optindCallCursor bool

	// optindCursorDropped records that a declaration of OPTIND in this call
	// asked GetoptsLocalOptindRestoresTheCursor and the answer was no, so
	// the caller's place *inside* a word does not come back. Read by the
	// call's own restore, which would otherwise put it back over the top
	// and make the answer that was never measured win.
	optindCursorDropped bool

	// savedTraps is what this call displaced while its dialect was scoping
	// traps to the function, keyed by the canonical condition. Nil until the
	// first such modification, so a call that traps nothing carries nothing.
	//
	// A map rather than a list because the entry is per condition and the
	// *first* save is the one that counts: a body that moves the same
	// condition twice goes back to what it found, not to what it set first.
	// See localtraps.go.
	savedTraps map[string]savedTrapState

	// trapTableWasTaken says the whole table was taken and emptied when this
	// call was entered, rather than one condition being saved as the body
	// moved it. The two are different shells' answers to the same question
	// and they need different returns: this one has to clear whatever the
	// body set before putting the snapshot back, because a condition the
	// call raised that the caller never had is not in the snapshot at all.
	// See localtraps.go.
	trapTableWasTaken bool

	// savedOptions is the shell's whole `set -o` table as this call found
	// it, where the dialect scopes options to a `function`-word call. Nil
	// for every other call, which is every call in five of the six columns.
	//
	// A snapshot taken on the way in rather than a save per modification,
	// because the shell that has this hands the body the caller's table
	// *live* and puts the whole of it back: the body can read an option the
	// caller moved, which a per-modification save would also get right, and
	// it can be left holding a name the caller never touched, which that
	// save would not. See localsetoptions.go.
	savedOptions map[string]bool

	// onReturn is what runs when this call unwinds, in reverse order of
	// registration.
	//
	// The seam a dialect needs for state the substrate does not hold. One
	// shell's `emulate -L` and its LOCAL_OPTIONS make every option change
	// in a function last only as long as the call, and the options live in
	// the dialect rather than here — so what this offers is the *moment*,
	// not the state. Closures rather than a saved copy of anything, because
	// the substrate cannot know what it would be copying.
	onReturn []func()
}

// fatal reports an error that abandons the script.
//
// Every fatal error goes through here so the three things that make one are
// decided in a single place: the diagnostic, the status — which is an axis,
// dash saying 2 where the others say 1 — and the unwinding. Setting the
// status and forgetting the unwinding is the bug this replaces, and it had
// been written independently at three sites.
// fatalAtStatus is fatalQuiet for a failure that carries its own number.
//
// Rare, and it has to be measured to be used: the status of a fatal error is
// normally Semantics.FatalErrorStatusIsOne's, and a caller that answers it
// itself is saying the panel's columns do not line up with that axis here.
// `break abc` is the case — bash answers Yes to the axis and reports 2 for
// this, where ksh93 and zsh report 1 (#2800).
//
// The axis is not asked at all on this path, which is the point: asking it
// and then discarding the answer would report an unanswered axis to a
// strict-core shell over a number it never used.
func (r *Runner) fatalAtStatus(status int) {
	r.status = status
	r.ctl, r.abandon, r.errexitStopped = controlExit, abandonError, false
}

// fatalQuiet is fatal for a failure that has already reported itself.
func (r *Runner) fatalQuiet() {
	r.setFatalStatus()
	// An error rather than a request to stop, which is what lets a boundary
	// reading a file of its own give up that file and carry on. Set here for
	// the reason the status and the unwinding are: every fatal error comes
	// through this one door, so nothing else has to remember to say so.
	r.ctl, r.abandon, r.errexitStopped = controlExit, abandonError, false
}

func (r *Runner) fatal(format string, args ...any) {
	r.diagf(format, args...)
	r.fatalQuiet()
}

// fatalUsageQuiet is fatalQuiet for a builtin's complaint about **how it was
// called** — see abandonUsage and Semantics.BuiltinUsageErrorEscapesBorrowedText.
//
// A door of its own rather than a flag set before fatalQuiet: the kind and the
// unwinding are written together everywhere else in this file, and a "mark the
// next fatal" flag is the shape of state a site forgets to clear.
func (r *Runner) fatalUsageQuiet() {
	r.fatalQuiet()
	r.abandon = abandonUsage
}

// fatalUsage is fatalUsageQuiet for a complaint this call still has to write.
func (r *Runner) fatalUsage(format string, args ...any) {
	r.fatal(format, args...)
	r.abandon = abandonUsage
}

// beginHeading clears what expanding a compound command's *heading* is about
// to answer, so that failedHeading asks about this heading and not about
// whatever ran before it.
//
// A simple command already does this — it is the same three fields — and a
// heading needs it for the same reason and one more: a failed expansion that
// gave up its line leaves the flag set, and the statement loop that consumed
// the give-up does not clear it. A heading that tested the flag without
// clearing it first would abandon itself over the previous line's failure.
func (r *Runner) beginHeading() {
	r.unspecified, r.expandErr, r.badSubscript = false, false, false
}

// failedHeading reports whether expanding a compound command's heading
// failed, having ended what the dialect says such a failure ends.
//
// The heading is the `for` word list, the `select` menu, the `case` subject,
// a `case` pattern, the parts of an arithmetic `for` and an array literal's
// element list — everything expanded *before* the construct decides what to
// run or what to store. It is a separate call from the simple command's
// because it guards a different thing: there, a failure means the command
// does not run; here it means the loop must not iterate, the `case` must not
// choose an arm and the assignment must not be made.
//
// Not choosing is the half that matters most. A subject that failed expands
// to empty, and empty *matches* — measured, `case "$((1/0))" in "") echo E;;
// *) echo A;; esac` reported the division and then ran the `""` arm, so the
// shell picked a branch from a value it had just said it could not compute,
// and exited 0 (#1215).
//
// Three questions, and the third of them used to be written outside this
// function at each site that remembered it. A heading can fail three ways: an
// axis no dialect answered, an expansion that reported through the flags, and
// an expansion that raised a **fatal error of its own** — an unmatched
// pattern where the dialect calls that an error — which reports through r.ctl
// and sets neither flag. The `case` subject and the `case` pattern each
// carried their own `|| r.ctl != controlNone` beside the call; the `select`
// menu and the array literal did not, and each was a report-then-do-it-anyway
// bug of its own: the menu was drawn from a word list the shell had refused
// (#1176) and the assignment was made from it (#1568). One clause, inside,
// rather than a fourth and fifth copy of it outside.
//
// The order is what keeps a failure from being reported twice. r.ctl is
// consulted before r.expandErr, so a heading that has already unwound is not
// sent through failedExpansion a second time to have its status rewritten.
func (r *Runner) failedHeading() bool {
	if r.unspecified {
		// An axis no dialect answered, already reported where it was asked.
		// The construct does not run, and the status is the one ask left.
		r.status = 2
		return true
	}
	if r.ctl != controlNone {
		// The heading raised a fatal error of its own, or the script is
		// being abandoned for a reason of its own. Either way the construct
		// does not run and the status it set stands.
		return true
	}
	if !r.expandErr {
		return false
	}
	r.failedExpansion()
	return true
}

// failedExpansion ends what a command whose expansion failed should end.
//
// One dialect gives up the *line* and carries on at the next one where the
// other three end the shell — see Semantics.FailedExpansionAbandonsTheLine
// for the measurement, and controlAbandon for what the line means. The
// diagnostic was already written by whoever failed, so this adds none.
//
// The status is the same either way and is FatalErrorStatusIsOne's, because
// it is the same failure: the shell that abandons the line leaves 1 behind on
// it, which is what the next command overwrites when it succeeds. That is why
// `echo pre; echo "${(P)x}"; echo after` exits 1 and the same three commands
// on three lines exit 0 — nothing about the status changed, only how much
// stopped running.
//
// The axis is **read** rather than asked, which is Runner.setArrayLetter's
// reason one construct over: where the answer is not yes, ending the shell is
// what the standard describes, what three of the four shells do, and what
// this path already did — so an unanswered axis has a correct answer to fall
// back on rather than a missing one to complain about. It would also be the
// second complaint on this path in a run with no dialect, since fatalQuiet
// asks FatalErrorStatusIsOne and the core does not answer that either.
func (r *Runner) failedExpansion() {
	if r.sem().FailedExpansionAbandonsTheLine != Yes {
		r.fatalQuiet()
		return
	}
	if r.badSubscript {
		// A bracketed expression gives up more than a bare one: see
		// Runner.giveUpForABadSubscript, which is where that is measured and
		// which the two builtin-operand sites already went through (#3502).
		r.giveUpForABadSubscript()
		return
	}
	r.setFatalStatus()
	r.abandonTheCommand()
}

// failedSubscript is what an element assignment the shell *refuses* ends: the
// diagnostic, the dialect's fatal status, and then exactly as much unwinding
// as that dialect does for any other failed expansion.
//
// A door of its own rather than Runner.fatal, because fatal ends the shell and
// one column does not. Measured 2026-09-16 from a script file — the refusal on
// one line and `echo "NEXT=$?"` on the next, which is the pairing this
// question has to be asked on, since on one line giving up the list and giving
// up the shell print the same nothing:
//
//	                            bash 5.3  bash 3.2  zsh 5.9  ksh93
//	a[1+]=q                     NEXT=1    NEXT=1    stops    stops
//	a[1+]+=q                    NEXT=1    NEXT=1    stops    stops
//	v="1+"; a[$v]=q             NEXT=1    NEXT=1    stops    stops
//	a=(1 2 3); a[-9]=q          NEXT=1    NEXT=1    —        —
//	a=([1+]=x)                  NEXT=1    NEXT=1    stops    —
//	a=(1 2 3); a[1]=(q)         NEXT=1    NEXT=1    —        —
//
// That is Semantics.FailedExpansionAbandonsTheLine's split exactly, and the
// axis already names "a bad subscript" among the failures it governs — these
// sites were simply never wired to it and ended the file instead. An em dash
// is a column that does not refuse the line at all and so never arrives here.
//
// Two neighboring subscript refusals already spell the unwinding out by hand
// — Runner.assignWholeArraySubscript and Runner.assocAssignKey — and both are
// reached by one column only, so neither had an axis to read. These four are
// reached by three, which is why this one asks.
func (r *Runner) failedSubscript(format string, args ...any) {
	r.diagf(format, args...)
	// A subscript, whatever wording reached here: the `-c` rule on
	// Runner.giveUpForABadSubscript is a rule about brackets and the door
	// below cannot see them. Set rather than assumed, because
	// Runner.subscriptFailure is only one of the two ways a caller words
	// this — an out-of-range index and a whole-array subscript have
	// sentences of their own (#3502).
	r.badSubscript = true
	r.failedExpansion()
}

// setFatalStatus is the status half of fatalQuiet, for the caller that wants
// the number without the unwinding.
// setFatalStatus writes the dialect's generic status for a fatal error.
//
// **A builtin's own return value wins over it**, because the dispatcher writes
// what the builtin returned after the control flow is set — so a builtin that
// reports a usage error, ends the script and returns 2 ends it at 2 however
// this answers. That is the reading the panel wants: measured 2026-09-13,
// `shift -x`, `export -q`, `return abc` and `exit status` end a shell in POSIX
// mode at 2 in bash 5.3.15 under `set -o posix`, in that build under the name
// `sh`, and in dash, ksh93 and BusyBox ash, while bash's generic fatal status
// is 1 — a failed redirection on a special builtin ends it at 1 in the same
// shell. bash 3.2.57 is the one column that parts from it, ending `shift -x`
// at 1 while its own `export -q` ends at 2; no dialect here claims that build.
//
// So a caller on that path returns the builtin's status and never `r.status`,
// and an assignment to r.status in front of fatalQuiet is dead either way.
// Both facts were invisible while only dash, ksh93 and BusyBox ash could reach
// it — all three answer 2 to both questions — and bash reaching the same path
// through POSIX mode is what told them apart (#2583).
func (r *Runner) setFatalStatus() { r.status = r.fatalStatus() }

// fatalStatus is that number without setting it, for the one caller that has
// to choose between it and another number before either is written. Split
// out rather than copied, because a second reading of the axis is a second
// place for it to be answered differently.
func (r *Runner) fatalStatus() int {
	if r.ask(r.sem().FatalErrorStatusIsOne, "the exit status of a fatal error") {
		return 1
	}
	return 2
}

func (r *Runner) setVar(name, value string) { r.setVarAs(name, value, assignedAnyhow) }

// savedVar is one variable's state before a transient assignment, held so the
// assignment can be taken back: the value it had, whether it was set at all,
// and whether `unset` had removed it from view.
//
// The compound stores are held too, because a scalar assignment is what
// *takes an array away* now — see scalarOverCompound. Putting only the scalar
// back left `a=(p q); a=x builtin` with the string `p` where every shell in
// the panel leaves the two-element array it had, since the prefix is taken
// back whole or not at all.
type savedVar struct {
	name    string
	value   string
	present bool
	removed bool
	array   Array
	inArray bool
	table   AssocArray
	inTable bool
	// The export attribute is part of the state, and it is a tri-state
	// rather than a flag: recorded on, recorded off, and never spoken about.
	// A prefix that exports the name for the length of a call has to be able
	// to put the *silence* back, not merely record `false` — a name nobody
	// has ever exported and one `export -n` took the attribute off are
	// different things to the shell that inherited it. See Runner.isExported.
	exported     bool
	exportSpoken bool
	// frozen is whether the name was readonly at the moment the prefix was
	// applied, so a declaration the command ran over it does not leave the
	// attribute behind. `b=7; b=8 readonly b` gives the name back to the
	// shell at `7` and *writable* in zsh 5.9.2, and `y=2 typeset -r y` the
	// same — the attribute went on the temporary the prefix made and leaves
	// with it. Measured 2026-09-16 (#3437).
	//
	// A plain flag and not the export attribute's tri-state: readonly is
	// recorded in one map with no environment behind it, so absent and
	// `false` are the same thing.
	frozen bool
	// attrs are the declaration attributes the name carried then — the
	// integer, case, float and width letters — for the same reason: a
	// declaration the command ran leaves none of them behind. `i=1; i=2
	// typeset -i i` lists `declare -- i="1"` in bash 5.3.20 and `typeset -g
	// i=1` in zsh 5.9.2, with no letter on either. Measured 2026-09-16
	// (#3437).
	//
	// The whole struct rather than a letter at a time, because it is the same
	// capture a local's shadow takes and the two must not come to disagree
	// about what an attribute is. See Runner.captureAttributes.
	attrs nameAttributes
	// assigned is the message a *produced* parameter's producer was last
	// sent, and a tri-state for the reason the export attribute is: a name
	// nobody has assigned and one assigned the empty string are different
	// starting points to a producer that counts from what it was given.
	//
	// An assignment to a produced name never lands in Vars at all — see
	// setVarAs, where it becomes a message in Runner.assigned — so putting
	// the four stores above back left the message standing and the seeding
	// outlived the command. `SECONDS=100 true; echo $SECONDS` read 100 here
	// against 0 in bash 5.3.15, bash 3.2.57 and zsh 5.9.2, and ksh93 gives
	// the same name back to its own clock (#2713).
	assigned       string
	assignedSpoken bool
	// produced is what the producer was *answering* at the moment the prefix
	// was applied, for the narrower set of names whose producer has a writer:
	// there the assignment is delivered rather than left to be found, so
	// putting the message back tells the producer nothing and the take-back
	// has to send it one of its own. Nil for every other name, which is
	// almost all of them — reading a producer costs a call, and a name with
	// no writer has nothing to be told.
	produced *string
	// partner is the other half of a tie, saved alongside, and nil for the
	// overwhelming majority of names that are not tied to anything. See
	// saveVar: a tie is two names for one value and writing either moves
	// both, so a prefix on one of them has changed two cells and giving one
	// back is giving half of it back.
	partner *savedVar
}

// saveVar takes the whole of a name's state, for a prefix that will give it
// back.
//
// The two compounds are **copied** and not merely referred to. Both stores are
// maps, so holding the one that is there holds a view of whatever the prefix
// then does to it: an element write reaches through and the restore puts back
// the array it had already changed. `a=(p q); a=x true` came back
// `([0]="x" [1]="q")` in the dialects that write the first element, which is
// the value the prefix was supposed to have taken with it.
//
// **A tie is two cells and both are saved.** Writing the scalar half of
// `typeset -T PATH path` splits it into the array half, so a prefix on one
// name moves the other one too — and this saved only the name that was
// written. Measured against zsh 5.9.2 with a scratch HOME, where the two are
// the shell's own pair: `PATH=/nowhere true; echo $path` leaves `path` holding
// what it held, and ours left it holding `/nowhere` — the value the prefix was
// supposed to have taken with it, in the other half of the same tie. The
// partner is saved *alone* so that saving it does not walk back to this name
// and never stop.
func (r *Runner) saveVar(name string) savedVar {
	u := r.saveVarAlone(name)
	if t, tied := r.tieOf(name); tied {
		other := t.scalar
		if other == name {
			other = t.array
		}
		if other != name {
			p := r.saveVarAlone(other)
			u.partner = &p
		}
	}
	return u
}

// saveVarAlone is saveVar for one cell, with no tie followed.
func (r *Runner) saveVarAlone(name string) savedVar {
	old, present := r.Vars[name]
	a, inArray := r.Arrays[name]
	m, inTable := r.AssocArrays[name]
	exported, exportSpoken := r.exported[name]
	assigned, assignedSpoken := r.assigned[name]
	return savedVar{
		name: name, value: old, present: present, removed: r.removed[name],
		frozen: r.readonly[name], attrs: r.captureAttributes(name),
		array: a.clone(), inArray: inArray,
		table: m.clone(), inTable: inTable,
		exported: exported, exportSpoken: exportSpoken,
		assigned: assigned, assignedSpoken: assignedSpoken,
		produced: r.producedValueToPutBack(name),
	}
}

// producedValueToPutBack is what the producer of a name is answering right
// now, for a name whose producer has a writer — and nil for every other name.
//
// The narrow gate is the whole of the design. A prefix to a produced name is
// *delivered*: setVarAs calls the writer, and a writer is registered exactly
// where the assignment has to do something rather than be found later. So the
// take-back has to send a message too, and the message is the value the
// producer was giving before this one arrived. A name with no writer needs
// none — putting Runner.assigned back is the whole of its state — and calling
// its producer would be a read of RANDOM or of the clock for nothing.
func (r *Runner) producedValueToPutBack(name string) *string {
	if _, hasWriter := r.dynamicWriters[name]; !hasWriter {
		return nil
	}
	produce, ok := r.Dynamic[name]
	if !ok {
		return nil
	}
	value := produce(r)
	return &value
}

// wasExported reports what isExported would have answered for this name at the
// moment it was saved, resolving the same tri-state the same way: a recorded
// attribute decides, a removed name is exported to nobody, and otherwise the
// name is exported exactly if the shell was born with it.
func (u savedVar) wasExported(r *Runner) bool {
	if u.exportSpoken {
		return u.exported
	}
	if u.removed {
		return false
	}
	_, born := r.bornWith(u.name)
	return born
}

// restoreVars takes back transient assignments, most recent first.
func (r *Runner) restoreVars(undo []savedVar) {
	for i := len(undo) - 1; i >= 0; i-- {
		r.restoreVar(undo[i])
	}
}

// restoreVar puts one name — and the other half of its tie, if it has one —
// back the way it was saved.
func (r *Runner) restoreVar(u savedVar) {
	if u.partner != nil {
		// First, so that a mirror driven by the restore below writes over
		// the partner rather than the partner writing over it. Nothing here
		// goes through setVarAs, so no mirror runs at all — but the order is
		// the one that stays right if one ever does.
		r.restoreVar(*u.partner)
	}
	if u.present {
		r.Vars[u.name] = u.value
	} else {
		delete(r.Vars, u.name)
	}
	if u.inArray {
		if r.Arrays == nil {
			r.Arrays = map[string]Array{}
		}
		r.Arrays[u.name] = u.array
	} else {
		delete(r.Arrays, u.name)
	}
	if u.inTable {
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[u.name] = u.table
	} else {
		delete(r.AssocArrays, u.name)
	}
	if u.removed {
		if r.removed == nil {
			r.removed = map[string]bool{}
		}
		r.removed[u.name] = true
	} else {
		delete(r.removed, u.name)
	}
	if u.exportSpoken {
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported[u.name] = u.exported
	} else {
		delete(r.exported, u.name)
	}
	if u.frozen {
		if r.readonly == nil {
			r.readonly = map[string]bool{}
		}
		r.readonly[u.name] = true
	} else {
		delete(r.readonly, u.name)
	}
	r.restoreAttributes(u.name, u.attrs)
	if u.assignedSpoken {
		if r.assigned == nil {
			r.assigned = map[string]string{}
		}
		r.assigned[u.name] = u.assigned
	} else {
		delete(r.assigned, u.name)
	}
	// And the producer is *told*, where the prefix told it something. The
	// store above is what a producer reads for itself; a writer is what the
	// shell calls, and one that moved a line editor's cursor or a prompt has
	// to be moved back rather than merely un-recorded. See
	// producedValueToPutBack.
	if u.produced != nil {
		if write, ok := r.dynamicWriters[u.name]; ok {
			write(r, *u.produced)
		}
	}
}

// takeBackFunctionPrefix ends an assignment prefix that stood in front of a
// *function* call, which is one axis away from ending one that stood in front
// of a builtin.
//
// The names are given back what they held, unless the dialect says a prefix to
// a function is a plain assignment to the shell that outlives the call — ksh93
// alone in the panel. See Semantics.AssignmentPrefixPersistsAfterAFunction.
//
// **Asked one name at a time, and only where the two readings differ.** A name
// the body left holding exactly what it held before the call reaches the same
// place under either answer, so there is nothing for a dialect to decide and
// an unanswered axis must not refuse it: `v=1; v=1 f` is not a question about
// anything. The comparison is of the whole state rather than the scalar,
// because the prefix is taken back whole or not at all — the array a name held
// and the export attribute it carried are as much a difference as the value.
func (r *Runner) takeBackFunctionPrefix(undo []savedVar) {
	for i := len(undo) - 1; i >= 0; i-- {
		if r.matchesSavedVar(undo[i]) {
			continue
		}
		if r.ask(r.sem().AssignmentPrefixPersistsAfterAFunction,
			"an assignment before a function persisting after the call") {
			continue
		}
		r.restoreVars(undo[i : i+1])
	}
}

// matchesSavedVar reports whether a name is in exactly the state that was
// saved for it, so that putting it back would change nothing.
func (r *Runner) matchesSavedVar(u savedVar) bool {
	value, present := r.Vars[u.name]
	if present != u.present || value != u.value || r.removed[u.name] != u.removed {
		return false
	}
	// The *effective* attribute rather than the recorded tri-state: a name
	// nobody has spoken about and one recorded as not exported reach every
	// child alike, so a prefix that only wrote the second over the first has
	// changed nothing for a dialect to decide. Comparing the tri-state made
	// the un-exporting answer ask this axis on `v=9; v=9 f`, where the two
	// readings land in exactly the same place.
	if r.isExported(u.name) != u.wasExported(r) {
		return false
	}
	// The message a produced parameter's producer is holding is part of the
	// state too, and it is the only part a prefix to such a name changes:
	// comparing the four stores alone reported "nothing moved" for
	// `SECONDS=100 f` and skipped the take-back entirely, so the axis was
	// never asked and the seeding stood in every dialect (#2713).
	if assigned, spoken := r.assigned[u.name]; spoken != u.assignedSpoken || assigned != u.assigned {
		return false
	}
	a, inArray := r.Arrays[u.name]
	if inArray != u.inArray || !a.equal(u.array) {
		return false
	}
	m, inTable := r.AssocArrays[u.name]
	return inTable == u.inTable && m.equal(u.table)
}

// assignForm says how an assignment was written, which one dialect answers a
// readonly reassignment by.
type assignForm uint8

const (
	// assignedAnyhow is every other way a name is set: through a builtin,
	// as a prefix to a command, by `read`, by the shell itself.
	assignedAnyhow assignForm = iota
	// assignedAlone is an assignment standing as a command of its own —
	// `x=2` on a line, and not `export x=2` or `x=2 cmd`.
	assignedAlone
	// assignedByDeclaration is an assignment made through a declaration
	// utility — `export x=2`, `typeset x=2`, `readonly x=2`.
	assignedByDeclaration
	// removedAttribute is not an assignment at all: it is `typeset +r x`
	// asking for an attribute the name may not give up, in a dialect that
	// does not let it — see Semantics.ReadonlyAttributeCanBeRemoved.
	//
	// A form rather than a refusal of its own, because everything the
	// refusal decides it already decides the same way: the sentence is the
	// declaration's, and so is the fatality, and so is giving up nothing of
	// the enclosing line. Only which builtins name themselves differs, and
	// that is one table — see Diagnostics.ReadonlyRemovalNamesBuiltin.
	removedAttribute
	// assignedAsTheCompoundView is not a script's assignment at all: it is
	// the store that keeps a plain `$a` answering for an array or a table
	// `a`, written by storeArray and setAssocElem after every element write.
	//
	// It exists so that every *other* form can mean what a script means by a
	// scalar store — that the name is a scalar now, whatever compound it was
	// holding, per Semantics.ScalarAssignedOverACompoundReplacesTheName. The
	// alternative was to name the callers that do mean it, which is a list
	// that grows with every construct that sets a name and had already been
	// missed by `for`, `read`, `select`, `getopts` and `${a::=x}` alike. One
	// caller is exempt and it says so; everything else is right by default.
	assignedAsTheCompoundView
	// assignedByBuiltin is a **builtin filling in its own output
	// parameter** — `getopts` writing OPTARG, OPTIND and the name it was
	// given.
	//
	// Neither of the two shapes the refusal above was written for. It is not
	// a declaration: the name was not written in the script at all. And it is
	// not a bare assignment: measured 2026-09-16, every panel shell that
	// reports the refusal runs the next command on the same line, where
	// `readonly x=1; x=2; echo one` never prints `one` in any of them. What
	// the refusal costs is decided where it happened, which for this form is
	// interp/getoptsbuiltin.go — see getoptsWrite, and
	// Semantics.ReadonlyRefusalInABuiltinIsFatal for the one shell that ends
	// the script over it (#3147).
	assignedByBuiltin
)

// declaresRatherThanAssigns reports whether the form is one a *declaration*
// utility wrote, which is the question three of refuseReadonly's answers turn
// on: the wording, the fatality, and whether the rest of the line is given up.
func (f assignForm) declaresRatherThanAssigns() bool {
	return f == assignedByDeclaration || f == removedAttribute
}

// namesTheBuiltin reports whether the refusal's *sentence* may carry the name
// of the builtin that made the write, which is a wider set than the question
// above: a builtin filling in its own output parameter names itself where a
// dialect names one, and dash writes `getopts: OPTARG: is read only` through
// the same wording it writes `export: x: is read only` with.
//
// Separate from declaresRatherThanAssigns because the other two things that
// question decides — the fatality and whether the rest of the line is given
// up — are not this form's, and reading one answer for all three is what put
// `getopts` on the bare assignment's path to begin with.
//
// Folding assignedByBuiltin into that question instead is an **equivalent
// mutant today** and was reverted rather than kept: `getopts` reaches this
// sentence without going through refuseReadonly at all, so the other two
// answers are never asked of the form and nothing can tell the two spellings
// apart. What the separation buys is the next caller — a second builtin
// handed this form would otherwise inherit a declaration's fatality and its
// hold on the rest of the line, which are the two things measured *not* to be
// a builtin's (#3147).
func (f assignForm) namesTheBuiltin() bool {
	return f.declaresRatherThanAssigns() || f == assignedByBuiltin
}

// setVarAs sets a variable, knowing how the assignment was written.
// attributeFolded is what a name's attributes make of a value: the integer
// attribute evaluates it as an expression rather than storing the text, and
// the case attributes fold it. The second result is false where the integer
// evaluation failed and has already said so, in which case nothing is stored.
//
// Its own function because two callers need it and only one of them is an
// assignment: applying `-i` or `-u` to a name that already holds a value
// re-reads what it holds through the attribute that just arrived — see
// declareEmpty — and that is not an assignment, so it must not meet the
// readonly refusal or anything else setVarAs does around it.
func (r *Runner) attributeFolded(name, value string) (string, bool) {
	if _, ok := r.floatPrecision[name]; ok {
		// The name was declared float, so what is assigned to it is an
		// expression too — `typeset -F 3 x=1+2` is `3.000` — and the places
		// it is written in are the name's rather than the value's.
		//
		// Ahead of the integer branch and not beside it: the two attributes
		// cannot both stand, so applyAttributes takes one off when the other
		// arrives and only one of these can run. Reaching this first is what
		// makes that a statement rather than a hope.
		v, ok := r.floatValue(value)
		if !ok {
			return "", false
		}
		// The rendered text is what is *stored*, exactly as an integer
		// name's base is: `${#x}` counts the five characters of `1.500`, a
		// child is told `x=1.500`, and arithmetic reads them back.
		value, ok = r.floatFormatted(name, v)
		if !ok {
			return "", false
		}
	} else if r.integer[name] {
		// The name was declared integer, so what is assigned to it is an
		// expression rather than text.
		//
		// The base the text was written in is learned before it is
		// evaluated, in the dialect that learns one: `0x10` teaches the name
		// 16 and a later plain `5` then reads back as `16#5`, so it is the
		// name's base and not the assignment's. See
		// Semantics.IntegerBaseComesFromTheValueAssigned.
		r.learnIntegerBase(name, value)
		if r.unspecified {
			return "", false
		}
		v, ok := r.integerValue(value)
		if !ok {
			return "", false
		}
		// Written back out in the name's base, which is what is stored:
		// every read sees these characters and arithmetic parses them back.
		value = r.integerRenderedText(name, v)
		if r.unspecified {
			return "", false
		}
	}
	// The case attributes, folded here or on the way *out* — see
	// Semantics.CaseAttributeFoldsWhenRead, which is the whole of the
	// difference. Where the fold happens at assignment, `declare -l v;
	// v=ABC` stores `abc` and there is no way back to what was written;
	// where it happens on the read, the store keeps `ABC` and every read
	// answers `abc`.
	//
	// Under the same locale policy as the case-changing operators, which this
	// site did not follow until #2027: measured under LC_ALL=C, all three of
	// bash 5.3.15, ksh93u+ and zsh 5.9.2 answer CAFé for `declare -u s=café`
	// and we answered CAFÉ. caseChanged is the one place that narrowing is
	// decided, so the attribute cannot drift from the operator again — and
	// caseFolded is the one place the *letters* are read, so the two sites
	// cannot drift from each other either.
	// And the width the name carries learns itself from the first value,
	// whichever way the shell folds: the width becomes part of the
	// declaration — `typeset -L f=xy` lists back as `typeset -L2 f=xy` — so
	// it is settled where the value arrives and not where one is printed.
	r.widthLearned(name, value)
	if !r.caseFoldsOnRead() {
		value = r.caseFolded(name, value)
		// Under the same answer, for the same reason: a shell that folds on
		// the way in has no way back to the text the assignment carried, and
		// the two families were measured to split the same way. Only zsh has
		// the width letters today and zsh folds on the read, so this arm is
		// ready for the preset rather than exercised by one — see
		// fieldwidth.go.
		value = r.widthPadded(name, value)
	}
	return value, true
}

// caseFolded is what the case attributes make of one value: the whole of what
// `-l` and `-u` do, in one place, so that the assignment site and the read
// site cannot come to disagree about which letter wins or about the locale.
//
// A name never carries both — applyAttributes takes one off when the other
// arrives — so there is no order to decide here.
func (r *Runner) caseFolded(name, value string) string {
	switch {
	case r.lowered[name]:
		return r.caseChanged(value, unicode.ToLower)
	case r.uppered[name]:
		return r.caseChanged(value, unicode.ToUpper)
	}
	return value
}

// caseFoldsOnRead reports whether this shell keeps what was assigned and
// folds every read of it, rather than folding once on the way in.
//
// Asked wherever a case-attributed scalar is stored or read, and nowhere
// else: a name with neither letter reads the same under both answers, so the
// question never arises for it. Measured 2026-09-12 with `env -i`, a scratch
// HOME and no startup files, over `typeset -l lo=AB`:
//
//	           $lo   typeset -p lo      typeset +l lo; $lo
//	bash 5.3   ab    declare -l lo="ab"  ab
//	ksh93u+    ab    typeset -l lo=ab    ab
//	zsh 5.9.2  ab    typeset -l lo=AB    AB
//
// The middle column is the listing #1755 is about — a shell that folds on
// the way in has no way back to the text the assignment carried, so its
// listing writes the folded value and the third column proves the store
// really holds it.
func (r *Runner) caseFoldsOnRead() bool {
	return r.sem().CaseAttributeFoldsWhenRead == Yes
}

// readAttributeFolded is a scalar on its way out of the store, presented as
// the attributes that act on the read say. The guard is the attribute rather
// than the axis, so the ordinary name — which is nearly every name — costs
// the map lookups every read already makes.
//
// Two families arrive here and they compose in this order, measured:
// `typeset -lL 4 d=ABCD` reads `abcd`, so the case fold runs and the width is
// applied to what it left. The width letters are fieldwidth.go's.
func (r *Runner) readAttributeFolded(name, value string) string {
	value = r.readCaseFolded(name, value)
	if !r.caseFoldsOnRead() {
		return value
	}
	if _, ok := r.fieldWidth[name]; ok {
		value = r.widthPadded(name, value)
	}
	return value
}

// readCaseFolded is the case half of readAttributeFolded on its own, for the
// one read that takes it without the other: what a child is told. See the
// environment loop, where the measurement is.
func (r *Runner) readCaseFolded(name, value string) string {
	if !r.lowered[name] && !r.uppered[name] {
		return value
	}
	if !r.caseFoldsOnRead() {
		return value
	}
	return r.caseFolded(name, value)
}

// appendedValue is what `+=` puts together: the value a name already holds
// and the text on the right of the operator.
//
// One spelling over two operations, and the *name* says which it is. A plain
// name joins the characters — `a=1; b=2; a+=b` is `1b` — while a name
// carrying the integer or the float attribute **adds**, because what it holds
// is a number and what is written on the right is an expression rather than
// text. Measured 2026-09-08: `typeset -i a=1; a+=2` is `3` in bash 5.3.15,
// bash 3.2.57, ksh93 and zsh 5.9.2 alike, and `typeset -F 3 a=1.5; a+=2.25`
// is `3.750` in the two that spell the float letter. Four columns with one
// answer is the core's answer and not a dialect's, so no axis is asked here —
// an axis with nothing to disagree about would be a question this shell can
// never be asked.
//
// The two sides are evaluated **apart** and never joined into one expression
// first. That is not a nicety: joining first is exactly the bug this
// replaced, where `typeset -i a=1 b=2; a+=b` built the text `1b` and only
// then tried to read it as a number, and the plain `a+=2` built `12` and
// stored a wrong number at status 0 with nothing said. The shells' own
// diagnostics say they do not join either — `typeset -i a=1; a+=2+` blames
// `2+` in all four and never mentions the `1` standing in front of it.
//
// The base is learned from the right-hand side, exactly as a plain
// assignment's value teaches it: measured in zsh 5.9.2, `typeset -i a=1;
// a+=0x10` is `16#11`, and a later plain `a=5` then reads back `16#5`, so
// what the append taught was the name's base and not that one value's.
//
// The second result is false where an evaluation failed and has already said
// so, in which case nothing is stored — attributeFolded's convention, and for
// the same reason: the failure ends the script and a half-written name would
// outlive it.
func (r *Runner) appendedValue(name, old, add string) (string, bool) {
	_, isFloat := r.floatPrecision[name]
	switch {
	case isFloat:
		// Ahead of the integer branch for attributeFolded's reason: the two
		// attributes cannot both stand, and reaching this first is what makes
		// that a statement rather than a hope.
		lhs, ok := r.floatValue(old)
		if !ok {
			return "", false
		}
		rhs, ok := r.floatValue(add)
		if !ok {
			return "", false
		}
		return r.floatFormatted(name, lhs+rhs)
	case r.integer[name]:
		r.learnIntegerBase(name, add)
		if r.unspecified {
			return "", false
		}
		lhs, ok := r.integerNumber(old)
		if !ok {
			return "", false
		}
		rhs, ok := r.integerNumber(add)
		if !ok {
			return "", false
		}
		// Decimal, and deliberately not written out in the name's base here:
		// the store this is on its way to folds what it is handed through
		// attributeFolded like any other value, which renders it — so a
		// render at this end would be a second copy of that one, and the
		// second copy is what goes stale.
		return itoa(lhs + rhs), true
	}
	return old + add, true
}

// readonlyRefusalNamesBuiltin reports whether the running builtin puts its own
// name in this refusal. A plus form refused the attribute it wanted to remove
// asks a different table, because one shell answers the two shapes
// differently through the identical word — see
// Diagnostics.ReadonlyRemovalNamesBuiltin.
func (r *Runner) readonlyRefusalNamesBuiltin(form assignForm) bool {
	if form == removedAttribute && r.diag().ReadonlyRemovalNamesBuiltin != nil {
		return r.diag().ReadonlyRemovalNamesBuiltin[r.inBuiltin]
	}
	return r.diag().ReadonlyRefusalNamesBuiltin[r.inBuiltin]
}

// reportReadonlyRefusal writes the complaint a refused assignment makes, and
// writes nothing else. What the refusal *costs* is the caller's, because it is
// not one answer: a bare assignment gives up the rest of its list, a
// declaration does not, and an assignment prefixed to a command splits by the
// kind of command that follows it — so the sentence is shared and the
// consequence is decided where the refusal happened (#1219).
func (r *Runner) reportReadonlyRefusal(name string, form assignForm, fatal bool) {
	// Two arguments only where the wording asks for two: a format with no
	// explicit indexes and a spare argument becomes "%!(EXTRA …)", which is
	// what Wording's own note is about.
	msg := Wording(r.diag().ReadonlyVariable, "%s: readonly variable", name)
	if form.namesTheBuiltin() && r.diag().ReadonlyVariableInDeclaration != "" &&
		r.readonlyRefusalNamesBuiltin(form) {
		wording := r.diag().ReadonlyVariableInDeclaration
		if r.inBuiltin == "read" && r.diag().ReadonlyVariableInRead != "" {
			// One dialect words `read`'s refusal apart from every other
			// builtin's, calling it a warning. See
			// Diagnostics.ReadonlyVariableInRead.
			wording = r.diag().ReadonlyVariableInRead
		}
		msg = Wording(wording, "", name, r.inBuiltin)
	}
	// The builtin has been taken for the wording above where a dialect wants
	// it, and this message does not carry it in the *location* in the dialect
	// that puts it there for everything else: zsh writes `zsh:1: read-only
	// variable: x` from inside `export`, not `zsh:export:1:`. So it is put
	// aside for the report and given back.
	//
	// Except where the dialect names the builtin for this very refusal, which
	// is the one case where it belongs in the location too: ksh93 answers
	// `set -A ro q` with `<script>[6]: set: ro: is read only` — its *builtin*
	// location — against `<script>: line 2: ro: is read only` for a plain
	// assignment to the same name. One table decides both, because a dialect
	// that puts the name in the sentence is the dialect that keeps the
	// builtin's location under it.
	if !r.readonlyRefusalNamesBuiltin(form) {
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	if fatal {
		r.fatal("%s\n", msg)
		return
	}
	r.diagf("%s\n", msg)
}

// refuseReadonly reports whether an assignment to a frozen name is refused,
// having said so and having decided what the refusal does to the script.
//
// Taken out of setVarAs because an array is not stored through it. An element
// write, a whole-array literal and a compound append each reach a store of
// their own, and every one of them was taking the write: the refusal was only
// ever consulted on the path a scalar takes. The indexed spellings *looked*
// guarded, because storeArray keeps the scalar view in step and that call does
// go through setVarAs — so the complaint printed after the element had already
// been written. `declare -a A=(x y); readonly A; A[0]=z` said `A: readonly
// variable` and left `z` in the array; the associative spelling has no scalar
// view to keep in step, so it said nothing at all and reported 0.
//
// One rule reached from a second place rather than a second rule: the wording
// is the same, and so is every answer about what the refusal costs — see the
// ReadonlyReassignment axes below.
func (r *Runner) refuseReadonly(name string, form assignForm) bool {
	if !r.readonly[name] {
		return false
	}
	if name == r.retypingFrozen {
		// A frozen scalar being replaced by a declaration's array literal,
		// which the dialect has already allowed at the top of assign. See
		// Runner.retypingFrozen.
		return false
	}
	if form == assignedAsTheCompoundView && r.freezeSurvivesAShadow(name) {
		// The empty a valueless declaration writes, over a produced
		// parameter whose freeze the shadow did *not* displace. Measured:
		// `f(){ local ARGC; print $ARGC }` is `0` in zsh and the outer value
		// is intact, where `f(){ local ARGC=5; … }` is refused — so the
		// declaration is taken and only a *value* is refused, and a letter
		// is not a value either (`local -i ARGC` is `0` there too).
		//
		// This form and no other, which is what keeps the refusal on the
		// line after: `f(){ local ARGC; ARGC=5 }` writes through
		// assignedAlone and is `read-only variable: ARGC` in both shells.
		// See freezeSurvivesAShadow, where the six rows are.
		return false
	}
	// Fatal everywhere but bash, measured with a plain assignment in a
	// script — which is the contaminated-probe case oracle.md records.
	//
	// The invocation route has nothing to do with it, which took a full
	// route-by-separator square to see: `bash -c 'readonly x=1; x=2; echo
	// after'` stops and exits 1, and so does the same one-line program from
	// a *file*; the same three commands on three lines print `after` and exit
	// 0 by **both** routes. What ends is the command list, which is
	// controlAbandon below. The field that used to be read here was measured
	// from the two cells on the diagonal of that square — `-c` with a `;`
	// against a file with newlines — which varied two things at once and is
	// confirmatory for either reading (#1182).
	//
	if (form == assignedAnyhow || form == assignedAsTheCompoundView) && (r.inBuiltin != "" || r.arithCommand > 0) {
		return r.refuseReadonlyInACommand(name)
	}
	fatal := r.sem().ReadonlyReassignmentFatal
	switch {
	case form.declaresRatherThanAssigns():
		// A second answer, and a different set of shells: `export x=2` stops
		// dash, ksh93 and zsh, and bash reports it and carries on — by both
		// invocation routes.
		fatal = r.sem().ReadonlyReassignmentByDeclarationFatal
		if r.IsSpecialBuiltinHere(r.inBuiltin) {
			// Except where the declaration utility is a *special* builtin —
			// see Semantics.ReadonlyReassignmentBySpecialBuiltinFatal.
			fatal = r.sem().ReadonlyReassignmentBySpecialBuiltinFatal
		}
	}
	if r.ask(fatal, "a readonly reassignment being fatal") {
		r.reportReadonlyRefusal(name, form, true)
		return true
	}
	r.reportReadonlyRefusal(name, form, false)
	r.status, r.assignFailed = 1, true
	// Reported and not fatal, and the shell still gives up what it was
	// running: `readonly r=1; r=2; echo one` never prints `one`, and the line
	// after it runs. Measured in every shape that encloses a statement — a
	// loop, a function body, an `if`, a group, a subshell.
	//
	// A *declaration* does not give anything up. `export x=2`, `declare x=2`
	// and `readonly x=2` against a readonly name all report and run the next
	// command on the same line, which is the tell that this is about a bare
	// assignment failing rather than about the refusal.
	if !form.declaresRatherThanAssigns() {
		r.abandonTheCommand()
	}
	return true
}

// refuseReadonlyInACommand is the refusal of a write a **command** made to a
// name it was handed — `printf -v x`, `mapfile x`, `let x=2`, `cd` updating
// OLDPWD, and `(( x = 2 ))` — rather than one the script wrote as an
// assignment.
//
// Two differences from an assignment's, and both measured 2026-09-16 from
// script files, one construct per line with an `echo` behind it on the same
// line:
//
//	                       bash 5.3.20       bash -o posix   zsh 5.9.2        ksh93u+
//	x=2                    line given up     script ends     script ends      script ends
//	printf -v x hi         same line runs    same line runs  script ends      no -v
//	let x=2                same line runs    same line runs  status 1, runs   status 1, runs
//	(( x = 2 ))            same line runs    same line runs  status 2, runs   script ends
//
// So nothing about the *line* is given up, and the status is the command's
// failure — 1 from a builtin in all three, and the arithmetic command's own
// failure status from `(( ))`, which is 2 in zsh exactly as a division by zero
// is. What ends the script is a question per kind of command: a builtin's is
// Semantics.ReadonlyRefusalInABuiltinIsFatal, which `read` and `getopts`
// already ask, and the arithmetic command's is ArithCommandErrorIsFatal,
// which a failed evaluation already asks. This path used to take the
// assignment's answers, so bash gave up the line after `printf -v` and bash
// in POSIX mode ended the script there.
func (r *Runner) refuseReadonlyInACommand(name string) bool {
	r.refusedInACommand = true
	if r.arithCommand > 0 && r.inBuiltin == "" {
		// The command reports and decides in arithCmd, through the same
		// door an evaluation error goes through.
		r.reportReadonlyRefusal(name, assignedAnyhow, false)
		return true
	}
	fatal := r.ask(r.sem().ReadonlyRefusalInABuiltinIsFatal,
		"a builtin's refused write to its own output parameter ending the script")
	r.reportReadonlyRefusal(name, assignedAnyhow, fatal)
	if !fatal {
		r.status = 1
	}
	return true
}

// frozenNameOfAnAssignment is the name a plain assignment's freeze is asked
// about, which is **where the value lands** and not what was written.
//
// A reference and what it points at are frozen separately, and only the
// second one can refuse a write through it: measured 2026-09-16 on bash
// 5.3.20, `v=1; declare -rn r=v; r=5` is a silent 0 leaving `v` at 5, where
// `v=1; readonly v; declare -n r=v; r=5` is `v: readonly variable` and gives
// up the rest of the line. Both the name consulted and the name in the
// sentence are the **target**. This shell asked about the reference, so a
// frozen reference made every write through it a refusal the shell being
// modeled does not make.
//
// The store below already resolves the reference before its own
// refuseReadonly, so this is the one check that was reading the wrong name —
// and it is here rather than in the store because a refused assignment gives
// up the rest of the line and the store is past that point.
//
// A reference aimed at an **element** keeps the name it was written with: the
// base array's freeze is what refuses `declare -n e=ra[0]; e=7`, and both
// shells name the array in it — a route storeThroughNamerefElement already
// takes.
// frozenNameOfAnUnset is the name whose freeze can refuse an `unset`, which is
// **where the unset lands** rather than what the script wrote.
//
// The same rule frozenNameOfAnAssignment states for a write, and it was
// missing from the other half: the store followed a reference for the *delete*
// while the refusal above was asked against the written name, so the two read
// different names and both directions were wrong. Measured 2026-09-17 on bash
// 5.3.20, script files under `env -i`:
//
//	x=1; declare -rn RO=x; unset RO
//	  there: 0, `x` gone, `RO` still a frozen reference
//	  here:  `unset: RO: cannot unset: readonly variable`, `x` kept
//
//	y=1; readonly y; declare -n RT=y; unset RT
//	  there: `unset: y: cannot unset: readonly variable`, `y` kept
//	  here:  0, and a frozen variable removed
//
// A **subscripted** operand keeps its base, and that is the reason for the
// second argument rather than a tidiness: `unset r[0]` names the array the
// brackets index, and a reference standing in front of that base is a
// different question nobody has measured.
//
// A reference aimed at an *element* never arrives here as a reference — the
// unset loop has already rewritten it to the element, so the base this sees is
// the array — and `unset -n` never arrives at all, because the letter names
// the reference and its own freeze is asked in that branch.
func (r *Runner) frozenNameOfAnUnset(name string, subscripted bool) string {
	if subscripted {
		return name
	}
	if target, aimed := r.namerefTarget(name); aimed && isNameLike(target) {
		return target
	}
	return name
}

func (r *Runner) frozenNameOfAnAssignment(name string) string {
	target, aimed := r.namerefTarget(name)
	if !aimed {
		return name
	}
	if _, _, element := r.indirectElement(target); element {
		return name
	}
	return target
}

func (r *Runner) setVarAs(name, value string, form assignForm) {
	if r.selfNameref(name) {
		// The write half of the read above: a reference aimed at its own
		// name lands on the global cell. Ahead of namerefAssignmentTarget,
		// which would read the walk's `not aimed` as an invitation to
		// re-point the reference at the text being assigned — the rule for a
		// reference that points nowhere, and the wrong one for this shape.
		r.selfNamerefAssignment(name, value, form)
		return
	}
	if target, write := r.namerefAssignmentTarget(name, value, form); !write {
		// The value **aimed** the reference rather than being written
		// through it, which is what a reference with nothing to point at
		// does with its first assignment. See interp/nameref.go.
		return
	} else if target != name {
		if base, sub, element := r.indirectElement(target); element {
			r.storeThroughNamerefElement(base, sub, value)
			return
		}
		name = target
	}
	if r.refuseReadonly(name, form) {
		return
	}
	// A `.set` discipline sees the value on its way in and may replace it —
	// `function s.set { .sh.value="<${.sh.value}>"; }; s=first` stores
	// `<first>`. Here rather than at the assignment statement because every
	// way of writing a scalar has to agree: measured, `typeset v=x`, `read
	// v` and `for v in a b` all fire it, and each of those reaches this
	// store without going through Runner.assign at all.
	//
	// After the readonly refusal, so a write the shell will not take does
	// not run a hook about it. See interp/discipline.go.
	if v, ran := r.disciplineWrite(name, disciplineSet, "", value); ran {
		value = v
	}
	// A name holding an array or a table is not a name a scalar simply lands
	// on — see scalarOverCompound, which either takes the write or takes the
	// compound away so that the store below is the whole of the name.
	if r.scalarOverCompound(name, value, form) {
		return
	}
	// And a name holding a *compound variable* keeps none of it: the scalar
	// replaces the whole tree. See compoundVariableRetyped for the five
	// stores this is one of, and why it is five rather than one.
	r.compoundVariableRetyped(name)
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	if !r.viewIsAlreadyFolded {
		// The compound view of an array that *has* elements is written from
		// text this name's attributes have already folded — see storeArray,
		// which folds every element and then writes the lowest one here — so
		// folding it again is a second evaluation of the same characters. It
		// is invisible where the attribute succeeds and a duplicated
		// complaint where it does not: `typeset -i a; a[0]=/x/y.z` wrote the
		// arithmetic refusal twice where ksh93 writes it once (#2818).
		//
		// An *empty* array is not that case and does not set the flag: there
		// are no elements to have been folded, and the view's own fold is
		// what makes `integer k` read back as `0`.
		var ok bool
		if value, ok = r.attributeFolded(name, value); !ok {
			return
		}
	}
	if _, dynamic := r.Dynamic[name]; dynamic && r.producerEndedByUnset(name) {
		// The `unset` ended the parameter in this dialect, so what the
		// assignment makes is an ordinary variable: the producer goes with
		// the name it was producing for, and the store further down is the
		// whole of what is left. Deleted from this runner's own tables, of
		// which a subshell holds a copy — a name unset inside one is still
		// the parent's produced parameter afterwards.
		delete(r.Dynamic, name)
		delete(r.dynamicWriters, name)
		delete(r.assigned, name)
		if r.endedProducers == nil {
			r.endedProducers = map[string]bool{}
		}
		r.endedProducers[name] = true
	}
	if _, dynamic := r.Dynamic[name]; dynamic {
		// Assigning a produced parameter is a message to its producer rather
		// than a replacement for it.
		if r.assigned == nil {
			r.assigned = map[string]string{}
		}
		r.assigned[name] = value
		delete(r.removed, name)
		// And where the producer has a writer, the message is delivered
		// rather than merely left for it to find. The two are not
		// alternatives: SECONDS reads what was assigned the next time it is
		// asked, and a parameter whose assignment *does* something has to act
		// now — see SetDynamicWriter.
		if write, ok := r.dynamicWriters[name]; ok {
			write(r, value)
		}
		return
	}
	if name == "OPTIND" {
		// Noted rather than compared: see optindAssigned.
		r.optindAssigned = true
	}
	if name == "PATH" {
		// A new PATH makes every remembered answer a guess about a search
		// nobody has run. Unanimous — bash 5.3.15, zsh 5.9.2, ksh93 and dash
		// all answer an empty table after `ls >/dev/null; PATH=$PATH; hash`,
		// and all four keep it after a bare `export PATH`, which is the
		// control: it is the assignment that empties the table and not the
		// mention.
		r.forgetEveryHashedCommand()
	}
	// The parameter that takes names out of a pathname expansion starts
	// where it is assigned rather than where it has a value, and the
	// hidden-name switch a non-null value writes goes with it. See
	// interp/ignorednames.go.
	r.ignoredNamesAssigned(name, value)
	// An assignment gives the name a value of its own, whatever a
	// declaration had left there. declareEmpty records the flag *after*
	// calling here, which is what lets one function do both.
	delete(r.declaredEmpty, name)
	r.nameIsBack(name)
	r.Vars[name] = value
	// And the other half of a tie, if this name is one. After the store, so
	// that the mirror's own read of this name sees the new value.
	r.mirrorScalarToArray(name, value)
}

// producerEndedByUnset reports whether `unset` has ended a produced parameter,
// so that an assignment to the name now makes an ordinary variable rather than
// sending its producer a message.
//
// An assignment is a message every time while the parameter is still there:
// `RANDOM=9` seeds the generator, `SECONDS=9` counts on from nine, and no
// shell in the panel lets the stored text shadow what the producer says. The
// split is only over what `unset` left behind — see
// Semantics.AssignmentRestoresAnUnsetProducedParameter, where one shell reads
// the assignment as the same message and the producer answers again, and four
// read the `unset` as having ended the parameter.
//
// Asked only where the two readings differ, which is the rule for every axis:
// a name nothing has unset has one answer and is never asked about. That is
// also what keeps `OPTIND` and `OPTARG` where they were — they reach
// r.removed directly (see getoptsbuiltin.go) and are no dialect's produced
// parameters, so nothing here sees them.
func (r *Runner) producerEndedByUnset(name string) bool {
	if !r.removed[name] {
		return false
	}
	return !r.ask(r.sem().AssignmentRestoresAnUnsetProducedParameter,
		"an assignment restoring a produced parameter `unset` took away")
}

// nameIsBack forgets that `unset` had taken a name away, which is what
// storing a value into it means.
//
// The record is kept as well as the value deleted, because a name that came
// from the environment is not in Vars to begin with and deleting nothing left
// it visible — see unsetName. So the record has to be lifted again by hand
// when the name comes back, and it was not: every read went on working,
// because a stored value answers ahead of the record, and everything that
// asks the record *directly* went on being told the name is gone.
//
// A listing is what that was visible in. `unset q; q=7; typeset -p q` reports
// the name as missing in every dialect here, where bash, ksh93 and zsh all
// say the name back with its value — and before `unset` cleared attributes it
// was worse than missing, listing `declare -i q` with no value at all: an
// attribute that should have gone, over a value that was there. One rule with
// #1047 rather than a second: `unset` leaves two records behind and both have
// to be lifted by whatever brings the name back.
//
// Called from the two stores rather than from the dozen assignment spellings
// above them: the scalar table, and the keyed one. An indexed array needs no
// call of its own — storeArray keeps `$a` in step through setVar, so it lifts
// the mark by that route, and a call beside it was a line no mutation could
// kill.
func (r *Runner) nameIsBack(name string) {
	delete(r.removed, name)
}

// ensurePWD gives `$PWD` a value before the first command runs.
//
// POSIX requires a shell to set it at startup, and until this was here `cd`
// was the only thing that ever did — so a shell handed an environment without
// PWD in it, which is exactly what the conformance harness hands one, expanded
// `$PWD` to nothing. `PATH=$PWD/d:$PATH` then meant `/d`, and a `.` lookup
// that should have found a file on PATH silently fell through to the current
// directory instead. Real bash sets it and got the case right; we did not.
//
// Only r.Vars is consulted, deliberately, and not getVar: getVar falls back to
// the process environment, whose PWD describes the *parent* rather than this
// runner. A Runner given a Dir of its own would otherwise report the directory
// the program was started in, which is a different place — and real shells do
// not trust an inherited PWD that disagrees with where they actually are
// either.
//
// Idempotent, because Run is called more than once on a runner — a dialect's
// prelude first and then the script — and because `cd` may already have moved,
// and because a caller may have set it deliberately.
func (r *Runner) ensurePWD() {
	if _, ok := r.Vars["PWD"]; ok {
		return
	}
	// Two shells in the panel take a name from the environment rather than
	// asking the kernel, and which name the shell knows its own directory by
	// follows from that — see StartupPwdNamePolicy, where the paragraph above
	// is half the measurement and the other half is why eight corpus rows
	// read `/private`.
	if r.settleStartupPwd() {
		return
	}
	r.setVar("PWD", r.workDir())
}

func (r *Runner) getVar(name string) (string, bool) { return r.varValue(name, true) }

// storedVar is getVar with the read fold left off: the text the store really
// holds, in the shell where those are two different things.
//
// One caller, and it is `+=`. Measured 2026-09-12 on zsh 5.9.2, `typeset -l
// lo=AB; lo+=CD` lists back as `ABCD` and reads as `abcd` — so the append
// joins what was *assigned*, not what a read of it answers. Reading through
// getVar there stored `abCD`, which is a third text neither shell has.
//
// Deliberately narrow. Everything else that wants the store rather than the
// value already has it: a listing gathers from the tables directly, and the
// attribute letters are their own question.
func (r *Runner) storedVar(name string) (string, bool) { return r.varValue(name, false) }

func (r *Runner) varValue(name string, folded bool) (string, bool) {
	if r.disciplined == nil {
		// Nothing has ever defined a discipline function in this shell,
		// which is every shell but the handful that have, and every read in
		// the ones that do until the definition runs. One nil check rather
		// than a name built and a table asked on the hottest path there is.
		return r.storedValue(name, folded)
	}
	// A `.get` runs before the store is read and may replace what the read
	// answers — but only if it assigned `${.sh.value}`, which is not the
	// same question as what the parameter ends up holding. See
	// interp/discipline.go, where the order is measured: a hook that assigns
	// its *own variable* is answered by the read that ran it.
	//
	// Set-ness stays the store's either way — measured, a `.get` on a name
	// nothing has assigned fires and `${g-word}` still takes the word.
	got, replaced := r.disciplineElementRead(r.throughNamerefName(name), r.bareReadSubscript(name))
	v, ok := r.storedValue(name, folded)
	if replaced {
		return got, ok
	}
	return v, ok
}

// bareReadSubscript is the element an unsubscripted `$name` is a read of, as
// a discipline hears it.
//
// Empty for nearly everything, because nearly everything is one place. An
// *indexed array* is the exception: measured on ksh93u+ 2012-08-01,
// 2026-09-16, `a=(p q); function a.get { … }; $a` enters the hook with
// `${.sh.subscript}` as `0` — a bare read of an array is a read of its first
// element and says so — where the same probe on a scalar and on a keyed table
// both enter with the subscript empty.
func (r *Runner) bareReadSubscript(name string) string {
	if _, ok := r.Arrays[name]; !ok {
		return ""
	}
	if !r.disciplineIsWatching(name, disciplineGet) {
		// The base is an axis, and asking it for a name with no hook would
		// make a dialect-free runner settle how arrays are numbered to read
		// a variable that has nothing to do with them.
		return ""
	}
	return itoa(r.arrayBase())
}

// storedValue is varValue with the discipline hooks left off: what the
// tables, the producers and the environment say the name holds.
func (r *Runner) storedValue(name string, folded bool) (string, bool) {
	// A read through a name reference lands on what it points at. Only a
	// plain-name target here: a reference aimed at an *element* has its
	// subscript read as arithmetic, and arithmetic reaches command
	// substitution, which reaches the builtin table this function is
	// initialized before. So that half is resolved one level up, in
	// paramSource — see namerefReadsAnElement, which says what a caller that
	// is not an expansion therefore does not get.
	name = r.throughNamerefName(name)
	if r.selfNameref(name) {
		// A reference aimed at its own name reads the **global** cell rather
		// than the binding it is standing in front of, which is what makes
		// `local -n r=r` a handle on the outer variable. The warning the
		// read carries was already written by throughNamerefName, whose walk
		// came back to where it started. See interp/nameref.go.
		return r.selfNamerefValue(name)
	}
	// A produced array answers a plain `$name` too, and what it answers with
	// is an axis: the whole array in one shell and its first element in the
	// others.
	if elems, ok := r.pipelineStatuses(name); ok {
		return r.arrayScalar(elems), true
	}
	if f, ok := r.Dynamic[name]; ok {
		// Ahead of the stored table, because a parameter that produces its
		// value cannot be overwritten by assigning to it: `RANDOM=5` seeds
		// the generator and the next read is still a new number. What was
		// assigned is kept where the producer can see it — SECONDS counts
		// from it — rather than shadowing the producer entirely.
		if !r.removed[name] {
			v := f(r)
			// This is the *expansion* of the parameter, so it is the reading
			// one dialect's listing writes back. Recorded here rather than
			// inside the producers because every one of them would otherwise
			// have to remember, and a producer a dialect adds tomorrow would
			// be the one that forgot. See Runner.producedReading.
			r.recordProducedReading(name, v)
			return v, true
		}
	}
	if r.isCompoundVariable(name) {
		// A compound answers with its whole tree laid out over several lines,
		// which is the shell's own — `${c}` there is `(`, a tab-indented line
		// per member, `)`. Ahead of every table because the name is in none
		// of them: see interp/compoundvariable.go.
		return r.compoundVariableText(name), true
	}
	if a, ok := r.Arrays[name]; ok && !r.removed[name] {
		// Ahead of Vars, which holds a copy of one element: the array is the
		// store, and both what a bare name reads and whether it is set at all
		// are read off it rather than off the copy.
		return r.arrayBareName(a)
	}
	if produce, ok := r.DynamicArrays[name]; ok {
		// A produced array read without a subscript, which is the same
		// question the stored one above answers and had no answer here at
		// all: `$FUNCNAME` was empty where bash reads `g`, `${BASH_SOURCE}`
		// was empty where bash names the shell, and `[[ -v FUNCNAME ]]` was
		// false for a parameter this runner had registered — set-ness comes
		// through paramSource and paramSource comes through here (#1600).
		//
		// producedBareName rather than a reading of its own, because which
		// of the two a bare name gives is ArrayScalarIsTheWholeArray's
		// question and a produced array is not a different kind of array:
		// zsh reads `$funcstack` as `g f` and bash reads `$FUNCNAME` as `g`,
		// which is the axis and not two rules. It answers set-ness off the
		// same axis, which is what keeps `[[ -v FUNCNAME ]]` false outside a
		// call in the shell where an empty array has no base element.
		//
		// After the stored table, matching arrayElems — the two have to
		// agree about which wins, or `$a` and `${a[@]}` would read different
		// sources for one name. It is deliberately *not* guarded on
		// r.removed for the same reason: arrayElems does not guard it, and
		// making the scalar view alone honor an `unset` would let the two
		// views disagree. What `unset` should do to a produced array is a
		// question per parameter and per shell — measured, zsh refuses
		// `unset funcstack` as a read-only variable, bash allows
		// `unset FUNCNAME` and refuses `unset BASH_SOURCE` — so it belongs
		// to whichever dialect registers the name, not here.
		return r.producedBareName(produce(r))
	}
	if a, ok := r.assocFor(name); ok && !r.removed[name] {
		// The associative table answers alone rather than falling through:
		// Vars may hold a scalar the name had before it was declared, and no
		// shell reads that back once the attribute is on.
		return r.assocScalar(a)
	}
	if v, ok := r.Vars[name]; ok {
		// Folded here rather than when it was stored, in the shell that
		// keeps what was assigned — see caseFoldsOnRead. This is the one
		// place a stored scalar becomes a value, so `$v`, `${#v}`,
		// `${v:0:1}`, `${v/A/x}`, a `case` subject and a `[[ ]]` operand all
		// see the fold without any of them knowing about it.
		if !folded {
			return v, true
		}
		return r.readAttributeFolded(name, v), true
	}
	if r.removed[name] {
		// `unset` took it away, and neither the environment nor a dynamic
		// parameter is allowed to put it back.
		return "", false
	}

	v, ok := r.inheritedValue(name)
	if !folded {
		return v, ok
	}
	return r.readAttributeFolded(name, v), ok
}

// assignOperands applies the array assignments a declaration utility was given
// as operands, which the parser kept apart from the prefix ones.
func (r *Runner) assignOperands(ctx context.Context, c *syntax.SimpleCmd) {
	for _, a := range c.Assigns {
		if a.Operand {
			r.assign(ctx, a)
		}
	}
}

// arrayLiteralStartsTheNameOver reports whether `a=(x y)` re-creates the name
// rather than replacing its elements, having asked the dialect.
//
// Asked only where the answer could be seen: the name has to be carrying one
// of the attributes there is something to lose — nameCarriesATypeAttribute,
// which is the same list clearTypeAttributes takes off, so the question and
// the answer cannot come apart — and the literal has to be the plain
// assignment spelling rather than a declaration's own operand. Each of those
// was measured — see the fields.
//
// **Three questions, not one**, and the panel answers them differently:
//
//   - The name is already holding an array, and the literal replaces it.
//     ArrayLiteralAssignmentStartsTheNameOver — ksh93 yes, zsh and bash no.
//   - The name is not an array at all, and the literal makes it one.
//     ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver — ksh93 and zsh
//     yes, bash no.
//   - The same as the second, appended rather than assigned.
//     AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver — zsh yes,
//     ksh93 and bash no.
//
// The middle one is #1264 and the last is its append half, which is a
// separate axis because ksh93 keeps on a join what it drops on a store.
func (r *Runner) arrayLiteralStartsTheNameOver(a *syntax.Assign) bool {
	if a.Operand {
		return false
	}
	if !r.nameCarriesATypeAttribute(a.Name) {
		return false
	}
	if !r.nameIsAnArray(a.Name) {
		// Never declared an array and not holding one, so the literal is
		// what makes the name one — and two of the three shells re-create it
		// rather than typing the elements. The `-l` pair is what says the
		// *letter* is the question and not the value: `typeset -l e; e=(AB
		// Cd)` loses the letter in both and `typeset -la f; f=(AB Cd)` keeps
		// it in both, over the same words.
		if a.Append {
			return r.ask(r.sem().AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
				"an appended array literal re-creating a name that is not an array")
		}
		return r.ask(r.sem().ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver,
			"an array literal re-creating a name that is not an array")
	}
	if a.Append || !r.compoundNameHolds(a.Name) {
		// An array the declaration declared, holding nothing yet: the *first*
		// literal a declared name receives keeps the letter and folds —
		// measured, `typeset -a -i c; c=(5+5 6+6)` is `10 12` and lists as
		// `typeset -a -i c=(10 12)` — so what re-creates the name is
		// replacing an array it is already holding. An append never does,
		// which every column agrees about.
		//
		// The array and not "anything", deliberately: a valueless
		// declaration gives the name the empty string in one dialect and
		// nothing in the others, so a guard that asked whether the name held
		// *a value* would have made this axis's answer depend on
		// DeclaredNameWithoutValueIsEmpty's. That coupling is the bug #979
		// was, and it was in the first draft of this line.
		return false
	}
	return r.ask(r.sem().ArrayLiteralAssignmentStartsTheNameOver,
		"a whole-array assignment re-creating the name it writes")
}

// nameIsAnArray reports whether the name is an array at all — declared with
// the letter, built by an element assignment, or produced.
//
// The store *is* the record of the array letter, which is what makes #1264's
// "the letter is recorded nowhere" out of date rather than wrong: markIndexed
// puts an empty array under the name, and compoundNameHolds documents that as
// the reason it asks for `len(a) > 0` instead of membership. So the two
// questions are membership and length of one table, and a second map of
// letters beside it would be a copy that could drift.
func (r *Runner) nameIsAnArray(name string) bool {
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.DynamicArrays[name]; ok {
		return true
	}
	_, ok := r.assocFor(name)
	return ok
}

// clearTypeAttributes takes off the letters that say what a name's values
// *are* — the integer letter, the two case letters, the float precision and
// the width attribute.
//
// Narrower than clearAttributes, which `unset` uses: this is not the name
// going away, so what is measured to go is measured to go, and nothing else
// is guessed at. The listing after a re-creating assignment keeps the array
// letter and loses these, which is what the field records.
//
// Measured 2026-09-12, zsh 5.9.2: `typeset -L 3 c; c=(a bb); typeset -p c`
// writes `typeset -a c=( a bb )` — no `L`, no width — and `typeset -F 3 b;
// b=(1 2)` writes `typeset -a b=( 1 2 )` the same way, beside the `-i`, `-l`
// and `-u` rows that were already here. The width has to go from the store
// and not only from the listing: the name can be assigned a scalar again
// afterwards, and `d=zz` then reads back `zz` rather than `zz ` (#1461).
// nameCarriesATypeAttribute reports whether clearTypeAttributes would take
// anything off the name. It is the guard on asking the re-creation question
// at all, and it is here rather than beside that question so the two lists
// are one list: an attribute added to the clearing and not to the guard is an
// attribute the question is never asked about, which is how the float
// precision and the width both survived a `c=(a bb)` that zsh drops them on.
func (r *Runner) nameCarriesATypeAttribute(name string) bool {
	if r.integer[name] || r.lowered[name] || r.uppered[name] {
		return true
	}
	if _, ok := r.floatPrecision[name]; ok {
		return true
	}
	_, ok := r.fieldWidth[name]
	return ok
}

func (r *Runner) clearTypeAttributes(name string) {
	delete(r.integer, name)
	delete(r.lowered, name)
	delete(r.uppered, name)
	delete(r.floatPrecision, name)
	delete(r.fieldWidth, name)
}

// assignAll performs a bare assignment list, tracing it as it goes.
//
// One loop, and every shell's order comes out of it, because the order follows
// from a single fact: the value is expanded once, and each assignment is stored
// before the next one is expanded. That much is unanimous — `set -x; x=1 y=$x`
// traces `y=1` in all four, so the second value was expanded after the first
// had landed. Expanding the whole list up front traced `y=”` while storing 1,
// which is a trace that disagrees with the run it is describing.
//
// What the dialects split on is when the line can be written. A shell that
// gives each assignment its own line writes it the moment that value is known,
// so a substitution's own trace sits immediately above the line reporting what
// it produced: bash 5.3 and ksh93 answer `set -x; x=$(echo a) y=$(echo b)`
// with `echo a`, `x=a`, `echo b`, `y=b`. A shell that writes one line for the
// whole list cannot write it until the last value is known, so dash answers
// `echo a`, `echo b`, `x=a y=b`. The same axis decides both, which is why it is
// asked once here rather than inside the printing.
//
// Nothing re-expands. Tracing observes a command; it does not run it again, and
// doing so ran the command substitution on a right-hand side twice with both
// sets of side effects (#1915).
func (r *Runner) assignAll(ctx context.Context, assigns []*syntax.Assign) {
	if !r.tracing() {
		for _, a := range assigns {
			r.assign(ctx, a)
		}
		return
	}
	separately := r.ask(r.sem().TraceAssignmentsSeparately,
		"each assignment getting its own trace line")
	values := make([]string, len(assigns))
	prepared := make([]*expandedAssign, len(assigns))
	for i, a := range assigns {
		values[i] = r.expandAssignValue(a.Value)
		prepared[i] = r.prepareTracedAssign(a, values[i])
		// The subscript is the one thing a trace can show that only the
		// *store* knows, because resolving it here as well would resolve it
		// twice — `a[i++]=v` steps `i` once in the column that prints the
		// number. So that column's line waits for the assignment, which is
		// also why a refused one leaves no line at all there.
		after := prepared[i].tracesAfterTheStore
		if separately && !after {
			r.traceAssignments(assigns[i:i+1], values[i:i+1], prepared[i:i+1])
		}
		r.withPreparedValue(ctx, prepared[i])
		if separately && after {
			r.traceAssignments(assigns[i:i+1], values[i:i+1], prepared[i:i+1])
		}
	}
	if !separately {
		r.traceAssignments(assigns, values, prepared)
	}
}

// prepareTracedAssign expands, ahead of the trace, whatever this dialect's
// trace prints the *value* of rather than the text of.
//
// Nothing here is a second expansion: what it produces is handed to the store
// through Runner.expanded, which is the same arrangement assignValue has used
// for a scalar since #1915. What is new is that an array literal's elements
// and an element write's subscript are two more things a trace can print the
// value of, and two more things that must therefore be expanded exactly once.
//
// Asked only where the construct is written, so a dialect that answers neither
// axis is never questioned about a line that has no literal and no subscript
// in it.
func (r *Runner) prepareTracedAssign(a *syntax.Assign, value string) *expandedAssign {
	e := &expandedAssign{assign: a, value: value}
	if a.IsArray && a.Members == nil && a.Index == nil && len(a.Elems) > 0 &&
		r.ask(r.sem().TraceArrayLiteralShowsTheExpandedElements,
			"an array literal traced as what its elements expanded to") {
		if parsed, ok := r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append)); ok {
			e.elems, e.elemsSet = parsed, true
		}
	}
	if a.Index != nil && r.ask(r.sem().TraceElementSubscriptIsEvaluated,
		"a traced subscript written as what it resolved to") {
		e.tracesAfterTheStore = true
	}
	return e
}

// withPreparedValue performs one assignment from what prepareTracedAssign
// expanded for it.
func (r *Runner) withPreparedValue(ctx context.Context, e *expandedAssign) {
	saved := r.expanded
	r.expanded = e
	defer func() { r.expanded = saved }()
	r.assign(ctx, e.assign)
}

// expandedAssign is one assignment's right-hand side, expanded once by
// whoever is about to perform it.
type expandedAssign struct {
	assign *syntax.Assign
	value  string

	// elems is the array literal's element list, expanded once for the
	// dialect whose trace prints what the elements came to rather than what
	// the script wrote. Held here for the same reason value is: expanding a
	// second time to print it would run the second time's side effects, which
	// is the double run #1915 fixed for a scalar.
	//
	// Consumed once, by the store that is about to use it — see
	// Runner.takeExpandedElements. Single use rather than "while this
	// assignment is under way", because a *nested* literal expanded during
	// the same assignment is a different element list and must not be handed
	// the outer one's.
	elems      []literalElem
	elemsSet   bool
	elemsTaken bool

	// subscript is what the store resolved this assignment's subscript to,
	// for the dialect whose trace prints that rather than the text: the
	// number for an index array, the key for a table. Recorded by the store
	// on its way past, which is why the trace for such an assignment is
	// written *after* it has been performed — see Runner.assignAll.
	//
	// Empty where nothing resolved one, which is also what a refused
	// assignment leaves: that column writes no trace line at all for
	// `readonly a=(1); a=(2 3)` or for `a[1/0]=v`, and this is why.
	subscript    string
	subscriptSet bool

	// tracesAfterTheStore says this assignment's line is written once it has
	// been performed, which is the arrangement the subscript above needs.
	tracesAfterTheStore bool
}

// resolvedSubscript records what an assignment's subscript came to, for the
// dialect that traces it. Nothing keeps it where no trace is waiting.
func (r *Runner) resolvedSubscript(text string) {
	if r.expanded == nil || !r.expanded.tracesAfterTheStore {
		return
	}
	r.expanded.subscript, r.expanded.subscriptSet = text, true
}

// assignValue is the value an assignment stores, expanded.
//
// It hands back the value the caller expanded already when there is one,
// because expanding the word a second time *runs* it a second time: the trace
// of a bare assignment prints the value, and printing it by re-expanding ran
// the command substitution on the right-hand side twice, with both sets of
// side effects. Measured on `set -x; x=$(echo . >> f)`, which wrote two bytes
// where the panel writes one (#1915). Tracing observes; it does not run.
//
// A word rather than a flag on the Runner would not do: only the caller knows
// which assignment the value belongs to, and a stale one silently stored the
// previous assignment's value under this one's name.
func (r *Runner) assignValue(a *syntax.Assign) string {
	if r.expanded != nil && r.expanded.assign == a {
		return r.expanded.value
	}
	return r.expandAssignValue(a.Value)
}

// assign performs one assignment, which is three different things wearing the
// same syntax: a scalar, a whole array, or one element of one.
func (r *Runner) assign(ctx context.Context, a *syntax.Assign) {
	// The refusal stands in front of all three, and it used to stand in front
	// of one: setVarAs is where it lived, and only the scalar branch below
	// goes through setVarAs. An element write, an array literal and a
	// compound append each reach a store of their own and every one of them
	// took the write — see refuseReadonly.
	//
	// Only the array form is ever an operand (see syntax.Assign.Operand), so
	// a declaration utility's own `a=(…)` arrives here too — and it is
	// refused as a bare assignment rather than as a declaration, which is not
	// what the spelling suggests. Measured in bash: `declare r=2` and
	// `export r=2` against a frozen name report and run the next command on
	// the same line, and `declare r=2` names the builtin in the complaint,
	// while `typeset -a a=(p q)` does neither — it gives up the rest of the
	// line and names only the variable, exactly as `a=(p q)` and `a[0]=z` do.
	// The array operand goes through the assignment machinery in every shell
	// that has it, and the builtin's name never reaches it.
	if n, ok := positionalAssignIndex(a.Name); ok {
		// A number where the name would be, which one dialect's grammar
		// admits. Ahead of the refusal because a positional parameter cannot
		// be frozen — `readonly 1` is `not an identifier: 1` in the shell
		// that has this — so there is nothing for refuseReadonly to consult
		// and a name of `1` in the frozen set would have come from nowhere.
		r.assignPositional(a, n)
		return
	}
	if r.frozenScalarRetyped(a) {
		// Exempt for the whole store and not just for this check. The
		// literal reaches storeArray, which keeps the scalar view in step
		// through setVarAs — so a name let past here met the same refusal one
		// frame down and printed the sentence anyway.
		outer := r.retypingFrozen
		r.retypingFrozen = a.Name
		defer func() { r.retypingFrozen = outer }()
	} else if r.refuseReadonly(r.frozenNameOfAnAssignment(a.Name), assignedAlone) {
		return
	}
	if r.unspecified {
		return
	}
	switch {
	case a.Members != nil:
		// A compound variable's body, which the parser told apart from an
		// element list by the first word inside the parentheses. Ahead of
		// every array branch because each of them answers to `a.IsArray`,
		// which is true here too: one spelling, two constructs.
		//
		// A subscript alongside it is not a shape this reaches — `c[1]=(a=1)`
		// makes the element a nested *array* of the one string there, which
		// is Semantics.SubscriptedArrayLiteral's question and not this one.
		r.assignCompoundVariable(ctx, a)
		return
	case a.IsArray && a.Index != nil:
		// An array literal *and* a subscript, which is a third thing rather
		// than either of the two below: the words go where the subscript
		// points instead of becoming the whole array. Ahead of both array
		// branches because they answer to `a.IsArray` alone and would take
		// this line, dropping the subscript — which is what made
		// `fpath[$i]=()` empty `fpath` (#1330).
		r.assignElemLiteral(a)
	case a.IsArray && r.isNameref(a.Name):
		// A literal assigned through a reference goes where the reference
		// points; the branches below read the name as itself, which wrote
		// into no container at all. See namerefArrayLiteralTarget for the
		// three answers and what was measured.
		if target, write := r.namerefArrayLiteralTarget(a.Name); write {
			through := *a
			through.Name = target
			r.assign(ctx, &through)
		}
	case a.IsArray && r.assocDeclared(a.Name):
		// The attribute was declared, so the literal's elements are keyed
		// rather than counted.
		r.assignAssocLiteral(a.Name, a.Elems, a.Append)
	case a.IsArray:
		// Each bare element is a word, so `a=(1 $x 3)` expands and splits
		// like any other — which is how an array is built from a command's
		// output — and a `[sub]=value` element places its value instead.
		// `a+=(d)` adds to the end rather than to the first element, which is
		// what makes append two operations sharing a spelling rather than one.
		if r.arrayLiteralStartsTheNameOver(a) {
			// One dialect reads this spelling as a *new* name rather than as
			// new elements for the one that is there, so the attributes go
			// before the elements land — see
			// Semantics.ArrayLiteralAssignmentStartsTheNameOver.
			r.clearTypeAttributes(a.Name)
		}
		if r.unspecified {
			return
		}
		r.assignArrayLiteral(a.Name, a.Elems, a.Append)
	case len(a.Leading) > 0:
		// A chain of subscripts — `a[1][2]=v` — where each one after the
		// first reaches into the value the one before it named. Ahead of
		// every branch below, because each of those reads Index as the
		// *only* subscript and would write the last one straight into the
		// name. See interp/chainassign.go.
		r.assignChainedElement(a)
	case a.Index != nil && a.IndexFlags == nil && wholeArraySubscript(a.IndexText):
		// `a[@]=Z` and `a[*]=Z`, the whole-array spelling on the *left*.
		// Ahead of both the keyed branch and the arithmetic one because each
		// of them would answer it: the table reading takes `@` for a key —
		// which two of the four columns do not — and the arithmetic reading
		// meets a character it has no operand for and complains about the
		// expression, which is nobody's answer at all. See
		// Runner.assignWholeArraySubscript.
		r.assignWholeArraySubscript(a)
	case a.Index != nil && a.IndexFlags != nil && r.assocDeclared(a.Name):
		// A group over a *table* is answered by the letters it holds rather
		// than stored under the text it was written with. Ahead of the
		// ordinary keyed branch because that one would take `(r)v` for a key,
		// which is a plausible wrong element and exactly what this construct
		// must not produce. See Runner.assignFlaggedTableElement.
		r.assignFlaggedTableElement(a)
	case a.Index != nil && r.assocDeclared(a.Name):
		// A declared name takes its subscript as a string, expanded and
		// never evaluated: `m[1+1]=x` stores under the three characters.
		// This is the switch the attribute exists to throw — the same text
		// on an undeclared name falls through to the arithmetic reading.
		r.assignAssocElement(a)
	case a.Index != nil && a.IndexFlags != nil:
		// A flag group names the element instead of an expression naming it:
		// `a[(r)y]=Q` replaces the element whose value is `y`, and
		// `a[(i)nomatch]=W` appends, because `(i)` missing answers one past
		// the last element. See Runner.flaggedAssignIndex.
		idx, ok := r.flaggedAssignIndex(a)
		if !ok {
			return
		}
		text := subscriptSubject(a.IndexText, r.subscriptAsWritten(a.Index))
		if a.Append {
			r.appendArrayElem(a.Name, idx, text, r.assignValue(a))
			return
		}
		r.setArrayElem(a.Name, idx, text, r.assignValue(a))
	case a.Index != nil:
		// The subscript is an expression, and one that will not evaluate ends
		// the script in every shell measured — the same complaint, worded the
		// same way, as the identical text inside `$(( ))`. It used to be a
		// wording of our own that named the array rather than the expression,
		// and it carried on to the next command.
		text := r.joinWord(a.Index)
		// What a refusal quotes back is the subscript as it was *written*,
		// which is not the text the arithmetic reads: `i=-9; a[$i]=q` is
		// `a[$i]: bad array subscript` in the column that names it. Only the
		// boundary refusals take it — an expression that will not evaluate is
		// quoted back expanded in every column (#1373).
		subject := subscriptSubject(a.IndexText, text)
		// A range on the left names a span of elements rather than one, and
		// the value is the single word that replaces the whole span:
		// `a=(1 2 3); a[2,3]=x` is `[1][x]`. Ahead of the single-subscript
		// reading because that one would take the pair for the arithmetic
		// comma's right operand and write element 3, leaving element 2 in
		// front of it — see Runner.assignSpan.
		from, to, outcome := r.assignSpan(a, text)
		switch {
		case outcome == spanReported:
			return
		case outcome == spanResolved && r.spanReplacesElements(a.Name):
			elems, _ := r.arrayElemsOfTheName(a.Name)
			r.spliceElementSpan(a.Name, subject, elems, from, to,
				[]string{r.assignValue(a)})
			return
		case outcome == spanResolved && r.subscriptSplicesCharacters(a.Name):
			// The name is holding a string, so the pair names a span of its
			// *characters* and the value replaces the whole span:
			// `v=abc; v[2,3]=XY` is `aXY`. Under the single-subscript reading
			// below the pair would be the arithmetic comma's right operand and
			// only the last character would move, which is a plausible wrong
			// answer at status 0 — see spliceCharacterSpan for the ends.
			if r.spanIsBelowTheFirstElement(from, to) {
				r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
					"%[1]s[%[2]s]: bad array subscript", a.Name, subject))
				return
			}
			r.spliceCharacterSpan(a.Name, from, to,
				r.assignValue(a), false)
			return
		}
		// Told what the source spelled, because that is what decides whether
		// a comma still in the expanded text separates anything: measured,
		// `a=(p q r s); i="1,2"; a[$i]=Z` writes the *first* element in the
		// shell with ranges, where a written `a[2,3]+=(x)` reads the comma
		// as the operator it is (#2160).
		idx, err := r.subscriptValueAsWritten(subject, text)
		if err != nil {
			r.failedSubscript("%s\n", r.subscriptFailure(text, err))
			return
		}
		// The number, for the trace that writes it — see
		// Semantics.TraceElementSubscriptIsEvaluated. Here rather than at the
		// store below because both the plain write and the append pass
		// through it, and because the value is this one whichever they do.
		r.resolvedSubscript(itoa(idx))
		if a.Append {
			// `a[0]+=Q` appends to element 0. Distinct from `a+=(Q)`, which
			// adds an element after the last: the subscript is what says
			// which of the two `+=` means.
			r.appendArrayElem(a.Name, idx, subject, r.assignValue(a))
			return
		}
		r.setArrayElem(a.Name, idx, subject, r.assignValue(a))
	default:
		value := r.assignValue(a)
		if r.compoundAssignedFromAName(a.Name, value, a.Append) {
			// A bare *name* on the right of an assignment whose target is
			// already a compound copies that variable rather than storing
			// the four characters: `c=(a=1 b=2); d=(a=9); d=c` is
			// `typeset -C d=(a=1;b=2)` there. Only a compound source is read
			// that way, which is why this asks and does not always take the
			// line. See interp/compoundcopy.go.
			return
		}
		if r.assocDeclared(a.Name) && a.Append {
			// `m+=x` over a declared table joins the element whose key is
			// `0`. The plain spelling is not here: what a scalar does to a
			// name already holding a compound is one question for an array
			// and a table alike, and it is asked at the store instead — see
			// scalarOverCompound.
			v, ok := r.appendedValue(a.Name, r.AssocArrays[a.Name]["0"].scalar(), value)
			if !ok {
				return
			}
			r.setAssocElem(a.Name, "0", v)
			return
		}
		if old, ok := r.Arrays[a.Name]; ok && a.Append {
			// `a+=x` over a name holding an *array* joins the array. The
			// lines below are the string append, and taking them would leave
			// a plain scalar where the script built an array — see
			// appendScalarToArray, which is also where the two answers about
			// which element the value joins are asked.
			//
			// Only the append. A plain `a=x` over an array goes on to
			// replace the name, which is #1390's question and not this one.
			r.appendScalarToArray(a.Name, old, value)
			return
		}
		lift := func() {}
		if a.Append {
			// `+=` is its own event. The hook is given **only the part being
			// appended** — `p=base; p+=more` enters `p.append` with
			// `${.sh.value}` as `more` — and rewriting it there is what makes
			// `p` `base<more>`. Measured on ksh93u+ 2012-08-01.
			if v, ran := r.disciplineWrite(a.Name, disciplineAppend, "", value); ran {
				value = v
			}
			// And an append fires *only* that event: with a `.set` discipline
			// and no `.append` one, `p+=more` runs nothing. The store below
			// is the same one a plain assignment ends at, so the mark is what
			// keeps the two events apart.
			lift = r.suppressDiscipline(a.Name, disciplineSet)
			// The *stored* text, which is what an append joins — see
			// storedVar, and Semantics.CaseAttributeFoldsWhenRead for the
			// shell where that is not what a read answers.
			old, _ := r.storedVar(a.Name)
			if r.isCompoundVariable(a.Name) {
				// A compound answers a value — its whole tree as text — and
				// it is not one a scalar append joins: measured,
				// `c=(a=1); c+=z` is `c=z` there and not the rendering with
				// a `z` after it. The whole-name write below then takes the
				// members, which is the other half of the same row. See
				// compoundVariableSubscripted for the subscripted shapes,
				// which keep them.
				old = ""
			}
			// The operator is not the whole of what `+=` means — see
			// appendedValue. An attributed name adds here, and the string
			// join is what is left when the name carries no attribute.
			v, ok := r.appendedValue(a.Name, old, value)
			if !ok {
				lift()
				return
			}
			value = v
		}
		// What this does to an array or a table the name is already holding
		// is setVarAs's, through scalarOverCompound — it used to be a
		// `delete(r.Arrays, a.Name)` written here, which made the assignment
		// *statement* the only spelling that got it right (#1645).
		r.setVarAs(a.Name, value, assignedAlone)
		lift()
		if r.allexport {
			// `set -a`: an assignment marks the name for the environment as
			// well as setting it.
			if r.exported == nil {
				r.exported = map[string]bool{}
			}
			r.exported[a.Name] = true
		}
	}
}

// exitStatus turns a wait error into a status.
//
// A process killed by a signal has no exit code of its own, and Go says -1 —
// which is not a status a shell can report at all. It reached `$?` as -1 and
// the process's own exit code as 255, which is how the run sweep found it:
// /usr/bin/wish is killed by its own launcher and every shell in the panel
// says 137 where we said 255.
//
// The signal goes in the number instead. Which base is the axis.
func (r *Runner) exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return r.signalDeathStatus(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 126
}

// signalDeathStatus is 128 or 256 plus the signal.
//
// Measured across INT, TERM, KILL, QUIT, HUP, PIPE, ABRT and SEGV: bash, dash
// and zsh use 128 for every one of them and ksh93 uses 256 for every one of
// them. Neither is a special case for any particular signal.
func (r *Runner) signalDeathStatus(sig syscall.Signal) int {
	base := 128
	if r.ask(r.sem().SignalDeathStatusIsTwoFiftySix, "the status of a command killed by a signal") {
		base = 256
	}
	return base + int(sig)
}

// retypingFrozen is the name whose freeze one array-literal store is exempt
// from, set only where Semantics.ArrayLiteralOperandRetypesAFrozenScalar said
// so and put back the moment the store is done.
//
// A field rather than an argument threaded through, because the store is four
// frames deep and every one of them is shared with the paths that must still
// refuse: assign, assignArrayLiteral, storeArray and setVarAs. A parameter
// would have to be added to all four and defaulted on the callers that are not
// this, which is the shape that goes wrong silently the next time one of them
// grows a caller.
//
// Empty is the ordinary state and no name is ever "" here, so the comparison
// needs no second flag beside it.

// frozenScalarRetyped reports whether this assignment is the one shape a
// frozen name still takes: a declaration utility's own `name=(…)` operand
// over a name that is not a compound yet.
//
// Asked ahead of refuseReadonly and only where a refusal was actually coming,
// so the axis stays off every path that had no question. Three guards before
// the dialect is reached, and each is a measured discriminator rather than an
// economy — see Semantics.ArrayLiteralOperandRetypesAFrozenScalar for the
// rows:
//
//   - the name is frozen, or there was nothing to be exempt from;
//   - the operand is a declaration's array literal, so a bare `q=(b)` and a
//     scalar `typeset -g q=b` both keep the refusal they had;
//   - the name is not already an array or a table, which is the *retype* half
//     — a frozen array's elements may not be replaced in either shell — and
//     is not a module's absent parameter, whose kind this shell does not know.
//
// An append is not one of these and needs no guard of its own: the spelling
// does not exist on a declaration operand, and `typeset -g q+=(b)` is `not
// valid in this context: q+` in the shell this is for.
func (r *Runner) frozenScalarRetyped(a *syntax.Assign) bool {
	if !r.readonly[a.Name] || !a.Operand || !a.IsArray || a.Index != nil || a.Append {
		return false
	}
	if r.nameIsAnArray(a.Name) {
		return false
	}
	if r.AbsentParameter(a.Name) {
		// A name a module reserved and this shell holds no cell for. It is
		// not a scalar the script may retype: the shell being modeled has a
		// real parameter of a real kind under the name, and it refuses the
		// same line — measured 2026-09-12, `typeset -g jobstates=(a)` is
		// `can't change type of autoloaded parameter` before the module is
		// loaded and `read-only variable: jobstates` after it. What this
		// engine has under the name is a refusal rather than a kind, so it
		// cannot tell a scalar from a table here and must not guess the one
		// answer that lets the write through.
		return false
	}
	return r.ask(r.sem().ArrayLiteralOperandRetypesAFrozenScalar,
		"a declaration's array literal replacing a frozen scalar")
}
