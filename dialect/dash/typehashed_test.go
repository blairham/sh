// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/testenv"
)

// TestTypeCallsAHashedExternalATrackedAlias is #3579 in this column.
//
// Measured 2026-09-18 against dash 0.5.12 at /bin/dash, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin /dev/null:
//
//	type ls;  ls /dev/null >/dev/null;  type ls;  command -V ls
//	  ls is /bin/ls
//	  ls is a tracked alias for /bin/ls
//	  ls is a tracked alias for /bin/ls
//
// The same words ksh93 writes and a different question: there they are the
// *first* answer and the discriminator is the slash (#2953), where here they
// arrive only once the name is in the command hash.
//
// **This shell's own `type` hashes the name and still writes the plain
// sentence for that same lookup** — `hash` afterwards shows `/bin/ls` — so
// the table is read before the search rather than after it. That is the row
// that says the report is about the table as it stood when the question was
// asked, and a report asking afterwards could never write the first sentence
// at all.
func TestTypeCallsAHashedExternalATrackedAlias(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "zzprog")
	if err := testenv.WriteExecutable(prog, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := dialecttest.Base{Dir: dir, Env: []string{"PATH=" + dir}}
	out, _, err := preset.Combined(t, base, `type zzprog; zzprog; type zzprog; command -V zzprog`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "zzprog is " + prog + "\n" +
		"zzprog is a tracked alias for " + prog + "\n" +
		"zzprog is a tracked alias for " + prog + "\n"
	if out != want {
		t.Errorf("said %q, want %q", out, want)
	}
	// And this shell's own `type` is what puts it there, with no run between
	// the two questions.
	out, _, err = preset.Combined(t, base, `type zzprog; type zzprog`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out, "zzprog is "+prog+"\n") {
		t.Errorf("said %q, want the plain sentence on the first lookup", out)
	}
	if !strings.HasSuffix(out, "zzprog is a tracked alias for "+prog+"\n") {
		t.Errorf("said %q, want the hashed sentence on the second", out)
	}
}
