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

	"github.com/blairham/sh/internal/syntax"
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
	// Dir is the working directory; empty means the process's own.
	Dir string
	// Env is the environment passed to commands. Nil means the process's own.
	Env []string

	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// Gate is consulted before every action that leaves this process. Nil
	// allows everything, so the zero value is usable.
	Gate Gate
	// Events receives what happened. Nil discards.
	Events Sink

	// status is the exit status of the last command run.
	status int
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
	r.errf("sh: %s: refused: %s\n", a.Kind, a.Path)
	r.status = 126
	return false
}

// Run executes a whole file, returning the last command's status.
func (r *Runner) Run(ctx context.Context, f *syntax.File) (int, error) {
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			return r.status, err
		}
	}
	return r.status, nil
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) error {
	if st.Background {
		return r.unsupported("background commands")
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
	if len(p.Cmds) != 1 {
		return r.unsupported("pipelines")
	}
	if err := r.command(ctx, p.Cmds[0]); err != nil {
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
	sc, ok := c.(*syntax.SimpleCmd)
	if !ok {
		return r.unsupported(fmt.Sprintf("%T", c))
	}
	return r.simple(ctx, sc)
}

// unsupported refuses rather than silently doing nothing.
func (r *Runner) unsupported(what string) error {
	return fmt.Errorf("not implemented yet: %s", what)
}

func (r *Runner) simple(ctx context.Context, c *syntax.SimpleCmd) error {
	var argv []string
	for _, w := range c.Args {
		argv = append(argv, r.expandWord(w)...)
	}

	if len(argv) == 0 {
		// Assignments with no command name persist, which is the difference
		// between `x=1` and `x=1 cmd`.
		for _, a := range c.Assigns {
			r.setVar(a.Name, strings.Join(r.expandWord(a.Value), " "))
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
		r.errf("sh: %s: not found\n", argv[0])
		// 127 is the status every shell in the panel uses for this.
		r.status = 127
		return nil
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})

	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	cmd.Dir = r.Dir
	cmd.Env = env
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()

	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		r.status = 0
	case errors.As(err, &ee):
		r.status = ee.ExitCode()
	default:
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		r.errf("sh: %s: %v\n", argv[0], err)
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
		out = append(out, k+"="+v)
	}
	return out
}

func (r *Runner) setVar(name, value string) {
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
