// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"sort"
)

// Builtin is a command the shell runs itself rather than executing.
//
// It returns an exit status. Writing to r.Stdout and r.Stderr rather than to
// the process's own streams is what makes it work inside a pipeline and under
// a redirection, because those are what the runner points at the right place.
//
// args are the operands, without the name the command was called by. `greet a
// b` gives {"a", "b"} and not {"greet", "a", "b"} — the same shape the
// builtins in this package are written against, where args[0] is the first
// thing after the name. Said here because guessing it the other way costs a
// silently dropped argument rather than a compile error.
type Builtin func(r *Runner, ctx context.Context, args []string) int

// Register adds a builtin, replacing any of the same name.
//
// This is one of the two ways to build a shell on this package, and the line
// between them is worth stating because most of a dialect belongs on the
// other side of it.
//
// Register what cannot be written in shell: commands that must reach the
// interpreter's own state or the operating system. `cd` has to change the
// working directory the runner uses, `read` has to put a value into a
// variable of the calling shell, `local` has to create a scope, `trap` has to
// install a handler. None of those can be expressed as a shell function
// because a shell function has no way to say them.
//
// Everything else is better as a **shell function**, sourced as a prelude and
// needing no Go at all. Functions already shadow builtins and external
// commands alike, so a dialect layer can define, replace or wrap anything
// this package provides without touching it:
//
//	basename() { printf '%s\n' "${1##*/}"; }
//	echo()     { printf '%s\n' "$*"; }   # replaces the builtin
//
// The distinction is not stylistic. A prelude is portable across every
// implementation of this language, can be tested with any shell, and cannot
// break the substrate. Reach for Go when shell genuinely cannot express the
// thing, and not before.
func (r *Runner) Register(name string, fn Builtin) {
	if r.custom == nil {
		r.custom = map[string]Builtin{}
	}
	r.custom[name] = fn
}

// Unregister removes a builtin, including one this package provides.
//
// A dialect that does not have a command should not offer it, and hiding one
// is how that is said — the same reasoning as the parser refusing a construct
// the dialect lacks rather than accepting it and meaning something else.
func (r *Runner) Unregister(name string) {
	if r.custom == nil {
		r.custom = map[string]Builtin{}
	}
	r.custom[name] = nil
}

// Builtin looks up a builtin by name, so a dialect can give one a second name
// without reimplementing it.
//
// `source` is why this exists. It is a synonym for `.` in bash, ksh93 and zsh
// and absent from dash, which makes it a dialect's answer rather than the
// substrate's — but a synonym should be the *same* function, not a copy of it,
// and a dialect package cannot reach an unexported one. Registering what this
// returns is the difference between two names for one builtin and two
// builtins that will drift.
//
// It follows the same precedence lookupBuiltin does, so a dialect that has
// already replaced `.` gets its own replacement back rather than the core's.
func (r *Runner) Builtin(name string) (Builtin, bool) { return r.lookupBuiltin(name) }

// lookupBuiltin resolves a name, letting a registration win over the built-in
// table so a dialect can replace as well as add.
func (r *Runner) lookupBuiltin(name string) (Builtin, bool) {
	if r.disabledBuiltins[name] {
		// `enable -n` puts a name aside without forgetting what it was, so
		// the word is looked up on PATH like any other and enabling it again
		// gets the same builtin back.
		return nil, false
	}
	if fn, ok := r.custom[name]; ok {
		// A nil entry is an explicit removal rather than a missing one.
		return fn, fn != nil
	}
	fn, ok := builtins[name]
	return fn, ok
}

// IsSpecialBuiltin reports whether a name is one of the POSIX special
// builtins, which a dialect layer needs in order to match the two behaviors
// that follow from that list: an assignment prefixed to one persists, and a
// failure in one is fatal to a non-interactive shell.
func IsSpecialBuiltin(name string) bool { return specialBuiltins[name] }

// The accessors below are what a registered builtin needs, and they are here
// because writing one found them missing.
//
// A builtin cannot use os.Stdout: inside a pipeline or under a redirection the
// runner's streams point somewhere else, and a builtin that writes to the
// process's own would escape both. It also cannot touch r.Vars directly and
// expect a nil map to work.

// Out is the stream a builtin should write its output to.
func (r *Runner) Out() io.Writer { return r.stdout() }

// Err is the stream a builtin should write diagnostics to.
func (r *Runner) Err() io.Writer { return r.stderr() }

// In is the stream a builtin should read from.
//
// The runner's own resolution, not a second copy of it. It was a copy, and it
// answered os.Stdin for a nil stream after the three streams had settled on
// meaning empty — a duplicate of a decision is a place for the decision to go
// stale.
func (r *Runner) In() io.Reader { return r.stdin() }

// SetVar sets a shell variable, creating the map if needed.
// Diagnosef writes a diagnostic the way a builtin of this package would:
// located the dialect's way, naming the shell — and the builtin, in the
// dialect that puts it there.
//
// Writing to Err directly is the other thing, and it is what a builtin's
// *output* uses. A complaint written that way carries no location, which is
// how a registered builtin ends up saying less than the one beside it. This
// is here because writing one found it missing.
func (r *Runner) Diagnosef(format string, args ...any) { r.diagf(format, args...) }

func (r *Runner) SetVar(name, value string) { r.setVar(name, value) }

// GetVar reads a shell variable, falling back to the environment.
func (r *Runner) GetVar(name string) (string, bool) { return r.getVar(name) }

// BuiltinNames is every builtin this runner has, sorted.
//
// For a shell that has to offer them: a completer at a prompt needs to know
// what running a word would find, and the table is this package's. Sorted
// because a caller listing them wants an order, and this is the only place
// that can give a stable one — the registrations live in a map.
func (r *Runner) BuiltinNames() []string {
	seen := make(map[string]bool, len(builtins)+len(r.custom))
	for name := range builtins {
		seen[name] = true
	}
	// A registration wins, and a nil one is a removal rather than an entry.
	for name, fn := range r.custom {
		if fn == nil {
			delete(seen, name)
			continue
		}
		seen[name] = true
	}
	// And one `enable -n` switched off is not what running the word would
	// find, which is what this list is for.
	for name := range r.disabledBuiltins {
		delete(seen, name)
	}
	return sortedNames(seen)
}

// SetBuiltinEnabled switches a builtin off, or back on.
//
// Off is not removal: the name is put aside and the word is then looked up on
// PATH like any other, and switching it back on gets the same builtin. That
// is what `enable -n` means in one dialect and what `disable` means in
// another, and the difference between them is entirely in the builtin that
// calls this.
//
// Unregister is the other thing, and they are not interchangeable: a dialect
// that never had `let` removes it, and a script that switched `cd` off can
// switch it back.
func (r *Runner) SetBuiltinEnabled(name string, enabled bool) {
	if enabled {
		delete(r.disabledBuiltins, name)
		return
	}
	if r.disabledBuiltins == nil {
		r.disabledBuiltins = map[string]bool{}
	}
	r.disabledBuiltins[name] = true
}

// DisabledBuiltins is every name switched off, sorted. One dialect lists
// exactly this when `disable` is given nothing to do.
func (r *Runner) DisabledBuiltins() []string {
	seen := make(map[string]bool, len(r.disabledBuiltins))
	for name := range r.disabledBuiltins {
		seen[name] = true
	}
	return sortedNames(seen)
}

// KnownBuiltin reports whether the name is a builtin at all, switched off or
// not. A dialect needs it to tell "switched off" from "never existed", which
// are different answers to `enable somename`.
func (r *Runner) KnownBuiltin(name string) bool {
	if fn, ok := r.custom[name]; ok {
		return fn != nil
	}
	_, ok := builtins[name]
	return ok
}

// FuncNames is every function this runner has defined, sorted.
func (r *Runner) FuncNames() []string {
	seen := make(map[string]bool, len(r.funcs))
	for name := range r.funcs {
		seen[name] = true
	}
	return sortedNames(seen)
}

func sortedNames(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// LookPath resolves a name through PATH the way a command word is resolved —
// against the runner's own PATH and directory, never the process's.
//
// For a registered builtin that answers "where would this run from" while
// deliberately passing over functions and builtins: one dialect has a builtin
// whose whole job is that narrower question, and without this it would have
// to reimplement the search the runner already does.
func (r *Runner) LookPath(name string) (string, bool) {
	path, err := r.lookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// WriterForFd is the stream a builtin writing "to descriptor n" needs: the
// named two by their numbers, anything past them from the shell's own table —
// the writing half of what `read -u` already reads. A number nothing is open
// at, or one held by something that cannot be written, is not a stream.
func (r *Runner) WriterForFd(fd int) (io.Writer, bool) {
	switch fd {
	case 1:
		return r.stdout(), true
	case 2:
		return r.stderr(), true
	}
	if v, held := r.fds[fd]; held {
		w, ok := v.(io.Writer)
		return w, ok
	}
	return nil, false
}

// NamedOption reads one `set -o` name's current state, for a registered
// builtin that presents the same state under its own names — a listing has to
// read the live answer, and the fields it lives in are the runner's own.
// The second result says whether this shell has the name at all.
func (r *Runner) NamedOption(name string) (on, known bool) {
	o, ok := r.lookupSetOption(name)
	if !ok {
		return false, false
	}
	return o.state(r), true
}

// Expand performs parameter and command expansion on raw text.
//
// For a caller that holds a *setting* which is a path with parameters in it —
// `ENV=$HOME/.shrc` is the usual spelling — and has to turn it into the path
// before opening it. Without this the caller would have to parse and expand,
// which is the whole of this package.
//
// No field splitting and no pathname expansion: a setting that names a file
// names one, and the same reasoning a here-document's body goes through
// applies here, which is why it goes through the same code.
func (r *Runner) Expand(text string) string {
	if text == "" {
		return ""
	}
	return r.expandRawText(text)
}
