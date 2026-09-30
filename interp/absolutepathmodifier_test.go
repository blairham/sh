// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"testing"
)

// symlinkTree builds the tree every row below is measured against.
//
// Built here rather than pointing at anything on the machine: `/tmp` is a
// symlink on macOS and is not on Linux, so a row written against it passes on
// one runner and says nothing on the other. These links are ours, so the rows
// mean the same thing everywhere.
//
//	real/dir/file   an ordinary directory and file
//	link -> real            a link to a directory
//	deep -> real/dir        a link to a path, so `deep/..` can part from it
//	rel  -> ../<base>/real  a link with a relative target
//	dangling -> nowhere     a link whose target does not exist
func symlinkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// The temp directory itself may be reached through a link — /var on
	// macOS is one — so the rows compare against its resolved name.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolving the temp directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(resolved, "real", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resolved, "real", "dir", "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, l := range []struct{ name, target string }{
		{"link", "real"},
		{"deep", "real/dir"},
		{"rel", "../" + filepath.Base(resolved) + "/real"},
		{"dangling", "nowhere"},
	} {
		if err := os.Symlink(l.target, filepath.Join(resolved, l.name)); err != nil {
			t.Fatal(err)
		}
	}
	return resolved
}

// Where `:P` says a path is: absolute, resolved as far as it resolves, and
// the rest left as written.
//
// See histexpand.Chars.AbsolutePathModifier for the reference's own grid.
// **Two plausible implementations pass the suite chunk and are wrong**, which
// is why these rows exist rather than the chunk alone: the chunk's operand is
// already absolute and already resolves, so an identity function satisfies it.
func TestResolvedAsFarAsItGoes(t *testing.T) {
	t.Parallel()
	base := symlinkTree(t)
	// Through the helper, which keeps a Dir a test set and adds the TMPDIR
	// and the CleanUp every Runner here is meant to have.
	r := newTestRunner(t, &Runner{Dir: base})
	for _, c := range []struct{ name, in, want string }{
		{"all of it resolves", base + "/real/dir", base + "/real/dir"},
		{"through a link to a directory", base + "/link/dir", base + "/real/dir"},
		{"a link to a path", base + "/deep", base + "/real/dir"},
		{"a link with a relative target", base + "/rel/dir", base + "/real/dir"},
		{"a dot component", base + "/real/./dir", base + "/real/dir"},
		{"a trailing slash", base + "/real/dir/", base + "/real/dir"},
		// The rows a strict resolver refuses.
		{"a missing tail is kept", base + "/real/nope/deep", base + "/real/nope/deep"},
		{"a link in front of a missing tail is still followed", base + "/link/nope", base + "/real/nope"},
		{"a dangling link is kept as written", base + "/dangling", base + "/dangling"},
		{"a dangling link with a tail", base + "/dangling/more", base + "/dangling/more"},
		// And past the point where resolution stops, the remainder is
		// verbatim — `..` included, which is why this is not a clean.
		{"a dotdot behind a dangling link is not cleaned", base + "/dangling/..", base + "/dangling/.."},
		// The row a textual implementation cannot fake: `deep` is followed
		// first, so the parent is the link's *target's* parent.
		{"a dotdot is physical, not lexical", base + "/deep/..", base + "/real"},
		{"and physical for what follows it", base + "/deep/../dir", base + "/real/dir"},
		{"a link to a directory, then dotdot", base + "/link/..", base},
		// Relative operands resolve against the runner's directory, and the
		// dotdot stays physical there too.
		{"a relative path", "real/dir", base + "/real/dir"},
		{"a relative path with a dot", "./real/dir", base + "/real/dir"},
		{"a bare dot is the directory", ".", base},
		{"a relative physical dotdot", "deep/..", base + "/real"},
		// The root, and a dotdot that would climb past it.
		{"the root", "/", "/"},
		{"dotdot past the root clamps", "/../..", "/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := r.resolvedAsFarAsItGoes(c.in); got != c.want {
				t.Errorf("%q resolved to %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A runner with no directory leaves a relative word alone rather than
// borrowing the process's directory.
//
// The rule this package keeps everywhere: two Runners in one program have two
// directories and neither is the process's. An absolute word still resolves,
// because it needs no directory to start from.
func TestResolvedAsFarAsItGoesWithoutADirectory(t *testing.T) {
	t.Parallel()
	base := symlinkTree(t)
	// testrunner:bare — the **unset** Dir is the subject: the helper would
	// give it one, and then there would be a directory to resolve against
	// and nothing left to assert.
	r := &Runner{}
	if got := r.resolvedAsFarAsItGoes("real/dir"); got != "real/dir" {
		t.Errorf("a relative word became %q, want it left as written", got)
	}
	if got := r.resolvedAsFarAsItGoes(base + "/link/dir"); got != base+"/real/dir" {
		t.Errorf("an absolute word became %q, want it resolved", got)
	}
}
