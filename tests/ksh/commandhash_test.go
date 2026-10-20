// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// ksh93 calls the table of remembered locations *tracked aliases*, and a
// tracked entry does not stand in front of the search: a copy that appears in
// a directory PATH looks at **earlier** is found on the next call, with no
// assignment to PATH in between. bash, zsh, dash and BusyBox ash all keep the
// location they remembered (#2936).
func TestATrackedEntryDoesNotShadowAnEarlierDirectory(t *testing.T) {
	if got := ksh.Semantics().HashedPathShadowsAnEarlierDirectory; got != interp.No {
		t.Errorf("HashedPathShadowsAnEarlierDirectory = %v, want No", got)
	}

	dir := t.TempDir()
	for _, sub := range []string{"bin1", "bin2"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bin2", "tool2"),
		[]byte("#!/bin/sh\nprintf 'bin2 tool2\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// bin1 is searched first and is empty, so the first call has to reach
	// bin2 — which is what makes the second call's answer the measurement
	// rather than the order of the directories.
	out, st := runKsh(t, dir, `PATH="$PWD/bin1:$PWD/bin2:/usr/bin:/bin"
tool2
printf '#!/bin/sh\nprintf "bin1 tool2\\n"\n' > bin1/tool2; chmod 755 bin1/tool2
tool2
hash | grep tool2 | sed "s|$PWD/||"`)
	const want = "bin2 tool2\nbin1 tool2\ntool2=bin1/tool2\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
