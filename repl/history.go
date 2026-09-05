// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/secret"
)

// The history that outlives the session.
//
// A shell that forgets everything the moment it exits is one nobody keeps
// using, and this is the cheapest part of it: a file of lines, read at the
// start and appended to at the end.
//
// Appended rather than rewritten. Two shells open at once both want to add to
// it, and a rewrite makes the last one to exit the only one that happened —
// which is the bug every shell's history has had at some point.

// defaultHistorySize is how many lines are kept when HISTFILESIZE says
// nothing. Enough to be worth searching and small enough to read at startup
// without thinking about it.
const defaultHistorySize = 1000

// historyFile is where the session's lines are kept, and how many.
type historyFile struct {
	path string
	size int
	// bound is the session's gate and event sink, because this file is
	// inside the boundary rather than beside it: HISTFILE is a shell
	// variable, so the path is one a line typed at the prompt can change,
	// and an open a script can aim is an open a policy is entitled to refuse.
	// A zero Boundary allows and records nothing, which is every session that
	// was never given a policy.
	bound boundary.Boundary
}

// historyFrom reads the settings a session should use.
//
// HISTFILE names the file and an empty one turns the history off, which is how
// a shell is told not to record anything — a session in a directory someone
// does not want remembered. So does having no HOME to put a default under.
func historyFrom(get func(string) (string, bool), home string) historyFile {
	path, ok := get("HISTFILE")
	if !ok {
		if home == "" {
			return historyFile{}
		}
		path = filepath.Join(home, ".sh_history")
	}
	// An empty path is what turns the history off, and both load and save
	// check for it — so there is nothing to do here but carry it through.
	size := defaultHistorySize
	if v, ok := get("HISTFILESIZE"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			size = n
		}
	}
	return historyFile{path: path, size: size}
}

// load reads the lines a previous session left.
func (h historyFile) load(ctx context.Context) []string {
	if h.path == "" || h.size == 0 {
		// Nothing to open, so nothing to ask a policy about: a history that
		// is turned off is not an access that was refused.
		return nil
	}
	if !h.bound.Open(ctx, h.path, false) {
		// A refused history reads as no history, which is what a missing
		// file already means here — the session starts empty rather than
		// failing to start.
		return nil
	}
	// A missing file is not an error, and neither is no file at all: the
	// first session a person runs has no history, and a history turned off
	// has no path — the open fails either way and there is nothing to read.
	// Complaining would be the first thing they saw.
	f, err := os.Open(h.path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	// A line longer than the scanner's default is not a reason to lose the
	// file; a pasted command can be very long.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if line := sc.Text(); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > h.size {
		lines = lines[len(lines)-h.size:]
	}
	return lines
}

// save appends what this session added.
//
// Only what it added: the lines it read at the start are already in the file,
// and writing them again would double it every time a shell is opened.
func (h historyFile) save(ctx context.Context, added []string) error {
	added = withoutCredentials(added)
	if h.path == "" || h.size == 0 || len(added) == 0 {
		return nil
	}
	if !h.bound.Open(ctx, h.path, true) {
		// Refused, and silently: the session is ending, there is nobody left
		// to tell, and a policy that hid the file meant for it not to be
		// written. The sink has the refusal.
		return nil
	}
	if dir := filepath.Dir(h.path); dir != "" {
		// The directory may not exist on a first run — a HISTFILE somewhere
		// deliberate rather than in a home that is already there.
		_ = os.MkdirAll(dir, 0o700)
	}
	// 0600: a shell history is a record of what someone typed, which is not
	// something to leave readable by everyone on the machine.
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, line := range added {
		_, _ = w.WriteString(line)
		_ = w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// withoutCredentials drops the lines that carry a secret.
//
// This is the write path, and putting the rule here rather than only where a
// line is typed is the point: the file is the thing that outlives the
// session, gets copied into a backup, gets read by whoever ends up with the
// machine. Everything that reaches disk goes through this function, so
// "the history file never held a credential" is a property of the file rather
// than a property of one loop remembering to ask.
//
// Rejected rather than redacted, which is the split the rules are built for.
// A command line carrying a secret usually *is* the secret — `export
// TOKEN=…` is nothing else — so a redacted skeleton of it recalls nothing
// anyone wanted, and dropping the line whole is proportionate. Output is the
// opposite case and is redacted instead; the same table answers both.
//
// Silent here, deliberately. A history that quietly loses lines is its own
// confusion, so the person is told at the moment they type the line — see
// Shell.recording — where the notice lands next to the thing it is about
// instead of arriving in a rush as the shell exits.
func withoutCredentials(added []string) []string {
	kept := added[:0:0]
	for _, line := range added {
		if _, found := secret.Default().Match(line); found {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}
