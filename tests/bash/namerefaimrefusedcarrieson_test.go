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

// A write that would aim a name reference at a word that is no name is refused
// in bash's own sentence and the shell carries on, which is the other column of
// Semantics.RefusedReferenceAimIsFatal. Measured 2026-11-01 on bash 5.3.20
// from script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// each with `echo next` on the line after: the bare assignment gives up the
// rest of its line, and the loop and the declaration report 1 and go on
// (#7085).
func TestARefusedReferenceAimCarriesOnHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the bare assignment", `declare -n m; m=1; echo st=$?`, "p.sh: line 1: `1': not a valid identifier\nnext\n"},
		{
			"a loop over an aimed reference", `x=0; declare -n m=x; for m in 1; do :; done; echo st=$?`,
			"p.sh: line 1: `1': not a valid identifier\nst=1\nnext\n",
		},
		{
			"a declaration", `declare -n m; declare m=1; echo t=$?`,
			"p.sh: line 1: declare: `1': not a valid identifier\nt=1\nnext\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "p.sh")
			if err := os.WriteFile(path, []byte(c.src+"\necho next\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var both bytes.Buffer
			st := driver.MainArgs(bashShell(&both, &both), []string{"bash", path})
			got := strings.ReplaceAll(both.String(), dir+string(filepath.Separator), "")
			if got != c.want || st != 0 {
				t.Errorf("%q at %d, want %q at 0", got, st, c.want)
			}
		})
	}
}
