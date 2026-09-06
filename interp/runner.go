// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

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
	// changing is setting the old value straight back, which is what the
	// builtin does and why it cannot be done by a caller who only offered a
	// getter.
	SetUmask func(mask int) (old int, err error)

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

	// procSubs are the named pipes this command's process substitutions made,
	// waiting to be removed once it is done with them.
	procSubs    []procSubPipe
	procSubSeq  int
	procSubHome *procSubDirs
	// substRan records that a command substitution reported a status during
	// the expansion just performed — see simple().
	substRan bool
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
	// funcFiles is where each function was defined, because that is the file
	// its frame reports rather than the one that called it.
	funcFiles map[string]string
	// exportedFuncs are the functions written into a command's environment,
	// and funcExportPrefix/Suffix are what the entry is called. Only one
	// dialect carries functions that way, so the naming comes from it.
	// functionLayout and exportedFunctionLayout are how this shell arranges
	// a function it has to say back — see SetFunctionLayout.
	functionLayout                     syntax.Layout
	exportedFunctionLayout             syntax.Layout
	exportedFuncs                      map[string]bool
	importedFuncs                      bool
	funcExportPrefix, funcExportSuffix string

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

	// Dynamic holds parameters whose value is produced when they are read,
	// rather than stored: `LINENO` is wherever execution has reached, and
	// `RANDOM` is a different number every time. A dialect fills in the ones
	// it has through SetDynamic.
	Dynamic map[string]func(*Runner) string

	// DynamicArrays is the same for arrays, and the call stack is why it
	// exists: `BASH_SOURCE` and its relatives cannot be stored and stay
	// right. A dialect fills in the names it has through SetDynamicArray.
	DynamicArrays map[string]func(*Runner) []string

	// assigned holds what a script assigned to a *produced* parameter, which
	// is a message to whatever produces it rather than a value of its own.
	assigned map[string]string

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
	// optindAssigned records that a script wrote OPTIND since the last
	// `getopts` wrote it. The value cannot say so on its own: resetting it to
	// 1 while a cluster is half read is an assignment of the value it already
	// had, and three of the four restart the word on the strength of the
	// assignment rather than the number.
	optindAssigned bool

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

	// aliases is the table `alias` and `unalias` keep. Substitution happens
	// when a line is parsed, which is the other half of the feature and lives
	// in the parser rather than here; the two meet at [Runner.ExpandingAlias].
	aliases map[string]string

	// aliasExpansion is whether a word being parsed *right now* is replaced
	// by what the table holds for it, and aliasExpansionBase is the answer
	// this shell started with.
	//
	// Two fields rather than one because the pair is what a mode needs. The
	// route a program arrived by decides the base — a dialect's
	// syntax.Dialect.ExpandAliases, which the front end reads and hands here
	// — and two things move the live one off it: `shopt -s expand_aliases`
	// in the one dialect with the name, and POSIX mode, which turns it on for
	// as long as the mode lasts. Measured, and the reason leaving the mode
	// restores the *base* rather than what was set before entering it: `shopt
	// -s expand_aliases; set -o posix; set +o posix; shopt expand_aliases`
	// answers `off` in bash 5.3.
	//
	// Plain bools, so a subshell clone carries its own copy: `(shopt -s
	// expand_aliases)` is the subshell's business, the same as every other
	// option here.
	aliasExpansion     bool
	aliasExpansionBase bool

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
	// loopDepth is how many loops execution is inside right now, which is
	// what a ^Z has to break out of — see breakLoopsForAStop. Dynamic rather
	// than lexical: a loop that calls a function that loops is two, because
	// what the stop is inside is what matters.
	loopDepth int
	// ctx is the context of the current Run, so expansion can reach it. A
	// command substitution runs commands, and threading a context through
	// every expander signature to reach one place would be worse.
	ctx context.Context
	// inBuiltin is the builtin currently speaking, for the one dialect that
	// names it in a diagnostic's location. Empty at every other moment, and
	// deliberately cleared by `.` and `eval` while they run borrowed text:
	// what a sourced script reports is the script's, not the builtin's.
	inBuiltin string
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
	// disabledBuiltins are the names `enable -n` has switched off. Kept
	// apart from custom so that switching one on again gets back whatever
	// was registered rather than the core's.
	disabledBuiltins map[string]bool
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
	// funcLine is the line the function being run was written on, which one
	// dialect counts a message's line from instead of from the top of the
	// file.
	funcLine int
	// sourceDepth is how many sourced files are running, which is the other
	// place a `return` has something to return from. A count rather than a
	// flag because a sourced file may source another.
	sourceDepth int
	// expandErr records that an expansion failed — a division by zero, a
	// number that is not one. The command does not run, which is what every
	// shell in the panel does and what the exit status has to say.
	expandErr bool
	// expandingWord is the word being expanded and expandingSpan which of
	// its spans, so a diagnostic about an expansion can name the text around
	// it: two dialects blame the word rather than the `${…}`, and by the
	// time anything has failed the word is a list of spans. Nil where an
	// expansion was reached from something that is not a word.
	expandingWord *syntax.Word
	expandingSpan int
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
	completions map[string]string

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
	// noexec is `set -n`: read, never run, never unset — even the `set +n`
	// that would clear it is a command.
	noexec bool
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
	// where commands were found, and a shell that searches afresh every
	// time keeps the promise in either state, so the state is real here
	// even though no cache hangs off it — the same honesty `hash` answers
	// with an empty table.
	tracksCommands bool
	// histIgnoreDups is zsh's histignoredups, which its `set -h`
	// abbreviates. It governs a history this shell does not keep, and with
	// no history there are no duplicates to ignore, so either state is kept
	// truthfully.
	histIgnoreDups bool
	// posixMode is `set -o posix`, and the posixSaved fields are the answers
	// the axes it moves held before it was turned on, so turning it off
	// restores the dialect's rather than asserting the standard's opposite.
	// Named fields rather than a saved vector: an option changed *while*
	// posix mode is on is not part of the mode and must survive leaving it.
	//
	// One per axis, and they cannot be collapsed into one: the dialects do
	// not agree on the two, so a single remembered answer would put the wrong
	// one back for whichever shell disagrees with the other axis. zsh carries
	// on past a failed redirection on a special builtin and stops on `unset`
	// of a readonly name; ksh93 is the reverse.
	posixMode               bool
	posixSaved              Answer
	posixSavedUnsetReadonly Answer

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
	jobs    []*Job
	lastJob *Job
	// lastJobPID is `$!`, which is a *value* and not a reference to a job.
	//
	// Separate from lastJob because the two stop being the same thing the
	// moment the job ends. lastJob is the *current* job — what `%%` names and
	// what a bare `fg` picks — so it has to go when the job leaves the table,
	// or those two would name something that is not there. `$!` does not:
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
	// toldOfStoppedJobs says the chunk *before* this one showed the person
	// the jobs that are stopped, so the shell will not hold its exit for them
	// again; tellingOfStoppedJobs is this chunk saying so, and becomes the
	// other at the next one.
	//
	// Two fields because what suppresses the warning is the thing immediately
	// before it and not anything that has ever happened. Measured through a
	// pseudo-terminal: ^Z then `exit` warns and a second `exit` leaves; ^Z
	// then `jobs` then `exit` leaves, because the listing is the shell showing
	// the same thing on purpose; and ^Z, `jobs`, any other command, `exit`
	// warns again. One sticky flag gets the first three right and the fourth
	// wrong.
	toldOfStoppedJobs    bool
	tellingOfStoppedJobs bool
	// bg is set on the runner *inside* a background job, so the process it
	// starts can be recorded against the job.
	bg *Job
	// scopes is the stack `local` unwinds. Shell scoping is dynamic, so
	// there is one set of variables and this records what to put back.
	scopes []*scope
	// redirErr records that a redirection failed to open. The command must
	// not run: a redirect that could not be applied would otherwise send its
	// output to the terminal, which is the loudest possible wrong answer.
	redirErr bool
	// writeFailed records a builtin's output write that failed — into a
	// descriptor closed with `>&-`, most plainly. The write already
	// happened and went nowhere, so there is nothing to retry; the question
	// left is whether the command is said to have worked, and that is the
	// dispatcher's to fold in once the builtin returns. Cleared before each
	// builtin runs, so a failure is only ever read by the builtin it
	// belongs to.
	writeFailed error
	// line is where execution currently is, for diagnostics that name it.
	// Real shells report the line of the command that failed, so this is
	// updated per statement rather than per token.
	line int
	// setOptionStatus is what the last refused `set -o` name reports, which
	// is one of the four dialects' answers rather than a constant.
	setOptionStatus int
	// atInvocation marks a `set` option applied by the front end from the
	// words the shell was started with, rather than by the builtin from a
	// line of script. The panel words the two refusals differently — nobody
	// names `set` at an invocation, and the two shells that print a usage
	// block print the *shell's* there and the builtin's here — so the same
	// refusal has to know which it is. Set for the length of one call in
	// SetOptionLetters and SetNamedOption, which are the front end's only
	// way in.
	atInvocation bool
	// fromEnvironment is the third of those: an option name that arrived in
	// the environment rather than in an argument vector or a script. It is
	// the plainest refusal of the three — the location and the sentence, with
	// nothing standing where `set` would and no usage block — and it is set
	// for the length of one call in ApplyInheritedShellOptions.
	fromEnvironment bool
	// allexport marks every assignment for the environment: `set -a`.
	allexport bool
	// extraOptions are the `set -o` names this dialect has beyond the ones
	// every shell has. Declared through AddSetOptions; see setoptions.go.
	extraOptions map[string]bool
	// promptUser is the login name the `%n` prompt escape reports, brought in
	// by whoever is allowed to ask the system for it. Empty in a runner
	// nobody told, where the escape is refused rather than guessed at.
	// Installed through SetPromptUser; see extend.go.
	promptUser string
	// optionNamespace is the wider set of names `[[ -o name ]]` reads, for a
	// dialect that has one. Nil in a shell whose option names are its
	// `set -o` names and nothing more, which is where `[[ -o ]]` falls back
	// to those. Installed through SetOptionNamespace; see extend.go.
	optionNamespace func(name string) (on, known bool)
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

	// abandonLine is the line the statement that gave up was on, so the rest
	// of that *line* is given up with it. Measured: `r=2; echo one` on one
	// line prints nothing, and `r=2` with `echo one` on the line after it
	// runs the echo.
	abandonLine int

	// assignFailed marks an assignment that was refused rather than made,
	// so the status it left is not zeroed by the assignment that follows
	// it. `readonly x=1; x=2` reports and carries on in one dialect, and
	// carrying on with a status of 0 said the refusal had not happened.
	assignFailed bool

	// linePin overrides the line a node reports, for the dialect that names
	// where a trap fired rather than where in its body a failure was.
	linePin int

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
	// traceWait and traceDone order the trace lines of a pipeline without
	// ordering the pipeline itself: an element waits for the one before it
	// to have printed, then prints, then releases the next. Only the
	// printing is serialized.
	traceWait <-chan struct{}
	traceDone chan struct{}
	traceOnce *sync.Once
	// xtrace is `set -x`: every simple command is printed before it runs.
	xtrace bool
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

	// matchOptions is the run-time pattern behaviors a dialect's builtin has
	// switched on, one bit per MatchOption. A plain value so a subshell's
	// clone carries the state and its changes stay its own.
	matchOptions uint8

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
	// pipeStatus is what the last pipeline's elements reported, and
	// pipeStatusName is what the dialect calls it. The record is only kept
	// when a dialect has named it, because nothing else can read it.
	pipeStatus     []int
	pipeStatusName string
	// regexMatchName is what the dialect calls the record of what the last
	// `=~` captured. With no name, nothing is recorded — see regexmatch.go.
	regexMatchName string
	// shellOptsName is what the dialect calls the variable holding the long
	// names of the options that are on. With no name there is no such
	// variable and nothing is seeded from the environment — see shellopts.go.
	shellOptsName string
	// readonly names refuse assignment.
	readonly map[string]bool
	// integer names evaluate what is assigned to them: with the attribute,
	// `n=5+2` stores 7 rather than the four characters. It is a property of
	// the name and not of the assignment, which is why it is recorded here.
	integer map[string]bool
	// lowered and uppered are the case attributes — `declare -l` and `-u` —
	// which fold what is assigned to the name, the same shape integer has:
	// a property of the name that changes what a later assignment means.
	lowered map[string]bool
	uppered map[string]bool
	// funcs holds defined functions.
	funcs map[string]*syntax.FuncDecl
	// depth bounds function recursion, because a shell script can recurse
	// and a stack overflow is not a diagnostic anyone can act on.
	depth int
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
	c := *r
	c.inSubshell = true
	// A pending process substitution belongs to the command being built in
	// the runner that made it, not to a subshell cloned while it was being
	// built. Carrying them over meant the second `<(…)` of a command cloned
	// the first one's descriptor and then closed it on its way out, so
	// `cat <(echo one) <(echo two)` reported a bad file descriptor for the
	// half it had already opened.
	c.procSubs = nil
	c.Vars = make(map[string]string, len(r.Vars))
	for k, v := range r.Vars {
		c.Vars[k] = v
	}
	c.exported = make(map[string]bool, len(r.exported))
	for k, v := range r.exported {
		c.exported[k] = v
	}
	c.Arrays = make(map[string]Array, len(r.Arrays))
	for k, v := range r.Arrays {
		copied := make(Array, len(v))
		for i, e := range v {
			copied[i] = e
		}
		c.Arrays[k] = copied
	}
	c.AssocArrays = make(map[string]AssocArray, len(r.AssocArrays))
	for k, v := range r.AssocArrays {
		copied := make(AssocArray, len(v))
		for key, e := range v {
			copied[key] = e
		}
		c.AssocArrays[k] = copied
	}
	c.Params = append([]string(nil), r.Params...)
	// The table is copied, the streams in it are shared: a subshell's
	// `exec 7>&1` must not appear in the parent, and its writes through a
	// descriptor the parent made must still land where the parent pointed it.
	c.fds = maps.Clone(r.fds)
	// A subshell begins with the parent's handled traps back at their
	// defaults — see trapsubshell.go for what crosses and what is only
	// still visible.
	c.inheritTraps(r)
	c.completions = maps.Clone(r.completions)
	return &c
}

// withRedirs applies a compound command's redirections around its body. Every
// compound node carries its own list because a redirection on one applies to
// everything inside it.
func (r *Runner) withRedirs(ctx context.Context, rs []*syntax.Redirect, body func() error) error {
	closers, err := r.applyRedirs(ctx, rs, true)
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
	msg := fmt.Sprintf(format, args...)
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
	return r.locationPrefix() + msg
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
	return p.Line + r.lineBase
}

// builtinIsSpeaking reports whether this diagnostic belongs to a builtin,
// which includes a redirection opened for one. Separate from naming the
// builtin, because the two dialects that ask want different answers for a
// failed redirection: ksh93 counts it as the builtin's and zsh does not.
func (r *Runner) builtinIsSpeaking() bool {
	return r.speaking() != "" || r.redirectForBuiltin != ""
}

// locationPrefix is what goes in front of a diagnostic.
//
// The shell's name and the line, except in the dialect that names the
// *function* a message came from and counts the line within it. There the
// file is not mentioned at all, and the count is the offset from the line
// the function was written on — so a body on the same line as its `f() {`
// is offset zero and the number is left out entirely.
func (r *Runner) locationPrefix() string {
	d := r.diag()
	// The dialect's own function is located the way a builtin is: at the line
	// the script called it on, and never as a function — the dialect that
	// names a function in place of a file names the builtin there instead,
	// because to the script there is no function to name.
	if r.inFunc == "" || r.speaker != "" || !d.LocationNamesTheFunction {
		name := r.name()
		if d.LocationNamesTheCurrentFile {
			// The file the failing line was read from: the sourced file while
			// it runs, and the defining file inside a function called later.
			// At the top level of a script the current file is the script,
			// and under `-c` or standard input there is no file at all — the
			// stack answers the shell's own name for both, so neither route
			// changes here.
			if f := r.currentFile(); f != "" {
				name = f
			}
		}
		line := r.line
		if r.speaker != "" {
			line = r.speakerLine
		}
		return d.prefix(name, r.speaking(), r.builtinIsSpeaking(), line)
	}
	if n := r.line - r.funcLine; n > 0 {
		return d.prefix(r.inFunc, r.inBuiltin, r.builtinIsSpeaking(), n)
	}
	// Nothing to count, so nothing is written: `f: ` and not `f:0: `.
	return d.prefixWithoutLine(r.inFunc, r.inBuiltin)
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
func (r *Runner) Run(ctx context.Context, f *syntax.File) (int, error) {
	if err := r.RunPart(ctx, f); err != nil {
		r.runExitTrap(ctx)
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
	// A chunk is a typed line, and whether the shell has just shown the person
	// its stopped jobs is a fact about the line before this one — see the two
	// fields for what that buys over remembering it forever.
	r.toldOfStoppedJobs, r.tellingOfStoppedJobs = r.tellingOfStoppedJobs, false
	r.ensurePWD()
	r.ensureSpecials()
	r.ensureImportedFunctions()
	// Before the descriptors are published, because publishing them is itself
	// an action and the first one this session records.
	r.ensureActionIDs()
	r.publishInheritedFds(ctx)
	if r.started.IsZero() {
		r.started = r.Now()
	}
	r.programEnd = f.End().Line + 1
	abandoned := 0
	for _, st := range f.Stmts {
		if abandoned != 0 && r.lineOf(st.Pos()) == abandoned {
			// The rest of the line the last statement gave up on goes with
			// it. Everything inside a construct has already unwound; this is
			// what makes `r=2; echo one` print nothing where the same two on
			// separate lines run the echo.
			//
			// Never cleared, because a later statement cannot be on an
			// earlier line: the numbers only go up, so a stale one matches
			// nothing. Clearing it was equivalent under mutation, which is
			// how that was established rather than assumed.
			continue
		}
		if err := r.stmt(ctx, st); err != nil {
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
	return nil
}

// Exited reports whether the shell has been asked to stop, so a front end
// feeding it chunks knows not to read another.
func (r *Runner) Exited() bool { return r.ctl == controlExit }

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
	if !r.inSubshell {
		r.stopSignals()
	}
	if r.killedBy != "" && r.DieBySignal != nil {
		// Last, because a shell that is dying still runs its EXIT trap first
		// where the dialect says so. This does not come back.
		if err := r.DieBySignal(r.killedBySig); err != nil {
			r.diagf("kill: %v\n", err)
		}
	}
	return r.status
}

// runExitTrap runs `trap … EXIT` as the script ends.
//
// Once, and only for the main script: a subshell and a command substitution
// both leave it alone, which is unanimous across the panel and the reason
// clone marks its copies.
//
// The status is left as it is so the body can read `$?` — measured: a trap set
// after `false` sees 1. If the body exits with a status of its own, that wins,
// which is why the control flag is cleared first and consulted after.
func (r *Runner) runExitTrap(ctx context.Context) {
	if r.exitTrap == nil || r.inSubshell {
		return
	}
	// A shell that was killed rather than ended is a two-two split: bash and
	// ksh93 treat dying as exiting and run the trap, dash and zsh do not.
	// Asked only where there is a trap and a death to disagree about.
	if r.killedBy != "" && !r.ask(r.sem().ExitTrapRunsOnSignalDeath, "the EXIT trap after a fatal signal") {
		return
	}
	body := *r.exitTrap
	// Cleared before running so the body cannot fire it again, and so a
	// `trap` inside it replaces rather than recurses.
	r.exitTrap = nil
	before := r.status
	r.ctl = controlNone
	// Kept for a bare `exit` inside the body, which in three of the four
	// reports this rather than whatever the body's last command did.
	r.inExitTrap, r.exitTrapEntryStatus = true, before
	r.runTrapBody(ctx, body)
	// Cleared for hygiene rather than for effect: the EXIT trap is the last
	// thing a shell runs, so nothing reads this afterwards.
	r.inExitTrap = false
	if r.ctl != controlExit {
		// The body ran to the end without exiting, so the script keeps the
		// status it already had.
		r.status = before
	}
	r.ctl = controlExit
}

// runTrapBody parses and runs a trap's text, which is re-parsed at fire time
// because that is when a shell reads it.
//
// Whether the part that parsed runs before the failure is reported is a
// question: bash and dash read a line at a time, so `trap "echo a
// if" EXIT` prints `a` and then complains, and ksh93 reads the whole body
// first and prints nothing. zsh never reaches this — it reads the action
// when the trap is set and refuses one that will not parse.
func (r *Runner) runTrapBody(ctx context.Context, body string) {
	defer r.enterTrapBody()()
	p := syntax.NewParser(body, r.dialect())
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
	moved.Pos.Line += by
	if moved.ConstructLine > 0 {
		moved.ConstructLine += by
	}
	if moved.EndLine > 0 {
		moved.EndLine += by
	}
	return &moved
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) error {
	// Whatever arrived while the previous command ran. A shell finishes what
	// it is doing and runs the handler between commands, which is measured
	// and unanimous — so this is the point where a signal becomes visible.
	r.runPendingTraps(ctx)
	if r.ctl != controlNone {
		return nil
	}
	r.statusBefore = r.status
	if st.Background {
		return r.background(ctx, st)
	}
	if err := r.expr(ctx, st.Expr); err != nil {
		return err
	}
	if _, isChain := st.Expr.(*syntax.BinaryExpr); !isChain && !lastIsNegated(st.Expr) {
		// A chain judges itself, inside expr, because only its final operand
		// counts and only when that operand actually ran.
		r.checkErrExit(ctx)
	}
	return nil
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
	pipefailOnly := r.pipefailRaised
	r.runErrTrap(ctx)
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
	r.ctl = controlExit
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
	r.pipefailRaised = false
	if len(p.Cmds) == 1 {
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
		r.recordSingleStatus(p.Cmds[0])
	} else if err := r.runPipeline(ctx, p, timing); err != nil {
		return err
	}
	if p.Negated {
		// `!` applies to the whole pipeline and inverts its status.
		if r.status == 0 {
			r.status = 1
		} else {
			r.status = 0
		}
	}
	return nil
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
		r.line = r.lineOf(c.Pos())
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
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		return r.simple(ctx, x)
	case *syntax.Group:
		return r.group(ctx, x)
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

func (r *Runner) simple(ctx context.Context, c *syntax.SimpleCmd) error {
	// The DEBUG trap fires here, before anything about the command is even
	// expanded — a simple command is its unit, measured: compound headings
	// fire nothing and each command inside one fires its own. An action
	// that exits takes the command it was about to precede with it.
	r.runDebugTrap(ctx)
	if r.ctl != controlNone {
		return nil
	}
	r.unspecified, r.expandErr, r.assignFailed = false, false, false
	// Whatever this command's process substitutions opened is closed when the
	// command is done, whether it turned out to be a builtin, a function or
	// something on PATH.
	//
	// Here rather than beside the exec, which was where it started and was
	// wrong: `echo hi > >(tr a-z A-Z)` never reaches an exec at all, so the
	// pipe stayed open, `tr` waited for an end-of-file that was never coming,
	// and the substitution simply produced nothing.
	defer func() { removeProcSubs(r.takeProcSubs()) }()
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
		r.fatalQuiet()
		return nil
	}
	var argv []string
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
		if i > 0 && len(argv) > 0 && r.declares(argv[0]) && assignShaped(w) {
			argv = append(argv, r.expandAssignArg(w))
			continue
		}
		argv = append(argv, r.expandWord(w)...)
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
		// A failed arithmetic expansion is fatal to the script in every
		// shell measured — dash, bash, ksh93 and zsh all abandon the rest of
		// the list rather than run the next command. That much the core can
		// decide, and running on was the silent wrong answer: the corpus case
		// added for the status axis is what caught it.
		//
		// *Which* non-zero status it carries is the axis, asked inside
		// fatalQuiet. The diagnostic was already written by whoever failed,
		// so this adds none.
		r.fatalQuiet()
		return nil
	}
	if r.ctl == controlExit {
		// An expansion raised a fatal error of its own — an unmatched
		// pattern, where the dialect calls that an error rather than passing
		// it through. The command does not run, and nothing below may
		// overwrite the status it set.
		return nil
	}

	// `$_` moves to this command's last expanded argument before it runs,
	// so the command's own expansion saw the previous one's — and a bare
	// assignment moves it to empty. Tracked unconditionally and cheaply;
	// whether a read of `$_` answers with it is the dialect's question,
	// asked where the read happens rather than on every command here.
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
		if r.xtrace {
			values := make([]string, 0, len(c.Assigns))
			for _, a := range c.Assigns {
				values = append(values, strings.Join(r.expandWord(a.Value), " "))
			}
			r.traceAssignments(c.Assigns, values)
		}
		for _, a := range c.Assigns {
			r.assign(a)
		}
		if r.ctl == controlExit || r.ctl == controlAbandon {
			// A readonly reassignment is fatal in three of the four shells
			// and abandons the statement in the fourth. Zeroing the status
			// here is what made it look survivable: the script stopped, and
			// then reported success for having done so.
			return nil
		}
		if !r.substRan && !r.expandErr && !r.assignFailed && !r.unspecified {
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

			closers, err := r.applyRedirs(ctx, c.Redirs, false)
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

	r.traceCommand(argv)

	// On the record while the redirections are opened, so a dialect that
	// counts a redirection opened for a builtin as the builtin's own can
	// say so. Cleared before the builtin runs: from there on it is the
	// builtin itself that is speaking.
	if len(argv) > 0 {
		if _, ok := r.lookupBuiltin(argv[0]); ok {
			r.redirectForBuiltin = argv[0]
		}
	}
	closers, err := r.applyRedirs(ctx, c.Redirs, false)
	r.redirectForBuiltin = ""
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
		if specialBuiltins[argv[0]] &&
			r.ask(r.sem().RedirectErrorOnSpecialBuiltinFatal, "a failed redirection on a special builtin ending the script") {
			r.fatalQuiet()
		}
		return nil
	}

	// A function shadows a builtin and an external command alike.
	if fn, ok := r.funcs[argv[0]]; ok {
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
		var undo []savedVar
		for _, a := range c.Assigns {
			if a.Operand {
				// An argument to the builtin, not a prefix to it.
				continue
			}
			v := strings.Join(r.expandWord(a.Value), " ")
			if !specialBuiltins[argv[0]] || !r.ask(r.sem().AssignmentPrefixPersistsOnSpecialBuiltin, "an assignment before a special builtin persisting") {
				old, present := r.Vars[a.Name]
				undo = append(undo, savedVar{
					name: a.Name, value: old, present: present, removed: r.removed[a.Name],
				})
			}
			r.setVar(a.Name, v)
		}
		defer r.restoreVars(undo)
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
		locks := argv[0] == "readonly"
		if locks {
			r.assignOperands(c)
		}
		st := r.callBuiltin(ctx, argv[0], fn, argv[1:])
		r.inBuiltin = outer
		if st == 0 && !locks {
			r.assignOperands(c)
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
	env := r.environ()
	for _, a := range c.Assigns {
		if a.Operand {
			// Unreachable as things stand — every name that takes an operand
			// assignment is a builtin, so no external command ever gets here
			// with one. Kept so that the two kinds are told apart wherever
			// assignments are read, rather than in some of the places.
			continue
		}
		env = append(env, a.Name+"="+strings.Join(r.expandWord(a.Value), " "))
	}
	return r.exec(ctx, argv, env)
}

func (r *Runner) exec(ctx context.Context, argv, env []string) error {
	// This runner's PATH, not the process's — see lookpath.go for why that
	// distinction is the whole bug and not a detail.
	path, lookErr := r.lookPath(argv[0])
	if lookErr != nil {
		path = argv[0]
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
	if r.bg != nil || r.WaitForCommand != nil {
		// A process group of its own, which is what makes signaling and
		// terminal ownership answerable at all — for a foreground command as
		// much as a background one, once there is something able to notice it
		// stopped.
		setProcessGroup(cmd)
	}
	cmd.Dir = r.Dir
	cmd.Env = env
	// The fields rather than the resolved streams, so that a nil one reaches
	// os/exec as nil and the child is given /dev/null. That is the same
	// emptiness the shell's own reads and writes get, spelled the way a
	// process spells it — and cheaper, since a non-file reader or writer
	// makes os/exec build a pipe and copy through it.
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	// The descriptors past the three named streams, rebuilt into the child's
	// own table — see childFiles for why that has to be done by hand and why
	// the numbering is preserved rather than packed.
	cmd.ExtraFiles = r.childFiles()

	if r.bg != nil {
		// Started rather than run, so the pid can be recorded before it is
		// waited for — `$!` has to be answerable immediately.
		if err := cmd.Start(); err != nil {
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			r.diagf("%s: %v\n", argv[0], err)
			r.status = 126
			return nil
		}
		r.bg.PID = cmd.Process.Pid
		// The pid is final now, so anything waiting to read `$!` may proceed
		// while this goroutine blocks on the process.
		r.bg.markReady()
		err := cmd.Wait()
		r.status = r.exitStatus(err)
		r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
		return nil
	}

	if r.WaitForCommand != nil {
		return r.runWatched(ctx, cmd, argv, action)
	}

	err := cmd.Run()
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
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
	}
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
	return nil
}

// runWatched runs a foreground command through the caller's own wait, which is
// the only kind that can report a command that *stopped*.
//
// Started rather than run: the wait is the caller's, so this must not also be
// waiting — two waits on one child is a race over who reaps it, and the loser
// gets an error instead of a status.
func (r *Runner) runWatched(ctx context.Context, cmd *exec.Cmd, argv []string, action Action) error {
	if err := cmd.Start(); err != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
	}
	pid := cmd.Process.Pid
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
	w, err := r.WaitForCommand(pid)
	if err != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
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
		if k == r.shellOptsName && k != "" {
			// The option record is produced, so what a child must be handed
			// is this shell's options *now* and not the string this shell was
			// launched with. Nothing else in Vars can supersede it — it is
			// readonly and never stored — so this entry is the only place the
			// stale value could reach a command, and it is where it did:
			// a shell handed `xtrace` that then ran `set +x` would still have
			// been turning tracing on in everything it started. Measured, the
			// shell that has this variable hands the recomputed value down.
			out = append(out, k+"="+r.shellOptions())
			continue
		}
		out = append(out, kv)
	}
	// The functions this shell was told to carry, written as source because
	// there is nothing but a string to carry them in.
	out = append(out, r.functionEnviron()...)
	for k, v := range r.Vars {
		// Only exported names reach a command's environment; the rest are
		// the shell's own.
		if r.isExported(k) {
			out = append(out, k+"="+v)
		}
	}
	return out
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
	for k := range r.inheritedEnv {
		if k == name {
			return true
		}
	}
	return false
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

// inheritedValue is the value a name was born with, if the shell was handed
// one and `unset` has not taken it away.
func (r *Runner) inheritedValue(name string) (string, bool) {
	for k, v := range r.inheritedEnv {
		if k == name {
			return v, true
		}
	}
	return "", false
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
	// savedAssoc shadows the associative table the same way, attribute and
	// all: what comes back on exit is whether the name was associative as
	// much as what it held.
	savedAssoc   map[string]AssocArray
	assocExisted map[string]bool
	// savedExported and exportedSpoken are the export attribute a shadowed
	// name had, for the dialects where a local does not inherit it. Taking
	// the attribute off is a change to the runner's record and has to be put
	// back like the value, and the record is a tri-state — so what was there
	// is two maps rather than one: whether it had been spoken about at all,
	// and what it said.
	savedExported  map[string]bool
	exportedSpoken map[string]bool
	// keyword records that the function was defined with the `function` word
	// rather than with parentheses. ksh93 gives only those functions a local
	// scope, so `typeset` needs to know which kind it is standing in.
	keyword bool
}

// fatal reports an error that abandons the script.
//
// Every fatal error goes through here so the three things that make one are
// decided in a single place: the diagnostic, the status — which is an axis,
// dash saying 2 where the others say 1 — and the unwinding. Setting the
// status and forgetting the unwinding is the bug this replaces, and it had
// been written independently at three sites.
// fatalQuiet is fatal for a failure that has already reported itself.
func (r *Runner) fatalQuiet() {
	if r.ask(r.sem().FatalErrorStatusIsOne, "the exit status of a fatal error") {
		r.status = 1
	} else {
		r.status = 2
	}
	r.ctl = controlExit
}

func (r *Runner) fatal(format string, args ...any) {
	r.diagf(format, args...)
	r.fatalQuiet()
}

func (r *Runner) setVar(name, value string) { r.setVarAs(name, value, assignedAnyhow) }

// savedVar is one variable's state before a transient assignment, held so the
// assignment can be taken back: the value it had, whether it was set at all,
// and whether `unset` had removed it from view.
type savedVar struct {
	name    string
	value   string
	present bool
	removed bool
}

// restoreVars takes back transient assignments, most recent first.
func (r *Runner) restoreVars(undo []savedVar) {
	for i := len(undo) - 1; i >= 0; i-- {
		u := undo[i]
		if u.present {
			r.Vars[u.name] = u.value
		} else {
			delete(r.Vars, u.name)
		}
		if u.removed {
			r.removed[u.name] = true
		} else {
			delete(r.removed, u.name)
		}
	}
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
)

// setVarAs sets a variable, knowing how the assignment was written.
func (r *Runner) setVarAs(name, value string, form assignForm) {
	if r.readonly[name] {
		// Fatal everywhere but bash, measured with a plain assignment in a
		// script — which is the contaminated-probe case oracle.md records.
		//
		// And in bash it is fatal after all when the program came from an
		// argument: `bash -c 'readonly x=1; x=2; echo after'` stops and
		// exits 1, where the same three lines in a file print `after` and
		// exit 0. Only for an assignment standing alone — `export x=2` and
		// `x=2 cmd` are not fatal there either way.
		// Two arguments only where the wording asks for two: a format with
		// no explicit indexes and a spare argument becomes "%!(EXTRA …)",
		// which is what Wording's own note is about.
		msg := Wording(r.diag().ReadonlyVariable, "%s: readonly variable", name)
		if form == assignedByDeclaration && r.diag().ReadonlyVariableInDeclaration != "" &&
			r.diag().ReadonlyRefusalNamesBuiltin[r.inBuiltin] {
			msg = Wording(r.diag().ReadonlyVariableInDeclaration, "", name, r.inBuiltin)
		}
		// The builtin has been taken for the wording above where a dialect
		// wants it, and this message does not carry it in the *location* in
		// the dialect that puts it there for everything else: zsh writes
		// `zsh:1: read-only variable: x` from inside `export`, not
		// `zsh:export:1:`. So it is put aside for the report and given back.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
		fatal := r.sem().ReadonlyReassignmentFatal
		switch {
		case form == assignedAlone && r.Route == RouteCommandString:
			fatal = r.sem().ReadonlyReassignmentFatalFromCommandString
		case form == assignedByDeclaration:
			// A third answer, and a different set of shells from either of
			// the two above: `export x=2` stops dash, ksh93 and zsh, and
			// bash reports it and carries on — by both invocation routes.
			fatal = r.sem().ReadonlyReassignmentByDeclarationFatal
		}
		if r.ask(fatal, "a readonly reassignment being fatal") {
			r.fatal("%s\n", msg)
			return
		}
		r.diagf("%s\n", msg)
		r.status, r.assignFailed = 1, true
		// Reported and not fatal, and the shell still gives up what it was
		// running: `readonly r=1; r=2; echo one` never prints `one`, and the
		// line after it runs. Measured in every shape that encloses a
		// statement — a loop, a function body, an `if`, a group, a subshell.
		//
		// A *declaration* does not give anything up. `export x=2`, `declare
		// x=2` and `readonly x=2` against a readonly name all report and run
		// the next command on the same line, which is the tell that this is
		// about a bare assignment failing rather than about the refusal.
		if form != assignedByDeclaration {
			r.ctl, r.abandonLine = controlAbandon, r.line
		}
		return
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	if r.integer[name] {
		// The name was declared integer, so what is assigned to it is an
		// expression rather than text.
		v, ok := r.integerValue(value)
		if !ok {
			return
		}
		value = v
	}
	// The case attributes, folded at assignment the way the integer
	// attribute evaluates there: `declare -l v; v=ABC` stores `abc` in both
	// shells that spell the letter.
	switch {
	case r.lowered[name]:
		value = strings.ToLower(value)
	case r.uppered[name]:
		value = strings.ToUpper(value)
	}
	if _, dynamic := r.Dynamic[name]; dynamic {
		// Assigning a produced parameter is a message to its producer rather
		// than a replacement for it.
		if r.assigned == nil {
			r.assigned = map[string]string{}
		}
		r.assigned[name] = value
		delete(r.removed, name)
		return
	}
	if name == "OPTIND" {
		// Noted rather than compared: see optindAssigned.
		r.optindAssigned = true
	}
	r.Vars[name] = value
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
	r.setVar("PWD", r.workDir())
}

func (r *Runner) getVar(name string) (string, bool) {
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
			return f(r), true
		}
	}
	if a, ok := r.Arrays[name]; ok && len(a) > 1 && !r.removed[name] {
		// Ahead of Vars, which holds the first element: with more than one
		// element the two views differ and the dialect decides.
		return r.arrayScalar(r.readArray(a)), true
	}
	if a, ok := r.AssocArrays[name]; ok && !r.removed[name] {
		// The associative table answers alone rather than falling through:
		// Vars may hold a scalar the name had before it was declared, and no
		// shell reads that back once the attribute is on.
		return r.assocScalar(a)
	}
	if v, ok := r.Vars[name]; ok {
		return v, true
	}
	if r.removed[name] {
		// `unset` took it away, and neither the environment nor a dynamic
		// parameter is allowed to put it back.
		return "", false
	}

	return r.inheritedValue(name)
}

// assignOperands applies the array assignments a declaration utility was given
// as operands, which the parser kept apart from the prefix ones.
func (r *Runner) assignOperands(c *syntax.SimpleCmd) {
	for _, a := range c.Assigns {
		if a.Operand {
			r.assign(a)
		}
	}
}

// assign performs one assignment, which is three different things wearing the
// same syntax: a scalar, a whole array, or one element of one.
func (r *Runner) assign(a *syntax.Assign) {
	switch {
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
		r.assignArrayLiteral(a.Name, a.Elems, a.Append)
	case a.Index != nil && r.assocDeclared(a.Name):
		// A declared name takes its subscript as a string, expanded and
		// never evaluated: `m[1+1]=x` stores under the three characters.
		// This is the switch the attribute exists to throw — the same text
		// on an undeclared name falls through to the arithmetic reading.
		key := r.subscriptText(a.Index)
		value := r.expandAssignValue(a.Value)
		if a.Append {
			// `m[k]+=v` joins the element it names, the same operation the
			// indexed form performs on a subscript — an unset key leaves
			// nothing in front of the value.
			value = r.AssocArrays[a.Name][key] + value
		}
		r.setAssocElem(a.Name, key, value)
	case a.Index != nil:
		// The subscript is an expression, and one that will not evaluate ends
		// the script in every shell measured — the same complaint, worded the
		// same way, as the identical text inside `$(( ))`. It used to be a
		// wording of our own that named the array rather than the expression,
		// and it carried on to the next command.
		text := r.joinWord(a.Index)
		idx, err := r.subscriptValue(text)
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(text, err))
			return
		}
		if a.Append {
			// `a[0]+=Q` appends to element 0. Distinct from `a+=(Q)`, which
			// adds an element after the last: the subscript is what says
			// which of the two `+=` means.
			r.appendArrayElem(a.Name, idx, text, r.expandAssignValue(a.Value))
			return
		}
		r.setArrayElem(a.Name, idx, text, r.expandAssignValue(a.Value))
	default:
		value := r.expandAssignValue(a.Value)
		if r.assocDeclared(a.Name) {
			// A scalar assignment to a declared name lands on the element
			// whose key is `0`, keeping the rest — measured in the two
			// shells that read a plain `$m` as that element; the whole-array
			// shell replaces the table instead, which no script can watch
			// for without also depending on the scalar axis itself.
			if a.Append {
				value = r.AssocArrays[a.Name]["0"] + value
			}
			r.setAssocElem(a.Name, "0", value)
			return
		}
		if a.Append {
			old, _ := r.getVar(a.Name)
			value = old + value
		}
		r.setVarAs(a.Name, value, assignedAlone)
		if r.allexport {
			// `set -a`: an assignment marks the name for the environment as
			// well as setting it.
			if r.exported == nil {
				r.exported = map[string]bool{}
			}
			r.exported[a.Name] = true
		}
		// A scalar assignment replaces any array of the same name.
		delete(r.Arrays, a.Name)
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
