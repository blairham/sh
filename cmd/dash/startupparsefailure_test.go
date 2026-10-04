// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An `$ENV` that will not parse runs up to the line that will not, and the
// command the shell was started for still runs (#5869).
//
// Measured 2026-10-04 on dash 0.5.12 (/bin/dash) with standard input on the null device, `$ENV`
// holding `echo before`, `false`, `echo )`, `echo after`. Each row is that
// shell's own output, with the file's path standing for @. This shell used to
// parse the file whole first, so it ran none of it and ended before the
// command, at the parse failure's status.
func TestAnEnvFileThatWillNotParseRunsUpToTheFailure(t *testing.T) {
	for _, c := range []struct {
		name, out, errs string
		argv            []string
	}{
		{
			// Named by the shell and the file's own line, as everything said
			// from inside the file is.
			name: "under -i",
			argv: []string{"dash", "-i", "-c", "echo main $?"},
			out:  "before\nmain 2\n",
			errs: "dash: 3: Syntax error: \")\" unexpected\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			env := filepath.Join(home, "env.sh")
			if err := os.WriteFile(env, []byte("echo before\nfalse\necho )\necho after\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ENV", env)
			null, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = null.Close() })
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, null
			code := driver.MainArgs(sh, c.argv)
			if code != 0 {
				t.Errorf("status %d, want the command's own 0", code)
			}
			if o.String() != c.out {
				t.Errorf("stdout %q, want %q", o.String(), c.out)
			}
			// The remark an interactive shell makes about the terminal it
			// has not got comes first, and is not what this is about.
			if want := strings.ReplaceAll(c.errs, "@", env); !strings.HasSuffix(e.String(), want) {
				t.Errorf("stderr %q, want it to end %q", e.String(), want)
			}
		})
	}
}
