// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryPanelMemberIsReachedByARoute pins the shape rather than the
// contents: a member either says nothing, and is a binary on this machine, or
// names a Reach. What it forbids is the third thing — a member with no route
// at all, which used to be impossible only because the route was not a value.
func TestEveryPanelMemberIsReachedByARoute(t *testing.T) {
	t.Parallel()
	for _, s := range Panel {
		if len(s.Lookup) == 0 {
			t.Errorf("%s: no candidate paths, so neither route can find it", s.Name)
		}
		if s.Via == nil {
			continue // LocalReach, which is the answer for all but one
		}
		if s.Via.route() == "" {
			t.Errorf("%s: its route cannot say what it is, so an absence cannot say why", s.Name)
		}
	}
}

// TestAContainerColumnIsPinnedByDigest is the provenance rule, and it is a
// test rather than a comment because the failure it prevents is silent.
//
// A tag moves. `alpine:3` is a different BusyBox every few weeks, so a column
// recorded against the tag would change underneath its own drift check, and
// the change would be indistinguishable from a shell that behaved
// differently — which is the one thing the golden record exists to tell
// apart. Moving to a newer image has to be an edit here that shows in a diff.
func TestAContainerColumnIsPinnedByDigest(t *testing.T) {
	t.Parallel()
	containers := 0
	for _, s := range Panel {
		c, ok := s.Via.(*ContainerReach)
		if !ok {
			continue
		}
		containers++
		if !strings.HasPrefix(c.Digest, "sha256:") || len(c.Digest) != len("sha256:")+64 {
			t.Errorf("%s: %q is not a digest, so the column pins a moving target", s.Name, c.Digest)
		}
		if strings.ContainsAny(c.Image, ":@") {
			t.Errorf("%s: image %q carries a tag; the digest is what is pulled and the repository is what a reader looks up", s.Name, c.Image)
		}
		if s.MustReport == "" {
			t.Errorf("%s: a container column with no MustReport records whatever shell the image happens to have at that path", s.Name)
		}
	}
	if containers == 0 {
		t.Error("no panel member is reached through a container, so this guard proves nothing; ash was one when it was written")
	}
}

// TestTheCommittedRecordKeepsTheAshColumn is the guard against the way this
// column can rot back to nothing, which is not a code change at all.
//
// `make oracle` on a machine with no container runtime writes a record with
// no ash column and says so loudly while it does it — but the record it
// writes is still committable, and a record with the column silently gone is
// the state this whole issue was about. Reading the committed file costs no
// shells and gives the same answer everywhere, so it is the right place to
// say the column has to be there.
func TestTheCommittedRecordKeepsTheAshColumn(t *testing.T) {
	t.Parallel()
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatalf("golden record: %v", err)
	}
	recorded := map[string]bool{}
	for _, s := range golden.Shells {
		recorded[s.Name] = true
		if s.Version == "" {
			t.Errorf("%s: recorded with no build string", s.Name)
		}
	}
	if !recorded["ash"] {
		t.Fatal("the golden record has no ash column: it was regenerated where the container could not run.\n" +
			"\tRegenerate it where it can, rather than committing a record that says nothing about dialect/ash")
	}
	missing := 0
	for _, c := range Corpus {
		if row, ok := golden.Results[c.ID]; ok {
			if _, ok := row["ash"]; !ok {
				missing++
			}
		}
	}
	if missing > 0 {
		t.Errorf("%d recorded case(s) have no ash cell; a half-recorded column grades a subset nobody chose", missing)
	}
}

// TestAnAbsentColumnCarriesItsReason is the honest-degradation rule.
//
// A missing column is only safe while nobody can mistake it for an agreeing
// one, and a bare list of names is exactly the shape that gets skimmed as the
// second. This repository's recorded scar is the same thing from the other
// side: a gate inert on one platform reads as a pass.
func TestAnAbsentColumnCarriesItsReason(t *testing.T) {
	t.Parallel()
	_, absent := Resolve(context.Background())
	for _, a := range absent {
		if a.Reason == "" {
			t.Errorf("%s is absent with no reason, so nothing distinguishes it from a column that agreed", a.Name)
		}
		if !strings.Contains(a.String(), a.Name) || !strings.Contains(a.String(), a.Reason) {
			t.Errorf("Absence.String() = %q, which loses one half of what it is for", a.String())
		}
	}
	if got := Names(absent); len(got) != len(absent) {
		t.Errorf("Names() lost %d of %d", len(absent)-len(got), len(absent))
	}
}

// TestARouteThatCannotOpenIsAnAbsenceAndNotAPanic pins the failure mode. A
// machine with nothing installed has to produce a report, not a crash: that
// is what makes the degradation the loud kind rather than the kind that takes
// the run with it.
func TestARouteThatCannotOpenIsAnAbsenceAndNotAPanic(t *testing.T) {
	t.Parallel()
	_, err := LocalReach{}.open(context.Background(), Shell{
		Name:   "nowhere",
		Lookup: []string{"/nonexistent/oracle/no-such-shell"},
	})
	if err == nil {
		t.Fatal("a shell at a path that does not exist opened anyway")
	}
	if !strings.Contains(err.Error(), "/nonexistent/oracle/no-such-shell") {
		t.Errorf("the reason does not say what was looked for: %v", err)
	}
}

// TestAFoundDoesNotCarryItsRouteAcrossTheWire is the invariant that makes the
// container column one implementation of Exec rather than two.
//
// The runner inside the image is handed the very Found the harness holds, so
// that Argv0, Args and SelfName cannot be two different ideas of what the
// column is. On the far side the shell *is* a local binary, so neither the
// route nor the open session may travel: a Found that still claimed a
// container would ask the runner to start one inside itself.
func TestAFoundDoesNotCarryItsRouteAcrossTheWire(t *testing.T) {
	t.Parallel()
	sh := Found{
		Shell: Shell{
			Name:     "ash",
			Lookup:   []string{"/bin/ash"},
			Argv0:    "sh",
			SelfName: "ash",
			Via:      &ContainerReach{Image: "alpine", Digest: "sha256:" + strings.Repeat("0", 64)},
		},
		Path:    "/bin/ash",
		Version: "BusyBox v1.37.0",
		sess:    &session{},
	}
	sh.sess = nil // as roundTrip does, and for the same reason
	b, err := json.Marshal(Request{Shell: sh, Case: Case{ID: "x", Snippet: "echo hi"}})
	if err != nil {
		t.Fatalf("a request would not serialize: %v", err)
	}
	var back Request
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("a request would not come back: %v", err)
	}
	if back.Shell.Via != nil {
		t.Error("the route crossed the wire, so the runner would try to reach the container from inside it")
	}
	if back.Shell.sess != nil {
		t.Error("the session crossed the wire")
	}
	for _, tc := range []struct{ name, got, want string }{
		{"Argv0", back.Shell.Argv0, sh.Argv0},
		{"SelfName", back.Shell.SelfName, sh.SelfName},
		{"Path", back.Shell.Path, sh.Path},
		{"Name", back.Shell.Name, sh.Name},
	} {
		if tc.got != tc.want {
			t.Errorf("%s did not survive: %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if back.Case.Snippet != "echo hi" {
		t.Errorf("the case did not survive: %q", back.Case.Snippet)
	}
}

// TestTheAshColumnAgreesWithWhatWasRecorded is the one test here that needs
// the container, and it is written so that the machine that cannot run it
// still checks something: the degradation itself.
//
// A skip that asserts nothing is the inert gate this repository already has a
// scar from, so the else branch is not a courtesy — where ash cannot be
// reached, the thing under test is that the harness said so and said why.
func TestTheAshColumnAgreesWithWhatWasRecorded(t *testing.T) {
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatalf("golden record: %v", err)
	}
	found, absent := Resolve(context.Background())
	var ash Found
	for _, f := range found {
		if f.Name == "ash" {
			ash = f
		}
	}
	if ash.Path == "" {
		for _, a := range absent {
			if a.Name != "ash" {
				continue
			}
			if a.Reason == "" {
				t.Fatal("ash is absent and will not say why")
			}
			t.Skipf("ash is not reachable here, which the harness reports as %q — "+
				"that half is what this machine can check", a.Reason)
		}
		t.Fatal("ash is neither reachable nor reported absent, which is the one answer that cannot be acted on")
	}
	if !strings.Contains(strings.ToLower(ash.Version), "busybox") {
		t.Fatalf("the ash column reports %q, which is not BusyBox", ash.Version)
	}
	// A handful rather than the corpus: the whole column is what `make
	// oracle-check` runs, and what this asks is whether the route is wired to
	// the same normalization the record was made with.
	checked := 0
	for _, c := range Corpus {
		if c.ReferenceRaces || checked == 25 {
			continue
		}
		want, ok := golden.Results[c.ID]["ash"]
		if !ok {
			continue
		}
		if got := Exec(context.Background(), ash, c); got != want {
			t.Errorf("%s: the container column answered %s, recorded %s", c.ID, describe(got), describe(want))
		}
		checked++
	}
	if checked == 0 {
		t.Error("no recorded ash cell was re-run, so this proves nothing")
	}
}

// TestTheRunnerIsBuiltFromThisTree guards the claim that makes the two routes
// one implementation: the binary that runs inside the image is compiled here,
// so it cannot be a stale corpus or an older normalizer measured under this
// commit's heading.
func TestTheRunnerIsBuiltFromThisTree(t *testing.T) {
	t.Parallel()
	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("the module root is not findable from a package directory: %v", err)
	}
	for _, name := range []string{"go.mod", filepath.Join("internal", "cmd", "oraclerunner", "main.go")} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("%s is not where the runner build expects it: %v", name, err)
		}
	}
}
