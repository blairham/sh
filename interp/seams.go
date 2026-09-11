// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package interp executes a syntax tree.
//
// Two seams are built in from the first commit rather than added later, and
// docs/design.md gives the reason: a policy enforced *outside* the interpreter
// does not reach inside it. `eval`, `source`, command substitution and
// subshells all re-enter with their own input, so anything that must hold
// universally — a sandbox boundary, an audit trail, a permission prompt — has
// to live at the point of execution or the first `eval` walks around it.
//
// Sandboxing, agent permission prompts and AI context turn out to be the same
// two questions: may this proceed, and what just happened. So there are two
// primitives rather than three subsystems.
package interp

import (
	"context"
	"syscall"
)

// ActionKind is what an action does. These are the points where control
// crosses the interpreter's own memory, which is the complete boundary worth
// recording: everything else is arithmetic on values it already holds.
//
// Almost all of them cross outwards and are gated. ActionInherit is the one
// that crosses inwards — a capability the shell was handed before it read a
// line — and it is recorded rather than gated, for the reason set out on it.
type ActionKind uint8

const (
	// ActionExec runs a program.
	ActionExec ActionKind = iota
	// ActionOpen opens a file: a redirection's target, the file `.` reads,
	// the shell's own end of a process substitution's pipe.
	//
	// Not every file the interpreter touches is an open the gate sees. The
	// scaffolding a process substitution stands on — the temporary directory
	// made for its pipes, the mkfifo that creates one, the pipe itself, the
	// regular file `=(cmd)` writes instead of a pipe, their removal — is
	// deliberately outside the boundary: those paths are chosen by the
	// interpreter, never by the script, and gating them would let a policy
	// refuse the mechanism while believing it refused an access.
	//
	// The pipe was on the wrong side of that sentence until #941, and the
	// rule's own words are what put it right: a script writes `<(cmd)` and
	// can never write the pipe's name, because the directory is made per
	// shell with a name the operating system picks. So it is recorded — an
	// EventAccess naming it, wherever in the shell the open happens — and
	// never refused. Runner.ownPipe is the recognition and carries the whole
	// argument, including what is still refused, which is everything worth
	// refusing: the inner command is an ActionExec, and it runs in a Runner
	// of its own whose every access passes the gate.
	ActionOpen
	// ActionStat asks whether a path exists and what it is: a `test -f`, a
	// `[[ -d ]]`, `cd`'s check of where it is going, each candidate a PATH
	// or CDPATH search tries, glob descent deciding what is a directory,
	// `.`'s search for a readable file.
	//
	// Reading where a symlink points is the same action, because it is the
	// same question about the same component. `cd -P` and `pwd -P` are the
	// callers that matter: reporting where a directory *is* means following
	// every link on the way there, so each component of that walk is a stat
	// of its own rather than one stat of the answer. A single stat is what
	// this used to be, with the walk done by the standard library outside
	// the boundary, and a policy refusing a subtree could neither stop
	// `cd -P` crossing it nor learn that it had.
	//
	// A denied stat answers as a missing path does, quietly: the file tests
	// go false, glob drops the candidate, a search walks on, and `cd` fails
	// the way it fails for a directory that is not there. No diagnostic and
	// no status of its own, because to the construct asking, a path the
	// policy hides is a path that does not exist.
	ActionStat
	// ActionReadDir lists a directory: glob descending into it, `**`
	// enumerating what is beneath it.
	//
	// A denied read yields no entries, quietly — the directory reads as
	// empty, exactly as it does when the process may not list it.
	ActionReadDir
	// ActionSignal sends a signal to a process: `kill -9 1234`, `kill %1`
	// reaching a job's process group, `kill -0` asking whether a process is
	// there at all.
	//
	// The boundary is the system call rather than the builtin, which is what
	// makes the exemption exact. A signal a script aims at *this* shell and
	// this shell alone never reaches the kernel — a trap for it runs the
	// shell's own handler and an untrapped fatal one stops the script and
	// leaves the dying to the driver — so nothing leaves the process and
	// there is nothing to gate. Everything else asks the kernel: another
	// process, a process group, an existence probe, and a stopping signal
	// aimed here, because that one really does need the kernel.
	//
	// PID is the target as kill(2) takes it, so a negative value names a
	// process group. Signal is what is being sent, and 0 is the probe that
	// delivers nothing.
	//
	// A denied signal answers EPERM, which is the errno for a process this
	// one may not signal — not an error of the gate's own, for the reason a
	// denied stat is ENOENT-shaped: `kill` reports it with the wording it
	// already has, so a policy hiding a process and a kernel refusing one
	// are indistinguishable to the script.
	ActionSignal
	// ActionInherit records a descriptor the shell was started holding and
	// has just published to the script — `sh 3<&0 script`, where 3 is
	// readable as `<&3` because the caller opened it. Path names it the way
	// the operating system does, /dev/fd/N.
	//
	// It is the one action the gate is never asked about, and that is a
	// decision rather than an omission. Every other kind here has a
	// construct behind it: a line to name, a command to refuse, a status to
	// report. This has none — the table is filled before the first statement
	// runs, so there is nothing to fail — and a veto would be a promise the
	// boundary cannot keep. The descriptor is in the *process's* table
	// whatever this package decides: it is not close-on-exec, which is how
	// it was recognized, so `exec cmd` hands it to its replacement by number
	// and any external child inherits it, both without passing through
	// anything interp holds. Refusing publication would hide it from `<&3`
	// and leave it reachable through both, which is a boundary that reports
	// more than it enforces.
	//
	// The control that is real is total rather than per-descriptor, and it
	// is upstream: a Runner is handed the descriptors it may publish
	// (Runner.InheritedFiles) and never goes looking, so an embedder that
	// wants a script to see none of its own descriptors gives it none. What
	// the boundary owes here is the record — a shell that starts already
	// holding a file it never opened is exactly what an audit trail must
	// contain — so this is emitted as an EventAccess, always, and asks the
	// gate nothing.
	ActionInherit
)

func (k ActionKind) String() string {
	switch k {
	case ActionOpen:
		return "open"
	case ActionStat:
		return "stat"
	case ActionReadDir:
		return "read-dir"
	case ActionSignal:
		return "signal"
	case ActionInherit:
		return "inherit"
	}
	return "exec"
}

// Action describes something about to happen.
type Action struct {
	// ID names this action, and is the same string on the Action a Gate is
	// consulted about and on every Event that action produces. That is the
	// whole of what it promises: a permission request and the records of what
	// was permitted are provably the same action, rather than two records that
	// look alike.
	//
	// It is needed because ordering does not answer the question. A background
	// job and each half of a pipeline emit from their own goroutines, so a
	// command's start and its end are not adjacent and cannot be paired by
	// position — a consumer that matched them by a fingerprint of kind, path
	// and arguments got the wrong one whenever a script ran the same command
	// twice at once.
	//
	// Unique within a session and not beyond it: it is a small counter, so the
	// pair (Runner.Session, ID) is what is unique everywhere, and a stream that
	// several shells append to carries the session beside it for exactly that
	// reason. Empty when nothing is watching — a Runner with no Gate and no
	// Sink numbers nothing, because there is no record for an id to appear in.
	//
	// Deliberately not the stream's sequence number, which is a different
	// thing: seq orders emission within one stream, and this identifies one
	// action across every stream that mentions it.
	ID   string
	Kind ActionKind
	// Path is the program or file.
	Path string
	// Args is the argument vector for an exec, nil otherwise.
	Args []string
	// Write is set when an open is for writing.
	Write bool
	// Resolved is the kernel's own name for the object an open reached, when
	// the name in Path reached somewhere else. Empty otherwise, which is
	// every action but a resolved open — a name is almost always the object's
	// own name.
	//
	// It exists because a record that says only one of the two names cannot be
	// acted on. "This script read /srv/data/x, and /srv/data is a link to
	// /mnt/vol1" is the fact somebody reviewing a run wants, and until #943
	// the shell learned it and threw it away: an allowed open recorded the
	// name the script wrote and nothing said the name had gone elsewhere,
	// while a refused one recorded the object and nothing said which name
	// reached it. Each record held half of one fact.
	//
	// So the pair is on every record about a resolved open, and Path means one
	// thing there rather than two: **the name that was asked about**, which is
	// what it means for an exec, a stat, a listing and a signal.
	//
	// # The Action a Gate is consulted about is not shaped this way
	//
	// A second consultation — the one that happens when an open's name reached
	// somewhere else — carries the *object* in Path, with Resolved set to the
	// same string. That looks redundant and is deliberate: a decision must be
	// about the object, so the object is what a rule matches, and a Gate that
	// has never heard of this field goes on deciding correctly. Handing it the
	// name as well would invite matching on the name, which is the bug #703
	// exists to have closed.
	//
	// Resolved being non-empty is therefore how a Gate tells the second
	// consultation from the first — what a prompting one needs in order not to
	// ask a person the same question twice about one open.
	//
	// # What this does not change
	//
	// Nothing the *script* is told. A refusal names the path as written and
	// never where it went, because a diagnostic that reported the target would
	// hand a script the one fact the rule exists to withhold and would let it
	// map a hidden directory one link at a time by asking to be refused. That
	// asymmetry is between *parties* — the script is told one thing, the
	// operator another — and this field is entirely inside the operator's
	// half. Filling it in makes the records consistent with each other; it
	// does not make the script's view symmetric with the operator's, and must
	// not.
	Resolved string
	// PID is the process a signal is aimed at, as kill(2) takes it: a
	// negative value names a process group. Meaningful for ActionSignal.
	PID int
	// Signal is the signal being sent, for ActionSignal. Zero there is the
	// existence probe rather than a signal, which is `kill -0`.
	Signal syscall.Signal
}

// Decision is a gate's answer.
type Decision uint8

const (
	// Allow lets the action proceed.
	Allow Decision = iota
	// Deny refuses it. The interpreter reports the refusal and carries on
	// with a failing status rather than aborting, because a denied command
	// is a command that failed, not a broken shell.
	Deny
)

// Gate decides whether an action may proceed.
//
// A nil Gate allows everything, so the zero value of a Runner is usable and
// nothing has to opt in to being unsandboxed.
//
// Called from more than one goroutine. A background job runs on its own, and
// so does each half of a pipeline, so a gate that keeps anything — a count, a
// log, a set of paths already allowed — has to guard it. The first gate
// written against this package was a closure appending to a slice, and it had
// a data race the moment a script said `&`.
type Gate interface {
	Allow(ctx context.Context, a Action) Decision
}

// GateFunc adapts a function to Gate.
type GateFunc func(context.Context, Action) Decision

func (f GateFunc) Allow(ctx context.Context, a Action) Decision { return f(ctx, a) }

// EventKind classifies an event.
type EventKind uint8

const (
	// EventCommandStart is emitted before a command runs, after the gate
	// allowed it.
	EventCommandStart EventKind = iota
	// EventCommandEnd carries the exit status.
	EventCommandEnd
	// EventDenied is emitted when a gate refused.
	EventDenied
	// EventError carries a failure that was not an exit status — a program
	// that could not be found, a file that could not be opened.
	EventError
	// EventAccess records an action that went ahead: an open, a stat, a
	// directory read, a signal — Action.Kind says which. Without it a Sink
	// held every file the shell failed to open and none it opened, which is
	// backwards for an audit trail.
	//
	// For an open it follows the successful open. For a stat or a directory
	// read it records the probe itself, whichever answer came back: an
	// existence probe is the auditable act whether or not the file exists,
	// and "not there" is an answer rather than a failure. A signal is
	// recorded the same way and for the same reason — the auditable act is
	// aiming one, and a target that has already exited is an answer.
	EventAccess
)

func (k EventKind) String() string {
	switch k {
	case EventCommandEnd:
		return "command-end"
	case EventDenied:
		return "denied"
	case EventError:
		return "error"
	case EventAccess:
		return "access"
	}
	return "command-start"
}

// Event is a structured record of something that happened.
//
// It is structured rather than a formatted line because its consumers are not
// people: an agent protocol streams it, an AI assistant assembles context from
// it, an audit trail stores it. Formatting is the caller's business.
//
// What a command wrote is deliberately not carried. The streams are the
// caller's own io.Writers already, handed to the Runner before anything ran,
// so a consumer that wants output taps the writer it supplied — copying every
// byte through the event path as well would buy a second copy of what the
// caller already holds.
type Event struct {
	Kind   EventKind
	Action Action
	// Status is the exit status, meaningful for EventCommandEnd.
	Status int
	// Err is the failure, for EventError.
	Err error
	// Line is where in the source the action came from — the line a
	// diagnostic about it would name.
	Line int
	// File is the file that line is in: the sourced file while one runs,
	// the script otherwise, and empty when the input was a command string
	// or standard input.
	File string
	// Session identifies the shell this event came from, copied from
	// Runner.Session so that records written by different consumers of one run
	// can be lined up against each other.
	//
	// A run has two records of itself at least — an audit stream and whatever
	// a front end keeps of what it ran — and without this they describe the
	// same commands and cannot be joined. It is the embedder's string rather
	// than one this package invents: interp does not generate identity, for
	// the same reason it does not read a clock on an event's behalf, and a
	// front end that has no use for one leaves it empty.
	Session string
}

// Sink receives events. A nil Sink discards them.
//
// Called from more than one goroutine, for the same reason a Gate is: what a
// background job does is reported from the goroutine running it.
type Sink interface {
	Emit(ctx context.Context, e Event)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(context.Context, Event)

func (f SinkFunc) Emit(ctx context.Context, e Event) { f(ctx, e) }
