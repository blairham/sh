// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"sort"
	"sync"

	"github.com/blairham/sh/syntax"
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
//
// Diagnostics used to be one of those things and are not any more: a function
// the prelude defines speaks for the shell, so its refusals carry the location
// and the name a builtin's carry, and `diagnose` writes one. See prelude.go.
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
	if name == diagnoseCommand && r.speaker != "" {
		// The prelude's diagnostics seam, and only there: a script running
		// the word gets whatever the dialect it is written for would give
		// it, which is a command that was not found. See prelude.go.
		return biDiagnose, true
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

// DiagnoseAsTheShellf is Diagnosef for a complaint that is not the running
// builtin's own.
//
// The dialect that puts a builtin's name in the location — `zsh:cd:1:` —
// leaves it out when the message comes from machinery the builtin merely
// asked for, and one command can write both kinds. Measured:
// `zmodload zsh/nosuch` is `<file>:1: failed to load module …` with no
// `zmodload:` in the location, while the same builtin's bad option is
// `<file>:zmodload:1: bad option: -Q`. The module loader speaks in the first
// and the builtin in the second, and the location says which.
//
// Here rather than at the call site because the builtin's name is this
// package's to know: a dialect cannot reach it, and a registered builtin that
// wrote its own prefix would be spelling a rule that already exists.
func (r *Runner) DiagnoseAsTheShellf(format string, args ...any) {
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	r.diagf(format, args...)
}

// DynamicParameter reports whether a name is a parameter this shell
// *produces* — one whose value is generated when it is read, registered
// through SetDynamic or SetDynamicArray — rather than one a script assigned.
//
// For a builtin that has to answer whether the shell provides a parameter
// as a feature, which is a different question from whether a variable of
// that name happens to hold something: a script's own `options=(a b)` must
// not make this shell look as though it had zsh's `$options`.
func (r *Runner) DynamicParameter(name string) bool {
	if _, ok := r.Dynamic[name]; ok {
		return true
	}
	if _, ok := r.DynamicArrays[name]; ok {
		return true
	}
	_, ok := r.DynamicAssocs[name]
	return ok
}

func (r *Runner) SetVar(name, value string) { r.setVar(name, value) }

// GetVar reads a shell variable, falling back to the environment.
func (r *Runner) GetVar(name string) (string, bool) { return r.getVar(name) }

// SetArray sets an indexed array from a list, and GetArray reads one back.
//
// The pair is here because a registered builtin whose answer is *several*
// words has nowhere else to put it: SetVar would join them, which is the one
// thing an array exists not to do. `read -a` needs none of this because it is
// the core's own, and the gap only shows up from outside.
//
// Reaching into the Arrays map instead is what a caller does without them,
// and it is wrong in a way that is invisible until a dialect changes: the
// first element answers to subscript 1 in one shell and 0 in another, so a
// list written in at 0 reads back short — or empty — under the other answer.
// The base is this package's to know, and these two ask it.
func (r *Runner) SetArray(name string, values []string) { r.setArray(name, values) }

// GetArray is the list an indexed array holds, and whether there is one.
func (r *Runner) GetArray(name string) ([]string, bool) { return r.arrayElems(name) }

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
//
// Every function *callable*, including the ones a dialect's prelude defined,
// which is what an embedder completing a command word wants: the line editor
// builds its command list from this and [Runner.BuiltinNames], and `pushd`
// is in neither if this one narrows. A *listing* of "what functions exist"
// wants the other set — the script's own, with the shell's left out — and
// that is scriptFuncNames, which every listing and `compgen -A function` ask
// instead (#1035, #1081). Two callers want two sets; the predicate behind
// both is Runner.speaksForTheShell, so there is still one notion of whose a
// function is.
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

// LookPathAll is every PATH entry a name resolves to, in PATH order and
// duplicates included, for a builtin whose job is to list them all rather than
// to pick one.
func (r *Runner) LookPathAll(name string) []string { return r.lookPathAll(name) }

// NameKind is what a shell would run for a word: the resolution itself, with
// no wording attached to it.
//
// The two are separated because they vary independently. Every shell in the
// panel resolves a name the same way and none of them says so in the same
// words — one calls a builtin `a shell builtin`, another `builtin`, another
// `shell built-in command`, and one prints the name back bare — so a dialect
// with a builtin of its own asking this question needs the resolution and not
// a sentence. Without it, a dialect that words the answer differently has to
// redo the lookup, and a redone lookup is a lookup that can disagree.
//
// Aliases are deliberately absent. Whether a word expands as one is the
// parser's fact rather than the runner's — see syntax.Dialect.ExpandAliases —
// so a builtin that speaks for aliases asks [Runner.LookupAlias] first and
// this afterwards, exactly as `type` never names one.
type NameKind uint8

const (
	// NameNotFound is a word this shell would not run at all.
	NameNotFound NameKind = iota
	// NameFunction is a function this shell has defined.
	NameFunction
	// NameBuiltin is a builtin, registered or the core's.
	NameBuiltin
	// NameReserved is a word the grammar owns — `if`, `while`, `[[`.
	NameReserved
	// NameFile is an executable found through PATH.
	NameFile
)

func (k NameKind) String() string {
	switch k {
	case NameFunction:
		return "function"
	case NameBuiltin:
		return "builtin"
	case NameReserved:
		return "reserved"
	case NameFile:
		return "file"
	}
	return "not found"
}

// ResolveName reports what this shell would run for name, and for a file the
// path it would run. Everything else has no path, and the empty string says so.
//
// The order is the resolution's own — a function, then a builtin, then a
// reserved word, then PATH — which is the order `type` and `command -v`
// already answer in, from the same lookup rather than from a copy of it.
func (r *Runner) ResolveName(name string) (NameKind, string) {
	if _, ok := r.funcs[name]; ok {
		return NameFunction, ""
	}
	if _, ok := r.lookupBuiltin(name); ok {
		return NameBuiltin, ""
	}
	if reservedWord(name) {
		return NameReserved, ""
	}
	if r.reservedBuiltin(name) {
		// A name this shell must answer itself is never resolved from PATH,
		// even where the builtin is missing — see reserved.go. Reporting the
		// file would say the shell would run it, which is exactly what it
		// refuses to do.
		return NameNotFound, ""
	}
	if path, ok := r.LookPath(name); ok {
		return NameFile, path
	}
	return NameNotFound, ""
}

// DefineFunction gives a name a body written as text, parsed with this
// shell's own grammar.
//
// The write half of FunctionText, and the seam an *autoloading* builtin
// needs: zsh's `autoload` makes a file's contents the body of a function
// named after the file, which is a definition arriving from outside the
// script rather than from a `f() { … }` the parser already read.
//
// The text is the body without its braces — what a file on `$fpath` holds —
// and the grammar is this runner's, because what that text means here is
// what this shell says it means. That is the same rule the environment's
// imported functions follow, and this is deliberately the same three steps
// they take.
//
// The result reports whether the text was a body this shell could read. A
// caller that gets false has been handed something that is not a function,
// and saying so is its business — this does not write a word.
func (r *Runner) DefineFunction(name, body string) bool {
	f, err := syntax.Parse(name+"() {\n"+body+"\n}", r.dialect())
	if err != nil || len(f.Stmts) != 1 {
		return false
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		return false
	}
	decl, ok := pipe.Cmds[0].(*syntax.FuncDecl)
	if !ok {
		// Unreachable by construction: the text this builds is a definition,
		// so a parse that got this far produced one. Kept because storing a
		// nil declaration would turn a bad body into a crash on the next
		// call rather than a false here, and a mutant that returns true
		// instead survives for exactly that reason — there is no body that
		// reaches it.
		return false
	}
	if r.funcs == nil {
		r.funcs = map[string]*syntax.FuncDecl{}
	}
	r.funcs[name] = decl
	return true
}

// FunctionText is a function's definition written back the way this shell
// prints one, for a builtin that shows a body rather than naming it.
func (r *Runner) FunctionText(name string) (string, bool) {
	fn, ok := r.funcs[name]
	if !ok {
		return "", false
	}
	return r.listedFunction(name, fn), true
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

// SystemDescriptor is the *operating system's* descriptor for one of this
// shell's, and whether the thing open at that number really is one.
//
// The two numbers are not the same number, and that is the whole reason this
// exists. A shell models its own descriptor table — `exec {FD}< file` picks a
// number out of the shell's table and puts an open file at it, and the file's
// own descriptor is whatever the kernel happened to give it. Everything inside
// this package reaches through the table by the shell's number and never needs
// the other one; a front end that has to *wait* on a descriptor does, because
// only the kernel can be asked whether one is readable.
//
// It is a question and not a handle: the number is what the caller may pass to
// a system call, and the file behind it stays this shell's to close. A number
// nothing is open at, and one held by something that is not a descriptor at
// all — a here-document's text, an embedder's in-memory buffer — is not one,
// which is the honest answer rather than a guess, and a caller that gets false
// has learned that waiting on it is not possible.
//
// The named three answer for themselves, because a shell can be asked to watch
// its own standard input and in a session that is the terminal.
func (r *Runner) SystemDescriptor(fd int) (int, bool) {
	var held any
	switch fd {
	case 0:
		held = r.stdin()
	case 1:
		held = r.stdout()
	case 2:
		held = r.stderr()
	default:
		var open bool
		if held, open = r.fds[fd]; !open {
			return 0, false
		}
	}
	f, ok := held.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	// An *os.File that has been closed reports ^uintptr(0), which as a signed
	// number is -1: the file object is still there and the descriptor behind
	// it is not.
	n := int(f.Fd())
	if n < 0 {
		return 0, false
	}
	return n, true
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

// ListedOptions is every row a `set -o` listing would write, in the order it
// writes them: the dialect's own table where it installed one with
// [Runner.SetOptionTable], and the substrate's names with their live states
// otherwise.
//
// The read half of SetOptionTable, and the plural of [Runner.NamedOption].
// Exported for a registered builtin that presents the same rows under its own
// spelling and has to *narrow* them — bash's `shopt -o -s`, which writes the
// options that are on and no others. A builtin cannot ask `set` for that: the
// listing arrives already formatted, so filtering it would mean parsing text
// this package just printed.
//
// It returns the same rows [Runner.listOptions] iterates, rather than
// rebuilding them, so a caller and `set -o` itself can never disagree about
// which names exist or what they say.
func (r *Runner) ListedOptions() []ListedOption { return r.listedOptions() }

// SetPromptUser names the user the `%n` prompt escape reports.
//
// Carried in rather than read here, for the rule the package comment states:
// the login name for a uid comes from the system, and a Runner embedded in
// another program must not go asking. The shell binaries fill it in beside
// the `$UID` they already read.
//
// It is deliberately *not* a variable, which is measured rather than assumed.
// `%n` ignores `$USER`, `$LOGNAME` and `$USERNAME` — set inside the shell or
// injected into its environment before it starts, in both `${(%)…}` and a
// prompt, in zsh and in bash's `\u` alike. Reading one of those names would
// have made `env USER=someone-else zsh` draw the wrong person, which is the
// case that says this is a fact about the process and not about the script.
func (r *Runner) SetPromptUser(name string) { r.promptUser = func() string { return name } }

// SetPromptUserFunc is SetPromptUser with the asking deferred to the moment
// something draws `%n`, and is the form a shell binary should prefer.
//
// The rule SetPromptUser states is unchanged — this package still does not ask
// the system anything, it calls back what the caller handed it. What changes is
// *when*. Measured on darwin, `os/user.Current` costs 0.83-1.10 ms with cgo
// and 0.81-1.29 ms without, because it is Directory Services either way; asked
// eagerly it was ~25% of a `zsh -c ':'`, spent on an escape that route cannot
// draw (#1403).
//
// The answer is asked for once and kept, because a prompt is redrawn on every
// keystroke that redraws the line and a login name does not change while a
// shell runs. A function that answers empty is refused exactly as
// SetPromptUser("") is: told, and with no answer, is not the same as not told,
// but both are wrong to draw.
func (r *Runner) SetPromptUserFunc(ask func() string) { r.promptUser = sync.OnceValue(ask) }

// SetPromptHost names the machine the `%m` and `%M` prompt escapes report.
//
// Carried in for the reason SetPromptUser is: the host name comes from the
// system, and a Runner embedded in another program must not go asking. The
// shell binaries fill it in beside the login name they already read.
//
// A runner nobody told refuses both escapes by name rather than drawing an
// empty host, which would be a prompt describing no machine at status 0.
func (r *Runner) SetPromptHost(name string) { r.promptHost = func() string { return name } }

// SetPromptHostFunc is SetPromptHost with the asking deferred to the moment
// something draws `%m` or `%M`, and is the form a shell binary should prefer.
//
// The same shape as SetPromptUserFunc and installed beside it, so that the two
// facts a prompt wants about the machine are carried in the same way. The host
// name itself is cheap — `os.Hostname` measures 3.9 µs — and this exists so
// that the pair does not have one eager half and one lazy one, which is the
// kind of asymmetry a later reader has to re-derive.
func (r *Runner) SetPromptHostFunc(ask func() string) { r.promptHost = sync.OnceValue(ask) }

// SetPromptStyle installs the prompt-escape table this dialect spells.
//
// The same value the prompt drawer is given — `repl.PromptStyle` is an alias
// for [PromptStyle], not a copy — so a dialect fills in one table and both
// readers read it. Before this there were two, and `print -P '%F{196}…'` was
// refused by name while a drawn prompt answered it (#1090).
//
// A runner nobody told has no escape character, and therefore no escape
// language: `${(%)v}` is the value as it stands. That is the substrate's own
// answer rather than a borrowed one, and it is reachable only through this
// package's own API — the one dialect whose grammar reads the `%` flag at all
// supplies the table in its Apply.
func (r *Runner) SetPromptStyle(st PromptStyle) { r.promptStyle = st }

// PromptField is what this runner can answer about one prompt code, for a
// prompt drawer that has a Runner and wants the interpreter's answer rather
// than one of its own.
//
// The drawer answers the session's facts itself — its history number, its
// terminal, what the parser is still inside — and reaches for this where the
// answer belongs to the interpreter, so that a code written *in* a prompt and
// the same code expanded by a script agree. The second result is false where
// this runner has no answer, which is the by-name refusal's condition.
func (r *Runner) PromptField(f PromptField, arg string, braced bool) (string, bool) {
	return r.promptField(f, arg, braced)
}

// PromptStyleValue is the table this runner was given, for a caller that has
// to hand the same one to a prompt drawer.
//
// Exported so that "the drawer reads the same table" can be *asserted* rather
// than arranged: a test can compare what the front end gave the editor with
// what the interpreter answers from. See driver.
func (r *Runner) PromptStyleValue() PromptStyle { return r.promptStyle }

// SetOptionNamespace installs the names `[[ -o name ]]` reads, for a dialect
// whose option namespace is wider than the `set -o` names it declares.
//
// Two namespaces rather than one because the panel has two. bash and ksh93
// test the same names `set -o` takes, so their `[[ -o ]]` needs nothing here
// and reads NamedOption. zsh's is its own: about a hundred and eighty names
// where `set -o` shows a couple of dozen, folded case-insensitively, with
// underscores ignored and a single `no` prefix negating what follows — which
// is why the lookup is a function the dialect supplies rather than a longer
// list this package could hold. Measured: `[[ -o Err_Exit ]]` and
// `[[ -o errexit ]]` are one question in zsh and two unknown names in bash.
//
// The second result is what decides between a plain false and the dialect's
// complaint, so a namespace that guesses `true, false` for a name it has
// never heard of would turn every typo into a silent no. It answers about
// names, never about states.
func (r *Runner) SetOptionNamespace(lookup func(name string) (on, known bool)) {
	r.optionNamespace = lookup
}

// ListedOption is one row of a dialect's own `set -o` listing: the spelling
// this shell prints for the name, and whether that spelling is in effect.
//
// The spelling is the dialect's and is not derived from the name here,
// because it is not always the name. zsh prints each option in the direction
// that is *off* by default — `noclobber` for one that defaults on, `autocd`
// for one that defaults off — so a row is `noclobber` with On false in a
// shell that clobbers, and the same shell's `set -o noclobber` makes it true.
type ListedOption struct {
	Name string
	On   bool
}

// SetOptionTable installs the `set -o` namespace of a dialect that has one of
// its own: the rows `set -o` and `set +o` write, and what moving one name
// does.
//
// It is [Runner.SetOptionNamespace] reached by the other route, and a dialect
// that installs one installs both — measured, `set -o Err_Exit`,
// `setopt err_exit` and `[[ -o err_exit ]]` are one namespace in zsh, where
// its `set -o` writes 185 rows against the couple of dozen names every shell
// shares. Ours wrote the shared two dozen in that shell, so `set +o` — a
// capture surface, one of the three sections a harness snapshots — recorded a
// zsh with 23 options and said nothing about the 170 that decide what it does
// (#1080).
//
// move reports two things and neither is a status. `known` false is a name
// this shell does not have, which is refused in the dialect's own words on
// whichever of the three routes asked — a script's `set -o`, an invocation's,
// or an inherited value — and that wording, its status and whether it ends
// the script are already Diagnostics and Semantics values. `moved` false with
// `known` true is a name the shell has and will not move, which is
// Diagnostics.SetImmovableOptionName. So the mover is **silent**: it decides,
// and this package speaks, which is the only way one refusal can be worded
// three ways by route.
//
// It does not replace [Runner.ApplyNamedOption], which stays on the
// substrate's own table on purpose: that is the seam a dialect's option
// builtin uses to move a substrate option by its substrate name, so routing
// it here as well would have zsh's `setopt err_exit` call back into the table
// it is being called from.
func (r *Runner) SetOptionTable(listed func() []ListedOption, move func(name string, on bool) (moved, known bool)) {
	r.optionListing, r.optionMover = listed, move
}

// DialectOption reads one option name through this shell's own option
// namespace: the names `setopt` and `[[ -o ]]` take where a dialect has
// installed one, and the `set -o` names where it has not.
//
// For a front end holding a *setting* a shell spells as an option rather than
// as a variable — zsh's HIST_IGNORE_SPACE is the case this was added for. Its
// sibling MatchPattern exists for the same reason and answers the same shape
// of question: the settings a session reads are the shell's own, and a front
// end that brought its own answer would be a second shell disagreeing with
// the first about what it was told.
//
// The name is asked for exactly as the caller spells it, because folding is
// the namespace's business and not the caller's: `HIST_IGNORE_SPACE`,
// `hist_ignore_space` and `histignorespace` are one question in zsh and three
// unknown names in a dialect that installed nothing.
//
// The second result says whether this shell has the name at all, so a caller
// can tell a name that is off from a name nobody has — which is the whole
// difference between a knob turned down and a knob that is not there.
func (r *Runner) DialectOption(name string) (on, known bool) {
	return r.conditionOption(name)
}

// conditionOption reads one option name the way `[[ -o ]]` asks for it:
// through the dialect's namespace where it has one, and through the `set -o`
// names otherwise.
func (r *Runner) conditionOption(name string) (on, known bool) {
	if r.optionNamespace != nil {
		return r.optionNamespace(name)
	}
	return r.NamedOption(name)
}

// MatchPattern reports whether a shell pattern matches a whole string, by this
// shell's own pattern rules.
//
// For a caller holding a *setting* written as a pattern rather than a pattern
// found in a script — a list of command lines not to record in the history is
// the case this was added for. Without it such a caller would have to bring
// its own matcher, and a shell whose `case` and whose settings disagreed about
// what `@(a|b)` means would be one thing pretending to be two.
//
// Anchored at both ends, which is what a pattern means everywhere in a shell
// except inside `[[ =~ ]]`: `pwd` matches the line `pwd` and not `pwd /tmp`.
// Not the condition-context reading, since a setting is not a condition.
func (r *Runner) MatchPattern(pattern, s string) bool {
	return r.matchPatternR(pattern, s, false)
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

// SetAssoc sets an associative array from a name-to-value table, giving the
// name the associative attribute the way `typeset -A` does.
//
// The pair with GetAssoc is here for the same reason SetArray and GetArray
// are, one shape further along: a registered builtin whose answer is a set of
// *named* values has nowhere to put it. SetArray would lose the names and
// reaching into the AssocArrays map directly skips the attribute, which is the
// part that decides how a subscript is read — a table written in without it
// answers `${m[1+1]}` as an arithmetic index rather than as a key, and the
// difference is invisible until a script uses a key that looks like a sum.
//
// A nil or empty table still declares the name, because an empty associative
// array and an absent one are different things to `${(k)m}` and to
// `${m[k]:-d}`.
func (r *Runner) SetAssoc(name string, values map[string]string) {
	r.markAssoc(name)
	table := make(AssocArray, len(values))
	for k, v := range values {
		table[k] = v
	}
	r.AssocArrays[name] = table
}

// GetAssoc is the table an associative array holds, and whether there is one.
//
// A copy rather than the runner's own map, so a caller that reads a table,
// adds to it and writes it back cannot change the shell's state halfway
// through deciding what to write.
func (r *Runner) GetAssoc(name string) (map[string]string, bool) {
	a, ok := r.AssocArrays[name]
	if !ok || r.removed[name] {
		return nil, false
	}
	out := make(map[string]string, len(a))
	for k, v := range a {
		out[k] = v
	}
	return out, true
}

// ArithValue is the value of an arithmetic expression, for a builtin holding
// an expression it did not parse.
//
// The sibling of Expand and MatchPattern, and it exists for the same reason:
// a builtin that is handed `1+1` and has to compare it with a number would
// otherwise have to bring its own evaluator, and a shell whose `$(( ))` and
// whose builtins disagreed about what an expression means would be one thing
// pretending to be two.
//
// A word that is not a number is **not** a failure — a shell reads a bare
// name in arithmetic as the variable's value and an unset one as zero, so
// `x` is 0 and this returns it. An expression that will not *evaluate* is a
// different thing, and it is reported and made fatal here exactly as
// `$(( ))`'s is: `echo $((1/0))` stops a script in every shell in the panel,
// and a builtin that swallowed the same division and carried on with a zero
// would hand its caller a plausible answer to a question that failed. The
// second result says it happened, so a caller can stop before writing
// anything.
//
// The complaint is located as the shell rather than as the builtin, which is
// what the arithmetic machinery does everywhere else: the expression failed,
// not the command that held it.
func (r *Runner) ArithValue(text string) (int, bool) {
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	tree, err := r.arithTree(nil, text)
	if err != nil {
		r.diagf("%s\n", r.diag().ParseFailure(err))
		r.fatalQuiet()
		return 0, false
	}
	n, err := r.evalNum(tree)
	if err != nil {
		r.diagf("%s\n", r.arithFailure(text, err))
		r.fatalQuiet()
		return 0, false
	}
	// An integer context, which is what a comparison with a test number is:
	// a float truncates, exactly as an array subscript does.
	return n.asInt(), true
}
