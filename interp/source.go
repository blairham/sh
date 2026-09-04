// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/syntax"
)

// This file implements `eval` and `.`, the two special builtins that run shell
// source in *this* shell rather than a child.
//
// They were both in the specialBuiltins table long before either existed, so
// the table asserted something false: `eval echo hi` was `command not found`
// while IsSpecialBuiltin("eval") said yes. That is the gap this closes.
//
// The pair shares one engine because the pair is one idea — text, parsed, run
// on the current runner, so an assignment inside it survives. They differ in
// three measured ways, which is what `sourced` names.

// Registered here rather than in the builtins literal because these two are
// the only builtins that run arbitrary shell: each reaches the dispatcher,
// which reads the map they would be declared in, and Go rejects that as an
// initialization cycle. Assigning after the literal is built is the whole fix
// and costs nothing at the call site.
//
// `source` is deliberately absent. dash does not have it — `command -v source`
// says so — which makes that name a dialect's answer, added in its Apply.
func init() {
	builtins["eval"] = biEval
	builtins["."] = biDot
}

// sourced is how a run of borrowed text differs from the script around it.
type sourced struct {
	// label names the text in a diagnostic: "eval", or the path of the file.
	label string

	// eval marks the text as `eval`'s rather than a file's, because one
	// dialect calls it something of its own — `(eval)` — where the others
	// use the builtin's name.
	eval bool

	// syntaxStatus is what a parse failure reports when it is not fatal.
	// It is passed rather than read from the dialect because the two callers
	// disagree: measured, a parse failure inside `eval` carries the dialect's
	// ordinary syntax-error status, but inside a *sourced file* zsh reports
	// 126 where its own syntax-error status is 1.
	syntaxStatus int

	// catchReturn stops at `return` instead of letting it unwind the function
	// around it.
	//
	// True for `.` and false for `eval`, and measured both ways: `return`
	// inside a sourced file ends the source and becomes its status, while
	// `f() { eval return 3; echo no; }` returns from *f* — the `echo` does not
	// run in any shell in the panel. So eval is transparent to control flow
	// and `.` is a boundary for exactly one kind of it.
	catchReturn bool
}

// sourceName is what a diagnostic calls this text.
func (s sourced) sourceName(d Diagnostics) string {
	switch {
	case s.eval && d.EvalSourceName != "":
		return d.EvalSourceName
	case !s.eval && d.SourceFileIsTheBuiltin:
		// The builtin that read the file rather than the file itself.
		return "."
	}
	return s.label
}

// naming is where this kind of borrowed text puts its name.
func (s sourced) naming(d Diagnostics) SourceNaming {
	if s.eval {
		return d.EvalNaming
	}
	return d.SourceFileNaming
}

// runSourced parses src and runs it on this runner.
//
// The status is the last command's, or 0 when nothing ran — which is not the
// same as leaving the status alone. Measured: `false; eval ""` and `false;
// . empty.sh` both end at 0 in every shell in the panel, so an empty script
// *clears* a failure rather than preserving it.
func (r *Runner) runSourced(ctx context.Context, src string, s sourced) int {
	p := syntax.NewParser(src, r.dialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		// The failure's own line, not the caller's. `.` on line 1 of a script
		// that sources a file whose `if` never closes is reported at the
		// line in *that file* by every shell in the panel, and this reported
		// line 1 for all of them.
		line := r.line
		if at := r.diag().ParseFailureLine(err); at > 0 {
			line = at
		}
		d := r.diag()
		r.errf("%s\n", d.SourceReport(s.naming(d), r.name(), s.sourceName(d),
			line, d.ParseFailure(err)))
		// POSIX makes a special builtin's failure fatal to a non-interactive
		// shell. dash is the only member of the panel that does it here; the
		// other three report the error and carry on.
		if r.ask(r.sem().BuiltinSyntaxErrorFatal, "a parse failure inside a special builtin being fatal") {
			r.fatalQuiet()
			return r.status
		}
		return s.syntaxStatus
	}
	// Whatever a borrowed script reports is the *script's*, not this
	// builtin's. Measured: an unset parameter inside a sourced file is
	// `./f.sh:2: NOPE: parameter not set` in the one dialect that names
	// builtins at all, with no `.` anywhere in it. Leaving the marker set
	// would have put one there on every line the script produced.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()

	// Cleared before the first command rather than after the last, which is
	// what makes an empty script report success: with nothing to run the loop
	// below never executes and this is the answer. An `if len(f.Stmts) == 0`
	// guard stood here too and was dead — removing it changed no test, and
	// removing this line failed two.
	if s.catchReturn {
		// Inside a sourced file there is something for a `return` to return
		// from, which is the question one dialect asks before allowing one
		// at all.
		r.sourceDepth++
		defer func() { r.sourceDepth-- }()
	}
	r.status = 0
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			// A Builtin returns a status and not an error, so there is nowhere
			// for this to go but a diagnostic — the same place runTrapBody
			// puts it, and for the same reason.
			r.diagf("%s: %v\n", s.label, err)
			return 2
		}
		if r.ctl != controlNone {
			break
		}
	}
	if s.catchReturn && r.ctl == controlReturn {
		r.ctl = controlNone
	}
	return r.status
}

// parseMessage strips the parser's own "line:col: " prefix.
//
// The parser reports a position because a caller drawing a caret needs one; a
// diagnostic already says where it happened in the dialect's own shape, and
// leaving this on printed it twice.
func parseMessage(err error) string {
	var se *syntax.Error
	if errors.As(err, &se) {
		return se.Msg
	}
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.IndexFunc(msg[:i], func(r rune) bool {
		return r != ':' && (r < '0' || r > '9')
	}) < 0 {
		return msg[i+2:]
	}
	return msg
}

// biEval joins its arguments and runs the result.
//
// The join is with a single space and it is the *arguments* that are joined,
// not the original words: `eval echo a b c` prints "a b c" because eval
// rejoins them, and `eval "echo" "a""b"` prints "ab" because the shell had
// already joined those two into one word before eval saw anything. Both are
// unanimous across the panel.
func biEval(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		// Not r.status: an eval with nothing to run reports success even
		// after a failure, which is measured and unanimous.
		return 0
	}
	return r.runSourced(ctx, strings.Join(args, " "), sourced{
		eval:         true,
		label:        "eval",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
}

// biDot implements `.`, and `source` where a dialect registers that name too.
//
// `source` is deliberately not in the builtins table: dash does not have it,
// and `command -v source` says so, which makes it a dialect's answer rather
// than the substrate's. The dialects that have it add it in Apply.
func biDot(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		// Four answers in the panel, so this asks two questions rather than
		// guessing. dash calls a missing operand success and does nothing at
		// all; the rest call it an error, and ksh93 alone makes it fatal.
		if !r.ask(r.sem().DotWithNoOperandIsAnError, "`.` with no operand being an error") {
			return 0
		}
		usage := Wording(r.diag().DotNoOperand, ".: filename argument required")
		if r.diag().DotNoOperandUnprefixed {
			r.errf("%s\n", usage)
		} else {
			r.diagf("%s\n", usage)
		}
		// The status is the measured one either way, rather than the generic
		// fatal status: ksh93 ends the script here *and* reports 2, where the
		// fatal status it uses everywhere else is 1. Getting that from
		// fatalQuiet gave 1 and was wrong by one.
		status := r.diag().dotNoOperandStatus()
		if r.ask(r.sem().DotMissingFileFatal, "`.` with no operand being fatal") {
			r.status = status
			r.ctl = controlExit
		}
		return status
	}

	path, err := r.resolveDotPath(args[0])
	if err != nil {
		return r.dotFailed(args[0], err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return r.dotFailed(args[0], err)
	}

	// Arguments after the file become its positional parameters, and are put
	// back afterwards. dash is the exception: it ignores them, so a script
	// there still sees the caller's `$1`. With no arguments at all, every
	// shell leaves the parameters alone — which is why this is guarded on
	// having some rather than on the axis.
	if len(args) > 1 {
		pass := r.ask(r.sem().DotPassesArguments, "`.` giving a sourced file its own positional parameters")
		if r.unspecified {
			// Refused before the file runs rather than after. A sourced file
			// that sees its own `$1` and one that sees the caller's are two
			// different programs, so there is no version of running it that
			// is not a guess — and the refusal would be lost anyway: the flag
			// is cleared per command, so the first line of the file would
			// erase it.
			return 2
		}
		if pass {
			saved := r.Params
			r.Params = append([]string(nil), args[1:]...)
			defer func() { r.Params = saved }()
		}
	}

	// A frame of its own, because a sourced file is a place a script can be
	// *in*: the functions it declares remember it, and a script asking where
	// it is while being sourced means the file rather than whatever sourced
	// it. Named for the builtin, which is what the shells put in the stack.
	r.pushFrame(Frame{File: path, Name: "source"})
	defer r.popFrame()

	st := r.runSourced(ctx, string(b), sourced{
		label:        path,
		syntaxStatus: r.diag().sourcedSyntaxStatus(),
		catchReturn:  true,
	})
	// The RETURN trap fires as a sourced file finishes — wherever the trap
	// was set, which is the half of the rule functions do not share. The
	// action sees the file's status, and an `exit` of its own wins.
	r.status = st
	r.runReturnTrap(ctx, sourcedFrame)
	return r.status
}

// dotFailed reports a file `.` could not read.
//
// A bare name PATH did not have and a path that would not open are the same
// failure to three of the four and two different messages in dash — "not
// found" for the first and "cannot open …" for the second — so the format is
// chosen by which it was.
func (r *Runner) dotFailed(name string, err error) int {
	format := r.diag().DotCannotOpen
	if errors.Is(err, errNotOnPath) && r.diag().DotNotFound != "" {
		format = r.diag().DotNotFound
	}
	r.diagf("%s\n", Wording(format, ".: %[1]s: %[2]s", name, reason(err)))
	// dash and ksh93 end the script here; bash and zsh report it and go on.
	if r.ask(r.sem().DotMissingFileFatal, "`.` failing to open a file being fatal") {
		r.fatalQuiet()
		return r.status
	}
	return r.diag().dotCannotOpenStatus()
}

// reason is the part of an os error a shell prints, without the operation and
// the path it already named itself.
//
// Capitalized, because every shell in the panel does: they print the C
// strerror text — "No such file or directory" — where Go's syscall.Errno
// lowercases it. Four dialects differed from the real shell by that one letter
// until this was here.
func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	msg := err.Error()
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// errNotOnPath is what resolveDotPath returns when no candidate existed. It
// carries no path because there is no one file to name.
var errNotOnPath = errors.New("no such file or directory")

// resolveDotPath finds the file `.` should read.
//
// An operand with a slash in it is a path and is used as written. Without one
// it is searched on PATH — which surprises people, and is POSIX, and is
// unanimous in the panel, including that PATH wins over an identically named
// file in the current directory.
//
// Only if PATH misses does the current directory come into it, and only in
// bash: measured, `PATH=/usr/bin:/bin; . dotcwd.sh` finds the file in bash and
// is "not found" in dash, ksh93 and zsh.
// It returns the path to *read*, already resolved against this runner's
// directory, because that is the only form the caller can safely open.
func (r *Runner) resolveDotPath(name string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return r.atDir(name), nil
	}
	path, _ := r.getVar("PATH")
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			// An empty PATH element means the current directory, which is a
			// POSIX rule and is not the same as the bash fallback below: this
			// one was asked for.
			dir = "."
		}
		if candidate := r.atDir(filepath.Join(dir, name)); readableFile(candidate) {
			return candidate, nil
		}
	}
	if r.ask(r.sem().DotFallsBackToCurrentDirectory, "`.` looking in the current directory after PATH misses") {
		if candidate := r.atDir(name); readableFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w", errNotOnPath)
}

// atDir resolves a relative path against *this runner's* directory rather than
// the process's.
//
// A Runner carries its own Dir precisely so that two of them embedded in one
// program do not fight over a single process-wide cwd, and reaching for
// os.Stat or os.ReadFile on a bare relative path quietly opts out of that. It
// passed in a test that happened to run from the right directory and found
// nothing anywhere else. redirect.go resolves paths the same way, for the same
// reason.
func (r *Runner) atDir(path string) string {
	if !filepath.IsAbs(path) && r.Dir != "" {
		return filepath.Join(r.Dir, path)
	}
	return path
}

// readableFile reports whether a path is something `.` could read, which
// excludes a directory: `. /tmp` is not a source file in any shell that
// reports it at all.
func readableFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
