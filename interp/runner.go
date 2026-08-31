// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Runner executes a syntax tree.
//
// This is a first slice, and the honest boundary is stated rather than
// discovered: it runs simple commands, assignments, and-or lists, and file
// redirections. Pipelines, subshells, compound commands and builtins are not
// here yet. Everything it does not implement is refused with a message saying
// so, never silently skipped — a shell that quietly does nothing is worse than
// one that says it cannot.
type Runner struct {
	// Vars holds shell variables. A nil map is initialised on first use.
	Vars map[string]string
	// Arrays holds indexed array variables, which are a different kind of
	// thing from Vars rather than a formatting of one: an element can hold a
	// space without becoming two.
	Arrays map[string][]string
	// Params holds the positional parameters, $1 first. `$0` is not one of
	// them and is kept separate, because `shift` moves these and never
	// touches that.
	Params []string
	// Name is `$0`.
	Name string
	// exported names go into a command's environment; the rest do not.
	exported map[string]bool
	// Dir is the working directory; empty means the process's own.
	Dir string
	// Env is the environment passed to commands. Nil means the process's own.
	Env []string
	// Dialect is what nested input — a command substitution, an `eval` — is
	// parsed with. Nil means the core, not the zero value: the zero Dialect
	// is posix and would refuse constructs the outer parse had accepted.
	Dialect *syntax.Dialect

	// Diagnostics is how failure is reported and which status it carries.
	// Nil means the substrate's own.
	Diagnostics *Diagnostics
	// Semantics is where the shells disagree about what identical syntax
	// means, as distinct from which syntax they accept. Nil means bash's.
	Semantics *Semantics

	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// Gate is consulted before every action that leaves this process. Nil
	// allows everything, so the zero value is usable.
	Gate Gate
	// Events receives what happened. Nil discards.
	Events Sink

	// status is the exit status of the last command run.
	status int
	// ctl carries break, continue and return out of a construct. They are
	// control flow rather than errors, so they are not returned as ones.
	ctl      control
	ctlDepth int
	// ctx is the context of the current Run, so expansion can reach it. A
	// command substitution runs commands, and threading a context through
	// every expander signature to reach one place would be worse.
	ctx context.Context
	// inFunc is the name of the function being run, for `$0`.
	inFunc string
	// expandErr records that an expansion failed — a division by zero, a
	// number that is not one. The command does not run, which is what every
	// shell in the panel does and what the exit status has to say.
	expandErr bool
	// unspecified records that a script depended on an axis no dialect had
	// answered, so a caller can tell that from an ordinary failure.
	unspecified bool
	// globMissed records that a pattern matched nothing, so the no-match
	// axis can report it once the whole field is known.
	globMissed bool
	// custom holds builtins registered by a shell built on this package. A
	// nil value is an explicit removal.
	custom map[string]Builtin
	// jobs are the background commands started by this shell.
	jobs    []*Job
	lastJob *Job
	// bg is set on the runner *inside* a background job, so the process it
	// starts can be recorded against the job.
	bg *Job
	// scopes is the stack `local` unwinds. Shell scoping is dynamic, so
	// there is one set of variables and this records what to put back.
	scopes []*scope
	// redirErr records that a redirection failed to open. The command must
	// not run: a redirect that could not be applied would otherwise send its
	// output to the terminal, which is the loudest possible wrong answer.
	redirErr bool
	// line is where execution currently is, for diagnostics that name it.
	// Real shells report the line of the command that failed, so this is
	// updated per statement rather than per token.
	line int
	// noclobber is `set -C`: a plain `>` will not truncate an existing file.
	noclobber bool
	// readonly names refuse assignment.
	readonly map[string]bool
	// funcs holds defined functions.
	funcs map[string]*syntax.FuncDecl
	// depth bounds function recursion, because a shell script can recurse
	// and a stack overflow is not a diagnostic anyone can act on.
	depth int
}

// maxDepth bounds nested function calls.
const maxDepth = 256

// clone copies the state for a subshell, so nothing it does escapes.
func (r *Runner) clone() *Runner {
	c := *r
	c.Vars = make(map[string]string, len(r.Vars))
	for k, v := range r.Vars {
		c.Vars[k] = v
	}
	c.exported = make(map[string]bool, len(r.exported))
	for k, v := range r.exported {
		c.exported[k] = v
	}
	c.Arrays = make(map[string][]string, len(r.Arrays))
	for k, v := range r.Arrays {
		c.Arrays[k] = append([]string(nil), v...)
	}
	c.Params = append([]string(nil), r.Params...)
	return &c
}

// withRedirs applies a compound command's redirections around its body. Every
// compound node carries its own list because a redirection on one applies to
// everything inside it.
func (r *Runner) withRedirs(ctx context.Context, rs []*syntax.Redirect, body func() error) error {
	closers, err := r.applyRedirs(ctx, rs)
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	if err != nil {
		return err
	}
	if r.redirErr {
		return nil
	}
	return body()
}

// ExitStatus reports the status of the last command.
func (r *Runner) ExitStatus() int { return r.status }

func (r *Runner) stdout() io.Writer {
	if r.Stdout == nil {
		return os.Stdout
	}
	return r.Stdout
}

func (r *Runner) stderr() io.Writer {
	if r.Stderr == nil {
		return os.Stderr
	}
	return r.Stderr
}

// errf writes a diagnostic to the shell's error stream.
//
// The write error is discarded deliberately: this is already the error path,
// there is nowhere better to report a failure to report, and a shell whose
// stderr is closed should still run the command.
func (r *Runner) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.stderr(), format, args...)
}

// diagf writes a diagnostic with the dialect's own prefix.
//
// Every message goes through here rather than spelling "sh: " itself, because
// the prefix is the dialect's answer and not this package's: dash, bash, ksh93
// and zsh each name the location differently, and one of them names it not at
// all.
func (r *Runner) diagf(format string, args ...any) {
	name := r.Name
	if name == "" {
		name = "sh"
	}
	r.errf("%s%s", r.diag().prefix(name, r.line), fmt.Sprintf(format, args...))
}

func (r *Runner) emit(ctx context.Context, e Event) {
	if r.Events != nil {
		r.Events.Emit(ctx, e)
	}
}

// allowed consults the gate. A refusal is reported and becomes a failing
// status rather than an abort: a denied command is a command that failed.
func (r *Runner) allowed(ctx context.Context, a Action) bool {
	if r.Gate == nil || r.Gate.Allow(ctx, a) == Allow {
		return true
	}
	r.emit(ctx, Event{Kind: EventDenied, Action: a})
	r.diagf("%s: refused: %s\n", a.Kind, a.Path)
	r.status = 126
	return false
}

// Run executes a whole file, returning the last command's status.
func (r *Runner) Run(ctx context.Context, f *syntax.File) (int, error) {
	r.ctx = ctx
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			return r.status, err
		}
		if r.ctl == controlExit {
			break
		}
	}
	return r.status, nil
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) error {
	if st.Expr != nil {
		r.line = st.Expr.Pos().Line
	}
	if st.Background {
		return r.background(ctx, st)
	}
	return r.expr(ctx, st.Expr)
}

func (r *Runner) expr(ctx context.Context, e syntax.Expr) error {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		if err := r.expr(ctx, x.X); err != nil {
			return err
		}
		// && runs the right side when the left succeeded, || when it failed.
		// Both are one level and left-associative, which the tree already
		// encodes; nothing here re-decides it.
		runRight := (x.Op == syntax.TokAndAnd) == (r.status == 0)
		if !runRight {
			return nil
		}
		return r.expr(ctx, x.Y)
	case *syntax.Pipeline:
		return r.pipeline(ctx, x)
	}
	return r.unsupported(fmt.Sprintf("%T", e))
}

func (r *Runner) pipeline(ctx context.Context, p *syntax.Pipeline) error {
	if len(p.Cmds) == 1 {
		if err := r.command(ctx, p.Cmds[0]); err != nil {
			return err
		}
	} else if err := r.runPipeline(ctx, p); err != nil {
		return err
	}
	if p.Negated {
		// `!` applies to the whole pipeline and inverts its status.
		if r.status == 0 {
			r.status = 1
		} else {
			r.status = 0
		}
	}
	return nil
}

func (r *Runner) command(ctx context.Context, c syntax.Command) error {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		return r.simple(ctx, x)
	case *syntax.Group:
		return r.group(ctx, x)
	case *syntax.Subshell:
		return r.subshell(ctx, x)
	case *syntax.IfClause:
		return r.ifClause(ctx, x)
	case *syntax.LoopClause:
		return r.loop(ctx, x)
	case *syntax.ForClause:
		return r.forClause(ctx, x)
	case *syntax.CaseClause:
		return r.caseClause(ctx, x)
	case *syntax.FuncDecl:
		return r.funcDecl(x)
	case *syntax.TestClause:
		return r.testClause(ctx, x)
	case *syntax.ArithCmdClause:
		return r.arithCmd(ctx, x)
	}
	return r.unsupported(fmt.Sprintf("%T", c))
}

// unsupported refuses rather than silently doing nothing.
func (r *Runner) unsupported(what string) error {
	return fmt.Errorf("not implemented yet: %s", what)
}

func (r *Runner) simple(ctx context.Context, c *syntax.SimpleCmd) error {
	r.unspecified, r.expandErr = false, false
	var argv []string
	for _, w := range c.Args {
		argv = append(argv, r.expandWord(w)...)
	}
	// A command whose expansion failed, or depended on an axis no dialect
	// answered, does not run. Reporting and then running anyway would be the
	// silent wrong answer this whole structure exists to avoid.
	if r.unspecified {
		r.status = 2
		return nil
	}
	if r.expandErr {
		// A failed arithmetic expansion is fatal to the script in every
		// shell measured — dash, bash, ksh93 and zsh all abandon the rest of
		// the list rather than run the next command. That much the core can
		// decide, and running on was the silent wrong answer: the corpus case
		// added for the status axis is what caught it.
		//
		// *Which* non-zero status it carries is the axis, asked inside
		// fatalQuiet. The diagnostic was already written by whoever failed,
		// so this adds none.
		r.fatalQuiet()
		return nil
	}
	if r.ctl == controlExit {
		// An expansion raised a fatal error of its own — an unmatched
		// pattern, where the dialect calls that an error rather than passing
		// it through. The command does not run, and nothing below may
		// overwrite the status it set.
		return nil
	}

	if len(argv) == 0 {
		// Assignments with no command name persist, which is the difference
		// between `x=1` and `x=1 cmd`.
		for _, a := range c.Assigns {
			r.assign(a)
		}
		if r.ctl == controlExit {
			// A readonly reassignment is fatal in three of the four shells.
			// Zeroing the status here is what made it look survivable: the
			// script stopped, and then reported success for having done so.
			return nil
		}
		// `>b` with no command still opens the file, and truncates it if it
		// exists. Returning early skipped that, so a redirection that was
		// the whole command did nothing at all — which is how `echo hi &>b`
		// in a dialect without `&>` came to leave no file behind, the exact
		// silent case the AmpersandRedirect comment warns about.
		if len(c.Redirs) > 0 {
			closers, err := r.applyRedirs(ctx, c.Redirs)
			for _, cl := range closers {
				_ = cl.Close()
			}
			if err != nil {
				return err
			}
			if r.redirErr {
				return nil
			}
		}
		r.status = 0
		return nil
	}

	closers, err := r.applyRedirs(ctx, c.Redirs)
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	if err != nil {
		return err
	}
	if closers == nil && len(c.Redirs) > 0 && r.status == 126 {
		// The gate refused an open; the status is already set.
		return nil
	}
	if r.redirErr {
		// The open failed — noclobber, a missing directory, a permission.
		// The status is already set and the command does not run.
		return nil
	}

	// A function shadows a builtin and an external command alike.
	if fn, ok := r.funcs[argv[0]]; ok {
		return r.callFunc(ctx, fn, argv[1:])
	}

	// A builtin runs in this shell, which is the whole reason it is one:
	// `set` and `shift` change state a child process could not.
	if fn, ok := r.lookupBuiltin(argv[0]); ok {
		// An assignment prefixed to a *special* builtin persists, which is
		// the POSIX rule dash and ksh93 follow and bash and zsh do not.
		// Following POSIX here; the divergence is a dialect question the
		// interpreter does not yet carry.
		for _, a := range c.Assigns {
			v := strings.Join(r.expandWord(a.Value), " ")
			// POSIX keeps an assignment prefixed to a special builtin;
			// dash and ksh93 comply, bash and zsh do not.
			if specialBuiltins[argv[0]] &&
				r.ask(r.sem().AssignmentPrefixPersistsOnSpecialBuiltin, "an assignment before a special builtin persisting") {
				r.setVar(a.Name, v)
			}
		}
		st := fn(r, ctx, argv[1:])
		// A builtin can consult an axis of its own — `echo` asks about
		// backslash escapes — so the check is repeated after it runs as well
		// as before, and its status is discarded when one went unanswered.
		if r.unspecified {
			r.status = 2
			return nil
		}
		r.status = st
		return nil
	}

	// An assignment prefix applies to this command's environment only.
	env := r.environ()
	for _, a := range c.Assigns {
		env = append(env, a.Name+"="+strings.Join(r.expandWord(a.Value), " "))
	}
	return r.exec(ctx, argv, env)
}

func (r *Runner) exec(ctx context.Context, argv, env []string) error {
	path, lookErr := exec.LookPath(argv[0])
	if lookErr != nil {
		path = argv[0]
	}
	action := Action{Kind: ActionExec, Path: path, Args: argv}
	if !r.allowed(ctx, action) {
		return nil
	}
	if lookErr != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: lookErr})
		r.diagf("%s: not found\n", argv[0])
		// 127 is the status every shell in the panel uses for this.
		r.status = 127
		return nil
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})

	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	if r.bg != nil {
		// A background command runs in a process group of its own, which is
		// what makes signalling and terminal ownership answerable at all.
		setProcessGroup(cmd)
	}
	cmd.Dir = r.Dir
	cmd.Env = env
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()

	if r.bg != nil {
		// Started rather than run, so the pid can be recorded before it is
		// waited for — `$!` has to be answerable immediately.
		if err := cmd.Start(); err != nil {
			r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
			r.diagf("%s: %v\n", argv[0], err)
			r.status = 126
			return nil
		}
		r.bg.PID = cmd.Process.Pid
		// The pid is final now, so anything waiting to read `$!` may proceed
		// while this goroutine blocks on the process.
		r.bg.markReady()
		err := cmd.Wait()
		r.status = exitStatus(err)
		r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
		return nil
	}

	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		r.status = 0
	case errors.As(err, &ee):
		r.status = ee.ExitCode()
	default:
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.diagf("%s: %v\n", argv[0], err)
		r.status = 126
		return nil
	}
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: r.status})
	return nil
}

func (r *Runner) environ() []string {
	base := r.Env
	if base == nil {
		base = os.Environ()
	}
	out := make([]string, len(base), len(base)+len(r.Vars))
	copy(out, base)
	for k, v := range r.Vars {
		// Only exported names reach a command's environment; the rest are
		// the shell's own.
		if r.exported[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// scope records the variables a function made local, and what they were.
type scope struct {
	saved   map[string]string
	existed map[string]bool
}

// fatal reports an error that abandons the script.
//
// Every fatal error goes through here so the three things that make one are
// decided in a single place: the diagnostic, the status — which is an axis,
// dash saying 2 where the others say 1 — and the unwinding. Setting the
// status and forgetting the unwinding is the bug this replaces, and it had
// been written independently at three sites.
// fatalQuiet is fatal for a failure that has already reported itself.
func (r *Runner) fatalQuiet() {
	if r.ask(r.sem().FatalErrorStatusIsOne, "the exit status of a fatal error") {
		r.status = 1
	} else {
		r.status = 2
	}
	r.ctl = controlExit
}

func (r *Runner) fatal(format string, args ...any) {
	r.diagf(format, args...)
	r.fatalQuiet()
}

func (r *Runner) setVar(name, value string) {
	if r.readonly[name] {
		// Fatal everywhere but bash, measured with a plain assignment in a
		// script — which is the contaminated-probe case oracle.md records.
		if r.ask(r.sem().ReadonlyReassignmentFatal, "a readonly reassignment being fatal") {
			r.fatal("%s: readonly variable\n", name)
			return
		}
		r.diagf("%s: readonly variable\n", name)
		r.status = 1
		return
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = value
}

func (r *Runner) getVar(name string) (string, bool) {
	if v, ok := r.Vars[name]; ok {
		return v, true
	}
	for _, kv := range r.environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v, true
		}
	}
	return "", false
}

// assign performs one assignment, which is three different things wearing the
// same syntax: a scalar, a whole array, or one element of one.
func (r *Runner) assign(a *syntax.Assign) {
	switch {
	case a.IsArray:
		var elems []string
		for _, w := range a.Elems {
			// Each element is a word, so `a=(1 $x 3)` expands and splits
			// like any other — which is how an array is built from a
			// command's output.
			elems = append(elems, r.expandWord(w)...)
		}
		r.setArray(a.Name, elems)
	case a.Index != nil:
		idx, err := r.parseNum(strings.TrimSpace(r.joinWord(a.Index)))
		if err != nil {
			r.diagf("%s: bad array subscript\n", a.Name)
			return
		}
		r.setArrayElem(a.Name, idx, strings.Join(r.expandWord(a.Value), " "))
	default:
		r.setVar(a.Name, strings.Join(r.expandWord(a.Value), " "))
		// A scalar assignment replaces any array of the same name.
		delete(r.Arrays, a.Name)
	}
}

// exitStatus turns a wait error into a status.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 126
}
