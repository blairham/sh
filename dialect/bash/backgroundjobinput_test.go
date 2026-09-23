// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A background job's input substitution reaches the shell's **own** standard
// input and nothing a script put there.
//
// POSIX XCU 2.9.3 says an asynchronous command's standard input is an empty
// file while job control is off, and this shell read that as "whatever fd 0
// holds". It is narrower than that: `for … done < names` with a
// `{ read line; … } &` inside reads the file in the reference, and an empty
// input there prints blanks at status 0 with nothing said — the shape
// `while read -r line; do work "$line" & done < input` is written in.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file, six words in `names`:
//
//	                                  5.3.20  3.2.57  ksh93  zsh   dash  ash
//	the shell's own input             empty   empty   empty  read  empty empty
//	`exec < names` and then the job   empty   empty   empty  read  empty empty
//	a redirection on the enclosing    read    read    read   read  empty empty
//	  `for … done < names`
//	a pipeline feeding the group      read    empty   read   read  empty empty
//	a redirection on the job itself   read    read    read   read  read  read
//
// The first two rows are the control, and they are what makes this a question
// about the *stream* rather than about whether a redirection is in force:
// `exec` does not wrap a region, it replaces what the shell reads, so what it
// installs is the shell's own from then on.
//
// This is the six lines `redir.tests` parts on (#4153).
func TestABackgroundJobsInputSubstitutionIsOnlyTheShellsOwn(t *testing.T) {
	dir := t.TempDir()
	names := filepath.Join(dir, "names")
	if err := os.WriteFile(names, []byte("ab\ncd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, src string, stdin string) string {
		t.Helper()
		var buf strings.Builder
		b := dialecttest.Base{
			Name: "sh", Dir: dir, Stdout: &buf, Stderr: &buf,
			Env: []string{"PATH=/usr/bin:/bin"},
		}
		r := preset.Runner(b)
		if stdin != "" {
			f, err := os.Open(stdin)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					t.Error(err)
				}
			}()
			r.Stdin = f
		}
		if _, err := r.Run(t.Context(), preset.Parse(t, src)); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return buf.String()
	}
	const job = `{ read line; echo "[$line]"; } &` + "\nwait\n"
	for _, tc := range []struct{ name, src, stdin, want string }{
		{
			// The row the suite reaches: the redirection is on the loop and
			// the job inherits it.
			"a redirection on the enclosing loop",
			"for i in 1 2; do\n" + job + "done < names\n", "",
			"[ab]\n[cd]\n",
		},
		{
			// The control, and the axis this one sits beside: the shell's
			// own input is substituted away.
			"the shell's own input", job, names, "[]\n",
		},
		{
			// And `exec` makes its redirection the shell's own, so the
			// substitution follows it rather than stopping at the stream
			// the process started with.
			"an exec redirection", "exec < names\n" + job, "", "[]\n",
		},
		{
			// A redirection on the job itself is applied inside the job and
			// is unanimous across the panel.
			"a redirection on the job itself",
			"{ read line; echo \"[$line]\"; } < names &\nwait\n", "",
			"[ab]\n",
		},
		{
			// And a pipeline's pipe is a stream a script asked for too.
			"a pipeline feeding the group",
			"printf 'ab\\ncd\\n' | { " + job + " }\n", "",
			"[ab]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, tc.src, tc.stdin); got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// And the preset says so, pinned so a vector edit cannot put this column back
// on the reading that hands every job an empty input.
func TestTheSubstitutionIsThisDialectsAnswer(t *testing.T) {
	s := bash.Semantics()
	if got := s.BackgroundJobInputIsOnlyTheShellsOwn; got != interp.Yes {
		t.Errorf("BackgroundJobInputIsOnlyTheShellsOwn = %v, want Yes", got)
	}
	if got := s.BackgroundJobInput; got != interp.BackgroundJobInputEmpty {
		t.Errorf("BackgroundJobInput = %v, want empty", got)
	}
}
