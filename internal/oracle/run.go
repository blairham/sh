// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// RunTimeout bounds a single snippet. A shell that waits on stdin would
// otherwise hang the whole run, so stdin is closed as well; both are needed,
// because a snippet can block on things other than input.
const RunTimeout = 10 * time.Second

// Result is what one shell did with one case.
type Result struct {
	// Output is combined stdout and stderr, normalized and with the trailing
	// newline removed. Both streams are kept because a diagnostic is part of
	// the behaviour: two shells that print the same thing to different streams
	// have not behaved the same way.
	Output string

	// Status is the exit status, or -1 if the shell could not be run at all.
	// It is recorded separately because a case can produce identical output
	// with a different status, and that is still a difference.
	Status int

	// TimedOut marks a snippet the shell never finished.
	TimedOut bool
}

// Exec executes one case in one shell.
//
// The snippet is passed with -c rather than written to a file unless the case
// asks otherwise: some behaviour depends on how input is read, and a case that
// cares says so. See Case.Script.
func Exec(ctx context.Context, sh Found, c Case) Result {
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "oracle-")
	if err != nil {
		return Result{Output: "harness error: " + err.Error(), Status: -1}
	}
	// The scratch directory is this run's alone, so a failed removal is a
	// leaked temp dir rather than a wrong measurement; nothing here can act
	// on it usefully.
	defer func() { _ = os.RemoveAll(dir) }()

	cmd := command(ctx, sh, c, dir)
	cmd.Dir = dir
	// A snippet must not read the terminal, and must not inherit a
	// user's environment: HOME, IFS or PATH from the developer's shell
	// would make the record depend on whose machine produced it.
	cmd.Stdin = nil
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"LC_ALL=C",
		"TERM=dumb",
	}

	out, err := cmd.CombinedOutput()
	res := Result{Output: normalize(string(out), sh, dir), Status: 0}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.Status = -1
		return res
	}
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		res.Status = ee.ExitCode()
	case err != nil:
		res.Output = "harness error: " + err.Error()
		res.Status = -1
	}
	return res
}

func command(ctx context.Context, sh Found, c Case, dir string) *exec.Cmd {
	if c.Script {
		// Written to a file and run as an argument, because a few behaviours
		// differ between a script and -c: a readonly reassignment is fatal in
		// one and not the other, which is how the contaminated-probe trap in
		// oracle.md was found.
		path := filepath.Join(dir, "case.sh")
		_ = os.WriteFile(path, []byte(c.Snippet+"\n"), 0o600)
		return exec.CommandContext(ctx, sh.Path, append(append([]string(nil), sh.Args...), path)...)
	}
	args := append(append([]string(nil), sh.Args...), "-c", c.Snippet)
	cmd := exec.CommandContext(ctx, sh.Path, args...)
	if sh.Argv0 != "" {
		cmd.Args[0] = sh.Argv0
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
	// A shell names itself by basename at the start of a diagnostic. Replacing
	// that basename anywhere would corrupt ordinary words — with sh, "can't
	// shift that many" became "can't <shell>ift that many" — so it is anchored
	// to the start of a line and required to be followed by a colon.
	s = diagPrefix(filepath.Base(sh.Path)).ReplaceAllString(s, "<shell>:")
	// A shell invoked under another name reports *that* name, not its
	// binary's — bash-as-sh says "sh:", which the line above cannot match.
	if sh.Argv0 != "" {
		s = diagPrefix(sh.Argv0).ReplaceAllString(s, "<shell>:")
	}
	s = strings.TrimRight(s, "\n")
	// Newlines are shown as ~ so a result stays one table cell. Real output
	// containing ~ is rare enough that the ambiguity has not bitten; if it
	// does, this is the place to escape it.
	return strings.ReplaceAll(s, "\n", "~")
}

// diagPrefix matches a shell naming itself at the start of a diagnostic.
func diagPrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(base) + `:`)
}
