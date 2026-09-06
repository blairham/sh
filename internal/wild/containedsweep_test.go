// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/wild"
)

// The plumbing a contained sweep rests on, asserted from the shell's side.
//
// The policy is written per run into a directory that exists for the length
// of that run, so nothing outside the sweep can look at it afterwards. What
// can be checked is what the shell under test was actually handed, and the
// only witness to that is the shell — so these fixtures answer by reading
// their own arguments and their own policy file.

// reportsItsPolicy is a stand-in shell that says whether it was given a
// policy and whether that policy names the directory it was started in.
func reportsItsPolicy(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reports-its-policy")
	const body = `#!/bin/sh
if [ "$1" != "-policy" ]; then echo no-policy; exit 0; fi
if grep -q "^allow write $HOME/\*\*$" "$2"; then
  echo policy-names-my-own-directory
else
  echo policy-names-somewhere-else
fi
`
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// aScriptThatDoesNothing is a script for the sweep to run. Its contents do
// not matter: the fixtures above never read it.
func aScriptThatDoesNothing(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// oursSaid digs the shell-under-test's output out of a sweep whose two sides
// were always going to disagree.
func oursSaid(t *testing.T, ours wild.UnderTest, script string) string {
	t.Helper()
	rep := wild.RunSweep(context.Background(), []string{script}, ours, "/bin/sh", 10*time.Second)
	if len(rep.Mismatches) == 0 {
		t.Fatalf("the fixture agreed with /bin/sh, so it said nothing: %+v", rep)
	}
	return rep.Mismatches[0].Ours
}

// TestAContainedRunIsHandedAPolicyNamingItsOwnDirectory.
//
// The directory is made per run and its name is different every time, so the
// only thing that can check the policy names *that* one is something running
// inside it. This asserts the whole chain: the flag reaches the binary, the
// file is there when the binary looks, and the rule in it is about the place
// the run was put.
func TestAContainedRunIsHandedAPolicyNamingItsOwnDirectory(t *testing.T) {
	got := oursSaid(t, wild.UnderTest{Path: reportsItsPolicy(t), Contained: true}, aScriptThatDoesNothing(t))
	if !strings.Contains(got, "policy-names-my-own-directory") {
		t.Errorf("the contained shell said %q", got)
	}
}

// TestAnUncontainedRunIsHandedNothing, which is the half that would catch a
// flag that had become unconditional — a sweep that always passed a policy
// would report the boundary working on a run nobody asked to contain.
func TestAnUncontainedRunIsHandedNothing(t *testing.T) {
	got := oursSaid(t, wild.UnderTest{Path: reportsItsPolicy(t)}, aScriptThatDoesNothing(t))
	if !strings.Contains(got, "no-policy") {
		t.Errorf("an uncontained shell said %q", got)
	}
}

// TestTheReferenceIsNeverContained. Handing a real shell a flag it does not
// have would end its run and turn every script into a disagreement, which
// would read as a sweep full of findings.
func TestTheReferenceIsNeverContained(t *testing.T) {
	fixture := reportsItsPolicy(t)
	rep := wild.RunSweep(context.Background(), []string{aScriptThatDoesNothing(t)},
		wild.UnderTest{Path: fixture, Contained: true}, fixture, 10*time.Second)
	if len(rep.Mismatches) == 0 {
		t.Fatal("the same fixture on both sides agreed, so the reference was contained too")
	}
	if got := rep.Mismatches[0].Theirs; !strings.Contains(got, "no-policy") {
		t.Errorf("the reference said %q, so it was handed a policy", got)
	}
}

// TestArgsReachTheBinaryBeforeTheScript, which is what lets the substrate
// binary be swept as the dialect it is being compared against.
func TestArgsReachTheBinaryBeforeTheScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "echoes-its-first-argument")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho \"first=[$1]\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := oursSaid(t, wild.UnderTest{Path: path, Args: []string{"-dialect", "bash"}}, aScriptThatDoesNothing(t))
	if !strings.Contains(got, "first=[-dialect]") {
		t.Errorf("the binary saw %q, want its own flags before the script", got)
	}
}
