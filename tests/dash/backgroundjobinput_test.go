// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Here the substitution reaches **every** inherited stream, not the shell's
// own alone — which is the other side of the axis and is this column's answer
// rather than an absence of one.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C dash f.sh`
// over a script file: a `{ read line; echo "[$line]"; } &` inside
// `for … done < names` reads nothing here, where bash, ksh93 and zsh all read
// the file. BusyBox ash 1.38.0 answers every shape exactly as this does.
//
// A redirection on the **job itself** still wins, in this column as in every
// other: it is applied inside the job's own runner, after the substitution.
// That is the control that says this is about what the job *inherits*.
func TestEveryInheritedStreamIsSubstitutedAway(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "names"), []byte("ab\ncd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, src string) string {
		t.Helper()
		var buf strings.Builder
		r := preset.Runner(dialecttest.Base{
			Name: "sh", Dir: dir, Stdout: &buf, Stderr: &buf,
			Env: []string{"PATH=/usr/bin:/bin"},
		})
		if _, err := r.Run(t.Context(), preset.Parse(t, src)); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return buf.String()
	}
	const job = `{ read line; echo "[$line]"; } &` + "\nwait\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			"a redirection on the enclosing loop",
			"for i in 1 2; do\n" + job + "done < names\n",
			"[]\n[]\n",
		},
		{
			"a redirection on the job itself",
			"{ read line; echo \"[$line]\"; } < names &\nwait\n",
			"[ab]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, tc.src); got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// And the preset says so.
func TestEveryStreamIsThisDialectsAnswer(t *testing.T) {
	if got := dash.Semantics().BackgroundJobInputIsOnlyTheShellsOwn; got != interp.No {
		t.Errorf("BackgroundJobInputIsOnlyTheShellsOwn = %v, want No", got)
	}
}
