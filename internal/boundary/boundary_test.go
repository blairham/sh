// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// touch makes a file for an open to reach, in a directory of the test's own.
// Never in the checkout and never under a real home: what these tests do to a
// path is open it, and one of the paths this package is about is HISTFILE.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A front end's own access says which run it belongs to and which access it is.
//
// These records are the ones most likely to be joined and were the last to
// carry an identity: what this package opens is a shell's own files, and one of
// them is the store that records what the shell ran. Without the session here,
// an audit stream's account of the block index being written could not be lined
// up with the blocks in it.
func TestAFrontEndAccessCarriesTheRunsIdentity(t *testing.T) {
	var asked []interp.Action
	var got []interp.Event
	b := boundary.Boundary{
		Session: "SESSIONUNDERTEST",
		Gate: interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
			asked = append(asked, a)
			return interp.Allow
		}),
		Events: interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) }),
	}
	dir := t.TempDir()
	if _, err := b.ReadFile(context.Background(), touch(t, dir, "one")); err != nil {
		t.Fatalf("the open was refused: %v", err)
	}
	if len(asked) != 1 || len(got) != 1 {
		t.Fatalf("asked %d times and recorded %d events, want one of each", len(asked), len(got))
	}
	if got[0].Session != "SESSIONUNDERTEST" {
		t.Errorf("the record says session %q, want the run's", got[0].Session)
	}
	if asked[0].ID == "" {
		t.Fatal("the gate was consulted about an access with no id")
	}
	if got[0].Action.ID != asked[0].ID {
		t.Errorf("the record names action %q and the gate was asked about %q",
			got[0].Action.ID, asked[0].ID)
	}
	// And a second access is a second id. One id for everything a front end
	// opens would be an identity that identifies nothing.
	if _, err := b.ReadFile(context.Background(), touch(t, dir, "two")); err != nil {
		t.Fatalf("the second open was refused: %v", err)
	}
	if got[1].Action.ID == got[0].Action.ID {
		t.Errorf("both accesses have id %q", got[0].Action.ID)
	}
}

// A refusal is identified too, which is the record that matters most.
func TestARefusedFrontEndAccessCarriesTheIdentity(t *testing.T) {
	var got []interp.Event
	b := boundary.Boundary{
		Session: "SESSIONUNDERTEST",
		Gate: interp.GateFunc(func(context.Context, interp.Action) interp.Decision {
			return interp.Deny
		}),
		Events: interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) }),
	}
	if _, err := b.ReadFile(context.Background(), touch(t, t.TempDir(), "one")); !errors.Is(err, boundary.ErrRefused) {
		t.Fatalf("a denied open answered %v, want a refusal", err)
	}
	if len(got) != 1 || got[0].Kind != interp.EventDenied {
		t.Fatalf("recorded %v, want one denial", got)
	}
	if got[0].Session != "SESSIONUNDERTEST" || got[0].Action.ID == "" {
		t.Errorf("the denial is %+v, want a session and an id on it", got[0])
	}
}

// A shell nobody is watching mints nothing, which is the nil check the rest of
// this package already costs.
func TestAnUnwatchedBoundaryMintsNoIdentity(t *testing.T) {
	dir := t.TempDir()
	var b boundary.Boundary
	if _, err := b.ReadFile(context.Background(), touch(t, dir, "one")); err != nil {
		t.Fatalf("the zero Boundary refused an open: %v", err)
	}
	// Nothing observable to assert against a zero Boundary except that it
	// allowed and recorded nothing, so the claim about the id is made against a
	// Boundary with only a sink — which is the case that must be numbered.
	var got []interp.Event
	b.Events = interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) })
	if _, err := b.ReadFile(context.Background(), touch(t, dir, "two")); err != nil {
		t.Fatalf("a sink-only Boundary refused an open: %v", err)
	}
	if len(got) != 1 || got[0].Action.ID == "" {
		t.Errorf("recorded %+v, want a numbered access once something is watching", got)
	}
}
