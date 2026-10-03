// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"os"

	"github.com/blairham/sh/syntax"
)

// TailExec is how far into a command string a shell reaches to find the
// program it becomes. A shell that has nothing left to run after a program
// starts can exec the program in its own place instead of forking it and
// waiting.
//
// Measured 2026-10-03 under `-c` with the probe `P` = `/bin/sh -c 'echo
// $PPID'` beside an `echo $$`. **E** means the probe's parent is the shell's
// own parent, so the shell became the probe; **f** means the shell forked it:
//
//	                               zsh 5.9.2  dash   ksh93u+  ash 1.37  bash 5.3  bash 3.2
//	P                              E          E      E        E         E         f
//	true; P                        E          E      E        E         E         f
//	x=1 P, command P               E          E      E        E         E         f
//	true && P, false || P          E          E      E        E         f         f
//	if true; then P; fi            E          E      E        E         f         f
//	{ P; }, case a in a) P;; esac  E          E      E        E         f         f
//	P >/dev/stdout                 E          E      E        E         f         f
//	for i in 1; do P; done         E          f      f        f         f         f
//	P; true                        f          f      f        f         f         f
//	trap 'echo t' EXIT; P          f          f      f        f         f         f
//	f() { P; }; f / eval 'P'       f          f      f        f         f         f
//	! P, P | cat, time P           f          f      f        f         f         f
//
// The same lines in a script file are forked in all six columns. So bash
// reaches only a plain command at the top of the string, and the other four
// reach wherever the program is the last thing the shell runs. zsh alone
// counts the last pass of a `for` loop as that.
// unforkedtail.go already follows that "wherever" through the compound
// commands for the `( … )` zsh does not fork, and this reuses it.
//
// The program the shell becomes is handed the environment `exec` would hand
// it. That includes `$SHLVL` with this shell taken back out of the count in
// the columns that do that (Semantics.ShellLevelExec), so
// `SHLVL=1 zsh -c 'zsh -c "echo \$SHLVL"'` prints 2 where a forked child
// would print 3.
//
// It is reached only where the embedder has supplied ReplaceProcess, and
// otherwise the program is forked as it always was. A Runner inside some
// other program must not replace that program.
//
// **Not modeled here**, and forked instead:
//
//   - a `( … )` in the tail, in the dialects that fork the parentheses: dash,
//     ksh93 and ash. Here a subshell is a clone in one process. zsh does not
//     fork them, and that clone is the shell itself, so it is replaced;
//   - a shell that still has a job running, or has ever made a process
//     substitution. Those are goroutines here and would die with the
//     process. zsh, dash and ksh93 replace themselves anyway, and bash 5.3
//     forks;
//   - a file the kernel will not start by itself, a script with no `#!`.
//     That runs through this shell, as it does when it is forked.
type TailExec int

const (
	// TailExecUnspecified is no answer, and replaces nothing.
	TailExecUnspecified TailExec = iota
	// TailExecPlainTopLevel is bash 5.3: only a lone simple command with no
	// redirections, as the last statement of the string itself.
	TailExecPlainTopLevel
	// TailExecWhereverLast is zsh: the last command the shell runs, however
	// deep in the compound commands it is, the last pass of a `for` loop
	// included.
	TailExecWhereverLast
	// TailExecWhereverLastOutsideALoop is dash, ksh93 and BusyBox ash: the
	// same, except that a loop's body is never the last thing run.
	TailExecWhereverLastOutsideALoop
)

func (t TailExec) String() string {
	switch t {
	case TailExecPlainTopLevel:
		return "TailExecPlainTopLevel"
	case TailExecWhereverLast:
		return "TailExecWhereverLast"
	case TailExecWhereverLastOutsideALoop:
		return "TailExecWhereverLastOutsideALoop"
	}
	return "TailExecUnspecified"
}

// plainTopLevelTail is the simple command a list ends on where it is the
// whole of its statement: a pipeline of one that is not negated, not under an
// `&&` or `||`, and carries no redirection.
func plainTopLevelTail(list []*syntax.Stmt) *syntax.SimpleCmd {
	if len(list) == 0 {
		return nil
	}
	st := list[len(list)-1]
	if st.Background || st.Coprocess || st.Disown {
		return nil
	}
	p, ok := st.Expr.(*syntax.Pipeline)
	if !ok || len(p.Cmds) != 1 || p.Negated {
		return nil
	}
	c, ok := p.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(c.Redirs) != 0 {
		return nil
	}
	return c
}

// replacesItselfHere reports whether the simple command now running is the
// program this shell should become rather than fork.
func (r *Runner) replacesItselfHere() bool {
	c := r.runningSimple
	// A clone is a subshell, and replacing the process from one would take
	// the parent shell with it. The one exception is a `( … )` the dialect
	// does not fork because nothing follows it: that clone is the shell
	// itself. See unforkedtail.go.
	if c == nil || r.ReplaceProcess == nil || (r.inSubshell && !r.unforkedSelf) || r.bg != nil {
		return false
	}
	switch r.sem().TailExec {
	case TailExecWhereverLast:
		if !r.isTail(c) {
			return false
		}
	case TailExecWhereverLastOutsideALoop:
		if !r.isTail(c) || r.tailInALoop {
			return false
		}
	case TailExecPlainTopLevel:
		if r.plainTail == nil || r.plainTail != c || !r.isTail(c) {
			return false
		}
	default:
		return false
	}
	if r.holdsATrapWithAnAction() || !r.namedStreamsCanBePlaced() {
		return false
	}
	// A named stream that is not a file has no number to hand over: a
	// forked child gets its bytes copied through a pipe, and a replacement
	// would start with the number closed. An embedder's buffer is the usual
	// case, so this is forked as before.
	if !r.streamsAreFiles() {
		return false
	}
	// Nothing else of this shell may still be running, because here it is a
	// goroutine and would go with the process.
	for _, j := range r.jobs {
		if j != nil && !j.Finished() {
			return false
		}
	}
	if r.procSubHome != nil && r.procSubHome.seq.Load() > 0 {
		return false
	}
	return true
}

// becomeTheProgram replaces this shell with the program at path, as `exec`
// would. It reports false if the program could not be started that way. In
// that case the caller forks the program as usual.
func (r *Runner) becomeTheProgram(ctx context.Context, path string, argv, env []string) bool {
	if !kernelStartsIt(path) {
		return false
	}
	_ = ctx
	dir := r.dirNow()
	if r.unforkedSelf {
		// The parentheses are the shell itself, so its end is this one.
		r.CleanUp()
	} else {
		r.cleanUpAtEnd()
	}
	releaseMask := r.holdMaskForFork()
	name, env := r.namedByTheEnvironment(argv[0], env)
	args := append([]string{r.dashed(name)}, argv[1:]...)
	err := r.ReplaceProcess(dir, path, args, r.lowerShellLevelIn(env), r.replacementFiles())
	releaseMask()
	return err == nil
}

// kernelStartsIt reports whether the file begins the way a file the kernel
// runs by itself does: an interpreter line, or an executable image.
func kernelStartsIt(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 4)
	n, _ := f.Read(head)
	head = head[:n]
	for _, magic := range [][]byte{
		[]byte("#!"),
		[]byte("\x7fELF"),
		{0xfe, 0xed, 0xfa, 0xce},
		{0xfe, 0xed, 0xfa, 0xcf},
		{0xce, 0xfa, 0xed, 0xfe},
		{0xcf, 0xfa, 0xed, 0xfe},
		{0xca, 0xfe, 0xba, 0xbe},
	} {
		if bytes.HasPrefix(head, magic) {
			return true
		}
	}
	return false
}
