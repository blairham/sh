// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// The front end lists directories too, and until #951 it listed them through
// the os package.
//
// A listing is not an open of a file and it is not a probe: it enumerates,
// which is why interp gates it as its own kind and why a glob descending into
// a denied directory has always found nothing there. What was missing out here
// is the same call for the same reason — completion reads a directory on a
// path a *person typed*, which is the definition this package opens with of
// what is inside the boundary.
//
// Every fixture below is built under t.TempDir(). None points into a home.

// listing is a Boundary with a gate and a sink whose answers a test can read.
func listing(t *testing.T, deny func(string) bool) (boundary.Boundary, *[]interp.Action, *[]interp.Event) {
	t.Helper()
	asked := &[]interp.Action{}
	got := &[]interp.Event{}
	return boundary.Boundary{
		Session: "SESSIONUNDERTEST",
		Gate: interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
			*asked = append(*asked, a)
			if deny != nil && deny(a.Path) {
				return interp.Deny
			}
			return interp.Allow
		}),
		Events: interp.SinkFunc(func(_ context.Context, e interp.Event) { *got = append(*got, e) }),
	}, asked, got
}

// physical is a temporary directory under the spelling the kernel will answer
// with, which on macOS is not the one t.TempDir hands out: $TMPDIR is under
// /var, and /var is one of the three links the operating system itself
// installs into /private. A fixture built under the logical spelling would
// make every open here look as though it had resolved somewhere else, and the
// tests below are about a link the *test* made.
//
// internal/policy knows about those three and rewrites patterns across them,
// which is why the repl tests can use the name t.TempDir gave them; a gate
// written as a Go function here knows nothing, so the fixture is physical.
func physical(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// names is what a listing came to, as a comparable string.
func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestAListingThePolicyRefusesYieldsNothingAndSaysSo.
//
// The refusal is ErrRefused rather than a listing of the entries the gate did
// not object to, because there is no such thing: a directory is enumerated or
// it is not, and half a listing is a leak with extra steps.
func TestAListingThePolicyRefusesYieldsNothingAndSaysSo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	b, asked, got := listing(t, func(string) bool { return true })

	entries, err := b.ReadDir(t.Context(), dir)
	if !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("a denied listing answered %v, want a refusal", err)
	}
	if len(entries) != 0 {
		t.Errorf("a denied listing produced %v", names(entries))
	}
	if len(*asked) != 1 || (*asked)[0].Kind != interp.ActionReadDir || (*asked)[0].Path != dir {
		t.Fatalf("the gate was asked %+v, want one read-dir about %s", *asked, dir)
	}
	if len(*got) != 1 || (*got)[0].Kind != interp.EventDenied {
		t.Fatalf("recorded %+v, want one denial", *got)
	}
	if (*got)[0].Session != "SESSIONUNDERTEST" || (*got)[0].Action.ID == "" {
		t.Errorf("the denial is %+v, want a session and an id on it", (*got)[0])
	}
	if (*got)[0].Action.ID != (*asked)[0].ID {
		t.Errorf("the record names action %q and the gate was asked about %q",
			(*got)[0].Action.ID, (*asked)[0].ID)
	}
}

// And an allowed one is the whole directory, sorted, recorded once.
func TestAnAllowedListingIsTheWholeDirectoryInNameOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Written in an order that is not the answer, so a sort that was dropped
	// shows up as a different slice rather than as luck.
	for _, n := range []string{"zeta", "alpha", "mu"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b, _, got := listing(t, nil)

	entries, err := b.ReadDir(t.Context(), dir)
	if err != nil {
		t.Fatalf("the listing was refused: %v", err)
	}
	want := []string{"alpha", "mu", "zeta"}
	if g := names(entries); len(g) != 3 || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] {
		t.Errorf("listed %v, want %v in name order", g, want)
	}
	if len(*got) != 1 || (*got)[0].Kind != interp.EventAccess ||
		(*got)[0].Action.Kind != interp.ActionReadDir || (*got)[0].Action.Path != dir {
		t.Fatalf("recorded %+v, want one read-dir access naming %s", *got, dir)
	}
	if (*got)[0].Action.Resolved != "" {
		t.Errorf("the record says the name resolved to %q, and it is its own name",
			(*got)[0].Action.Resolved)
	}
}

// TestAListingIsCheckedOnWhatTheNameReached is the half a name-matching gate
// cannot do for itself, and it is the one a person at a prompt can aim: the
// word typed is `link/`, and the directory the kernel hands back is the one
// the policy hides.
func TestAListingIsCheckedOnWhatTheNameReached(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := physical(t, t.TempDir())
	hidden := filepath.Join(dir, "hidden")
	if err := os.Mkdir(hidden, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "secret"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent")
	if err := os.Symlink(hidden, link); err != nil {
		t.Fatal(err)
	}
	b, asked, got := listing(t, func(p string) bool { return p == hidden })

	entries, err := b.ReadDir(t.Context(), link)
	if !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("listing through a link into a denied directory answered %v, want a refusal", err)
	}
	if len(entries) != 0 {
		t.Errorf("it produced %v", names(entries))
	}
	// Two consultations, the second about the object rather than the name —
	// which is what makes the rule about the object and not about the spelling.
	if len(*asked) != 2 {
		t.Fatalf("the gate was consulted %d times, want the name and then the object", len(*asked))
	}
	if (*asked)[0].Path != link {
		t.Errorf("the first consultation was about %q, want the name as typed", (*asked)[0].Path)
	}
	if (*asked)[1].Path != hidden {
		t.Errorf("the second consultation was about %q, want %q", (*asked)[1].Path, hidden)
	}
	if (*asked)[1].ID != (*asked)[0].ID {
		t.Error("the two consultations carry different ids, and they are one access")
	}
	// And the record carries both names, which is #943's rule kept here.
	if len(*got) != 1 || (*got)[0].Kind != interp.EventDenied {
		t.Fatalf("recorded %+v, want one denial", *got)
	}
	if (*got)[0].Action.Path != link || (*got)[0].Action.Resolved != hidden {
		t.Errorf("the denial says path %q resolved %q, want %q and %q",
			(*got)[0].Action.Path, (*got)[0].Action.Resolved, link, hidden)
	}
}

// TestARefusedListingSaysTheSameThingWhicheverWayItWasRefused.
//
// The two refusals above are one error on purpose, and this is the assertion
// that keeps them one: a caller that could tell "the name is denied" from "the
// name reached a denied object" has been told where the name went, which is
// exactly what #943 says a refusal must not do.
func TestARefusedListingSaysTheSameThingWhicheverWayItWasRefused(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := physical(t, t.TempDir())
	hidden := filepath.Join(dir, "hidden")
	plain := filepath.Join(dir, "plain")
	for _, d := range []string{hidden, plain} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "innocent")
	if err := os.Symlink(hidden, link); err != nil {
		t.Fatal(err)
	}
	b, _, _ := listing(t, func(p string) bool { return p == hidden || p == plain })

	_, byName := b.ReadDir(t.Context(), plain)
	_, byObject := b.ReadDir(t.Context(), link)
	if byName == nil || byObject == nil {
		t.Fatalf("one of the two was allowed: by name %v, by object %v", byName, byObject)
	}
	if byName.Error() != byObject.Error() {
		t.Errorf("a refusal by name reads %q and a refusal by object reads %q; "+
			"the difference tells the caller where the name went", byName, byObject)
	}
}

// TestAListingThatFailsIsStillRecorded, for the reason a front-end open that
// fails is: out here a directory that is not there is the ordinary case — a
// person half-way through typing a name — and the auditable act is the attempt.
func TestAListingThatFailsIsStillRecorded(t *testing.T) {
	t.Parallel()
	b, _, got := listing(t, nil)
	missing := filepath.Join(t.TempDir(), "no-such-directory")

	if _, err := b.ReadDir(t.Context(), missing); err == nil {
		t.Fatal("listing a directory that is not there succeeded")
	} else if errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("it answered a refusal: %v", err)
	}
	if len(*got) != 1 || (*got)[0].Kind != interp.EventAccess || (*got)[0].Action.Path != missing {
		t.Errorf("recorded %+v, want the attempt", *got)
	}
}

// TestAnUnwatchedListingIsTheCallItAlwaysWas. A shell with no policy pays two
// nil checks and makes os.ReadDir, which is what keeps this off the price of
// every Tab in every unsandboxed session.
func TestAnUnwatchedListingIsTheCallItAlwaysWas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var b boundary.Boundary
	entries, err := b.ReadDir(t.Context(), dir)
	if err != nil {
		t.Fatalf("the zero Boundary refused a listing: %v", err)
	}
	if g := names(entries); len(g) != 1 || g[0] != "one" {
		t.Errorf("listed %v, want [one]", g)
	}
}

// And a Boundary that only watches records without refusing.
func TestAWatchedButUngatedListingIsRecordedAndAllowed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var got []interp.Event
	b := boundary.Boundary{
		Events: interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) }),
	}
	entries, err := b.ReadDir(t.Context(), dir)
	if err != nil {
		t.Fatalf("a sink-only Boundary refused a listing: %v", err)
	}
	if g := names(entries); len(g) != 1 || g[0] != "one" {
		t.Errorf("listed %v, want [one]", g)
	}
	if len(got) != 1 || got[0].Kind != interp.EventAccess || got[0].Action.ID == "" {
		t.Errorf("recorded %+v, want one numbered access", got)
	}
}

// TestNoGateInThisTreeStopsAPersonMidTab is the prompting half of #951,
// asserted rather than argued.
//
// The objection was that a gate which asks a person something would ask it in
// the middle of a keystroke-driven completion, where there is no good answer
// and no good place to put the question. It does not arise, and the reason is
// not new: internal/acp holds the only gate in this tree that asks anybody
// anything, and its Escalates already answers false for every read — a stat, a
// non-writing open, and a directory listing — because a prompt arriving at the
// rate a PATH search or a glob generates them is one people click through.
//
// A listing raised by completion is the same ActionReadDir a glob raises, so
// there is nothing here for an escalation rule to have to learn about. An
// embedder whose own gate chose to prompt on a listing would already be
// prompting once per directory of every glob a script writes; Tab is not a new
// hazard for it, and giving completion an action of its own so that such a
// gate could tell the two apart would be inventing a distinction nothing in
// the tree has asked for.
func TestNoGateInThisTreeStopsAPersonMidTab(t *testing.T) {
	t.Parallel()
	if acp.Escalates(interp.Action{Kind: interp.ActionReadDir, Path: "/srv"}) {
		t.Error("the one prompting gate in this tree would ask a person about a directory listing, " +
			"which is what completion now raises on every Tab")
	}
	// Stated as the pair it belongs to, so that a change to either is a change
	// to a test rather than to nothing: a listing is a read, and reads are
	// recorded and never asked about.
	if acp.Escalates(interp.Action{Kind: interp.ActionOpen, Path: "/srv/x"}) {
		t.Error("a read would be escalated, and completion's listing is a read")
	}
}
