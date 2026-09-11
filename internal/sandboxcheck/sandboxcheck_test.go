// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package sandboxcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The instrument's own machinery, which is the half `make sandbox` cannot
// check: the target runs real binaries and reports on them, so a fault in
// the grader looks exactly like a fact about the shell.
//
// Everything here is about the ways this could report a green table for a
// boundary that is not there.

// The rule the whole design rests on. A route that did nothing without a
// policy proves nothing with one, and calling that "contained" is the
// specific lie this instrument exists to avoid.
func TestARouteThatDoesNothingUngatedIsInertRatherThanContained(t *testing.T) {
	t.Parallel()
	// Nothing happened in any of the three runs — which is what a shell
	// that failed to start looks like, and what sixteen "refused" rows once
	// looked like.
	if got := verdictOf([3]bool{false, false, false}); got != Inert {
		t.Errorf("verdict = %v, want %v: a route that never worked cannot be a pass", got, Inert)
	}
	// And it stays inert even when the denied and allowed runs would have
	// read well on their own.
	if got := verdictOf([3]bool{false, false, true}); got != Inert {
		t.Errorf("verdict = %v, want %v", got, Inert)
	}
}

func TestARouteThePolicyDidNotStopIsAnEscape(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, true, true}); got != Escaped {
		t.Errorf("verdict = %v, want %v", got, Escaped)
	}
}

// The other direction, and it is a defect rather than a curiosity: a gate
// that cannot be opened where it was told to is the failure people turn
// sandboxing off over.
func TestARoutePermittedAndStillRefusedIsOverblocked(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, false}); got != Overblocked {
		t.Errorf("verdict = %v, want %v", got, Overblocked)
	}
}

func TestContainedNeedsAllThreeRunsToAgree(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, true}); got != Contained {
		t.Errorf("verdict = %v, want %v", got, Contained)
	}
}

// Only an escape or an overblocked row is a failure. An inert one is a fact
// about what the shell implements, and failing on it would mean the sweep
// went red for a feature nobody has written yet.
func TestOnlyABrokenBoundaryFailsTheSweep(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		v    Verdict
		fail bool
	}{{Contained, false}, {Inert, false}, {Escaped, true}, {Overblocked, true}} {
		rep := Report{Results: []Result{{Verdict: c.v}}}
		if got := rep.Failed(); got != c.fail {
			t.Errorf("%v: Failed() = %v, want %v", c.v, got, c.fail)
		}
	}
}

// A typo in a placeholder is the quiet way a route stops testing anything:
// `{{secrit}}` is substituted by nothing, the script names a path that is
// not there, the route grades inert, and the row looks like an unimplemented
// feature rather than a broken test.
func TestNoRouteCarriesAPlaceholderNothingFillsIn(t *testing.T) {
	t.Parallel()
	f := Fixture{
		Root: "/root", Ws: "/root/ws", Secret: "/root/secret",
		Target: "/root/target", Victim: "/root/victim",
		VictimDir: "/root/victimdir", Sock: "/root/s",
	}
	left := regexp.MustCompile(`\{\{[a-z]+\}\}`)
	for _, rt := range Routes() {
		if m := left.FindString(rt.script(f)); m != "" {
			t.Errorf("route %s: %s is never substituted, so the script names a path that is not there",
				rt.Name, m)
		}
	}
}

// Rows are keyed by name when the table is printed, so two routes sharing
// one would print as a single row and the second would be invisible.
func TestEveryRouteHasItsOwnName(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, rt := range Routes() {
		if seen[rt.Name] {
			t.Errorf("two routes are called %s: one of them will not be printed", rt.Name)
		}
		seen[rt.Name] = true
	}
}

// Every route has to be able to say whether it worked, and a nil Did would
// panic rather than report — in the middle of a sweep, after minutes.
func TestEveryRouteCanSayWhetherItWorked(t *testing.T) {
	t.Parallel()
	for _, rt := range Routes() {
		if rt.Did == nil {
			t.Errorf("route %s has no Did, so nothing decides its verdict", rt.Name)
		}
		if rt.Script == "" {
			t.Errorf("route %s has no script", rt.Name)
		}
		for _, d := range rt.dialects() {
			if !strings.Contains(strings.Join(AllDialects, " "), d) {
				t.Errorf("route %s names dialect %q, which -dialect does not take", rt.Name, d)
			}
		}
	}
}

// The denied policy is the one every verdict turns on. If it granted more
// than the workspace, every row would read "contained" for the wrong reason
// — the route would have been refused by a rule nobody meant to write, or
// permitted by one nobody noticed.
func TestTheDeniedPolicyGrantsTheWorkspaceAndNothingElse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := Fixture{Root: dir, Ws: filepath.Join(dir, "ws")}
	at, err := policy(f, Denied)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(at)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "default deny") {
		t.Errorf("policy = %q, want it to start from a refusal", text)
	}
	if !strings.Contains(text, "allow path "+f.Ws+"/**") {
		t.Errorf("policy = %q, want the workspace granted", text)
	}
	// Exactly one grant. A second would be a hole in every row at once.
	if n := strings.Count(text, "allow"); n != 1 {
		t.Errorf("policy = %q has %d allow rules, want 1", text, n)
	}
}

// The policy file must not be written where a route can see it: one of them
// enumerates a directory and another writes into one, and a rule set that
// turned up in a listing — or could be overwritten by the script it governs
// — would be a finding about the instrument rather than the shell.
func TestThePolicyIsNotInsideTheWorkspaceItGoverns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := Fixture{Root: dir, Ws: filepath.Join(dir, "ws")}
	for _, mode := range []Mode{Denied, Allowed} {
		at, err := policy(f, mode)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(at, f.Ws) {
			t.Errorf("policy at %s is inside the workspace at %s", at, f.Ws)
		}
	}
}

// The ungated run takes no policy at all, rather than one that allows
// everything. A permissive policy still engages the gate, so a route it
// happened to refuse would grade inert and hide a real escape.
func TestTheUngatedRunHasNoPolicyAtAll(t *testing.T) {
	t.Parallel()
	at, err := policy(Fixture{Root: t.TempDir()}, Ungated)
	if err != nil {
		t.Fatal(err)
	}
	if at != "" {
		t.Errorf("policy = %q, want none: the first run must not engage the gate", at)
	}
}

// Each run gets its own directory, and the reason is that the ungated run
// succeeds by design — it removes the victim, or creates the target. Two
// runs sharing a fixture would have the second one measuring what the first
// one left, and a `rm` route would report "refused" for a file that was
// already gone.
func TestEachRunGetsAFixtureOfItsOwn(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	one, err := newFixture(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := newFixture(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if one.Root == two.Root {
		t.Fatalf("both runs got %s", one.Root)
	}
	// And the fixture is actually built, not merely named: a route that
	// tries to remove a victim which was never created grades inert.
	for _, path := range []string{one.Ws, one.Secret, one.Victim, one.VictimDir} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was not created: %v", path, err)
		}
	}
	// The target must *not* exist, or every creating route reports that it
	// worked before the shell has run.
	if _, err := os.Lstat(one.Target); err == nil {
		t.Errorf("%s exists before the run", one.Target)
	}
	if got := string(mustRead(t, one.Secret)); !strings.Contains(got, SecretMark) {
		t.Errorf("secret = %q, want it to carry %q", got, SecretMark)
	}
}

// sun_path is 104 bytes on Darwin, and the whole absolute path counts. The
// budget is spent on the scratch root, so the fixture's own contribution has
// to stay small — this is the check that keeps a plainly-named socket path
// from silently turning that row inert. See Fixture.Sock.
func TestTheSocketPathLeavesRoomForARealScratchRoot(t *testing.T) {
	t.Parallel()
	f, err := newFixture(t.TempDir(), 9999)
	if err != nil {
		t.Fatal(err)
	}
	const budget = 104
	// What the fixture adds beyond the root the caller chose.
	added := len(f.Sock) - len(t.TempDir())
	if added > 16 {
		t.Errorf("the fixture adds %d bytes to the socket path (%s), which leaves %d "+
			"for the scratch root out of %d", added, f.Sock, budget-added, budget)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
