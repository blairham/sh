// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/blairham/sh/interp"
)

// inherited opens a file holding text and hands it back for a test to put in
// a Runner's inherited table, which is what a caller's `3<&0` amounts to: a
// descriptor already open when the shell started, that no line of the script
// could have opened for itself.
func inherited(t *testing.T, text string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inherited")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// A descriptor the shell was started holding is the script's to read, and it
// has to be published or it does not exist: the interpreter models the table,
// so `exec <&3` on a descriptor nothing here opened said "3: bad file
// descriptor" where every shell measured dups it silently.
func TestADescriptorTheShellWasStartedWithIsReadable(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"read through it", `read x <&3; echo "got:$x"`},
		{"move it onto standard input", `exec <&3; read x; echo "got:$x"`},
		{"duplicate it first", `exec 4<&3; read x <&4; echo "got:$x"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := inherited(t, "hello\n")
			out, st := run(t, tc.src, func(r *Runner) {
				r.InheritedFiles = []*os.File{f}
			})
			if out != "got:hello\n" {
				t.Errorf("got %q, want %q", out, "got:hello\n")
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}

// The table is read by number rather than packed, which is the layout the
// outbound half already uses: entry i is descriptor 3+i, and a nil entry is a
// number nothing arrived on. So a caller that opened only 4 leaves 3 as
// unopened as it was, and a script asking for it gets what any other unopened
// number gets.
func TestTheInheritedTableKeepsItsNumbersAndItsGaps(t *testing.T) {
	f := inherited(t, "hello\n")
	out, _ := run(t, `read x <&3; echo "three=$? $x"; read y <&4; echo "four=$? $y"`,
		func(r *Runner) { r.InheritedFiles = []*os.File{nil, f} })
	if !strings.Contains(out, "four=0 hello") {
		t.Errorf("descriptor 4 did not carry the file: %q", out)
	}
	if strings.Contains(out, "three=0") {
		t.Errorf("the gap below it should be as unopened as it was: %q", out)
	}
}

// The two halves of the boundary meet: a descriptor that came in on 3 goes
// out on 3, so a child started by the script reads the caller's file through
// the number the caller used.
func TestAnInheritedDescriptorReachesAnExternalChild(t *testing.T) {
	f := inherited(t, "hello\n")
	out, st := run(t, `/bin/sh -c 'read y <&3; echo "child:$y"'; echo "st=$?"`,
		func(r *Runner) { r.InheritedFiles = []*os.File{f} })
	if st != 0 {
		t.Errorf("status %d, output %q", st, out)
	}
	if !strings.Contains(out, "child:hello") {
		t.Errorf("the child did not read through the inherited descriptor: %q", out)
	}
}

// Closing one closes it for the child too, and that cannot be asked here: a
// file this test opened is close-on-exec, so a child never sees it whatever
// the table says, and a descriptor a *caller* left open cannot be conjured
// inside one process at the number the caller used. It is asked in driver,
// where a second process makes the situation real — see
// TestAClosedInheritedDescriptorIsClosedForAChildToo.

// Publication is recorded and is not gated, and both halves of that are the
// point. There is no construct behind it to refuse — the table is filled
// before the first statement runs — and the descriptor is in the process's
// own table whatever this package decides, so a veto would report more than
// it enforces. What the boundary owes is the record.
func TestPublishingAnInheritedDescriptorIsRecordedAndNeverAsked(t *testing.T) {
	f := inherited(t, "hello\n")
	var mu sync.Mutex
	var seen []Event
	var asked []ActionKind
	out, st := run(t, `read x <&3; echo "got:$x"`, func(r *Runner) {
		r.InheritedFiles = []*os.File{f}
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			mu.Lock()
			defer mu.Unlock()
			asked = append(asked, a.Kind)
			return Deny
		})
		r.Events = SinkFunc(func(_ context.Context, e Event) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, e)
		})
	})
	if out != "got:hello\n" || st != 0 {
		t.Errorf("a gate that refuses everything should not have stopped this: %q status %d", out, st)
	}
	for _, k := range asked {
		if k == ActionInherit {
			t.Error("the gate was asked about an inherited descriptor")
		}
	}
	found := 0
	for _, e := range seen {
		if e.Action.Kind == ActionInherit {
			found++
			if e.Kind != EventAccess {
				t.Errorf("event kind %v, want %v", e.Kind, EventAccess)
			}
			if e.Action.Path != "/dev/fd/3" {
				t.Errorf("event path %q, want %q", e.Action.Path, "/dev/fd/3")
			}
		}
	}
	if found != 1 {
		t.Errorf("%d inherit records, want exactly 1", found)
	}
}

// Once, however many times the shell is re-entered. A subshell is a cloned
// Runner rather than a process here, and a front end feeding the shell one
// chunk at a time calls in again for every line, so the guard has to travel
// with the clone and survive the next chunk.
func TestAnInheritedDescriptorIsPublishedOnlyOnce(t *testing.T) {
	f := inherited(t, "one\ntwo\n")
	var mu sync.Mutex
	records := 0
	out, _ := run(t, `( read a <&3; echo "sub:$a" ); read b <&3; echo "outer:$b"`,
		func(r *Runner) {
			r.InheritedFiles = []*os.File{f}
			r.Events = SinkFunc(func(_ context.Context, e Event) {
				mu.Lock()
				defer mu.Unlock()
				if e.Action.Kind == ActionInherit {
					records++
				}
			})
		})
	if records != 1 {
		t.Errorf("%d inherit records, want exactly 1: %q", records, out)
	}
}

// The ceiling the outbound table already has applies inbound too: an entry
// past it describes a descriptor no child could be handed, so publishing it
// would give a script a number it could read and never pass on.
func TestAnInheritedDescriptorPastTheCeilingIsNotPublished(t *testing.T) {
	f := inherited(t, "hello\n")
	// 1025 is the first number past the bound; entry i is descriptor 3+i.
	files := make([]*os.File, 1025-3+1)
	files[len(files)-1] = f
	out, _ := run(t, `read x <&1025; echo "st=$?"`,
		func(r *Runner) { r.InheritedFiles = files })
	if strings.Contains(out, "st=0") {
		t.Errorf("a descriptor past the ceiling should not be readable: %q", out)
	}
}
