// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/opened"
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
	if r.withdrawnBuiltins[name] {
		// A module selection has taken the name out of the table: it is not
		// a builtin at all until the selection puts it back, which is a
		// different thing from `enable -n` below — see withdrawnbuiltin.go.
		return nil, false
	}
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

// DiagnoseAsf is the third of the same three: a complaint from machinery the
// builtin asked for that has a *name* of its own, which is neither the
// builtin's nor the bare shell's.
//
// One command writes all three kinds. Measured on zsh 5.9.2, 2026-09-11,
// re-selecting a module parameter over a name a script has since assigned:
//
//	<file>:zmodload:4:        bad option: -Q             the builtin
//	<file>:4:                 Can't add module parameter …   the shell
//	<file>:zsh/parameter:4:   error when adding parameter …  the module
//
// The name goes where the dialect puts a builtin's — in the location for the
// shell that words it that way, at the front of the sentence for the shells
// that do not — so a caller writes it as a prefix on the message exactly as
// it would write a builtin's, and this decides where it lands.
func (r *Runner) DiagnoseAsf(name, format string, args ...any) {
	outer := r.inBuiltin
	r.inBuiltin = name
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

// InSubshell reports whether this runner stands for a body a real shell would
// have run in a process of its own: a subshell, a command substitution, a
// process substitution, a pipeline element, or a background job.
//
// It is the seam for the corollary in AGENTS.md — where a real shell relies on
// process boundaries, this one has to reconstruct the boundary by hand. Those
// bodies are cloned Runners on goroutines of one process here, so anything
// that reports *which process is running this* has a true answer that carries
// a false claim: the body reads it as "me, and not the shell", and it is the
// shell. A parameter whose value is a process id is the sharp case, because a
// body that believes it has its own process also believes it leads its own
// process group, and a teardown written `kill -- -$mypid` then reaches the
// interactive shell that started it (#2046).
//
// So a dialect asks this where a value it produces would be read as an
// identity rather than as a number. It is not job control and not a promise
// about isolation — the clone shares the process either way, and this only
// says that the script thinks otherwise.
func (r *Runner) InSubshell() bool { return r.inSubshell }

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
	// A withdrawn one is not either, and it is out for a stronger reason: a
	// module selection took the name out of the table altogether, so it is
	// not a builtin at all until the selection puts it back. Measured on zsh
	// 5.9.2, 2026-09-12, after `zmodload -F zsh/zutil -b:zparseopts`, the
	// name is absent from `enable`'s listing and from `$builtins` alike —
	// and absent from `disable`'s too, which is the whole difference between
	// this state and `enable -n` (see withdrawnbuiltin.go).
	for name := range r.withdrawnBuiltins {
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
	if _, ok := r.reportedFunc(name); ok {
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
	return r.defineFromText(name, body, nil)
}

// DefineFunctionExpandingAliases is DefineFunction with the alias table
// offered to the parse, so a word the table holds is replaced while the body
// is read.
//
// **The caller has already decided**, and this expands from the table without
// asking anything else. That is deliberate: the switch [Runner.ExpandingAlias]
// consults is the *route the program arrived by*, and text read here is not
// that route — it is a file of its own. Measured 2026-09-11 on zsh 5.9.2 with
// `env -i` and `-f`, a function file holding the one word `myalias`, and
// `alias myalias='print -r -- X'`:
//
//	zsh -c 'alias …; autoload af; af'   X
//	zsh -c 'alias hi=…; hi'             command not found: hi
//
// The same invocation expands the word in the file and not the word in its own
// command string, so a route answer cannot be borrowed for this. What the
// dialect asks instead is its own option — `unsetopt aliases` leaves the file
// unexpanded on both routes — and whatever letter the declaration carried.
//
// Two methods rather than a bool on one, because a bool at a call site says
// nothing about which way round it runs, and the letter that reaches this one
// runs the opposite way from its own name: zsh's `-U` *suppresses* expansion.
func (r *Runner) DefineFunctionExpandingAliases(name, body string) bool {
	return r.defineFromText(name, body, r.LookupAlias)
}

// defineFromText is the whole of both of these and of
// [Runner.DefineFunctionFromText] beside them.
//
// One function because three ways of reading a body out of text is three
// places for the next fix to reach two of — and that is not hypothetical: the
// two that existed differed only in the spelling of the wrapper they built,
// `name() {` against `name () {`, which is the same definition to this grammar
// and was never a difference anybody chose. The alias fix (#1993) had to land
// in both.
//
// aliases is the parser's hook, and nil expands nothing.
func (r *Runner) defineFromText(name, body string, aliases syntax.Aliases) bool {
	p := syntax.NewParser(name+"() {\n"+body+"\n}\n", r.dialect())
	if aliases != nil {
		// All three kinds together, because the file is read by the same
		// rules a script is: measured, a function file read by `autoload`
		// without `-U` has its global aliases expanded and its suffix
		// aliases applied exactly as a line of the script would. A body
		// read *with* `-U` has none of them, which is the nil below.
		//
		// Not [Runner.ExpandAliasesIn], deliberately: this route asks the
		// *tables* and not the option, because the letter that reaches it
		// runs the opposite way from the switch — see the two methods
		// above. Which is why `aliases` is still a parameter.
		p.GlobalAliases = r.LookupGlobalAlias
		p.SuffixAliases = r.LookupSuffixAlias
	}
	p.Aliases = aliases
	f := p.Parse()
	if p.Err() != nil || len(f.Stmts) != 1 {
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
	// The same bookkeeping a definition the parser read gets. Without it a
	// function defined through this seam had no origin at all, so the one
	// route that most needs one — a file on the function search path, which
	// is the whole reason this seam exists — was the one that named nothing
	// (#1706). A caller that knows a better file than the current one says
	// so with [Runner.SetFunctionFile] straight after.
	r.recordFunctionFile(name, r.currentFile())
	return true
}

// SetFunctionFile says which file a function was defined in, for a builtin
// that read the body out of one.
//
// The definition seams above cannot know it: they are handed text, and the
// file it came from is the caller's fact. An empty path clears the entry
// rather than storing one, so a caller with nothing to say leaves the name
// where the definition put it.
func (r *Runner) SetFunctionFile(name, file string) {
	r.recordFunctionFile(name, file)
}

// recordFunctionFile is the one write to the table, so that a new way of
// defining a function cannot quietly skip it.
func (r *Runner) recordFunctionFile(name, file string) {
	if file == "" {
		delete(r.funcFiles, name)
		return
	}
	if r.funcFiles == nil {
		r.funcFiles = map[string]string{}
	}
	r.funcFiles[name] = file
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

// SetUndefinedFunctions installs this shell's notion of a function whose body
// has not been read yet: whether a name is one, and the text a listing writes
// in its place.
//
// Two of the four shells have the notion and each renders it its own way, so
// it is a dialect's answer rather than something this package could hold.
// Measured 2026-09-10 against zsh 5.9.2 and ksh93u+, with `myfn` on the
// search path and never called:
//
//	zsh    functions myfn      myfn () {\n\t# undefined\n\tbuiltin autoload -XUz\n}
//	zsh    whence -v myfn      myfn is an autoload shell function
//	ksh93  typeset -f myfn     typeset -fu myfn
//	ksh93  whence -v myfn      myfn is an undefined function
//
// zsh keeps the header and the braces and writes two lines between them; ksh93
// writes a declaration and no body at all. So what is shared is the *question*
// — is this name still undefined — and neither what stands in the body nor the
// sentence is, which is why the hook answers with the one and
// [Diagnostics.TypeUndefinedFunction] with the other.
//
// The text is the *block*, braces included, exactly as syntax.PrintWith would
// have returned it for a real body — so the header, and the quoting a name
// needs in it, stay in the one place that writes them. ksh93's shape, which
// has no header and no braces at all, would need more than this; it has no
// `typeset -f` listing in this implementation to reach here, and the hook is
// deliberately not widened for a caller that does not exist.
//
// The marker is not a comment in the body, which is measured rather than
// assumed: a function written by hand with `# undefined` as its first line
// lists *without* it in zsh, because a listing is printed from the tree and
// the tree has no comments in it. `# undefined` is the shell saying what the
// function is, and that is why it cannot be arranged by giving the stub a
// cleverer body.
//
// The text is what [Runner.FunctionText], `typeset -f` and `functions` write.
// It deliberately does not reach the *names-only* forms, which name a name
// whatever state it is in, nor `$functions`-style body views, which one shell
// answers with a third text again — see the dialect's own association.
func (r *Runner) SetUndefinedFunctions(undefined func(name string) (string, bool)) {
	r.undefinedFunctions = undefined
}

// undefinedFunction is that hook asked, for a shell that installed one.
func (r *Runner) undefinedFunction(name string) (string, bool) {
	if r.undefinedFunctions == nil {
		return "", false
	}
	return r.undefinedFunctions(name)
}

// SetFunctionMarkedUndefined is the write half of [Runner.SetUndefinedFunctions]:
// how this shell *makes* a name one that is still to be defined.
//
// A declaration builtin reaches it, because the two shells with the notion
// spell it there as well as under their own word: `typeset -f` written with
// the letters [Semantics.FunctionLettersThatMarkUndefined] names is not a
// listing at all, it is `autoload` under a second name. Measured 2026-09-12
// on zsh 5.9.2, `typeset -fu nm` and `autoload nm` leave the identical stub,
// and `typeset -fUz nm` matches `autoload -Uz nm`.
//
// The hook is handed the operands and **the letters the line carried**, minus
// the `f` that got it here, in the order they were written. Which of them
// mean anything is the dialect's business and not this package's: one shell
// records `U` and `z` on the stub it writes, and a shell that spelled the
// same letters differently would map them here. This is deliberately not a
// pre-digested set of options — an option struct crossing the seam would be
// one shell's vocabulary in the substrate.
//
// The status is the hook's, because marking can fail: a name that is not a
// name has nowhere to put the stub.
func (r *Runner) SetFunctionMarkedUndefined(mark func(r *Runner, names []string, letters string) int) {
	r.markUndefinedFunctions = mark
}

// SetMarkedFunctions is the third half of that seam, and the one a *listing*
// needs: given the marking letters a line carried, the names that hold them.
//
// A `-f` listing with no operands is not the whole function table when it
// carries one of [Semantics.FunctionLettersThatMarkUndefined] — it is the
// functions holding that mark, which in the shell that has the notion is the
// same set a bare `autoload` writes out. Measured 2026-09-12 on zsh 5.9.2,
// with `f1` autoloaded plainly, `f2` autoloaded with `-U` and `g` an ordinary
// function:
//
//	functions -u   f1 and f2      typeset -fu   f1 and f2, byte for byte
//	functions -U   f2 alone       typeset -fU   f2 alone
//	functions -uU  f1 and f2      functions     all three
//
// So the letters are a **union** and not an intersection — `-uU` is `-u`'s
// answer and not `-U`'s — and the population is the dialect's to name,
// because what a mark *is* here is a stub this engine cannot read the letters
// out of. Handing the letters over rather than a digested question is the
// same choice [Runner.SetFunctionMarkedUndefined] makes and for the same
// reason: a set of options crossing the seam would be one shell's vocabulary
// in the substrate.
//
// The names come back in the order the listing should write them, since
// ordering a listing is the same shell's business as choosing it. Nil, or a
// hook returning nothing, leaves the listing exactly as wide as it was.
func (r *Runner) SetMarkedFunctions(marked func(r *Runner, letters string) []string) {
	r.markedFunctions = marked
}

// markedFunctionListing is that hook asked for a line's letters: the names a
// narrowed listing writes, and whether there was a narrowing at all.
//
// Both halves are needed and an empty slice is not the second: a dialect that
// has the notion and holds no marked functions must write *nothing*, where a
// dialect without the notion writes the whole table.
func (r *Runner) markedFunctionListing(f declareFlags) (names []string, narrowed, namesOnly bool) {
	letters, plus := f.markingLettersWritten(r.sem().FunctionLettersThatMarkUndefined)
	if letters == "" || r.markedFunctions == nil {
		return nil, false, false
	}
	return r.markedFunctions(r, letters), true, plus
}

// markingLetters reports the letters a `-f` line carried that this dialect
// says turn it into a marking, and whether there is a hook to do it. Both
// halves, because a dialect that names the letters and installs no hook has
// said something this package cannot act on, and a listing is the safer of
// the two readings.
func (r *Runner) markingLetters(letters string) bool {
	if r.markUndefinedFunctions == nil {
		return false
	}
	return strings.ContainsAny(letters, r.sem().FunctionLettersThatMarkUndefined)
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

// ReaderForFd is the reading half of [Runner.WriterForFd]: the stream a
// builtin reading "from descriptor n" needs, the named one by its number and
// anything past the three from the shell's own table.
//
// A builtin that took [Runner.SystemDescriptor] and read the number itself
// would be doing something subtly different and worse. The runtime may hold a
// pipe or a fifo non-blocking behind its own poller, so a bare read on the
// number can come back EAGAIN where a read on the stream would have waited —
// which would turn `sysread` on a process substitution into a builtin that
// failed at random. The stream knows; the number does not.
func (r *Runner) ReaderForFd(fd int) (io.Reader, bool) {
	if fd == 0 {
		return r.stdin(), true
	}
	if v, held := r.fds[fd]; held {
		in, ok := v.(io.Reader)
		return in, ok
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

// DescriptorOffset is where the file open at one of this shell's descriptors
// is positioned, counted in bytes from its start, and whether the question had
// an answer at all.
//
// The seam under a math function one shell's `zsh/system` module spells
// `systell`. It is [Runner.SystemDescriptor] plus the one system call that
// reports a position without moving it, kept here rather than in the dialect
// for the reason every platform-shaped fact in this tree sits below the
// dialects: a dialect that reached for a system call would be a dialect that
// had to be written twice for the platforms that lack it.
//
// False is a number nothing is open at, a number holding something that is not
// a descriptor — a here-document's text, an embedder's in-memory buffer — and
// a stream that has no position, which is what a pipe and a terminal are. The
// caller says what its own vocabulary calls that; here they are one answer,
// because in every one of them there is no offset to report.
func (r *Runner) DescriptorOffset(fd int) (int64, bool) {
	sys, ok := r.SystemDescriptor(fd)
	if !ok {
		return 0, false
	}
	return seekCurrent(sys)
}

// SeekDescriptor moves where one of this shell's descriptors is reading or
// writing, and reports whether the move happened.
//
// The writing half of [Runner.DescriptorOffset], and it is here rather than in
// a dialect for the same reason that one is: a dialect reaching for a system
// call would be a dialect that had to be written twice for the platforms
// without it. whence is one of the [io] package's three seek origins.
//
// False is a number nothing is open at, a number holding something that is not
// a descriptor, a stream with no position — a pipe, a terminal — and a
// position the kernel refuses, such as one before the start of the file. A
// caller says what its own vocabulary calls those; here they are one answer,
// because in every one of them the seek did not happen.
func (r *Runner) SeekDescriptor(fd int, offset int64, whence int) bool {
	sys, ok := r.SystemDescriptor(fd)
	if !ok {
		return false
	}
	return seekTo(sys, offset, whence)
}

// OpenDescriptor puts a file this shell has *already opened* into its
// descriptor table and reports the number a script may reach it by, and
// SetDescriptor does the same at a number the script chose.
//
// The inward half of [Runner.SystemDescriptor], and needed for the same reason
// that one is: the shell's table and the kernel's are two tables. A registered
// builtin that opens something — a socket, a file at a number a script names —
// has an *os.File and no way to say "and this is descriptor 12 from here on",
// which is what makes `zsocket path; print -u $REPLY hello` and `sysopen -u 5
// f` expressible at all. Without it a builtin could open anything and hand
// back nothing a redirection could name.
//
// The file becomes the shell's: it is closed when the script writes `exec
// {n}>&-`, and it is handed to an external child the way every other entry in
// the table is, because it is an *os.File and that is the only question the
// child's descriptor rebuild asks. A caller that wants the descriptor to
// survive nothing should not put it here.
//
// SetDescriptor replaces whatever the number held without closing it, which is
// the same thing `exec 3>&4` does to a 3 that was already open. Numbers are the
// script's to reuse and the shell does not audit them.
func (r *Runner) OpenDescriptor(f *os.File) int {
	fd := r.nextFreeFd(-1)
	r.setFd(fd, f)
	return fd
}

// SetDescriptor puts a file at one of this shell's descriptor numbers. See
// [Runner.OpenDescriptor], which chooses the number instead.
func (r *Runner) SetDescriptor(fd int, f *os.File) { r.setFd(fd, f) }

// CloseDescriptor is the way back out of [Runner.OpenDescriptor]: the number
// stops being one of this shell's, and what was open at it is closed. The
// result says whether the number held anything at all.
//
// A registered builtin that hands a script a descriptor sometimes has to take
// it back — a lock the script says it is done with is the case this was
// written for — and `exec {n}>&-` is not available to it, because the number
// is one the builtin chose rather than one the script wrote down.
//
// It closes where the redirection route does not, and the difference is which
// of the two is the last word about the file. `>&-` drops the entry and leaves
// the open file to the runtime, because a script may be closing a number that
// was duplicated from another one still in use. A builtin calling this is
// saying the opposite: it opened this file, nothing else has it, and the thing
// it wants is for the *system* to be done with it — a lock is not given up
// until the descriptor holding it is gone.
//
// The three named descriptors are not the shell's to close and are not
// numbers [Runner.OpenDescriptor] ever returns, so they are refused here
// rather than routed to a close nothing could undo.
func (r *Runner) CloseDescriptor(fd int) bool {
	if fd < 3 {
		return false
	}
	held, open := r.fds[fd]
	if !open {
		return false
	}
	delete(r.fds, fd)
	delete(r.cloexecFds, fd)
	if c, ok := held.(io.Closer); ok {
		_ = c.Close()
	}
	return true
}

// KeepDescriptorFromChildren marks a descriptor already in the table as one
// that must **not** reach what this shell runs, which is what close-on-exec
// means to a script that asked for it by name.
//
// A mark rather than a different kind of table entry, because the descriptor
// is the script's in every other respect: it is written through, read from,
// closed with `exec {n}>&-` and renumbered like any other, and the one thing
// that differs is what an external command inherits. See childFiles, which is
// the only reader, and dropExecOpened beside it, which nils entries for a
// neighboring reason.
//
// It has to be a mark this package keeps rather than a flag on the open,
// because Go opens every file close-on-exec already: the kernel's answer is
// the same either way and the shell's table is what decides, since childFiles
// rebuilds the boundary by hand. So a builtin that passed O_CLOEXEC through
// and stopped there would have registered the letter and changed nothing —
// the descriptor would still be handed to the child by number.
//
// The mark comes off whenever the number is written again, because setFd is
// the one way a number acquires a new file and a new file was not the one
// marked. A number that is merely *closed* keeps a stale mark, which decides
// nothing: childFiles only reads the mark for a number the table still holds
// an [os.File] at.
func (r *Runner) KeepDescriptorFromChildren(fd int) {
	if r.cloexecFds == nil {
		r.cloexecFds = map[int]bool{}
	}
	r.cloexecFds[fd] = true
}

// SetDescriptorVariable puts a descriptor number in the variable a builtin was
// given the name of, by the route `exec {fd}<file` already takes.
//
// `sysopen -u fd file` and `exec {fd}<file` are the same act spelled twice, so
// they resolve the name the same way: a plain identifier, an array element, an
// association key. A builtin doing its own [Runner.SetVar] would answer the
// first and quietly invent a variable literally named `h[k]` for the third.
//
// [Runner.StoreThroughOperand] and not setFdVar, because this is the
// *builtin* side of the pair: measured, `sysopen -u 'h[1+]' -r f` is `bad
// math expression` and ends the script, where a redirection that has already
// opened its file has nowhere to put the number and says nothing. A
// subscripted `-u` reaches a string's characters through it too — measured,
// `s=abc; sysopen -u 's[2]' -r f` leaves the descriptor number spliced into
// `s` — which is the store's rule and not this call's.
func (r *Runner) SetDescriptorVariable(ref string, fd int) {
	r.StoreThroughOperand(ref, strconv.Itoa(fd))
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
// shell runs.
//
// A function that answers empty means *told, and the system had no name* — a
// uid with no password-database entry — which is not the same as not told and
// no longer draws the same thing. It draws PromptStyle.NoLoginName, which is
// the dialect's own word for that state; see FieldUser and #1451. Not telling
// this Runner at all is still the refusal.
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

// PromptQuantity is what this runner can count for one conditional escape,
// for a prompt drawer that has a Runner and wants the interpreter's answer
// rather than one of its own.
//
// The same split PromptField makes: the drawer counts what belongs to a
// session — what the parser is still inside, how deep an eval is — and reaches
// for this where the count belongs to the interpreter, so that `%(?.…)`
// written in a prompt and the same test written in a script agree. The second
// result is false where this runner has no answer, which is the by-name
// refusal's condition.
func (r *Runner) PromptQuantity(c PromptCondition, n int) (int, bool) {
	return r.promptQuantity(c, n)
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
//
// The lookup is handed the runner to answer *about*, the way a builtin is,
// and must read that one rather than the one it was installed on. A subshell
// is a cloned runner that keeps this field, so a closure over the installing
// runner answers for the shell that spawned the subshell instead of for the
// subshell — every option kind at once, which is what said the closure was
// the cause rather than any one entry's storage (#1855).
func (r *Runner) SetOptionNamespace(lookup func(r *Runner, name string) (on, known bool)) {
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
//
// Both halves are handed the runner to act on, for the reason
// [Runner.SetOptionNamespace] gives: a mover that closed over the installing
// runner would make `( set -o autocd )` set the option on the *parent* and
// leave the subshell without it, which is a wrong report turning into an
// escaped write (#1855).
func (r *Runner) SetOptionTable(listed func(r *Runner) []ListedOption, move func(r *Runner, name string, on bool) (moved, known bool)) {
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
		return r.optionNamespace(r, name)
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

// AssocElement is one element of a stored associative array, and whether
// there is one.
//
// The keyed half of GetAssoc, which copies the whole table — the wrong unit
// for a membership test or a single lookup. A dialect keeping a set of names
// in one paid a copy of the whole set to ask about a single name, and on a
// real startup that was a third of what the shell spent reading its rc file.
func (r *Runner) AssocElement(name, key string) (string, bool) {
	a, ok := r.AssocArrays[name]
	if !ok || r.removed[name] {
		return "", false
	}
	v, ok := a[key]
	return v, ok
}

// SetAssocElement puts one element into a stored associative array, creating
// the table if the name has none.
//
// The writing half, and the same saving again: SetAssoc rebuilds the table
// from a map the caller has just copied out of it, so adding one name to a
// set of A costs O(A) and filling the set costs O(A squared). Measured on a
// real startup, `autoload` marking fifteen hundred names that way was a
// fifth of the whole rc file.
//
// The write goes where a script's own `m[k]=v` goes, so a produced
// association's writer is honored rather than shadowed — which is the one
// way this differs from calling SetAssoc with one more key in the map.
func (r *Runner) SetAssocElement(name, key, value string) {
	r.markAssoc(name)
	r.setAssocElem(name, key, value)
}

// StoreThroughOperand assigns to a destination a builtin was handed **as a
// word**, which is not the same thing as a name.
//
// `read v`, `read 'a[2]'`, `read 'h[k]'` and `read "buf[$#buf+1]"` are one
// operand shape with four meanings, and which one applies is the language's
// question rather than the builtin's: an association takes the subscript as
// a key, an array takes it as an element, a name already holding a string
// takes it as a span of *characters* to splice, and a bare word is a plain
// assignment. A builtin that reached for SetVar instead got the last of
// those for all four.
//
// The complaint a bad subscript makes is the store's and is located there —
// measured, `read 'a[1/0]'` is `division by zero` and `read 'v[0]'` is
// `v: assignment to invalid subscript range`, the same two sentences the
// bare assignments give, with no builtin named in front of them.
//
// For a builtin whose operand is a destination: `read` uses it, and so does
// one dialect's `sysread`, which had to refuse a subscripted operand by name
// until this was reachable from outside the package.
func (r *Runner) StoreThroughOperand(name, value string) { r.storeThroughOperand(name, value) }

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
	tree, text, err := r.arithTreeOver(nil, text)
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

// AllowOpen asks the gate about a file a dialect's builtin is about to open,
// and is half of the seam a builtin needs to be inside the boundary.
//
// It exists because a builtin a dialect registers opens a file as much as a
// redirection does, and until #1805 nothing said so. `sysopen` and `zsystem
// flock` called os.OpenFile directly, so a policy that refused every read and
// every write still handed a script the whole filesystem — and left no record
// of it, because an open nobody consults is an open nobody reports either.
// The gate lives in this package and a dialect could not reach it, which is
// what made that possible rather than merely unnoticed.
//
// The refusal is reported to the script here, in the same words a refused
// redirection gets, and the caller decides the status: what a refused open
// means to `sysopen` is what an unopenable file means to it, which is the
// builtin's answer and not this package's.
//
// The returned Action must be handed to VerifyOpened once the descriptor is
// in hand. Consulting without verifying is the #943 defect with a new caller:
// the gate would have answered about a *name*, and a symlink under an allowed
// directory is a name that is allowed and an object that is not.
func (r *Runner) AllowOpen(ctx context.Context, path string, write bool) (Action, bool) {
	action := r.act(Action{Kind: ActionOpen, Path: path, Write: write})
	return action, r.allowed(ctx, action)
}

// VerifyOpened is the other half: it confirms that the descriptor a builtin
// has just opened is the object the gate allowed, and records the access.
//
// Asking again about where the name *went* is what makes the first answer
// mean anything, and it works on any descriptor because the question is put
// to the kernel rather than to the path — see verifyOpened and Action.Resolved.
// A builtin that opens with flags this package cannot express, as the
// nonblocking half of `sysopen` does, therefore keeps its own open and is
// still inside the boundary.
//
// False means the open is refused: the script has been told, the denial is on
// the record, and the caller closes the descriptor and fails the way it fails
// for a file it could not open.
func (r *Runner) VerifyOpened(ctx context.Context, a *Action, f *os.File) bool {
	// The name the *kernel* has for the descriptor, which is what makes this
	// answerable at all: opened.Reached carries the path a walk resolved, and
	// a builtin that did its own open has no walk to report. Without it
	// Elsewhere has nothing to compare, returns false, and this whole check
	// quietly passes everything — which is how the first version of this fix
	// still leaked a symlink under an allowed directory.
	//
	// A platform that cannot name a descriptor leaves it empty, and that is
	// the nameless case the package already has an answer for: no rule is
	// speaking about an object with no name.
	reached := opened.Reached{File: f}
	if name, ok := opened.Path(f); ok {
		reached.Name = name
	}
	if !r.verifyOpened(ctx, a, reached) {
		r.reportRefusal(*a)
		return false
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: *a})
	return true
}

// ReadFileGated reads a file a dialect's builtin was asked for, through the
// gate, the verification and the sink the interpreter's own reads use.
//
// It is the whole-file half of AllowOpen and VerifyOpened, and it exists for
// the same reason they do: `autoload` read a function's file with a bare
// os.ReadFile, so a script could point `$fpath` at a directory the policy
// refused and *run* whatever shell code was there — with nothing consulted and
// nothing recorded (#1812). That is `eval` on a file the policy named, which
// is the one thing docs/design/sandboxing.md claims a gate here can stop.
//
// A refusal comes back as "not there" rather than as a diagnostic of its own,
// and that is the information-hiding rule the rest of the sandbox follows: a
// name autoload may not read is a name that is not on `$fpath`, which is
// already the answer for a file the *kernel* withholds. A refusal that
// identified itself would be an oracle for what the policy hides.
//
// The context is the runner's own, as the probes in fsgate.go take theirs, and
// for the same reason: the callers are search and resolution paths several
// frames below any builtin that was handed one.
//
// A relative name is resolved against **this runner's** directory, through the
// same atDir `.` and a redirection go through. That is the PATH rule from
// AGENTS.md applied to a read: os.ReadFile below resolves a bare `fns/f`
// against the *process's* directory, which a Runner does not own and a second
// Runner in the same program does not share. It is reachable from a script —
// `cd d; fpath=(fns)` is a relative entry on zsh's function search path, which
// real zsh finds and this shell did not, because the shell's `cd` had moved
// r.Dir and the process had stayed where it started (#1968). Resolving here
// rather than in each caller is what keeps the gate honest as well: a policy
// asked about `fns/f` is being asked about a path that depends on a directory
// it cannot see.
func (r *Runner) ReadFileGated(path string) ([]byte, error) {
	path = r.atDir(path)
	notThere := func() ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.ENOENT}
	}
	action := r.act(Action{Kind: ActionOpen, Path: path})
	if r.openQuietlyDenied(action) {
		return notThere()
	}
	b, err := r.readFileGated(r.ctx, &action, path)
	if errors.Is(err, errRefused) {
		// The name was allowed and the object it reached was not — a link out
		// of an allowed directory. Hidden the same way, and nothing said about
		// where the link went.
		return notThere()
	}
	if err != nil {
		r.emit(r.ctx, Event{Kind: EventError, Action: action, Err: err})
		return nil, err
	}
	r.emit(r.ctx, Event{Kind: EventAccess, Action: action})
	return b, nil
}

// AllowModify asks the gate about a path a dialect's builtin is about to
// change in place: unlink it, rename it, create a directory at it, or change
// its mode or its owner.
//
// It is the seam #1808 named as missing. #1807 gave a dialect `AllowOpen` and
// `VerifyOpened`, which answer "may this builtin read or write the *contents*
// of a file", and every other verb had nothing to call at all. So `zsh/files`
// arrived with nine builtins whose entire purpose is modifying the
// filesystem, each one reaching for the `os` package directly, and a policy
// that refused every write still let `zf_rm` delete whatever it was pointed
// at — silently, with a zero status, and with nothing on the audit stream
// (#1819).
//
// # A modification is a write, and deliberately not a kind of its own
//
// The action is an `ActionOpen` with `Write` set, which is the slot the
// `write` selector already covers. That is a decision rather than a shortcut.
// A new `ActionUnlink` would have to be added to the policy grammar, to the
// `-deny` surface, and to every Gate anyone has written — and until all three
// caught up, `default deny write` would not refuse `zf_rm`. A policy author
// who writes "deny write" and is then handed `rm` has been given a policy
// that does not mean what it says, which is the sentence policy.go already
// uses to explain why the probes group with the read.
//
// What it costs is a record that says `open` for something that never opened
// a descriptor. That is the honest reading of it: the audit says a write to
// this path was permitted, which is exactly what happened, and the builtin's
// own name is on the same line through Event.Line and the diagnostic.
//
// # There is no verify half, and why that is sound rather than unfinished
//
// `AllowOpen` must be followed by `VerifyOpened` because a name is not an
// object: a symbolic link under an allowed directory is a name the gate
// permits and an object it would not. These verbs have no descriptor to
// verify against, so the same question has to be answered differently, and
// the answer differs by verb:
//
//   - `unlink` and `rmdir` act on the *name*, never on what it points at.
//     Removing a link out of an allowed directory removes the link, so the
//     object the rule was protecting is untouched and the name is the right
//     thing to have checked.
//   - `rename` likewise moves a name.
//   - `mkdir` creates at a name that by definition resolves to nothing yet.
//   - `chmod` and `chown` *do* follow a link, and the caller is expected to
//     ask about the path with symlinks refused — see fileGatedPaths in
//     dialect/zsh, which is the only caller and does exactly that.
//
// A refusal is reported to the script in the same words a refused redirection
// gets, and sets the failing status, because unlike a probe there is no
// honest way to carry on: a `zf_rm` that was refused has not removed
// anything, and saying so is the only answer that is not a lie.
func (r *Runner) AllowModify(ctx context.Context, path string) bool {
	action := r.act(Action{Kind: ActionOpen, Path: path, Write: true})
	if !r.allowed(ctx, action) {
		return false
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: action})
	return true
}

// AllowReadPath asks the gate whether a builtin may make the contents at a
// path reachable, where it will not be opening the file to do it.
//
// `zf_ln` is why it exists, and it is worth stating because the case looks
// like it should not need a read check at all: the builtin never reads a
// byte. A hard link is a second *name* for one object, so `zf_ln secret
// ./inside` puts the object inside the allowed subtree, and every later read
// of `./inside` is then allowed on its own merits — there is no link for the
// resolving walk to notice, because a hard link is not a link in that sense.
// The contents crossed the boundary at the moment the name was created, which
// makes creating it a read of the source (#1819).
//
// A symbolic link needs no such check and does not get one: reading through
// it walks to the object, and internal/opened checks what the walk reached.
func (r *Runner) AllowReadPath(ctx context.Context, path string) bool {
	action := r.act(Action{Kind: ActionOpen, Path: path})
	if !r.allowed(ctx, action) {
		return false
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: action})
	return true
}

// AllowProbe asks the gate a question about a path on behalf of a dialect's
// builtin, with the probe semantics the rest of the shell's probes have.
//
// `zstat` is the caller. fsgate.go opens by saying every stat in the
// interpreter comes through the gate because a probe is an oracle, and
// `zstat` was outside that sentence: on one path in one script, `[[ -f
// secret ]]` was refused into "not there" while `zstat +size secret`
// answered with the true size and mtime (#1819).
//
// False means hidden, and the caller must treat it exactly as it treats a
// path that is not there — quietly, with no diagnostic of its own. That is
// the same rule ActionStat documents and for the same reason: a refusal that
// identified itself would be an oracle for what the policy hides, which is
// the thing the refusal exists to prevent.
func (r *Runner) AllowProbe(ctx context.Context, path string) bool {
	return !r.probeDenied(r.act(Action{Kind: ActionStat, Path: path}))
}

// AllowList asks the gate whether a builtin may enumerate a directory.
//
// `zf_rm -r` is the caller: a recursive removal reads every directory on its
// way down, which is the enumeration ActionReadDir exists to cover — `echo
// /**` and a tree walk learn the same thing. Refused the same quiet way a
// probe is, and the caller reports the failure a directory it cannot read
// would already have caused.
func (r *Runner) AllowList(ctx context.Context, path string) bool {
	return !r.probeDenied(r.act(Action{Kind: ActionReadDir, Path: path}))
}

// ShellContext is the context of the command this shell is running, for a
// dialect's callback that needs one and was not handed one.
//
// A dynamic association's producer and its writer are the callers, and the
// shape is the one `interp/mathfunc.go` already settled: a read of
// `${mapfile[p]}` is an *expansion* and a write to it is an *assignment*, and
// neither is a command with a context of its own. Threading one through every
// expander signature to reach two callbacks would be worse than saying here
// that the shell has exactly one context at a time and this is it.
//
// It matters because the gate takes a context — [Runner.AllowReadPath] and
// the rest pass it straight to [Gate.Allow] and to every event the access
// raises. Handing those [context.Background] instead would leave a policy's
// own deadline and a front end's cancellation off the one kind of access that
// reaches the filesystem through a parameter.
//
// Background before the first Run, which is not a state a script can observe:
// a callback registered by a dialect runs when a script reads the name, and a
// script is running by then.
func (r *Runner) ShellContext() context.Context {
	if r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}
