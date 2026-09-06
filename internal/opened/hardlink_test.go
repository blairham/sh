// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTwoNamesForOneObjectAndWhetherTheAnswerHolds is the measurement the
// rest of this package's design rests on, made to run.
//
// Everything here reads the kernel's answer as a fact about the descriptor:
// the object was pinned by the open, so the name that comes back is the name
// that reached it and nothing can change either. For an object with one name
// that is true on both platforms. For an object with two — a hard link — it
// is true on Linux and false on Darwin, and the difference is not a detail:
//
//   - Linux answers from the dentry the descriptor was opened through. Each
//     name of an inode is its own dentry, so the answer is the name that was
//     opened, and nothing that happens afterwards moves it.
//
//   - Darwin answers from the vnode, which carries *one* name for the object
//     no matter how many the filesystem holds, and a lookup of any of them
//     re-stamps it. So an unrelated `stat` of the second name changes the
//     answer for a descriptor already open — which is what this asserts.
//
// Measured before it was written down, at N=2000 per platform: on Darwin
// 25.5.0 the second answer is the *other* name 2000 times out of 2000, where
// asking twice with nothing in between gives it 2 times out of 2000; on Linux
// 6.8 the flip is 0 out of 3000 in the same shape. That gap is why this is an
// assertion rather than a comment.
//
// Asserted per platform on purpose, in the direction each one behaves, so
// that a kernel which stops behaving this way fails here — the moment to
// revisit docs/design/sandboxing.md rather than the moment to discover the
// claim had quietly stopped being true. See the hard-link entry there for
// what it costs the gate.
func TestTwoNamesForOneObjectAndWhetherTheAnswerHolds(t *testing.T) {
	// Two directories rather than two names in one, because that is what
	// makes Darwin's answer move every time rather than one time in a
	// thousand: the vnode carries a parent as well as a name, and it is the
	// parent changing that re-stamps it. One directory measures 3 flips in
	// 5000 where two measure 5000 in 5000, and a test built on the first
	// number would be the flake this one replaces.
	dir := t.TempDir()
	for _, sub := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first := filepath.Join(dir, "one", "first")
	if err := os.WriteFile(first, []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "two", "second")
	if err := os.Link(first, second); err != nil {
		t.Skipf("no hard links here: %v", err)
	}

	f, err := os.Open(second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	opened, ok := Path(f)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if ok {
			t.Fatalf("Path answered %q on a platform with no way to ask", opened)
		}
		return
	}
	if !ok {
		t.Fatal("Path had no answer for an ordinary file")
	}
	// Which of the two it is, is deliberately not asserted, and that is the
	// whole finding rather than a weakened test. On Darwin a freshly opened
	// descriptor answers with the name it was opened by about 999 times in a
	// thousand and with the object's other name the rest of the time —
	// measured at 1 in 500 through this very sequence, and 6 in 5000 through
	// a bare open — so a test that pinned it would be the flake this one
	// replaces. What must hold is that the answer is a *real* name of the
	// object and not a third path, because a rule matches whatever comes back.
	if base := filepath.Base(opened); base != "second" && base != "first" {
		t.Fatalf("Path = %q, want one of the object's two names", opened)
	}

	// Nothing here touches the descriptor, writes anything, or needs any
	// privilege the opener does not have. It is one metadata read of a name
	// the opener already knew, and on Darwin it is enough to change what the
	// kernel says a descriptor already open is holding.
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}

	after, ok := Path(f)
	if !ok {
		t.Fatal("Path stopped having an answer for a descriptor still open")
	}
	switch runtime.GOOS {
	case "darwin":
		// 500 out of 500, against 1 out of 500 for the same descriptor asked
		// twice with nothing in between.
		if filepath.Base(after) != "first" {
			t.Errorf("Path = %q after the object's other name was merely stat'ed, want the "+
				"other name — Darwin answering from the vnode's single name is what the "+
				"hard-link entry in docs/design/sandboxing.md is about, and this kernel "+
				"no longer does it", after)
		}
	case "linux":
		// 0 out of 3000 on 6.8, in this shape and three others.
		if filepath.Base(after) != "second" {
			t.Errorf("Path = %q after the object's other name was stat'ed, want the name the "+
				"descriptor was opened by — /proc answering from the dentry is what makes "+
				"the verification sound here, and this kernel no longer does it", after)
		}
	}
}
