// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package sandboxcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	if got := verdictOf([3]bool{false, false, false}, false); got != Inert {
		t.Errorf("verdict = %v, want %v: a route that never worked cannot be a pass", got, Inert)
	}
	// And it stays inert even when the denied and allowed runs would have
	// read well on their own.
	if got := verdictOf([3]bool{false, false, true}, false); got != Inert {
		t.Errorf("verdict = %v, want %v", got, Inert)
	}
}

func TestARouteThePolicyDidNotStopIsAnEscape(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, true, true}, false); got != Escaped {
		t.Errorf("verdict = %v, want %v", got, Escaped)
	}
}

// The other direction, and it is a defect rather than a curiosity: a gate
// that cannot be opened where it was told to is the failure people turn
// sandboxing off over.
func TestARoutePermittedAndStillRefusedIsOverblocked(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, false}, false); got != Overblocked {
		t.Errorf("verdict = %v, want %v", got, Overblocked)
	}
}

func TestContainedNeedsAllThreeRunsToAgree(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, true}, false); got != Contained {
		t.Errorf("verdict = %v, want %v", got, Contained)
	}
}

// Only an escape or an overblocked row is a failure. An inert one is a fact
// about what the shell implements, and failing on it would mean the sweep
// went red for a feature nobody has written yet. A documented one is a fact
// about what an OS-less boundary can claim, and failing on it would mean the
// sweep went red permanently — which is the state a reader stops reading.
func TestOnlyABrokenBoundaryFailsTheSweep(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		v    Verdict
		fail bool
	}{{Contained, false}, {Inert, false}, {Escaped, true}, {Overblocked, true}, {Documented, false}} {
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

func readPolicy(t *testing.T, f Fixture, mode Mode, grant ...string) string {
	t.Helper()
	at, err := policy(f, mode, grant)
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
		at, err := policy(f, mode, nil)
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
	at, err := policy(Fixture{Root: t.TempDir()}, Ungated, nil)
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

// The DOCUMENTED verdict is the one addition here that could be used to make
// a red row go away, so the tests below are about the ways it must not.
//
// It softens exactly one clause of verdictOf, and the point of that placement
// is that the row keeps reporting on itself: it goes green on its own the day
// the gate contains the process tree, and it goes inert out loud the day the
// route stops working. Both of those would be lost by a verdict that simply
// meant "ignore this row".

// The day a Gate implementation contains the process tree, the denied run
// stops leaking and this has to become an ordinary pass — with nobody
// remembering to come back and edit the table. That is half of why the hole
// is a row rather than a paragraph.
func TestADocumentedRouteThatStopsEscapingBecomesContainedOnItsOwn(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, true}, true); got != Contained {
		t.Errorf("verdict = %v, want %v: a documented row must go green by itself once the gate holds it", got, Contained)
	}
}

// And the other half. A documented hole that quietly stops being measured is
// worse than an undocumented one, because the ledger goes on printing a claim
// nothing is checking. The inert clause comes first for exactly this.
func TestADocumentedRouteThatStopsWorkingIsInertRatherThanDocumented(t *testing.T) {
	t.Parallel()
	for _, did := range [][3]bool{{false, false, false}, {false, true, true}} {
		if got := verdictOf(did, true); got != Inert {
			t.Errorf("verdictOf(%v, documented) = %v, want %v: a route that does nothing ungated is measuring nothing, documented or not", did, got, Inert)
		}
	}
}

// Overblocked is a defect wherever it turns up. A documented row whose
// *permitting* policy refuses it is a gate that cannot be opened where it was
// told to, which the citation says nothing about.
func TestADocumentedRouteIsStillOverblockedWhenAPermittingPolicyRefusesIt(t *testing.T) {
	t.Parallel()
	if got := verdictOf([3]bool{true, false, false}, true); got != Overblocked {
		t.Errorf("verdict = %v, want %v", got, Overblocked)
	}
}

// The verdict has to change something. If `documented` were ignored the two
// tests above would still pass — every one of their cases is a clause the
// flag does not reach — and the row would be graded ESCAPED while reading as
// though it had been accounted for.
func TestTheDocumentedFlagIsWhatSeparatesThisFromAnEscape(t *testing.T) {
	t.Parallel()
	const escaping = true
	if got := verdictOf([3]bool{true, escaping, true}, false); got != Escaped {
		t.Errorf("undocumented: verdict = %v, want %v", got, Escaped)
	}
	if got := verdictOf([3]bool{true, escaping, true}, true); got != Documented {
		t.Errorf("documented: verdict = %v, want %v", got, Documented)
	}
}

// A citation nobody opens is prose, and prose is what this row exists to stop
// the hole being. The section has to still be in the file it names.
func TestTheDocumentedRowCitesSomethingThatIsActuallyThere(t *testing.T) {
	t.Parallel()
	file, section, ok := strings.Cut(DesignDocSection, " § ")
	if !ok {
		t.Fatalf("DesignDocSection = %q, want `<file> § <section heading>`", DesignDocSection)
	}
	// From internal/sandboxcheck up to the repository root.
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
	if err != nil {
		t.Fatalf("the documented row cites %s, which cannot be read: %v", file, err)
	}
	if !strings.Contains(string(b), "## "+section) {
		t.Errorf("%s has no section %q: the citation has rotted, and a rotted citation is why this is a heading and not a line number", file, section)
	}
	// And the section has to be the one that accounts for the escape, not
	// merely a heading that still exists. This is the sentence the row is
	// standing on, and #4411 is the grammar half of it.
	for _, claim := range []string{"contains the shell, not the process tree", "exec-unconfined"} {
		if !strings.Contains(string(b), claim) {
			t.Errorf("%s no longer says %q, so the documented row is citing a reason that is gone", file, claim)
		}
	}
}

// Every route that carries a citation is claiming its escape is the operating
// system's to close, and every route that does not is claiming the opposite.
// A row that escapes without a citation must stay ESCAPED and red.
func TestOnlyRoutesThatMeanToEscapeCarryACitation(t *testing.T) {
	t.Parallel()
	var cited []string
	for _, rt := range Routes() {
		if rt.Cites != "" {
			cited = append(cited, rt.Name)
			if rt.Cites != DesignDocSection {
				t.Errorf("route %s cites %q, which is not the section this package checks exists", rt.Name, rt.Cites)
			}
		}
	}
	// Named rather than counted, so that adding a second documented hole is
	// a deliberate edit to this list with a reason in the commit, rather
	// than a number going up.
	want := []string{"exec/child-reads-denied"}
	if !slices.Equal(cited, want) {
		t.Errorf("documented routes = %v, want %v: a new one needs the argument on the package comment made for it too", cited, want)
	}
}

// Grant opens the denied policy, which is the one thing in this package that
// every verdict turns on. Two limits, both narrow on purpose: only a route
// that is admitting an escape may use it, and it may only grant the exec
// slot — so the path rules a route aims past are never what it loosened.
func TestOnlyADocumentedRouteMayOpenTheDeniedPolicy(t *testing.T) {
	t.Parallel()
	for _, rt := range Routes() {
		if len(rt.Grant) == 0 {
			continue
		}
		if rt.Cites == "" {
			t.Errorf("route %s opens the denied policy with %v but does not cite a reason: a grant that is not admitting a documented hole is an instrument grading itself",
				rt.Name, rt.Grant)
		}
		for _, rule := range rt.Grant {
			if !strings.HasPrefix(rule, "allow exec-unconfined ") {
				t.Errorf("route %s grants %q; only `allow exec-unconfined` may be granted, or the row could be passing on a path rule it was supposed to be refused by",
					rt.Name, rule)
			}
		}
	}
}

// And what that grant does to the file, since the base policy test asserts a
// single allow and would not notice a second one arriving for one route.
func TestAGrantAddsTheExecRuleAndLeavesTheDeniesAlone(t *testing.T) {
	t.Parallel()
	for _, shape := range Shapes {
		f, err := newFixture(t.TempDir(), 1, shape)
		if err != nil {
			t.Fatal(err)
		}
		base := readPolicy(t, f, Denied)
		granted := readPolicy(t, f, Denied, "allow exec-unconfined /bin/cat")
		// Every line of the ungranted policy survives, in order. The grant
		// is an addition and must not be able to be a replacement.
		if !strings.HasPrefix(granted, base) {
			t.Errorf("%s: granted policy\n%s\ndoes not start with the ungranted one\n%s", shape, granted, base)
		}
		if got := strings.TrimPrefix(granted, base); got != "allow exec-unconfined /bin/cat\n" {
			t.Errorf("%s: the grant added %q, want the one exec rule", shape, got)
		}
		// The denies the route is graded against are still there.
		if strings.Count(granted, "deny path ") != strings.Count(base, "deny path ") {
			t.Errorf("%s: the grant changed the denies:\n%s", shape, granted)
		}
	}
}

// The grant reaches the shell that runs the route, not merely the file. A
// route whose script never sees it would grade contained for the wrong
// reason — the exec refused rather than the deny holding — which is the
// false calm this whole package is shaped around.
func TestTheGrantOnARouteReachesThePolicyThatRouteIsRunUnder(t *testing.T) {
	t.Parallel()
	var documented Route
	for _, rt := range Routes() {
		if rt.Cites != "" {
			documented = rt
		}
	}
	if len(documented.Grant) == 0 {
		t.Fatal("the documented route carries no grant, so this test is measuring nothing")
	}
	f, err := newFixture(t.TempDir(), 1, Outside)
	if err != nil {
		t.Fatal(err)
	}
	at, err := policy(f, Denied, documented.Grant)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(at)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range documented.Grant {
		if !strings.Contains(string(b), rule) {
			t.Errorf("the denied policy the route runs under is %q, which does not carry %q", b, rule)
		}
	}
	// The program the script actually names has to be the program the grant
	// names, or the grant is exact and irrelevant and the row grades on a
	// refused exec.
	for _, rule := range documented.Grant {
		prog := strings.TrimPrefix(rule, "allow exec-unconfined ")
		if !strings.Contains(documented.Script, prog) {
			t.Errorf("route %s grants %s but its script %q never runs it", documented.Name, prog, documented.Script)
		}
	}
}

// The ledger is the whole visible output of the verdict. A documented row
// that printed like any other escape, or that printed without saying where
// the reason is written, would leave the reader exactly where the paragraph
// left them.
func TestTheReportLaysOutTheDocumentedRowsAndSaysWhereTheReasonIs(t *testing.T) {
	t.Parallel()
	rep := Report{Shell: "test", Results: []Result{{
		Route: "exec/child-reads-denied", Dialect: "bash", Shape: Outside,
		Verdict: Documented, Cites: DesignDocSection,
	}}}
	text := rep.Text(false)
	for _, want := range []string{"exec/child-reads-denied", DesignDocSection, "documented 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("report does not mention %q:\n%s", want, text)
		}
	}
	// And it is not reported as a failure, which is the whole reason it is
	// not spelled ESCAPED.
	if rep.Failed() {
		t.Error("a documented row failed the sweep")
	}
}
