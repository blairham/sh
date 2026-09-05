// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package driver is the front end a dialect needs in order to be a program.
//
// The three vectors say which shell a thing is; none of them says how it is
// invoked. That part — read `-c` or a script file, name yourself the way a
// shell names itself, word a parse failure the way this dialect words it, exit
// with the status this dialect exits with — is identical for every dialect and
// is not shell-specific at all. Written once per binary it drifts, and it did:
// cmd/sh grew script support and the dialect binaries did not, so the
// conformance harness was grading the drivers rather than the core and
// reported the dialects fourteen cases worse than they are.
//
// It is public rather than internal on purpose. A dialect built outside this
// repository is meant to need nothing but the public API, and it needs a front
// end as much as it needs a semantics vector; putting this under internal/
// would make copying the file the only way to ship a dialect binary, which is
// the situation that caused the drift. cmd/bash exists to prove the core is a
// library rather than a program with options, and that proof is worth
// something only if everything the binary imports is importable by anyone.
//
// Nothing here imports a dialect package or names a shell. A dialect arrives
// as a value, the same way interp takes one.
package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/internal/panicguard"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// Shell is everything that makes a binary one shell rather than another.
//
// The three vector fields are what a dialect package already exports, so
// filling them in is naming the package three times. Prelude and Register are
// the other two extension points from docs/design.md, in the same value,
// because a binary needs all three and there is no reason to install them by
// three different routes.
type Shell struct {
	// Name is what the shell calls itself when it has no argv[0] to use.
	// A real shell prefers the path it was invoked by, which MainArgs does;
	// this is the fallback, and it is what tests use.
	Name string

	// The three vectors. Zero values are legal and mean the substrate's own
	// answers, which is what cmd/sh's `core` dialect is.
	Dialect     syntax.Dialect
	Semantics   interp.Semantics
	Diagnostics interp.Diagnostics

	// Prelude is the dialect written as shell. It is sourced on the runner
	// before the script, so its functions shadow builtins and external
	// commands for the rest of the run.
	Prelude string

	// PromptStyle is what this dialect does to a prompt parameter's value
	// before it is drawn. The zero value draws it as it stands, which is
	// what a shell without a dialect does.
	PromptStyle repl.PromptStyle

	// EditorStyle is what this dialect draws while a line is being typed —
	// the mark on a line abandoned with ^C. The zero value draws nothing.
	EditorStyle repl.EditorStyle

	// HistoryStyle is how this dialect draws a search of the session's
	// history and what it declines to record in it. The zero value searches
	// in the substrate's own wording and records everything.
	HistoryStyle repl.HistoryStyle

	// Register adds or removes builtins — the part of a dialect that shell
	// cannot express. Nil means the dialect needs none, which is the common
	// case now that cd, pwd and read live in the core.
	Register func(*interp.Runner)

	// KeyBindings is what a person has rebound in this session, read from the
	// Runner because the command that changes it is a builtin and the Runner
	// is where a builtin's state lives. Nil is a dialect with no way to
	// rebind a key, which is three of the four.
	//
	// It takes the Runner rather than being a table because the table is
	// *state*: it changes while the session runs, and a value collected here
	// would be the one the shell started with.
	KeyBindings func(*interp.Runner) map[string]repl.Widget

	// Stdin is where the shell reads from: the lines a person types, and the
	// program itself where the invocation named nothing to run. It has to be
	// a terminal for the editor to work, and it is handed to the Runner, so
	// what the front end has not read of a program on it is what the script
	// running on it finds — see program.
	Stdin *os.File

	// Stdout and Stderr default to the process's. Tests set them; a binary
	// leaves them alone.
	Stdout, Stderr io.Writer

	// Dir is where the shell starts, and empty means where the process is.
	//
	// A binary leaves it alone: a shell invoked from a command line begins in
	// the directory it was invoked from, which is what the default reads.
	// What it is here for is a front end holding more than one shell at once
	// — an agent protocol gives every session a directory of its own — and
	// there the process's own working directory cannot be the answer for all
	// of them. Chdir-ing between them is the thing the library rule in
	// docs/design.md forbids, one level up: a Runner's directory is r.Dir
	// precisely so that two of them in one process need not agree.
	Dir string

	// AxisRemedy is what this binary would have a person do about an axis no
	// dialect answered — "pass -dialect …", for a binary that has such a
	// flag. It is handed to every Runner this front end builds, and it is
	// used for the one refusal the front end makes on its own account: `-c`
	// with `-s`, where the panel splits over which operand is `$0`.
	//
	// Empty is right for a dialect binary. `bash` reaching an unanswered axis
	// is a hole in bash's vector, and telling its user to choose a dialect
	// would be telling them to fix our bug.
	//
	// It is a string this package carries rather than a sentence it composes,
	// because the sentence names a flag this package does not have and shells
	// this package may not name. See interp.Runner.AxisRemedy, which says the
	// same thing one level down.
	AxisRemedy string

	// Gate is asked about every action that leaves the process — an exec, a
	// file open, a stat, a directory read — before it happens. Nil allows
	// everything, which is what a shell without a policy is, and costs
	// nothing: interp's fast path is a nil check.
	//
	// The gate is interpreter-internal by design, because `eval`, `.`,
	// command substitution and subshells all re-enter with their own input
	// and a policy applied out here would be walked around by the first one
	// of them. This field is only how a binary *supplies* the value; what
	// the decision means, and which actions are asked about, is interp's —
	// see interp.Gate and docs/design.md.
	//
	// It is consulted from more than one goroutine: a background job and
	// each half of a pipeline run on their own, so a gate that keeps any
	// state has to guard it.
	Gate interp.Gate

	// Events receives a structured record of what happened — a command
	// starting, its status, a refusal, a failure, a file access. Nil
	// discards them, and costs nothing for the same reason Gate does.
	//
	// This is the attachment point a front end that has to report on a
	// running shell consumes: an agent protocol streaming what a script
	// did, an audit trail, a trace. What a command *wrote* is deliberately
	// not carried here — the streams above are the caller's own writers
	// already.
	//
	// Called from more than one goroutine, exactly as Gate is.
	Events interp.Sink

	// Session identifies this run to everything that records it: the event
	// stream carries it on every record, and so does anything else the front
	// end writes about the same commands.
	//
	// Left empty it is filled in here, once, with a fresh identity. That is
	// the point of it being the front end's: a run has more than one account
	// of itself — an audit stream, and whatever a prompt keeps of what it ran
	// — and those are joinable only if one value reaches both. Generated at
	// the same place the streams and the name are defaulted, because that is
	// the one point every route passes through before anything is built, so
	// the Runner and the prompt cannot end up with different answers.
	//
	// A caller that has its own idea of a session — an agent protocol with
	// several shells behind one connection — sets it and this leaves it alone.
	Session string

	// KeepProcess stops `exec cmd` from replacing this process, which a
	// binary being a shell does not want and a test does.
	//
	// interp defaults to *not* replacing the process, because it is a library
	// and cannot do that behind an embedder's back. A shell binary is exactly
	// the case where it is correct, so this is where the default flips: a
	// driver.Shell replaces its process unless a caller says otherwise. The
	// field is negative for that reason — the zero value is what a binary
	// wants.
	KeepProcess bool
}

// usageStatus is what a shell exits with when it was invoked wrongly, as
// opposed to when the script it was given failed. Every shell in the panel
// uses 2 for this, so unlike a syntax error's status it is not a dialect
// question.
const usageStatus = 2

// Main is the whole of a dialect binary's main:
//
//	func main() { os.Exit(driver.Main(driver.Shell{...})) }
//
// It reads the conventional invocations — `-c COMMAND`, a script path, or
// standard input — and returns the status to exit with rather than exiting, so
// that it is testable.
func Main(sh Shell) int { return MainArgs(sh, os.Args) }

// MainArgs is Main with the argument vector given rather than taken from the
// process, which is what makes it testable without a subprocess.
//
// argv includes argv[0]. An argv with nothing in it is treated as bare, since
// a shell that cannot name itself still has to produce diagnostics.
func MainArgs(sh Shell, argv []string) int {
	sh = sh.withDefaults(argv)
	in, err := sh.input(argv)
	if err != nil {
		var se *scriptError
		if errors.As(err, &se) {
			// A script operand that would not open is the shell failing to
			// reach a program rather than failing to understand its own
			// argument vector, and the panel numbers it that way: 127 for a
			// path that is not there and 126 for one that will not open, in
			// the shells that tell them apart. Reported before `$0` exists,
			// so the shell names itself.
			sh.errf("%s", sh.Diagnostics.ScriptDiagnostic(sh.Name, se.path, se.err))
			return sh.Diagnostics.ScriptStatus(se.err)
		}
		sh.errf("%s: %v\n", sh.Name, err)
		return usageStatus
	}
	if in.prompt {
		// A prompt rather than a script, and reached from here so that every
		// binary built on this front end has one. It was in a single binary's
		// main once, which is how `sh` learned to prompt and `bash` did not.
		//
		// With whatever parameters the invocation supplied: `sh -s one two`
		// sets `$1` at a prompt in all four.
		return sh.interactive(argv, in)
	}
	// Asked here rather than inside the option loop: both are facts about
	// argv[0], which is the one word that loop never looks at. The prompt
	// route above asks the same questions of the same vector for itself.
	in.login = LoginShell(argv)
	in.posix = PosixNamed(argv)
	return sh.run(in)
}

// Run parses and executes src, returning the status to exit with.
//
// It is exported because a binary with argument handling of its own — cmd/sh
// has -tokens, -parse and -dialect — still wants this half, and this is the
// half that has to agree across every driver.
func Run(sh Shell, src, name string) int {
	sh = sh.withDefaults(nil)
	if name == "" {
		name = sh.Name
	}
	return sh.run(source{src: src, name: name, dg: sh.Diagnostics})
}

// RunCommand is Run for a `-c` command string, which is not the same as
// running its text: the origin is labeled `-c` in a parse failure's location,
// one dialect parses the whole string before running any of it, and the first
// operand is `$0` with the rest the positional parameters.
//
// It exists because a front end with options of its own — `sh` has `-tokens`,
// `-parse` and `-dialect`, which are not shell conventions — still has to
// reach the same `-c` as everyone else. Reaching past it instead is what left
// `sh -dialect bash` printing a location without the `-c` in it and running
// `-c 'echo $1' a b` with no parameters at all.
func RunCommand(sh Shell, src string, operands []string) int {
	sh = sh.withDefaults(nil)
	return sh.run(commandSource(sh, src, operands))
}

// RunStdin is Run for a script arriving on standard input, where there is
// no $0 to name — two dialects change the shape of their prefixes on that
// route rather than substituting a name.
//
// It takes the text rather than the descriptor, so it is the *diagnostics* of
// that route and not the whole of it: a caller that has already read the
// program has nothing left on the descriptor for the script to share, which is
// the other half of what MainArgs does here. See program.
func RunStdin(sh Shell, src string) int {
	sh = sh.withDefaults(nil)
	return sh.run(source{src: src, name: sh.Name, dg: sh.Diagnostics.ForStdin()})
}

// RunScript is Run for input that came from a file, which selects the
// script-form diagnostics and names the script rather than the shell.
func RunScript(sh Shell, src, path string) int {
	sh = sh.withDefaults(nil)
	return sh.run(source{src: src, name: path, file: path, dg: sh.Diagnostics.ForScript()})
}

// errf writes to the shell's error stream.
//
// One place, so the ignored error is ignored once and on purpose: there is
// nothing a shell can usefully do about a failed write to stderr except try to
// report it to stderr. interp's own errf exists for the same reason.
func (sh Shell) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(sh.Stderr, format, args...)
}

func (sh Shell) withDefaults(argv []string) Shell {
	if sh.Stdin == nil {
		sh.Stdin = os.Stdin
	}
	if sh.Stdout == nil {
		sh.Stdout = os.Stdout
	}
	if sh.Stderr == nil {
		sh.Stderr = os.Stderr
	}
	// A real shell names itself by the path it was invoked by — dash reports
	// "/bin/dash: 1: …" — so argv[0] wins over the configured name, which is
	// left as the fallback for a test or for an argv with nothing in it.
	if len(argv) > 0 && argv[0] != "" {
		sh.Name = argv[0]
	}
	if sh.Session == "" {
		// One identity for the run, made here so that everything built from
		// this Shell shares it. A shell that recorded its commands under one
		// id and its accesses under another would produce two accounts of one
		// session that look joinable and are not, which is worse than two that
		// do not claim to be.
		sh.Session = event.NewID(time.Now())
	}
	return sh
}

// input decides what to run and what to call it.
//
// The answers differ in more than where the bytes came from. A script is named
// by its own path in every diagnostic and takes the script-form diagnostics,
// because some behavior differs between a script and `-c`; `-c` and standard
// input are named after the shell.
// source is where a shell's script came from: the text, what to call it in a
// diagnostic, and how the dialect words one about it.
type source struct {
	src  string
	name string
	// input is the front end's label for the origin — "-c", and empty for a
	// file or for standard input. One dialect names it in a parse failure's
	// location and nowhere else, which is why it travels beside the name
	// rather than being folded into it.
	input string
	// params are the positional parameters the invocation supplies, which is
	// everything after whichever operand became the name. Unanimous across
	// the panel: a script's path is `$0` and the operands after it are `$1`
	// onward; `-c` takes the *first* operand after the command as `$0` and
	// the rest as parameters; and reading standard input leaves `$0` as the
	// shell and makes every operand a parameter.
	params []string
	// opts are the set options the invocation asked for — `sh -e`, `+x`,
	// `-o pipefail` — held as text until the runner they apply to exists.
	// They are `set` options, not the front end's own: every shell in the
	// panel hands them to the same machinery `set` uses, which is why `$-`
	// reflects them and `set +e` can undo them.
	opts []optionSpec
	// prompt says there is nothing to run and a person at a keyboard, so the
	// shell should prompt rather than read a script. It travels on the
	// source because deciding it is part of reading the invocation: `-i`,
	// or no operands and a terminal on standard input.
	prompt bool
	// interactive says the shell *is* an interactive one, which is a wider
	// question than whether it prompts and is why the two are separate
	// fields. `-i` with a script operand runs the script — the panel splits
	// only over whether a prompt follows it, and the core follows the three
	// that exit — but the shell it runs the script in is interactive all the
	// same: measured, `i` is in `$-` for `-i` on every route in all four,
	// and all four expand aliases there even where they would not in a
	// script. Conflating the two dropped `-i` outright whenever an operand
	// was given (#472).
	interactive bool
	// file is the path the input was read from, or empty where there was no
	// file — `-c`, or standard input. It is the floor of the call stack: a
	// script asking where it is means this, and only the front end knows.
	file string
	// wholeFirst parses the whole input before running any of it, which one
	// dialect does for a command string and no dialect does for a script.
	wholeFirst bool
	// login says argv[0] began with a dash, the convention by which `login`
	// and every terminal emulator's "run as a login shell" tell a shell what
	// it is. It travels on the source for the same reason interactive does:
	// deciding it is part of reading the invocation, and the routes that run
	// a script are reached without an argv.
	//
	// It is only ever set from an argument vector, so a caller reaching Run,
	// RunCommand, RunScript or RunStdin directly is never a login shell —
	// which is right, because a program embedding a runner has an invocation
	// of its own and this shell is not it.
	login bool
	// posix says argv[0] named the shell `sh`, so it starts in POSIX mode.
	// The other fact read off the same word, carried for the same reason and
	// set from the same two places, and it is only ever set from an argument
	// vector for the reason login is: a caller reaching Run or RunCommand
	// directly has an invocation of its own.
	posix bool
	// onStdin says the program itself arrives on standard input, so it is
	// read as it runs rather than handed over as text. The shell holds one
	// descriptor: what it has not read yet is still there for the script's
	// own `read`, for a command the script runs, and for an `exec 0<` that
	// points the descriptor somewhere else entirely.
	onStdin bool
	// stdinOption says the invocation wrote `-s`, which is nearly the same
	// fact as onStdin and separate for the one spelling where it is not:
	// `sh -s -c cmd` runs the command string and still shows `s` in `$-`,
	// unanimously across the panel.
	stdinOption bool
	// startup is what the invocation said about which startup files to read:
	// `-l`, `--norc`, `--noprofile`, `--rcfile FILE`, `-f`. Carried here for
	// the reason login and posix are — deciding it is part of reading an
	// argument vector, and a caller reaching Run or RunCommand directly has
	// an invocation of its own and none of these.
	startup startupFlags
	dg      interp.Diagnostics
}

// loginShell reports whether this invocation is a login shell, by either of
// the two routes there are: a dashed argv[0], which is what `login` and every
// terminal emulator's "run as a login shell" does, or an explicit option,
// which is the only way a person at a keyboard can say it.
//
// The two are the same fact for *which* files are read and differ for whether
// a shell with a script to run reads them at all; see readsLoginProfile.
func (s source) loginShell() bool { return s.login || s.startup.login }

// invocationRoute is which of the three routes the program came by, for the
// runner. The same three cases aliasRoute splits on and the same reason: a
// fact about the invocation rather than about the language.
//
// A prompt is not among them, and not because it has no route — it is the
// standard-input route with a person on the other end — but because it never
// reaches here. The prompt branch hands off to interactive before run is
// called, and session says so itself, beside the two other facts it states
// diagName is what a diagnostic about this input calls the shell.
//
// The same question interp's own answers for a diagnostic raised while
// running, asked here because a parse failure is raised before there is a
// runner to ask. One dialect names itself rather than the path it was invoked
// by, and the two have to agree or the same shell gives two names depending on
// whether the script got as far as running.
//
// Only where the shell is what is being named: a script is named by its own
// path in every shell in the panel, and `file` is exactly the routes that have
// one. `$0` is s.name either way — the dialect that shortens its diagnostic
// still reports the whole path there, which is why this is a separate answer
// and not a change to the name.
func (s source) diagName() string {
	if s.file == "" && s.dg.SelfName != "" {
		return s.dg.SelfName
	}
	return s.name
}

// about a session.
func (s source) invocationRoute() interp.Route {
	switch {
	case s.onStdin:
		return interp.RouteStandardInput
	case s.file != "":
		return interp.RouteScriptFile
	}
	return interp.RouteCommandString
}

// aliasRoute is which of the three non-interactive routes this program
// arrived by, for the one grammar question that differs between them.
//
// It lives here because only the front end knows: `syntax` asks whether a
// dialect expands an alias on this route, and the route is what reading an
// invocation produced. The same three cases `$0` already splits on, and the
// same reason — a fact about the invocation rather than about the language.
func (s source) aliasRoute() syntax.AliasRoutes {
	switch {
	case s.onStdin:
		return syntax.AliasOnStandardInput
	case s.file != "":
		return syntax.AliasFromScriptFile
	}
	return syntax.AliasFromCommandString
}

// optionSpec is one run of set options the invocation asked for: the letters
// of `-eu`, or the name after `-o`. Text rather than applied state, because
// the runner the options belong to does not exist while the argument vector
// is being read.
type optionSpec struct {
	// spec is the option letters, or the long name where isName is set.
	spec string
	// isName says spec is a `-o`/`+o` name rather than letters, which an
	// empty spec cannot: `sh -o ''` names an option called nothing, and
	// three of the panel refuse it rather than ignoring it.
	isName bool
	// on is `-` rather than `+`. Both signs work on every option, which is
	// measured and unanimous: `sh +x script` is how xtrace is kept *off*
	// regardless of what the parent had.
	on bool
}

// invocation is the state the option loop accumulates: how to read the words
// that are not operands, gathered before anything decides what to run.
type invocation struct {
	// forcePrompt is `-i`: a prompt even where standard input is not a
	// terminal, which is how a shell is driven by something that is not a
	// person — a test, or a program feeding it lines.
	forcePrompt bool
	// fromStdin is `-s`: the script arrives on standard input and every
	// operand is a parameter, none of them a path. An option like any
	// other, not a terminator — measured, all four shells still read
	// options after it: `sh -s -e arg` sets errexit and makes `arg` `$1`.
	fromStdin bool
	// sawC is `-c`: the script is a command string taken from the first
	// operand rather than read from a file or from standard input. A flag
	// rather than the string itself, because the letter and its operand are
	// read at different times — `sh -ce cmd` says `c` in the middle of a
	// bundle and the string is still two letters away, and `sh -c -x cmd`
	// puts a whole other option word between them. Measured and unanimous:
	// all four shells keep reading options after the `c` and take the
	// command string from the first operand, whichever sign the word had.
	sawC bool
	// plusC is that letter written with a plus — `+c`, `+ce`. Both signs
	// select the command string, which is unanimous and is what sawC
	// records; the sign is kept because one shell then names the operands
	// differently, and the sign of the word the letter was in is the whole
	// of the question. See Semantics.PlusSignedCommandStringIsDollarZero.
	plusC bool
	// startup is what the startup-file options said, accumulated the same
	// way the rest is and read off the dialect's own spellings; see
	// Semantics.StartupFileOptions.
	startup startupFlags
	opts    []optionSpec
}

// namesStartupOption reports whether this dialect spells a startup-file option
// this way, which the letter loop has to know *before* it decides the letter
// is a set option's rather than this front end's.
//
// Separate from startupOption because that one has a side effect and this
// question is asked in a switch's condition. The two read the same four lists,
// which is why neither takes an opinion about what a spelling means.
func (sh Shell) namesStartupOption(spelling string) bool {
	o := sh.Semantics.StartupFileOptions
	return spelt(o.Login, spelling) ||
		spelt(o.SuppressAll, spelling) ||
		spelt(o.SuppressLogin, spelling) ||
		spelt(o.SuppressInteractive, spelling) ||
		spelt(o.NameInteractive, spelling)
}

// spelt reports whether a whitespace-separated list of option spellings holds
// this one. The lists are two or three words long, so they are read on demand
// rather than split once and kept — the vector is a value a caller may still
// be editing, and a cache of it would be a second answer to the same question.
func spelt(list, spelling string) bool {
	return slices.Contains(strings.Fields(list), spelling)
}

// startupOption applies a startup-file option this dialect spells `spelling`,
// reporting whether it was one at all and what arguments are left.
//
// The spellings are the dialect's rather than this front end's, which is the
// whole reason they are data: `-f` is "read no startup files" in zsh and "turn
// globbing off" in every other shell in the panel, so a front end with an
// opinion about the letter would have to hold both. A dialect that names none
// of them — dash and ksh93 name none — is a shell whose startup files cannot
// be skipped, which is measured and is what those two do.
func (sh Shell) startupOption(spelling string, args []string, inv *invocation) (rest []string, matched bool, err error) {
	o := sh.Semantics.StartupFileOptions
	switch {
	case spelt(o.Login, spelling):
		inv.startup.login = true
	case spelt(o.SuppressAll, spelling):
		inv.startup.none = true
	case spelt(o.SuppressLogin, spelling):
		inv.startup.noLogin = true
	case spelt(o.SuppressInteractive, spelling):
		inv.startup.noInteractive = true
	case spelt(o.NameInteractive, spelling):
		// The file is the next word. Refused rather than ignored when there
		// is none: an invocation that named a startup file and did not say
		// which is not one a shell can guess at, and every other option here
		// that takes an argument is refused the same way.
		if len(args) < 1 {
			return nil, true, fmt.Errorf("%s requires an argument", spelling)
		}
		inv.startup.file = args[0]
		return args[1:], true, nil
	default:
		return args, false, nil
	}
	return args, true, nil
}

func (sh Shell) input(argv []string) (source, error) {
	args := argv
	if len(args) > 0 {
		args = args[1:]
	}

	var inv invocation

	// Hand-parsed rather than with the flag package, because a shell's
	// conventions are not Go's: options stop at the first operand, `-c` says
	// the first operand is a command string rather than a path, a `+` turns
	// an option off, and the words after a command string must not be
	// claimed as more flags.
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--", a == "-":
			// Both end the options. A lone `-` does not mean "read standard
			// input": every shell in the panel treats `sh - a b` as running
			// the script `a`, and only a `-` with nothing after it falls
			// through to standard input.
			return sh.operands(args[1:], inv)
		case len(a) >= 2 && (a[0] == '-' || a[0] == '+'):
			rest, err := sh.optionWord(a, args[1:], &inv)
			if err != nil {
				return source{}, err
			}
			args = rest
		default:
			return sh.operands(args, inv)
		}
	}
	return sh.operands(nil, inv)
}

// optionWord reads one word of options — a single letter, a bundle, either
// sign — recording what it finds on inv. It returns the arguments still to
// read.
//
// No letter ends the option loop. `-c` in particular does not: it records
// that the command string is coming and reading continues, because the panel
// is unanimous that `sh -c -x cmd` sets xtrace and runs `cmd`, and that
// `sh -c -- cmd` runs `cmd` too. What ends the loop is an operand.
//
// The letters this front end owns are the ones that say where the script
// comes from — `c`, `i`, `s` — plus whichever the dialect spends on its
// startup files, and they bundle with the rest: `sh -ec cmd` is unanimous
// across the panel. Every other letter is a `set` option and stays text here;
// whether the shell has it is the dialect's question, answered by the same
// machinery `set` uses once the runner exists.
func (sh Shell) optionWord(a string, args []string, inv *invocation) (rest []string, err error) {
	on := a[0] == '-'
	body := a[1:]
	// A double-dash word is a whole spelling and never a bundle, so it is
	// matched before the letters are read — `--` itself never reaches here,
	// having ended the options one level up.
	//
	// One this dialect does not name is refused rather than read as letters,
	// which is measured and unanimous: every shell in the panel refuses an
	// unknown long option outright. Reading it as a bundle is not merely a
	// worse diagnostic — the letters of `--rcfile` include a `c`, so a shell
	// without that option would have taken the *next word* as a command
	// string and run it.
	if strings.HasPrefix(a, "--") {
		rest, matched, err := sh.startupOption(a, args, inv)
		if matched {
			return rest, err
		}
		return nil, fmt.Errorf("unknown option %q", a)
	}
	letters := ""
	flush := func() {
		if letters != "" {
			inv.opts = append(inv.opts, optionSpec{spec: letters, on: on})
			letters = ""
		}
	}
	for k := 0; k < len(body); k++ {
		switch ch := body[k]; {
		case ch == 'c':
			// The command string comes from the first *operand*, and this
			// letter only records that it is coming. The rest of the word is
			// more option letters, not the string: no shell in the panel
			// accepts an attached `-c<string>` — bash reads the tail as
			// letters and refuses the space in `-cecho hi`, and dash, ksh93
			// and zsh each refuse it in their own words. So `sh -ce cmd` and
			// `sh -ec cmd` are the same invocation, and both are errexit
			// plus a command string, which is what all four do.
			//
			// Both signs, and that is measured rather than assumed: all four
			// run the command for `sh +c cmd`. Reading `+c` as a set letter
			// instead opened the command string as a script file.
			inv.sawC = true
			// Which sign it was written with, for the one shell that
			// reads a plus-signed command string as its own `$0`.
			inv.plusC = !on
		case ch == 'i' && on:
			inv.forcePrompt = true
		case ch == 's' && on:
			inv.fromStdin = true
		case on && sh.namesStartupOption("-"+string(ch)):
			// A startup-file option written as one letter, which bundles
			// like any other: `zsh -if` is `-i` and `-f`. Only the minus
			// spelling — no shell in the panel gives `+f` a meaning, and a
			// plus-signed letter here is a set option's the way it always
			// was.
			//
			// Handled before the default arm so the letter never reaches the
			// runner as a set option. It is the same letter POSIX gives to
			// globbing, and one dialect spends it on this instead.
			if args, _, err = sh.startupOption("-"+string(ch), args, inv); err != nil {
				return nil, err
			}
		case ch == 'o':
			// The long spelling, whose name is the next word — read at the
			// end of a bundle exactly as `set` reads it, so `sh -euo
			// pipefail script` works the way the line at the top of so many
			// scripts does. Elsewhere in a word it is refused: measured,
			// the panel splits over `-oNAME` — zsh and ksh93 read the rest
			// of the word as the name, bash and dash read it as more
			// letters — so the core takes neither side.
			if k != len(body)-1 {
				return nil, fmt.Errorf("unknown option %q", a)
			}
			flush()
			if len(args) < 1 {
				// Refused rather than listed. With no name at all three of
				// the four print the option table and read on; zsh refuses
				// with "string expected after -o". A listing at invocation
				// is not worth the machinery until something needs it, and
				// refusing is the honest half of a split panel.
				return nil, fmt.Errorf("%s requires an argument", a)
			}
			inv.opts = append(inv.opts, optionSpec{spec: args[0], isName: true, on: on})
			return args[1:], nil
		default:
			// A set option's letter, ours to carry and the dialect's to
			// judge. An unknown one is refused before anything runs, just
			// not here: the front end has no option table of its own, so
			// `sh -Q` is refused by the runner the way `set -Q` would be.
			letters += string(ch)
		}
	}
	flush()
	return args, nil
}

// scriptError is a script operand the shell could not read, carried as its own
// type so that the one place which knows an invocation went wrong can still
// tell *which* way it went wrong.
//
// The path travels beside the error because a diagnostic names the operand as
// it was written, and Go's own message would name it a second time in its own
// words — "open nosuch.sh: no such file or directory" reads like a Go program
// and not like a shell.
type scriptError struct {
	path string
	err  error
}

func (e *scriptError) Error() string { return e.err.Error() }
func (e *scriptError) Unwrap() error { return e.err }

// operands handles what is left once the options are gone: the command string
// for `-c`, a script path, or nothing at all, which means standard input.
//
// Choosing the route is route's; carrying `-i` past it is this function's, in
// one place and after the fact rather than at each of the four returns.
// Deciding it per route is how it came to be decided in exactly one of them —
// `sh -i script.sh` ran the script with `-i` dropped on the floor, because
// the only branch that consulted the flag was the one with nothing to run
// (#472). A fifth route added below would inherit the answer rather than have
// to remember it.
func (sh Shell) operands(args []string, inv invocation) (source, error) {
	in, err := sh.route(args, inv)
	if err != nil {
		return source{}, err
	}
	// Either half makes the shell interactive: `-i` says so outright on
	// every route, and a prompt is interactive whether or not `-i` was
	// given. Measured, all four shells agree on both.
	in.interactive = inv.forcePrompt || in.prompt
	// `-s` as written, carried here for the same reason and in the same
	// place: it survives a route that overrode it, and `sh -s -c cmd` shows
	// `s` in `$-` in all four shells while running the command string.
	in.stdinOption = inv.fromStdin
	// And what the invocation said about its startup files, carried past the
	// route for the reason `-i` is: every route reads at least one of them,
	// so deciding it per route is how one of them would come to be forgotten.
	in.startup = inv.startup
	return in, nil
}

// route is operands without the part that is the same for every route: which
// of the four the invocation named, and what the shell is to run.
func (sh Shell) route(args []string, inv invocation) (source, error) {
	if inv.sawC {
		// `-c` wins over both of the other routes, which is measured rather
		// than a precedence invented here: all four shells run the command
		// for `sh -cs cmd` and for `sh -ci cmd` — neither reading standard
		// input nor prompting — and take the string from the first operand.
		if len(args) == 0 {
			return source{}, errors.New("-c requires an argument")
		}
		if inv.fromStdin && len(args) > 1 {
			// Both routes were named, and there is an operand for them to
			// disagree about. Which route the *program* comes from is
			// settled above and is unanimous; which route's rule names the
			// operands is not. With no operand past the command string the
			// two rules name the same nothing, so the question is not
			// raised and every dialect runs it.
			return sh.commandWithStdinOption(args[0], args[1:], inv)
		}
		s := commandSource(sh, args[0], args[1:])
		if inv.plusC && sh.Semantics.PlusSignedCommandStringIsDollarZero {
			// The plus spelling keeps `$0` for the command string, so no
			// operand is named by it and every one is a parameter. Applied
			// after commandSource rather than instead of it: everything
			// else about the route — the `-c` label in a location, the
			// dialect's diagnostics — is the same invocation.
			s.name, s.params = args[0], args[1:]
		}
		s.opts = inv.opts
		return s, nil
	}
	if inv.fromStdin || len(args) == 0 {
		// Standard input, and standard input from a terminal is a person:
		// all four prompt for `sh -s` there and read a script for `echo x |
		// sh -s`. Reading it as a script either way meant waiting for an
		// end-of-file nobody was going to type, which does not look like a
		// shell waiting for a decision — it looks like a hang.
		//
		// With `-s` every operand is a parameter and none of them is `$0`,
		// at a prompt as much as in a script: all four answer `sh -s one
		// two` with `$1` set. Without it there are no operands at all.
		if inv.forcePrompt || Interactively(sh, false) {
			// Nothing to run and someone at a keyboard. Asked before
			// reading, not after: reading standard input from a terminal
			// waits for an end-of-file that a person has not typed yet.
			return source{prompt: true, name: sh.Name, params: args, dg: sh.Diagnostics, opts: inv.opts}, nil
		}
		// Not read here. The program is on the same descriptor everything
		// else the script does reads from, so how much of it the shell takes
		// is a behavior rather than plumbing, and it is taken as the script
		// runs — see program and Semantics.StdinProgramReadInBlocks. Reading
		// it all here is what handed `read x` end of input and then ran the
		// data line as a command.
		return source{onStdin: true, name: sh.Name, params: args, dg: sh.Diagnostics.ForStdin(), opts: inv.opts}, nil
	}
	path := args[0]
	// Through the gate: the program a shell was pointed at is an access
	// chosen by whoever invoked it, so a policy hiding a path hides it from
	// `sh /that/path` too — and an audit trail that recorded every file a
	// script opened and not the script itself was missing the first one.
	b, err := sh.readFile(path)
	if err != nil {
		// Not a usage error. Every shell in the panel tells this apart from
		// being invoked wrongly, and three of the four tell the two ways it
		// fails apart from each other as well — so the error is wrapped
		// rather than returned bare, and the dialect words it and numbers it.
		return source{}, &scriptError{path: path, err: err}
	}
	// A shell running a script names the *script* in `$0` and in every
	// diagnostic, not itself, and reports in the script form — ksh93 also
	// changes how it names the line.
	return source{
		src: string(b), name: path, file: path,
		params: args[1:], dg: sh.Diagnostics.ForScript(), opts: inv.opts,
	}, nil
}

// commandWithStdinOption is `-c` and `-s` together, where the two routes have
// different rules for naming operands and the panel is split 2-2 over which
// of them applies.
//
// The command string wins about where the program comes from — all four run
// it, and neither read standard input nor prompt — so the only question left
// is whether the first operand becomes `$0` or whether every operand is a
// positional parameter and `$0` stays the shell. See
// Semantics.StdinOptionNamesTheOperands, which is the whole of it: nothing
// else about the invocation changes.
func (sh Shell) commandWithStdinOption(cmd string, operands []string, inv invocation) (source, error) {
	switch sh.Semantics.StdinOptionNamesTheOperands {
	case interp.Yes:
		// The standard-input rule: no operand is `$0`.
		s := commandSource(sh, cmd, nil)
		s.params, s.opts = operands, inv.opts
		return s, nil
	case interp.No:
		s := commandSource(sh, cmd, operands)
		s.opts = inv.opts
		return s, nil
	}
	// Refused rather than given one side's answer, the way an unanswered
	// axis is refused everywhere else. A usage error, because what could not
	// be understood is the argument vector.
	return source{}, errors.New(sh.unanswered("`-c` with `-s`, and which operand is $0"))
}

// unanswered is the refusal of an axis nothing answered, worded the way interp
// words the twenty-odd it refuses — the same sentence, and the same remedy
// appended where the binary supplied one.
//
// Spelled here as well as there because this is the one such refusal the front
// end makes on its own account: whether `-c` with `-s` names its operands is a
// question about the *invocation*, and there is no runner yet to ask.
func (sh Shell) unanswered(what string) string {
	msg := what + ": the shells disagree here and no dialect was chosen"
	if sh.AxisRemedy != "" {
		msg += "; " + sh.AxisRemedy
	}
	return msg
}

// commandSource is the source for `-c`, whose operands are named differently
// from every other route: the first is `$0` and only the rest are parameters,
// so `sh -c 'echo $0' name a` prints `name`. With no operands at all the shell
// keeps its own name and there are no parameters.
func commandSource(sh Shell, src string, operands []string) source {
	in := source{
		src: src, name: sh.Name, input: "-c",
		wholeFirst: sh.Diagnostics.CommandStringParsedWhole,
		dg:         sh.Diagnostics,
	}
	if len(operands) > 0 {
		in.name, in.params = operands[0], operands[1:]
	}
	return in
}

// run parses and executes src. input is what the front end calls where the
// script came from — "-c", or empty for a file or standard input — which one
// dialect names in a parse failure's location.
// newRunner builds the interpreter a shell runs in.
//
// Shared by the script path and the interactive one, which have to agree
// about every hook: an interactive shell that could not `exec`, or whose
// `umask` did nothing, would be a different shell from the one that runs the
// same lines from a file.
func (sh Shell) newRunner(name string, params []string, dg interp.Diagnostics, route interp.Route) *interp.Runner {
	r := &interp.Runner{
		// Where the program came from. Three things read it: the status one
		// dialect gives a failed expansion, the fatality another gives a
		// readonly reassignment, and the route letters in `$-`.
		Route:       route,
		Dialect:     &sh.Dialect,
		Semantics:   &sh.Semantics,
		Diagnostics: &dg,
		Name:        name,
		// argv[0], which is a different fact from Name and is the one the
		// startup `$_` follows. withDefaults has already preferred the real
		// argv[0] over the configured fallback, so this is the word the
		// process was executed as wherever there was one.
		Invocation: sh.Name,
		// What to tell a person who has just met an axis nothing answered.
		// Carried rather than composed, here and in interp both: naming the
		// remedy means naming a flag and the shells it takes, and neither
		// package may name a shell.
		AxisRemedy: sh.AxisRemedy,
		// `$1` onward. A nil slice and an empty one mean the same thing to
		// the interpreter, so nothing distinguishes "no operands" from
		// "operands that were all consumed as the name".
		Params: params,
		// The process's environment, read here and not in interp: os.Environ
		// answers for the whole process, and a library Runner must take the
		// environment it is handed rather than reach for shared state. This
		// binary *is* the process, so the read is made once, where it is
		// visible — the same split as ReplaceProcess below.
		Env: os.Environ(),
		// The shell's own descriptor rather than the process's, for the
		// reason the other two are the caller's: a front end that reaches
		// past its Shell is not usable by anything embedding it, and its own
		// tests silently read the *test binary's* input instead. It matters
		// twice over on the standard-input route, where this descriptor is
		// also where the rest of the program is coming from.
		Stdin:  sh.Stdin,
		Stdout: sh.Stdout,
		Stderr: sh.Stderr,
		// The policy and the observer, carried straight through. Both are
		// nil for every shell that has not asked for one, which is the
		// behavior every binary had before this field existed and costs a
		// nil check on interp's side.
		//
		// Here rather than at either call site, because this is the one
		// place a Runner is built: an interactive shell whose gate was not
		// installed would be a hole in the boundary shaped exactly like a
		// shell that works, which is the failure this whole seam exists to
		// make impossible.
		Gate:   sh.Gate,
		Events: sh.Events,
		// And which run this is, so every event it emits says so. Set beside
		// the sink because it is only meaningful to something reading one.
		Session: sh.Session,
		// And the guard, for the goroutines a shell runs beside itself.
		// Outside the KeepProcess block below, because this is not a
		// process hook: it changes nothing about the process and is the
		// same decision the guard around every route already makes — that
		// an interpreter bug is a diagnostic and a status rather than a
		// crash. An embedder that has told this front end to keep the
		// process is exactly the caller who most wants it.
		GuardConcurrent: sh.guardConcurrent(),
	}
	r.Dir = sh.Dir
	if r.Dir == "" {
		if wd, err := os.Getwd(); err == nil {
			// And where the process is, for the same reason: interp treats an
			// empty Dir as "stay relative" and never calls os.Getwd itself. A
			// failed Getwd — the directory was deleted under the process —
			// leaves Dir empty, which is that relative reading and the best
			// available.
			r.Dir = wd
		}
	}
	if !sh.KeepProcess {
		// This is a shell, so `exec` may really replace it. interp will not
		// reach for syscall.Exec itself — it is a library, and a Runner
		// embedded in some other program must not replace that program — so
		// the decision is made here, in the binary, where it is visible.
		r.ReplaceProcess = replaceProcess
		// And for the same reason it may really die: a script that signals
		// the shell fatally ends the process rather than the script.
		r.DieBySignal = dieBySignal
		// And for the same reason again, `umask` may really change the mask
		// this process creates files with. A Runner embedded in some other
		// program would be changing *its* mask for everything it writes
		// afterwards, so the decision belongs here rather than in interp.
		r.SetUmask = setUmask
		// And the limits it runs under, for the same reason again — a limit
		// outlives the command that set it.
		r.GetRlimit, r.SetRlimit = getRlimit, setRlimit
		// And it waits for its own children, which is the only way to be
		// told that one *stopped* rather than finished.
		r.WaitForCommand = waitForCommand
		// And it asks after the ones nothing is waiting for — what `bg` let
		// go of — which is reaping just as much and so belongs here too.
		r.PollCommand = pollCommand
		// And it may resume one it stopped, which is a signal leaving this
		// process and so the binary's to send.
		r.SignalGroup = signalGroup
		// And it hands the terminal to whatever it is running, which is what
		// makes ^C and ^Z reach the command rather than the shell.
		r.Foreground = foreground
		// And the descriptors it was started holding: `sh 3<&0 script` opens
		// 3 for the script, and a script can neither open one nor find out
		// that it has one. Here for the reason everything else in this block
		// is here — finding them means reading the *process's* open
		// descriptors, and a Runner embedded in another program would be
		// publishing that program's own files to a script it was handed.
		// interp takes the answer; only a binary that is the shell goes
		// looking for it.
		r.InheritedFiles = inheritedFiles()
		// And it dies of a fatal signal that arrives from *outside*, which
		// dieBySignal alone does not cover: that puts the disposition back
		// before a raise this shell makes itself, and a signal from another
		// process never reaches it. The runtime's handler answers those, and
		// for the throwing class it answers with a goroutine dump where every
		// shell in the panel prints nothing. See fatalsignal.go.
		watchFatalSignals(r)
	}
	if sh.Register != nil {
		// The dialect's own adjustment: what it adds to or removes from the
		// substrate's builtins, which is neither grammar nor semantics.
		sh.Register(r)
	}
	return r
}

// run parses and executes one whole invocation, guarding it against an
// interpreter bug.
//
// The route is the unit here, and that is the difference from a prompt. A
// script has control flow a panic has already broken — the line after the one
// that failed is not a fresh start, it is the middle of something — and there
// is no session to keep, so the run ends. What the guard buys is that it ends
// the way a shell ends: a diagnostic on stderr and a status, rather than a Go
// stack trace and whatever the runtime exits with.
func (sh Shell) run(in source) (status int) {
	if sh.guard().Do(func() { status = sh.runInput(in) }) {
		return panicguard.Status
	}
	return status
}

func (sh Shell) runInput(in source) int {
	// Two names, because they are two questions. `name` is `$0` — the path
	// the shell was invoked by — and `said` is what a diagnostic calls the
	// shell, which one dialect answers with a fixed name instead. Folding
	// them together shortened `$0` along with the diagnostic, which is a
	// thing no shell in the panel does.
	src, name, said, input, dg := in.src, in.name, in.diagName(), in.input, in.dg
	// One dialect reads a command string whole before running any of it, and
	// the rest run each line as they reach it. Parsing everything up front is
	// how that is done: the failure is then reported before anything has run.
	if in.wholeFirst {
		p := syntax.NewParser(src, sh.Dialect)
		p.Parse()
		if err := p.Err(); err != nil {
			// Input that ends unfinished is a syntax error rather than a
			// prompt when it did not come from a terminal. The whole
			// diagnostic is the dialect's: its wording, whether it names
			// where the script came from, and whether it echoes the line.
			// Before the error, and whether or not there is one: a remark
			// can accompany a fatal failure, which is measured — a here
			// document with neither its delimiter nor its enclosing `}`
			// produces both, warning first.
			sh.sayRemarks(dg, said, p.Remarks(), 0)
			sh.errf("%s", dg.ParseDiagnostic(said, input, err, src))
			return dg.StatusForParseError(err)
		}
	}

	r := sh.newRunner(name, in.params, dg, in.invocationRoute())
	// `-s` as written, which `$-` shows even where `-c` supplied the
	// program instead.
	r.StandardInputOption = in.stdinOption
	// What the invocation decided, handed to the runner rather than looked
	// up by it: interp is a library and has no standing to ask the process
	// whether anyone is watching. `$-` reports it as `i`, which is measured
	// unanimous for `-i` on every route.
	r.Interactive = in.interactive
	r.SetScriptFile(in.file)
	pr := wholeProgram(src, sh.Dialect)
	if in.onStdin {
		// The program is on the descriptor rather than in hand, so it is read
		// as it runs. How much at a time is the dialect's answer, and it is
		// the whole of what this route disagrees about.
		pr.more = stdinProgram(r, sh.Semantics.StdinProgramReadInBlocks)
	}
	// Aliases are expanded when a line is *parsed*, and the table is the
	// runner's, so the front end is the only place the two can be joined.
	// This works because execute reads a line at a time: the `alias` on one
	// line has run by the time the next is read, which is exactly the rule
	// every shell has — an alias is never expanded on the line that defines
	// it.
	//
	// An interactive shell expands them whatever the dialect says, which is
	// measured unanimous: all four expand an alias in `sh -i script.sh`, and
	// bash is the one that would not have in `sh script.sh`. This arm was
	// unreachable until now — the field it read meant "took the prompt
	// route", and the prompt route does not come through here.
	if sh.Dialect.ExpandAliases.Has(in.aliasRoute()) || in.interactive {
		pr.aliases = r.LookupAlias
	}
	if sh.Prelude != "" {
		if code := sh.source(r, name); code != 0 {
			return code
		}
	}
	// The invocation's own options, in effect before the script's first line
	// the way the panel has them — `sh -e script.sh` fails on the first
	// failing command, and `$-` says so from the start. After the prelude,
	// which is the dialect's plumbing rather than the user's text: tracing
	// it under `-x` or killing it under `-e` would be reporting on machinery
	// nobody wrote.
	if code, ok := sh.applyOptions(r, in.opts); !ok {
		return code
	}
	// The environment's own option list, after the argument vector and before
	// the files — both measured. An inherited `xtrace` beats the invocation's
	// own `+x`, so the environment is read second; and the startup files below
	// are traced by it, so it is read before them.
	r.ApplyInheritedShellOptions()
	// The startup files, before the script. After the options, which is where
	// the panel has them: `-x` given to the invocation traces the profile's
	// own lines in dash, ksh93 and zsh alike. After the runner is built rather
	// than before, because they are run *by* this shell and see what it sees —
	// measured, `$0` and `$#` inside ~/.profile are the script's in dash and
	// ksh93.
	//
	// The same call the prompt route makes, which is what it took to make
	// `sh -i script.sh` read a person's run-commands file: measured, `bash -i
	// -c cmd` reads `~/.bashrc`, and one shell reads a file on *every*
	// invocation whether or not there is anyone to prompt.
	if code := sh.startup(r, in); code != 0 {
		return code
	}
	if r.Exited() {
		// A startup file ended the shell, which is measured: `exit 3` in it
		// exits 3 and the script never runs, in all three of the shells that
		// read one. Through Finish, so an EXIT trap it set still fires.
		return r.Finish(context.Background())
	}
	if in.posix {
		// Called `sh`, so the shell starts in POSIX mode however the program
		// arrived — measured on the command string, on a script operand and
		// on standard input alike.
		//
		// Last, and both halves of that are measured. It is after the
		// invocation's own options because the name wins over them: `sh +o
		// posix -c …` still stops on a failed redirection where `bash +o
		// posix -c …` does not, while `+o errexit` on the same invocation is
		// honored, so this is the one option the name overrides rather than
		// the option loop being ignored. And it is after the profile because
		// the profile sees the mode *off* — `set -o` in a `~/.profile` read
		// by `-sh -l` reports `posix off`, and the script that follows it
		// stops all the same.
		//
		// Through the runner's own knob rather than by writing the axis,
		// because the mode has to be leavable: a script's `set +o posix` puts
		// back the answer this recorded on the way in, and an axis written
		// here directly would leave it nothing to find.
		r.SetPosixMode(true)
	}
	// Before the file below, which is the composition of the two: POSIX mode
	// suppresses that file, measured, and being called `sh` is one of the two
	// ways into the mode.
	if code := sh.nonInteractiveStartupFile(r, in); code != 0 {
		return code
	}
	if r.Exited() {
		// The file ended the shell, which is measured: `exit 3` in it exits 3
		// and the script never runs. Through Finish, so an EXIT trap it set
		// still fires — the same shape the profile above has.
		return r.Finish(context.Background())
	}

	return sh.execute(r, pr, in)
}

// applyOptions installs the set options the invocation named, once the
// runner they apply to exists. A refusal — an unknown letter, a name the
// dialect does not have — has already been reported by the same machinery
// `set` uses, and stops the shell before anything runs, which is measured
// and unanimous.
//
// *What it exits with* is not unanimous, and the two spellings are read the
// same way here for that reason: the status is the dialect's on both. It was
// the front end's own `usageStatus` for a letter, so `zsh -q` exited 2 where
// zsh exits 1 while `zsh -o nosuchoption` already exited 1 — one question
// answered two ways because only one of the paths could carry an answer
// (#483).
func (sh Shell) applyOptions(r *interp.Runner, opts []optionSpec) (int, bool) {
	for _, o := range opts {
		apply := r.SetOptionLetters
		if o.isName {
			apply = r.SetNamedOption
		}
		if code := apply(o.spec, o.on); code != 0 {
			return code, false
		}
	}
	return 0, true
}

// execute reads and runs the script a line at a time.
//
// A shell runs what it has read rather than reading everything first, so
// `echo one` on line 1 runs before line 3 fails to parse. The line is the
// unit and not the statement: with `echo one; { fi; }` on one line nothing
// runs, so a whole line is parsed before any of it is.
//
// However the run ended, the EXIT trap fires, which is why every ending
// returns through Finish rather than around it. The refusal used to return
// around it and was the only ending that did — a silent exception to the rule
// beside it, and one no input in the corpus reaches, so nothing said so. The
// library had it right all along: interp's own Run runs the EXIT trap on the
// same error, and only this front end disagreed.
func (sh Shell) execute(r *interp.Runner, pr *program, in source) int {
	ctx := context.Background()
	status, how := sh.executeLines(ctx, r, pr, in)
	switch how {
	case endingParseFailure, endingRefused:
		// The EXIT trap fires even when the last thing read would not parse,
		// which is unanimous across the panel — and equally when the front
		// end refused to run what it was given. A script's cleanup is not
		// conditional on why the script stopped, and the refusal used to be
		// the one ending that skipped it. The status is the failure's rather
		// than the trap's, in both.
		r.Finish(ctx)
		return status
	}
	return r.Finish(ctx)
}

// ending is how a run of lines stopped, which decides what the caller owes the
// shell afterwards. Split out from execute so that a front end holding a shell
// open across several inputs — a prompt, an agent protocol session — can run
// one input without ending the shell, and can still say what just happened.
//
// Three endings and **two** answers: a run that read everything exits with
// the shell's own status, and one that stopped on something exits with that
// something's — and either way the shell ends through Finish. The refusal
// used to be a third answer, which was the bug. What still tells it from a
// parse failure is which diagnostic executeLines wrote, not what it leaves
// the caller to do, so a switch that separates the two here is describing a
// difference that no longer exists.
type ending int

const (
	// endingRanOut is the ordinary case: the input was all read and run.
	endingRanOut ending = iota
	// endingParseFailure is a parse failure that ended the run.
	endingParseFailure
	// endingRefused is the front end declining to run it at all, which is a
	// usage error rather than anything the script did.
	//
	// It is reached: a builtin this shell must answer itself and cannot —
	// `enable -n umask` and then `umask 077` — comes back from RunPart as an
	// error rather than as a status, because it is the interpreter saying it
	// has nothing to run and not a command that failed. It ends the shell
	// like any other ending, EXIT trap included.
	endingRefused
)

// executeLines runs the program a line at a time, and never ends the shell.
//
// A shell runs what it has read rather than reading everything first, so
// `echo one` on line 1 runs before line 3 fails to parse. The line is the
// unit and not the statement: with `echo one; { fi; }` on one line nothing
// runs, so a whole line is parsed before any of it is.
func (sh Shell) executeLines(
	ctx context.Context, r *interp.Runner, pr *program, in source,
) (int, ending) {
	shown := 0
	var echoed verbosePos
	// A builtin can change the grammar for the lines after it — a run-time
	// option can decide whether a quantified group is a group. The runner
	// says so by replacing its Dialect, never by writing through it, so a
	// changed pointer is the whole signal; the front end is the only place
	// the runner and the parser meet, exactly as it is for aliases.
	dialect := r.Dialect
	for {
		if r.Dialect != dialect && r.Dialect != nil {
			dialect = r.Dialect
			pr.setDialect(*dialect)
		}
		line, ok := pr.nextLine()
		// Said as soon as it is known and before anything the line does,
		// which is where the one shell that remarks puts it.
		shown = sh.sayRemarks(in.dg, in.diagName(), pr.remarks(), shown)
		if !ok {
			if err := pr.err(); err != nil {
				// The input ended part-way through something. Reported here
				// rather than below, because a program read as it runs has no
				// line to hand back when the last of it is unfinished.
				// No readOn here, and it is not an oversight: this branch
				// is reached when the parser could return nothing at all,
				// which on this route means the input ran out part-way
				// through a construct. There is nothing left to read on to,
				// so the shell that reads on and the shell that stops end
				// the same way — measured, an unterminated quote piped in
				// is one complaint and status 1 in all four.
				sh.errf("%s", in.dg.ParseDiagnostic(in.diagName(), in.input, err, pr.text()))
				return in.dg.StatusForParseError(err), endingParseFailure
			}
			// Whatever is left once the last line has been handed out is
			// still input the shell read, and `set -v` writes back what it
			// reads. Blank lines and comments *between* commands come out
			// with the line after them; the ones after the last command have
			// no line after them and were dropped outright.
			sh.sayVerboseRest(pr.text(), echoed, r.Verbose())
			break
		}
		if err := pr.err(); err != nil {
			// The line did not parse, so none of it runs — not even the
			// statements before the failure, which is measured.
			sh.errf("%s", in.dg.ParseDiagnostic(in.diagName(), in.input, err, pr.text()))
			if sh.readOn(r, pr, in, err) {
				continue
			}
			return in.dg.StatusForParseError(err), endingParseFailure
		}
		// `set -v` — the input written back as it is read. The front end is
		// the one holding the raw text, which is why the echo lives here: the
		// runner only says whether the option is on. Accounted for up to the
		// line the parser has consumed, so a here-document's body goes out
		// with the line that owns it, and never twice.
		//
		// Passed over rather than skipped when the option is off, because the
		// position has to keep up either way: lines read while it is off are
		// spent, not saved — the line that says `set -v` is not echoed by the
		// shell it turns on — and the line after it can only be found once
		// every line before it has been walked past.
		echoed = sh.sayVerbose(pr.text(), line.Last.Line, echoed, r.Verbose())
		if err := r.RunPart(ctx, line); err != nil {
			// Refused rather than silently doing nothing: a shell that
			// quietly skips what it cannot do is worse than one that says so.
			sh.errf("%s", in.dg.Report(in.diagName(), 1, err.Error()+"\n"))
			return usageStatus, endingRefused
		}
		if r.Exited() {
			break
		}
	}
	// The status of a run that read everything is the shell's own, which the
	// caller reads once it has decided whether to end the shell.
	return 0, endingRanOut
}

// verbosePos is how far `set -v` has walked through the input: the physical
// lines already accounted for, and the byte just after them.
//
// The offset is the whole of the type and the reason it is a type. Without it
// the only record of where the echo has got to is a line *number*, and finding
// the text of a line from its number means splitting the program on newlines —
// so an n-line script split an n-line string n times, allocating a slice of
// every line in the program for every line it echoed. `set -v` on 8 000 lines
// cost 0.57 s where the same script without it cost 0.011 s, and doubling the
// script quadrupled that: the shell was at its slowest exactly when it was
// being asked to explain itself (#580). A byte offset carried alongside the
// number makes the whole echo one pass over the text.
type verbosePos struct {
	// line is how many physical lines have been passed, and off the byte in
	// the program text where the next one starts. off runs one past the end
	// of the text once the piece after the final newline has been passed,
	// which is a position strings.Split has and a byte offset otherwise
	// cannot distinguish from resting on it.
	line int
	off  int
}

// sayVerbose accounts for the physical lines up to and including upTo,
// resuming where it left off and reporting how far it got. It writes them when
// echo says to and passes over them silently when it does not.
func (sh Shell) sayVerbose(src string, upTo int, at verbosePos, echo bool) verbosePos {
	if upTo <= at.line {
		return at
	}
	for at.line < upTo && at.off <= len(src) {
		rest := src[at.off:]
		text := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			text, at.off = rest[:i], at.off+i+1
		} else {
			// The piece after the last newline, which the input may yet
			// continue but which is a line of its own for as long as this is
			// all there is. One past the end says it has been taken.
			at.off = len(src) + 1
		}
		if echo {
			sh.errf("%s\n", text)
		}
		at.line++
	}
	// Where the text ran out before upTo did, the lines that are not there are
	// counted anyway rather than echoed again when more arrives.
	at.line = upTo
	return at
}

// sayVerboseRest accounts for the physical lines left over once the last
// logical line has been handed out, which is the tail of the input: the blank
// lines and comments after the final command, and a program that is nothing
// but those.
//
// It walks to the end of the text rather than to a line number, because there
// is no line to name — a line number is what the parser hands back for input
// it turned into a command, and this is exactly the input it did not. The end
// of the text is the only bound there is.
//
// Not called where the shell stopped early. A script that runs `exit` is done
// reading, and three of the four echo nothing after it; a line that failed to
// parse is a separate question with its own answer.
func (sh Shell) sayVerboseRest(src string, at verbosePos, echo bool) {
	for at.off < len(src) {
		rest := src[at.off:]
		text := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			text, at.off = rest[:i], at.off+i+1
		} else {
			// The piece after the last newline. One past the end says it has
			// been taken, which is the same convention sayVerbose keeps.
			at.off = len(src) + 1
		}
		if echo {
			sh.errf("%s\n", text)
		}
	}
}

// source runs the prelude on an existing runner, which is how a prelude is
// installed: no special entry point, just the same Run.
//
// A prelude that fails is the dialect being broken rather than the script, so
// it is reported plainly and never through the dialect's script wording.
func (sh Shell) source(r *interp.Runner, name string) int {
	f, err := syntax.Parse(sh.Prelude, sh.Dialect)
	if err == nil {
		_, err = r.Run(context.Background(), f)
		// The prelude is the dialect's plumbing and not a command the
		// script ran, so what it leaves in `$_` is not an answer about the
		// script. Without this the parameter arrived at the script's first
		// line already moved — empty, after the assignment bash's prelude
		// ends on — and the startup value was unreachable through any
		// binary that has a prelude at all.
		r.ForgetLastArgument()
	}
	if err != nil {
		sh.errf("%s: prelude: %v\n", name, err)
		return usageStatus
	}
	return 0
}

// sayRemarks writes what the parser had to say about input it accepted
// anyway, and reports how many have been written.
//
// The count is carried because a parser produces these as it reads, and the
// loop asks after every line: without it the first remark would be repeated
// for every line after the one that raised it.
func (sh Shell) sayRemarks(dg interp.Diagnostics, name string, rs []syntax.Remark, shown int) int {
	for _, rk := range rs[min(shown, len(rs)):] {
		if msg := dg.Remark(rk); msg != "" {
			sh.errf("%s", dg.Report(name, rk.Pos.Line, msg+"\n"))
		}
	}
	return len(rs)
}

// readOn settles whether a line that did not parse ends the shell, and leaves
// the reader ready for the next line where it does not.
//
// One dialect carries on, and only where the program itself arrived on
// standard input: the same lines in a file stop it. The route is the whole of
// the condition, which is why this is asked here rather than folded into the
// parse failure's status — see Diagnostics.StdinProgramSurvivesAParseFailure
// for the measurement.
//
// The status is recorded rather than returned. What runs after the failure
// reports as it always would, so `false` on the line after a bad one still
// exits 1 and `exit 7` still exits 7; the parse status is what is left when
// nothing runs after it at all, which is the same shell exiting 1 for a
// program whose last line is the bad one.
//
// The failed text has to be retired before reading on, because the parser
// keeps its error until the pending text is replaced — otherwise the next turn
// of the loop would find the same failure and report it again forever.
func (sh Shell) readOn(r *interp.Runner, pr *program, in source, err error) bool {
	if !in.onStdin || !in.dg.StdinProgramSurvivesAParseFailure {
		return false
	}
	r.SetExitStatus(in.dg.StatusForParseError(err))
	return pr.fill(true)
}
