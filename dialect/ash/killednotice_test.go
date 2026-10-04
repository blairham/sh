// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestACommandASignalEndedIsTheWordsAlone: no shell name, no line, no process
// id and no command — measured 2026-10-03 in the pinned image, where
// `/bin/sh -c 'kill -TERM $$'` writes `Terminated` and nothing else. The
// words themselves are the machine's, so only their company is checked.
func TestACommandASignalEndedIsTheWordsAlone(t *testing.T) {
	out, _ := run(t, `/bin/sh -c 'kill -TERM $$' 2>&1; echo "st=$?"`)
	line, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(line, "Terminated") || strings.Contains(line, "kill") || !strings.HasSuffix(out, "st=143\n") {
		t.Errorf("got %q, want the words alone and then st=143", out)
	}
}
