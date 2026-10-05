// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// Without a completion system loaded, the editor's own file listing marks
// each file as `ls -F` does under LIST_TYPES, marks none once the option is
// unset — at the prompt, and on the next Tab — and inserts a link to a
// directory as a directory either way (#6179).
//
// Measured 2026-10-05 against zsh 5.9.2 on a pseudo-terminal with no
// `compinit`, over the same six names: `dlink@  exe*    fifo|   link@   plain
// sub/`, then after `unsetopt listtypes` `dlink   exe     fifo    link
// plain   sub`. This drew `dlink/  exe     fifo    link    plain   sub/`
// both times.
func TestTheEditorsOwnFileListingFollowsListTypes(t *testing.T) {
	control, screen := widgetSession(t, "")
	// widgetSession's scratch HOME, which is the session's directory.
	fx := filepath.Join(os.Getenv("HOME"), "fx")
	for _, step := range []func() error{
		func() error { return os.Mkdir(fx, 0o755) },
		func() error { return os.Mkdir(filepath.Join(fx, "sub"), 0o755) },
		func() error { return os.WriteFile(filepath.Join(fx, "plain"), nil, 0o644) },
		func() error { return os.WriteFile(filepath.Join(fx, "exe"), nil, 0o755) },
		func() error { return syscall.Mkfifo(filepath.Join(fx, "fifo"), 0o644) },
		func() error { return os.Symlink("plain", filepath.Join(fx, "link")) },
		func() error { return os.Symlink("sub", filepath.Join(fx, "dlink")) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct{ keys, want string }{
		{"ls fx/\t", "dlink@  exe*    fifo|   link@   plain   sub/"},
		// A marker the typed line cannot contain, and then the prompt after
		// it: a listing redraws the prompt too, so the mark alone could be
		// the one drawn under the first listing.
		{"\x15unsetopt listtypes; print -r -- LT$((1+1))\r", "LT2"},
		{"", widgetMark},
		{"ls fx/\t", "dlink   exe     fifo    link    plain   sub"},
		{"\x15ls fx/dl\t", "ls fx/dlink/"},
		{"\x15", ""},
	}
	for _, step := range steps {
		if step.keys == "" && step.want == "" {
			continue
		}
		if _, err := control.WriteString(step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if step.want == "" {
			continue
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%q\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
