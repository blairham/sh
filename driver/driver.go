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

	// Register adds or removes builtins — the part of a dialect that shell
	// cannot express. Nil means the dialect needs none, which is the common
	// case now that cd, pwd and read live in the core.
	Register func(*interp.Runner)

	// Stdout and Stderr default to the process's. Tests set them; a binary
	// leaves them alone.
	Stdout, Stderr io.Writer

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

// RunScript is Run for input that came from a file, which selects the
// script-form diagnostics and names the script rather than the shell.
func RunScript(sh Shell, src, path string) int {
	sh = sh.withDefaults(nil)
	return sh.run(source{src: src, name: path, dg: sh.Diagnostics.ForScript()})
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
	// wholeFirst parses the whole input before running any of it, which one
	// dialect does for a command string and no dialect does for a script.
	wholeFirst bool
	dg         interp.Diagnostics
}

func (sh Shell) input(argv []string) (source, error) {
	args := argv
	if len(args) > 0 {
		args = args[1:]
	}

	// Hand-parsed rather than with the flag package, because a shell's
	// conventions are not Go's: options stop at the first operand, `-c` takes
	// either the rest of its word or the next argument, and the words after
	// the command must not be claimed as more flags.
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--":
			return sh.operands(args[1:])
		case a == "-c":
			if len(args) < 2 {
				return source{}, errors.New("-c requires an argument")
			}
			// Anything after the command word is a positional parameter
			// rather than another option. Nothing here sets them yet; taking
			// them at all is what keeps that a gap rather than a misparse.
			return commandSource(sh, args[1], args[2:]), nil
		case strings.HasPrefix(a, "-c") && len(a) > 2:
			// `-c'echo hi'` as a single word, which getopt allows.
			return commandSource(sh, a[2:], args[1:]), nil
		case a == "-s":
			// The explicit "read standard input" spelling.
			s, err := readAll(os.Stdin)
			// Standard input keeps the shell's own name, so every operand
			// after `-s` is a parameter and none of them is `$0`.
			return source{src: s, name: sh.Name, params: args[1:], dg: sh.Diagnostics}, err
		case a == "-":
			// A lone `-` ends the options, exactly as `--` does. It does not
			// mean "read standard input": every shell in the panel treats
			// `sh - a b` as running the script `a`, and only a `-` with
			// nothing after it falls through to standard input.
			return sh.operands(args[1:])
		case !strings.HasPrefix(a, "-"):
			return sh.operands(args)
		default:
			return source{}, fmt.Errorf("unknown option %q", a)
		}
	}
	return sh.operands(args)
}

// operands handles what is left once the options are gone: a script path, or
// nothing at all, which means standard input.
func (sh Shell) operands(args []string) (source, error) {
	if len(args) == 0 {
		src, err := readAll(os.Stdin)
		return source{src: src, name: sh.Name, dg: sh.Diagnostics}, err
	}
	path := args[0]
	b, err := os.ReadFile(path)
	if err != nil {
		return source{}, err
	}
	// A shell running a script names the *script* in `$0` and in every
	// diagnostic, not itself, and reports in the script form — ksh93 also
	// changes how it names the line.
	return source{src: string(b), name: path, params: args[1:], dg: sh.Diagnostics.ForScript()}, nil
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
			sh.errf("%s", dg.ParseDiagnostic(name, input, err, src))
			return dg.StatusForParseError(err)
		}
		p = syntax.NewParser(src, sh.Dialect)
	}

	r := &interp.Runner{
		Dialect:     &sh.Dialect,
		Semantics:   &sh.Semantics,
		Diagnostics: &dg,
		Name:        name,
		// `$1` onward. A nil slice and an empty one mean the same thing to
		// the interpreter, so nothing distinguishes "no operands" from
		// "operands that were all consumed as the name".
		Params: in.params,
		Stdout: sh.Stdout,
		Stderr: sh.Stderr,
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
	}
	if sh.Register != nil {
		// The dialect's own adjustment: what it adds to or removes from the
		// substrate's builtins, which is neither grammar nor semantics.
		sh.Register(r)
	}
	if sh.Prelude != "" {
		if code := sh.source(r, name); code != 0 {
			return code
		}
	}

	return sh.execute(r, p, in)
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
	for {
		line, ok := p.NextLine()
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
