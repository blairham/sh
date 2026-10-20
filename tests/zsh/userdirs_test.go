// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAnInteractiveShellsUserDirsAreThePasswordDatabase: in an interactive
// shell `$userdirs` holds every account to its home directory, which is what
// `compadd -k userdirs` offers `ssh <Tab>` as login names; in a script it is
// empty (#6156). Measured 2026-10-06 on zsh 5.9.2: `${#userdirs}` is 133
// under `-f -i -c` and `-i -c`, standard input `/dev/null`, and 0 under
// `-f -c`, `-c` and a script on standard input.
//
// The accounts are read from the file here, so the expectation is too: root's
// line is what the table must say about root.
func TestAnInteractiveShellsUserDirsAreThePasswordDatabase(t *testing.T) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		t.Skipf("no password file: %v", err)
	}
	homes := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) >= 7 && fields[0] != "" && fields[0][0] != '#' {
			if _, ok := homes[fields[0]]; !ok {
				homes[fields[0]] = fields[5]
			}
		}
	}
	_ = f.Close()
	root, ok := homes["root"]
	if !ok {
		t.Skip("no root in the password file")
	}
	const src = `print -r -- "n=$(( ${#userdirs} >= N )) root=${userdirs[root]-unset}"`
	src2 := strings.Replace(src, "N", strconv.Itoa(len(homes)), 1)

	interactive := dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"PATH": t.TempDir()}, Interactive: true}
	out, _, err := preset.Combined(t, interactive, src2)
	if err != nil {
		t.Fatal(err)
	}
	if want := "n=1 root=" + root + "\n"; out != want {
		t.Errorf("interactive: %q, want %q", out, want)
	}

	script := dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"PATH": t.TempDir()}}
	out, _, err = preset.Combined(t, script, `print -r -- "n=${#userdirs} root=${userdirs[root]-unset}"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "n=0 root=unset\n"; out != want {
		t.Errorf("in a script: %q, want %q", out, want)
	}
}
