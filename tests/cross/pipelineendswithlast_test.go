// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"context"
	"os"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Whether a pipeline ends when its last element does, in each dialect —
// interp.Semantics.PipelineEndsWithItsLastElement (#7169). The rows are the
// issue's `{ /bin/sleep 0.3; echo first >&2; } | :; echo after` with the
// sleep made a handshake: the first element ends only once the shell has
// moved past the pipeline and created `go`, or once it has given up looking,
// four seconds on, in a shell that waits for it. So the order says which the
// shell did however loaded the machine is.
//
// Measured 2026-11-01 from -c with the sleep, both streams on one pipe:
// ksh93u+ 2012-08-01 writes `after` and then `first`, and a bare `wait` after
// the pipeline waits for `first`; bash 5.3.20 and 3.2.57, zsh 5.9.2, dash
// 0.5.12 and BusyBox ash 1.37.0 in the pinned alpine image write `first` and
// then `after`. With `set -o pipefail` ksh93 waits too: `first`, `after 3`.
// A last element of `( :; )` gives the same rows in every column.
//
// Both last elements are run because they take different roads here: `:`
// runs in the shell itself in ksh and zsh, and `( :; )` on a copy. In zsh the
// first road lists the elements before it as a job of their own, and that
// pipeline waits for them whatever this axis says, so the zsh row that can
// tell the axis's answers apart is the second.
//
// The streams are a file rather than the usual buffer, because an element is
// left running only where what it writes into is a descriptor it can hold a
// copy of, as ksh93's process does. See interp/strayelements.go.
const outlivesTheLast = `{ i=0; until [ -e go ] || [ $i -ge 80 ]; do /bin/sleep 0.05; i=$((i+1)); done; ` +
	`echo first >&2; exit 3; }`

func TestAPipelineEndsWithItsLastElementWhereTheShellsDo(t *testing.T) {
	t.Parallel()
	const then = `; echo "after $?"; : >go; wait; echo end`
	const src, sub = outlivesTheLast + ` | :` + then, outlivesTheLast + ` | ( :; )` + then
	const waits = "first\nafter 0\nend\n"
	for _, r := range []struct{ name, dialect, src, want string }{
		{"ksh", "ksh", src, "after 0\nfirst\nend\n"},
		{"ksh, last in a subshell", "ksh", sub, "after 0\nfirst\nend\n"},
		{"ksh, pipefail", "ksh", `set -o pipefail; ` + src, "first\nafter 3\nend\n"},
		{"bash", "bash", src, waits},
		{"zsh", "zsh", src, waits},
		{"zsh, last in a subshell", "zsh", sub, waits},
		{"dash", "dash", src, waits},
		{"ash", "ash", src, waits},
		{"posix", "posix", src, waits},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			if got := runToAFile(t, presets[r.dialect], r.src); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}

// runToAFile runs src in a directory of its own with both streams on one file,
// and returns what the file holds once the run is over.
func runToAFile(t *testing.T, p dialecttest.Preset, src string) string {
	t.Helper()
	dir := t.TempDir()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := p.Runner(dialecttest.Base{Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Stdout: f, Stderr: f})
	if _, err := r.Run(context.Background(), p.ParseThrough(t, r, src)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
