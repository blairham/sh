// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// The two remarks an interactive shell with no terminal writes name this
// shell by the **last element** of the path it was started by, where every
// other diagnostic it writes names the path.
//
// Measured 2026-09-21, no terminal on any of the three standard streams:
//
//	/opt/homebrew/bin/bash -i -c true      bash: cannot set terminal process
//	                                       group (-1): …
//	                                       bash: no job control in this shell
//	/opt/homebrew/bin/bash -c nosuchcmd    /opt/homebrew/bin/bash: line 1:
//	                                       nosuchcmd: command not found
//
// One binary, one run apart, two spellings — which is what makes this a
// field on these two lines rather than a reading of how the shell names
// itself. Through a link called `mybash` it writes `mybash`, and as a login
// shell with an argv[0] of `-bash` it writes `-bash`, leading dash and all:
// the base name and no more processing than that.
//
// It shows wherever a shell is started by an absolute path with no terminal,
// which is how a suite drives one — two lines per inner shell, and fourteen
// of them in one file of bash's own suite (#2298).
func TestTheJobControlRemarksNameTheBaseNameOfThePath(t *testing.T) {
	for _, c := range []struct{ name, invocation, want string }{
		{"an absolute path is shortened", "/opt/somewhere/bin/bash", "bash"},
		{"a link keeps the name it was reached by", "/opt/somewhere/bin/mybash", "mybash"},
		{"and a login shell keeps its leading dash", "-bash", "-bash"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var errs bytes.Buffer
			sem, diag := bash.Semantics(), bash.Diagnostics()
			r := &interp.Runner{
				Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
				Dir: t.TempDir(), Name: "/opt/somewhere/script.sh",
				Invocation: c.invocation,
				Dialect:    presetDialect(),
			}
			bash.Apply(r)
			// No terminal, which is the only state these two lines are
			// written in: the monitor was asked for and cannot be had.
			r.SetInteractiveMonitor()

			lines := strings.Split(strings.TrimSuffix(errs.String(), "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("wrote %q, want two lines", errs.String())
			}
			// The name is what this test is about, so the number is
			// matched by shape. It is two answers rather than one — the
			// group the shell is in, or -1 where it already leads that
			// group — and both are measured and asserted where the rule
			// lives, in interp's terminalProcessGroup.
			want := regexp.MustCompile(`^` + regexp.QuoteMeta(c.want) +
				`: cannot set terminal process group \(-?\d+\): Inappropriate ioctl for device$`)
			if !want.MatchString(lines[0]) {
				t.Errorf("the first line was %q, want %v", lines[0], want)
			}
			// Both lines, because they are two fields and only one of them
			// would be noticed by eye: a run naming the path on the second
			// line looks like an ordinary diagnostic.
			if want := c.want + ": no job control in this shell"; lines[1] != want {
				t.Errorf("the second line was %q, want %q", lines[1], want)
			}
			// And it is the invocation that is shortened rather than `$0`,
			// which here is a script in a directory of its own — a shell
			// reading the name the other way would write `script.sh`.
			if strings.Contains(errs.String(), "script.sh") {
				t.Errorf("wrote %q, which names $0 rather than the shell", errs.String())
			}
		})
	}
}
