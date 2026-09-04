// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
		if err := runnable(full); err != nil {
			return "", &pathError{
				name: name, resolved: full,
				missing: errors.Is(err, os.ErrNotExist), err: err,
			}
		}
		return full, nil
	}

	// A file that exists and will not run is remembered rather than returned:
	// a later entry may still have a good one, and only if none does is this
	// the answer. A directory of the matching name is remembered apart from
	// it, because whether a directory counts as a candidate at all is the
	// one part of this search the panel disagrees on.
	var denied, dirDenied *pathError
	path, _ := r.getVar("PATH")
	for _, dir := range r.pathElements(path) {
		if dir == "" {
			dir = "."
		}
		candidate := r.absolute(filepath.Join(dir, name))
		err := runnable(candidate)
		if err == nil {
			return candidate, nil
		}
		switch {
		case errors.Is(err, errIsDirectory):
			if dirDenied == nil {
				dirDenied = &pathError{
					name: name, resolved: candidate,
					onPathDirectory: true, err: err,
				}
			}
		case !errors.Is(err, os.ErrNotExist) && denied == nil:
			denied = &pathError{name: name, resolved: candidate, err: err}
		}
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
func runnable(path string) error {
	st, err := os.Stat(path)
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
	return 127
}
