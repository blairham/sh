// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// The bypass these tests are about: `ln -s secret/file work/innocent` and a
// front end that opens `work/innocent`. Until #942 the gate was asked about
// the name and the open was done on the object, so `HISTFILE=<link>` read and
// appended to whatever the link pointed at, and a policy that named the target
// said nothing about it.
//
// Every fixture is built under t.TempDir(). A symbolic link is an odd thing to
// leave behind in a checkout and a worse one to point at somebody's home, and
// one of the paths this package is about is the history file.

// gated is a Boundary carrying a real parsed policy and a sink that keeps
// what it was told.
type gated struct {
	boundary.Boundary
	events *[]interp.Event
	asked  *[]interp.Action
}

// policyGate parses rules rather than hand-writing a Gate.
//
// The parser is used on purpose. A selector or a pattern that does not mean
// what its author thought matches nothing, and a rule that matches nothing is
// indistinguishable from a rule being obeyed — so a hand-written gate can pass
// a test the real policy language would fail. #944 found that one the hard way.
func policyGate(t *testing.T, rules ...string) (gated, *policy.Policy) {
	t.Helper()
	p, err := policy.Parse(strings.NewReader("version 1\n" + strings.Join(rules, "\n") + "\n"))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	events := &[]interp.Event{}
	asked := &[]interp.Action{}
	g := recordingGate{p: p, asked: asked}
	return gated{
		Boundary: boundary.Boundary{
			Gate:    g,
			Events:  interp.SinkFunc(func(_ context.Context, e interp.Event) { *events = append(*events, e) }),
			Session: "SESSIONUNDERTEST",
		},
		events: events,
		asked:  asked,
	}, p
}

// recordingGate is the policy, plus a note of every consultation — which is
// how "asked twice" and "asked once" become assertions rather than beliefs.
type recordingGate struct {
	p     *policy.Policy
	asked *[]interp.Action
}

func (g recordingGate) Allow(ctx context.Context, a interp.Action) interp.Decision {
	*g.asked = append(*g.asked, a)
	return g.p.Allow(ctx, a)
}

// fixture builds `<dir>/secret/data` holding text and `<dir>/work/innocent`
// pointing at it, and answers with the link and the object's physical name.
//
// The physical name is taken with EvalSymlinks here, in the test, because the
// test needs an oracle for what the kernel will say. Nothing in the code under
// test resolves anything — that is the whole design — so the two arriving at
// the same answer is a claim worth making rather than a tautology.
func fixture(t *testing.T, text string) (dir, link, object string) {
	t.Helper()
	dir = t.TempDir()
	for _, sub := range []string{"secret", "work"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	object = filepath.Join(dir, "secret", "data")
	if err := os.WriteFile(object, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "work", "innocent")
	if err := os.Symlink(object, link); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(object)
	if err != nil {
		t.Fatal(err)
	}
	return dir, link, physical
}

// verifies reports whether this platform can be asked what an open reached.
// Elsewhere there is nothing to verify and the gate matches the name alone,
// which is what every platform did before #922.
func verifies() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "linux" }

// TestAFrontEndReadThroughALinkIsCheckedOnWhatItReached is the bypass itself,
// on the front end's side of the boundary.
func TestAFrontEndReadThroughALinkIsCheckedOnWhatItReached(t *testing.T) {
	if !verifies() {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir, link, object := fixture(t, "TOPSECRET")
	b, _ := policyGate(t, "default allow", fmt.Sprintf("deny read %s/secret/**", dir))

	// The name the front end was given is not denied by any rule, so the
	// first consultation allows it. Everything after that is the verification.
	body, err := b.ReadFile(t.Context(), link)
	if !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("reading through the link gave (%q, %v), want a refusal", body, err)
	}
	if strings.Contains(string(body), "TOPSECRET") {
		t.Fatal("the refused read handed back the file anyway")
	}

	// The gate was asked twice about one access: the name, then the object.
	// Same id both times, because it is one open whose name resolved
	// elsewhere and not two opens.
	if len(*b.asked) != 2 {
		t.Fatalf("the gate was consulted %d times, want the name and then the object", len(*b.asked))
	}
	first, second := (*b.asked)[0], (*b.asked)[1]
	if first.Path != link {
		t.Errorf("first consultation was about %q, want the name as written", first.Path)
	}
	if second.Path != object {
		t.Errorf("second consultation was about %q, want the object at %q", second.Path, object)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Errorf("consultations carry ids %q and %q, want one id for one access", first.ID, second.ID)
	}

	// And the record names where it went, which is the half of the split that
	// belongs to whoever wrote the policy.
	var denials []interp.Event
	for _, e := range *b.events {
		if e.Kind == interp.EventDenied {
			denials = append(denials, e)
		}
	}
	if len(denials) != 1 {
		t.Fatalf("recorded %d denials, want one", len(denials))
	}
	if denials[0].Action.Path != link {
		t.Errorf("the denial names %q, want the path as written", denials[0].Action.Path)
	}
	if denials[0].Action.Resolved != object {
		t.Errorf("the denial resolved to %q, want the object at %q",
			denials[0].Action.Resolved, object)
	}
	if denials[0].Session != "SESSIONUNDERTEST" {
		t.Errorf("the denial says session %q, want the run's", denials[0].Session)
	}
}

// TestAFrontEndWriteThroughALinkDoesNotEmptyTheFileFirst is the worse half.
//
// O_TRUNC destroys as part of the open, so a front end that opened and then
// refused would report a refusal over a file it had already emptied. The flag
// is held back past the verification, which is what this measures: the refusal
// happens *and* the file still has its bytes.
func TestAFrontEndWriteThroughALinkDoesNotEmptyTheFileFirst(t *testing.T) {
	if !verifies() {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir, link, object := fixture(t, "KEEP THIS\n")
	b, _ := policyGate(t, "default allow", fmt.Sprintf("deny write %s/secret/**", dir))

	err := b.WriteFile(t.Context(), boundary.File{Path: link, Perm: 0o600}, []byte("clobbered"))
	if !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("writing through the link gave %v, want a refusal", err)
	}
	body, readErr := os.ReadFile(object)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(body) != "KEEP THIS\n" {
		t.Errorf("the object holds %q, want it untouched by a refused write", body)
	}
}

// TestAnOrdinaryFrontEndOpenIsAskedOnce.
//
// The suppression this pins is what keeps a gate that prompts from asking a
// person twice for every file the shell opens — on a Mac the kernel answers
// with the /private spelling for everything under /tmp and /var, which is
// where a temporary directory is.
func TestAnOrdinaryFrontEndOpenIsAskedOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := policyGate(t, "default allow")

	body, err := b.ReadFile(t.Context(), path)
	if err != nil {
		t.Fatalf("reading an allowed file gave %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("read %q, want the file", body)
	}
	if len(*b.asked) != 1 {
		t.Errorf("the gate was consulted %d times for one ordinary open, want once: %+v",
			len(*b.asked), *b.asked)
	}
}

// TestAnAllowedFrontEndResolutionStillOpens. The verification refuses what the
// policy refuses and nothing else — a link into a place the policy permits is
// followed, and the bytes come back.
func TestAnAllowedFrontEndResolutionStillOpens(t *testing.T) {
	dir, link, _ := fixture(t, "PERMITTED")
	b, _ := policyGate(t, "default allow", fmt.Sprintf("deny read %s/nowhere/**", dir))

	body, err := b.ReadFile(t.Context(), link)
	if err != nil {
		t.Fatalf("reading through an allowed link gave %v", err)
	}
	if string(body) != "PERMITTED" {
		t.Errorf("read %q, want the target's contents", body)
	}
}

// TestAFrontEndWriteToSomethingWithNoLengthStillWorks is the measured guard on
// the held-back truncation, and it is the one that reads differently on the
// two platforms.
//
// On Linux `open("/dev/null", O_WRONLY|O_TRUNC)` succeeds while `ftruncate` on
// that descriptor answers EINVAL, so applying the held-back flag without
// asking whether the file is a regular one breaks writing to /dev/null.
// Darwin accepts both — which is why removing the guard survives on a Mac and
// dies on Linux, and why this test is not evidence on one machine alone.
func TestAFrontEndWriteToSomethingWithNoLengthStillWorks(t *testing.T) {
	b, _ := policyGate(t, "default allow")
	if err := b.WriteFile(t.Context(), boundary.File{Path: os.DevNull}, []byte("discarded")); err != nil {
		t.Errorf("writing to %s under a gate gave %v", os.DevNull, err)
	}
}

// TestAnUngatedFrontEndOpenIsNotVerified. A shell nobody gave a policy makes
// exactly the calls it always made: one open, no fcntl, and no second
// consultation of a gate that is not there.
func TestAnUngatedFrontEndOpenIsNotVerified(t *testing.T) {
	_, link, _ := fixture(t, "ORDINARY")
	var b boundary.Boundary
	body, err := b.ReadFile(t.Context(), link)
	if err != nil {
		t.Fatalf("an ungated read through a link gave %v", err)
	}
	if string(body) != "ORDINARY" {
		t.Errorf("read %q, want the target's contents", body)
	}
}

// TestAFrontEndParentIsMadeOnlyOnceTheGateHasAllowed.
//
// The directory a history file or a block store goes in is made by this
// package rather than by the caller, and the ordering is the reason: a caller
// that made it first would have created a tree under a path the policy went on
// to refuse. A refused write leaves nothing behind at all.
func TestAFrontEndParentIsMadeOnlyOnceTheGateHasAllowed(t *testing.T) {
	dir := t.TempDir()
	denied := filepath.Join(dir, "denied", "deeper", "file")
	b, _ := policyGate(t, "default allow", fmt.Sprintf("deny write %s/denied/**", dir))

	err := b.WriteFile(t.Context(), boundary.File{Path: denied, Perm: 0o600, Parents: true}, []byte("x"))
	if !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("the refused write gave %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "denied")); !os.IsNotExist(err) {
		t.Errorf("a refused write left a directory behind: %v", err)
	}

	// And an allowed one does make it, or the field would be doing nothing.
	allowed := filepath.Join(dir, "kept", "deeper", "file")
	if err := b.WriteFile(t.Context(), boundary.File{Path: allowed, Perm: 0o600, Parents: true}, []byte("x")); err != nil {
		t.Fatalf("the allowed write gave %v", err)
	}
	if _, err := os.Stat(allowed); err != nil {
		t.Errorf("the allowed write did not land: %v", err)
	}
}
