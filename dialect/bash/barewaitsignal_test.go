// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// TestABareWaitReportsASignalDeathItReaps pins that a bare `wait` writes the
// row a named one writes for a job a loud signal killed while the wait was
// waiting for it, and nothing for a quiet signal or for a job that had been
// collected before the wait began (#5699). Measured 2026-10-03 on bash 5.3.20.
// The row names the signal in the host's words — `Killed: 9` on macOS,
// `Killed` on Linux — so only the name is asserted.
func TestABareWaitReportsASignalDeathItReaps(t *testing.T) {
	pids := regexp.MustCompile(`[0-9]{3,} `)
	for _, tc := range []struct{ src, want, row string }{
		{`/bin/sleep 5 & p=$!; (/bin/sleep 0.2; kill -9 $p) & wait; echo st=$?`, "st=0\n", "Killed"},
		{`/bin/sleep 5 & p=$!; (/bin/sleep 0.2; kill -SEGV $p) & wait; echo st=$?`, "st=0\n", "Segmentation fault"},
		{`/bin/sleep 5 & p=$!; (/bin/sleep 0.2; kill $p) & wait; echo st=$?`, "st=0\n", ""},
		{`/bin/sleep 5 & kill -9 %1; /bin/sleep 0.3; wait; echo st=$?`, "st=0\n", ""},
	} {
		got, _ := runBash(t, t.TempDir(), tc.src)
		got = pids.ReplaceAllString(got, "N ")
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s\n got %q\nwant it to end %q", tc.src, got, tc.want)
		}
		if has := strings.Contains(got, "N "+tc.row); tc.row != "" && !has {
			t.Errorf("%s\n got %q\nwant the row %q", tc.src, got, tc.row)
		}
		if tc.row == "" && strings.Count(got, "\n") != 1 {
			t.Errorf("%s\n got %q\nwant no row", tc.src, got)
		}
	}
}
