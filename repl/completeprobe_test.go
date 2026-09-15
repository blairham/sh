// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// Completion asks one question about an entry the listing already handed it:
// does this symbolic link point at a directory, which decides the trailing
// slash. Until #1824 it asked the os package, because `Boundary` had no probe
// seam — so a policy that hid a tree from every other probe in the shell
// answered this one, and the answer says whether the path behind the link is a
// directory.
//
// The fixture is built under t.TempDir(). None of it points into a home.

// linkFixture is a directory holding two links to directories, so that a run
// can tell a refused probe from the feature being broken. The name returned is
// the one the probe will be *about*: the link, as the listing named it, which
// is the same rule interp's probes follow — a policy is written against the
// name a caller used and not against where it leads.
func linkFixture(t *testing.T) (dir, link string) {
	t.Helper()
	dir = t.TempDir()
	for _, d := range []string{"hidden-target", "shown-target"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][2]string{
		{"hidden-target", "to-hidden"},
		{"shown-target", "to-shown"},
	} {
		if err := os.Symlink(filepath.Join(dir, l[0]), filepath.Join(dir, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	return dir, filepath.Join(dir, "to-hidden")
}

// probing is a completer whose gate refuses one path and records what it was
// asked.
func probing(dir, refuse string) (shellCompleter, *[]interp.Action) {
	asked := &[]interp.Action{}
	gate := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		*asked = append(*asked, a)
		if a.Path == refuse {
			return interp.Deny
		}
		return interp.Allow
	})
	return shellCompleter{
		dir:   dir,
		bound: boundary.Boundary{Gate: gate, Session: "SESSIONUNDERTEST"},
		ctx:   context.Background(),
	}, asked
}

// TestTabAsksTheGateWhereALinkPoints, with both halves from one policy: the
// link into the hidden tree completes as a plain name, and the one beside it
// still gets its slash. Asserting only the first would pass just as well if
// the slash had been dropped from every completion.
func TestTabAsksTheGateWhereALinkPoints(t *testing.T) {
	t.Parallel()
	dir, link := linkFixture(t)
	s, asked := probing(dir, link)

	got := s.paths("to-", nil)
	want := []string{"to-hidden", "to-shown/"}
	if len(got) != len(want) {
		t.Fatalf("Tab offered %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Tab offered %q, want %q", got, want)
			break
		}
	}
	// And the question really went to the gate, as the kind every other probe
	// in this shell raises.
	probe := false
	for _, a := range *asked {
		if a.Kind == interp.ActionStat && a.Path == link {
			probe = true
		}
	}
	if !probe {
		t.Errorf("the gate was asked %+v, want an ActionStat about %q", *asked, link)
	}
}

// A completer with no gate and no sink is the session that was given no
// policy, and the link still resolves — which is what says the seam consults
// rather than replaces.
func TestTabFollowsALinkWhenNothingIsWatching(t *testing.T) {
	t.Parallel()
	dir, _ := linkFixture(t)
	s := shellCompleter{dir: dir, ctx: context.Background()}
	got := s.paths("to-hidden", nil)
	if len(got) != 1 || got[0] != "to-hidden/" {
		t.Errorf("Tab offered %q, want [to-hidden/]", got)
	}
}
