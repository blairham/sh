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
	"sync"
	"syscall"
	"time"

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
	// Vars holds shell variables. A nil map is initialized on first use.
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

	// ReplaceProcess makes `exec cmd` actually replace this process, and nil
	// — the default — makes it run the command as a child and then stop the
	// script with its status.
	//
	// It is opt-in because this package is a library. A real shell calls
	// execve and becomes the command; a Runner embedded in some other program
	// doing that would replace *that* program with whatever a script named,
	// which is not a shell feature but a way to lose a program. The core's
	// `cd` declines to call os.Chdir for the same reason and much smaller
	// stakes.
	//
	// A program that *is* a shell says so by setting this, and interp does
	// not provide the implementation: reaching for syscall.Exec is the
	// caller's decision to make, in the caller's own code, where it is
	// visible. cmd/sh and the dialect binaries set it via driver.
	//
	// It returns only on failure — a successful replacement does not come
	// back — and the error it returns is reported as the exec having failed.
	ReplaceProcess func(path string, argv, env []string) error

	// DieBySignal ends this process with the signal a script sent it and had
	// no handler for, and nil — the default — stops the script with 128 plus
	// the number instead.
	//
	// Opt-in for the reason ReplaceProcess is. `kill -INT $$` in a real shell
	// kills the shell; the same line in a Runner embedded in some other
	// program would kill *that* program, and a library that can be talked
	// into killing its host by the text it was asked to interpret is not a
	// library. So interp decides *when* — after the script has stopped and
	// the EXIT trap has had its turn, which is the order a shell uses — and
	// the caller decides whether, in the caller's own code.
	//
	// It does not return on success. The status matters even so: a process
	// killed by a signal and one exiting with 128 plus the number are the
	// same number to `$?` and different to `wait`, which is a difference the
	// corpus records.
	DieBySignal func(sig syscall.Signal) error

	// Dynamic holds parameters whose value is produced when they are read,
	// rather than stored: `LINENO` is wherever execution has reached, and
	// `RANDOM` is a different number every time. A dialect fills in the ones
	// it has through SetDynamic.
	Dynamic map[string]func(*Runner) string

	// assigned holds what a script assigned to a *produced* parameter, which
	// is a message to whatever produces it rather than a value of its own.
	assigned map[string]string

	// started is when this runner was made, which is what `SECONDS` counts
	// from in the dialects that have it.
	started time.Time

	// optChar is how far into a clustered option `getopts` has read — `-ab`
	// is two options in one word, and OPTIND cannot say which of them is
	// next because it counts words. lastOptind is what this builtin last set
	// OPTIND to, so a script moving it itself is noticed and the position
	// inside the cluster dropped.
	optChar int
	// optindAssigned records that a script wrote OPTIND since the last
	// `getopts` wrote it. The value cannot say so on its own: resetting it to
	// 1 while a cluster is half read is an assignment of the value it already
	// had, and three of the four restart the word on the strength of the
	// assignment rather than the number.
	optindAssigned bool

	// removed are names `unset` took away that came from the environment
	// rather than from Vars.
	//
	// Nothing clears an entry, and nothing needs to: a later assignment puts
	// the name in Vars, which every lookup reads first, and exports append
	// after the environment is filtered. A `delete` here looked right and no
	// test could tell whether it was there.
	//
	// Deleting from Vars cannot hide those: the environment is a second
	// source and getVar reads both, so `unset PATH` left PATH exactly where
	// it was and every lookup still found it. A name here is gone until
	// something assigns it again.
	removed map[string]bool

	// killedBy is the signal this shell sent itself and had no handler for,
	// with the number kept beside it so the death does not have to look the
	// name up again.
	killedBy    string
	killedBySig syscall.Signal

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
	// inBuiltin is the builtin currently speaking, for the one dialect that
	// names it in a diagnostic's location. Empty at every other moment, and
	// deliberately cleared by `.` and `eval` while they run borrowed text:
	// what a sourced script reports is the script's, not the builtin's.
	inBuiltin string
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
	// keepRedirs records that this command's redirections outlive it, which
	// is `exec > log` and nothing else. The dispatcher clears it after acting
	// on it, so it cannot leak into the next command.
	keepRedirs bool
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
	// signals is the signal-trap machinery, behind a pointer because clone
	// copies a Runner by value and a mutex cannot be copied — the race
	// detector says so, and it is right: handlers belong to the process, not
	// to one runner among several sharing it.
	signals *signalState
	// statusBefore is `$?` as it was before the current statement, which one
	// dialect shows to a signal handler instead of the current one.
	statusBefore int
	// exitTrap is the body of `trap … EXIT`, or nil when none is set. Only
	// EXIT is stored: the other signals need delivery, which is a separate
	// piece, and `trap` refuses them rather than accepting one and never
	// firing it.
	exitTrap *string
	// trapDepth is the function nesting the EXIT trap was set at, which zsh
	// alone needs: there a trap set inside a function fires when the
	// function returns rather than when the script ends.
	trapDepth int
	// inSubshell marks a runner that stands for a subshell or a command
	// substitution. Measured: the EXIT trap fires once, at the end of the
	// main script, and not in either of those — so the copy must know it is
	// a copy.
	inSubshell bool
	// traceWait and traceDone order the trace lines of a pipeline without
	// ordering the pipeline itself: an element waits for the one before it
	// to have printed, then prints, then releases the next. Only the
	// printing is serialized.
	traceWait <-chan struct{}
	traceDone chan struct{}
	traceOnce *sync.Once
	// xtrace is `set -x`: every simple command is printed before it runs.
	xtrace bool
	// nounset is `set -u`: expanding an unset parameter is an error.
	nounset bool
	// errexit is `set -e`: a command that fails ends the script.
	errexit bool
	// tested counts the contexts where a command's status is being *used*
	// rather than checked for failure — an `if` condition, a non-final
	// operand of `&&`, the operand of `!`. `set -e` does not fire while it
	// is above zero.
	//
	// A counter on the runner rather than a parameter, because the
	// exemption is inherited: a function called from an `if` condition has
	// it suppressed inside its body too, all the way down. That is measured,
	// unanimous across the panel, and the part of `set -e` most
	// implementations get wrong.
	tested int
	// noclobber is `set -C`: a plain `>` will not truncate an existing file.
	noclobber bool
	// declaring names commands whose `name=value` arguments are assignments,
	// beyond the ones the core already knows. A dialect adds its own.
	declaring map[string]bool
	// pipeStatus is what the last pipeline's elements reported, and
	// pipeStatusName is what the dialect calls it. The record is only kept
	// when a dialect has named it, because nothing else can read it.
	pipeStatus     []int
	pipeStatusName string
	// readonly names refuse assignment.
	readonly map[string]bool
	// integer names evaluate what is assigned to them: with the attribute,
	// `n=5+2` stores 7 rather than the four characters. It is a property of
	// the name and not of the assignment, which is why it is recorded here.
	integer map[string]bool
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
	c.inSubshell = true
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

// stdin is the shell's input, defaulting to the process's own — the reading
// half of what stdout and stderr already do.
func (r *Runner) stdin() io.Reader {
	if r.Stdin == nil {
		return os.Stdin
	}
	return r.Stdin
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
// printf writes to the shell's output stream.
func (r *Runner) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.stdout(), format, args...)
}

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
	msg := fmt.Sprintf(format, args...)
	if r.inBuiltin != "" && r.diag().NamesBuiltinInLocation {
		// The builtin's name belongs in exactly one place. Most dialects put it
		// at the front of the message — `cd: /x: no such directory` — and this
		// one puts it in the location instead, so a message that also opens with
		// it would say it twice: `zsh:cd:1: cd: /x: …`.
		//
		// Stripping it here rather than at each of the sixteen sites that write
		// one keeps the rule in a single place, and keeps those messages readable
		// as the sentence every other dialect prints.
		msg = strings.TrimPrefix(msg, r.inBuiltin+": ")
	}
	r.errf("%s%s", r.diag().prefix(r.name(), r.inBuiltin, r.line), msg)
}

// name is what the shell calls itself in a diagnostic.
func (r *Runner) name() string {
	if r.Name == "" {
		return "sh"
	}
	return r.Name
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
	r.ensurePWD()
	r.ensureSpecials()
	if r.started.IsZero() {
		r.started = time.Now()
	}
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.runExitTrap(ctx)
			return r.status, err
		}
		if r.ctl == controlExit {
			break
		}
	}
	// Anything that arrived during the last command still runs, before the
	// EXIT trap does.
	r.runPendingTraps(ctx)
	r.runExitTrap(ctx)
	if !r.inSubshell {
		r.stopSignals()
	}
	if r.killedBy != "" && r.DieBySignal != nil {
		// Last, because a shell that is dying still runs its EXIT trap first
		// where the dialect says so. This does not come back.
		if err := r.DieBySignal(r.killedBySig); err != nil {
			r.diagf("kill: %v\n", err)
		}
	}
	return r.status, nil
}

// runExitTrap runs `trap … EXIT` as the script ends.
//
// Once, and only for the main script: a subshell and a command substitution
// both leave it alone, which is unanimous across the panel and the reason
// clone marks its copies.
//
// The status is left as it is so the body can read `$?` — measured: a trap set
// after `false` sees 1. If the body exits with a status of its own, that wins,
// which is why the control flag is cleared first and consulted after.
func (r *Runner) runExitTrap(ctx context.Context) {
	if r.exitTrap == nil || r.inSubshell {
		return
	}
	// A shell that was killed rather than ended is a two-two split: bash and
	// ksh93 treat dying as exiting and run the trap, dash and zsh do not.
	// Asked only where there is a trap and a death to disagree about.
	if r.killedBy != "" && !r.ask(r.sem().ExitTrapRunsOnSignalDeath, "the EXIT trap after a fatal signal") {
		return
	}
	body := *r.exitTrap
	// Cleared before running so the body cannot fire it again, and so a
	// `trap` inside it replaces rather than recurses.
	r.exitTrap = nil
	before := r.status
	r.ctl = controlNone
	r.runTrapBody(ctx, body)
	if r.ctl != controlExit {
		// The body ran to the end without exiting, so the script keeps the
		// status it already had.
		r.status = before
	}
	r.ctl = controlExit
}

// runTrapBody parses and runs a trap's text, which is re-parsed at fire time
// because that is when a shell reads it.
func (r *Runner) runTrapBody(ctx context.Context, body string) {
	p := syntax.NewParser(body, r.dialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		r.diagf("trap: %v\n", err)
		r.status = r.diag().SyntaxStatus()
		return
	}
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.diagf("trap: %v\n", err)
			return
		}
		if r.ctl != controlNone {
			return
		}
	}
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) error {
	// Whatever arrived while the previous command ran. A shell finishes what
	// it is doing and runs the handler between commands, which is measured
	// and unanimous — so this is the point where a signal becomes visible.
	r.runPendingTraps(ctx)
	if r.ctl != controlNone {
		return nil
	}
	if st.Expr != nil {
		r.line = st.Expr.Pos().Line
	}
	r.statusBefore = r.status
	if st.Background {
		return r.background(ctx, st)
	}
	if err := r.expr(ctx, st.Expr); err != nil {
		return err
	}
	if _, isChain := st.Expr.(*syntax.BinaryExpr); !isChain && !lastIsNegated(st.Expr) {
		// A chain judges itself, inside expr, because only its final operand
		// counts and only when that operand actually ran.
		r.checkErrExit()
	}
	return nil
}

// lastIsNegated reports whether the command `set -e` would judge carries a
// `!`. Negation tests a status rather than requiring success, so `! true`
// yields 1 and does not end the script — measured, and unanimous.
//
// It asks about the *last* operand for the same reason the `&&` rule does:
// that is the one whose status the statement reports.
func lastIsNegated(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		return lastIsNegated(x.Y)
	case *syntax.Pipeline:
		return x.Negated
	}
	return false
}

// checkErrExit ends the script when `set -e` is on and the statement failed.
//
// One place, after a whole statement, because that is the granularity the
// shells use: `false | true` does not fire and `true | false` does, and both
// are one statement whose status is the pipeline's.
func (r *Runner) checkErrExit() {
	if r.errexit && r.tested == 0 && r.status != 0 && r.ctl == controlNone {
		// The status is the failing command's, not a status of its own —
		// `set -e; exit` reports what failed.
		r.ctl = controlExit
	}
}

func (r *Runner) expr(ctx context.Context, e syntax.Expr) error {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		// Only the *last* command of an `&&`/`||` chain is subject to
		// `set -e`. The tree is left-associative, so everything but the
		// final operand is inside X, and one counter covers the lot.
		r.tested++
		err := r.expr(ctx, x.X)
		r.tested--
		if err != nil {
			return err
		}
		// && runs the right side when the left succeeded, || when it failed.
		// Both are one level and left-associative, which the tree already
		// encodes; nothing here re-decides it.
		runRight := (x.Op == syntax.TokAndAnd) == (r.status == 0)
		if !runRight {
			// The chain short-circuited, so the final operand never ran and
			// there is nothing for `set -e` to judge: `false && :` leaves
			// status 1 and does not end the script.
			return nil
		}
		if err := r.expr(ctx, x.Y); err != nil {
			return err
		}
		if !lastIsNegated(x.Y) {
			r.checkErrExit()
		}
		return nil
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
		r.recordSingleStatus(p.Cmds[0])
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
	case *syntax.ForArithClause:
		return r.forArithClause(ctx, x)
	case *syntax.CaseClause:
		return r.caseClause(ctx, x)
	case *syntax.FuncDecl:
		return r.funcDecl(x)
	case *syntax.TestClause:
		return r.testClause(ctx, x)
	case *syntax.SelectClause:
		return r.selectClause(ctx, x)
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
	// `=cmd` is resolved across the whole command before any of it is
	// expanded, which is measured rather than assumed: `echo [[a == a]]`
	// reports the `==` and never reaches the `[[a`, so zsh has finished this
	// phase before pathname expansion begins on the first word.
	for _, w := range c.Args {
		r.expandEquals(w)
	}
	if r.expandErr {
		// Only what this pass just set. Testing r.ctl here as well made
		// every *later* command in an already-abandoned script report a
		// fresh failure of its own.
		r.fatalQuiet()
		return nil
	}
	var argv []string
	for i, w := range c.Args {
		// A declaration utility's `name=value` arguments are assignments and
		// expand as ones, which is what keeps `typeset -i n=3*3` from being
		// read as a pattern. Only after the first word is expanded is it
		// known which utility this is, so the test is inside the loop.
		if i > 0 && len(argv) > 0 && r.declares(argv[0]) && assignShaped(w) {
			argv = append(argv, r.expandAssignArg(w))
			continue
		}
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
		//
		// The status is set *before* they run, not after: `x=1` succeeds,
		// and `x=$(false)` reports what the substitution reported, because
		// the substitution sets the status as it goes. Zeroing afterwards
		// hid that, and `set -e; x=$(false)` carried on.
		r.status = 0
		if r.xtrace {
			values := make([]string, 0, len(c.Assigns))
			for _, a := range c.Assigns {
				values = append(values, strings.Join(r.expandWord(a.Value), " "))
			}
			r.traceAssignments(c.Assigns, values)
		}
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
			r.traceCommand(argv)

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
		// No zeroing here: the status was set before the assignments ran, so
		// a command substitution inside one has already reported. This line
		// used to overwrite it, which is why `set -e; x=$(false)` carried on.
		return nil
	}

	r.traceCommand(argv)

	closers, err := r.applyRedirs(ctx, c.Redirs)
	defer func() {
		if r.keepRedirs {
			// `exec > log` is the one command whose redirections outlive it.
			// Not closing them is the whole of that: the first closer is what
			// puts the saved streams back, and the rest hold files the script
			// still needs open.
			r.keepRedirs = false
			return
		}
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
		// the POSIX rule dash and ksh93 follow and bash and zsh do not. The
		// dialect answers it, three lines down.
		for _, a := range c.Assigns {
			v := strings.Join(r.expandWord(a.Value), " ")
			if specialBuiltins[argv[0]] &&
				r.ask(r.sem().AssignmentPrefixPersistsOnSpecialBuiltin, "an assignment before a special builtin persisting") {
				r.setVar(a.Name, v)
			}
		}
		// The builtin is on the record for the duration, so a dialect that
		// names it in a diagnostic's location can. Saved and put back rather
		// than cleared: a builtin can run another one.
		outer := r.inBuiltin
		r.inBuiltin = argv[0]
		st := fn(r, ctx, argv[1:])
		r.inBuiltin = outer
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
	// This runner's PATH, not the process's — see lookpath.go for why that
	// distinction is the whole bug and not a detail.
	path, lookErr := r.lookPath(argv[0])
	if lookErr != nil {
		path = argv[0]
	}
	action := Action{Kind: ActionExec, Path: path, Args: argv}
	if !r.allowed(ctx, action) {
		return nil
	}
	if lookErr != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: lookErr})
		// 127 for a name that resolved to nothing and 126 for a file that is
		// there and will not start. Both unanimous, and collapsing them into
		// one number was the other half of this bug: `./noexec` reported
		// "command not found" where every shell says "Permission denied".
		r.status = r.cannotRun(lookErr, naming{
			bare:     r.diag().NotFound,
			fallback: "%[1]s: not found",
		})
		return nil
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})

	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	if r.bg != nil {
		// A background command runs in a process group of its own, which is
		// what makes signaling and terminal ownership answerable at all.
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
	out := make([]string, 0, len(base)+len(r.Vars))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok && r.removed[k] {
			// A name the shell unset does not reach a command either: the
			// child would otherwise see what the parent cannot.
			continue
		}
		out = append(out, kv)
	}
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
	// keyword records that the function was defined with the `function` word
	// rather than with parentheses. ksh93 gives only those functions a local
	// scope, so `typeset` needs to know which kind it is standing in.
	keyword bool
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
			r.fatal("%s\n", Wording(r.diag().ReadonlyVariable, "%s: readonly variable", name))
			return
		}
		r.diagf("%s\n", Wording(r.diag().ReadonlyVariable, "%s: readonly variable", name))
		r.status = 1
		return
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	if r.integer[name] {
		// The name was declared integer, so what is assigned to it is an
		// expression rather than text.
		v, ok := r.integerValue(value)
		if !ok {
			return
		}
		value = v
	}
	if _, dynamic := r.Dynamic[name]; dynamic {
		// Assigning a produced parameter is a message to its producer rather
		// than a replacement for it.
		if r.assigned == nil {
			r.assigned = map[string]string{}
		}
		r.assigned[name] = value
		delete(r.removed, name)
		return
	}
	if name == "OPTIND" {
		// Noted rather than compared: see optindAssigned.
		r.optindAssigned = true
	}
	r.Vars[name] = value
}

// ensurePWD gives `$PWD` a value before the first command runs.
//
// POSIX requires a shell to set it at startup, and until this was here `cd`
// was the only thing that ever did — so a shell handed an environment without
// PWD in it, which is exactly what the conformance harness hands one, expanded
// `$PWD` to nothing. `PATH=$PWD/d:$PATH` then meant `/d`, and a `.` lookup
// that should have found a file on PATH silently fell through to the current
// directory instead. Real bash sets it and got the case right; we did not.
//
// Only r.Vars is consulted, deliberately, and not getVar: getVar falls back to
// the process environment, whose PWD describes the *parent* rather than this
// runner. A Runner given a Dir of its own would otherwise report the directory
// the program was started in, which is a different place — and real shells do
// not trust an inherited PWD that disagrees with where they actually are
// either.
//
// Idempotent, because Run is called more than once on a runner — a dialect's
// prelude first and then the script — and because `cd` may already have moved,
// and because a caller may have set it deliberately.
func (r *Runner) ensurePWD() {
	if _, ok := r.Vars["PWD"]; ok {
		return
	}
	r.setVar("PWD", r.workDir())
}

func (r *Runner) getVar(name string) (string, bool) {
	// A produced array answers a plain `$name` too, and what it answers with
	// is an axis: the whole array in one shell and its first element in the
	// others.
	if elems, ok := r.pipelineStatuses(name); ok {
		return r.arrayScalar(elems), true
	}
	if f, ok := r.Dynamic[name]; ok {
		// Ahead of the stored table, because a parameter that produces its
		// value cannot be overwritten by assigning to it: `RANDOM=5` seeds
		// the generator and the next read is still a new number. What was
		// assigned is kept where the producer can see it — SECONDS counts
		// from it — rather than shadowing the producer entirely.
		if !r.removed[name] {
			return f(r), true
		}
	}
	if a, ok := r.Arrays[name]; ok && len(a) > 1 && !r.removed[name] {
		// Ahead of Vars, which holds the first element: with more than one
		// element the two views differ and the dialect decides.
		return r.arrayScalar(a), true
	}
	if v, ok := r.Vars[name]; ok {
		return v, true
	}
	if r.removed[name] {
		// `unset` took it away, and neither the environment nor a dynamic
		// parameter is allowed to put it back.
		return "", false
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
		if a.Append {
			// `a+=(d)` adds to the end of the array rather than to its first
			// element, which is what makes append two operations sharing a
			// spelling rather than one.
			elems = append(append([]string(nil), r.Arrays[a.Name]...), elems...)
		}
		r.setArray(a.Name, elems)
	case a.Index != nil:
		idx, err := r.parseNum(strings.TrimSpace(r.joinWord(a.Index)))
		if err != nil {
			r.diagf("%s: bad array subscript\n", a.Name)
			return
		}
		r.setArrayElem(a.Name, idx, r.expandAssignValue(a.Value))
	default:
		value := r.expandAssignValue(a.Value)
		if a.Append {
			old, _ := r.getVar(a.Name)
			value = old + value
		}
		r.setVar(a.Name, value)
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
