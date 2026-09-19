// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/testenv"
)

// A third wording for an external, once the name is in the command hash — see
// Diagnostics.TypeHashedExternal (#3579).
//
// Measured 2026-09-18, a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with stdin /dev/null, asking `type ls`, running `ls`, and asking
// again:
//
//	              first lookup     after it has run
//	bash 5.3.20   ls is /bin/ls    ls is hashed (/bin/ls)
//	dash 0.5.12   ls is /bin/ls    ls is a tracked alias for /bin/ls
//	zsh 5.9.2     ls is /bin/ls    the same
//	ksh93u+       tracked alias    the same
//	BusyBox ash   ls is /bin/ls    the same
//
// `command -V` writes whichever sentence `type` writes in every column, so
// this is one wording and not two. Empty means the same sentence either way,
// which is what three of the five answer.
//
// **The table is read before the lookup**, and that is measured rather than
// tidy: dash's own `type ls` puts the name in the table and still writes the
// plain sentence for that same lookup — `hash` afterwards shows `/bin/ls` and
// the line said `ls is /bin/ls`. A report that asked the table after its own
// search would never write the first of the two sentences at all.
func TestTypeHasAThirdSentenceOnceTheNameIsHashed(t *testing.T) {
	dir := t.TempDir()
	if err := testenv.WriteExecutable(filepath.Join(dir, "zzprog"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(dir, "zzprog")
	setup := func(r *Runner) {
		r.Env = []string{"PATH=" + dir}
		r.Diagnostics = &Diagnostics{TypeHashedExternal: "%[1]s is hashed (%[2]s)"}
	}
	// Two `type`s and nothing between them, which is the discriminating
	// probe: this engine's own `type` hashes the name it resolved, exactly as
	// dash does, so a report that asked the table *after* its own search
	// would write the hashed sentence both times and the first of the two
	// wordings would be unreachable.
	out, _ := run(t, `type zzprog; type zzprog`, setup)
	first, rest, cut := strings.Cut(out, "\n")
	if !cut {
		t.Fatalf("said %q, want two lines", out)
	}
	if want := "zzprog is " + prog; first != want {
		t.Errorf("the first lookup said %q, want %q", first, want)
	}
	if want := "zzprog is hashed (" + prog + ")\n"; rest != want {
		t.Errorf("after running, said %q, want %q", rest, want)
	}
	// Empty is the usual answer and means one sentence either way.
	out, _ = run(t, `type zzprog; zzprog; type zzprog`, func(r *Runner) {
		r.Env = []string{"PATH=" + dir}
		r.Diagnostics = &Diagnostics{}
	})
	if want := "zzprog is " + prog + "\n"; out != want+want {
		t.Errorf("with no hashed wording, said %q, want %q twice", out, want)
	}
}
