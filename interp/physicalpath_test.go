// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// physicalTree builds a directory a `-P` resolution has to walk through, and
// answers with a root that is already resolved — a temporary directory is
// itself reached through a symlink on some systems, and a test that compared
// paths without allowing for that would be measuring the machine.
//
// Its shape: real/sub is where things are, link points at real, back is a
// link out of hidden into open, and cyc_a and cyc_b point at each other.
func physicalTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"real/sub", "hidden", "open"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range []struct{ target, name string }{
		{"real", "link"},
		{filepath.Join(root, "open"), "hidden/back"},
		{"cyc_b", "cyc_a"},
		{"cyc_a", "cyc_b"},
	} {
		if err := os.Symlink(l.target, filepath.Join(root, l.name)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// physicalRun runs src with a gate that records every path it is asked about
// and refuses the ones under deny, and returns what the shell printed, what
// it said, its status, and the paths the gate saw.
func physicalRun(t *testing.T, dir, src string, deny []string) (out, errs string, status int, asked []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	var denied []string
	refuse := func(p string) bool {
		for _, d := range deny {
			if p == d || strings.HasPrefix(p, d+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	var stdout, stderr strings.Builder
	sem := CoreSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Dir: dir, Stdout: &stdout, Stderr: &stderr,
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			mu.Lock()
			defer mu.Unlock()
			if a.Kind == ActionStat {
				seen = append(seen, a.Path)
			}
			if a.Kind == ActionStat && refuse(a.Path) {
				return Deny
			}
			return Allow
		}),
		// The sink is here because a gate and an event stream are two
		// separate claims: a walk the gate is asked about but that nothing
		// records is still a hole in the audit trail.
		Events: SinkFunc(func(_ context.Context, e Event) {
			mu.Lock()
			defer mu.Unlock()
			if e.Kind == EventDenied && e.Action.Kind == ActionStat {
				denied = append(denied, e.Action.Path)
			}
		}),
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	mu.Lock()
	defer mu.Unlock()
	// Every refusal the gate made has a record; without that the sink holds
	// a walk that half happened.
	if len(deny) > 0 && len(denied) == 0 {
		t.Errorf("the gate refused nothing it was asked about in %q", src)
	}
	return stdout.String(), stderr.String(), st, seen
}

func askedAbout(asked []string, path string) bool {
	for _, p := range asked {
		if p == path {
			return true
		}
	}
	return false
}

// Resolving a path to where it physically is walks the filesystem, so every
// component of that walk passes the gate.
//
// It did not. `cd -P` and `pwd -P` handed the whole path to the standard
// library, which lstats and reads each component through the os package, so a
// policy saw one stat of an answer it had no part in reaching — and a subtree
// it was refusing could be walked through and reported on. The check that
// followed was gated and too late: the resolved path was already in hand.
func TestThePhysicalWalkPassesTheGateAtEveryComponent(t *testing.T) {
	root := physicalTree(t)
	link := filepath.Join(root, "link")
	real := filepath.Join(root, "real")
	sub := filepath.Join(real, "sub")

	for _, tc := range []struct{ name, src string }{
		{"cd -P", "cd -P " + filepath.Join(link, "sub")},
		{"pwd -P", "pwd -P"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// `pwd -P` resolves where the runner already is, so it starts
			// from the name rather than from an operand.
			dir := root
			if tc.src == "pwd -P" {
				dir = filepath.Join(link, "sub")
			}
			_, _, _, asked := physicalRun(t, dir, tc.src, nil)
			// The link itself, what it pointed at, and what was left of the
			// path afterwards — the three steps the library call swallowed.
			for _, want := range []string{link, real, sub} {
				if !askedAbout(asked, want) {
					t.Errorf("the gate was never asked about %s; it saw %v", want, asked)
				}
			}
			// And the walk really is a walk: the ancestors above the tree
			// are components too, and each one can hold a symlink.
			if !askedAbout(asked, filepath.Dir(root)) {
				t.Errorf("the gate was never asked about the parent %s; it saw %v",
					filepath.Dir(root), asked)
			}
		})
	}
}

// A refused component is a resolution that did not happen, and `cd -P` fails
// the way it fails for a directory that is not there.
//
// This is the consequence the missing gate had no way to produce: the link
// leads out of the refused subtree into one the policy allows, so before this
// the script walked through what it was denied, learned where the link
// pointed, and ended up standing there.
func TestARefusedComponentStopsAPhysicalCd(t *testing.T) {
	root := physicalTree(t)
	hidden := filepath.Join(root, "hidden")
	open := filepath.Join(root, "open")

	out, errs, status, _ := physicalRun(t, root,
		"cd -P "+filepath.Join(hidden, "back")+"\necho st=$?\necho PWD=$PWD\n",
		[]string{hidden})

	if !strings.Contains(out, "st=1") {
		t.Errorf("cd -P through a refused component gave %q, want a failure", out)
	}
	if status != 0 {
		t.Errorf("the script exited %d; the echo after cd should still run", status)
	}
	if !strings.Contains(errs, "No such file") {
		t.Errorf("said %q, want the refusal to read as a missing path", errs)
	}
	// The point of the whole exercise: the resolved target never reached the
	// script. Before the walk was gated, PWD held it.
	if strings.Contains(out, open) {
		t.Errorf("output %q names %s — the refused walk still resolved the link", out, open)
	}
	if !strings.Contains(out, "PWD="+root+"\n") {
		t.Errorf("output %q, want the shell still standing in %s", out, root)
	}
}

// `pwd -P` is a question about the filesystem, and a policy that hides part of
// it refuses the answer. What is printed then is the name as held, which is
// what every shell in the panel prints for a path it cannot resolve.
func TestARefusedComponentLeavesPwdPhysicalWithTheName(t *testing.T) {
	root := physicalTree(t)
	logical := filepath.Join(root, "link", "sub")
	physical := filepath.Join(root, "real", "sub")

	out, _, _, _ := physicalRun(t, logical, "pwd -P\n", []string{filepath.Join(root, "real")})

	if strings.TrimSpace(out) != logical {
		t.Errorf("pwd -P = %q, want the unresolvable path printed as held (%q)",
			strings.TrimSpace(out), logical)
	}
	if strings.Contains(out, physical) {
		t.Errorf("pwd -P = %q — it reported a location the policy hides", out)
	}
}

// A resolver written by hand has to end. Two links pointing at each other are
// a path with no bottom, and the panel is unanimous that both `-P` and `-L`
// report a cycle as too many levels of symbolic links — the kernel's answer,
// which arrives from the check that follows an abandoned resolution.
//
// The gate assertion is the half that could only pass with the walk gated:
// reaching the second link at all means the first one's target was read.
func TestAPhysicalCdIntoASymlinkCycleEndsAndFails(t *testing.T) {
	root := physicalTree(t)

	out, errs, _, asked := physicalRun(t, root, "cd -P cyc_a\necho st=$?\n", nil)

	if !strings.Contains(out, "st=1") {
		t.Errorf("cd -P into a cycle gave %q, want a failure", out)
	}
	if !strings.Contains(errs, "symbolic links") {
		t.Errorf("said %q, want the cycle reported as too many levels of symbolic links", errs)
	}
	if !askedAbout(asked, filepath.Join(root, "cyc_b")) {
		t.Errorf("the gate was never asked about the second link of the cycle; it saw %v", asked)
	}
}

// `cd -L` keeps the name it was reached by, and keeping it means not walking
// it. This is a regression guard rather than a claim about the gate: it passes
// with the walk gated and with the walk done behind the gate's back, and it is
// here because making `-P` resolve component by component is exactly the
// change that could quietly make `-L` do it too.
func TestALogicalCdDoesNotWalkThePath(t *testing.T) {
	root := physicalTree(t)
	logical := filepath.Join(root, "link", "sub")

	out, _, _, asked := physicalRun(t, root, "cd -L "+logical+"\necho PWD=$PWD\n", nil)

	if !strings.Contains(out, "PWD="+logical+"\n") {
		t.Errorf("cd -L left PWD as %q, want the name it was reached by (%s)", out, logical)
	}
	if askedAbout(asked, filepath.Join(root, "real")) {
		t.Errorf("cd -L resolved the link: the gate saw %v", asked)
	}
}
