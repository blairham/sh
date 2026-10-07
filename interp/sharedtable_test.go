// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"sync"
	"testing"
)

// sharedTableRow is one copy-on-write table, written through the runner's own
// write path and read back by name.
type sharedTableRow struct {
	name  string
	seed  func(r *Runner)
	write func(r *Runner, tag string)
	read  func(r *Runner) string
}

// sharedTableRows covers every table sharedtable.go copies on write, and for
// the two that hold more than one kind of write, each kind: a write into an
// element as well as to the table of names, and a hit as well as an entry.
func sharedTableRows() []sharedTableRow {
	return []sharedTableRow{
		{
			name: "an associative array's element",
			seed: func(r *Runner) { r.setAssocElem("tbl", "k", "parent") },
			write: func(r *Runner, tag string) {
				r.setAssocElem("tbl", "k", tag)
			},
			read: func(r *Runner) string { return r.AssocArrays["tbl"]["k"].scalar() },
		},
		{
			name: "an associative array's element, unset",
			seed: func(r *Runner) { r.setAssocElem("tbl", "gone", "parent") },
			write: func(r *Runner, tag string) {
				r.unsetAssocElem("tbl", "gone")
				r.setAssocElem("tbl", "gone-"+tag, tag)
			},
			read: func(r *Runner) string { return fmt.Sprint(r.AssocArrays["tbl"].keys()) },
		},
		{
			name: "the table of associative arrays",
			seed: func(r *Runner) { r.setAssocElem("old", "k", "v") },
			write: func(r *Runner, tag string) {
				r.dropAssocTable("old")
				r.markAssoc("new-" + tag)
			},
			read: func(r *Runner) string {
				var names []string
				for name := range r.AssocArrays {
					names = append(names, name)
				}
				return fmt.Sprint(len(names), r.AssocArrays["old"] != nil)
			},
		},
		{
			name:  "a new associative array",
			seed:  func(r *Runner) { r.setAssocElem("tbl", "k", "v") },
			write: func(r *Runner, tag string) { r.markAssoc("new-" + tag) },
			read:  func(r *Runner) string { return fmt.Sprint(len(r.AssocArrays)) },
		},
		{
			name: "a function's origin",
			seed: func(r *Runner) { r.recordFunctionOrigin("f", funcOrigin{file: "parent"}) },
			write: func(r *Runner, tag string) {
				r.recordFunctionOrigin("f", funcOrigin{file: tag})
				r.recordFunctionOrigin("g-"+tag, funcOrigin{file: tag})
			},
			read: func(r *Runner) string { return fmt.Sprint(r.funcOrigins["f"].file, len(r.funcOrigins)) },
		},
		{
			name: "a function's origin, removed",
			seed: func(r *Runner) { r.recordFunctionOrigin("f", funcOrigin{file: "parent"}) },
			write: func(r *Runner, _ string) {
				r.removeFunctionQuietly("f")
			},
			read: func(r *Runner) string { return r.funcOrigins["f"].file },
		},
		{
			name: "a command hash entry",
			seed: func(r *Runner) { r.putHashedCommand("ls", "/parent/ls", 0) },
			write: func(r *Runner, tag string) {
				r.putHashedCommand("ls", "/"+tag+"/ls", 0)
				r.putHashedCommand("new-"+tag, "/"+tag, 0)
			},
			read: func(r *Runner) string { return fmt.Sprint(r.cmdHash["ls"].path, r.cmdHashOrder) },
		},
		{
			name: "a command hash entry, forgotten",
			seed: func(r *Runner) { r.putHashedCommand("ls", "/parent/ls", 0) },
			write: func(r *Runner, _ string) {
				r.forgetHashedCommand("ls")
			},
			read: func(r *Runner) string { return fmt.Sprint(r.cmdHash["ls"].path, r.cmdHashOrder) },
		},
		{
			name: "a command hash hit",
			seed: func(r *Runner) { r.putHashedCommand("ls", "/parent/ls", 0) },
			write: func(r *Runner, _ string) {
				r.hashCommandHit("ls")
				r.hashCommandHit("ls")
			},
			read: func(r *Runner) string { return fmt.Sprint(r.hashedHits("ls", r.cmdHash["ls"])) },
		},
	}
}

// TestASubshellWriteToASharedTableStaysInTheSubshell is the guarantee
// clonetables.go exists for, held for the tables that are no longer copied at
// the clone: a write on either side of a clone reaches neither the other side
// nor, when the two run at once, the same map.
//
// Three halves. The subshell writes and the parent must not see it; the
// parent writes and the subshell must not see it; and both write at once on
// two goroutines, which is a process substitution's shape — there the race
// detector is the instrument, since a write into a table the other side holds
// is a data race whether or not the values happen to collide. The concurrent
// half also checks each side reads back its own write, so a run where a write
// landed nowhere does not pass as one where nothing leaked.
func TestASubshellWriteToASharedTableStaysInTheSubshell(t *testing.T) {
	for _, row := range sharedTableRows() {
		t.Run(row.name+"/subshell writes", func(t *testing.T) {
			parent := newTestRunner(t, &Runner{})
			row.seed(parent)
			before := row.read(parent)
			child := parent.clone()
			row.write(child, "child")
			if got := row.read(child); got == before {
				t.Fatalf("the subshell's write did not take: still %q", got)
			}
			if got := row.read(parent); got != before {
				t.Errorf("a subshell's write reached the parent: %q became %q", before, got)
			}
		})
		t.Run(row.name+"/parent writes", func(t *testing.T) {
			parent := newTestRunner(t, &Runner{})
			row.seed(parent)
			child := parent.clone()
			before := row.read(child)
			row.write(parent, "parent2")
			if got := row.read(parent); got == before {
				t.Fatalf("the parent's write did not take: still %q", got)
			}
			if got := row.read(child); got != before {
				t.Errorf("the parent's write reached the subshell: %q became %q", before, got)
			}
		})
		t.Run(row.name+"/both at once", func(t *testing.T) {
			parent := newTestRunner(t, &Runner{})
			row.seed(parent)
			before := row.read(parent)
			child := parent.clone()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 50 {
					row.write(child, "child")
					_ = row.read(child)
				}
			}()
			for range 50 {
				row.write(parent, "parent2")
				_ = row.read(parent)
			}
			wg.Wait()
			if row.read(parent) == before || row.read(child) == before {
				t.Fatalf("a write did not take: parent %q, subshell %q", row.read(parent), row.read(child))
			}
		})
	}
}

// TestAFinishedSubshellLeavesItsParentTheTable is the half of copy-on-write
// that makes it worth having: a clone that has given its hold back leaves the
// parent writing its own table in place, rather than copying it as it would
// while the clone still held it. Without the release every `$( … )` would
// cost the parent a copy on its next write, which is the cost the clone used
// to pay up front.
func TestAFinishedSubshellLeavesItsParentTheTable(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	parent.setAssocElem("tbl", "k", "v")
	parent.recordFunctionOrigin("f", funcOrigin{file: "x"})
	parent.putHashedCommand("ls", "/bin/ls", 0)
	assocs, origins, hash := parent.AssocArrays, parent.funcOrigins, parent.cmdHash

	child := parent.clone()
	child.releaseSharedTables()
	parent.setAssocElem("tbl", "k", "w")
	parent.recordFunctionOrigin("f", funcOrigin{file: "y"})
	parent.putHashedCommand("ls", "/usr/bin/ls", 0)

	if fmt.Sprintf("%p", parent.AssocArrays) != fmt.Sprintf("%p", assocs) {
		t.Error("the parent copied its associative arrays after the subshell had given them back")
	}
	if fmt.Sprintf("%p", parent.funcOrigins) != fmt.Sprintf("%p", origins) {
		t.Error("the parent copied its function origins after the subshell had given them back")
	}
	if fmt.Sprintf("%p", parent.cmdHash) != fmt.Sprintf("%p", hash) {
		t.Error("the parent copied its command hash after the subshell had given it back")
	}

	// And the control: while a clone still holds them, the same writes copy.
	held := parent.clone()
	parent.setAssocElem("tbl", "k", "z")
	if fmt.Sprintf("%p", parent.AssocArrays) == fmt.Sprintf("%p", held.AssocArrays) {
		t.Error("the parent wrote in place into associative arrays a subshell still holds")
	}
	if got := held.AssocArrays["tbl"]["k"].scalar(); got != "w" {
		t.Errorf("the held subshell reads %q, want the value at the clone, w", got)
	}
}

// TestAHitOnASharedHashIsCountedBesideIt pins the one write that does not copy
// the command hash: a hit on a shared table is counted beside it, read back
// by the listing's count, and added in when the table becomes this runner's.
// The count is the subshell's and never the parent's.
func TestAHitOnASharedHashIsCountedBesideIt(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	parent.putHashedCommand("ls", "/bin/ls", 1)
	child := parent.clone()
	shared := fmt.Sprintf("%p", child.cmdHash)

	child.hashCommandHit("ls")
	child.hashCommandHit("ls")
	if got := fmt.Sprintf("%p", child.cmdHash); got != shared {
		t.Error("a hit copied the shared command hash")
	}
	if got := child.hashedHits("ls", child.cmdHash["ls"]); got != 3 {
		t.Errorf("the subshell counts %d hits, want 3", got)
	}
	if got := parent.hashedHits("ls", parent.cmdHash["ls"]); got != 1 {
		t.Errorf("the parent counts %d hits after the subshell's, want 1", got)
	}

	// A write of the subshell's own makes the table its own, hits and all.
	child.putHashedCommand("cat", "/bin/cat", 0)
	if got := child.cmdHash["ls"].hits; got != 3 || child.cmdHashHits != nil {
		t.Errorf("the owned table holds %d hits with %v beside it, want 3 and nothing", got, child.cmdHashHits)
	}
	if got := parent.cmdHash["ls"].hits; got != 1 {
		t.Errorf("the parent's entry holds %d hits, want 1", got)
	}
}

// TestATableMovedWhileSharedIsNotTheSubshells is the hazard copy-on-write adds
// that a copy at the clone never had: a table taken out of the live set and
// put back under another name. Taken from a set a subshell still shares, it
// would go into this shell's own copy as the very map the subshell reads, and
// every later write to it would reach the subshell — and race it, where the
// subshell runs on a goroutine.
func TestATableMovedWhileSharedIsNotTheSubshells(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	parent.setAssocElem("src", "k", "before")
	child := parent.clone()

	if st := parent.moveParameter("dst", "src"); st != 0 {
		t.Fatalf("the move returned %d", st)
	}
	parent.setAssocElem("dst", "k", "after")

	if got := parent.AssocArrays["dst"]["k"].scalar(); got != "after" {
		t.Fatalf("the moved table reads %q, want after", got)
	}
	if got := child.AssocArrays["src"]["k"].scalar(); got != "before" {
		t.Errorf("a write to the moved table reached the subshell: src[k] is %q, want before", got)
	}
}
