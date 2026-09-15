// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
)

// The two marks a `-f` line puts on a function here: `-u`, which says the
// body is to be read from `$FPATH` the first time the name is called, and
// `-t`, which traces it.
//
// Measured 2026-09-15 against ksh93u+ 2012-08-01, `env -i` with a scratch
// HOME and no startup files, with `$FPATH` a directory holding a file `zz`
// whose whole content is `zz(){ echo "zz def $1"; }`:
//
//	typeset -fu zz            silent, 0
//	typeset -f zz             typeset -fu zz        — a declaration, no body
//	zz hello                  zz def hello
//	typeset -f zz             zz(){ echo "zz def $1"; }
//	functions -u zz           the same declaration under the other word
//	typeset -fu nope; nope    ksh: function: not found, status 127
//
// # `-u` is a *file* being sourced, not a body being read
//
// This is where it parts from zsh's autoloading, which reads the file **as**
// the function's body. Here the file is run, and it has to define the name:
// a file holding bare commands runs them and is then `function, built-in or
// type definition for bb not found in <path>`. Measured with a file whose
// content is `echo "bare body $1"` — the line runs, prints an empty argument,
// and the complaint follows.
//
// So the load is the `.` builtin over the path, and the check afterwards is
// whether the name is a function now. Neither half is optional: a shell that
// only read the file would define nothing, and one that only defined would
// lose the commands beside the definition.
//
// # `-t` traces, and only a keyword-defined function
//
// Measured in the same run, and it is the rule that made the letter look
// unimplementable at first:
//
//	function f { echo in; }; typeset -ft f; f; echo out
//	    + echo in / in / out
//	f(){ echo hi; }; typeset -ft f; f; echo out
//	    hi / out — no trace at all
//
// The POSIX spelling is not traced. That is this shell's standing split
// between the two function forms — `typeset` declares a local in a keyword
// body and assigns the global in a parenthesised one, which
// Semantics.TypesetLocalNeedsKeywordFunction already carries — reaching one
// more feature, and it is why interp.Runner.SetTracedFunctions is handed the
// keyword flag rather than only the name.
//
// The trace is the call's and not the option's: a function the traced one
// calls has its *call* traced, because that command is in the traced body,
// and its own body is not. `+t` takes the mark off.
//
// # Where the marks are kept
//
// In two arrays under names beginning with a dot, which is the shape
// dialect/zsh/sched.go uses and for the same reason: they are the *runner's*
// state, so a subshell gets its own copy and a mark made inside one does not
// escape it. A Go map in this package would be shared by every Runner the
// process holds.
const (
	markedUndefined = ".sh.fpath.undefined"
	markedTraced    = ".sh.fpath.traced"
)

// The two statuses a failed load leaves, which are the shell's own and are
// the two halves of "could not run it": 127 for a file that is not there and
// 126 for one that is and defines no function of that name.
const (
	notFound            = 127
	foundAndNotRunnable = 126
)

// registerFPath installs the three seams the marks need.
func registerFPath(r *interp.Runner) {
	// Making a mark: `typeset -fu nm`, `typeset -ft f`, and the same two
	// under `functions`. See Semantics.FunctionLettersThatMarkUndefined for
	// which letters get a line here.
	r.SetFunctionMarkedUndefined(markFunctions)
	// Reading one back: `typeset -f nm` on a name still waiting writes a
	// declaration and no body, which is this shell's whole rendering of it.
	// The *row* is Diagnostics.UndefinedFunctionListing, because there is no
	// header and no braces here for the substrate's block form to go in.
	r.SetUndefinedFunctions(func(name string) (string, bool) {
		if !markedWith(r, markedUndefined, name) {
			return "", false
		}
		return "", true
	})
	// And the listing narrowed by a marking letter with no operands:
	// `typeset -ft` writes the traced functions and `typeset -fu` the ones
	// still waiting.
	r.SetMarkedFunctions(markedFunctions)
	// The trace itself, asked once per call.
	r.SetTracedFunctions(func(rr *interp.Runner, name string, keyword bool) bool {
		// The keyword gate is measured and is not a caution: the
		// parenthesised spelling is not traced here, so a shell that traced
		// it would write lines this one never writes.
		return keyword && markedWith(rr, markedTraced, name)
	})
	// And the load, at the call.
	r.SetUndefinedFunctionLoader(loadFromFPath)
}

// markFunctions is what a `-f` line carrying `-u` or `-t` does to its
// operands.
//
// The letters arrive as written, minus the `f` that got the line here, so a
// line carrying both marks both. remove is the sign: `typeset +ft f` takes
// the tracing mark off and the function runs untraced afterwards, measured.
func markFunctions(r *interp.Runner, names []string, letters string, remove bool) int {
	status := 0
	for _, name := range names {
		if strings.ContainsRune(letters, 'u') {
			if remove {
				removeMark(r, markedUndefined, name)
				continue
			}
			if _, defined := r.FunctionText(name); !defined {
				// The name has to *be* a function for the call to reach the
				// loader at all, and an empty body is what stands in for the
				// one the file will bring. It is never listed — the listing
				// writes the declaration instead — and never run: the load
				// replaces the declaration before the call is made.
				r.DefineFunction(name, "{ :; }")
			}
			addMark(r, markedUndefined, name)
		}
		if strings.ContainsRune(letters, 't') {
			if remove {
				removeMark(r, markedTraced, name)
				continue
			}
			if _, defined := r.FunctionText(name); !defined {
				// The tracing mark needs a function to go on, where the
				// autoloading one *makes* one: measured, `typeset -ft
				// nosuch` is a silent 1 and `typeset -fu nosuch` a silent 0
				// that leaves the name waiting. The two letters part company
				// here and nowhere else.
				status = 1
				continue
			}
			addMark(r, markedTraced, name)
		}
	}
	return status
}

// markedFunctions is the population a listing narrowed by the marking letters
// asks for, which is the union over the letters written: `typeset -ftu` is
// what `-ft` writes together with what `-fu` writes.
func markedFunctions(r *interp.Runner, letters string) []string {
	var names []string
	seen := map[string]bool{}
	add := func(store string) {
		for _, name := range marks(r, store) {
			if !seen[name] {
				seen[name], names = true, append(names, name)
			}
		}
	}
	if strings.ContainsRune(letters, 'u') {
		add(markedUndefined)
	}
	if strings.ContainsRune(letters, 't') {
		add(markedTraced)
	}
	sort.Strings(names)
	return names
}

// loadFromFPath reads the file `$FPATH` holds for a name the shell is waiting
// for, and reports whether it tried.
//
// False for every name that carries no mark, which is nearly every call: this
// is asked before each one, so it has to be cheap and it has to be silent.
func loadFromFPath(r *interp.Runner, name string) bool {
	if !markedWith(r, markedUndefined, name) {
		return false
	}
	// Taken off first, whatever happens next. A file that does not define
	// the name leaves nothing to try again with, and a mark left standing
	// would send the next call back to the same file — measured, ksh93
	// answers a second call the same way it answers the first, with the
	// file's own commands running again, so the *file* is re-read and the
	// name is not still waiting. Removing the mark and letting the
	// definition stand is the same observable behavior for every file that
	// works, and it is what keeps a file that does not from looping.
	removeMark(r, markedUndefined, name)
	path, found := fpathFile(r, name)
	if !found {
		// ksh93's own sentence, and it names neither the function nor the
		// path: measured, `typeset -fu nope; nope` is `ksh: function: not
		// found` at 127. Recorded as it stands rather than improved on —
		// a script branching on the status sees 127 either way, and the
		// wording is the shell's.
		r.RemoveFunction(name)
		r.DiagnoseAsTheShellf("function: not found\n")
		// 127, and the script carries on — the same status and the same
		// non-fatality a command nothing could find leaves. Measured:
		// `typeset -fu nope; nope; echo st=$?` writes `st=127`.
		r.SetExitStatus(notFound)
		return true
	}
	dot, ok := r.Builtin(".")
	if !ok {
		// A dialect that unregistered the one builtin this reads a file
		// with. Nothing to load from, and nothing to run.
		r.RemoveFunction(name)
		r.SetExitStatus(notFound)
		return true
	}
	// The stub goes first, which is what makes the check below mean
	// anything: an empty body is still a function, so a file that defines
	// nothing would look like a file that had.
	r.RemoveFunction(name)
	// The file is **run**, not read as a body: its commands take effect and
	// the definition it leaves is what the call then runs.
	dot(r, r.ShellContext(), []string{path})
	if _, defined := r.FunctionText(name); !defined {
		// The file was found and read and left no function of that name.
		// ksh93 names both, which is what makes this worth wording: a script
		// that put the definition in the wrong file learns which file it
		// read.
		r.DiagnoseAsTheShellf(
			"function, built-in or type definition for %s not found in %s\n", name, path)
		// And this one **ends the script** at 126, which is the difference
		// between the two failures: a file that is not there is a command
		// that could not be found, and a file that is there and defines the
		// wrong thing is one that could not be run. Measured — the `echo`
		// after `typeset -fu other; other` never runs, and a `( … )` around
		// it ends the subshell and leaves the script going.
		r.StopTheScript(foundAndNotRunnable)
	}
	return true
}

// fpathFile is the first readable file `$FPATH` holds for a name.
//
// A colon-separated *string* here, where zsh's `$fpath` is an array — the two
// shells keep the same idea in the two shapes they keep `$PATH` in, and this
// one's empty entry means the working directory the way `$PATH`'s does.
func fpathFile(r *interp.Runner, name string) (string, bool) {
	list, _ := r.GetVar("FPATH")
	if list == "" {
		return "", false
	}
	for _, dir := range strings.Split(list, ":") {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, name)
		if _, err := r.ReadFileGated(path); err != nil {
			continue
		}
		return path, true
	}
	return "", false
}

// marks reads one of the two stores.
func marks(r *interp.Runner, store string) []string {
	names, _ := r.GetArray(store)
	return names
}

func markedWith(r *interp.Runner, store, name string) bool {
	return slices.Contains(marks(r, store), name)
}

func addMark(r *interp.Runner, store, name string) {
	names := marks(r, store)
	if slices.Contains(names, name) {
		return
	}
	r.SetArray(store, append(slices.Clone(names), name))
}

func removeMark(r *interp.Runner, store, name string) {
	names := marks(r, store)
	at := slices.Index(names, name)
	if at < 0 {
		return
	}
	r.SetArray(store, slices.Delete(slices.Clone(names), at, at+1))
}
