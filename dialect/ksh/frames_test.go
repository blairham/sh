// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The frame chain ksh93 renders in front of a diagnostic, measured 2026-09-12
// against ksh93u+ 2012-08-01 with `env -i PATH=/usr/bin:/bin` and a scratch
// HOME. The corpus grades the same rule over a whole script; these are the
// shapes, written down where a reader looking for ksh93's wording will find
// them.
//
// Every frame but the innermost is `NAME[line]: `, where the line is the one
// *in that frame* that entered the frame above it; the innermost is located
// the way any other diagnostic in this dialect is. The suppression that leaves
// line 1 unwritten belongs to the shell's own name — the first component —
// and a frame entered after it names line 1 anyway.

// wrote puts body in dir under name and answers its path.
func wrote(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// located is the line of out that carries the diagnostic, which is the only
// one starting with the shell's own name here.
func located(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ksh") {
			return line
		}
	}
	t.Fatalf("no located diagnostic in %q", out)
	return ""
}

func TestAFailureInsideASourcedFileNamesTheDotItIsInside(t *testing.T) {
	dir := t.TempDir()
	inc := wrote(t, dir, "inc.sh", "nosuchcmd-xyz\n")
	out, _ := runKsh(t, dir, "echo pad\n. "+inc+"\n")
	if got, want := located(t, out), "ksh[2]: .: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

func TestAFileSourcedByASourcedFileIsTwoFrames(t *testing.T) {
	dir := t.TempDir()
	inner := wrote(t, dir, "inner.sh", "nosuchcmd-xyz\n")
	mid := wrote(t, dir, "mid.sh", "echo mid\n. "+inner+"\n")
	out, _ := runKsh(t, dir, "echo pad\n. "+mid+"\n")
	if got, want := located(t, out), "ksh[2]: .[2]: .: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

func TestAnEvalInsideASourcedFileMixesTheTwoKinds(t *testing.T) {
	dir := t.TempDir()
	inc := wrote(t, dir, "inc.sh", "eval 'nosuchcmd-xyz'\n")
	out, _ := runKsh(t, dir, "echo pad\n. "+inc+"\n")
	if got, want := located(t, out), "ksh[2]: .[1]: eval: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestOnlyTheShellsOwnNameLeavesLineOneOut is the pair that separates the two
// readings of the suppression: `ksh: eval: line 1:` for an `eval` on the first
// line, and `ksh: eval[1]: eval: line 1:` for one inside it. A rule applying
// the suppression to every frame writes `ksh: eval: eval: line 1:` for the
// second and passes the first.
func TestOnlyTheShellsOwnNameLeavesLineOneOut(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, `eval 'nosuchcmd-xyz'`)
	if got, want := located(t, out), "ksh: eval: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("one eval located %q, want a %q prefix", got, want)
	}
	out, _ = runKsh(t, dir, `eval 'eval "nosuchcmd-xyz"'`)
	if got, want := located(t, out), "ksh: eval[1]: eval: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("eval in eval located %q, want a %q prefix", got, want)
	}
}

// TestABuiltinsComplaintTakesTheBracketAtBothEnds: this shell locates a
// builtin's own complaint `NAME[line]:` and a message it speaks itself `NAME:
// line N:`, and the choice is made at the end of a chain exactly as it is
// without one.
func TestABuiltinsComplaintTakesTheBracketAtBothEnds(t *testing.T) {
	dir := t.TempDir()
	inc := wrote(t, dir, "inc.sh", "echo one\ncd /nonexistent-xyz\n")
	out, _ := runKsh(t, dir, "echo pad\n. "+inc+"\n")
	if got, want := located(t, out), "ksh[2]: .[2]: cd: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestUnsetOfAReadonlyNameIsTheBuiltinsComplaint is the other half of that
// choice, and the one this tree used to get wrong: the refusal is `unset`'s
// and takes the bracket, where the assignment refused for the same reason is
// the shell's and takes the word.
func TestUnsetOfAReadonlyNameIsTheBuiltinsComplaint(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "readonly r=1\nunset r\n")
	if got, want := located(t, out), "ksh[2]: unset: "; !strings.HasPrefix(got, want) {
		t.Errorf("unset located %q, want a %q prefix", got, want)
	}
	out, _ = runKsh(t, dir, "readonly r=1\nr=2\n")
	if got, want := located(t, out), "ksh: line 2: "; !strings.HasPrefix(got, want) {
		t.Errorf("assignment located %q, want a %q prefix", got, want)
	}
}

// TestAParseFailureInsideASourcedFileCarriesTheChain: the parse path is
// different code and the same rule reaches it, with no line of its own at the
// end because this shell's wording already carries one.
func TestAParseFailureInsideASourcedFileCarriesTheChain(t *testing.T) {
	dir := t.TempDir()
	inc := wrote(t, dir, "inc.sh", "echo a\nif true\n")
	out, _ := runKsh(t, dir, "echo pad\n. "+inc+"\n")
	got := located(t, out)
	if want := "ksh[2]: .: syntax error at line "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
	if strings.Contains(got, ".: line ") {
		t.Errorf("located %q: the wording names the line, so the location must not", got)
	}
}

// TestNothingBorrowedIsStillLocatedAsBefore is the control: a diagnostic at
// the top level of what the shell was given has no frame, and the chain must
// leave it exactly where it was.
func TestNothingBorrowedIsStillLocatedAsBefore(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "echo pad\nnosuchcmd-xyz\n")
	if got, want := located(t, out), "ksh: line 2: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}
