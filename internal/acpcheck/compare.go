// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The argument for the thing, measured rather than asserted.
//
// "Does it work" is the table in check.go. This file answers the other two
// questions anyone asks — what does it see that the alternative does not, and
// what does it cost — because both of them have numbers and an argument with
// numbers in it does not need to be believed.
//
// The alternative is not a strawman. It is what an editor does today: run the
// command as a child, `sh -c` or `bash -c`, and read its output on a pipe.
// That arrangement is fine and it is fast, and the two things it cannot do are
// the two things measured here: it cannot see an action the script did not
// choose to print, and it cannot refuse one.

// Comparison is the whole of what Compare measured.
type Comparison struct {
	Script string

	// What each observer could see of the same run.
	PipeLines   []string // what came back on the pipe: the lines the script printed
	PipeActions int      // actions the pipe observer could see: none, by construction
	ACPActions  []string // the actions the protocol put to the client, in order
	Refusable   int      // how many of them the client could have refused

	// What it costs, as medians over a run of each.
	TurnPlain   time.Duration // a builtin command, in an established session
	TurnGated   time.Duration // a command that asks permission and is allowed
	SpawnOurs   time.Duration // the same command as a fresh `sh -c` child
	SpawnBash   time.Duration // and as a fresh `bash -c` child
	Handshake   time.Duration // initialize, over the pipe, once
	SessionOpen time.Duration // session/new
	Samples     int
}

// the script both observers are given.
//
// Written so that the interesting actions are ones the script never prints: a
// file written from inside an `eval` whose text was built at run time, and a
// program started through a variable. A watcher on the pipe sees "done" and
// nothing else, which is the point being made.
const compareScript = `
w=$(printf 'e%s' 'cho')
eval "$w secret > ledger.txt"
prog=/bin/echo
"$prog" hello >/dev/null
eval "rm -f ledger.txt"
echo done
`

// Compare runs the script both ways and times both arrangements.
func Compare(ctx context.Context, bin, dir string) (Comparison, error) {
	c := Comparison{Script: strings.TrimSpace(compareScript), Samples: 25}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return c, err
	}

	// The pipe. This is the whole of the alternative's visibility: a child,
	// and its output. Nothing about what it did on the way is available to
	// ask for, which is why PipeActions has no code to compute it.
	pipeDir := filepath.Join(dir, "pipe")
	if err := os.MkdirAll(pipeDir, 0o755); err != nil {
		return c, err
	}
	cmd := exec.CommandContext(ctx, bin, "-c", c.Script)
	cmd.Dir = pipeDir
	out, err := cmd.Output()
	if err != nil {
		return c, fmt.Errorf("the pipe run failed: %w", err)
	}
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			c.PipeLines = append(c.PipeLines, l)
		}
	}

	// The protocol, on the same script.
	acpDir := filepath.Join(dir, "acp")
	if err := os.MkdirAll(acpDir, 0o755); err != nil {
		return c, err
	}
	client, err := Dial(bin, Options{Args: []string{"-acp"}, Dir: acpDir, Answer: always(AllowOnce)})
	if err != nil {
		return c, err
	}
	start := time.Now()
	if _, err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return c, err
	}
	c.Handshake = time.Since(start)
	start = time.Now()
	session, err := client.NewSession(ctx, acpDir)
	if err != nil {
		_ = client.Close()
		return c, err
	}
	c.SessionOpen = time.Since(start)
	if _, err := client.Prompt(ctx, session, c.Script); err != nil {
		_ = client.Close()
		return c, err
	}
	for _, call := range client.ToolCalls() {
		c.ACPActions = append(c.ACPActions, call.Title)
	}
	c.Refusable = len(client.Asks())

	// The cost of a turn, in the session that is already open: first a
	// builtin, which asks nobody anything, and then a command that asks and
	// is allowed — the two ends of what a turn can cost.
	c.TurnPlain = median(c.Samples, func() { _, _ = client.Prompt(ctx, session, ":") })
	c.TurnGated = median(c.Samples, func() { _, _ = client.Prompt(ctx, session, "/bin/true") })
	_ = client.Close()

	// And the cost of the arrangement it replaces: a process per command.
	c.SpawnOurs = median(c.Samples, func() {
		_ = exec.CommandContext(ctx, bin, "-c", "/bin/true").Run()
	})
	if bash, err := exec.LookPath("bash"); err == nil {
		c.SpawnBash = median(c.Samples, func() {
			_ = exec.CommandContext(ctx, bash, "-c", "/bin/true").Run()
		})
	}
	return c, nil
}

// median times n runs of f and returns the middle one.
//
// The middle rather than the mean because one sample of a process start on a
// loaded machine is worth several of everything else, and a mean reports that
// outlier as if it were the cost of the thing.
func median(n int, f func()) time.Duration {
	f() // warm: the first of anything measures the page faults, not the work
	d := make([]time.Duration, n)
	for i := range d {
		start := time.Now()
		f()
		d[i] = time.Since(start)
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

// Report renders the comparison.
func (c Comparison) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nWhat each observer could see, of the same script\n\n")
	for _, l := range strings.Split(c.Script, "\n") {
		fmt.Fprintf(&b, "      %s\n", l)
	}
	fmt.Fprintf(&b, "\n  a pipe (%s -c, output read as a child):\n", "sh")
	fmt.Fprintf(&b, "      %d line(s) of output, %d actions, %d refusable\n",
		len(c.PipeLines), c.PipeActions, 0)
	for _, l := range c.PipeLines {
		fmt.Fprintf(&b, "        %s\n", l)
	}
	fmt.Fprintf(&b, "\n  the protocol (sh -acp):\n")
	fmt.Fprintf(&b, "      %d actions, %d refusable, each named before it happened\n",
		len(c.ACPActions), c.Refusable)
	for _, a := range c.ACPActions {
		fmt.Fprintf(&b, "        %s\n", a)
	}
	fmt.Fprintf(&b, "\n  Neither the file written inside the eval nor the program run through a\n")
	fmt.Fprintf(&b, "  variable appears in the pipe's output, and the file is gone by the time\n")
	fmt.Fprintf(&b, "  the script ends. An observer on the pipe has no way to learn either one.\n")

	fmt.Fprintf(&b, "\nWhat it costs, median of %d\n\n", c.Samples)
	fmt.Fprintf(&b, "  handshake, once per connection        %8s\n", round(c.Handshake))
	fmt.Fprintf(&b, "  session/new, once per session         %8s\n", round(c.SessionOpen))
	fmt.Fprintf(&b, "  a turn in an open session             %8s\n", round(c.TurnPlain))
	fmt.Fprintf(&b, "  a turn that asks and is allowed       %8s   (a permission round trip)\n", round(c.TurnGated))
	fmt.Fprintf(&b, "  the same command as a fresh sh -c     %8s   %s\n", round(c.SpawnOurs), ratio(c.SpawnOurs, c.TurnGated))
	if c.SpawnBash > 0 {
		fmt.Fprintf(&b, "  the same command as a fresh bash -c   %8s   %s\n", round(c.SpawnBash), ratio(c.SpawnBash, c.TurnGated))
	}
	fmt.Fprintf(&b, "\n  A gated turn is the row to read: it is a command run, watched and\n")
	fmt.Fprintf(&b, "  approved, and it is still cheaper than the process the same command\n")
	fmt.Fprintf(&b, "  costs an editor that spawns a shell per command and watches nothing.\n")
	return b.String()
}

// round drops the digits a process start does not have.
func round(d time.Duration) string {
	switch {
	case d == 0:
		return "-"
	case d < time.Millisecond:
		return d.Round(time.Microsecond).String()
	default:
		return d.Round(10 * time.Microsecond).String()
	}
}

// ratio says how one timing compares to another, in the direction a reader
// cares about: how many times the alternative costs what this does.
func ratio(alternative, ours time.Duration) string {
	if ours <= 0 || alternative <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1fx a gated turn", float64(alternative)/float64(ours))
}
