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
	left := regexp.MustCompile(`\{\{[a-z]+\}\}`)
	// Both shapes, since a placeholder only one of them fills in is the same
	// silent failure one step further along.
	for _, shape := range Shapes {
		f, err := newFixture(t.TempDir(), 1, shape)
		if err != nil {
			t.Fatal(err)
		}
		for _, rt := range Routes() {
			if m := left.FindString(rt.script(f)); m != "" {
				t.Errorf("route %s under %s: %s is never substituted, so the script names a path that is not there",
					rt.Name, shape, m)
			}
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
	// Under both shapes. What the carved-out one adds is denies, and a grant
	// arriving with them would be the same hole in every row at once.
	for _, shape := range Shapes {
		f, err := newFixture(t.TempDir(), 1, shape)
		if err != nil {
			t.Fatal(err)
		}
		text := readPolicy(t, f, Denied)
		if !strings.Contains(text, "default deny") {
			t.Errorf("%s: policy = %q, want it to start from a refusal", shape, text)
		}
		if !strings.Contains(text, "allow path "+f.Ws+"/**") {
			t.Errorf("%s: policy = %q, want the workspace granted", shape, text)
		}
		if n := strings.Count(text, "allow"); n != 1 {
			t.Errorf("%s: policy = %q has %d allow rules, want 1", shape, text, n)
		}
	}
}

// The carve-out is the whole of #2055, and it is two lines rather than one.
// `<region>/**` covers what is under the region and not the region itself, so
// a glob that reads the directory would fall through to the allow beside it
// and enumerate what the policy meant to hide — an escape written into the
// instrument rather than found by it.
func TestTheCarvedPolicyDeniesTheRegionAndEverythingUnderIt(t *testing.T) {
	t.Parallel()
	f, err := newFixture(t.TempDir(), 1, Carved)
	if err != nil {
		t.Fatal(err)
	}
	text := readPolicy(t, f, Denied)
	for _, want := range []string{"deny path " + f.Denied + "\n", "deny path " + f.Denied + "/**\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("policy = %q, want it to carry %q", text, want)
		}
	}
	// And the outside shape must not carry them: its whole claim is that the
	// refusal comes from there being no rule, so a deny would grade the other
	// half of Policy.Allow under both names.
	outside, err := newFixture(t.TempDir(), 2, Outside)
	if err != nil {
		t.Fatal(err)
	}
	if got := readPolicy(t, outside, Denied); strings.Contains(got, "deny path "+outside.Denied+"\n") {
		t.Errorf("the outside shape denies its own region by rule: %q", got)
	}
}

// The two shapes have to aim at different places, or the second one is three
// more runs of the first and the table says "graded twice" about one question.
func TestTheCarvedShapeAimsInsideTheWorkspaceAndTheOtherDoesNot(t *testing.T) {
	t.Parallel()
	inside, err := newFixture(t.TempDir(), 1, Carved)
	if err != nil {
		t.Fatal(err)
	}
	outside, err := newFixture(t.TempDir(), 2, Outside)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(inside.Denied, inside.Ws+string(filepath.Separator)) {
		t.Errorf("the carved region %s is not inside the workspace %s", inside.Denied, inside.Ws)
	}
	if strings.HasPrefix(outside.Denied, outside.Ws+string(filepath.Separator)) {
		t.Errorf("the outside region %s is inside the workspace %s", outside.Denied, outside.Ws)
	}
	// Every path a route aims at moves with the region, or the shape changes
	// the policy and not the question.
	for _, f := range []Fixture{inside, outside} {
		for _, path := range []string{f.Secret, f.Target, f.Victim, f.VictimDir, f.Sock} {
			if !strings.HasPrefix(path, f.Denied+string(filepath.Separator)) {
				t.Errorf("%s: %s is not in the denied region %s", f.Shape, path, f.Denied)
			}
		}
		// And the fixture really built it, in both shapes — a route aiming at
		// a directory that is not there grades inert, which is not a pass and
		// is also not the measurement.
		if _, err := os.Lstat(f.Secret); err != nil {
			t.Errorf("%s: %v", f.Shape, err)
		}
		// The link is the one entry that lives in the workspace and points
		// into the region, which is what makes it a test of resolution rather
		// than of the pattern.
		to, err := os.Readlink(f.Link)
		if err != nil || to != f.Denied {
			t.Errorf("%s: link points at %q (%v), want %s", f.Shape, to, err, f.Denied)
		}
	}
}

// A relative name is its own route: the shell resolves it and the gate sees
// whatever comes out. Hardcoding `..` would have left the carved-out shape
// with no relative row at all — a row silently graded twice against the same
// place.
func TestTheRelativeNameFollowsTheShape(t *testing.T) {
	t.Parallel()
	for _, shape := range Shapes {
		f, err := newFixture(t.TempDir(), 1, shape)
		if err != nil {
			t.Fatal(err)
		}
		// Resolved from the workspace a script runs in, it has to name the
		// denied region under either shape.
		got, err := filepath.EvalSymlinks(filepath.Join(f.Ws, f.relative()))
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.EvalSymlinks(f.Denied)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: %q from the workspace is %s, want the denied region %s",
				shape, f.relative(), got, want)
		}
	}
}

func readPolicy(t *testing.T, f Fixture, mode Mode) string {
	t.Helper()
	at, err := policy(f, mode)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(at)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
	one, err := newFixture(root, 1, Outside)
	if err != nil {
		t.Fatal(err)
	}
	two, err := newFixture(root, 2, Outside)
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
	// Both shapes, because the carved-out one puts the socket two components
	// deeper and the budget is spent on the scratch root either way.
	for _, shape := range Shapes {
		root := t.TempDir()
		f, err := newFixture(root, 9999, shape)
		if err != nil {
			t.Fatal(err)
		}
		const budget = 104
		// What the fixture adds beyond the root the caller chose.
		added := len(f.Sock) - len(root)
		if added > 16 {
			t.Errorf("%s: the fixture adds %d bytes to the socket path (%s), which leaves %d "+
				"for the scratch root out of %d", shape, added, f.Sock, budget-added, budget)
		}
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
