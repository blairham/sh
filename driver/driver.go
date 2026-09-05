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
	"strings"

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

	// Register adds or removes builtins — the part of a dialect that shell
	// cannot express. Nil means the dialect needs none, which is the common
	// case now that cd, pwd and read live in the core.
	Register func(*interp.Runner)

	// Stdin is where an interactive shell reads its lines from, and has to be
	// a terminal for the editor to work. A script's input is the script, so
	// nothing else here reads it.
	Stdin *os.File

	// Stdout and Stderr default to the process's. Tests set them; a binary
	// leaves them alone.
	Stdout, Stderr io.Writer

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
		sh.errf("%s: %v\n", sh.Name, err)
		return usageStatus
	}
	if in.interactive {
		// A prompt rather than a script, and reached from here so that every
		// binary built on this front end has one. It was in a single binary's
		// main once, which is how `sh` learned to prompt and `bash` did not.
		//
		// With whatever parameters the invocation supplied: `sh -s one two`
		// sets `$1` at a prompt in all four.
		return sh.interactive(argv, in.params, in.opts)
	}
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
	// interactive says there is nothing to run and a person at a keyboard,
	// so the shell should prompt rather than read a script. It travels on
	// the source because deciding it is part of reading the invocation:
	// `-i`, or no operands and a terminal on standard input.
	interactive bool
	// file is the path the input was read from, or empty where there was no
	// file — `-c`, or standard input. It is the floor of the call stack: a
	// script asking where it is means this, and only the front end knows.
	file string
	// wholeFirst parses the whole input before running any of it, which one
	// dialect does for a command string and no dialect does for a script.
	wholeFirst bool
	dg         interp.Diagnostics
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
	opts      []optionSpec
}

func (sh Shell) input(argv []string) (source, error) {
	args := argv
	if len(args) > 0 {
		args = args[1:]
	}

	var inv invocation

	// Hand-parsed rather than with the flag package, because a shell's
	// conventions are not Go's: options stop at the first operand, `-c` takes
	// either the rest of its word or the next argument, a `+` turns an
	// option off, and the words after a command string must not be claimed
	// as more flags.
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
			rest, src, done, err := sh.optionWord(a, args[1:], &inv)
			if err != nil {
				return source{}, err
			}
			if done {
				return src, nil
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
// read, or the source itself when a letter ends the loop the way `-c` does.
//
// The letters this front end owns are the ones that say where the script
// comes from — `c`, `i`, `s` — and they bundle with the rest: `sh -ec cmd`
// is unanimous across the panel. Every other letter is a `set` option and
// stays text here; whether the shell has it is the dialect's question,
// answered by the same machinery `set` uses once the runner exists.
func (sh Shell) optionWord(a string, args []string, inv *invocation) (rest []string, src source, done bool, err error) {
	on := a[0] == '-'
	body := a[1:]
	letters := ""
	flush := func() {
		if letters != "" {
			inv.opts = append(inv.opts, optionSpec{spec: letters, on: on})
			letters = ""
		}
	}
	for k := 0; k < len(body); k++ {
		switch ch := body[k]; {
		case ch == 'c' && on:
			// The command string: the rest of this word if there is any —
			// `-c'echo hi'` as a single word, which getopt allows — and the
			// next argument otherwise. Anything after the command word is a
			// positional parameter rather than another option. The letters
			// before it in a bundle still count: `sh -ec cmd` is errexit
			// and a command, in all four shells.
			flush()
			if cmd := body[k+1:]; cmd != "" {
				s := commandSource(sh, cmd, args)
				s.opts = inv.opts
				return nil, s, true, nil
			}
			if len(args) < 1 {
				return nil, source{}, false, errors.New("-c requires an argument")
			}
			s := commandSource(sh, args[0], args[1:])
			s.opts = inv.opts
			return nil, s, true, nil
		case ch == 'i' && on:
			inv.forcePrompt = true
		case ch == 's' && on:
			inv.fromStdin = true
		case ch == 'o':
			// The long spelling, whose name is the next word — read at the
			// end of a bundle exactly as `set` reads it, so `sh -euo
			// pipefail script` works the way the line at the top of so many
			// scripts does. Elsewhere in a word it is refused: measured,
			// the panel splits over `-oNAME` — zsh and ksh93 read the rest
			// of the word as the name, bash and dash read it as more
			// letters — so the core takes neither side.
			if k != len(body)-1 {
				return nil, source{}, false, fmt.Errorf("unknown option %q", a)
			}
			flush()
			if len(args) < 1 {
				// Refused rather than listed. With no name at all three of
				// the four print the option table and read on; zsh refuses
				// with "string expected after -o". A listing at invocation
				// is not worth the machinery until something needs it, and
				// refusing is the honest half of a split panel.
				return nil, source{}, false, fmt.Errorf("%s requires an argument", a)
			}
			inv.opts = append(inv.opts, optionSpec{spec: args[0], isName: true, on: on})
			return args[1:], source{}, false, nil
		default:
			// A set option's letter, ours to carry and the dialect's to
			// judge. An unknown one is refused before anything runs, just
			// not here: the front end has no option table of its own, so
			// `sh -Q` is refused by the runner the way `set -Q` would be.
			letters += string(ch)
		}
	}
	flush()
	return args, source{}, false, nil
}

// operands handles what is left once the options are gone: a script path, or
// nothing at all, which means standard input.
func (sh Shell) operands(args []string, inv invocation) (source, error) {
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
			return source{interactive: true, name: sh.Name, params: args, dg: sh.Diagnostics, opts: inv.opts}, nil
		}
		// sh.Stdin, not os.Stdin: a Runner's streams are its own, and a
		// front end that reaches past them is not usable by anything that
		// embeds it — including its own tests, where the difference is that
		// a test for the standard-input path silently reads the *test
		// binary's* input and passes whatever it is given.
		src, err := readAll(sh.Stdin)
		return source{src: src, name: sh.Name, params: args, dg: sh.Diagnostics.ForStdin(), opts: inv.opts}, err
	}
	path := args[0]
	b, err := os.ReadFile(path)
	if err != nil {
		return source{}, err
	}
	// A shell running a script names the *script* in `$0` and in every
	// diagnostic, not itself, and reports in the script form — ksh93 also
	// changes how it names the line.
	return source{
		src: string(b), name: path, file: path,
		params: args[1:], dg: sh.Diagnostics.ForScript(), opts: inv.opts,
	}, nil
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

func readAll(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	return string(b), err
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
func (sh Shell) newRunner(name string, params []string, dg interp.Diagnostics, commandString bool) *interp.Runner {
	r := &interp.Runner{
		// Where the program came from, which one dialect answers a failed
		// expansion by.
		CommandString: commandString,
		Dialect:       &sh.Dialect,
		Semantics:     &sh.Semantics,
		Diagnostics:   &dg,
		Name:          name,
		// `$1` onward. A nil slice and an empty one mean the same thing to
		// the interpreter, so nothing distinguishes "no operands" from
		// "operands that were all consumed as the name".
		Params: params,
		// The process's environment, read here and not in interp: os.Environ
		// answers for the whole process, and a library Runner must take the
		// environment it is handed rather than reach for shared state. This
		// binary *is* the process, so the read is made once, where it is
		// visible — the same split as ReplaceProcess below.
		Env:    os.Environ(),
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
	}
	if wd, err := os.Getwd(); err == nil {
		// And where the process is, for the same reason: interp treats an
		// empty Dir as "stay relative" and never calls os.Getwd itself. A
		// failed Getwd — the directory was deleted under the process — leaves
		// Dir empty, which is that relative reading and the best available.
		r.Dir = wd
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
		// And it may resume one it stopped, which is a signal leaving this
		// process and so the binary's to send.
		r.SignalGroup = signalGroup
		// And it hands the terminal to whatever it is running, which is what
		// makes ^C and ^Z reach the command rather than the shell.
		r.Foreground = foreground
	}
	if sh.Register != nil {
		// The dialect's own adjustment: what it adds to or removes from the
		// substrate's builtins, which is neither grammar nor semantics.
		sh.Register(r)
	}
	return r
}

func (sh Shell) run(in source) int {
	src, name, input, dg := in.src, in.name, in.input, in.dg
	p := syntax.NewParser(src, sh.Dialect)
	// One dialect reads a command string whole before running any of it, and
	// the rest run each line as they reach it. Parsing everything up front is
	// how that is done: the failure is then reported before anything has run.
	if in.wholeFirst {
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
			sh.sayRemarks(dg, name, p, 0)
			sh.errf("%s", dg.ParseDiagnostic(name, input, err, src))
			return dg.StatusForParseError(err)
		}
		p = syntax.NewParser(src, sh.Dialect)
	}

	r := sh.newRunner(name, in.params, dg, input == "-c")
	r.SetScriptFile(in.file)
	// Aliases are expanded when a line is *parsed*, and the table is the
	// runner's, so the front end is the only place the two can be joined.
	// This works because execute reads a line at a time: the `alias` on one
	// line has run by the time the next is read, which is exactly the rule
	// every shell has — an alias is never expanded on the line that defines
	// it.
	if sh.Dialect.ExpandAliases || in.interactive {
		p.Aliases = r.LookupAlias
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

	return sh.execute(r, p, in)
}

// applyOptions installs the set options the invocation named, once the
// runner they apply to exists. A refusal — an unknown letter, a name the
// dialect does not have — has already been reported by the same machinery
// `set` uses, and stops the shell before anything runs, which is measured
// and unanimous.
func (sh Shell) applyOptions(r *interp.Runner, opts []optionSpec) (int, bool) {
	for _, o := range opts {
		if o.isName {
			if code := r.SetNamedOption(o.spec, o.on); code != 0 {
				return code, false
			}
			continue
		}
		if !r.SetOptionLetters(o.spec, o.on) {
			return usageStatus, false
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
// The EXIT trap fires either way, which is why the parse failure returns
// through Finish rather than around it.
func (sh Shell) execute(r *interp.Runner, p *syntax.Parser, in source) int {
	ctx := context.Background()
	shown, echoed := 0, 0
	// A builtin can change the grammar for the lines after it — a run-time
	// option can decide whether a quantified group is a group. The runner
	// says so by replacing its Dialect, never by writing through it, so a
	// changed pointer is the whole signal; the front end is the only place
	// the runner and the parser meet, exactly as it is for aliases.
	dialect := r.Dialect
	for {
		if r.Dialect != dialect && r.Dialect != nil {
			dialect = r.Dialect
			p.SetDialect(*dialect)
		}
		line, ok := p.NextLine()
		// Said as soon as it is known and before anything the line does,
		// which is where the one shell that remarks puts it.
		shown = sh.sayRemarks(in.dg, in.name, p, shown)
		if !ok {
			break
		}
		if err := p.Err(); err != nil {
			// The line did not parse, so none of it runs — not even the
			// statements before the failure, which is measured.
			sh.errf("%s", in.dg.ParseDiagnostic(in.name, in.input, err, in.src))
			r.Finish(ctx)
			return in.dg.StatusForParseError(err)
		}
		if r.Verbose() {
			// `set -v` — the input written back as it is read. The front end
			// is the one holding the raw text, which is why the echo lives
			// here: the runner only says whether the option is on. Echoed up
			// to the line the parser has consumed, so a here-document's body
			// goes out with the line that owns it, and never twice.
			echoed = sh.sayVerbose(in.src, line.Last.Line, echoed)
		} else {
			// Lines read while the option is off are spent, not saved: the
			// line that says `set -v` is not echoed by the shell it turns on.
			echoed = line.Last.Line
		}
		if err := r.RunPart(ctx, line); err != nil {
			// Refused rather than silently doing nothing: a shell that
			// quietly skips what it cannot do is worse than one that says so.
			sh.errf("%s", in.dg.Report(in.name, 1, err.Error()+"\n"))
			return usageStatus
		}
		if r.Exited() {
			break
		}
	}
	return r.Finish(ctx)
}

// sayVerbose writes the physical lines up to and including upTo, resuming
// after the last one already written, and reports how far it got.
func (sh Shell) sayVerbose(src string, upTo, echoed int) int {
	if upTo <= echoed {
		return echoed
	}
	lines := strings.Split(src, "\n")
	for i := echoed; i < upTo && i < len(lines); i++ {
		sh.errf("%s\n", lines[i])
	}
	return upTo
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
func (sh Shell) sayRemarks(dg interp.Diagnostics, name string, p *syntax.Parser, shown int) int {
	rs := p.Remarks()
	for _, rk := range rs[min(shown, len(rs)):] {
		if msg := dg.Remark(rk); msg != "" {
			sh.errf("%s", dg.Report(name, rk.Pos.Line, msg+"\n"))
		}
	}
	return len(rs)
}
