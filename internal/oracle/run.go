// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// RunTimeout bounds a single snippet. A shell that waits on stdin would
// otherwise hang the whole run, so stdin is closed unless the case supplies
// some — and a supplied one is a finite string that ends. Both are needed,
// because a snippet can block on things other than input.
const RunTimeout = 10 * time.Second

// Result is what one shell did with one case.
type Result struct {
	// Stdout and Stderr are what the shell wrote to each stream, normalized
	// and with the trailing newline removed.
	//
	// They are recorded apart because a diagnostic is part of the behavior:
	// two shells that print the same thing to different streams have not
	// behaved the same way, and a merged capture cannot tell. That claim was
	// written here while the harness took cmd.CombinedOutput() and could not
	// make it good — a diagnostic printed to standard output read exactly
	// like one printed to standard error, so a whole class of regression was
	// invisible unless the snippet separated the streams itself with `2>`.
	// A handful of cases do that; the rest were being graded on a merge.
	//
	// What a merge bought was the order the two streams interleaved in, and
	// that is given up here. It was never worth much: a shell block-buffers
	// standard output into a pipe and writes standard error unbuffered, so
	// the merged order was the order the buffers happened to flush rather
	// than the order the shell wrote. A case that means to pin ordering
	// across the two streams still says so the only reliable way, by
	// redirecting one of them where it can see it.
	Stdout string
	Stderr string

	// Status is the exit status, or -1 if the shell could not be run at all.
	// It is recorded separately because a case can produce identical output
	// with a different status, and that is still a difference.
	//
	// It is also -1 for a shell that died by a *signal*, because there is no
	// exit status in that case: the wait status carries a signal number
	// instead, and Go reports the absence as -1. Which signal is in Signal.
	Status int

	// Signal is the signal that killed the shell, or 0 if it exited normally.
	//
	// Without it, every way of dying by a signal looks the same in the
	// record, because Status is -1 for all of them: a shell that ended on
	// SIGINT and one that ended on SIGKILL were indistinguishable, and so
	// were a shell killed by a signal and one the harness could not run at
	// all. That made a shell's whole exit-on-signal discipline untestable —
	// the convention that a shell dying of an untrapped fatal signal kills
	// *itself* with that same signal, so its own caller sees the signal
	// rather than a status. Every case that wanted to look at a signal had
	// to hide the death inside a nested child and read the number the parent
	// shell computed from it, which measures the parent's arithmetic rather
	// than the child's death.
	//
	// Zero is "no signal" and not a signal number: 0 is the valid argument
	// to kill(2) that sends nothing, so nothing is ever killed by it.
	Signal syscall.Signal

	// TimedOut marks a snippet the shell never finished.
	TimedOut bool
}

// harnessError is the Result for a case that could not be measured.
//
// It goes to Stderr because that is what it is — a diagnostic — and because
// a reader scanning the record for the stream a message came out on should
// not find the harness's own failures filed under a shell's output.
func harnessError(err error) Result {
	return Result{Stderr: "harness error: " + err.Error(), Status: -1}
}

// ArgSnippet and ArgScript are the placeholders a Case.Args may use to say
// where its snippet goes: as the word after `-c`, or as a file the shell is
// asked to run. Case.Stdin honors them too, which is how a case says its
// program arrives on standard input. See Case.Args and Case.Stdin, which
// document the contract.
//
// They are spelled with braces because no shell gives that spelling a meaning
// in an argument, so a placeholder can never be confused with a word a case
// actually meant to pass.
const (
	ArgSnippet = "{snippet}"
	ArgScript  = "{script}"
)

// Exec executes one case in one shell.
//
// The snippet is passed with -c rather than written to a file unless the case
// asks otherwise: some behavior depends on how input is read, and a case that
// cares says so. See Case.Script, and Case.Args for a case that spells its
// whole invocation out.
func Exec(ctx context.Context, sh Found, c Case) Result {
	if err := c.validate(); err != nil {
		return harnessError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "oracle-")
	if err != nil {
		return harnessError(err)
	}
	// The scratch directory is this run's alone, so a failed removal is a
	// leaked temp dir rather than a wrong measurement; nothing here can act
	// on it usefully.
	defer func() { _ = os.RemoveAll(dir) }()

	cmd := command(ctx, sh, c, dir)
	cmd.Dir = dir
	// A snippet must not inherit a user's environment: HOME, IFS or PATH
	// from the developer's shell would make the record depend on whose
	// machine produced it. Standard input is the same argument and is
	// settled in command(), which knows what the case asked for.
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"LC_ALL=C",
		"TERM=dumb",
	}

	// A buffer per stream rather than one shared one. os/exec sends both to
	// the same pipe when they are the same writer, which is what
	// CombinedOutput does and what this exists not to do.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res := Result{
		Stdout: normalize(stdout.String(), sh, dir),
		Stderr: normalize(stderr.String(), sh, dir),
		Status: 0,
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.Status = -1
		return res
	}
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		res.Status = ee.ExitCode()
		res.Signal = signalOf(ee.ProcessState)
	case err != nil:
		// Whatever the shell managed to write before the harness failed is
		// not a measurement of anything, so it is replaced rather than
		// annotated.
		res = harnessError(err)
	}
	return res
}

// signalOf is the signal a process died of, or 0.
//
// The wait status is the only place the number survives: an exit status and a
// signal death are alternatives in it, which is why ExitCode() has to answer
// -1 for the second. Anything that is not a wait status this package
// understands reports no signal rather than guessing.
func signalOf(st *os.ProcessState) syscall.Signal {
	ws, ok := st.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return 0
	}
	return ws.Signal()
}

// validate reports a case whose invocation cannot be built.
//
// It is checked here rather than only in the corpus test because a harness
// error has to be visible in the record: a case that quietly ran something
// other than what it says would be measured, and the measurement is the
// whole product.
func (c Case) validate() error {
	if len(c.Args) == 0 {
		return nil
	}
	if c.Script {
		return errors.New("Case.Args and Case.Script are exclusive: put ArgScript in Args instead")
	}
	n := 0
	for _, a := range c.Args {
		if a == ArgSnippet || a == ArgScript {
			n++
		}
	}
	if n > 1 {
		return errors.New("Case.Args names the snippet's place more than once")
	}
	return nil
}

// command builds the invocation: the shell's own flags first — which is where
// the binary under test is told which dialect to be — and then either the
// case's own argv or the harness's default `-c` and snippet.
func command(ctx context.Context, sh Found, c Case, dir string) *exec.Cmd {
	// Written to a file and run as an argument, because a few behaviors
	// differ between a script and -c: a readonly reassignment is fatal in one
	// and not the other, which is how the contaminated-probe trap in
	// oracle.md was found.
	script := func() string {
		path := filepath.Join(dir, "case.sh")
		_ = os.WriteFile(path, []byte(c.Snippet+"\n"), 0o600)
		return path
	}

	args := append([]string(nil), sh.Args...)
	named := true
	switch {
	case len(c.Args) > 0:
		for _, a := range c.Args {
			switch a {
			case ArgSnippet:
				a = c.Snippet
			case ArgScript:
				a = script()
			}
			args = append(args, a)
		}
	case c.Script:
		args = append(args, script())
		// The script route does not honor Argv0, and that is measured rather
		// than chosen: bash invoked as sh makes a readonly reassignment fatal,
		// so honoring it here rewrites what twenty recorded rows say bash-as-sh
		// does. The column is therefore plain bash for a Script case and the
		// named shell everywhere else. Straightening that out means re-recording
		// those rows and deciding which answer the column should have been
		// giving, which is its own change rather than a side effect of this one.
		named = false
	default:
		args = append(args, "-c", c.Snippet)
	}

	cmd := exec.CommandContext(ctx, sh.Path, args...)
	if named && sh.Argv0 != "" {
		cmd.Args[0] = sh.Argv0
	}
	// Standard input is closed unless the case asked for some. A nil Stdin is
	// os/exec's own spelling of that, and it is the right default twice over:
	// a shell with nothing to read must not wait for a terminal, and a case
	// that says nothing about input must not be handed whatever the harness
	// itself was started with.
	//
	// The placeholders are honored here too, so a case whose *program*
	// arrives on standard input writes it once as its Snippet. The script
	// file is only written if something asks for it.
	if c.Stdin != "" {
		in := strings.ReplaceAll(c.Stdin, ArgSnippet, c.Snippet)
		if strings.Contains(in, ArgScript) {
			in = strings.ReplaceAll(in, ArgScript, script())
		}
		cmd.Stdin = strings.NewReader(in)
	}
	return cmd
}

// normalize removes what is true of this machine rather than of this shell.
//
// Diagnostics quote the shell's own path and the script's path, both of which
// differ between a laptop and a CI runner. Without this the golden file would
// record where it was generated and drift for everyone else.
func normalize(s string, sh Found, dir string) string {
	rep := strings.NewReplacer(
		dir+"/case.sh", "<script>",
		dir, "<tmp>",
		sh.Path, "<shell>",
	)
	s = rep.Replace(s)
	// Some snippets mask their own digits before the harness sees anything —
	// the `times` cases do, because the figures are real timings — and `sed
	// -E "s/[0-9]+/N/g"` cannot tell a timing from the digits in the shell's
	// own path. So a binary living under a directory with a number in it,
	// which is every macOS TMPDIR, arrived here already spelled differently
	// from sh.Path and went unreplaced: `make conformance-dialects` scored
	// zsh one lower than the same corpus run against the same code built in
	// /tmp. Where the binary sits is not a fact about the shell, so the
	// masked spelling is replaced too.
	if masked := digitRuns.ReplaceAllString(sh.Path, "N"); masked != sh.Path {
		s = strings.ReplaceAll(s, masked, "<shell>")
	}
	// A shell names itself by basename at the start of a diagnostic. Replacing
	// that basename anywhere would corrupt ordinary words — with sh, "can't
	// shift that many" became "can't <shell>ift that many" — so it is anchored
	// to the start of a line and required to be followed by a colon.
	s = diagPrefix(filepath.Base(sh.Path)).ReplaceAllString(s, "<shell>:")
	s = tracePrefix(filepath.Base(sh.Path)).ReplaceAllString(s, "+<shell>:")
	// A shell invoked under another name reports *that* name, not its
	// binary's — bash-as-sh says "sh:", which the line above cannot match.
	if sh.Argv0 != "" {
		s = diagPrefix(sh.Argv0).ReplaceAllString(s, "<shell>:")
		s = tracePrefix(sh.Argv0).ReplaceAllString(s, "+<shell>:")
	}
	s = strings.TrimRight(s, "\n")
	// Newlines are shown as ~ so a result stays one table cell. Real output
	// containing ~ is rare enough that the ambiguity has not bitten; if it
	// does, this is the place to escape it.
	return strings.ReplaceAll(s, "\n", "~")
}

// digitRuns is the masking a snippet applies to its own output, spelled the
// same way `sed -E "s/[0-9]+/N/g"` spells it.
var digitRuns = regexp.MustCompile(`[0-9]+`)

// diagPrefix matches a shell naming itself at the start of a diagnostic.
// tracePrefix matches a shell naming itself inside a `set -x` line, which is
// the one place the name is not at the start.
//
// zsh writes `+zsh:1> echo a b`, so the line-anchored rule above cannot see
// it — and without this the trace cases compared our path against zsh's name
// and every one of them looked like a divergence.
func tracePrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\+` + regexp.QuoteMeta(base) + `:`)
}

func diagPrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(base) + `:`)
}
