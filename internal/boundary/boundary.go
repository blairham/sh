// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package boundary asks a shell's gate about what a *front end* does itself.
//
// The boundary is drawn around execution, and interp holds it for everything a
// script does. But a shell opens two kinds of file before and after any script
// runs — the program it was invoked with, the startup files it sources, the
// history it keeps — and those went through the os package directly. They were
// latent while nothing supplied a gate; they stopped being latent when
// driver.Shell grew Gate and Events, because a policy that refuses a path can
// now be handed to a binary that reads that path without asking.
//
// The rule for what belongs here is the one on interp's action vocabulary: an
// access is inside the boundary when the *path was chosen by whoever the
// policy is about*. A script operand, $ENV, HISTFILE — all of them come from
// the invocation or from the shell's own variables, which a line of script can
// set. The front end's own plumbing on fixed paths is outside it, and each
// such exemption is written down rather than left to be inferred; docs/design.md
// carries the list.
//
// The vocabulary is interp's and stays interp's. This package only asks: it
// does not name new kinds, does not decide what a refusal means, and does
// nothing at all when a shell has neither a gate nor a sink, which is the
// common case and costs two nil checks.
package boundary

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/internal/opened"
	"github.com/blairham/sh/interp"
)

// Boundary is a shell's gate and event sink, as a pair, for the front end's
// own accesses. The zero value allows everything and records nothing, which
// is what a shell without a policy is.
type Boundary struct {
	Gate   interp.Gate
	Events interp.Sink
	// Session identifies the run these accesses belong to, and is the same
	// string the Runner carries — the front end hands one value to both.
	//
	// It matters here more than it looks. What this package opens is a shell's
	// *own* files, and one of them is the store that records what the shell
	// ran: without the session on these records, the audit stream's account of
	// the block store being written could not be joined to the blocks in it.
	Session string
}

// ErrRefused is what a refused open comes back as, and it is the whole of what
// this package says about one.
//
// What a denied open *means* stays the caller's, because the callers mean
// different things by it: a script the shell cannot read is a failure it names
// and exits over, a history file it cannot read is a session that starts
// empty, and a file an agent asked for is answered on the wire as a path that
// is not there. Each of them tests for this and says its own sentence.
//
// A refusal from the *second* consultation — the name reached an object the
// gate will not have — is this same error, deliberately. The two are
// indistinguishable to whoever asked, which is the property the diagnostic
// split in internal/opened exists to keep: a caller that could tell "the name
// is denied" from "the name reached a denied object" has been told where the
// name went.
var ErrRefused = errors.New("refused")

// File is one open a front end wants to make.
//
// Flags and Perm are os.OpenFile's, so the zero value is a read of a file that
// must already exist, which is what most of these are.
type File struct {
	Path  string
	Flags int
	Perm  fs.FileMode

	// Parents makes the directory the file goes in, at 0700, when the open may
	// create the file.
	//
	// It happens *after* the gate has allowed the path and before the open, so
	// a policy that refuses the write does not get a directory tree made for
	// it under a denied path — which is what reordering it to the caller would
	// have cost, since the caller has to run before it can call this. 0700
	// because what goes in these is a record of what somebody typed and what
	// it printed; the history file already sets that bar.
	//
	// Only the three stores the shell keeps for itself want this. An agent
	// writing a file over ACP deliberately does not: `os.WriteFile` to a
	// directory that is not there is an error, and quietly creating the tree
	// would be this package inventing a capability the protocol never gave it.
	Parents bool
}

// write reports whether this open is one a policy should see as a write.
//
// Derived from the flags rather than declared beside them, because a field
// that can disagree with the flags eventually does, and the disagreement that
// matters is the one where the flags write and the field says read.
func (f File) write() bool { return f.Flags&(os.O_WRONLY|os.O_RDWR) != 0 }

// OpenFile opens a path on the front end's behalf: consult the gate, open,
// ask the kernel what the open reached, and consult again if that is somewhere
// else.
//
// This package used to answer a bool and leave the open to the caller, and
// that was the hole #942 names. Nine call sites each did their own os.Open,
// os.ReadFile or os.WriteFile afterwards, so the gate decided about the *name*
// and nothing ever looked at the object — the exact defect #703 closed for
// everything a script does, still open for everything the shell does for
// itself, with HISTFILE the one a script can aim. A shell that verifies a
// script's opens and not its own is a gate with a door beside it.
//
// So the descriptor is made here rather than by the caller. Not because that
// is tidier: because it is the only arrangement in which the check cannot be
// forgotten. There is no longer a way to ask this package's permission and
// then open something else.
//
// A shell with no gate takes the plain os.OpenFile, flags included, so a run
// nobody is watching makes exactly the calls it always made.
func (b Boundary) OpenFile(ctx context.Context, f File) (*os.File, error) {
	a := interp.Action{ID: b.id(), Kind: interp.ActionOpen, Path: f.Path, Write: f.write()}
	if b.Gate != nil && b.Gate.Allow(ctx, a) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: a})
		return nil, ErrRefused
	}
	if f.Parents {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
			return nil, err
		}
	}
	file, err := b.open(ctx, &a, f)
	if errors.Is(err, ErrRefused) {
		// The verification refused, and recorded it with both names on it.
		return nil, err
	}
	// The auditable act out here is the *attempt*, which is why this is
	// recorded whether or not the open worked: a startup file that is not
	// there is the normal case and is not an error. It is recorded after the
	// attempt rather than before it so that it can say where the name went —
	// which a record written before there was a descriptor could not. See
	// interp.Action.Resolved.
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
	return file, err
}

// open is OpenFile's syscall half, once the gate has allowed the name.
func (b Boundary) open(ctx context.Context, a *interp.Action, f File) (*os.File, error) {
	if b.Gate == nil {
		// Watching without gating: there is nothing a verification could
		// refuse, so the standard library's own call stands.
		return os.OpenFile(f.Path, f.Flags, f.Perm)
	}
	return opened.Verified(f.Path, f.Flags, f.Perm, func(file *os.File) error {
		if !b.reached(ctx, a, file) {
			return ErrRefused
		}
		return nil
	})
}

// ReadFile is os.ReadFile through the gate, verified.
func (b Boundary) ReadFile(ctx context.Context, path string) ([]byte, error) {
	f, err := b.OpenFile(ctx, File{Path: path})
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// WriteFile is os.WriteFile through the gate, verified.
//
// The flags are os.WriteFile's own, O_TRUNC included, and the truncation is
// the reason this cannot be os.WriteFile after a bool: the kernel empties the
// file as part of the open, so a write refused because the name reached a
// denied object would already have destroyed it. internal/opened holds the
// flag back until the verification has passed.
func (b Boundary) WriteFile(ctx context.Context, f File, data []byte) error {
	f.Flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	file, err := b.OpenFile(ctx, f)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// reached is the second consultation: the gate asked again, about the kernel's
// name for what the open actually got.
//
// The action carries the same ID as the one already asked about, because it is
// the same access — a consumer joining the two records sees one open whose
// name resolved elsewhere, not two opens. It is the same promise interp keeps
// on its side of the boundary, made here in the same words on purpose.
func (b Boundary) reached(ctx context.Context, a *interp.Action, f *os.File) bool {
	actual, elsewhere := opened.Elsewhere(f, a.Path)
	if !elsewhere {
		return true
	}
	// The action gains where it went and keeps the name it was raised about,
	// so the caller's record says both. interp keeps the same promise on its
	// side of the boundary, in the same words.
	a.Resolved = actual
	// The gate is asked about the object and about nothing else, so a rule
	// matches what was reached even from a Gate that has never heard of the
	// field. See interp.Action.Resolved.
	asked := *a
	asked.Path = actual
	if b.Gate.Allow(ctx, asked) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: *a})
		return false
	}
	return true
}

// ReadDir lists a directory on the front end's behalf: consult the gate, open,
// ask the kernel what the open reached, consult again if that is somewhere
// else, and take the entries from that descriptor.
//
// It is here because completion reads directories, and the path it reads is
// one a *person typed at the prompt*. This package's rule for what is inside
// the boundary is who chose the path, and there is no reading of that rule on
// which a script's `echo /srv/*` is inside and a person's `/srv/<Tab>` is
// outside — the person is the policy's subject at least as much as the script
// is. The interpreter's equivalent has been gated since the beginning as
// ActionReadDir, so before this the same policy answered the same question two
// different ways depending on which half of the shell was asked.
//
// The entries are returned rather than the descriptor, which is the same
// property OpenFile has and is the point of both: there is no way to ask this
// package's permission and then list something else.
//
// A refusal is ErrRefused, and what it means is the caller's as it is for an
// open. Completion answers it the way it answers a directory that is not
// there — no entries, no diagnostic — which is what ActionReadDir already
// documents a denied listing as, and is the same quiet shape a denied stat
// has. Tab offering nothing is what a person sees, and a person who may not
// list a directory should see exactly that whether the policy hid it or the
// operating system did.
//
// The listing is recorded, always, and is not coarsened or suppressed. What
// that costs was measured rather than assumed: a Tab in the file position
// makes one record, a Tab in the command position makes one per PATH entry,
// and a single ordinary command line already makes more than that — a PATH
// search stats a candidate per directory, so `ls | grep x` on this machine
// raises 21 actions before either command starts. Completion runs on Tab and
// on nothing else, not on every keystroke, so a long prefix costs nothing
// until it is asked to complete. A person exploring the filesystem from the
// prompt is precisely what an audit trail is for, and it would be a strange
// stream that held every directory a glob descended into and none of the ones
// a person looked at by hand.
//
// A gate that *prompts* is the objection worth taking seriously, and it does
// not arise: Escalates in internal/acp — the only gate in this tree that asks
// a person anything — already answers false for every read, ActionReadDir
// included, and says why in the same words. An embedder's gate that chose to
// prompt on a listing would already be prompting once per directory of every
// glob a script writes, so Tab is not a new hazard for it; it is the same
// action kind it has always been consulted about.
func (b Boundary) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if b.Gate == nil && b.Events == nil {
		// Nothing is watching, so this is the call it always was.
		return os.ReadDir(path)
	}
	a := interp.Action{ID: b.id(), Kind: interp.ActionReadDir, Path: path}
	if b.Gate != nil && b.Gate.Allow(ctx, a) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: a})
		return nil, ErrRefused
	}
	if b.Gate == nil {
		// Watching without gating: there is nothing a verification could
		// refuse, so the standard library's one-call form stands.
		b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
		return os.ReadDir(path)
	}
	f, err := opened.Verified(path, os.O_RDONLY, 0, func(file *os.File) error {
		if !b.reached(ctx, &a, file) {
			return ErrRefused
		}
		return nil
	})
	if errors.Is(err, ErrRefused) {
		// The verification refused, and recorded it with both names on it.
		return nil, err
	}
	// Recorded after the open for the reason OpenFile is: a record written
	// before there is a descriptor cannot say where the name went. And
	// recorded whether or not the open worked, because out here the attempt is
	// the auditable act — a directory a person half-typed is usually not
	// there, and that is not an error.
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(-1)
	// Sorted, because os.ReadDir sorts and both callers were written against
	// that: completion offers its matches in name order and would otherwise
	// offer them in whatever order the directory happens to be stored in.
	slices.SortFunc(entries, func(x, y os.DirEntry) int { return strings.Compare(x.Name(), y.Name()) })
	return entries, err
}

// Exec reports whether the front end may run a program, recording it either
// way. Argv is the whole vector, argv[0] included, as interp builds one.
//
// The caller is an ACP client: an agent asking us to run a command, which is
// the one arrangement where a program a policy is about is named by somebody
// outside this process. It is squarely inside the boundary by this package's
// own rule — the program was chosen by whoever the policy is about — and it is
// the reason the rule is worth stating as a rule rather than as a list of the
// files a shell opens.
func (b Boundary) Exec(ctx context.Context, path string, argv []string) bool {
	return b.ask(ctx, interp.Action{ID: b.id(), Kind: interp.ActionExec, Path: path, Args: argv})
}

// Signal reports whether the front end may signal a process, recording it
// either way. The pid is as kill(2) takes it, so a negative value names a
// process group.
func (b Boundary) Signal(ctx context.Context, pid int, sig syscall.Signal) bool {
	return b.ask(ctx, interp.Action{ID: b.id(), Kind: interp.ActionSignal, PID: pid, Signal: sig})
}

// Record notes an access the front end made without asking about it.
//
// It is deliberately narrow, and the rule for reaching for it is this
// package's own: an act belongs here rather than at the gate when the *front
// end* chose it rather than whoever the policy is about. The worked example is
// an ACP client releasing a terminal — the protocol's only way for an agent to
// say it is finished, whose kill is the client ending something the client
// started. Everything the policy subject chose goes through the three above,
// which ask and record together.
//
// It stamps the id and the session for the same reason those do: a record that
// names no action and no run is a record nothing can be joined to, which is
// exactly what this field was added to fix.
func (b Boundary) Record(ctx context.Context, a interp.Action) {
	a.ID = b.id()
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
}

// ask is the whole of Exec and Signal: consult, record, answer.
//
// An open does not come through here, and the difference is one line: it has
// something to add to the record afterwards — where the name went — so its
// record is written once the descriptor is in hand. Both still record the
// *attempt*, which is the rule out here and is not interp's: interp records a
// successful open because a failed one already becomes an EventError with a
// reason, while a startup file that is not there is the normal case and not an
// error at all.
func (b Boundary) ask(ctx context.Context, a interp.Action) bool {
	if b.Gate != nil && b.Gate.Allow(ctx, a) == interp.Deny {
		b.emit(ctx, interp.Event{Kind: interp.EventDenied, Action: a})
		return false
	}
	b.emit(ctx, interp.Event{Kind: interp.EventAccess, Action: a})
	return true
}

func (b Boundary) emit(ctx context.Context, e interp.Event) {
	if b.Events == nil {
		return
	}
	// The session is put on here for the reason interp puts it on in emit():
	// one place, so no call site can produce a record that belongs to nothing.
	e.Session = b.Session
	b.Events.Emit(ctx, e)
}

// id names one front-end access, so the consultation and the record of it are
// provably the same access — the promise interp.Action.ID makes, kept on this
// side of the boundary too.
//
// Minted rather than counted, which is the opposite of interp's choice and for
// the opposite reason. There a counter is forced by the hot path: a PATH search
// stats a candidate per directory. Out here a run makes a handful of these — a
// script, a startup file or two, a history file, a block store — so there is
// nothing to economize, and an id that needs no coordination is what lets the
// several places that build a Boundary go on building one independently. A
// shared counter would have to be threaded through every one of them, and the
// first that forgot would issue a duplicate.
//
// Nothing is minted when nothing is watching, which is the same nil check the
// rest of this package costs a shell without a policy.
func (b Boundary) id() string {
	if b.Gate == nil && b.Events == nil {
		return ""
	}
	return event.NewID(time.Now())
}
