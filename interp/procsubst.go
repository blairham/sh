// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/blairham/sh/syntax"
)

// Process substitution: a command run with one end of a pipe, expanding to a
// path the other end can be opened by.
//
// It is the last of the core language — docs/spec/core.md has named it since
// before there was code to refuse it — and the reason it waited is that it is
// plumbing rather than grammar. A word that expands to text needs nothing from
// the process; a word that expands to a path is a promise that something can
// open that path, and keeping it reaches out of expansion entirely.
//
// The inner command runs concurrently, and that is not an optimization. Its
// output goes into a pipe whose buffer is finite, so running it to completion
// first would deadlock on anything longer than that buffer — which is most
// things worth substituting.
//
// A named pipe rather than /dev/fd, which was tried first and is worth
// recording. /dev/fd needs the descriptor to survive into the command that
// opens the path, and Go marks everything it opens close-on-exec — so it has
// to be cleared, and clearing it leaks the descriptor into *every* command the
// shell runs afterwards. That is not a tidiness problem: a later command
// holding the write end open means the substitution never sees end-of-file, so
//
//	echo x | tee >(tr a-z A-Z); sleep 0.4
//
// produced nothing at all, because `sleep` was holding the pipe. Clearing the
// flag only around the right fork is what a shell in C does; Go's os/exec
// takes the fork lock itself, so there is no window a caller can hold.
//
// A FIFO has none of that. It is a real path any process can open, inherits
// nothing, and leaks nothing — at the cost of a file to make and remove.
//
// # The third spelling, which is a file
//
// `=(cmd)` is the same construct with a *regular file* where the other two
// have a pipe, and everything that differs follows from that one change. The
// command runs to completion first, because there is no reader to deadlock
// against and nothing to overlap with: measured 2026-09-11 on zsh 5.9.2,
// `wc -c < =(head -c 200000 /dev/zero)` answers 200000 where the same
// through a pipe would fill it. What the word names is then seekable and
// reopenable, which is the whole reason to have it — `diff =(a) =(b)` seeks,
// `vi =(cmd)` opens — and is why a pipe will not do.
//
// The body's status is discarded: `cat =(false)` and `cat =(exit 7)` are
// both 0 there, and `=(nosuchcmd)` prints the diagnostic and still hands over
// the path of an empty file. So is `=()` itself, which is a valid file of
// zero bytes.
//
// The lifetime is the pipe's lifetime exactly — removeProcSubs, at the end
// of the command that named it. `f==(echo hi); cat $f` is `No such file or
// directory` there, so the file is recorded in the same list the pipes are
// and needs no second mechanism.

// procSub runs the inner command and returns the path the word becomes: a
// pipe for `<(cmd)` and `>(cmd)`, a file already holding the output for
// `=(cmd)`.
func (r *Runner) procSub(ctx context.Context, kind syntax.SpanKind, src string) (string, bool) {
	f, ok := r.substBody(src)
	if !ok {
		return "", false
	}
	if kind == syntax.ProcSubstFile {
		return r.procSubToFile(ctx, f)
	}
	path, err := r.newFifo()
	if err != nil {
		r.diagf("%v\n", err)
		r.expandErr = true
		return "", false
	}
	// The shell's own end of the pipe is an open and is recorded as one. It
	// is not put to the gate, and that is the recognition #941 asked for:
	// this path is the shell's own scaffolding, on the same side of the line
	// as the temporary directory and the mkfifo that made it. See ownPipe,
	// which carries the argument and which the two other places an open of
	// this path can happen consult.
	action := r.act(Action{Kind: ActionOpen, Path: path, Write: kind != syntax.ProcSubstOut})

	sub := r.substRunner(kind)
	if kind == syntax.ProcSubstOut {
		sub.Stdout = r.Stdout
	}

	// The end this shell keeps is counted rather than closed on the body's
	// return, and the count starts at one for the body itself. What else can
	// join it, and why the body is not always the last, is in substEnd.
	keep := &substEnd{held: 1, anchor: newProcAnchor(r.ProcessAnchor)}
	if kind != syntax.ProcSubstOut {
		// Only the writing end delivers an end-of-file by closing, so only
		// that direction has a nudge to repeat. See nudgeFifoEOF.
		keep.nudge = path
	}
	sub.pipeEnd = keep

	// `>(cmd)` reads the command's input out of the pipe, and the shell can
	// take that end without waiting for anybody — so it is taken here, on
	// this goroutine, and a failure is the shell's own and is reported like
	// one. `hold` is what makes waiting unnecessary; see openFifoReadEnd.
	var hold *os.File
	if kind == syntax.ProcSubstOut {
		var end *os.File
		var oerr error
		end, hold, oerr = openFifoReadEnd(path)
		if oerr != nil {
			_ = os.Remove(path)
			r.diagf("%v\n", oerr)
			r.expandErr = true
			return "", false
		}
		sub.Stdin = end
		keep.opened(end)
		r.spawn(func() {
			// Through the clone, which this goroutine owns: the record of
			// the open that the gate already allowed.
			sub.emit(ctx, Event{Kind: EventAccess, Action: action})
			if _, err := sub.Run(ctx, f); err != nil {
				sub.diagf("%v\n", err)
			}
		}, func() {
			// However the goroutine ended: this end closing is the
			// end-of-file the substituted command's reader is waiting for,
			// and skipping it would leave the command that named the path
			// waiting for one that is never coming.
			//
			// Through the count rather than straight at the descriptor,
			// because the body is not always the last to want it: a job the
			// body backgrounded reads this same end and outlives the return.
			// See substEnd.
			keep.letGo()
		})
	} else {
		// What this direction's body *reads* was chosen in substRunner,
		// where all three spellings are prepared — see substStdin for which
		// stream that is and why. What is left here is the writing.
		//
		// `<(cmd)` writes cmd's output into the pipe, so this end is the
		// writer — and a writer has to wait for its reader, which is why
		// this half is on a goroutine and the other half is not. The wait
		// is bounded now rather than endless; openFifoWriteEnd is where
		// that is done and why.
		r.spawn(func() {
			end, err := openFifoWriteEnd(path)
			if err != nil {
				// Nobody opened the other end — the command did not use the
				// path it was given, and the pipe went with it. There is
				// nothing to run and nothing to report: `echo <(true)`
				// prints a path and is not an error anywhere.
				return
			}
			sub.emit(ctx, Event{Kind: EventAccess, Action: action})
			sub.Stdout = end
			keep.opened(end)
			if _, err := sub.Run(ctx, f); err != nil {
				sub.diagf("%v\n", err)
			}
		}, func() {
			// The same close as the other direction, and the same reason:
			// it is the end-of-file the command that named the path is
			// reading until — and the same count in front of it, because a
			// job the body backgrounded writes through this end after the
			// body has returned. Nothing happens when nobody ever opened the
			// other end, so there was nothing to close.
			//
			// The nudge is inside letGo for the same reason the close is:
			// repeating a last-writer close while a writer is still there
			// delivers nothing, and would spin until the pipe was taken
			// away. See substEnd.
			keep.letGo()
		})
	}

	r.procSubs = append(r.procSubs, procSubPipe{path: path, hold: hold})
	return path, true
}

// substBody parses a substitution's inner source.
//
// One reader for all three spellings, because the body is a program in each
// of them and a parse failure is reported the same way: the word produces
// nothing and the expansion is in error.
func (r *Runner) substBody(src string) (*syntax.File, bool) {
	f, perr := syntax.Parse(src, r.dialect())
	if perr != nil {
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		r.expandErr = true
		return nil, false
	}
	return f, true
}

// substRunner is the shell a substitution's body runs in, prepared the same
// way for every spelling of the construct.
//
// **One helper rather than one per spelling**, and that is the whole reason
// it exists: each line below was a bug once, and a second copy of this
// preparation is a second place for the next fix to miss. The terminal hook
// is the sharpest of them — #1830 is three weeks old — and a file-writing
// body that had been given its own clone would have taken the terminal again
// with nothing to say so.
func (r *Runner) substRunner(kind syntax.SpanKind) *Runner {
	sub := r.clone()
	sub.inheritJobs(jobBoundarySubstitution)
	// **Which input the body reads is one question, asked once.** `<(cmd)`
	// and `=(cmd)` keep what this chooses; `>(cmd)` replaces it in procSub
	// with the reading end of its own pipe, which is what that spelling *is*
	// and is why it cannot observe the axis. Deciding it here rather than in
	// each branch is the point of this helper — #1933 was filed before the
	// file form landed precisely so the two could not be fixed apart.
	if kind != syntax.ProcSubstOut {
		sub.Stdin = r.substStdin()
	}
	// And the record itself does not cross: the body is a shell of its own,
	// whose Stdin is now whatever it is going to read, so nothing inside it
	// is still waiting for a pipe to be installed.
	sub.shellStdin = nil
	// **A substitution's body never takes the terminal.** The shell hands the
	// terminal to a command it is *waiting for*, so that ^C and ^Z reach the
	// command rather than the shell — see runWatched. A substitution's body
	// is not one: it runs beside the command that named it, on a goroutine,
	// and the shell carries on. Leaving the hook in place made it one anyway,
	// and the body then held the terminal for as long as its command lived.
	//
	// What that cost was a session. `exec {fd}< <(sleep 60)` in a startup
	// file put `sleep`'s process group in front, and the shell's own group
	// was a background one from that moment: a shell reading its terminal
	// from a background group is sent SIGTTIN, which a shell ignores, and an
	// ignored SIGTTIN turns the read into EIO. So the first read after the
	// prompt failed with `read /dev/stdin: input/output error` and the
	// session ended — measured through a pseudo-terminal with the shell in a
	// session of its own, where real zsh 5.9.2 answers every line typed. It
	// reproduces with the substitution alone; nothing else in the startup
	// file is needed, which is why #1759 saw it from a watcher that never
	// fired. A body whose command is quick — `<(echo hi)` — hid it, because
	// the terminal came back before the prompt was read.
	//
	// nil rather than a flag on the clone, because the hook *is* the
	// question: a Runner with nowhere to send the terminal is a Runner that
	// does not send it, which is already what a script and a pipeline get.
	// A background job reaches the same place by a different road — `r.bg`
	// takes it down a branch that never asks — and a coprocess with it.
	sub.Foreground = nil
	// The one boundary no shell's `trap` sees across: even the dialect that
	// keeps the parent's listing everywhere else lists nothing in
	// `<(trap)` — measured, `cat <(trap)` prints nothing in all three
	// shells that have the construct.
	sub.trapsModified()
	// A substitution runs beside the command that names it, so it shares the
	// caller's streams the way a background job does — and needs the same
	// guard for the same reason: an io.Writer carries no promise of being
	// safe to write from two places, and the shell is what created the
	// concurrency.
	//
	// Both sides, which is background()'s rule and is here for the reason it
	// gives: a lock one party takes and the other does not excludes nothing,
	// and the shell that named the substitution is the other party. It left
	// the shell writing raw — and a *child* of the shell too, because os/exec
	// copies into a writer that is not a file on a goroutine of its own, with
	// no share of this lock. `cat <(cmd)` raced on exactly that, and
	// bytes.Buffer grows before it reads, so a child that writes nothing at
	// all raced as well. That was #735.
	//
	// It costs nothing to wrap what is already a pipe: lockWriter leaves an
	// *os.File alone, so a shell whose streams are files hands its children
	// the descriptors, and one whose streams are not was giving them pipes
	// either way.
	//
	// Both *streams*, not only the shared one, and that is the same argument
	// again: an embedder may hand one writer to Stdout and Stderr both, so
	// the substitution's diagnostic and the outer command's output are the
	// same object. Guarding only stderr left `cat <(cmd)` racing on the
	// stdout the shell had not wrapped — one mutex covers both streams for
	// exactly this reason; see lockedWriter.
	//
	// The file spelling shares the guard although it runs the body to
	// completion and races with nobody: what it costs is a wrapper the
	// *os.File case does not even take, and a stream rule that held for two
	// of three spellings would be read as an accident by whoever arrives
	// next.
	sub.Stderr = r.lockedStderr()
	r.Stderr, r.Stdout = sub.Stderr, r.lockedStdout()
	return sub
}

// substStdin is the stream a substitution's body reads, guarded.
//
// Two questions in one place. **Which stream**: the input of the command the
// word stands in, or the input of the *shell*. They are the same stream
// everywhere but one — inside a pipeline element, whose input is the pipe —
// which is why `cat <(cat)` cannot tell them apart and
// `printf "PIPE\n" | cat <(cat)` can. The panel splits there, so it is an
// axis and not a correction; see
// Semantics.ProcessSubstitutionBodyReadsTheShellsInput for the measurements
// and for what bounds it.
//
// Asked only where the two differ, which is what shellStdin being nil says.
// An axis asked on the common path is an axis every script pays for and that
// no dialect can leave unanswered — and here it would refuse `cat <(cat)` in
// a Runner built without a preset, for a disagreement that snippet is not in.
//
// **Guarded**: the shell and the command it named may read one stream at the
// same time, which is the third stream needing what the other two have — and
// it needs it in both directions, because a lock one party takes and the
// other does not excludes nothing. os/exec copies from a reader that is not a
// file on a goroutine of its own, so `cat <(exec /bin/echo sub)` with a
// caller-supplied reader had two of those copying out of one io.Reader, which
// the race detector reports inside strings.Reader.
//
// The guard is written back where it came from, so that the shell's own reads
// take it too and a second substitution in the same command finds it already
// there rather than wrapping it again — `cat <(cat) <(cat)` is what a second
// wrapper over one mutex would stop on, since a sync.Mutex is not reentrant.
func (r *Runner) substStdin() io.Reader {
	if r.shellStdin != nil && r.ask(r.sem().ProcessSubstitutionBodyReadsTheShellsInput,
		"a process substitution's body reading the shell's input rather than the command's") {
		r.shellStdin = r.lockReader(r.shellStdin)
		return r.shellStdin
	}
	r.Stdin = r.lockedStdin()
	return r.Stdin
}

// procSubToFile runs the body to completion with its output in a regular
// file, and returns that file's path.
//
// No goroutine and no waiting on anybody, which is the difference the
// construct exists for: the reader opens a finished file rather than one end
// of a pipe, so it may seek in it, reopen it, and read it more than once. The
// deadlock the pipe forms cannot arise — measured 2026-09-11, `wc -c < =(head
// -c 200000 /dev/zero)` is 200000 in zsh 5.9.2 — because nothing is waiting
// for the shell while the shell waits for the body.
//
// The body's status is dropped on the floor, which is measured rather than an
// omission: `cat =(false)`, `cat =(exit 7)` and `echo =(nosuchcmd-xyz)` are
// all status 0 there, the last one after printing its diagnostic. A word
// expands to a path or it fails to expand at all, and a command that ran and
// failed still wrote the file it was given.
func (r *Runner) procSubToFile(ctx context.Context, body *syntax.File) (string, bool) {
	path, f, err := r.newSubstFile()
	if err != nil {
		r.diagf("%v\n", err)
		r.expandErr = true
		return "", false
	}
	// Recorded before anything is written to it, so that the file is removed
	// at the end of the command however the body ends — and so that ownPipe
	// recognizes the path while the command that named it runs. It is the
	// interpreter's own scaffolding on the same argument the pipe is; see
	// ownPipe.
	r.procSubs = append(r.procSubs, procSubPipe{path: path})
	action := r.act(Action{Kind: ActionOpen, Path: path, Write: true})

	sub := r.substRunner(syntax.ProcSubstFile)
	sub.Stdout = f
	sub.emit(ctx, Event{Kind: EventAccess, Action: action})
	if _, err := sub.Run(ctx, body); err != nil {
		sub.diagf("%v\n", err)
	}
	// Closed before the path is handed over rather than left to the command
	// that named it: what makes this spelling usable is that the file is
	// *finished*, and a reader that opened it while the shell still held a
	// write end could see a short one.
	_ = f.Close()
	return path, true
}

// newSubstFile makes the regular file a `=(cmd)` writes into.
//
// In the directory the pipes go in, numbered from the same counter and for
// the same reasons — one directory per shell tree, removed by CleanUp,
// distinct names needed only within it. Sharing it is what keeps the file on
// the same side of the boundary the pipe is on: a script writes `=(cmd)` and
// can no more write this path than it can write a pipe's.
//
// 0600 and O_EXCL. The mode is measured — `ls -l =(echo hi)` reports
// `-rw-------` in zsh 5.9.2, which is also why naming one as a command is
// `permission denied` there rather than running it — and the exclusion is
// what makes a name this shell has not used before an error instead of a
// silent overwrite.
func (r *Runner) newSubstFile() (string, *os.File, error) {
	dir, err := r.procSubDir()
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "file"+strconv.FormatUint(r.procSubHomeBox().seq.Add(1), 10))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, err
	}
	return path, f, nil
}

// procSubDirPrefix names the directory a shell puts its substitution pipes in.
//
// A constant rather than a literal at the one place that makes the directory,
// because a second reader depends on it: the guard that fails a test run
// leaving a process holding one of these pipes finds it by this name in the
// command line, and a prefix that changed in one place and not the other would
// leave the guard quietly finding nothing. See internal/childguard.
const procSubDirPrefix = "sh-procsub"

// ownPipe reports whether path names one of the pipes — or one of the files —
// this shell made for a process substitution in the command it is running.
//
// It is the recognition #941 asked for, and the rule it enforces is
// ActionOpen's own, quoted here because the pipe is the one thing the rule
// reached and stopped one line short of:
//
//	The scaffolding a process substitution stands on — the temporary
//	directory made for its pipes, the mkfifo that creates one, their removal
//	— is deliberately outside the boundary: those paths are chosen by the
//	interpreter, never by the script, and gating them would let a policy
//	refuse the mechanism while believing it refused an access.
//
// The pipe is chosen by the interpreter too. A script writes `<(cmd)` and
// never writes `<TMPDIR>/sh-procsubNNNNNNNN/sub1`; it cannot, because the
// directory is made per shell with a name the operating system picks and the
// pipe is numbered inside it. So a policy that refuses this open refuses
// `<(cmd)` itself, and there is nothing in the policy file that says so — an
// operator reads `default deny write` and gets a shell whose process
// substitutions have stopped working, with a diagnostic naming a path they
// have never seen.
//
// `=(cmd)`'s file is the same class and is in the same set. It is made in the
// same directory, numbered by the same counter, removed by the same
// removeProcSubs at the end of the command that named it — so every sentence
// above is true of it word for word, and the only thing that differs is
// whether the path leads to a pipe or to a finished file. Two sets would have
// been two answers to one question. Measured: that is 6 of the 1426 corpus cases under the
// containment posture, and every case in `make conformance-gated` that
// differed in more than the wording of a diagnostic.
//
// # What is not exempted, which is everything worth refusing
//
// The inner command is an ActionExec the gate is asked about, in the ordinary
// way and before it runs. It executes in a Runner of its own whose every
// access passes the gate, so `<(cat /etc/secret)` is refused at the read,
// where the rule about /etc can see it and name it. What crosses this pipe is
// that command's output and nothing else — no path is reached through it that
// was not reached, and recorded, on the other side.
//
// Nor does this stop being recorded. The open still reaches the event stream
// as an EventAccess naming the pipe, so an audit trail says the shell made
// one and when. This is the shape ActionInherit already has and is documented
// for: recorded always, asked never, because a veto there would be a promise
// the boundary cannot keep.
//
// # Why the set is the command's and not the shell's
//
// A pipe is in it from the moment the word expands until removeProcSubs takes
// it away, at the end of the command that named it — which is what keeps this
// from being an exemption a script could aim at. `p=$(echo <(true))` prints a
// path and then lets its command end, so the name it captured is in nothing
// and reaches nothing; `> "$p"` afterwards is an ordinary open of an ordinary
// path and the gate is asked about it. And clone() empties the set, so a
// subshell does not inherit its parent's exemptions.
//
// Probes are deliberately not included. `[ -f <(cmd) ]` is a stat, refused
// quietly and answered the way a path that is not there is answered, so a
// policy hiding the temporary directory makes it false rather than making the
// construct fail — a wrong answer to a question nobody asks rather than a
// mechanism that stopped working. Widening the suppression to the loudest
// oracle the filesystem has, for that, is a trade this does not make.
func (r *Runner) ownPipe(path string) bool {
	for _, p := range r.procSubs {
		if p.path == path {
			return true
		}
	}
	return false
}

// newFifo makes a named pipe in this shell's own directory.
//
// One directory per shell, made when the first substitution needs it, so a
// shell that never uses one leaves nothing behind. The names are numbered
// rather than random: they only have to be distinct within a directory nothing
// else writes to.
func (r *Runner) newFifo() (string, error) {
	dir, err := r.procSubDir()
	if err != nil {
		return "", err
	}
	// From the box rather than from this Runner, because the directory is
	// the box's and every shell in the tree makes its pipes in it. A counter
	// per Runner numbered from whatever the clone happened to copy, so the
	// two halves of `cat <(echo a) | ( cat <(echo b) )` — clones taken from
	// the same parent, running at the same time — both asked for `sub1` in
	// the one directory and the second mkfifo said the file exists.
	//
	// Atomic for the same reason: those two are goroutines.
	path := filepath.Join(dir, "sub"+strconv.FormatUint(r.procSubHomeBox().seq.Add(1), 10))
	if err := mkfifo(path); err != nil {
		return "", err
	}
	return path, nil
}

// procSubDirs is the directory each shell makes for its named pipes, and the
// lock that keeps two goroutines from making two.
//
// On the Runner rather than a package variable, because a Runner is a shell
// and two of them in one program each want their own — the same reason the
// working directory is a field.
type procSubDirs struct {
	once sync.Once
	dir  string
	err  error
	// seq numbers the pipes inside dir. Here and not on the Runner because
	// the directory is here: distinct names are only needed within one
	// directory, and one directory is what a whole shell tree has.
	seq atomic.Uint64
}

func (r *Runner) procSubDir() (string, error) {
	home := r.procSubHomeBox()
	// Read out here rather than inside the closure: this runs on the
	// goroutine that is expanding the word, which is where every other read
	// of the variable table happens, and once.Do would otherwise be the one
	// place a second goroutine reads r.Vars.
	parent := r.tempHome()
	home.once.Do(func() {
		// An explicit parent, never MkdirTemp's empty one. Empty means
		// os.TempDir, which is os.Getenv("TMPDIR") wearing a different name
		// — the process's environment, read from inside the library, on the
		// live path of every substitution. It is the os.Getwd fallback the
		// glob path used to have, in a second place.
		//nolint:forbidigo // the parent is the Runner's, computed above; only MkdirTemp's empty-string form asks the process
		home.dir, home.err = os.MkdirTemp(parent, procSubDirPrefix)
	})
	return home.dir, home.err
}

// procSubHomeBox is the box the pipe directory goes in, made if this shell has
// not needed one yet.
//
// One box per shell *tree*, not per Runner, which is why clone asks for it
// too. A subshell copies the pointer, so parent and child name the same
// directory and the second one to want a pipe does not make a second
// directory — that is the property the once is there for.
//
// A clone taken before the box exists is where that broke. `( cat <(echo hi) )`
// cloned a nil pointer, the subshell then made a box of its own, and the
// parent finished knowing nothing about the directory in it: the one route
// that still leaked after Finish learned to clean up, because the shell doing
// the cleaning had never heard of what was left. Making the box on the way
// into clone is what keeps the two halves talking.
func (r *Runner) procSubHomeBox() *procSubDirs {
	if r.procSubHome == nil {
		r.procSubHome = &procSubDirs{}
	}
	return r.procSubHome
}

// tempHome is where this shell puts what it has to write to disk.
//
// TMPDIR through r.getVar, which is the Runner's own answer: its variables
// first and the environment its embedder handed in second. That is the same
// route `~` takes to HOME, and it is the only one that keeps two Runners in
// one program separable — an embedder that seeds Env decides where its
// shell's pipes go, and a script that assigns TMPDIR moves its own and
// nobody else's.
//
// No shell in the panel exposes this: measured on darwin, bash, zsh and
// ksh93 all expand `<(cmd)` to a /dev/fd path and none of them consults
// TMPDIR for it, while zsh's `=(cmd)` — the one construct that does write a
// file — reads TMPPREFIX and ignores TMPDIR too. So there is no behavior to
// match here and no axis to add. The named pipe is ours, forced by Go's
// close-on-exec (see the file comment), and where it lives is therefore our
// decision rather than a compatibility question. What settles it is the
// library rule: the answer belongs to the Runner.
//
// That reading survived implementing `=(cmd)` rather than being overtaken by
// it. TMPPREFIX is a *process* variable a shell reads once at startup and
// this package may not read one at all, and a Runner an embedder gave a
// TMPDIR to is already saying where its scratch goes — so the file joins the
// pipes under r.tempHome() and the spelling of the path stays ours. What is
// matched is what a script can observe about the file: that it is regular,
// that it is 0600, and that it is gone when the command that named it ends.
//
// A relative TMPDIR is resolved against r.Dir rather than left for the
// operating system to resolve, because the directory the *process* happens
// to sit in is exactly the ambient state this is here to stop reading.
func (r *Runner) tempHome() string {
	dir, _ := r.getVar("TMPDIR")
	if dir == "" {
		// POSIX names /tmp as the directory that is there. A Runner built
		// with no Env at all — the zero value, which this package promises
		// is usable — still has to be able to make a pipe.
		return "/tmp"
	}
	if !filepath.IsAbs(dir) {
		return filepath.Join(r.workDir(), dir)
	}
	return dir
}

// procSubPipe is one substitution's path: the name a command was given, and —
// for `>(cmd)` only — the descriptor holding that pipe open until the command
// is done with it. See openFifoReadEnd.
//
// `=(cmd)`'s regular file is one of these too, with no descriptor to hold: it
// is finished before the path is handed over, and what the two forms share is
// the only thing this type is for — a path that the command which named it
// owns, and that goes away with it.
type procSubPipe struct {
	path string
	hold *os.File
}

// takeProcSubs hands over the paths a command's substitutions made, and
// forgets them.
//
// Taken rather than read, because they belong to one command: the next one has
// its own, and a path left on the runner would be removed after some later
// command that never mentioned it.
func (r *Runner) takeProcSubs() []procSubPipe {
	pipes := r.procSubs
	r.procSubs = nil
	return pipes
}

// removeProcSubs takes away what a command's substitutions left behind.
//
// It is also the whole of `=(cmd)`'s lifetime, and measured to be: `f==(echo
// hi); cat $f` is `No such file or directory` in zsh 5.9.2, so the file lives
// exactly as long as the pipe does and needs no mechanism of its own.
//
// The pipe is removed as soon as the command that named it is done. Whatever
// is still reading or writing it holds an open file and does not care that the
// name is gone, which is the property that makes this safe to do early rather
// than at exit — a long session would otherwise fill its directory.
//
// And it is what ends the two waits a substitution can be in, which is why
// this runs even when the command never touched the path. The placeholder
// closing is the end-of-file a `>(cmd)` is waiting for; the name going away is
// the answer a `<(cmd)`'s writer is waiting for.
//
// # Except while this shell still holds a descriptor onto the pipe
//
// "Whatever is still reading it holds an open file" is true of a *process*
// that was handed the path and false of the one arrangement where the shell
// keeps the descriptor itself and reads it later: `sysopen -r -o nonblock -u
// fd <(cmd)`, or `exec {fd}< <(cmd)`. There the open does not wait for a
// peer, so the command that named the path finishes before the writer's poll
// has seen the reader — and the poll's give-up condition is exactly this
// removal, so the name going away is read as "nobody opened it" and the
// substituted command never runs. Measured 2026-09-11 on the nonblocking
// form: two runs in ten answered end-of-file with the command lost, where
// zsh 5.9.2 answers the command's output every time (#1750).
//
// So the name stays while one of this shell's own descriptors is open on it.
// Nothing else about the lifetime moves: the pipe still goes with the
// directory when the shell stops being one, which is what CleanUp is for, and
// a command whose substitution nobody opened still has its pipe taken away at
// once. The placeholder is closed either way — a `>(cmd)` whose writing end
// the script now holds gets its end-of-file from the script closing that,
// which is where it belongs.
func (r *Runner) removeProcSubs(pipes []procSubPipe) {
	for _, p := range pipes {
		if p.hold != nil {
			_ = p.hold.Close()
		}
		if r.holdsDescriptorOnto(p.path) {
			continue
		}
		_ = os.Remove(p.path)
	}
}

// holdsDescriptorOnto reports whether one of this shell's own descriptors is
// still open on path.
//
// The table is asked rather than the filesystem, because the question is
// about this shell and not about the machine: a *process* the shell handed
// the path to has its own descriptor and is unaffected by the name going
// away, which is the case the removal above was written for.
//
// A file's name is what it was opened by, which is this path exactly — both
// routes that put one in the table open it from the word the substitution
// expanded to.
func (r *Runner) holdsDescriptorOnto(path string) bool {
	for _, v := range r.fds {
		if f, ok := v.(*os.File); ok && f.Name() == path {
			return true
		}
	}
	return false
}

// CleanUp removes what this shell made for itself.
//
// Only the directory the named pipes went in, at present. Finish calls it, so
// a shell that ran to its end — a binary, a Session that was closed, a
// Runner.Run that returned — has already had this done. It stays public for
// the caller that drives a Runner with RunPart and never reaches Finish,
// which is the one shape that would otherwise leave the directory.
//
// It was documented the other way round once: that a binary which exits need
// not bother, since the directory is under the machine's temporary one. That
// was wrong in the way a leak is always wrong — nothing removes /tmp between
// reboots, so every invocation that used a substitution left a directory
// there for good. 10,585 of them accumulated in one working session (#1284).
//
// Not safe to call from a subshell: clone shares the directory with its
// parent rather than making a second one, so a copy removing it would take
// the original's pipes away mid-command. Finish and the `exec` replacement
// both ask cleanUpAtEnd instead, which is this behind that guard.
func (r *Runner) CleanUp() {
	if r.procSubHome != nil && r.procSubHome.dir != "" {
		_ = os.RemoveAll(r.procSubHome.dir)
	}
}

// cleanUpAtEnd is CleanUp for the two places a shell stops being one.
//
// Guarded on the subshell flag, which is the same guard runExitTrap uses and
// for the same reason: a clone is a shell ending, but it is not *the* shell
// ending, and the directory belongs to the whole tree. clone copies the
// procSubDirs pointer rather than the directory, so `( cat <(echo hi) )` in a
// long script would otherwise remove the parent's directory on the way out of
// the parentheses and leave every later substitution making a pipe in a
// directory that is not there.
//
// One helper called from both places rather than the line written twice: the
// two exits are already easy to fix one and forget the other — that is how
// this bug reached a shipped binary in the first place, with CleanUp written
// and nothing calling it.
func (r *Runner) cleanUpAtEnd() {
	if r.inSubshell {
		return
	}
	r.CleanUp()
}

// substEnd is the end of a substitution's pipe that this shell holds, and the
// count of shells still entitled to use it.
//
// # Why a count
//
// A real shell forks for `<(cmd)`, and the descriptor belongs to the *process*
// it forked. A job that process backgrounds is another fork, with a copy of
// the same descriptor, so the pipe stays open until the last of them goes —
// nobody arranges that and nothing in the shell decides it. Here the
// substitution's body is a goroutine and a job it backgrounds is another
// goroutine of the same process, sharing one `*os.File`, so the lifetime that
// a fork gives for free has to be reconstructed: closing on the body's return
// takes the descriptor away from a job that is still writing through it.
//
// That is #1767, and it is a core behavior rather than a dialect's: measured
// 2026-09-10,
//
//	cat <( { for i in 1 2 3; do printf B; sleep 0.2; done } & )
//
// prints `BBB` in zsh 5.9.2, bash 5.3, bash 3.2 and ksh93 alike, in about six
// hundred milliseconds — the reader waits for the job, not for the body. Here
// it printed one `B`, and in one dialect a `write error: File already closed`
// with it, because the second `printf` had a descriptor the body's return had
// closed underneath it. dash has no process substitution, which is an absence
// rather than a disagreement, so there is no axis here and no dialect is
// asked.
//
// # What counts as holding it
//
// A `&` job and nothing else. Every other clone a body makes — a subshell, a
// pipeline element, a function, a nested substitution — is joined before the
// body returns, so the body's own reference already outlives it. A background
// job is the one that does not, which is why the count is taken in
// Runner.background and released when that job finishes.
//
// A redirection does *not* take it away, and that is measured rather than
// assumed: `cat <( { sleep 0.6; printf X; } >/dev/null & )` still makes the
// reader wait the full six hundred milliseconds in all four shells, so a job
// that has pointed its own output somewhere else is still holding the pipe
// open. Reading the redirection here would be a refinement onto the wrong
// side of the measurement.
//
// # Why the nudge moved
//
// nudgeFifoEOF repeats a last-writer close until no reader is left to tell.
// With a writer still in the pipe there is nothing for it to deliver, and its
// give-up condition — ENXIO, no reader — cannot be reached while the command
// that named the path is still reading, so it would spin at its longest
// interval until removeProcSubs took the pipe away. It belongs with the close
// it is repeating, which is the last one.
type substEnd struct {
	mu   sync.Mutex
	held int
	file *os.File
	// anchor is the process group this body leads, if it ever asked for one.
	// It is here rather than beside it because the group's lifetime is the
	// same lifetime this count already reconstructs — see procanchor.go.
	anchor *procAnchor
	// nudge names the pipe where closing this end is what delivers the
	// end-of-file — `<(cmd)`, where the shell is the writer. Empty for
	// `>(cmd)`, whose reader has a placeholder instead. See openFifoReadEnd.
	nudge string
}

// opened records the descriptor the count is guarding.
//
// Separate from making the substEnd because the two directions learn it at
// different times: `>(cmd)`'s end is opened on the goroutine that expands the
// word, and `<(cmd)`'s is opened on the goroutine that runs the body, after a
// wait that may end in nobody having opened the other side at all. A count
// that never learns a descriptor closes nothing, which is the right answer
// for that case.
func (e *substEnd) opened(f *os.File) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.file = f
}

// keep adds a shell to the count.
//
// Called while the body is still running — a job is backgrounded by the body,
// so this happens before the body's own letGo — which is what keeps the count
// from reaching zero and then being raised again.
func (e *substEnd) keep() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.held++
}

// letGo takes a shell off the count, and closes the end when the last one
// goes.
func (e *substEnd) letGo() {
	e.mu.Lock()
	e.held--
	last := e.held == 0
	f, nudge, a := e.file, e.nudge, e.anchor
	e.mu.Unlock()
	if !last {
		return
	}
	// The process group goes with the descriptor, because it is the same
	// lifetime — see procanchor.go. Before the descriptor's own early return
	// and not after it: a direction that never opened one still has a group
	// to let go, since `echo <(true)` names a path nothing opens and a body
	// may have asked which process it was all the same.
	a.stop()
	if f == nil {
		return
	}
	_ = f.Close()
	if nudge != "" {
		nudgeFifoEOF(nudge)
	}
}

// holdPipeEnd keeps this shell's process-substitution end open for something
// that outlives the shell, and answers with what releases it.
//
// A shell that is not a substitution's body holds no end, and the release is
// then a no-op rather than a condition every caller has to write out.
func (r *Runner) holdPipeEnd() func() {
	if r.pipeEnd == nil {
		return func() {}
	}
	e := r.pipeEnd
	e.keep()
	return e.letGo
}
