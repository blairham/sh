// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package wild sweeps the shell scripts installed on the machine and reports
// which of them this parser cannot read.
//
// It exists because the corpus cannot find this class of bug. Every case in
// the corpus is a snippet someone wrote to pin one behavior down, so the
// corpus proves that what it *asks about* is right and says nothing about
// what nobody thought to ask. Positional parameters were missing entirely
// while the corpus stood at 100%: it invokes everything with `-c` and no
// operands, so `$#` is legitimately 0 in every case it has.
//
// Scripts in the wild ask different questions, in bulk, and they were written
// without knowing this implementation exists. Three real gaps came out of one
// afternoon of running them by hand — which is three more than the corpus
// found, and the reason this is a target rather than a habit.
package wild

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Result is what one script did.
type Result struct {
	Path string
	// Err is why our parser refused it, or nil.
	Err error
	// NotShell records that the reference shell refused it too, which means
	// the file is not the shell script its shebang claims — a Tcl or Perl
	// program that execs its real interpreter from the first line is the
	// usual case, and those must not be counted against us.
	NotShell bool
}

// Report is a whole sweep.
type Report struct {
	Scanned  int
	Parsed   int
	NotShell int
	Failures []Result
}

// DefaultDirs are where a machine keeps its scripts. Missing ones are skipped,
// so the same list serves a mac and a container.
var DefaultDirs = []string{
	"/bin", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin",
	"/opt/homebrew/bin", "/opt/homebrew/sbin",
}

// Sweep parses every shell script it finds and grades this parser against the
// reference for the ones it refuses.
//
// The reference is what makes the result trustworthy: a file whose first line
// says `#!/bin/sh` is not necessarily shell, and refusing one that the
// reference also refuses says nothing about us.
func Sweep(ctx context.Context, dirs []string, dialect syntax.Dialect, reference string) Report {
	var rep Report
	for _, path := range Find(dirs) {
		rep.Scanned++
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if _, perr := syntax.Parse(string(src), dialect); perr == nil {
			rep.Parsed++
			continue
		} else if !accepts(ctx, reference, path) {
			rep.NotShell++
			continue
		} else {
			rep.Failures = append(rep.Failures, Result{Path: path, Err: perr})
		}
	}
	sort.Slice(rep.Failures, func(i, j int) bool { return rep.Failures[i].Path < rep.Failures[j].Path })
	return rep
}

// Find is every readable regular file in dirs whose first line names a shell.
func Find(dirs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			if seen[path] || !isShellScript(path) {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// isShellScript reads the shebang. A symbolic link is followed, which is how
// most of these are installed.
func isShellScript(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var head [128]byte
	n, _ := f.Read(head[:])
	line, _, _ := strings.Cut(string(head[:n]), "\n")
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		return false
	}
	// `#!/bin/sh`, `#!/bin/bash -e`, `#!/usr/bin/env bash`. The interpreter
	// is the last path component of the first word, or the word after `env`.
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return false
	}
	name := filepath.Base(fields[0])
	if name == "env" && len(fields) > 1 {
		name = filepath.Base(fields[1])
	}
	return name == "sh" || name == "bash"
}

// accepts reports whether the reference shell parses the file.
//
// `-n` reads without running, which is the whole reason a reference is usable
// here at all: nothing in the sweep executes anything.
func accepts(ctx context.Context, reference, path string) bool {
	if reference == "" {
		return true
	}
	return exec.CommandContext(ctx, reference, "-n", path).Run() == nil
}
