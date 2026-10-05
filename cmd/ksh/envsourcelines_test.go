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

// A file `.` reads from `$ENV` is numbered on from the `.`'s line, and `$ENV`'s
// own later lines move on by every line the file held (#6013). See
// interp.Semantics.SourcedFileContinuesTheStartupLineCount.
//
// Measured 2026-10-05 on ksh93u+ 2012-08-01 (/bin/ksh), `env -i
// PATH=/usr/bin:/bin`, `ksh -E -c :`; each line is that shell's with the
// `$ENV` path standing for @. This shell numbered every file from its own
// first line: `.: line 2: noA2`, and `line 3: nosuch3`.
func TestAFileSourcedFromEnvContinuesItsLineCount(t *testing.T) {
	for _, c := range []struct {
		name, env string
		files     map[string]string
		want      string
	}{
		{
			"two dots",
			"nosuch1\n. ./a.sh\nnosuch3\n. ./a.sh\nnosuch5\n",
			map[string]string{"a.sh": "true\nnoA2\nnoA3\n"},
			"@: nosuch1: not found\n" +
				"@[2]: .: line 4: noA2: not found\n@[2]: .: line 5: noA3: not found\n" +
				"@: line 6: nosuch3: not found\n" +
				"@[7]: .: line 9: noA2: not found\n@[7]: .: line 10: noA3: not found\n" +
				"@: line 11: nosuch5: not found\n",
		},
		{
			"a dot inside a dot",
			"nosuch1\n. ./c.sh\nnosuch3\n",
			map[string]string{"a.sh": "true\nnoA2\nnoA3\n", "c.sh": "noC1\n. ./a.sh\nnoC3\n"},
			"@: nosuch1: not found\n@[2]: .: line 3: noC1: not found\n" +
				"@[2]: .[4]: .: line 6: noA2: not found\n@[2]: .[4]: .: line 7: noA3: not found\n" +
				"@[2]: .: line 8: noC3: not found\n@: line 9: nosuch3: not found\n",
		},
		{
			// eval is the control: it moves nothing.
			"eval",
			"nosuch1\neval \"true\nnoE2\"\nnosuch4\n",
			nil,
			"@: nosuch1: not found\n@[2]: eval: line 2: noE2: not found\n@: line 4: nosuch4: not found\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)
			env := filepath.Join(home, "env.sh")
			if err := os.WriteFile(env, []byte(c.env), 0o600); err != nil {
				t.Fatal(err)
			}
			for name, body := range c.files {
				if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("ENV", env)
			null, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = null.Close() }()
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, null
			driver.MainArgs(sh, []string{"ksh", "-E", "-c", ":"})
			if want := strings.ReplaceAll(c.want, "@", env); e.String() != want {
				t.Errorf("stderr:\n%s\nwant:\n%s", e.String(), want)
			}
		})
	}
}
