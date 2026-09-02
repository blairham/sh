// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
func (h historyFile) load() []string {
	if h.size == 0 {
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
func (h historyFile) save(added []string) error {
	if h.path == "" || h.size == 0 || len(added) == 0 {
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
