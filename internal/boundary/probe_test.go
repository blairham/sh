// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// The front end asks about paths as well as opening and listing them, and
// until #1824 it had nothing to ask with: `Boundary` offered ReadDir and
// OpenFile, interp's own probes had raised ActionStat since fsgate.go, and a
// dialect's builtin had AllowProbe — so the one asker of the three that is the
// front end could put two of the four questions and not the other two.
//
// Every fixture below is built under t.TempDir(). None points into a home.

// TestARefusedProbeAnswersAsAnAbsentPathDoes is the rule the whole seam turns
// on, and it is a security property rather than a nicety: a refusal a caller
// could tell from a missing path is an oracle for what the policy hides, which
// Tab would read out one prefix at a time.
func TestARefusedProbeAnswersAsAnAbsentPathDoes(t *testing.T) {
	t.Parallel()
	dir := physical(t, t.TempDir())
	hidden := filepath.Join(dir, "hidden")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	b, asked, got := listing(t, func(p string) bool { return p == hidden })

	_, refused := b.Stat(context.Background(), hidden)
	_, absent := b.Stat(context.Background(), filepath.Join(dir, "nothing-here"))
	if refused == nil {
		t.Fatal("a refused probe answered with an info, want an error")
	}
	// The same operation, the same errno, and nothing in the message that the
	// absent one does not also say.
	var rp, ap *fs.PathError
	if !errors.As(refused, &rp) || !errors.As(absent, &ap) {
		t.Fatalf("refused %v / absent %v, want both to be *fs.PathError", refused, absent)
	}
	if rp.Op != ap.Op || !errors.Is(rp.Err, syscall.ENOENT) || !errors.Is(ap.Err, syscall.ENOENT) {
		t.Errorf("refused %+v against absent %+v, want the same op and the same ENOENT", rp, ap)
	}
	// And the refusal is not ErrRefused either, which an open's is: a caller
	// that tested for it would have the answer the errno withholds.
	if errors.Is(refused, boundary.ErrRefused) {
		t.Error("a refused probe came back as ErrRefused, which names the policy")
	}
	// The gate was asked, with the kind interp's own probes raise.
	if len(*asked) != 2 || (*asked)[0].Kind != interp.ActionStat || (*asked)[0].Path != hidden {
		t.Errorf("asked %+v, want an ActionStat about %q first", *asked, hidden)
	}
	// Refused and allowed are both recorded, and the session is on them.
	if len(*got) != 2 || (*got)[0].Kind != interp.EventDenied || (*got)[1].Kind != interp.EventAccess {
		t.Errorf("events %+v, want a denial and then an access", *got)
	}
	for _, e := range *got {
		if e.Session != "SESSIONUNDERTEST" {
			t.Errorf("event %+v carries session %q, want the boundary's", e, e.Session)
		}
	}
}

// An allowed probe answers what the kernel answers, which is the half that
// says the seam is a consultation rather than a replacement.
func TestAnAllowedProbeAnswersTheKernel(t *testing.T) {
	t.Parallel()
	dir := physical(t, t.TempDir())
	b, _, _ := listing(t, nil)
	info, err := b.Stat(context.Background(), dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Stat(%q) = %v, %v; want a directory", dir, info, err)
	}
}

// A Boundary with neither a gate nor a sink is the shell that was given no
// policy, and it costs nothing: the probe is the os call it always was, with
// no id minted for a record nobody will read.
func TestAProbeWithNothingWatchingIsThePlainCall(t *testing.T) {
	t.Parallel()
	dir := physical(t, t.TempDir())
	info, err := boundary.Boundary{}.Stat(context.Background(), dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Stat(%q) = %v, %v; want a directory", dir, info, err)
	}
}

// The modify seam is the other half, and it is refused *loudly*: unlike a
// probe there is no honest way to carry on, so the caller is told no rather
// than told nothing.
func TestAModifyIsAskedAsAWriteAndAnsweredAsAnAnswer(t *testing.T) {
	t.Parallel()
	dir := physical(t, t.TempDir())
	target := filepath.Join(dir, "history")
	b, asked, got := listing(t, func(p string) bool { return p == target })

	if b.Modify(context.Background(), target) {
		t.Error("a refused modify answered true")
	}
	if !b.Modify(context.Background(), filepath.Join(dir, "other")) {
		t.Error("an allowed modify answered false")
	}
	// An ActionOpen with Write set rather than a kind of its own, which is
	// the decision AllowModify made and the reason a `deny write` policy
	// covers a rename: a new kind would be one every gate had to learn
	// before "deny write" meant what it says.
	if len(*asked) != 2 {
		t.Fatalf("asked %+v, want two consultations", *asked)
	}
	for _, a := range *asked {
		if a.Kind != interp.ActionOpen || !a.Write {
			t.Errorf("asked %+v, want an ActionOpen with Write set", a)
		}
	}
	if len(*got) != 2 || (*got)[0].Kind != interp.EventDenied || (*got)[1].Kind != interp.EventAccess {
		t.Errorf("events %+v, want a denial and then an access", *got)
	}
}

// Each consultation and the record of it carry the same id, which is the
// promise Action.ID makes and the thing that lets an audit stream join them.
func TestAProbeAndItsRecordShareAnId(t *testing.T) {
	t.Parallel()
	dir := physical(t, t.TempDir())
	b, asked, got := listing(t, nil)
	if _, err := b.Stat(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if !b.Modify(context.Background(), filepath.Join(dir, "x")) {
		t.Fatal("modify refused with an allowing gate")
	}
	if len(*asked) != 2 || len(*got) != 2 {
		t.Fatalf("asked %d, recorded %d; want two of each", len(*asked), len(*got))
	}
	for i := range *asked {
		if id := (*asked)[i].ID; id == "" || id != (*got)[i].Action.ID {
			t.Errorf("consultation %d asked with id %q and recorded id %q",
				i, id, (*got)[i].Action.ID)
		}
	}
	if (*asked)[0].ID == (*asked)[1].ID {
		t.Error("two accesses were given the same id")
	}
}
