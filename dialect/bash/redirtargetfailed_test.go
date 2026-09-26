// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A redirection **target** that will not expand, on a command this shell runs
// itself, is **this shell's own failed expansion** — not the redirection's.
//
// So it takes the *line*, which is what this shell does with any other failed
// expansion, and which no failed redirection costs it. Measured 2026-09-26 on
// bash 5.3.20 (#4689).
func TestAFailedRedirectTargetIsThisShellsExpansionHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// The discriminator, and it is the same word twice: an arithmetic
	// failure in an ordinary word takes the rest of the line here, and so
	// does the same failure in a target — where a file that will not open
	// leaves the line running and is caught by `||`.
	for _, c := range []struct {
		name, src string
		same      bool
	}{
		{"in an ordinary word", `echo $(( 1/0 )); echo SAME`, false},
		{"in a target", `: < $(( 1/0 )); echo SAME`, false},
		{"a failed open", `: < /nonexistent/f; echo SAME`, true},
	} {
		out, _ := runBash(t, dir, c.src+"\n")
		if got := strings.Contains(out, "SAME"); got != c.same {
			t.Errorf("%s: %q, want the rest of the line running=%v", c.name, out, c.same)
		}
	}
	if out, _ := runBash(t, dir, ": < $(( 1/0 )) || echo CAUGHT\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a failed target to escape || as any failed expansion does", out)
	}
	if out, _ := runBash(t, dir, ": < /nonexistent/f || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a failed open caught by ||", out)
	}
}

// And the command shape does not move it: the line is given up whatever the
// target was written on, where the redirection's reading would stop the shell
// on a special builtin and nothing else.
func TestAFailedRedirectTargetIgnoresTheCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: < $(( 1/0 ))`,
		`read x < $(( 1/0 ))`,
		`f() { echo RAN; }; f < $(( 1/0 ))`,
		`{ echo RAN; } < $(( 1/0 ))`,
	} {
		out, st := runBash(t, dir, src+"\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("%s = %q (status %d), want the next line running at 1", src, out, st)
		}
	}
}

// The kind of failure is this shell's own reading of it, which is the other
// half of "this shell's own failed expansion": `${q?word}` ends this shell in
// a target exactly as it does in a word.
func TestAParameterRefusalInATargetEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{`unset q; : < ${q?bad}`, `unset q; read x < ${q?bad}`} {
		if out, _ := runBash(t, dir, src+"\necho after\n"); strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
	}
	if out, _ := runBash(t, dir, "unset q; echo ${q?bad}\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want the same parameter in a word to end this shell", out)
	}
}

// The control: a target that expands is opened and its command runs.
func TestATargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runBash(t, dir, "echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
