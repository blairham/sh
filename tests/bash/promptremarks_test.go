// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **A here-document that runs out inside `$(…)` is warned about at the
// prompt** (#5239), when the construct finishes on a later line — as in a
// script, and before the command prints anything.
//
// The prompt wrote remarks only for input left over at the end, so a
// document that ran out inside a substitution finishing on the next line was
// never mentioned. Measured 2026-09-30 against bash 5.3.20 under `--norc
// --noprofile -i` with the lines on stdin: the warning comes once, straight
// after the line that finished the construct and before the next is read.
//
// bash echoes each line it reads to standard error here, which is what lets
// the order be asserted: the warning must stand between the finishing line
// and the next one. The count catches a warning repeated each time the growing
// construct was parsed again.
func TestAPromptWarnsAboutAHeredocTheSubstitutionEnded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prep = "PS1=''\nPS2=''\n"
	for _, c := range []struct {
		name, src, out, warn, after, before string
		n                                   int
	}{
		{
			"one document", "x=$(cat <<EOF\nhi\nEOF)\necho \"[$x]\"\n", "[hi]\n",
			"bash: warning: here-document at line 3 delimited by end-of-file (wanted `EOF')\n",
			"EOF)\n", "echo \"[$x]\"\n", 1,
		},
		// The last of two documents is the one the input ran out inside.
		{
			"two documents", "x=$(cat <<A; cat <<B\n1\nA\n2\nB)\necho \"[$x]\"\n", "[1\n2]\n",
			"bash: warning: here-document at line 5 delimited by end-of-file (wanted `B')\n",
			"B)\n", "echo \"[$x]\"\n", 1,
		},
		// The control: a document that has its delimiter says nothing.
		{"a proper document", "cat <<EOF\nhi\nEOF\n", "hi\n", "", "", "", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "in.sh")
			if err := os.WriteFile(path, []byte(prep+c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			sh.Stdin = f
			driver.MainArgs(sh, []string{"bash", "--norc", "--noprofile", "-i"})
			got := errs.String()
			if out.String() != c.out {
				t.Errorf("out %q, want %q", out.String(), c.out)
			}
			if n := strings.Count(got, "here-document at line"); n != c.n {
				t.Errorf("%d warnings in %q, want %d", n, got, c.n)
			}
			if c.n == 0 {
				return
			}
			if !strings.Contains(got, c.after+c.warn+c.before) {
				t.Errorf("%q not between %q and %q in %q", c.warn, c.after, c.before, got)
			}
		})
	}
}
