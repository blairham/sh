// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Finding a command is this shell's job, not the process's.
//
// os/exec's LookPath reads the *process's* PATH and resolves relative entries
// against the *process's* directory. A Runner holds both of those itself — Vars
// and Dir — for the same reason it declines to call os.Chdir: two Runners
// embedded in one program must not fight over one global. Handing the question
// to os/exec quietly opted out of that, and the result was a shell where
// setting PATH did nothing:
//
//	PATH=/tmp/mine; mycmd    → real bash runs it, we said "command not found"
//	PATH=; printf hi         → real bash finds nothing, we printed hi
//
// The second is the worse half. A script that clears PATH to control what it
// can reach was still reaching everything on the developer's.
//
// Every rule below was measured across dash, bash, ksh93 and zsh. Almost all
// of them are unanimous, so almost none of this is an axis — the wording is,
// and one rule: whether an *empty* PATH still means the current directory.

// pathError says why a command could not be run, which is two different
// answers the panel words and numbers differently: 127 for a name that
// resolved to nothing, 126 for a file that is there and will not start.
type pathError struct {
	// name is the operand as written.
	name string
	// resolved is the candidate that was found but would not run, empty when
	// nothing was found at all. One dialect prints it in place of the operand.
	resolved string
	// missing distinguishes "no such command" from "found it, cannot run it".
	missing bool
	// onPathDirectory marks a search whose only match was a directory, which
	// one dialect numbers 127 while still naming the candidate.
	onPathDirectory bool
	// err is the underlying failure, for the reason text and for a Gate's
	// event stream.
	err error
}

func (e *pathError) Error() string { return e.name + ": " + e.err.Error() }
func (e *pathError) Unwrap() error { return e.err }

// errNotFound is the "nothing by that name" case. It is deliberately not
// os.ErrNotExist: a bare name that PATH did not have is not a missing *file*,
// and three of the four shells word the two differently.
var errNotFound = errors.New("not found")

// lookPath resolves a command name the way a shell does.
//
// A name with a slash in it is a path and is used as written — PATH is not
// consulted at all, which is unanimous. Otherwise each PATH entry is tried in
// order and the first *executable regular file* wins: a directory of the same
// name is skipped, and so is a file without the execute bit, with the search
// carrying on past both. Only when nothing executable is found anywhere does
// the walk report, and then it distinguishes "saw a file that would not run"
// from "saw nothing at all" — measured: with a non-executable `cmd` in the
// first PATH entry and a good one in the second, every shell runs the second.
//
// An empty PATH *entry* means the current directory. That is POSIX and it is
// unanimous, and it is not the same as searching the current directory by
// default, which none of them do. Whether an empty PATH *variable* counts as
// one such entry is the one thing here the panel disagrees about — see
// pathElements.
func (r *Runner) lookPath(name string) (string, error) {
	if strings.ContainsRune(name, '/') {
		full := r.absolute(name)
		if err := r.runnable(full); err != nil {
			return "", &pathError{
				name: name, resolved: full,
				missing: errors.Is(err, os.ErrNotExist), err: err,
			}
		}
		return full, nil
	}

	// What the command hash already holds, which is the whole reason it is
	// kept — and the one place the panel parts company over it. bash runs
	// what it remembered and reports the *remembered path* when it has gone;
	// zsh, ksh93 and dash look before they leap, and a stale entry sends
	// them back to PATH, where they find the next copy and run it. Measured
	// with two copies of one name on PATH, the first hashed and then
	// deleted — the arrangement that tells the readings apart, since with
	// one copy all four fail and only the wording moves.
	if hashed, ok := r.hashedCommandPath(name); ok && r.rememberingLookups() {
		full := r.absolute(hashed)
		err := r.runnable(full)
		if err == nil {
			if earlier, found := r.executableBeforeTheHashedPath(name, full); found &&
				!r.ask(r.sem().HashedPathShadowsAnEarlierDirectory,
					"a remembered location standing in front of a copy that has appeared earlier on PATH") {
				// The entry does not shadow the search here, so the copy
				// that has appeared in front of it is the answer and the
				// table is pointed at it. See
				// Semantics.HashedPathShadowsAnEarlierDirectory.
				r.retrackCommand(name, earlier)
				return earlier, nil
			}
			r.hashCommandHit(name)
			return full, nil
		}
		if r.ask(r.sem().CommandHashIsTrusted, "a hashed path used without looking for it again") &&
			!r.checksHashedCommand {
			// The remembered path is the answer even now, so the failure is
			// reported as that *path* — the same sentence a command word
			// with a slash in it gets, and the same 127. Measured: bash says
			// `/tmp/hb/zzcmd: No such file or directory` where the other
			// three have already found the next copy and run it.
			r.hashCommandHit(name)
			return "", &pathError{
				name: hashed, resolved: full,
				missing: errors.Is(err, os.ErrNotExist), err: err,
			}
		}
		// Gone, and this dialect looks again. Forgotten here rather than
		// re-checked on every later lookup: a second run of the same name
		// walks PATH once and hashes what it finds.
		r.forgetHashedCommand(name)
	}

	// A file that exists and will not run is remembered rather than returned:
	// a later entry may still have a good one, and only if none does is this
	// the answer. A directory of the matching name is remembered apart from
	// it, because whether a directory counts as a candidate at all is the
	// one part of this search the panel disagrees on.
	var denied, dirDenied *pathError
	// And the third reading: the first candidate that *existed*, whatever it
	// was. One column keeps that one and reports a directory found there as
	// if nothing had been found — see PathCandidateReport.FirstExistingCandidate,
	// where the pair of rows that needs it is.
	var firstExisting *pathError
	// The other reading of the same walk, in the one column that has it: the
	// failure reported is the **last** entry searched rather than the first
	// interesting one. kept is what that entry left, and skipped is the run
	// of entries behind it whose candidate was simply not there — resolved
	// at the end, because only the last of them can matter and each costs a
	// stat of the directory. See Semantics.PathCandidateReported.
	last := r.sem().PathCandidateReported == LastSearchedEntry
	var kept *pathError
	var skipped []string
	for _, dir := range r.pathElements(r.commandSearchPath()) {
		if dir == "" {
			dir = "."
		}
		candidate := r.absolute(filepath.Join(dir, name))
		err := r.runnable(candidate)
		if err == nil {
			return candidate, nil
		}
		isDir := errors.Is(err, errIsDirectory)
		switch {
		case isDir:
			if dirDenied == nil {
				dirDenied = &pathError{
					name: name, resolved: candidate,
					onPathDirectory: true, err: err,
				}
			}
		case !errors.Is(err, os.ErrNotExist) && denied == nil:
			denied = &pathError{name: name, resolved: candidate, err: err}
		}
		if firstExisting == nil && !errors.Is(err, os.ErrNotExist) &&
			!errors.Is(err, syscall.ENOTDIR) {
			firstExisting = &pathError{
				name: name, resolved: candidate,
				onPathDirectory: isDir, err: err,
			}
		}
		if !last {
			continue
		}
		// A candidate that is not there says nothing about this entry until
		// the entry itself is looked at — `/nonexistent/zzcmd` and
		// `$emptydir/zzcmd` both fail with the same errno and the column
		// this is for treats them oppositely. Held and answered below.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			skipped = append(skipped, dir)
			continue
		}
		if isDir && !r.ask(r.sem().DirectoryOnPathIsACandidate,
			"a directory found on PATH standing as the failed candidate") {
			// This column does not keep a directory at all, so the entry
			// leaves nothing behind and what stood before it still holds.
			skipped = append(skipped, dir)
			continue
		}
		kept, skipped = &pathError{
			name: name, resolved: candidate,
			onPathDirectory: isDir, err: err,
		}, nil
	}
	if last {
		// The last entry that was really searched decides. Walking the held
		// run backwards is what finds it: the first of them that is an
		// existing directory was searched and found nothing, and if none of
		// them was then the entry before them is still the last searched.
		for i := len(skipped) - 1; i >= 0; i-- {
			if r.pathEntryIsADirectory(skipped[i]) {
				return "", &pathError{name: name, missing: true, err: errNotFound}
			}
		}
		if kept != nil {
			return "", kept
		}
		return "", &pathError{name: name, missing: true, err: errNotFound}
	}
	if r.sem().PathCandidateReported == FirstExistingCandidate {
		if firstExisting == nil {
			return "", &pathError{name: name, missing: true, err: errNotFound}
		}
		if firstExisting.onPathDirectory &&
			!r.ask(r.sem().DirectoryOnPathIsACandidate,
				"a directory found on PATH standing as the failed candidate") {
			// The column this reading is for answers that No, so a directory
			// kept here is reported as nothing found — the same sentence and
			// status it gives a name that was never on PATH at all. Keeping
			// it is still what suppresses the later non-executable file,
			// which is the whole difference from the other reading.
			return "", &pathError{name: name, missing: true, err: errNotFound}
		}
		if r.unspecified {
			return "", &pathError{name: name, missing: true, err: errNotFound}
		}
		return "", firstExisting
	}
	if denied != nil {
		return "", denied
	}
	if dirDenied != nil &&
		r.ask(r.sem().DirectoryOnPathIsACandidate, "a directory found on PATH standing as the failed candidate") {
		return "", dirDenied
	}
	return "", &pathError{name: name, missing: true, err: errNotFound}
}

// pathEntryIsADirectory reports whether a PATH entry is a directory that
// exists, which is what makes it an entry the search really *looked in*.
//
// Asked only where Semantics.PathCandidateReported is LastSearchedEntry, and
// there only for the run of entries at the end of the walk whose candidate
// was not there — so a successful lookup pays nothing and a failed one pays
// one stat per trailing miss, on a path that is already ending in a
// diagnostic.
func (r *Runner) pathEntryIsADirectory(dir string) bool {
	st, err := r.stat(r.absolute(dir))
	return err == nil && st.IsDir()
}

// executableBeforeTheHashedPath is the first thing PATH holds by this name in
// a directory it searches **before** the one the command hash remembered.
//
// It answers the one question the panel splits on once an entry is in the
// table and still runs, and it is called only where that question is live:
// the dialect that reads its table as the answer is spared the walk, because
// `Yes` and "nothing found in front" are the same outcome and the walk is
// exactly the cost the table exists to avoid. That guard is a read of the
// axis rather than an ask on purpose — this runs in front of every external
// command a shell has run twice, so an ask here would refuse each one in a
// Runner that has chosen no dialect, which is the hazard trackingIsOff
// carries. The *ask* is one caller up, where a copy really has appeared in
// front and the columns really do disagree.
//
// The walk stops at the remembered path rather than at its directory's index,
// so an entry whose directory has left PATH — `hash -p`, a PATH the table
// outlived — is searched past rather than treated as position zero.
func (r *Runner) executableBeforeTheHashedPath(name, hashed string) (string, bool) {
	if r.sem().HashedPathShadowsAnEarlierDirectory == Yes {
		return "", false
	}
	for _, dir := range r.pathElements(r.commandSearchPath()) {
		if dir == "" {
			dir = "."
		}
		candidate := r.absolute(filepath.Join(dir, name))
		if candidate == hashed {
			return "", false
		}
		if r.runnable(candidate) == nil {
			return candidate, true
		}
	}
	return "", false
}

// pathElements splits PATH, and the one interesting case is the empty string.
//
// strings.Split("", ":") is a slice holding *one empty element*, and an empty
// element means the current directory — so `PATH=; cmd` still finds a `cmd`
// sitting next to the script. That reads like a bug and is what dash, bash and
// zsh actually do; ksh93 alone treats an empty PATH as no elements at all.
//
// Both readings are defensible and the panel is split, so this asks rather
// than picks. It took two mistakes to land here: the corpus case that should
// have caught it put its command in a subdirectory, where all four agree, and
// a unit test written from that wrong assumption then "fixed" working code.
// The case now keeps a copy in the current directory, which is the only
// arrangement that tells the answers apart.
func (r *Runner) pathElements(path string) []string {
	if path == "" {
		if !r.ask(r.sem().EmptyPathIsTheCurrentDirectory,
			"an empty PATH meaning the current directory") {
			return nil
		}
		return []string{""}
	}
	return strings.Split(path, string(filepath.ListSeparator))
}

// absolute resolves a candidate against this runner's directory and makes it
// absolute.
//
// Absolute rather than merely runner-relative, because what comes back is
// handed to os/exec — and exec.Command runs LookPath *again* on any path with
// no separator in it. An empty PATH entry means the current directory, and
// filepath.Join(".", "dup") is "dup", so a command found that way went back
// through the process's PATH and failed with Go's own
// `exec: "dup": executable file not found in $PATH`. The corpus caught it; the
// unit tests did not, because they set Dir and never hit the bare form.
func (r *Runner) absolute(path string) string {
	if abs, err := filepath.Abs(r.atDir(path)); err == nil {
		return abs
	}
	return r.atDir(path)
}

// runnable reports whether a path is something the operating system would
// start, and says why not when it would not.
//
// A directory is its own answer rather than a permission error, because two
// dialects print "Is a directory" where the other two report the EACCES that
// execve actually returns. Deciding here keeps that a wording question.
//
// A method rather than a function so the probe passes the gate: a PATH
// search stats a candidate in every directory PATH names, which was a walk
// no policy could see. A denied stat reads as the candidate not existing, so
// the search moves on and a name whose every candidate is hidden is
// "command not found" — the deny short-circuits before anything is probed
// further, and the exec gate is still consulted after the search with
// whatever the search resolved.
func (r *Runner) runnable(path string) error {
	st, err := r.stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return &fs.PathError{Op: "exec", Path: path, Err: errIsDirectory}
	}
	if st.Mode().Perm()&0o111 == 0 {
		return &fs.PathError{Op: "exec", Path: path, Err: fs.ErrPermission}
	}
	return nil
}

// errIsDirectory is spelled out rather than taken from syscall so the message
// reads the way the shells print it — "Is a directory", which is what bash and
// ksh93 say and what the other two override.
var errIsDirectory = errors.New("is a directory")

// naming is how one caller reports the command it could not run. The two
// callers differ in exactly two ways, and both are measured rather than
// stylistic.
type naming struct {
	// bare is this caller's wording for a name that was never found at all.
	// It is the only case the panel words differently between a command word
	// and `exec`: "command not found" against "exec: name: not found".
	bare string
	// fallback is the substrate's own wording when the dialect sets none.
	fallback string
	// absolute absolutises an operand that was written with a slash. True for
	// `exec` and false for a command word — bash reports `exec ./x` as a full
	// path and a bare `./x` as written.
	absolute bool
	// cannotExecute overrides the wording for a file that will not run, which
	// two dialects prefix with the builtin's name when `exec` is asking.
	cannotExecute string
}

// cannotRun reports a command that could not be started and returns the status
// to carry.
//
// Shared by an ordinary command and by `exec`, because the panel words them
// identically in every case but one: a bare name that was never found is
// "command not found" from a command word and "exec: name: not found" from the
// builtin. Everything else — a path that is not there, a file that will not
// run, a directory — reads the same either way, so it is said in one place.
func (r *Runner) cannotRun(err error, how naming) int {
	var pe *pathError
	if !errors.As(err, &pe) {
		// Not a lookup failure — a start that failed for some other reason.
		r.diagf("%s\n", Wording(r.diag().CannotExecute, "%[1]s: %[2]s",
			"", r.diag().reasonText(reason(err))))
		return 126
	}

	name := pe.name
	// One dialect names the path it decided to run rather than the operand,
	// and it draws the line in a place worth measuring rather than guessing:
	// a name that came from PATH is reported resolved, and a name already
	// written with a slash is reported as written — `./noexec` stays
	// `./noexec`. `exec` is the exception and absolutises both, which is why
	// the caller says which it is.
	fromPath := !strings.ContainsRune(pe.name, '/')
	if r.diag().NamesResolvedPath && pe.resolved != "" && (fromPath || how.absolute) {
		if abs, absErr := filepath.Abs(pe.resolved); absErr == nil {
			name = abs
		}
	}

	if !pe.missing {
		why := reason(pe.err)
		if errors.Is(pe.err, syscall.ENOEXEC) && r.diag().BinaryFileReason != "" {
			// A file the kernel refused that this shell then looked inside
			// and found was not shell text. One dialect has a phrase for it
			// — see Diagnostics.BinaryFileReason and noexecscript.go.
			why = r.diag().BinaryFileReason
		}
		if errors.Is(pe.err, errIsDirectory) && r.diag().DirectoryReason != "" {
			// This dialect does not pre-check for a directory; it reports
			// what execve came back with, which is a permission error.
			why = r.diag().DirectoryReason
		}
		r.diagf("%s\n", Wording(orElse(how.cannotExecute, r.diag().CannotExecute),
			"%[1]s: %[2]s", name, r.diag().reasonText(why)))
		if pe.onPathDirectory && r.diag().DirectoryOnPathStatus != 0 {
			// One dialect names the directory it found and then numbers the
			// failure as if it had found nothing.
			return r.diag().DirectoryOnPathStatus
		}
		return 126
	}

	// Nothing by that name. A slash makes it a path that is not there, which
	// three of the four word differently from a bare name off PATH.
	format := orElse(how.bare, how.fallback)
	if strings.ContainsRune(pe.name, '/') {
		format = orElse(r.diag().PathNotFound, format)
	}
	// The name and nothing else. Every not-found wording in the panel spells
	// its reason out — "command not found", "No such file or directory" — so
	// there is none to pass, and passing one anyway printed
	// `%!(EXTRA string=Not found)` against bash's plain `%s`. An indexed
	// format tolerates an unused argument and a plain one does not, which is
	// the same trap interp/wording_test.go already pins.
	r.diagf("%s\n", Wording(format, how.fallback, name))
	if r.NotFoundHint != nil {
		// A second line, from a caller that knows something this package may
		// not — see Runner.NotFoundHint. The operand as written rather than
		// the resolved path: a hint is about the word the person typed.
		if hint := r.NotFoundHint(pe.name); hint != "" {
			r.errf("%s\n", hint)
		}
	}
	return 127
}

// lookPathAll is the search `type -a` makes: every runnable candidate on
// PATH, in PATH order, duplicates and all — the shells that list them do not
// deduplicate either. A name with a slash is its own answer, the same rule
// the single search applies.
func (r *Runner) lookPathAll(name string) []string {
	if strings.ContainsRune(name, '/') {
		full := r.absolute(name)
		if r.runnable(full) == nil {
			return []string{full}
		}
		return nil
	}
	var hits []string
	path, _ := r.getVar("PATH")
	for _, dir := range r.pathElements(path) {
		if dir == "" {
			dir = "."
		}
		candidate := r.absolute(filepath.Join(dir, name))
		if r.runnable(candidate) == nil {
			hits = append(hits, candidate)
		}
	}
	return hits
}

// autoCdInstead reads a command word that named a directory as a `cd`, and
// reports whether it did.
//
// The capability two shells in the panel call `autocd`, and it is one function
// because it is one behavior: bash's `shopt -s autocd` and zsh's `setopt
// auto_cd` ask for the same thing, and a copy in each dialect would be two
// places to fix the next time this is wrong. See Runner.autoCd.
//
// Four conditions, and each of them is measured rather than assumed —
// bash 5.3.15 through a pseudo-terminal, 2026-09-08, since none of this is
// askable on the `-c` route:
//
//   - The option is on. `shopt -u autocd` then `subdir` is `command not
//     found` at 127, which is what this shell already says.
//   - The shell is interactive. `bash -c 'shopt -s autocd; subdir'` is
//     `command not found` at 127 as well, with the option on: the name is
//     interactive-only in bash and granting it to a script would be this
//     shell doing something bash does not.
//   - Command lookup has already failed. This is called from exec, after the
//     builtins, the functions and PATH have all had their turn — measured
//     with a *directory* named `echo` in the working directory, where
//     `echo hello` still prints `hello`. So a directory never shadows a
//     command; it only catches a word nothing else would run.
//   - The word names a directory. `nosuchdir` is `command not found` at 127
//     with the option on, so this is a fallback for directories and not a
//     general one.
//
// What it then runs is `cd -- <every word>`, arguments and all, which is
// bash's own substitution rather than a simplification of it: with the option
// on, `subdir deeper` in bash 5.3.15 reports `cd: too many arguments` at
// status 2 — the operands reach `cd`, which is the only way that message
// could exist. This shell's `cd` ignores an operand after the first, which is
// a difference in `cd` and not in this, and is why the words are handed on
// whole rather than trimmed to one here.
//
// Whether the substitution is *announced* is where the two shells that have
// this part company, so it is an axis and not a default with an exception:
// bash writes `cd -- subdir` before moving and zsh writes nothing. See
// Semantics.AutoCdAnnouncesTheSubstitution.
//
// The line goes to this shell's error stream, which is where bash puts it —
// captured by `exec 2>file` and *not* suppressed by a redirection on the word
// itself, since bash rewrites the command before it opens one. Ours is
// written after the redirection is in place, so `subdir 2>/dev/null` hides
// here what it shows there; that is a difference of one stream on one line,
// and the alternative is threading an unredirected stream through exec for
// it.
func (r *Runner) autoCdInstead(ctx context.Context, argv []string) (status int, took bool) {
	if !r.autoCd || !r.Interactive || len(argv) == 0 {
		return 0, false
	}
	cd, ok := r.lookupBuiltin("cd")
	if !ok {
		// A shell whose `cd` was taken away with `enable -n` has nothing to
		// substitute, so the word goes back to being a command that was not
		// found.
		return 0, false
	}
	// Through the runner's own stat, so the probe passes the gate for the
	// reason runnable's does: this is the shell looking at the disk on a
	// script's behalf, and a policy that hides a directory must hide it here
	// too. A denied or missing candidate is simply not a directory, which is
	// the answer that leaves the word a command.
	info, err := r.stat(r.atDir(argv[0]))
	if err != nil || !info.IsDir() {
		return 0, false
	}
	operands := append([]string{"--"}, argv...)
	if r.ask(r.sem().AutoCdAnnouncesTheSubstitution, "`autocd` writing the `cd` it read a directory name as") {
		_, _ = fmt.Fprintln(r.stderr(), "cd "+strings.Join(operands, " "))
	}
	return cd(r, ctx, operands), true
}
