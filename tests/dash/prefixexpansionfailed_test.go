// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A command whose assignment prefix could not be expanded does not run, and
// here the failure ends the shell whatever the command word was — this shell
// makes no child before it evaluates a prefix, so there is nothing for the
// failure to be contained in. Measured 2026-09-26 on dash 0.5.12, each snippet
// on its own line, at the fatal status this shell carries (#4675).
func TestAPrefixThatWouldNotExpandEndsTheShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`a=$((1/0)) echo RAN`,
		`a=$((1/0)) :`,
		`f() { echo RAN; }; a=$((1/0)) f`,
		`a=$((1/0)) /bin/echo RAN`,
		`a=$((1/0)) command /bin/echo RAN`,
		`a=${q?bad} echo RAN`,
		`a=${q?bad} /bin/echo RAN`,
	} {
		out, st := runDash(t, dir, src+"\necho after\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st != 2 {
			t.Errorf("%s: status = %d, want 2", src, st)
		}
	}
}

// The control: a prefix that expands runs its command with the value in place.
func TestACleanPrefixStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runDash(t, dir, "f() { echo \"[$a]\"; }\na=ok f\n"); out != "[ok]\n" || st != 0 {
		t.Errorf("= %q (status %d), want [ok] at 0", out, st)
	}
}
